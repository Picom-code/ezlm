package serve

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// waitForLog reads path until it contains want, or fails the test.
func waitForLog(t *testing.T, path, want string) {
	t.Helper()
	var got string
	for i := 0; i < 100; i++ {
		b, _ := os.ReadFile(path)
		if got = string(b); strings.Contains(got, want) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("log %q has %q, want %q", path, got, want)
}

// The server is started with an argv and a log file handle, never with a
// command line of EZLM's own: cmd.exe's special characters in the log
// folder must stay literal.
func TestStartLoggedLogPathWithAmpAndPercent(t *testing.T) {
	log := filepath.Join(t.TempDir(), "A & B 100% (x) ^ José", "server.log")
	if err := startLogged(os.Getenv("ComSpec"), []string{"/c", "echo out-line"}, log); err != nil {
		t.Fatal(err)
	}
	waitForLog(t, log, "out-line")
}

// The same characters in the server's own path must reach CreateProcess
// unchanged, so a copy of cmd.exe in "100% & more" still runs.
func TestStartLoggedServerPathWithAmpAndPercent(t *testing.T) {
	cmdExe := os.Getenv("ComSpec")
	dir := filepath.Join(t.TempDir(), "100% & more")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "cmd.exe")
	copyFile(t, cmdExe, dst)
	log := filepath.Join(t.TempDir(), "server.log")
	if err := startLogged(dst, []string{"/c", "echo out-line"}, log); err != nil {
		t.Fatal(err)
	}
	waitForLog(t, log, "out-line")
}

// copyFile copies src to dst, keeping the read-only bit off so TempDir can
// clean up.
func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}
