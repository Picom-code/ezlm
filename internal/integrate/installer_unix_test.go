//go:build !windows

package integrate

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// An installer's every line reaches the log (keys removed), the UI gets a
// throttled share, and a failure names the telling line and the log.
func TestInstallStreamsAndExplains(t *testing.T) {
	key := "ezlm-InstallerSecretKey99"
	var shown []string
	r := NewRun("install-test", func(m string) { shown = append(shown, m) }, key)
	defer r.Close()
	err := r.Install(context.Background(), exec.Command("sh", "-c", `
echo "Installing thing with `+key+`"
i=0; while [ $i -lt 30 ]; do echo "line $i"; i=$((i+1)); done
printf '####      45%%\r'
echo "error: could not resolve host example.invalid" >&2
echo "npm error A complete log of this run can be found in: /tmp/x.log"
exit 3`), time.Minute)
	if err == nil || !strings.Contains(err.Error(), "error: could not resolve host example.invalid") || !strings.Contains(err.Error(), LogPath("install-test")) {
		t.Fatalf("err = %v", err)
	}
	b, _ := os.ReadFile(LogPath("install-test"))
	if !strings.Contains(string(b), "line 29") || strings.Contains(string(b), key) {
		t.Fatalf("log:\n%s", b)
	}
	if len(shown) == 0 || len(shown) > 5 || strings.Contains(strings.Join(shown, ""), key) {
		t.Fatalf("the UI gets a throttled, redacted share: %v", shown)
	}
}

func TestInstallTimeoutStopsTree(t *testing.T) {
	r := NewRun("install-timeout", nil)
	defer r.Close()
	start := time.Now()
	err := r.Install(context.Background(), exec.Command("sh", "-c", "sleep 30 & wait"), 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "didn't finish within") || time.Since(start) > 10*time.Second {
		t.Fatalf("err = %v after %v", err, time.Since(start))
	}
}

// What a successful installer leaves running is kept and reported by PID
// (this test stops it, since it started it).
func TestInstallReportsLeftovers(t *testing.T) {
	r := NewRun("install-left", nil)
	defer r.Close()
	if err := r.Install(context.Background(), exec.Command("sh", "-c", "sleep 30 >/dev/null 2>&1 & echo done"), time.Minute); err != nil {
		t.Fatal(err)
	}
	left := r.LeftRunning()
	for _, pid := range left {
		syscall.Kill(pid, syscall.SIGKILL)
	}
	if len(left) != 1 {
		t.Fatalf("left = %v", left)
	}
	if !strings.Contains(strings.Join(LogTail("install-left", 10), "\n"), "left 1 process(es) running") {
		t.Fatalf("log: %v", LogTail("install-left", 10))
	}
}

func TestShellCommandEnv(t *testing.T) {
	cmd := ShellCommand("true")
	env := strings.Join(cmd.Env, "\n")
	if !strings.Contains(env, "CI=1") || !strings.Contains(env, "NONINTERACTIVE=1") || cmd.Args[0] != "bash" {
		t.Fatalf("%v", cmd.Args)
	}
}

func TestBinaryArchOfThisProgram(t *testing.T) {
	exe, _ := os.Executable()
	if got := BinaryArch(exe); got != runtime.GOARCH {
		t.Fatalf("BinaryArch = %q, want %q", got, runtime.GOARCH)
	}
	if BinaryArch("/bin/sh") == "" && runtime.GOOS == "linux" {
		t.Fatal("ELF /bin/sh")
	}
}
