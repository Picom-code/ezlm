//go:build darwin

package serve

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeLaunchctl_restartdarwin puts a stand-in launchctl first on PATH and returns the file
// it logs every call to. The job's loaded state lives in a marker file, so a
// test can see what a real launchd would have done: bootout unloads, bootstrap
// loads, print says whether the job is loaded.
func fakeLaunchctl_restartdarwin(t *testing.T) (logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "calls.log")
	loaded := filepath.Join(dir, "loaded")
	fails := filepath.Join(dir, "bootstrap-fails")
	script := `#!/bin/sh
echo "$@" >> "$LAUNCHCTL_LOG"
case "$1" in
print)
	[ -f "$LAUNCHCTL_LOADED" ] && exit 0
	exit 1
	;;
bootout)
	rm -f "$LAUNCHCTL_LOADED"
	exit 0
	;;
bootstrap)
	if [ -f "$LAUNCHCTL_FAILS" ] && [ "$(cat "$LAUNCHCTL_FAILS")" -gt 0 ]; then
		n=$(cat "$LAUNCHCTL_FAILS")
		echo $((n-1)) > "$LAUNCHCTL_FAILS"
		echo "Bootstrap failed: 5: Input/output error" >&2
		exit 5
	fi
	touch "$LAUNCHCTL_LOADED"
	exit 0
	;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "launchctl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LAUNCHCTL_LOG", logPath)
	t.Setenv("LAUNCHCTL_LOADED", loaded)
	t.Setenv("LAUNCHCTL_FAILS", fails)
	t.Setenv("EZLM_HOME", filepath.Join(dir, "ezlm"))
	t.Setenv("HOME", filepath.Join(dir, "home"))
	if err := os.MkdirAll(logsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	return logPath
}

// logsDir is where LogPath() writes with the fake home in place.
func logsDir() string { return filepath.Dir(LogPath()) }

func calls(t *testing.T, logPath string) []string {
	t.Helper()
	b, err := os.ReadFile(logPath)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func countCalls(t *testing.T, logPath, verb string) int {
	t.Helper()
	n := 0
	for _, c := range calls(t, logPath) {
		if strings.HasPrefix(c, verb+" ") {
			n++
		}
	}
	return n
}

// plistValues parses the top-level dict of a plist into key -> text. A
// dict or array value is flattened to the text of its contents, which is
// enough to see which keys and strings it holds.
func plistValues(t *testing.T, path string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dec := xml.NewDecoder(bytes.NewReader(b))
	m := map[string]string{}
	key := ""
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "key":
			var s string
			if err := dec.DecodeElement(&s, &start); err != nil {
				t.Fatal(err)
			}
			key = strings.TrimSpace(s)
		case "dict", "array":
			if key == "" {
				continue // the plist's own <dict>
			}
			var text []string
			depth := 1
			for depth > 0 {
				tok, err := dec.Token()
				if err != nil {
					t.Fatal(err)
				}
				switch e := tok.(type) {
				case xml.StartElement:
					depth++
					if depth == 2 && (e.Name.Local == "true" || e.Name.Local == "false") {
						text = append(text, e.Name.Local)
					}
				case xml.CharData:
					if s := strings.TrimSpace(string(e)); s != "" {
						text = append(text, s)
					}
				case xml.EndElement:
					depth--
				}
			}
			m[key] = strings.Join(text, "|")
			key = ""
		case "true", "false", "string", "integer":
			var s string
			if err := dec.DecodeElement(&s, &start); err != nil {
				t.Fatal(err)
			}
			if key == "" {
				continue
			}
			if start.Name.Local == "true" || start.Name.Local == "false" {
				s = start.Name.Local
			}
			m[key] = s
			key = ""
		}
	}
	return m
}

func writeServer(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func testConfig(server string) *Config {
	return &Config{Server: server, Port: 18081, ModelPath: "/models/m.gguf", ModelID: "m", Ctx: 4096, NGL: 0}
}

// The plist's restart contract: start at login, restart after an unsuccessful
// exit (a crash or a non-zero exit), and never respawn faster than launchd's
// ten-second floor. A clean exit is a deliberate stop (Stop unloads the job),
// so launchd must not restart that.
func TestLaunchdRestartContract(t *testing.T) {
	fakeLaunchctl_restartdarwin(t)
	server := filepath.Join(os.Getenv("EZLM_HOME"), "bin", "llama-b1", "llama-server")
	writeServer(t, server)
	if err := Install(testConfig(server)); err != nil {
		t.Fatal(err)
	}
	v := plistValues(t, plistPath())
	if v["Label"] != label() {
		t.Errorf("Label = %q, want %q", v["Label"], label())
	}
	if v["RunAtLoad"] != "true" {
		t.Errorf("RunAtLoad = %q, want true", v["RunAtLoad"])
	}
	keep := v["KeepAlive"]
	if !strings.Contains(keep, "SuccessfulExit") || !strings.Contains(keep, "false") {
		t.Errorf("KeepAlive = %q, want a dict with SuccessfulExit false", keep)
	}
	if v["ThrottleInterval"] != "10" {
		t.Errorf("ThrottleInterval = %q, want 10 (launchd's floor, so a crash loop cannot spin)", v["ThrottleInterval"])
	}
	// Since T16 a05 the job runs through /bin/sh so a Stop survives a login.
	if got := v["ProgramArguments"]; !strings.Contains(got, server) {
		t.Errorf("ProgramArguments = %q, want it to mention %q", got, server)
	}
	if v["StandardOutPath"] != LogPath() || v["StandardErrorPath"] != LogPath() {
		t.Errorf("log paths = %q / %q, want %q", v["StandardOutPath"], v["StandardErrorPath"], LogPath())
	}
	if countCalls(t, os.Getenv("LAUNCHCTL_LOG"), "bootstrap") != 1 {
		t.Errorf("bootstrap calls = %d, want 1", countCalls(t, os.Getenv("LAUNCHCTL_LOG"), "bootstrap"))
	}
}

// An update moves or replaces the server program while the job is loaded.
// Install must unload the old job before loading the new one, and the plist
// must name the new program; launchd execs the program only when it loads the
// job, so a plain rewrite with no reload keeps running the old binary.
func TestInstallReloadsJobWhenTheServerMoves(t *testing.T) {
	logPath := fakeLaunchctl_restartdarwin(t)
	oldServer := filepath.Join(os.Getenv("EZLM_HOME"), "bin", "llama-b1", "llama-server")
	writeServer(t, oldServer)
	if err := Install(testConfig(oldServer)); err != nil {
		t.Fatal(err)
	}
	if got := plistValues(t, plistPath())["ProgramArguments"]; !strings.HasPrefix(got, oldServer) {
		t.Fatalf("ProgramArguments = %q, want %q", got, oldServer)
	}

	// The engine update installs a new folder and the app re-registers it.
	newServer := filepath.Join(os.Getenv("EZLM_HOME"), "bin", "llama-b2", "llama-server")
	writeServer(t, newServer)
	if err := os.RemoveAll(filepath.Dir(oldServer)); err != nil {
		t.Fatal(err)
	}
	if err := Install(testConfig(newServer)); err != nil {
		t.Fatal(err)
	}
	if got := plistValues(t, plistPath())["ProgramArguments"]; !strings.HasPrefix(got, newServer) {
		t.Fatalf("ProgramArguments = %q, want the moved server %q", got, newServer)
	}
	all := calls(t, logPath)
	lastBootout, lastBootstrap := -1, -1
	for i, c := range all {
		if strings.HasPrefix(c, "bootout ") {
			lastBootout = i
		}
		if strings.HasPrefix(c, "bootstrap ") {
			lastBootstrap = i
		}
	}
	if lastBootout < 0 || lastBootstrap < 0 || lastBootout > lastBootstrap {
		t.Fatalf("calls = %v, want a bootout before the last bootstrap", all)
	}

	// The update replaces the program in place: same path, new file. The job
	// must be reloaded so launchd execs the new file instead of keeping the
	// process it started from the old one.
	before := countCalls(t, logPath, "bootstrap")
	if err := Install(testConfig(newServer)); err != nil {
		t.Fatal(err)
	}
	if after := countCalls(t, logPath, "bootstrap"); after != before+1 {
		t.Fatalf("bootstrap calls = %d after an in-place update, want %d", after, before+1)
	}
}

// A plist that names a missing program is worse than no plist: launchd fails
// the exec and retries every ThrottleInterval, logging an error each time, so
// the server is silently down. Install must refuse, and leave the working job
// and plist alone.
func TestInstallKeepsTheJobWhenTheServerIsMissing(t *testing.T) {
	logPath := fakeLaunchctl_restartdarwin(t)
	server := filepath.Join(os.Getenv("EZLM_HOME"), "bin", "llama-b1", "llama-server")
	writeServer(t, server)
	if err := Install(testConfig(server)); err != nil {
		t.Fatal(err)
	}
	before, bootouts := countCalls(t, logPath, "bootstrap"), countCalls(t, logPath, "bootout")
	gone := filepath.Join(os.Getenv("EZLM_HOME"), "bin", "llama-b2", "llama-server")
	err := Install(testConfig(gone))
	if err == nil {
		t.Fatal("Install accepted a server program that isn't there")
	}
	if !strings.Contains(err.Error(), gone) {
		t.Errorf("error %q does not name the missing program %q", err, gone)
	}
	if got := plistValues(t, plistPath())["ProgramArguments"]; !strings.HasPrefix(got, server) {
		t.Errorf("ProgramArguments = %q, want the working server %q to stay", got, server)
	}
	if after := countCalls(t, logPath, "bootstrap"); after != before {
		t.Errorf("bootstrap calls = %d, want %d: a failed install must not load another job", after, before)
	}
	if after := countCalls(t, logPath, "bootout"); after != bootouts {
		t.Errorf("bootout calls = %d, want %d: the working job was unloaded for an install that could not succeed", after, bootouts)
	}
}

// launchd returns error 5 for a moment after a bootout; bootstrap retries
// instead of reporting a failure the user can do nothing about.
func TestBootstrapRetriesWhenLaunchdIsBusy(t *testing.T) {
	logPath := fakeLaunchctl_restartdarwin(t)
	if err := os.WriteFile(os.Getenv("LAUNCHCTL_FAILS"), []byte("2"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := filepath.Join(os.Getenv("EZLM_HOME"), "bin", "llama-b1", "llama-server")
	writeServer(t, server)
	if err := Install(testConfig(server)); err != nil {
		t.Fatal(err)
	}
	if n := countCalls(t, logPath, "bootstrap"); n != 3 {
		t.Errorf("bootstrap calls = %d, want 3 (two refusals, then success)", n)
	}
	if _, err := os.Stat(plistPath()); err != nil {
		t.Errorf("plist missing after a retried bootstrap: %v", err)
	}
}
