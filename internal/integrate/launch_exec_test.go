package integrate

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestLaunchHelperProcess isn't a test of its own: the launcher, smoke and
// pty tests run this test binary as the "app" (EZLM_LAUNCH_HELPER says what
// it does). It does nothing in a normal test run.
func TestLaunchHelperProcess(t *testing.T) {
	mode := os.Getenv("EZLM_LAUNCH_HELPER")
	if mode == "" {
		return
	}
	// os.Exit below skips TestMain's cleanup of this process's own temp home.
	if root := filepath.Dir(os.Getenv("EZLM_HOME")); strings.HasPrefix(filepath.Base(root), "ezlm-integrate-test-") {
		os.RemoveAll(root)
	}
	var args []string
	for i, a := range os.Args {
		if a == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	switch mode {
	case "ready":
		fmt.Println("\x1b[1mREADY\x1b[0m to go")
		time.Sleep(time.Minute)
	case "fatal":
		fmt.Println("Error: Database is not empty and has no session table")
		os.Exit(1)
	case "tree":
		child := exec.Command(os.Args[0], "-test.run=^TestLaunchHelperProcess$")
		child.Env = append(os.Environ(), "EZLM_LAUNCH_HELPER=ready")
		out, _ := child.StdoutPipe()
		if err := child.Start(); err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
		// Wait until the child is past its own cleanup before saying so.
		bufio.NewReader(out).ReadString('\n')
		fmt.Printf("CHILD:%d\nREADY\n", child.Process.Pid)
		time.Sleep(time.Minute)
	default: // an exit code: show what arrived, then exit with it
		wd, _ := os.Getwd()
		b, _ := json.Marshal(args)
		fmt.Printf("ARGS:%s\nDIR:%s\nPROBE:%s\n", b, wd, os.Getenv("EZLM_LAUNCH_PROBE"))
		if os.Getenv("EZLM_LAUNCH_HELPER_WAIT") != "" {
			fmt.Println("WAITING")
			bufio.NewReader(os.Stdin).ReadString('\n')
		}
		code, _ := strconv.Atoi(mode)
		os.Exit(code)
	}
	os.Exit(0)
}

// launchTestHelper is the helper as a program and its leading arguments.
func launchTestHelper(t *testing.T) (string, []string) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe, []string{"-test.run=^TestLaunchHelperProcess$", "--"}
}

// launchTerm runs cmd in a wide pty and collects what it draws.
type launchTerm struct {
	t       *testing.T
	p       *PTY
	mu      sync.Mutex
	raw     strings.Builder
	drained chan struct{} // closed when the terminal's output ends
}

func launchStartTerm(t *testing.T, cmd *exec.Cmd) *launchTerm {
	t.Helper()
	p, err := StartPTY(cmd, 400, 50)
	if err != nil {
		t.Fatal(err)
	}
	lt := &launchTerm{t: t, p: p, drained: make(chan struct{})}
	t.Cleanup(func() { p.Close() })
	go func() {
		defer close(lt.drained)
		buf := make([]byte, 4096)
		for {
			n, err := p.Read(buf)
			lt.mu.Lock()
			lt.raw.Write(buf[:n])
			lt.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	return lt
}

func (lt *launchTerm) text() string {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	return StripANSI(lt.raw.String())
}

func (lt *launchTerm) waitFor(s string, d time.Duration) {
	lt.t.Helper()
	for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		if strings.Contains(lt.text(), s) {
			return
		}
	}
	lt.t.Fatalf("%q did not appear; the screen:\n%s", s, lt.text())
}

func (lt *launchTerm) waitExit(d time.Duration) {
	lt.t.Helper()
	for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		if lt.p.Exited() {
			// The terminal passes on the last output after the program ends.
			select {
			case <-lt.drained:
			case <-time.After(5 * time.Second):
			}
			return
		}
	}
	lt.t.Fatalf("the launcher didn't end; the screen:\n%s", lt.text())
}

