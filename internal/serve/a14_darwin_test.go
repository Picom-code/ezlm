package serve

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeLaunchctl puts a fake launchctl first on PATH and returns the file it
// appends each invocation to. "print" fails, so bootout's wait returns at once.
func fakeLaunchctl(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls.log")
	t.Setenv("LAUNCHCTL_LOG", calls)
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$LAUNCHCTL_LOG"
case "$1" in
	print) exit 1 ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "launchctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return calls
}

func TestPlistContentsRoundTripsArguments(t *testing.T) {
	c := &Config{
		Server:    "/Applications/EZLM & Co/llama-server",
		ModelPath: "/Users/me/.ezlm/models/a<b>.gguf",
		ModelID:   "m&m",
		Port:      8080,
		Ctx:       4096,
		NGL:       999,
		Key:       "REDACTED-KEY",
	}
	plist := plistXML(c)

	var doc struct {
		Strings []string `xml:"dict>string"`
		Args    []string `xml:"dict>array>string"`
	}
	if err := xml.Unmarshal([]byte(plist), &doc); err != nil {
		t.Fatalf("the plist is not well-formed XML: %v", err)
	}
	if len(doc.Strings) != 3 {
		t.Fatalf("got %d plain strings, want label, stdout and stderr", len(doc.Strings))
	}
	if doc.Strings[0] != label() {
		t.Errorf("label = %q, want %q", doc.Strings[0], label())
	}
	if doc.Strings[1] != LogPath() || doc.Strings[2] != LogPath() {
		t.Errorf("log paths = %q and %q, want %q", doc.Strings[1], doc.Strings[2], LogPath())
	}

	// Since T16 a05 the job runs the server through /bin/sh so a Stop
	// survives the next login: three argv entries, and the whole server
	// command line lives in the third one, shell-quoted.
	if len(doc.Args) != 3 || doc.Args[0] != "/bin/sh" || doc.Args[1] != "-c" {
		t.Fatalf("got %d program arguments, want /bin/sh -c <script>: %q", len(doc.Args), doc.Args)
	}
	for _, a := range append([]string{c.Server}, c.Args()...) {
		if !strings.Contains(doc.Args[2], a) {
			t.Errorf("the job script does not mention %q: %s", a, doc.Args[2])
		}
	}
}

func TestInstallWritesPlistAndLogFolder(t *testing.T) {
	home := t.TempDir()
	ezlmHome := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("EZLM_HOME", ezlmHome)
	calls := fakeLaunchctl(t)

	c := &Config{
		Server: "/tmp/EZLM & Co/llama-server", ModelPath: "/tmp/m.gguf",
		ModelID: "m", Port: 8080, Ctx: 4096, NGL: 999, Key: "REDACTED-KEY",
	}
	if err := Install(c); err != nil {
		t.Fatal(err)
	}

	plist := filepath.Join(home, "Library", "LaunchAgents", label()+".plist")
	if _, err := os.Stat(plist); err != nil {
		t.Fatalf("the launch agent was not written: %v", err)
	}
	// launchd cannot start a job whose StandardOutPath folder is missing.
	if _, err := os.Stat(filepath.Dir(LogPath())); err != nil {
		t.Fatalf("the server log folder was not created: %v", err)
	}

	b, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, "bootout "+domain()+"/"+label()) {
		t.Errorf("the old agent was not unloaded, calls were:\n%s", got)
	}
	if !strings.Contains(got, "bootstrap "+domain()+" "+plist) {
		t.Errorf("the agent was not loaded from %s, calls were:\n%s", plist, got)
	}
}

func TestBootstrapRetriesAfterLaunchdRefuses(t *testing.T) {
	dir := t.TempDir()
	count := filepath.Join(dir, "count")
	t.Setenv("HOME", t.TempDir())
	script := "#!/bin/sh\n" +
		"n=$(cat " + count + " 2>/dev/null || echo 0)\n" +
		"n=$((n + 1))\n" +
		"echo $n > " + count + "\n" +
		"[ $n -ge 2 ]\n"
	if err := os.WriteFile(filepath.Join(dir, "launchctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := bootstrap(); err != nil {
		t.Fatalf("bootstrap did not retry: %v", err)
	}
	b, err := os.ReadFile(count)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(b)) != "2" {
		t.Errorf("launchctl was called %s times, want 2", strings.TrimSpace(string(b)))
	}
}
