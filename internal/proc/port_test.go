// Package proc_test proves the port-conflict path of internal/serve: the
// chosen port is taken, the port is taken between the free check and the
// bind, and the retry finds a port that really is free.
//
// The fake server is this test binary re-executed: it binds like
// llama-server and writes llama-server's own log lines, so no real server
// (and no GPU memory) is involved. Only processes started here are killed,
// by PID.
package proc_test

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/picom-code/ezlm/internal/serve"
)

// TestFakeServerHelper is the fake process, not a test: fakeCmd re-runs this
// binary with -test.run pointed at it.
func TestFakeServerHelper(t *testing.T) {
	if os.Getenv("EZLM_FAKE_SERVER") != "1" {
		t.Skip("helper process only")
	}
	fakeServer()
}

// fakeServer is llama-server's side of the race: bind, or say why not.
func fakeServer() {
	port := os.Getenv("EZLM_FAKE_SERVER_PORT")
	l, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		// The line llama.cpp writes when another program holds the port.
		fmt.Printf("srv  llama_server: couldn't bind HTTP server socket, port %s\n", port)
		os.Exit(1)
	}
	fmt.Printf("srv  llama_server: listening on 127.0.0.1:%s\n", port)
	for {
		c, err := l.Accept()
		if err != nil {
			return
		}
		c.Close()
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func fakeCmd(port int) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestFakeServerHelper$")
	cmd.Env = append(os.Environ(),
		"EZLM_FAKE_SERVER=1",
		fmt.Sprintf("EZLM_FAKE_SERVER_PORT=%d", port),
	)
	return cmd
}

// startFake runs the fake server on port and kills it (by PID) when the test
// ends. It does not wait for the server to answer.
func startFake(t *testing.T, port int) *exec.Cmd {
	t.Helper()
	cmd := fakeCmd(port)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fake server: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		done := make(chan struct{})
		go func() { cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Errorf("fake server on port %d did not exit", port)
		}
	})
	return cmd
}

func waitListening(t *testing.T, port int) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port)); err == nil {
			c.Close()
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("nothing answers on port %d", port)
}

// The race the app has to survive: PortFree said free, another program binds
// the port before llama-server does, llama-server exits with the bind line,
// and the retry moves to a port that really is free and starts there.
func TestPortTakenBetweenCheckAndBind(t *testing.T) {
	t.Setenv("EZLM_HOME", t.TempDir())
	port := freePort(t)
	if !serve.PortFree(port) {
		t.Fatalf("port %d is not free to start with", port)
	}

	// The check passed. A dev tool, a proxy or another model server now
	// takes the port before the server has bound it.
	thief, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Skipf("port %d was taken by another process: %v", port, err)
	}
	defer thief.Close()

	// llama-server starts and cannot bind: this is the failure the app sees.
	out, err := fakeCmd(port).CombinedOutput()
	if err == nil {
		t.Fatalf("fake server bound port %d, which another program holds", port)
	}
	if !serve.BindFailed(string(out)) {
		t.Fatalf("the bind failure was not recognised: %q", out)
	}

	// The retry: move off the taken port and start once more.
	cfg := &serve.Config{Port: port, Key: "REDACTED-KEY", ModelID: "test-model"}
	old, moved, err := serve.MoveToFreePort(cfg)
	if err != nil {
		t.Fatalf("no free port to move to: %v", err)
	}
	if old != port || moved == port {
		t.Fatalf("moved from %d to %d, want off %d", old, moved, port)
	}
	if !serve.PortFree(moved) {
		t.Fatalf("moved to port %d, which is not free", moved)
	}
	saved, err := serve.Load()
	if err != nil {
		t.Fatalf("config was not saved: %v", err)
	}
	if saved.Port != moved {
		t.Fatalf("saved port %d, want %d", saved.Port, moved)
	}

	// The second start binds the new port and answers.
	startFake(t, moved)
	waitListening(t, moved)

	// And the program that took the first port was not adopted or killed:
	// it still accepts connections.
	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("the program that took port %d was disturbed: %v", port, err)
	}
	c.Close()
}

// FreePort must not hand back a port it knows is busy when a run of ports is
// taken. Saving one of those fails the start the move was meant to fix.
func TestFreePortLooksPastABusyRun(t *testing.T) {
	base, listeners := busyRun(t, 100)
	defer func() {
		for _, l := range listeners {
			l.Close()
		}
	}()
	got := serve.FreePort(base)
	if got == base {
		t.Fatalf("FreePort(%d) returned the busy port", base)
	}
	if !serve.PortFree(got) {
		t.Fatalf("FreePort(%d) returned %d, which is busy", base, got)
	}
}

// busyRun holds n consecutive loopback ports from a free starting point.
func busyRun(t *testing.T, n int) (int, []net.Listener) {
	t.Helper()
	for try := 0; try < 20; try++ {
		base := freePort(t)
		var listeners []net.Listener
		for i := 0; i < n; i++ {
			l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", base+i))
			if err != nil {
				break
			}
			listeners = append(listeners, l)
		}
		if len(listeners) == n {
			return base, listeners
		}
		for _, l := range listeners {
			l.Close()
		}
	}
	t.Skip("could not hold 100 consecutive ports on this machine")
	return 0, nil
}
