package serve

import (
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tempHome gives the test its own home and EZLM_HOME, so the plist lands in a
// temp LaunchAgents folder and never in the real one.
func tempHome(t *testing.T) (home, ezlm string) {
	t.Helper()
	home = t.TempDir()
	ezlm = filepath.Join(home, ".ezlm")
	t.Setenv("HOME", home)
	t.Setenv("EZLM_HOME", ezlm)
	return home, ezlm
}

func testConfig_servicedarwin(t *testing.T) *Config {
	t.Helper()
	server := filepath.Join(os.Getenv("EZLM_HOME"), "bin", "llama-server")
	return &Config{
		Key: "REDACTED-KEY", Port: 8080, ModelID: "qwen3-8b", Quant: "Q4_K_M",
		ModelPath: "/Users/a b/models/qwen3 8b.gguf", Ctx: 32768, NGL: 99,
		Server: server,
	}
}

// jobCommand is the shell command launchd runs, as the shell sees it.
func jobCommand(t *testing.T, p string) string {
	t.Helper()
	const marker = "<string>-c</string>\n\t\t<string>"
	i := strings.Index(p, marker)
	if i < 0 {
		t.Fatalf("no shell command in the job:\n%s", p)
	}
	rest := p[i+len(marker):]
	j := strings.Index(rest, "</string>")
	if j < 0 {
		t.Fatalf("unterminated command:\n%s", p)
	}
	return html.UnescapeString(rest[:j])
}

func TestPlistQuotesEveryArgument(t *testing.T) {
	tempHome(t)
	c := testConfig_servicedarwin(t)
	p := plistXML(c)
	cmd := jobCommand(t, p)

	if !strings.Contains(cmd, "'/Users/a b/models/qwen3 8b.gguf'") {
		t.Fatalf("a model path with a space is not one argument:\n%s", cmd)
	}
	if !strings.HasPrefix(cmd, "test -f '"+StoppedFlag()+"' || exec '"+c.Server+"' ") {
		t.Fatalf("the job does not start the server itself:\n%s", cmd)
	}
	if strings.Contains(p, "<string>'") {
		t.Fatalf("arguments are not shell-quoted in the plist:\n%s", p)
	}
	// An apostrophe in a path must survive both the shell and XML.
	c.ModelPath = "/Users/o'brien/model.gguf"
	cmd = jobCommand(t, plistXML(c))
	if !strings.Contains(cmd, `'/Users/o'\''brien/model.gguf'`) {
		t.Fatalf("apostrophe not shell-quoted:\n%s", cmd)
	}
}

func TestPlistHonoursStop(t *testing.T) {
	tempHome(t)
	p := plistXML(testConfig_servicedarwin(t))
	if !strings.Contains(jobCommand(t, p), "test -f '"+StoppedFlag()+"' || exec ") {
		t.Fatalf("the login job ignores the stopped flag, so a stopped server starts again at login:\n%s", p)
	}
	if !strings.Contains(p, "<key>KeepAlive</key>") || !strings.Contains(p, "<key>RunAtLoad</key>") {
		t.Fatalf("crash restart lost:\n%s", p)
	}
}

func TestEnsureAutostartRepairsMissingAndStalePlist(t *testing.T) {
	home, _ := tempHome(t)
	c := testConfig_servicedarwin(t)
	if err := os.MkdirAll(filepath.Dir(c.Server), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.Server, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "Library", "LaunchAgents", label()+".plist")

	// A plist an older EZLM wrote: the server path moved, and it has no stop
	// guard. Nothing else repairs it until the next deploy.
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte("<plist>old</plist>"), 0o600); err != nil {
		t.Fatal(err)
	}
	EnsureAutostart()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != plistXML(c) {
		t.Fatalf("stale plist not repaired:\n%s", b)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("plist mode %v, want 0600", fi.Mode().Perm())
	}

	// A missing plist is written again, and a current one is left alone.
	os.Remove(path)
	EnsureAutostart()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("missing plist not written: %v", err)
	}
	before, _ := os.Stat(path)
	EnsureAutostart()
	after, _ := os.Stat(path)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("an up-to-date plist was rewritten")
	}

	// No server binary: no job that fails at every login.
	os.Remove(c.Server)
	os.Remove(path)
	EnsureAutostart()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("wrote a job for a missing server: %v", err)
	}
}
