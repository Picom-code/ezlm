// Package fakeproc builds small fake processes for lifecycle tests: a process
// that sleeps until it is killed, one that exits with a chosen code, one that
// starts children, and one that listens on a port. It re-execs the test binary,
// so it needs no build step and no files on disk.
//
// A package that uses it adds one test and calls Run from it:
//
//	func TestFakeProcHelper(t *testing.T) { fakeproc.Run() }
//
// Then tests start helpers with Start or Cmd:
//
//	p := fakeproc.Start(t, "sleep")
//	p.Ready(t)
//	p.Stop() // kills the helper and everything it started, by PID
//
// The helper only ever kills processes it started itself: the child is put in
// its own process group and the group is killed, so a reused PID can never be
// hit and no process on the machine is touched by name.
package fakeproc

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// HelperTest is the test that Run must be called from. Start re-execs the
// test binary with -test.run=^HelperTest$, so a package that never defines it
// gets a clear "no tests to run" failure instead of a wrong process.
const HelperTest = "TestFakeProcHelper"

// Ready is the line a helper prints once it is up.
const Ready = "READY"

// runTimeout bounds the wait for a helper's output. It is a variable so tests
// that expect silence can shorten it.
var runTimeout = 10 * time.Second

// Run is the helper side. Call it from a test named TestFakeProcHelper; it
// returns at once in a normal test run and otherwise runs the mode the parent
// asked for, then exits the process.
func Run() {
	mode := os.Getenv("FAKEPROC_MODE")
	if mode == "" {
		return
	}
	args := afterDashDash(os.Args)
	if len(args) > 0 { // args[0] is the mode itself
		args = args[1:]
	}
	switch mode {
	case "sleep":
		sleepFor(args)
	case "stubborn":
		signal.Ignore(os.Interrupt, syscall.SIGTERM)
		fmt.Println(Ready)
		select {}
	case "exit":
		code := 0
		if len(args) > 0 {
			code, _ = strconv.Atoi(args[0])
		}
		fmt.Println(Ready)
		os.Exit(code)
	case "tree":
		tree(args)
	case "http":
		serveHTTP(args)
	default:
		fmt.Println("fakeproc: unknown mode", mode)
		os.Exit(2)
	}
	os.Exit(0)
}

// afterDashDash returns the arguments the parent put after "--".
func afterDashDash(argv []string) []string {
	for i, a := range argv {
		if a == "--" {
			return argv[i+1:]
		}
	}
	return nil
}

func sleepFor(args []string) {
	if len(args) > 0 {
		if d, err := time.ParseDuration(args[0]); err == nil {
			fmt.Println(Ready)
			time.Sleep(d)
			return
		}
	}
	fmt.Println(Ready)
	select {}
}

// tree starts the given number of sleeping children (one by default) and
// prints CHILD <pid> for each before saying READY.
func tree(args []string) {
	n := 1
	if len(args) > 0 {
		if v, err := strconv.Atoi(args[0]); err == nil && v > 0 {
			n = v
		}
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Println("fakeproc: executable:", err)
		os.Exit(2)
	}
	for i := 0; i < n; i++ {
		child := exec.Command(exe, "-test.run=^"+HelperTest+"$", "--")
		child.Env = append(os.Environ(), "FAKEPROC_MODE=sleep")
		out, err := child.StdoutPipe()
		if err != nil {
			fmt.Println("fakeproc: child pipe:", err)
			os.Exit(2)
		}
		if err := child.Start(); err != nil {
			fmt.Println("fakeproc: child:", err)
			os.Exit(2)
		}
		// Say the child is up before the parent is, so a test can trust both.
		buf := make([]byte, len(Ready)+1)
		if _, err := out.Read(buf); err != nil {
			fmt.Println("fakeproc: child output:", err)
			os.Exit(2)
		}
		fmt.Printf("CHILD %d\n", child.Process.Pid)
	}
	fmt.Println(Ready)
	select {}
}

