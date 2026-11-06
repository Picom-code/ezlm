package awake

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// On a Mac the hold must show up as a PreventUserIdleSystemSleep assertion,
// and go away on release.
func TestHoldMac(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	// Other apps may hold their own caffeinate: look for the one EZLM started.
	ours := func() bool {
		kids, _ := exec.Command("pgrep", "-P", strconv.Itoa(os.Getpid()), "caffeinate").Output()
		out, _ := exec.Command("pmset", "-g", "assertions").Output()
		for _, pid := range strings.Fields(string(kids)) {
			for _, l := range strings.Split(string(out), "\n") {
				if strings.Contains(l, "pid "+pid+"(caffeinate)") && strings.Contains(l, "PreventUserIdleSystemSleep") {
					return true
				}
			}
		}
		return false
	}
	wait := func(want bool) bool {
		for i := 0; i < 30; i++ {
			if ours() == want {
				return true
			}
			time.Sleep(100 * time.Millisecond)
		}
		return false
	}
	release := Hold("test")
	if !wait(true) {
		release()
		t.Fatal("no sleep assertion while held")
	}
	release()
	release() // twice is fine
	if !wait(false) {
		t.Fatal("sleep assertion still there after release")
	}
}

func TestHoldTwiceIsSafe(t *testing.T) {
	a, b := Hold("one"), Hold("two")
	b()
	a()
	a()
}

// On Windows the hold must be listed by "powercfg /requests" under its reason,
// and be gone after release.
func TestHoldWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	const why = "ezlm-awake-test"
	listed := func() (bool, error) {
		out, err := exec.Command("powercfg", "/requests").CombinedOutput()
		return strings.Contains(string(out), why), err
	}
	if _, err := listed(); err != nil {
		t.Skip("powercfg /requests needs an administrator")
	}
	release := Hold(why)
	held, _ := listed()
	release()
	if !held {
		t.Fatal("powercfg doesn't list the request while held")
	}
	if still, _ := listed(); still {
		t.Fatal("powercfg still lists the request after release")
	}
}
