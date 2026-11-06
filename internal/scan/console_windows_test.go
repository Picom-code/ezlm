package scan

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// Windows PowerShell writes redirected output in the console's OEM code page
// (cp437, cp850, cp932, ...), not UTF-8, so a name with é or 王 comes back
// as invalid UTF-8 unless runPS asks for UTF-8 first. The old plain run()
// returned "Caf\x82 M\x81ller" for this text on an English Windows.
func TestRunPSDecodesUTF8(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	const want = "Café Müller 王"
	out, err := runPS(ctx, `Write-Output 'Café Müller 王'`)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out); got != want {
		t.Fatalf("runPS = %q (valid UTF-8: %v), want %q", got, utf8.ValidString(got), want)
	}
}

// The encoding statement must not leak into the output or add a byte order
// mark: the result is parsed as text, not read by a human.
func TestRunPSOutputIsPlain(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := runPS(ctx, `Write-Output 'PCI\VEN_10DE&DEV_2684'`)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out); got != `PCI\VEN_10DE&DEV_2684` {
		t.Fatalf("runPS = %q, want the line alone", got)
	}
	if strings.ContainsRune(out, '\uFEFF') {
		t.Fatalf("runPS output starts with a byte order mark: %q", out)
	}
}
