package serve

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func unitPath() string {
	cfg, err := os.UserConfigDir()
	if err != nil || !filepath.IsAbs(cfg) {
		// systemd ignores a relative XDG_CONFIG_HOME, so writing there would
		// put the unit somewhere systemd never looks.
		home, _ := os.UserHomeDir()
		cfg = filepath.Join(home, ".config")
	}
	return unitPathIn(cfg)
}

func systemctl(args ...string) error {
	out, err := exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}

// Install writes a systemd user unit that restarts on crash and starts at boot.
func Install(c *Config) error {
	if err := os.MkdirAll(filepath.Dir(unitPath()), 0o755); err != nil {
		return err
	}
	// systemd creates the log file but not its directory.
	if err := os.MkdirAll(filepath.Dir(LogPath()), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(unitPath(), []byte(unitFile(c)), 0o600); err != nil {
		return err
	}
	os.Truncate(LogPath(), 0)
	if err := systemctl("daemon-reload"); err != nil {
		return err
	}
	// Keep running after logout / start at boot. Best effort: may need polkit.
	exec.Command("loginctl", "enable-linger").Run()
	if err := systemctl("enable", unitName()); err != nil {
		return err
	}
	return systemctl("restart", unitName())
}

func Stop() error { return systemctl("stop", unitName()) }

// Start (re)writes the service from the current settings and starts it, so
// a setup made by an older EZLM picks up today's server flags (e.g. --metrics).
func Start() error {
	if c, err := Load(); err == nil {
		return Install(c)
	}
	os.Truncate(LogPath(), 0)
	return systemctl("start", unitName())
}

func Uninstall() error {
	systemctl("disable", "--now", unitName())
	if err := os.Remove(unitPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	systemctl("daemon-reload")
	return nil
}

// EnsureAutostart is a no-op here: the login service is set up by Install.
func EnsureAutostart() {}
