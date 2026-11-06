//go:build !windows

package proc

import (
	"bytes"
	"context"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// An installer that leaves a background process holding its output must
// not hang the install, and the leftover is reported by PID (then stopped
// here, since this test started it).
func TestInstallLeftReportsLeftovers(t *testing.T) {
	old := waitDelay
	waitDelay = 300 * time.Millisecond
	defer func() { waitDelay = old }()
	var out bytes.Buffer
	start := time.Now()
	left, err := InstallLeft(context.Background(), exec.Command("sh", "-c", "sleep 30 & echo started"), &out)
	for _, pid := range left {
		defer syscall.Kill(pid, syscall.SIGKILL)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("install waited for the leftover: %v", time.Since(start))
	}
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("started")) || len(left) != 1 {
		t.Fatalf("out %q left %v", out.String(), left)
	}
}
