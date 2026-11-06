//go:build !windows

package integrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The login shell's PATH and variables come from `env` after a marker, so
// whatever the user's shell files print first is ignored.
func TestProbeLoginEnv(t *testing.T) {
	dir := t.TempDir()
	sh := filepath.Join(dir, "fakesh")
	os.WriteFile(sh, []byte(`#!/bin/sh
echo "Welcome back! PATH=/not/this"
echo "args: $*" >&2
printf '%s\n' __EZLM_ENV_7f3c__
printf 'PATH=/opt/tools/bin:/usr/bin\nHERMES_HOME=/Users/sam/hermes data\nMULTI=line one\nline two\n1BAD=x\n'
`), 0o755)
	old := loginShell
	loginShell = func() string { return sh }
	defer func() { loginShell = old }()
	env := probeLoginEnv()
	if env["PATH"] != "/opt/tools/bin:/usr/bin" || env["HERMES_HOME"] != "/Users/sam/hermes data" || env["MULTI"] != "line one" {
		t.Fatalf("env = %v", env)
	}
	if _, ok := env["1BAD"]; ok {
		t.Fatal("not a variable name")
	}
	if parseLoginEnv([]byte("no marker here")) != nil {
		t.Fatal("output without the marker must give nil")
	}
}

// A shell that hangs (an rc file waiting for input) is stopped at the
// timeout, and a missing shell gives nothing.
func TestProbeLoginEnvTimeout(t *testing.T) {
	dir := t.TempDir()
	sh := filepath.Join(dir, "slowsh")
	os.WriteFile(sh, []byte("#!/bin/sh\nsleep 30\n"), 0o755)
	oldS, oldT := loginShell, loginTimeout
	loginShell, loginTimeout = func() string { return sh }, 300*time.Millisecond
	defer func() { loginShell, loginTimeout = oldS, oldT }()
	start := time.Now()
	if env := probeLoginEnv(); env != nil || time.Since(start) > 5*time.Second {
		t.Fatalf("env %v after %v", env, time.Since(start))
	}
	loginShell = func() string { return "" }
	if probeLoginEnv() != nil {
		t.Fatal("no shell: nil")
	}
}

// Under EZLM_TOOL_PATH (every test) the real shell is never asked.
func TestLoginEnvSkippedInTests(t *testing.T) {
	called := false
	old := loginShell
	loginShell = func() string { called = true; return "/bin/sh" }
	defer func() { loginShell = old }()
	if LoginEnv("PATH") != "" || called {
		t.Fatal("LoginEnv ran the shell under EZLM_TOOL_PATH")
	}
	RefreshToolSearch()
	if called {
		t.Fatal("RefreshToolSearch ran the shell under EZLM_TOOL_PATH")
	}
}

func TestFindAllToolsOrderAndDedupe(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	for _, d := range []string{a, b} {
		os.WriteFile(filepath.Join(d, "faketool"), []byte("#!/bin/sh\n"), 0o755)
	}
	os.Mkdir(filepath.Join(t.TempDir(), "faketool"), 0o755)
	t.Setenv("EZLM_TOOL_PATH", strings.Join([]string{b, a, b, ""}, string(os.PathListSeparator)))
	got := FindAllTools("faketool")
	if len(got) != 2 || got[0] != filepath.Join(b, "faketool") {
		t.Fatalf("got %v", got)
	}
	if d := SearchDirs(); len(d) != 4 {
		t.Fatalf("SearchDirs = %v", d)
	}
	if IsAppAlias(filepath.Join(a, "faketool")) {
		t.Fatal("only Windows has app aliases")
	}
}

func TestToolDirsCoverUsualFolders(t *testing.T) {
	t.Setenv("HERMES_HOME", "/srv/hermes")
	dirs := strings.Join(toolDirs(), "\n")
	home, _ := os.UserHomeDir()
	for _, want := range []string{filepath.Join(home, ".opencode", "bin"), filepath.Join(home, ".volta", "bin"), filepath.Join(home, ".hermes", "hermes-agent", ".hermes", "bin"), "/srv/hermes/bin", "/usr/local/bin"} {
		if !strings.Contains(dirs, want) {
			t.Errorf("missing %s in\n%s", want, dirs)
		}
	}
}
