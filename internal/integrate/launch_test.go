package integrate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestReadExit(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.exit")
	if _, _, ok := ReadExit(p); ok {
		t.Fatal("no file: still running")
	}
	os.WriteFile(p, []byte("3 1762400000\n"), 0o644)
	code, at, ok := ReadExit(p)
	if !ok || code != 3 || at.Unix() != 1762400000 {
		t.Fatalf("got %d %v %v", code, at, ok)
	}
}

func TestLaunchDryRunOpensNothing(t *testing.T) {
	os.MkdirAll(LaunchDir(), 0o755)
	os.WriteFile(ExitFile("dry"), []byte("1 0\n"), 0o644)
	l, err := Launch(LaunchSpec{ID: "dry", Bin: "/nonexistent", Dir: t.TempDir()})
	if err != nil || !l.DryRun {
		t.Fatalf("%+v %v", l, err)
	}
	if _, _, ok := ReadExit(ExitFile("dry")); ok {
		t.Fatal("a stale exit file must be removed before launching")
	}
}

func TestReadExitCodeOnly(t *testing.T) {
	// The Windows launcher writes the code alone; the time is the file's.
	p := filepath.Join(t.TempDir(), "x.exit")
	os.WriteFile(p, []byte("-1073741819 \r\n"), 0o644)
	code, at, ok := ReadExit(p)
	if !ok || code != -1073741819 || time.Since(at) > time.Minute {
		t.Fatalf("got %d %v %v", code, at, ok)
	}
	os.WriteFile(p, []byte("garbage"), 0o644)
	if _, _, ok := ReadExit(p); ok {
		t.Fatal("garbage is not an exit")
	}
}

func TestLaunchDryRunWritesLauncher(t *testing.T) {
	l, err := Launch(LaunchSpec{ID: "opencode", Title: "OpenCode (EZLM)", Bin: "/x/opencode", Args: []string{"--standalone"}, Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ext := map[string]string{"windows": ".cmd", "darwin": ".command"}[runtime.GOOS]
	if ext == "" {
		ext = ".sh"
	}
	if !l.DryRun || l.Launcher != filepath.Join(LaunchDir(), "opencode"+ext) || l.ExitFile != ExitFile("opencode") {
		t.Fatalf("%+v", l)
	}
	st, err := os.Stat(l.Launcher)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm()&0o100 == 0 {
		t.Errorf("launcher mode %v: Terminal only runs an executable .command", st.Mode())
	}
	left, _ := filepath.Glob(filepath.Join(LaunchDir(), "*.ezlm-tmp-*"))
	if len(left) > 0 {
		t.Errorf("temp files left: %v", left)
	}
}

func TestLaunchRefusesKeys(t *testing.T) {
	for _, spec := range []LaunchSpec{
		{ID: "k1", Bin: "/x/app", Env: []string{"EZLM_API_KEY=abc"}},
		{ID: "k2", Bin: "/x/app", Env: []string{"OPENAI_TOKEN=abc"}},
		{ID: "k3", Bin: "/x/app", Env: []string{"HARMLESS=ezlm-AbCdEf1234567890"}},
		{ID: "k4", Bin: "/x/app", Args: []string{"--api-key", "sk-proj-Ab3dEf6hIj9kLmN0pQrS"}},
		{ID: "k5", Bin: "/x/app", Args: []string{"-H", "Authorization: Bearer abcdefghijkl"}},
		{ID: "k6", Bin: "/x/app", Env: []string{"no-equals-sign"}},
	} {
		if _, err := Launch(spec); err == nil {
			t.Errorf("%+v: refused nothing", spec)
		}
		if m, _ := filepath.Glob(filepath.Join(LaunchDir(), spec.ID+".*")); len(m) > 0 {
			t.Errorf("%s: a launcher was written: %v", spec.ID, m)
		}
	}
	// Folders and values that only contain a key-like prefix are fine.
	for _, v := range []string{"/var/folders/x/ezlm-integrate-test-1234567/home/EZLM Workspace", "/tmp/ezlm-e2e.yuiGpR/home",
		`C:\Users\Sam\sk-learn-projects\work`, "ezlm-check.txt", "--title=EZLM (local)"} {
		if launchKeyLike(v) {
			t.Errorf("%q is not a key", v)
		}
	}
	for _, v := range []string{"ezlm-" + strings.Repeat("Ab3", 13) + "x", "KEY=ezlm-AbCdEf1234567890", "sk-ant-api03-Ab3dEf6hIj9kLmN0pQrS", "Authorization: Bearer abcdefghijkl"} {
		if !launchKeyLike(v) {
			t.Errorf("%q is a key", v)
		}
	}
	for _, name := range []string{"OPENAI_API_KEY", "GH_TOKEN", "HERMES_AUTH_FILE", "DB_PASSWORD", "AWS_SECRET_ACCESS_KEY"} {
		if !launchSecret.MatchString(name) {
			t.Errorf("%s must count as a secret", name)
		}
	}
	for _, name := range []string{"PATH", "OPENCODE_DISABLE_AUTOUPDATE", "DSH_TELEMETRY_MODE", "KEYBOARD_LAYOUT", "MONKEY", "PASSIVE"} {
		if launchSecret.MatchString(name) {
			t.Errorf("%s is not a secret", name)
		}
	}
	// Model ids and EZLM's own provider names are not keys.
	if _, err := Launch(LaunchSpec{ID: "ok", Bin: "/x/app", Args: []string{"--provider", "custom:ezlm", "-m", "ezlm/qwen3.8-27b"}, Env: []string{"DSH_TELEMETRY_MODE=DISABLED", "PATH=/x:${PATH}"}, Dir: t.TempDir()}); err != nil {
		t.Errorf("a plain spec was refused: %v", err)
	}
}

func TestLaunchURLOnlyAndBadSpecs(t *testing.T) {
	l, err := Launch(LaunchSpec{ID: "dsh", Mode: ModeWeb, URL: "http://127.0.0.1:3080"})
	if err != nil || !l.DryRun || l.Launcher != "" || l.ExitFile != "" {
		t.Fatalf("URL only: %+v %v", l, err)
	}
	if _, err := Launch(LaunchSpec{ID: "dsh"}); err == nil {
		t.Error("no program and no URL must fail")
	}
	if _, err := Launch(LaunchSpec{ID: `..\evil`, Bin: "/x"}); err == nil {
		t.Error("an id that isn't a plain name must fail")
	}
	t.Setenv("EZLM_LAUNCH_DRYRUN", "")
	if err := OpenURL("file:///etc/passwd"); err == nil {
		t.Error("OpenURL must only open web pages")
	}
}

func TestLaunchExpandEnv(t *testing.T) {
	got := launchExpandEnv([]string{"PATH=/n/bin:${PATH}", "A=${B}${B}x", "C=plain", "D="}, func(k string) string { return map[string]string{"PATH": "/usr/bin", "B": "b"}[k] })
	want := []string{"PATH=/n/bin:/usr/bin", "A=bbx", "C=plain", "D="}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%q", got)
	}
}
