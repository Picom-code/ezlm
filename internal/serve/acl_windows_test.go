package serve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A second user on the same Windows machine can read another user's process
// command line (Task Manager, Win32_Process), so the API key must not travel
// in llama-server's arguments. It goes in LLAMA_API_KEY, which llama-server
// reads from the environment (llama.cpp b11960).
func TestServerCommandLineHidesAPIKey(t *testing.T) {
	const key = "ezlm-TestKeyReadableByOtherUsers1234567890"
	c := &Config{Key: key, Keys: []APIKey{{ID: "k1", Name: "Default", Key: key}}}
	args, env := keylessArgs(c.Args())
	for _, a := range args {
		if strings.Contains(a, key) {
			t.Fatalf("the API key is still in the command line: %q", args)
		}
		if a == "--api-key" {
			t.Fatalf("--api-key is still in the command line: %q", args)
		}
	}
	want := "LLAMA_API_KEY=" + key
	for _, e := range env {
		if e == want {
			return
		}
	}
	t.Fatalf("the environment does not carry %s", want)
}

// The key stays out of the command line even when it is the last argument,
// and when there is no value after the flag.
func TestKeylessArgsEdges(t *testing.T) {
	args, env := keylessArgs([]string{"-m", "m.gguf", "--api-key", "ezlm-k"})
	if len(args) != 2 || args[0] != "-m" || args[1] != "m.gguf" {
		t.Fatalf("args = %q", args)
	}
	if !hasEnv(env, "LLAMA_API_KEY=ezlm-k") {
		t.Fatalf("env = %q", env)
	}
	args, _ = keylessArgs([]string{"-m", "m.gguf", "--api-key"})
	if len(args) != 3 {
		t.Fatalf("a flag without a value was dropped: %q", args)
	}
}

// os.WriteFile's mode is not an ACL on Windows: Go only maps the read-only
// bit, so a "0600" config.json reports 0666 and keeps the ACL it inherited
// from its folder. EZLM therefore cannot protect the key in the file with a
// mode; the file is as private as %USERPROFILE% is.
func TestConfigModeIsNotAnACLOnWindows(t *testing.T) {
	t.Setenv("EZLM_HOME", t.TempDir())
	c := &Config{Key: "ezlm-modecheck", Port: 1}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(os.Getenv("EZLM_HOME"), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := st.Mode().Perm(); got != 0o666 {
		t.Fatalf("mode = %v, want 0666 (a mode cannot carry a Windows ACL)", got)
	}
}

func hasEnv(env []string, want string) bool {
	for _, e := range env {
		if e == want {
			return true
		}
	}
	return false
}