// serveHTTP is a tiny OpenAI-shaped endpoint. It lets a test put a real
// listener on a port without starting llama-server: /health answers 200 and
// /v1/models answers only for the right key and model, like serve.Ours expects.
func serveHTTP(args []string) {
	key, model := "", "fake-model"
	if len(args) > 0 {
		key = args[0]
	}
	if len(args) > 1 {
		model = args[1]
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Println("fakeproc: listen:", err)
		os.Exit(2)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"ok"}`)
	})
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		if key != "" && r.Header.Get("Authorization") != "Bearer "+key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprintf(w, `{"data":[{"id":%q}]}`, model)
	})
	fmt.Printf("READY port=%d\n", l.Addr().(*net.TCPAddr).Port)
	http.Serve(l, mux)
}

// Cmd returns an unstarted helper command for this test binary. The test may
// change Dir, Env and Stdin before it starts it; call Start to get a handle.
func Cmd(t testing.TB, mode string, args ...string) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("fakeproc: executable: %v", err)
	}
	argv := append([]string{"-test.run=^" + HelperTest + "$", "--", mode}, args...)
	cmd := exec.Command(exe, argv...)
	cmd.Env = append(os.Environ(), "FAKEPROC_MODE="+mode)
	prepareGroup(cmd)
	return cmd
}

// Proc is a started helper. Stop it (or let the test's cleanup do it) before
// the test ends.
type Proc struct {
	t    testing.TB
	cmd  *exec.Cmd
	out  *output
	done chan struct{}
	err  error
}

// Start starts a helper and stops it when the test ends.
func Start(t testing.TB, mode string, args ...string) *Proc {
	t.Helper()
	return StartCmd(t, Cmd(t, mode, args...))
}

// StartCmd starts a command from Cmd, so a test can set Dir or Env first.
func StartCmd(t testing.TB, cmd *exec.Cmd) *Proc {
	t.Helper()
	p := &Proc{t: t, cmd: cmd, out: &output{}, done: make(chan struct{})}
	cmd.Stdout, cmd.Stderr = p.out, p.out
	if err := cmd.Start(); err != nil {
		t.Fatalf("fakeproc: start %v: %v", cmd.Args, err)
	}
	go func() {
		p.err = cmd.Wait()
		close(p.done)
	}()
	t.Cleanup(p.Stop)
	return p
}

// PID is the helper's process ID.
func (p *Proc) PID() int { return p.cmd.Process.Pid }

// Output is what the helper has printed so far.
func (p *Proc) Output() string { return p.out.String() }

// Exited reports whether the helper has exited, without waiting.
func (p *Proc) Exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

// Wait waits for the helper to exit and returns its error (nil for exit code
// 0, *exec.ExitError otherwise). A helper that is still running when timeout
// ends fails the test with its output.
func (p *Proc) Wait(timeout time.Duration) error {
	p.t.Helper()
	select {
	case <-p.done:
		return p.err
	case <-time.After(timeout):
		p.t.Fatalf("fakeproc: pid %d still running after %v; output:\n%s", p.PID(), timeout, p.Output())
		return nil
	}
}

// ExitCode is the helper's exit code; call Wait first.
func (p *Proc) ExitCode() int {
	if p.err == nil {
		return 0
	}
	if ee, ok := p.err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}

// WaitFor waits until the output contains s and returns the output. It fails
// the test after timeout, or earlier if the helper exits without printing s.
func (p *Proc) WaitFor(s string, timeout time.Duration) string {
	p.t.Helper()
	for end := time.Now().Add(timeout); ; time.Sleep(10 * time.Millisecond) {
		out := p.Output()
		if strings.Contains(out, s) {
			return out
		}
		if p.Exited() {
			// The process is gone, but its output was written before Wait
			// returned, so one more read is enough.
			if out := p.Output(); strings.Contains(out, s) {
				return out
			}
			p.t.Fatalf("fakeproc: pid %d exited without %q; output:\n%s", p.PID(), s, p.Output())
		}
		if time.Now().After(end) {
			p.t.Fatalf("fakeproc: pid %d printed no %q in %v; output:\n%s", p.PID(), s, timeout, out)
		}
	}
}

// Ready waits for the helper's READY line.
func (p *Proc) Ready() { p.t.Helper(); p.WaitFor(Ready, runTimeout) }

// Port waits for a READY port=N line and returns N.
func (p *Proc) Port() int {
	p.t.Helper()
	out := p.WaitFor("READY port=", runTimeout)
	i := strings.LastIndex(out, "READY port=")
	if i < 0 {
		p.t.Fatalf("fakeproc: no port in output:\n%s", out)
	}
	rest := out[i+len("READY port="):]
	if j := strings.IndexByte(rest, '\n'); j >= 0 {
		rest = rest[:j]
	}
	n, err := strconv.Atoi(strings.TrimSpace(rest))
	if err != nil {
		p.t.Fatalf("fakeproc: bad port %q: %v", rest, err)
	}
	return n
}

// Children are the PIDs the helper reported with CHILD lines.
func (p *Proc) Children() []int {
	var pids []int
	for _, line := range strings.Split(p.Output(), "\n") {
		if rest, ok := strings.CutPrefix(line, "CHILD "); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil {
				pids = append(pids, n)
			}
		}
	}
	return pids
}

// Signal sends sig to the helper alone (not its children).
func (p *Proc) Signal(sig os.Signal) error {
	if p.Exited() {
		return nil
	}
	return p.cmd.Process.Signal(sig)
}

// Stop kills the helper's process group and waits for it to go. It is safe to
// call twice and does nothing after the helper exited on its own.
func (p *Proc) Stop() {
	if p.Exited() {
		<-p.done
		return
	}
	killGroup(p.cmd)
	select {
	case <-p.done:
	case <-time.After(10 * time.Second):
		p.t.Errorf("fakeproc: pid %d did not die", p.PID())
	}
}

// Alive reports whether pid is a live process.
func Alive(pid int) bool {
	ok, err := process.PidExists(int32(pid))
	return err == nil && ok
}

// WaitGone waits until pid is gone and reports whether it went. It only looks
// at the PID it is given; it never signals anything.
func WaitGone(t testing.TB, pid int, timeout time.Duration) bool {
	t.Helper()
	for end := time.Now().Add(timeout); ; time.Sleep(10 * time.Millisecond) {
		if !Alive(pid) {
			return true
		}
		if time.Now().After(end) {
			return false
		}
	}
}

// output is a concurrency-safe buffer for a helper's combined output.
type output struct {
	mu sync.Mutex
	b  strings.Builder
}

func (o *output) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.b.Write(p)
}

func (o *output) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.b.String()
}
