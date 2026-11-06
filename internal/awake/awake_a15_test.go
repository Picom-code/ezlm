package awake

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// Hold must always hand back a release that can be called, even where the
// computer can't be kept awake.
func TestHoldReturnsWorkingRelease(t *testing.T) {
	release := Hold("ezlm a15 test")
	if release == nil {
		t.Fatal("Hold returned a nil release")
	}
	release()
}

// Release is documented as safe to call more than once; concurrent callers
// (a deferred release racing a shutdown path) must not panic or block.
func TestReleaseIsIdempotentAndConcurrent(t *testing.T) {
	release := Hold("ezlm a15 concurrent")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release()
		}()
	}
	wg.Wait()
	release()
}

// A release that never returns would wedge the shutdown path that defers it.
func TestReleaseReturnsPromptly(t *testing.T) {
	release := Hold("ezlm a15 prompt")
	done := make(chan struct{})
	go func() {
		release()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("release did not return within 10s")
	}
}

// Holds are taken and released repeatedly while EZLM works.
func TestHoldRepeat(t *testing.T) {
	for i := 0; i < 3; i++ {
		Hold("ezlm a15 repeat")()
	}
}

// On Linux the hold runs systemd-inhibit with the reason, and is a no-op when
// the tool is not installed.
func TestHoldLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	script := filepath.Join(dir, "systemd-inhibit")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsPath + "\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	release := Hold("ezlm a15 reason")
	var got []byte
	for i := 0; i < 100 && len(got) == 0; i++ {
		got, _ = os.ReadFile(argsPath)
		time.Sleep(20 * time.Millisecond)
	}
	release()
	for _, want := range []string{"--what=idle", "--who=EZLM", "--why=ezlm a15 reason", "--mode=block"} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("inhibitor args %q missing %q", got, want)
		}
	}
	t.Setenv("PATH", "")
	Hold("no inhibitor")()
}
