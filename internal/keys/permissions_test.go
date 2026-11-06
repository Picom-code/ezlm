package keys

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/picom-code/ezlm/internal/paths"
	"github.com/picom-code/ezlm/internal/serve"
)

// tempHome points EZLM_HOME at a fresh folder for the test.
func tempHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), ".ezlm")
	t.Setenv("EZLM_HOME", home)
	return home
}

func perm(t *testing.T, path string) os.FileMode {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return st.Mode().Perm()
}

// ~/.ezlm holds the API key in config.json. Another local user must not be
// able to list or traverse it, so the folder is owner-only.
func TestEZLMHomeOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no mode bits; see the report's ACL note")
	}
	home := tempHome(t)
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	if got := perm(t, home); got != 0o700 {
		t.Errorf("new %s is %o, want 700", home, got)
	}

	// An install made by an older EZLM has a 0755 folder. Ensure repairs it.
	if err := os.Chmod(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	if got := perm(t, home); got != 0o700 {
		t.Errorf("existing %s stayed %o, want 700", home, got)
	}
}

// The key lives in config.json: owner-only, including when an older copy of
// the file was world-readable.
func TestConfigKeyFileOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no mode bits; see the report's ACL note")
	}
	home := tempHome(t)
	c := &serve.Config{
		Key:       "REDACTED-KEY",
		Keys:      []serve.APIKey{{ID: "k1", Name: "Default", Key: "REDACTED-KEY", Created: time.Now()}},
		ModelPath: "/models/m.gguf",
		Port:      8080,
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	if got := perm(t, paths.Config()); got != 0o600 {
		t.Fatalf("config.json is %o, want 600", got)
	}

	// A leftover temp file from a crash must not decide the final mode.
	if err := os.Chmod(paths.Config(), 0o644); err != nil {
		t.Fatal(err)
	}
	tmp := paths.Config() + ".tmp"
	if err := os.WriteFile(tmp, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	if got := perm(t, paths.Config()); got != 0o600 {
		t.Errorf("config.json after a 0644 temp file is %o, want 600", got)
	}
	_ = home
}

// settings.json is written in place, so an older world-readable file keeps
// its mode unless SaveSettings sets it again.
func TestSettingsFileOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no mode bits; see the report's ACL note")
	}
	home := tempHome(t)
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.SettingsPath(), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := paths.SaveSettings(paths.Settings{}); err != nil {
		t.Fatal(err)
	}
	if got := perm(t, paths.SettingsPath()); got != 0o600 {
		t.Errorf("settings.json is %o, want 600", got)
	}
}

// The server log is created by EZLM. It carries model output and startup
// lines, so other local users must not read it either.
func TestServerLogOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no mode bits; see the report's ACL note")
	}
	home := tempHome(t)
	if err := os.MkdirAll(paths.Logs(), 0o755); err != nil {
		t.Fatal(err)
	}
	// A stand-in for llama-server: it ignores the flags and exits.
	server := filepath.Join(t.TempDir(), "server.sh")
	if err := os.WriteFile(server, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &serve.Config{Key: "REDACTED-KEY", Server: server, ModelPath: "/models/m.gguf", Port: 8080}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	if err := serve.RunForeground(); err != nil {
		t.Fatal(err)
	}
	if got := perm(t, serve.LogPath()); got != 0o600 {
		t.Errorf("%s is %o, want 600", serve.LogPath(), got)
	}
	_ = home
}
