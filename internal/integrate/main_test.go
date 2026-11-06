package integrate

import (
	"os"
	"path/filepath"
	"testing"
)

// Every test in this package runs in a throwaway home: HOME, the XDG
// folders, HERMES_HOME, DSH_HOME, OPENCODE_CONFIG_DIR, EZLM_HOME and the
// Windows profile folders all point into one temp folder, so no test can
// read or write the real OpenCode, Hermes, DeepSeek Harness or EZLM
// settings. EZLM_TOOL_PATH is set but empty, so FindTool finds nothing
// unless a test points it at its own fake programs (t.Setenv).
func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "ezlm-integrate-test-")
	if err != nil {
		panic(err)
	}
	home := filepath.Join(root, "home")
	os.MkdirAll(home, 0o755)
	for k, v := range map[string]string{
		"HOME":                          home,
		"USERPROFILE":                   home,
		"XDG_CONFIG_HOME":               filepath.Join(home, ".config"),
		"XDG_DATA_HOME":                 filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME":                filepath.Join(home, ".local", "state"),
		"XDG_CACHE_HOME":                filepath.Join(home, ".cache"),
		"HERMES_HOME":                   filepath.Join(home, ".hermes"),
		"DSH_HOME":                      filepath.Join(home, ".dsh"),
		"OPENCODE_CONFIG_DIR":           filepath.Join(home, ".config", "opencode"),
		"LOCALAPPDATA":                  filepath.Join(home, "AppData", "Local"),
		"APPDATA":                       filepath.Join(home, "AppData", "Roaming"),
		"EZLM_HOME":                     filepath.Join(root, "ezlm"),
		"EZLM_TOOL_PATH":                "",
		"EZLM_LAUNCH_DRYRUN":            "1",
		"DSH_TELEMETRY_MODE":            "DISABLED",
		"OPENCODE_DISABLE_MODELS_FETCH": "1",
	} {
		os.Setenv(k, v)
	}
	os.Unsetenv("EZLM_NO_INTEGRATIONS")
	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}
