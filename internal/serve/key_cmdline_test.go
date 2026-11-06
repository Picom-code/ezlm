package serve

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// keyCmdlineHome points EZLM_HOME at a temp dir so the key file this test
// inspects is never the real one.
func keyCmdlineHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("EZLM_HOME", home)
	return home
}

// keyCmdlineFlag returns the value after flag in args, or "" when the flag
// is missing. llama-server flags are either "--flag value" or "--flag=value".
func keyCmdlineFlag(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
		if v, ok := strings.CutPrefix(a, flag+"="); ok {
			return v
		}
	}
	return ""
}

func keyCmdlineHas(args []string, flag string) bool {
	for _, a := range args {
		if a == flag || strings.HasPrefix(a, flag+"=") {
			return true
		}
	}
	return false
}

// TestKeyNotOnLlamaServerCommandLine is the threat: every local user can read
// another user's process arguments with ps, so a key passed as --api-key is
// public on a shared machine. The server must read it from a private file
// instead.
func TestKeyNotOnLlamaServerCommandLine(t *testing.T) {
	home := keyCmdlineHome(t)
	const key = "REDACTED-KEY"
	c := &Config{
		Key:       key,
		Keys:      []APIKey{{ID: "k1", Name: "Default", Key: key}},
		ModelID:   "test-model",
		ModelPath: filepath.Join(home, "model.gguf"),
		Port:      8080,
	}

	args := c.Args()
	for _, a := range args {
		if strings.Contains(a, key) {
			t.Fatalf("the API key is on the llama-server command line: %q", a)
		}
	}
	if keyCmdlineHas(args, "--api-key") {
		t.Fatalf("llama-server is still given --api-key: %v", args)
	}
	path := keyCmdlineFlag(args, "--api-key-file")
	if path == "" {
		t.Fatalf("llama-server is not given --api-key-file: %v", args)
	}
	if !strings.HasPrefix(path, home+string(filepath.Separator)) {
		t.Fatalf("key file %q is not under the test EZLM_HOME %q", path, home)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the file llama-server was told to read does not exist: %v", err)
	}
	if got := strings.TrimSuffix(string(b), "\n"); got != key {
		t.Fatalf("key file holds %q, want %q", got, key)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := st.Mode().Perm(); perm != 0o600 {
			t.Fatalf("key file mode is %o, want 600: other local users can read it", perm)
		}
	}
}

// TestKeyFileHoldsEveryKeyOnePerLine: llama-server reads one key per line,
// so several keys (and a rotated set) must all be there and nothing stale.
func TestKeyFileHoldsEveryKeyOnePerLine(t *testing.T) {
	keyCmdlineHome(t)
	c := &Config{
		Keys: []APIKey{
			{ID: "k1", Name: "Default", Key: "REDACTED-KEY-1"},
			{ID: "k2", Name: "Laptop", Key: "REDACTED-KEY-2"},
		},
		ModelPath: "/models/m.gguf",
	}
	args := c.Args()
	path := keyCmdlineFlag(args, "--api-key-file")
	if path == "" {
		t.Fatalf("no --api-key-file in %v", args)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), "REDACTED-KEY-1\nREDACTED-KEY-2\n"; got != want {
		t.Fatalf("key file is %q, want %q", got, want)
	}

	// A key removed from the config must disappear from the file: llama-server
	// would otherwise keep accepting it after the user revoked it.
	c.Keys = []APIKey{{ID: "k3", Name: "New", Key: "REDACTED-KEY-3"}}
	c.Args()
	b, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), "REDACTED-KEY-3\n"; got != want {
		t.Fatalf("key file after rotation is %q, want %q", got, want)
	}
}

// TestKeyFileFallbackKeepsServerStarting: where the file cannot be written
// (a locked or read-only home), the server still starts with the old flag.
// The fallback is visible to ps, so it is a last resort only.
func TestKeyFileFallbackKeepsServerStarting(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EZLM_HOME", filepath.Join(file, "home")) // can never be created

	const key = "REDACTED-KEY"
	c := &Config{Key: key, ModelPath: "/models/m.gguf"}
	args := c.Args()
	if got := keyCmdlineFlag(args, "--api-key"); got != key {
		t.Fatalf("fallback --api-key = %q, want %q (args %v)", got, key, args)
	}
	if keyCmdlineHas(args, "--api-key-file") {
		t.Fatalf("args name an unwritable key file: %v", args)
	}
}
