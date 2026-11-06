package fakeproc_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/picom-code/ezlm/internal/proc/fakeproc"
)

// TestFakeProcHelper is not a test of its own: Start re-execs this binary with
// -test.run=^TestFakeProcHelper$, and fakeproc.Run does the rest.
func TestFakeProcHelper(t *testing.T) { fakeproc.Run() }

// A helper runs until it is stopped, and Stop really ends it.
func TestStartAndStop(t *testing.T) {
	p := fakeproc.Start(t, "sleep")
	p.Ready()
	pid := p.PID()
	if !fakeproc.Alive(pid) {
		t.Fatalf("pid %d is not alive after READY", pid)
	}
	p.Stop()
	if !fakeproc.WaitGone(t, pid, 5*time.Second) {
		t.Fatalf("pid %d survived Stop", pid)
	}
	if !p.Exited() {
		t.Fatalf("pid %d still reports running", pid)
	}
	p.Stop() // twice is fine
}

// A helper exits with the code the test asked for, which is how launchd and
// "the server died by itself" paths get their input.
func TestExitCode(t *testing.T) {
	p := fakeproc.Start(t, "exit", "7")
	p.Ready()
	if err := p.Wait(5 * time.Second); err == nil {
		t.Fatalf("exit 7 returned nil")
	}
	if got := p.ExitCode(); got != 7 {
		t.Fatalf("exit code %d, want 7", got)
	}
}

// The children a helper starts live in its process group and die with it.
// This is the invariant behind stop-by-PID-tree tests.
func TestChildrenDieWithParent(t *testing.T) {
	p := fakeproc.Start(t, "tree", "2")
	p.Ready()
	kids := p.Children()
	if len(kids) != 2 {
		t.Fatalf("children %v, want 2", kids)
	}
	for _, pid := range kids {
		if !fakeproc.Alive(pid) {
			t.Fatalf("child %d is not alive", pid)
		}
	}
	p.Stop()
	for _, pid := range kids {
		if !fakeproc.WaitGone(t, pid, 5*time.Second) {
			t.Fatalf("child %d survived its parent", pid)
		}
	}
}

// A helper that ignores SIGTERM needs a hard kill; tests that prove escalation
// can rely on this.
func TestStubbornNeedsAKill(t *testing.T) {
	p := fakeproc.Start(t, "stubborn")
	p.Ready()
	_ = p.Signal(syscall.SIGTERM)
	time.Sleep(100 * time.Millisecond)
	if !fakeproc.Alive(p.PID()) {
		t.Fatalf("pid %d died from SIGTERM", p.PID())
	}
	p.Stop()
	if !fakeproc.WaitGone(t, p.PID(), 5*time.Second) {
		t.Fatalf("pid %d survived the kill", p.PID())
	}
}

// The http helper holds a real port and answers like a llama-server, so
// port-conflict and "is it ours" tests can use a separate process.
func TestHTTPHelper(t *testing.T) {
	p := fakeproc.Start(t, "http", "REDACTED-KEY", "qwen3.8-27b")
	port := p.Port()
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := &http.Client{Timeout: 5 * time.Second}

	get := func(path, key string) (int, string) {
		t.Helper()
		req, err := http.NewRequest("GET", base+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}

	if code, body := get("/v1/models", "REDACTED-KEY"); code != 200 || !strings.Contains(body, "qwen3.8-27b") {
		t.Errorf("right key: %d %s", code, body)
	}
	if code, _ := get("/v1/models", "wrong"); code != http.StatusUnauthorized {
		t.Errorf("wrong key: %d, want 401", code)
	}
	if code, _ := get("/health", ""); code != 200 {
		t.Errorf("health: %d, want 200", code)
	}
}

// A test can set Dir or Env on the command before starting it.
func TestCmdCanBeCustomised(t *testing.T) {
	dir := t.TempDir()
	cmd := fakeproc.Cmd(t, "sleep")
	cmd.Dir = dir
	p := fakeproc.StartCmd(t, cmd)
	p.Ready()
	if !fakeproc.Alive(p.PID()) {
		t.Fatalf("pid %d is not alive", p.PID())
	}
	p.Stop()
}

// Alive does not touch anything: it reports on a PID and nothing else.
func TestAliveOfGonePID(t *testing.T) {
	if fakeproc.Alive(1 << 22) {
		t.Fatalf("pid %d reported alive", 1<<22)
	}
}
