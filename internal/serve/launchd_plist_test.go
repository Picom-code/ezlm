package serve

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// launchd gets its arguments as a plist array, not a shell string: a path with
// spaces must arrive as one element, and any XML metacharacter in it must not
// break the file (launchd rejects the whole agent on one bad character, and the
// server then never starts at login).

// parsePlist decodes a property list into maps, arrays and scalars. Only the
// value types EZLM writes are handled.
func parsePlist(t *testing.T, doc string) map[string]any {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(doc))
	for {
		tok, err := d.Token()
		if err == io.EOF {
			t.Fatal("plist has no root element")
		}
		if err != nil {
			t.Fatalf("plist is not well-formed XML: %v", err)
		}
		if start, ok := tok.(xml.StartElement); ok {
			if start.Name.Local != "plist" {
				t.Fatalf("root element is %q, want plist", start.Name.Local)
			}
			break
		}
	}
	var start xml.StartElement
	for {
		tok, err := d.Token()
		if err != nil {
			t.Fatalf("plist has no body: %v", err)
		}
		if s, ok := tok.(xml.StartElement); ok {
			start = s
			break
		}
	}
	if start.Name.Local != "dict" {
		t.Fatalf("plist body is not a dict: %q", start.Name.Local)
	}
	root, ok := parsePlistValue(t, d, start).(map[string]any)
	if !ok {
		t.Fatal("plist body is not a dict")
	}
	return root
}

func parsePlistValue(t *testing.T, d *xml.Decoder, start xml.StartElement) any {
	t.Helper()
	switch start.Name.Local {
	case "string":
		return plistText(t, d, start)
	case "integer":
		n, err := strconv.Atoi(plistText(t, d, start))
		if err != nil {
			t.Fatalf("bad integer in plist: %v", err)
		}
		return n
	case "true", "false":
		d.Skip()
		return start.Name.Local == "true"
	case "array":
		out := []any{}
		for {
			tok, err := d.Token()
			if err != nil {
				t.Fatalf("unterminated array: %v", err)
			}
			switch x := tok.(type) {
			case xml.StartElement:
				out = append(out, parsePlistValue(t, d, x))
			case xml.EndElement:
				return out
			}
		}
	case "dict":
		out := map[string]any{}
		for {
			tok, err := d.Token()
			if err != nil {
				t.Fatalf("unterminated dict: %v", err)
			}
			switch x := tok.(type) {
			case xml.StartElement:
				if x.Name.Local != "key" {
					t.Fatalf("dict member starts with %q, want key", x.Name.Local)
				}
				key := plistText(t, d, x)
				var value xml.StartElement
				for {
					vt, err := d.Token()
					if err != nil {
						t.Fatalf("key %q has no value: %v", key, err)
					}
					if s, ok := vt.(xml.StartElement); ok {
						value = s
						break
					}
					if _, ok := vt.(xml.EndElement); ok {
						t.Fatalf("key %q has no value", key)
					}
				}
				out[key] = parsePlistValue(t, d, value)
			case xml.EndElement:
				return out
			}
		}
	}
	t.Fatalf("unexpected plist element %q", start.Name.Local)
	return nil
}

func plistText(t *testing.T, d *xml.Decoder, start xml.StartElement) string {
	t.Helper()
	var b strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			t.Fatalf("unterminated <%s>: %v", start.Name.Local, err)
		}
		switch x := tok.(type) {
		case xml.CharData:
			b.Write(x)
		case xml.EndElement:
			return b.String()
		case xml.StartElement:
			t.Fatalf("<%s> contains an element", start.Name.Local)
		}
	}
}

