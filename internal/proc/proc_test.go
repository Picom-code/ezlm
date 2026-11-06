package proc

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestTimeoutKillsTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("covered on Windows by the e2e run")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	// The shell spawns a grandchild sleep; both must die at the deadline.
	_, err := Output(ctx, exec.Command("sh", "-c", "sleep 30 & wait"))
	if err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("err=%v after %v", err, time.Since(start))
	}
}

func TestOutput(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh on this machine")
	}
	out, err := Output(context.Background(), exec.Command("sh", "-c", "echo hi"))
	if err != nil || string(out) != "hi\n" {
		t.Fatalf("%q %v", out, err)
	}
}
