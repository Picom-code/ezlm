package serve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSystemctl puts a systemctl and a loginctl that only record their
// arguments first on PATH, so Install and Uninstall can be tested without
// touching this machine's services.
func fakeSystemctl(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	t.Setenv("FAKE_SYSTEMCTL_CALLS", calls)
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$FAKE_SYSTEMCTL_CALLS\"\n"
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "loginctl"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return calls
}

func readCalls(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func linuxTestConfig(tmp string) *Config {
	return &Config{
		Server:    filepath.Join(tmp, "home dir", ".ezlm", "bin", "llama-b11960", "llama-server"),
		ModelPath: filepath.Join(tmp, "models", "model.gguf"),
		ModelID:   "test-model",
		Port:      8080,
		Ctx:       4096,
		NGL:       99,
		Keys:      []APIKey{{ID: "k1", Name: "Default", Key: "REDACTED-KEY"}},
	}
}

func TestInstallWritesUnitAndEnablesIt(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("EZLM_HOME", filepath.Join(tmp, "home dir", ".ezlm"))
	calls := fakeSystemctl(t)

	c := linuxTestConfig(tmp)
	os.MkdirAll(filepath.Dir(LogPath()), 0o755)
	os.WriteFile(LogPath(), []byte("old run\n"), 0o644)

	if err := Install(c); err != nil {
		t.Fatal(err)
	}

	unit, err := os.ReadFile(unitPath())
	if err != nil {
		t.Fatalf("unit was not written: %v", err)
	}
	if string(unit) != unitFile(c) {
		t.Errorf("unit file differs from the builder:\n%s", unit)
	}
	if st, err := os.Stat(LogPath()); err != nil || st.Size() != 0 {
		t.Errorf("log was not truncated: %v", err)
	}

	got := readCalls(t, calls)
	want := []string{
		"--user daemon-reload",
		"--user enable " + unitName(),
		"--user restart " + unitName(),
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("systemctl calls = %q, want %q", got, want)
	}
}

func TestUninstallRemovesTheUnit(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("EZLM_HOME", filepath.Join(tmp, "ezlm"))
	calls := fakeSystemctl(t)

	if err := Install(linuxTestConfig(tmp)); err != nil {
		t.Fatal(err)
	}
	os.Remove(calls)

	if err := Uninstall(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unitPath()); !os.IsNotExist(err) {
		t.Errorf("unit still exists after Uninstall: %v", err)
	}
	got := readCalls(t, calls)
	want := []string{"--user disable --now " + unitName(), "--user daemon-reload"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("systemctl calls = %q, want %q", got, want)
	}
	if err := Uninstall(); err != nil {
		t.Errorf("a second Uninstall failed: %v", err)
	}
}

func TestStartWithoutConfigStartsTheUnit(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("EZLM_HOME", filepath.Join(tmp, "ezlm"))
	calls := fakeSystemctl(t)

	if err := Start(); err != nil {
		t.Fatal(err)
	}
	got := readCalls(t, calls)
	if len(got) != 1 || got[0] != "--user start "+unitName() {
		t.Errorf("systemctl calls = %q, want --user start %s", got, unitName())
	}
}
