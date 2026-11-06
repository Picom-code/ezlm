package serve

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// TestMain doubles as the fake server: the test copies this binary to
// $EZLM_HOME/bin/llama-server and runs the copy with EZLM_FAKE_LLAMA set, so
// the process has the name and the path EZLM looks for, without a model.
func TestMain(m *testing.M) {
	if os.Getenv("EZLM_FAKE_LLAMA") != "" {
		time.Sleep(60 * time.Second)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// startFakeServer starts a fake llama-server from EZLM's own folder and
// returns its PID. It is stopped at the end of the test, by PID.
func startFakeServer(t *testing.T) (int, *exec.Cmd) {
	t.Helper()
	home := homeFor(t)
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(bin, "llama-server")
	if err := os.WriteFile(exe, src, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "EZLM_FAKE_LLAMA=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	return pid, cmd
}

func homeFor(t *testing.T) string {
	t.Helper()
	home := os.Getenv("EZLM_HOME")
	if home == "" {
		t.Fatal("the test needs EZLM_HOME")
	}
	return home
}

func waitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok, _ := process.PidExists(int32(pid)); !ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d is still running", pid)
}

// A missing record is the normal case (launchd or systemd starts the server),
// and a garbled or stale one must not be trusted or left behind.
func TestServerPIDRejectsMissingCorruptAndStaleRecords(t *testing.T) {
	home := t.TempDir()
	t.Setenv("EZLM_HOME", home)
	path := serverPIDPath()

	if pid, ok := serverPID(); ok || pid != 0 {
		t.Fatalf("no file: got %d, %v", pid, ok)
	}

	cases := []string{"", "\n", "not-a-pid\n", "-4\n", "0\n", "999999999999\n", "12 13\n"}
	for _, record := range cases {
		os.WriteFile(path, []byte(record), 0o600)
		if pid, ok := serverPID(); ok || pid != 0 {
			t.Fatalf("record %q: got %d, %v", record, pid, ok)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("record %q was not removed", record)
		}
	}
}

// A live PID that belongs to another program is stale too: the number was
// reused, and stopping it would stop the wrong process.
func TestServerPIDRejectsAReusedPID(t *testing.T) {
	t.Setenv("EZLM_HOME", t.TempDir())
	os.WriteFile(serverPIDPath(), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600)
	if pid, ok := serverPID(); ok || pid != 0 {
		t.Fatalf("got %d, %v", pid, ok)
	}
	if _, err := os.Stat(serverPIDPath()); !os.IsNotExist(err) {
		t.Fatal("the reused record was not removed")
	}
}

// The record is usable while the server runs, and reads as stale once it has
// exited, even though the file is still there.
func TestServerPIDFindsTheFakeServerAndDropsItAfterExit(t *testing.T) {
	t.Setenv("EZLM_HOME", t.TempDir())
	pid, cmd := startFakeServer(t)
	recordServerPID(pid)
	if got, ok := serverPID(); !ok || int(got) != pid {
		t.Fatalf("got %d, %v, want %d", got, ok, pid)
	}

	cmd.Process.Kill()
	cmd.Wait() // reap it: until then it is a zombie, which still "exists"
	waitGone(t, pid)
	if got, ok := serverPID(); ok || got != 0 {
		t.Fatalf("after exit: got %d, %v", got, ok)
	}
	if _, err := os.Stat(serverPIDPath()); !os.IsNotExist(err) {
		t.Fatal("the stale record was not removed")
	}
}

// A profile reached through a link reports the real path; the home folder
// EZLM knows is the link. Both must count as inside EZLM's folder, or Stop
// never finds the server and Start starts a second one.
func TestIsOwnExeThroughASymlinkedHome(t *testing.T) {
	real := t.TempDir()
	home := filepath.Join(t.TempDir(), "ezlm")
	if err := os.Symlink(real, home); err != nil {
		t.Skipf("symlinks: %v", err)
	}
	exe := filepath.Join(real, "bin", "llama-server")
	os.MkdirAll(filepath.Dir(exe), 0o755)
	os.WriteFile(exe, []byte("x"), 0o755)
	if !isOwnExe(exe, home) {
		t.Fatalf("isOwnExe(%q, %q) = false", exe, home)
	}
	outside := filepath.Join(t.TempDir(), "elsewhere", "llama-server")
	if isOwnExe(outside, home) {
		t.Fatal("a program outside the linked folder counted as EZLM's")
	}
}
