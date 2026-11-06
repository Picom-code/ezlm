//go:build !windows

package proc

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A fake process that is its own group leader, started by this test and
// stopped only by pid. It is reaped as soon as it exits, so a killed fake is
// gone (not a zombie) by the time alive_stoptree is checked.
func startFake_stoptree(t *testing.T) (pid int, stop func()) {
	t.Helper()
	cmd := exec.Command("sh", "-c", "exec sleep 30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fake: %v", err)
	}
	pid = cmd.Process.Pid
	waited := make(chan struct{})
	go func() { cmd.Wait(); close(waited) }()
	return pid, func() {
		syscall.Kill(-pid, syscall.SIGKILL)
		<-waited
	}
}

func alive_stoptree(pid int) bool { return syscall.Kill(pid, 0) == nil }

func readPID(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatalf("pid %q: %v", b, err)
	}
	return pid
}

// The shell exits at once and leaves a child running in the same process
// group. Stopping the tree must still reach that child after the leader is
// gone.
func TestStopKillsAChildWhoseParentExitedFirst(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	out, err := Output(context.Background(), exec.Command("sh", "-c",
		"sleep 30 >/dev/null 2>&1 & echo $! > "+pidFile+"; exit 0"))
	if err != nil {
		t.Fatalf("output: %v (%s)", err, out)
	}
	child := readPID(t, pidFile)
	deadline := time.Now().Add(3 * time.Second)
	for alive_stoptree(child) {
		if time.Now().After(deadline) {
			syscall.Kill(child, syscall.SIGKILL)
			t.Fatalf("the child %d outlived the tree stop", child)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The walk stops at the process group: a stranger in another group keeps
// running while the command's own child is stopped.
func TestStopStopsAtTheProcessGroupBoundary(t *testing.T) {
	decoy, stopDecoy := startFake_stoptree(t)
	defer stopDecoy()

	pidFile := filepath.Join(t.TempDir(), "child.pid")
	_, err := Output(context.Background(), exec.Command("sh", "-c",
		"sleep 30 >/dev/null 2>&1 & echo $! > "+pidFile+"; exit 0"))
	if err != nil {
		t.Fatalf("output: %v", err)
	}
	child := readPID(t, pidFile)
	for i := 0; i < 100 && alive_stoptree(child); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if alive_stoptree(child) {
		t.Fatalf("the child %d outlived the tree stop", child)
	}
	if !alive_stoptree(decoy) {
		t.Fatalf("stopped %d, which is in another process group", decoy)
	}
}

// The leftovers reported by an installer are its own group only, never a
// stranger that happens to share the machine.
func TestLeftoversStopAtTheProcessGroupBoundary(t *testing.T) {
	decoy, stopDecoy := startFake_stoptree(t)
	defer stopDecoy()

	var out bytes.Buffer
	left, err := InstallLeft(context.Background(), exec.Command("sh", "-c",
		"sleep 30 >/dev/null 2>&1 & echo started"), &out)
	for _, pid := range left {
		defer syscall.Kill(pid, syscall.SIGKILL)
	}
	if err != nil || !bytes.Contains(out.Bytes(), []byte("started")) {
		t.Fatalf("out %q err %v", out.String(), err)
	}
	if len(left) != 1 {
		t.Fatalf("left %v, want one child", left)
	}
	if left[0] == decoy {
		t.Fatalf("listed %d, which is in another process group", decoy)
	}
	if !alive_stoptree(decoy) {
		t.Fatalf("stopped %d, which is in another process group", decoy)
	}
}

// A group id can be reused once the tree is empty. A tree whose recorded
// leader is gone and whose id now names a new process must not be stopped.
func TestStopDoesNotKillAReusedPID(t *testing.T) {
	pid, stop := startFake_stoptree(t)
	defer stop()
	birth, ok := leaderBirth(pid)
	if !ok {
		t.Fatal("no start time for the fake")
	}

	stale := &tree{pid: pid, birth: birth + 1000} // the old leader started at another time
	stale.close()
	stale.kill()
	time.Sleep(200 * time.Millisecond) // long enough for a wrong kill to land
	if !alive_stoptree(pid) {
		t.Fatalf("stopped %d, which reused the group id", pid)
	}
	if got := stale.pids(); len(got) != 0 {
		t.Fatalf("listed %v from a reused id", got)
	}

	// The same id, with the leader's real start time, is ours: it is stopped.
	ours := &tree{pid: pid, birth: birth}
	ours.close()
	stop()
	if alive_stoptree(pid) {
		t.Fatalf("%d is still running", pid)
	}
}

// A tree whose leader start time is unknown (the platform could not read it)
// still stops, so a missing start time never leaves an orphan.
func TestStopWithoutABirthTimeStillKills(t *testing.T) {
	pid, stop := startFake_stoptree(t)
	defer stop()
	unknown := &tree{pid: pid}
	unknown.close()
	stop()
	if alive_stoptree(pid) {
		t.Fatalf("%d is still running", pid)
	}
}
