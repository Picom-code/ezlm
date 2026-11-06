package serve

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// The registry is how EZLM knows what it started: a process is EZLM's server
// only when it is named llama-server and runs from inside EZLM's home. These
// tests use real fake processes built from source, so nothing touches the
// machine's own servers.

// buildFakeServer writes a tiny program at path that does nothing until it is
// killed. The file name decides the process name the registry sees.
func buildFakeServer(t *testing.T, path string) {
	t.Helper()
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go tool to build a fake server")
	}
	src := filepath.Join(t.TempDir(), "main.go")
	code := "package main\n\nimport \"time\"\n\nfunc main() { for { time.Sleep(time.Hour) } }\n"
	if err := os.WriteFile(src, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(goTool, "build", "-o", path, src)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake server: %v\n%s", err, out)
	}
}

// startFake starts a fake server and stops it by PID when the test ends.
func startFake(t *testing.T, path string) int32 {
	t.Helper()
	cmd := exec.Command(path)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	return int32(cmd.Process.Pid)
}

// contains_registry reports whether pids holds pid.
func contains_registry(pids []int32, pid int32) bool {
	for _, p := range pids {
		if p == pid {
			return true
		}
	}
	return false
}

// waitForOwn polls until OwnPIDs sees pid, so a slow process start doesn't
// make the test flaky.
func waitForOwn(t *testing.T, pid int32) {
	t.Helper()
	for i := 0; i < 60; i++ {
		if contains_registry(OwnPIDs(context.Background()), pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("OwnPIDs never saw %d", pid)
}

// A server started from EZLM's home is found; a llama-server of the same
// name running from anywhere else is not, and neither is a look-alike name.
func TestOwnPIDsFindsOnlyOurServer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake helper is a unix binary")
	}
	home := t.TempDir()
	t.Setenv("EZLM_HOME", home)
	ours := filepath.Join(home, "bin", "llama-server")
	buildFakeServer(t, ours)
	theirs := filepath.Join(t.TempDir(), "llama-server")
	buildFakeServer(t, theirs)

	ourPID := startFake(t, ours)
	theirPID := startFake(t, theirs)
	waitForOwn(t, ourPID)

	pids := OwnPIDs(context.Background())
	if !contains_registry(pids, ourPID) {
		t.Fatalf("OwnPIDs = %v, want it to include our server %d", pids, ourPID)
	}
	if contains_registry(pids, theirPID) {
		t.Fatalf("OwnPIDs = %v, must not include another app's server %d", pids, theirPID)
	}
}

// EZLM's home is often reached through a link (a moved ~/.ezlm, a profile on
// another volume). The kernel reports the resolved path of a process, so the
// registry has to resolve EZLM's home too, or the server it started looks
// like a stranger and a second one gets started on the same port.
func TestOwnPIDsThroughLinkedHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake helper is a unix binary")
	}
	home := t.TempDir()
	link := filepath.Join(t.TempDir(), "ezlm-home")
	if err := os.Symlink(home, link); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	t.Setenv("EZLM_HOME", link)
	server := filepath.Join(home, "bin", "llama-server") // the real path the kernel reports
	buildFakeServer(t, server)

	pid := startFake(t, server)
	waitForOwn(t, pid)
}

// Many status reads run at once (the app polls, the CLI asks); they must all
// agree on the one server EZLM started.
func TestOwnPIDsConcurrentReads(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake helper is a unix binary")
	}
	home := t.TempDir()
	t.Setenv("EZLM_HOME", home)
	server := filepath.Join(home, "bin", "llama-server")
	buildFakeServer(t, server)
	pid := startFake(t, server)
	waitForOwn(t, pid)

	var wg sync.WaitGroup
	errs := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := OwnPIDs(context.Background()); !contains_registry(got, pid) {
				errs <- "concurrent OwnPIDs missed the running server"
			}
		}()
	}
	wg.Wait()
	close(errs)
	for msg := range errs {
		t.Error(msg)
	}
}

// The memory readout is part of the registry: a live server has resident
// memory, a PID that has gone contributes nothing.
func TestOwnRSS(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake helper is a unix binary")
	}
	home := t.TempDir()
	t.Setenv("EZLM_HOME", home)
	server := filepath.Join(home, "bin", "llama-server")
	buildFakeServer(t, server)
	pid := startFake(t, server)
	waitForOwn(t, pid)

	if rss := OwnRSS(context.Background(), []int32{pid}); rss == 0 {
		t.Fatalf("OwnRSS(%d) = 0 for a live process", pid)
	}
	if rss := OwnRSS(context.Background(), []int32{1 << 30}); rss != 0 {
		t.Fatalf("OwnRSS for a missing PID = %d, want 0", rss)
	}
}

// A home path with a trailing separator (EZLM_HOME=/x/.ezlm/) still matches.
func TestIsOwnExePaths(t *testing.T) {
	home := filepath.FromSlash("/Users/p/.ezlm")
	cases := []struct {
		exe, home string
		want      bool
	}{
		{filepath.FromSlash("/Users/p/.ezlm/bin/llama-b11960/llama-server"), home, true},
		{filepath.FromSlash("/Users/p/.ezlm/bin/llama-server"), home + string(filepath.Separator), true},
		{filepath.FromSlash("/Users/p/.ezlm-old/bin/llama-server"), home, false},
		{filepath.FromSlash("/Users/p/.ezlmx/llama-server"), home, false},
		{filepath.FromSlash("/Users/p/llama.cpp/llama-server"), home, false},
	}
	for _, c := range cases {
		if got := isOwnExe(c.exe, c.home); got != c.want {
			t.Errorf("isOwnExe(%q, %q) = %v, want %v", c.exe, c.home, got, c.want)
		}
	}
}

// isOwnExe itself resolves a linked home: the caller may hold either form.
func TestIsOwnExeResolvesLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	real := filepath.Join(t.TempDir(), "real-home")
	link := filepath.Join(t.TempDir(), "linked-home")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	exe := filepath.Join(real, "bin", "llama-server")
	if !isOwnExe(exe, link) {
		t.Errorf("isOwnExe(%q, linked home) = false, want true", exe)
	}
	if !isOwnExe(filepath.Join(link, "bin", "llama-server"), real) {
		t.Errorf("isOwnExe(linked exe, real home) = false, want true")
	}
}
