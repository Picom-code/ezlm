package serve

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/picom-code/ezlm/internal/paths"
)

// label is the launchd job name (see testSuffix).
func label() string { return "com.ezlm.server" + testSuffix() }

func plistPath() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Library", "LaunchAgents", label()+".plist")
}

func domain() string { return fmt.Sprintf("gui/%d", os.Getuid()) }

// bootout unloads the agent and waits until launchd has really let go of it:
// bootout returns early, and an immediate bootstrap then fails with error 5.
func bootout() {
	exec.Command("launchctl", "bootout", domain()+"/"+label()).Run()
	for i := 0; i < 50; i++ {
		if exec.Command("launchctl", "print", domain()+"/"+label()).Run() != nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func bootstrap() error {
	var out []byte
	var err error
	for i := 0; i < 3; i++ { // launchd occasionally needs a beat even after print fails
		if out, err = exec.Command("launchctl", "bootstrap", domain(), plistPath()).CombinedOutput(); err == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("launchctl bootstrap: %s", strings.TrimSpace(string(out)))
}

// plistXML is the launch agent Install writes. The log folder must exist
// before launchd starts the job, or the job fails to spawn.
func plistXML(c *Config) string {
	// Run the server through /bin/sh so a Stop survives the next login: the
	// job checks the stopped marker before exec'ing the server (exec keeps
	// launchd's crash-restart behaviour).
	parts := append([]string{c.Server}, c.Args()...)
	for i, p := range parts {
		parts[i] = shellQuote(p)
	}
	script := "test -f " + shellQuote(filepath.Join(paths.Home(), "stopped")) + " || exec " + strings.Join(parts, " ")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>/bin/sh</string>
		<string>-c</string>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>ThrottleInterval</key>
	<integer>10</integer>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, label(), html.EscapeString(script), html.EscapeString(LogPath()), html.EscapeString(LogPath()))
}

// shellQuote quotes one argument for /bin/sh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Install writes a launchd agent that starts at login and restarts on crash,
// then (re)starts it.
func Install(c *Config) error {
	if err := paths.Ensure(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(plistPath()), 0o755); err != nil {
		return err
	}
	// Unload only once the new plist can be written: a failure here must not
	// leave the user with no service at all.
	Uninstall()
	if err := writePlistFile(plistPath(), plistXML(c), LogPath()); err != nil {
		return err
	}
	return bootstrap()
}

// Stop stops the server but keeps it registered to start at next login.
func Stop() error {
	bootout()
	return nil
}

// Start starts a stopped server.
// Start (re)writes the service from the current settings and starts it, so
// a setup made by an older EZLM picks up today's server flags (e.g. --metrics).
func Start() error {
	if c, err := Load(); err == nil {
		return Install(c)
	}
	bootout()
	os.Truncate(LogPath(), 0)
	return bootstrap()
}

// Uninstall stops the server and removes the launch agent.
func Uninstall() error {
	bootout()
	if err := os.Remove(plistPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// EnsureAutostart repairs the login agent for an existing setup: a plist that
// an older EZLM wrote, or one a cleaner removed, is rewritten from today's
// settings. The running job is left alone (reloading it would stop the server);
// launchd picks up the file at the next login.
func EnsureAutostart() {
	c, err := Load()
	if err != nil {
		return
	}
	if _, err := os.Stat(c.Server); err != nil {
		return // no server to start: don't leave a job that fails at login
	}
	if b, err := os.ReadFile(plistPath()); err == nil && string(b) == plistXML(c) {
		return
	}
	if err := writePlistFile(plistPath(), plistXML(c), LogPath()); err != nil {
		return
	}
}
