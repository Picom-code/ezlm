package paths

import (
	"strings"
	"testing"
)

// longPath builds an absolute drive path of exactly n bytes.
func testLongPath(n int) string {
	p := `C:\Users\sam\AppData\Local\EZLM\models\` + strings.Repeat("a", 64)
	for len(p) < n {
		p += `\` + strings.Repeat("b", 32)
	}
	return p[:n]
}

func TestExtended(t *testing.T) {
	short := `C:\Users\sam\.ezlm\bin\llama-server.exe`
	if got := Extended(short); got != short {
		t.Fatalf("short path changed: %q", got)
	}

	// Exactly one byte under the threshold stays alone; at it, it grows.
	if p := testLongPath(longPathThreshold - 1); Extended(p) != p {
		t.Fatalf("path of %d bytes was extended", len(p))
	}
	p := testLongPath(longPathThreshold)
	if got, want := Extended(p), `\\?\`+p; got != want {
		t.Fatalf("drive path:\n got %q\nwant %q", got, want)
	}

	unc := `\\server\share\` + strings.Repeat("d", longPathThreshold)
	if got, want := Extended(unc), `\\?\UNC\`+unc[2:]; got != want {
		t.Fatalf("UNC path:\n got %q\nwant %q", got, want)
	}

	for _, p := range []string{
		`\\?\C:\` + strings.Repeat("e", longPathThreshold),
		`\??\C:\` + strings.Repeat("e", longPathThreshold),
		`\\.\pipe\` + strings.Repeat("e", longPathThreshold),
	} {
		if got := Extended(p); got != p {
			t.Fatalf("already extended path changed: %q", got)
		}
	}

	// \\?\ turns off . and .. handling, so the path must be clean first.
	dirty := `C:\Users\sam\` + strings.Repeat("f", longPathThreshold) + `\drop\..\models`
	want := `\\?\C:\Users\sam\` + strings.Repeat("f", longPathThreshold) + `\models`
	if got := Extended(dirty); got != want {
		t.Fatalf("dirty path:\n got %q\nwant %q", got, want)
	}

	// Forward slashes are legal input but not legal after the prefix.
	slashed := "C:/Users/sam/" + strings.Repeat("g", longPathThreshold)
	if got, want := Extended(slashed), `\\?\C:\Users\sam\`+strings.Repeat("g", longPathThreshold); got != want {
		t.Fatalf("slashed path:\n got %q\nwant %q", got, want)
	}

	// Relative and drive-rooted paths have no volume to prefix.
	if p := strings.Repeat("h", longPathThreshold) + `\models`; Extended(p) != p {
		t.Fatalf("relative path was extended: %q", p)
	}
	if p := `\Windows\` + strings.Repeat("i", longPathThreshold); Extended(p) != p {
		t.Fatalf("rooted path was extended: %q", p)
	}
}
