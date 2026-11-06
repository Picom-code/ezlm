package app

import (
	"os"
	"path/filepath"
	"testing"
)

// Every test in this package runs against a throwaway EZLM home: tests delete
// and rewrite model files and settings, and must never reach the real
// ~/.ezlm (a prune test once removed a real model this way). The connectable
// apps' folders (HOME, the XDG folders, HERMES_HOME, DSH_HOME, the Windows
// profile folders) point there too, programs are only found in folders a test
// names (EZLM_TOOL_PATH), launchers open no window, and integrations stay off
// unless a test turns them on.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "ezlm-test-home-")
	if err != nil {
		panic(err)
	}
	user := filepath.Join(home, "user")
	os.MkdirAll(user, 0o755)
	for k, v := range map[string]string{
		"EZLM_HOME":            home,
		"EZLM_NO_INTEGRATIONS": "1",
		"EZLM_NO_AUTOSTART":    "1",
		"HOME":                 user,
		"USERPROFILE":          user,
		"XDG_CONFIG_HOME":      filepath.Join(user, ".config"),
		"XDG_DATA_HOME":        filepath.Join(user, ".local", "share"),
		"XDG_STATE_HOME":       filepath.Join(user, ".local", "state"),
		"XDG_CACHE_HOME":       filepath.Join(user, ".cache"),
		"HERMES_HOME":          filepath.Join(user, ".hermes"),
		"DSH_HOME":             filepath.Join(user, ".dsh"),
		"OPENCODE_CONFIG_DIR":  filepath.Join(user, ".config", "opencode"),
		"LOCALAPPDATA":         filepath.Join(user, "AppData", "Local"),
		"APPDATA":              filepath.Join(user, "AppData", "Roaming"),
		"EZLM_TOOL_PATH":       "",
		"EZLM_LAUNCH_DRYRUN":   "1",
		"DSH_TELEMETRY_MODE":   "DISABLED",
	} {
		os.Setenv(k, v)
	}
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
