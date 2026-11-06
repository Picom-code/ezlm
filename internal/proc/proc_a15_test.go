package proc

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// withWaitDelay shortens the grace period for output held open by a leftover
// process so the tests don't sit out the real five seconds.
func withWaitDelay(t *testing.T, d time.Duration) {
	t.Helper()
	old := waitDelay
	waitDelay = d
	t.Cleanup(func() { waitDelay = old })
}

func unixShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh and POSIX process groups")
	}
}

// alive reports whether pid is still a live process.
func alive(pid int) bool {
	p, err := os.FindProcess(pid)
	return err == nil && p.Signal(syscall.Signal(0)) == nil
}

// stop kills a process the test started, by PID.
func stop(pid int) {
	if p, err := os.FindProcess(pid); err == nil {
		p.Kill()
	}
}

// leftoverPid reads the PID a fake installer echoed.
func leftoverPid(t *testing.T, out []byte) int {
	t.Helper()
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("no PID in %q: %v", out, err)
	}
	return pid
}

// Output returns both streams and passes the exit status through.
func TestOutputCombinesStreamsAndExitError(t *testing.T) {
	unixShell(t)
	out, err := Output(context.Background(), exec.Command("sh", "-c", "echo out; echo err >&2; exit 3"))
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 3 {
		t.Fatalf("err = %v, want exit status 3", err)
	}
	if got := string(out); !strings.Contains(got, "out\n") || !strings.Contains(got, "err\n") {
		t.Fatalf("output %q is missing stdout or stderr", got)
	}
}

// A command that can't be started is an error, not an empty success.
func TestOutputStartError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	out, err := Output(context.Background(), exec.Command(missing))
	if err == nil || len(out) != 0 {
		t.Fatalf("out %q err %v", out, err)
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		t.Fatalf("start failure reported as an exit error: %v", err)
	}
}

// InstallTo hands the combined output to the writer.
func TestInstallToStreamsOutput(t *testing.T) {
	unixShell(t)
	var buf bytes.Buffer
	if err := InstallTo(context.Background(), exec.Command("sh", "-c", "echo hello"), &buf); err != nil {
		t.Fatalf("err = %v", err)
	}
	if buf.String() != "hello\n" {
		t.Fatalf("output %q", buf.String())
	}
}

func TestInstallToStartError(t *testing.T) {
	err := InstallTo(context.Background(), exec.Command(filepath.Join(t.TempDir(), "missing")), &bytes.Buffer{})
	if err == nil {
		t.Fatal("want an error for a command that cannot start")
	}
}

// A finished installer with nothing left behind reports no PIDs.
func TestInstallLeftNoLeftovers(t *testing.T) {
	unixShell(t)
	var buf bytes.Buffer
	left, err := InstallLeft(context.Background(), exec.Command("sh", "-c", "echo done"), &buf)
	if err != nil || len(left) != 0 || buf.String() != "done\n" {
		t.Fatalf("left %v out %q err %v", left, buf.String(), err)
	}
}

func TestInstallLeftStartError(t *testing.T) {
	left, err := InstallLeft(context.Background(), exec.Command(filepath.Join(t.TempDir(), "missing")), &bytes.Buffer{})
	if err == nil || left != nil {
		t.Fatalf("left %v err %v", left, err)
	}
}

// Install must not kill a service the installer deliberately left running.
func TestInstallKeepsLeftover(t *testing.T) {
	unixShell(t)
	withWaitDelay(t, 300*time.Millisecond)
	out, err := Install(context.Background(), exec.Command("sh", "-c", "sleep 30 & echo $!"))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	pid := leftoverPid(t, out)
	defer stop(pid)
	if !alive(pid) {
		t.Fatalf("installer leftover %d was killed", pid)
	}
}

// Output kills whatever the command left behind, by PID.
func TestOutputKillsLeftover(t *testing.T) {
	unixShell(t)
	withWaitDelay(t, 300*time.Millisecond)
	out, err := Output(context.Background(), exec.Command("sh", "-c", "sleep 30 & echo $!"))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	pid := leftoverPid(t, out)
	defer stop(pid)
	deadline := time.Now().Add(5 * time.Second)
	for alive(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("leftover %d still running after Output returned", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