// launchHelperArgs are the arguments the run tests pass through a launcher.
var launchHelperArgs = []string{"--model", "ezlm/qwen3.8-27b", "a b", `say "hi" & (bye) <ok> |x`, "100%", "50%PATH%", `C:\dir with space\`, "Jörg's", "$HOME", "!x"}

// launchWeirdHome points EZLM_HOME (and so LaunchDir and ExitFile) and the
// workspace into a folder with a space, %, &, ' and a non-ASCII letter.
func launchWeirdHome(t *testing.T) (ws string) {
	weird := filepath.Join(t.TempDir(), "Jörg 100% & Co's")
	ws = filepath.Join(weird, "EZLM Workspace")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EZLM_HOME", filepath.Join(weird, ".ezlm"))
	return ws
}

// launchCheckRun checks what the helper printed through a launcher.
func launchCheckRun(t *testing.T, screen, ws string, want []string) {
	t.Helper()
	m := regexp.MustCompile(`ARGS:(\[.*\])`).FindStringSubmatch(screen)
	if m == nil {
		t.Fatalf("no ARGS line:\n%s", screen)
	}
	var got []string
	if err := json.Unmarshal([]byte(m[1]), &got); err != nil {
		t.Fatalf("ARGS %s: %v", m[1], err)
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("arguments arrived as %q, want %q", got, want)
	}
	real, _ := filepath.EvalSymlinks(ws)
	if !strings.Contains(screen, "DIR:"+ws) && !strings.Contains(screen, "DIR:"+real) {
		t.Errorf("the app didn't start in %s:\n%s", ws, screen)
	}
	if !strings.Contains(screen, "PROBE:x:outer") {
		t.Errorf("Env with ${…} didn't arrive:\n%s", screen)
	}
}

// TestLaunchScriptRuns runs the real launcher for this OS (and on Unix the
// other shell's too) in a pty, with the test binary as the app: arguments,
// environment and working folder arrive exactly, the exit file gets the
// app's code and the window waits for a key after a failure.
func TestLaunchScriptRuns(t *testing.T) {
	ws := launchWeirdHome(t)
	t.Setenv("EZLM_LAUNCH_OUTER", "outer")
	exe, lead := launchTestHelper(t)
	spec := LaunchSpec{ID: "helper", Title: "Helper (EZLM)", Bin: exe, Args: append(lead, launchHelperArgs...), Dir: ws,
		Env: []string{"EZLM_LAUNCH_HELPER=3", "EZLM_LAUNCH_PROBE=x:${EZLM_LAUNCH_OUTER}"}}
	l, err := Launch(spec)
	if err != nil {
		t.Fatal(err)
	}
	if !l.DryRun || l.ExitFile != ExitFile("helper") || filepath.Dir(l.Launcher) != LaunchDir() {
		t.Fatalf("%+v", l)
	}
	launchers := map[string]string{runtime.GOOS: l.Launcher}
	if runtime.GOOS != "windows" {
		// Also the other Unix flavour, where its shell exists.
		other, zsh := "linux", false
		if runtime.GOOS != "darwin" {
			other, zsh = "darwin", true
		}
		if _, err := os.Stat("/bin/zsh"); err == nil || !zsh {
			body, err := launchShScript(spec, l.ExitFile, zsh)
			if err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(LaunchDir(), "helper-"+other)
			os.WriteFile(p, []byte(body), 0o755)
			launchers[other] = p
		}
	}
	for goos, path := range launchers {
		t.Run(goos, func(t *testing.T) {
			os.Remove(l.ExitFile)
			lt := launchStartTerm(t, exec.Command(path))
			lt.waitFor("Helper stopped", 30*time.Second)
			lt.waitFor("ress", 5*time.Second) // "Press any key …" / "Press Enter …"
			lt.p.Write([]byte("\r"))
			lt.waitExit(15 * time.Second)
			screen := lt.text()
			launchCheckRun(t, screen, ws, launchHelperArgs)
			code, at, ok := ReadExit(l.ExitFile)
			if !ok || code != 3 || time.Since(at) > time.Minute {
				t.Errorf("exit file: %d %v %v", code, at, ok)
			}
			for _, bad := range []string{"is not recognized", "not found", "syntax"} {
				if strings.Contains(screen, bad) {
					t.Errorf("the launcher printed %q:\n%s", bad, screen)
				}
			}
		})
	}
}

// TestLaunchScriptSuccess: exit code 0 is recorded and nothing waits.
func TestLaunchScriptSuccess(t *testing.T) {
	ws := launchWeirdHome(t)
	t.Setenv("EZLM_LAUNCH_OUTER", "outer")
	exe, lead := launchTestHelper(t)
	l, err := Launch(LaunchSpec{ID: "helper", Title: "Helper (EZLM)", Bin: exe, Args: append(lead, "ok"), Dir: ws,
		Env: []string{"EZLM_LAUNCH_HELPER=0", "EZLM_LAUNCH_PROBE=x:${EZLM_LAUNCH_OUTER}"}})
	if err != nil {
		t.Fatal(err)
	}
	lt := launchStartTerm(t, exec.Command(l.Launcher))
	lt.waitExit(30 * time.Second)
	launchCheckRun(t, lt.text(), ws, []string{"ok"})
	if code, _, ok := ReadExit(l.ExitFile); !ok || code != 0 {
		t.Errorf("exit file: %d %v", code, ok)
	}
	if strings.Contains(lt.text(), "stopped") {
		t.Errorf("a clean exit must not say it stopped:\n%s", lt.text())
	}
}

// TestLaunchRewriteWhileRunning: a launcher rewritten while its window still
// runs the app (the user opened the app again after a model switch) must not
// change what the first window does when its app ends. cmd.exe rereads a
// batch file by name, so this is the Windows launcher's block at work.
func TestLaunchRewriteWhileRunning(t *testing.T) {
	ws := launchWeirdHome(t)
	t.Setenv("EZLM_LAUNCH_OUTER", "outer")
	exe, lead := launchTestHelper(t)
	spec := LaunchSpec{ID: "helper", Title: "Helper (EZLM)", Bin: exe, Args: append(lead, "first"), Dir: ws,
		Env: []string{"EZLM_LAUNCH_HELPER=5", "EZLM_LAUNCH_HELPER_WAIT=1", "EZLM_LAUNCH_PROBE=x:${EZLM_LAUNCH_OUTER}"}}
	l, err := Launch(spec)
	if err != nil {
		t.Fatal(err)
	}
	lt := launchStartTerm(t, exec.Command(l.Launcher))
	lt.waitFor("WAITING", 30*time.Second)
	long := spec
	long.Title = "A Much Longer Helper Name (EZLM)"
	long.Args = append(lead, strings.Repeat("second ", 40))
	long.Env = append([]string{"EZLM_LAUNCH_EXTRA=" + strings.Repeat("y", 300)}, spec.Env...)
	if _, err := Launch(long); err != nil {
		t.Fatal(err)
	}
	lt.p.Write([]byte("\r"))
	lt.waitFor("Helper stopped", 30*time.Second)
	lt.p.Write([]byte("\r"))
	lt.waitExit(15 * time.Second)
	if code, _, ok := ReadExit(l.ExitFile); !ok || code != 5 {
		t.Errorf("exit file: %d %v\n%s", code, ok, lt.text())
	}
	for _, bad := range []string{"is not recognized", "not found", "Much Longer"} {
		if strings.Contains(lt.text(), bad) {
			t.Errorf("the first window ran the rewritten launcher (%q):\n%s", bad, lt.text())
		}
	}
}
