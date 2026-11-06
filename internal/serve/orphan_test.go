package serve

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// A crashed engine leaves its llama-server running: on the next start EZLM
// must still see that process as its own. One it cannot see is started a
// second time, and then two servers hold the same model in memory. These
// tests use a fake llama-server that outlives the test's "engine".

// fakeServer builds the helper named llama-server inside dir and returns it.
func fakeServer(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "llama-server")
	out, err := exec.Command("go", "build", "-o", exe, "./testdata/fakeserver").CombinedOutput()
	if err != nil {
		t.Fatalf("building the fake server: %v\n%s", err, out)
	}
	return exe
}

// orphan starts exe the way a crash leaves it: running, with nobody holding
// a handle it would notice. The test still stops it, by PID, when it ends.
func orphan(t *testing.T, exe string) int {
	t.Helper()
	cmd := exec.Command(exe)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	time.Sleep(100 * time.Millisecond) // let the kernel publish it
	return cmd.Process.Pid
}

func hasPID(pids []int32, pid int) bool {
	for _, p := range pids {
		if int(p) == pid {
			return true
		}
	}
	return false
}

// The engine crashed; the server it started is still running. On restart it
// must be found, or EZLM starts another one beside it.
func TestOwnPIDsFindsServerLeftByACrash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("EZLM_HOME", home)
	exe := fakeServer(t, filepath.Join(home, "bin"))
	pid := orphan(t, exe)

	if got := OwnPIDs(context.Background()); !hasPID(got, pid) {
		t.Fatalf("a server running from %s (pid %d) is EZLM's own, but OwnPIDs = %v; a restart would start a second one", exe, pid, got)
	}
}

// The same, when EZLM's home is reached through a link. The kernel reports
// the real path of a process, so the prefix test must resolve the link:
// ~/.ezlm moved to another drive, or a test home under /tmp on macOS.
func TestOwnPIDsFindsServerThroughASymlinkedHome(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	exe := fakeServer(t, filepath.Join(real, "bin"))
	home := filepath.Join(base, "home")
	if err := os.Symlink(real, home); err != nil {
		t.Skipf("symlinks not available: %v", err)
	}
	t.Setenv("EZLM_HOME", home)
	pid := orphan(t, exe)

	if got := OwnPIDs(context.Background()); !hasPID(got, pid) {
		t.Fatalf("a server running from the linked home (pid %d) is EZLM's own, but OwnPIDs = %v", pid, got)
	}
}

// Finding more processes is only safe if it is still EZLM's own: another
// app's llama-server, and a differently named program in EZLM's folder,
// must never be claimed.
func TestOwnPIDsClaimsOnlyEZLMsLlamaServer(t *testing.T) {
	home := t.TempDir()
	other := t.TempDir()
	t.Setenv("EZLM_HOME", home)

	elsewhere := orphan(t, fakeServer(t, filepath.Join(other, "bin")))

	own := fakeServer(t, filepath.Join(home, "bin"))
	notServer := filepath.Join(home, "bin", "python3")
	if err := os.Rename(own, notServer); err != nil {
		t.Fatal(err)
	}
	foreign := orphan(t, notServer)

	got := OwnPIDs(context.Background())
	if hasPID(got, elsewhere) {
		t.Errorf("pid %d runs from %s, outside EZLM's home, and must not be claimed: %v", elsewhere, other, got)
	}
	if hasPID(got, foreign) {
		t.Errorf("pid %d is not a llama-server and must not be claimed: %v", foreign, got)
	}
}