func plistStrings(t *testing.T, v any) []string {
	t.Helper()
	items, ok := v.([]any)
	if !ok {
		t.Fatalf("value is %T, want an array", v)
	}
	out := make([]string, len(items))
	for i, it := range items {
		s, ok := it.(string)
		if !ok {
			t.Fatalf("array item %d is %T, want a string", i, it)
		}
		out[i] = s
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// nastyArgs are the paths real machines produce: the app is in /Applications
// (a space), models live under "Application Support" or a folder the user named,
// and users type accents, CJK and emoji in those names.
var nastyArgs = []string{
	"/Users/Pat Model/.ezlm/bin/llama-b11960/llama-server",
	"/Users/Pat Model/Models/Ünïcödé 日本語 🚀/mödel.gguf",
	`/Users/p/Models/a & b <c> "d" 'e'/m.gguf`,
	"/Users/p/Models/tab\there/m.gguf",
	"/Users/p/Models/new\nline/m.gguf",
	"/Users/p/Models/trailing space /m.gguf",
	"/Users/p/Models/back\\slash $HOME %s/m.gguf",
	"--api-key",
	"REDACTED-KEY",
}

func TestLaunchdPlistRoundTripsAwkwardArguments(t *testing.T) {
	plist := plistContent("com.ezlm.server", nastyArgs[0], nastyArgs[1:], "/tmp/log file.log")
	root := parsePlist(t, plist)

	got := plistStrings(t, root["ProgramArguments"])
	want := append([]string{nastyArgs[0]}, nastyArgs[1:]...)
	if !equalStrings(got, want) {
		t.Fatalf("ProgramArguments did not survive the plist:\n got %q\nwant %q", got, want)
	}
	// Each argument must be one array element, not one element with the
	// arguments joined, and the array must not have picked up the label or
	// the log path.
	if len(got) != len(want) {
		t.Fatalf("got %d ProgramArguments, want %d", len(got), len(want))
	}
}

func TestLaunchdPlistStructure(t *testing.T) {
	logPath := "/Users/Pat Model/Library/Logs/ezlm server.log"
	root := parsePlist(t, plistContent("com.ezlm.server-test-1a2b3c4d", "/Users/Pat Model/bin/llama server", []string{"-m", "/m.gguf"}, logPath))

	if got := root["Label"]; got != "com.ezlm.server-test-1a2b3c4d" {
		t.Errorf("Label = %v", got)
	}
	if got, ok := root["RunAtLoad"].(bool); !ok || !got {
		t.Errorf("RunAtLoad = %v, want true", root["RunAtLoad"])
	}
	keepAlive, ok := root["KeepAlive"].(map[string]any)
	if !ok {
		t.Fatalf("KeepAlive = %T, want a dict", root["KeepAlive"])
	}
	if got, ok := keepAlive["SuccessfulExit"].(bool); !ok || got {
		t.Errorf("KeepAlive.SuccessfulExit = %v, want false: a crash must restart the server, a clean stop must not", keepAlive["SuccessfulExit"])
	}
	if got, ok := root["ThrottleInterval"].(int); !ok || got != 10 {
		t.Errorf("ThrottleInterval = %v, want 10", root["ThrottleInterval"])
	}
	if got := root["StandardOutPath"]; got != logPath {
		t.Errorf("StandardOutPath = %v, want %q", got, logPath)
	}
	if got := root["StandardErrorPath"]; got != logPath {
		t.Errorf("StandardErrorPath = %v, want %q", got, logPath)
	}
	if _, ok := root["EnvironmentVariables"]; ok {
		t.Log("note: the agent now sets an environment; update this test's expectations")
	}
}

// A control character has no XML escape, so it must not reach the file: one
// would make launchd reject the agent and the server would never start.
func TestLaunchdPlistControlCharacters(t *testing.T) {
	plist := plistContent("com.ezlm.server", "/bin/llama\x1b[0m", []string{"-m", "a\x00b\x07c"}, "/tmp/x.log")
	for _, r := range plist {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			t.Fatalf("plist contains control character %q", r)
		}
	}
	args := plistStrings(t, parsePlist(t, plist)["ProgramArguments"])
	if args[0] != "/bin/llama [0m" {
		t.Errorf("control character was not replaced: %q", args[0])
	}
	if args[2] != "a b c" {
		t.Errorf("control characters were not replaced: %q", args[2])
	}
}

// launchd opens StandardOutPath itself but does not create its folder, and a
// job whose log file can't be opened fails to start. The installer prepares
// both the agent's folder and the log.
func TestWritePlistFilePreparesLogFolder(t *testing.T) {
	dir := t.TempDir()
	plistPath := filepath.Join(dir, "Library", "LaunchAgents", "com.ezlm.server.plist")
	logPath := filepath.Join(dir, "ezlm", "logs", "server.log")

	if err := writePlistFile(plistPath, "content", logPath); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(plistPath); err != nil || string(b) != "content" {
		t.Fatalf("plist = %q, %v", b, err)
	}
	if st, err := os.Stat(logPath); err != nil || st.IsDir() {
		t.Fatalf("log file was not created: %v", err)
	}

	// A second install starts the log fresh instead of appending to a crash
	// from a previous run.
	if err := os.WriteFile(logPath, []byte("old crash\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writePlistFile(plistPath, "content2", logPath); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(logPath); len(b) != 0 {
		t.Errorf("log was not emptied: %q", b)
	}
}

func TestWritePlistFileReportsUnwritableLog(t *testing.T) {
	dir := t.TempDir()
	// A regular file where the log's folder should be: mkdir must fail, and
	// the installer must say so instead of loading a job that can't log.
	blocker := filepath.Join(dir, "logs")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	err := writePlistFile(filepath.Join(dir, "a.plist"), "content", filepath.Join(blocker, "server.log"))
	if err == nil {
		t.Fatal("writePlistFile succeeded with a file where the log folder should be")
	}
}
