package serve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The server's log lands in a folder whose name has spaces and letters
// outside the ANSI code page, as under C:\Users\José Müller.
func TestStartLoggedUnicodeFolder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "José Müller 王", "logs")
	log := filepath.Join(dir, "server.log")
	// cmd.exe stands in for llama-server: it writes a line to stdout and one to stderr.
	if err := startLogged(os.Getenv("ComSpec"), []string{"/c", "echo out-line & echo err-line 1>&2"}, log); err != nil {
		t.Fatal(err)
	}
	var got string
	for i := 0; i < 50; i++ {
		b, _ := os.ReadFile(log)
		if got = string(b); strings.Contains(got, "out-line") && strings.Contains(got, "err-line") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("log in %q has %q", log, got)
}
