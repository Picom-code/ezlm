//go:build windows

package proc

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// prepare must ask for both a hidden window and CREATE_NO_WINDOW (HideWindow
// alone still creates a console, which flashes when EZLM runs as a GUI app),
// and must keep flags the caller set.
func TestPrepareHidesConsole(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "exit 0")
	prepare(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil")
	}
	if !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatalf("SysProcAttr = %+v", cmd.SysProcAttr)
	}
	cmd = exec.Command("cmd.exe", "/c", "exit 0")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	prepare(cmd)
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NEW_PROCESS_GROUP == 0 {
		t.Fatalf("prepare dropped the caller's flags: %+v", cmd.SysProcAttr)
	}
}

// ping is a grandchild of cmd.exe and holds the output pipe. With waitDelay
// raised well above the deadline, only the job kill can end the wait, so a
// tree kill that silently did nothing shows up as a 60-second test.
func TestTimeoutKillsTreeOnWindows(t *testing.T) {
	old := waitDelay
	waitDelay = 60 * time.Second
	defer func() { waitDelay = old }()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Output(ctx, exec.Command("cmd.exe", "/c", "ping -n 60 127.0.0.1"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the context error", err)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("the deadline took %v: the tree was not killed", d)
	}
}

// If no job could be used, the root must still be killed; otherwise the
// timeout path waits for a child nobody owns.
func TestKillWithoutJob(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "ping -n 60 127.0.0.1")
	prepare(cmd)
	if err := cmd.Start(); err != nil {
		t.Skipf("could not start cmd.exe: %v", err)
	}
	tr := track(cmd)
	if tr.proc == 0 {
		t.Fatal("no process handle: the fallback kill cannot work")
	}
	job := tr.job
	tr.job = 0 // as if CreateJobObject or AssignProcessToJobObject failed
	defer func() {
		if job != 0 {
			windows.CloseHandle(job)
		}
	}()
	start := time.Now()
	tr.kill()
	tr.close()
	if err := cmd.Wait(); err != nil && time.Since(start) > 5*time.Second {
		t.Fatalf("kill took %v (%v)", time.Since(start), err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("kill took %v", d)
	}
}

// pids() decodes the job's process list; cmd.exe plus ping is two entries.
func TestPidsSeesGrandchild(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "ping -n 60 127.0.0.1")
	prepare(cmd)
	if err := cmd.Start(); err != nil {
		t.Skipf("could not start cmd.exe: %v", err)
	}
	tr := track(cmd)
	defer func() {
		tr.kill()
		tr.close()
		cmd.Wait()
	}()
	if tr.job == 0 {
		t.Skip("no job object on this machine")
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		ids := tr.pids()
		if len(ids) >= 2 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pids() = %v, want cmd.exe and ping", ids)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// An installer that leaves a background process must not hang the install,
// and the leftover is reported by PID.
func TestInstallLeftReportsLeftovers_procwindows(t *testing.T) {
	old := waitDelay
	waitDelay = 300 * time.Millisecond
	defer func() { waitDelay = old }()

	var out bytes.Buffer
	start := time.Now()
	left, err := InstallLeft(context.Background(), exec.Command("cmd.exe", "/c", "start /b ping -n 60 127.0.0.1 & echo started"), &out)
	for _, pid := range left {
		if h, oerr := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid)); oerr == nil {
			windows.TerminateProcess(h, 1)
			windows.CloseHandle(h)
		}
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("install waited for the leftover: %v", time.Since(start))
	}
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("started")) {
		t.Fatalf("out %q", out.String())
	}
	if len(left) != 1 {
		t.Fatalf("left = %v, want the ping that start left behind", left)
	}
}
