//go:build !windows

package integrate

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/picom-code/ezlm/internal/proc"
)

// The login shell's environment. EZLM started from the Dock or by launchd
// gets a minimal PATH (/usr/bin:/bin:…) and none of what the user's shell
// files set: ~/.opencode/bin, nvm, HERMES_HOME. So EZLM asks the user's
// shell once, the way editors do ($SHELL -i -l -c env), caches the answer
// for 10 minutes and refreshes it in the background. A Status poll never
// waits more than loginWait for it.

var (
	loginMu      sync.Mutex
	loginEnv     map[string]string // nil until the first probe ends
	loginAt      time.Time
	loginRunning chan struct{} // closed when the running probe ends
	loginTTL     = 10 * time.Minute
	loginWait    = 1500 * time.Millisecond
	loginTimeout = 5 * time.Second
	loginShell   = defaultShell // tests swap it
)

// defaultShell is $SHELL, else the system's default login shell (apps that
// launchd starts may have no SHELL).
func defaultShell() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	for _, sh := range []string{"/bin/zsh", "/bin/bash"} {
		if _, err := os.Stat(sh); err == nil {
			return sh
		}
	}
	return ""
}

const loginMarker = "__EZLM_ENV_7f3c__"

// userPath is the login shell's PATH as a folder list.
func userPath() []string {
	return filepath.SplitList(LoginEnv("PATH"))
}

// LoginEnv is a variable as the user's login shell sets it ("" if unset or
// unknown). Tools use it for settings that live only in shell files, such
// as a HERMES_HOME exported in ~/.zshrc.
func LoginEnv(key string) string {
	if _, ok := os.LookupEnv("EZLM_TOOL_PATH"); ok {
		return "" // tests: never run the real shell
	}
	return loginSnapshot(false)[key]
}

// RefreshToolSearch drops the cached shell environment and probes again,
// waiting for it: installers add folders to PATH in shell files.
func RefreshToolSearch() {
	if _, ok := os.LookupEnv("EZLM_TOOL_PATH"); ok {
		return
	}
	loginSnapshot(true)
}

func loginSnapshot(force bool) map[string]string {
	loginMu.Lock()
	env, fresh := loginEnv, time.Since(loginAt) < loginTTL
	if env != nil && fresh && !force {
		loginMu.Unlock()
		return env
	}
	done := loginRunning
	if done == nil {
		done = make(chan struct{})
		loginRunning = done
		go func() {
			e := probeLoginEnv()
			loginMu.Lock()
			if e != nil || loginEnv == nil {
				if e == nil {
					e = map[string]string{} // no shell answer: don't ask again until the TTL ends
				}
				loginEnv = e
			}
			loginAt, loginRunning = time.Now(), nil
			loginMu.Unlock()
			close(done)
		}()
	}
	loginMu.Unlock()
	wait := loginWait
	if force {
		wait = loginTimeout + time.Second
	}
	if env != nil && !force {
		return env // stale but usable; the refresh runs in the background
	}
	select {
	case <-done:
	case <-time.After(wait):
	}
	loginMu.Lock()
	defer loginMu.Unlock()
	return loginEnv
}

// probeLoginEnv runs the user's shell as an interactive login shell (so
// .zshrc and .bash_profile both count) with no terminal and no input, and
// parses `env` after a marker, ignoring whatever the shell files print.
func probeLoginEnv() map[string]string {
	sh := loginShell()
	if sh == "" || !filepath.IsAbs(sh) {
		return nil
	}
	for _, args := range [][]string{{"-i", "-l", "-c"}, {"-l", "-c"}} {
		ctx, cancel := context.WithTimeout(context.Background(), loginTimeout)
		cmd := exec.Command(sh, append(args, "printf '%s\\n' "+loginMarker+"; /usr/bin/env")...)
		cmd.Env = append(os.Environ(), "EZLM_SHELL_PROBE=1")
		out, err := proc.Output(ctx, cmd)
		cancel()
		if env := parseLoginEnv(out); env != nil && (err == nil || len(env) > 0) {
			return env
		}
	}
	return nil
}

// parseLoginEnv reads KEY=VALUE lines after the marker. A value that spans
// lines keeps only its first line; PATH-like values never do.
func parseLoginEnv(out []byte) map[string]string {
	i := bytes.LastIndex(out, []byte(loginMarker+"\n"))
	if i < 0 {
		return nil
	}
	env := map[string]string{}
	for _, l := range strings.Split(string(out[i+len(loginMarker)+1:]), "\n") {
		k, v, ok := strings.Cut(l, "=")
		if ok && validEnvKey(k) {
			env[k] = v
		}
	}
	return env
}

func validEnvKey(k string) bool {
	if k == "" {
		return false
	}
	for i, c := range k {
		if !(c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}
