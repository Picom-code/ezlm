//go:build darwin || linux

package awake

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestFakeHoldChild is not a test: hold re-runs the test binary with this name
// so the hold has a real child process the test can watch and kill.
func TestFakeHoldChild(t *testing.T) {
	if os.Getenv("AWAKE_FAKE_CHILD") != "1" {
		t.Skip("helper process for the hold tests")
	}
	pid := strconv.Itoa(os.Getpid())
	if err := os.WriteFile(os.Getenv("AWAKE_FAKE_PIDFILE"), []byte(pid), 0o644); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(20 * time.Millisecond)
	}
}

// fakeHold makes hold start the test binary instead of caffeinate or
// systemd-inhibit, and restores the real one when the test ends.
func fakeHold(t *testing.T) string {
	t.Helper()
	pidfile := t.TempDir() + "/hold.pid"
	real := holdCmd
	holdCmd = func(string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=^TestFakeHoldChild$")
		cmd.Env = append(os.Environ(), "AWAKE_FAKE_CHILD=1", "AWAKE_FAKE_PIDFILE="+pidfile)
		return cmd
	}
	t.Cleanup(func() { holdCmd = real })
	return pidfile
}

func waitPID(t *testing.T, pidfile string) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(pidfile)
		if err == nil && len(b) > 0 {
			pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
			if err == nil {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the hold child did not write %s", pidfile)
	return 0
}

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func waitGone(t *testing.T, pid int, what string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !alive(pid) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s: process %d is still running", what, pid)
}

// The hold starts exactly one child and releasing it stops that child.
func TestHoldStartsAndStopsOneChild(t *testing.T) {
	pidfile := fakeHold(t)
	release := Hold("test")
	pid := waitPID(t, pidfile)
	defer syscall.Kill(pid, syscall.SIGKILL)
	if !alive(pid) {
		t.Fatalf("child %d is not running right after the hold", pid)
	}
	release()
	waitGone(t, pid, "release")
}

// A second hold is a second child: releasing one must not release the other.
func TestHoldsAreIndependent(t *testing.T) {
	pidfile := fakeHold(t)
	first := Hold("one")
	pid := waitPID(t, pidfile)
	defer syscall.Kill(pid, syscall.SIGKILL)
	// A second hold writes the same file; wait until the pid changes.
	second := Hold("two")
	deadline := time.Now().Add(10 * time.Second)
	other := 0
	for time.Now().Before(deadline) {
		if p := waitPID(t, pidfile); p != pid {
			other = p
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if other == 0 {
		t.Fatal("the second hold did not start its own child")
	}
	defer syscall.Kill(other, syscall.SIGKILL)
	first()
	waitGone(t, pid, "first release")
	if !alive(other) {
		t.Fatalf("releasing the first hold killed the second hold's child %d", other)
	}
	second()
	waitGone(t, other, "second release")
}

// If the child dies on its own (caffeinate exits when EZLM does), a release
// afterwards must return instead of waiting forever.
func TestReleaseAfterChildExit(t *testing.T) {
	pidfile := fakeHold(t)
	release := Hold("test")
	pid := waitPID(t, pidfile)
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	waitGone(t, pid, "killed child")
	done := make(chan struct{})
	go func() { release(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("release hung after the child had already exited")
	}
}

// A machine without the helper still gives a release that does nothing.
func TestHoldWithoutHelper(t *testing.T) {
	real := holdCmd
	holdCmd = func(string) *exec.Cmd { return nil }
	t.Cleanup(func() { holdCmd = real })
	release := Hold("test")
	done := make(chan struct{})
	go func() { release(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("release hung without a helper")
	}
}

// A helper that cannot start must not panic or block the release.
func TestHoldWhenStartFails(t *testing.T) {
	real := holdCmd
	holdCmd = func(string) *exec.Cmd { return exec.Command("/nonexistent/ezlm-hold") }
	t.Cleanup(func() { holdCmd = real })
	release := Hold("test")
	done := make(chan struct{})
	go func() { release(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("release hung after the helper failed to start")
	}
}
