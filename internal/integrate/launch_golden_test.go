package integrate

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Golden launchers live in testdata/launch. EZLM_UPDATE_GOLDEN=1 rewrites
// them; read the diff before committing.
func launchGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "launch", name)
	if os.Getenv("EZLM_UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v (EZLM_UPDATE_GOLDEN=1 writes it)", name, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from the golden file:\n--- got\n%s\n--- want\n%s", name, got, want)
	}
}

// launchGoldenCases are the launchers PLAN 9.2 asks for: a non-ASCII
// profile folder, % in a path, an npm .cmd shim, a quote in an argument, '
// in a Mac path, the dsh web server with its private Node first on PATH.
var launchGoldenCases = []struct {
	name, goos, exit string
	spec             LaunchSpec
}{
	{"win-opencode-jorg.cmd", "windows", `C:\Users\Jörg\.ezlm\launch\opencode.exit`, LaunchSpec{
		ID: "opencode", Title: "OpenCode (EZLM)",
		Bin:  `C:\Users\Jörg\.ezlm\tools\opencode\2.2.5\opencode.exe`,
		Args: []string{"--standalone", "--model", "ezlm/qwen3.8-27b"},
		Dir:  `C:\Users\Jörg\EZLM Workspace`,
	}},
	{"win-hermes-percent.cmd", "windows", `C:\Users\100%Sam\.ezlm\launch\hermes.exit`, LaunchSpec{
		ID: "hermes", Title: "Hermes (EZLM)",
		Bin:  `C:\Users\100%Sam\AppData\Local\hermes\bin\hermes.exe`,
		Args: []string{"chat", "--provider", "custom:ezlm", "-m", "qwen%2"},
		Env:  []string{`PATH=C:\Users\100%Sam\AppData\Local\hermes\bin;${PATH}`},
		Dir:  `C:\Users\100%Sam\EZLM Workspace`,
	}},
	{"win-opencode-npm-shim.cmd", "windows", `C:\Users\Sam & Co\.ezlm\launch\opencode.exit`, LaunchSpec{
		ID: "opencode", Title: "OpenCode (EZLM)",
		Bin:  `C:\Users\Sam & Co\AppData\Roaming\npm\opencode.cmd`,
		Args: []string{"--model", "ezlm/qwen%x"},
		Dir:  `C:\Users\Sam & Co\EZLM Workspace`,
	}},
	{"win-quote-arg.cmd", "windows", `C:\Users\Sam\.ezlm\launch\hermes.exit`, LaunchSpec{
		ID: "hermes", Title: "Hermes (EZLM)",
		Bin:  `C:\Users\Sam\AppData\Local\Microsoft\WindowsApps\hermes.exe`,
		Args: []string{"chat", "-q", `say "hi" & (bye) ^ <ok>`, `C:\dir with space\`},
		Dir:  `C:\Users\Sam\EZLM Workspace`,
	}},
	{"win-dsh-web.cmd", "windows", `C:\Users\Sam\.ezlm\launch\dsh.exit`, LaunchSpec{
		ID: "dsh", Mode: ModeWeb, Title: "DeepSeek Harness (EZLM)",
		Bin:  `C:\Users\Sam\.ezlm\tools\node\24.11.1\node.exe`,
		Args: []string{`C:\Users\Sam\.ezlm\tools\dsh\0.2.1-rc.5\node_modules\@deepseek-ai\dsh\bin\dsh.js`, "web", "--port", "3081"},
		Env:  []string{"DSH_TELEMETRY_MODE=DISABLED", `PATH=C:\Users\Sam\.ezlm\tools\node\24.11.1;${PATH}`},
		Dir:  `C:\Users\Sam\EZLM Workspace`,
		URL:  "http://127.0.0.1:3081",
	}},
	{"mac-opencode-quote.command", "darwin", `/Users/o'brien/.ezlm/launch/opencode.exit`, LaunchSpec{
		ID: "opencode", Title: "OpenCode (EZLM)",
		Bin:  `/Users/o'brien/.ezlm/tools/opencode/2.2.5/opencode`,
		Args: []string{"--standalone", "--model", "ezlm/qwen3.5-4b"},
		Dir:  `/Users/o'brien/EZLM Workspace`,
	}},
	{"mac-hermes.command", "darwin", `/Users/sam/.ezlm/launch/hermes.exit`, LaunchSpec{
		ID: "hermes", Title: "Hermes (EZLM)",
		Bin:  `/Users/sam/.local/bin/hermes`,
		Args: []string{"chat", "--provider", "custom:ezlm", "-m", "qwen3.5-4b", `it's "$HOME"`},
		Dir:  `/Users/sam/EZLM Workspace`,
	}},
	{"mac-dsh-web.command", "darwin", `/Users/sam/.ezlm/launch/dsh.exit`, LaunchSpec{
		ID: "dsh", Mode: ModeWeb, Title: "DeepSeek Harness (EZLM)",
		Bin:  `/Users/sam/.ezlm/tools/dsh/0.2.1-rc.5/node_modules/.bin/dsh`,
		Args: []string{"web", "--port", "3081"},
		Env:  []string{"DSH_TELEMETRY_MODE=DISABLED", "PATH=/Users/sam/.ezlm/tools/node/24.11.1/bin:${PATH}"},
		Dir:  `/Users/sam/EZLM Workspace`,
		URL:  "http://127.0.0.1:3081",
	}},
	{"linux-opencode.sh", "linux", `/home/sam/.ezlm/launch/opencode.exit`, LaunchSpec{
		ID: "opencode", Title: "OpenCode (EZLM)",
		Bin:  `/home/sam/.ezlm/tools/opencode/2.2.5/opencode`,
		Args: []string{"--standalone", "--model", "ezlm/qwen3.5-4b"},
		Env:  []string{"OPENCODE_DISABLE_AUTOUPDATE=1"},
		Dir:  `/home/sam/EZLM Workspace`,
	}},
}

func TestLaunchGoldenScripts(t *testing.T) {
	for _, c := range launchGoldenCases {
		t.Run(c.name, func(t *testing.T) {
			path, body, err := launchFile(c.goos, c.spec, c.exit)
			if err != nil {
				t.Fatal(err)
			}
			if want := map[string]string{"windows": ".cmd", "darwin": ".command", "linux": ".sh"}[c.goos]; filepath.Base(path) != c.spec.ID+want {
				t.Errorf("launcher name %s", path)
			}
			if c.goos == "windows" {
				if bytes.HasPrefix(body, []byte("\xef\xbb\xbf")) {
					t.Error("the .cmd must have no BOM")
				}
				if bytes.Count(body, []byte("\n")) != bytes.Count(body, []byte("\r\n")) {
					t.Error("every .cmd line must end in CRLF")
				}
				lines := bytes.SplitN(body, []byte("\r\n"), 3)
				for _, b := range append(lines[0], lines[1]...) {
					if b >= 0x80 {
						t.Error("the first two .cmd lines must be ASCII")
					}
				}
			}
			launchGolden(t, c.name, body)
		})
	}
}

func TestLaunchGoldenOpeners(t *testing.T) {
	var b strings.Builder
	q := func(argv []string) string {
		out := make([]string, len(argv))
		for i, a := range argv {
			out[i] = fmt.Sprintf("%q", a)
		}
		return strings.Join(out, " ")
	}
	for _, l := range []string{`C:\Users\Jörg\.ezlm\launch\opencode.cmd`, `C:\Users\Sam & Co (2)\.ezlm\launch\hermes.cmd`} {
		fmt.Fprintf(&b, "wt.exe %s\n", q(launchWTArgs("OpenCode (EZLM)", l)))
		fmt.Fprintf(&b, "start  %s\n", launchStartLine("OpenCode (EZLM)", l))
	}
	fmt.Fprintf(&b, "wt.exe %s\n", q(launchWTArgs("A; B (EZLM)", `C:\x\a.cmd`)))
	fmt.Fprintf(&b, "start  %s\n", launchStartLine(`A "B" (EZLM)`, `C:\x\a.cmd`))
	fmt.Fprintf(&b, "mac    %s\n", q(launchMacArgv(`/Users/o'brien/.ezlm/launch/opencode.command`)))
	look := func(name string) (string, error) {
		if name == "konsole" || name == "xterm" {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	argv, err := launchLinuxArgv("/home/sam/.ezlm/launch/opencode.sh", look)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(&b, "linux  %s\n", q(argv))
	if _, err := launchLinuxArgv("/x.sh", func(string) (string, error) { return "", errors.New("no") }); !errors.Is(err, errLaunchNoTerminal) {
		t.Errorf("no terminal: %v", err)
	}
	launchGolden(t, "openers.txt", []byte(b.String()))
}

func TestLaunchWindowsFallback(t *testing.T) {
	const l = `C:\Users\Sam\.ezlm\launch\opencode.cmd`
	var calls []string
	wtOK := func(wt string, args []string) error { calls = append(calls, "wt"); return nil }
	wtFail := func(wt string, args []string) error {
		calls = append(calls, "wt")
		return errors.New("the alias is stale")
	}
	start := func(line string) error {
		calls = append(calls, "start")
		if line != launchStartLine("OpenCode (EZLM)", l) {
			t.Errorf("start line %s", line)
		}
		return nil
	}
	for _, c := range []struct {
		wt    string
		run   func(string, []string) error
		used  string
		calls string
	}{
		{`C:\Users\Sam\AppData\Local\Microsoft\WindowsApps\wt.exe`, wtOK, "wt", "wt"},
		{`C:\Users\Sam\AppData\Local\Microsoft\WindowsApps\wt.exe`, wtFail, "start", "wt start"},
		{"", wtOK, "start", "start"},
	} {
		calls = nil
		used, err := launchWindowsOpen(c.wt, "OpenCode (EZLM)", l, c.run, start)
		if err != nil || used != c.used || strings.Join(calls, " ") != c.calls {
			t.Errorf("wt %q: used %q, calls %v, err %v", c.wt, used, calls, err)
		}
	}
	_, err := launchWindowsOpen("wt.exe", "OpenCode (EZLM)", l, wtFail, func(string) error { return errors.New("no console") })
	if err == nil {
		t.Error("both failing must fail")
	}
}

func TestLaunchCRTQuote(t *testing.T) {
	for in, want := range map[string]string{
		`a b`:        `"a b"`,
		`say "hi"`:   `"say \"hi\""`,
		`C:\dir\`:    `"C:\dir\\"`,
		`a\\"b`:      `"a\\\\\"b"`,
		`a\b`:        `"a\b"`,
		``:           `""`,
		`ezlm/qwen3`: `"ezlm/qwen3"`,
	} {
		if got := launchCRTQuote(in); got != want {
			t.Errorf("%s: got %s, want %s", in, got, want)
		}
	}
}
