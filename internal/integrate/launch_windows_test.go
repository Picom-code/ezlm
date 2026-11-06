package integrate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLaunchCmdShim: a .cmd program (an npm shim) is started with call, so
// the launcher carries on after it, and % in its arguments survives call's
// second expansion.
func TestLaunchCmdShim(t *testing.T) {
	ws := launchWeirdHome(t)
	shim := filepath.Join(filepath.Dir(ws), "npm", "opencode.cmd")
	os.MkdirAll(filepath.Dir(shim), 0o755)
	os.WriteFile(shim, []byte("@echo off\r\necho SHIM:[%1] [%2] [%3] [%EZLM_LAUNCH_PROBE%]\r\nexit /b 4\r\n"), 0o644)
	l, err := Launch(LaunchSpec{ID: "shim", Title: "Shim (EZLM)", Bin: shim, Args: []string{"--model", "ezlm/qwen%x", "a b"}, Dir: ws,
		Env: []string{"EZLM_LAUNCH_PROBE=probe"}})
	if err != nil {
		t.Fatal(err)
	}
	lt := launchStartTerm(t, exec.Command(l.Launcher))
	lt.waitFor("Shim stopped", 30*time.Second)
	lt.p.Write([]byte("\r"))
	lt.waitExit(15 * time.Second)
	if !strings.Contains(lt.text(), `SHIM:[--model] ["ezlm/qwen%x"] ["a b"] [probe]`) {
		t.Errorf("the shim got other arguments:\n%s", lt.text())
	}
	if code, _, ok := ReadExit(l.ExitFile); !ok || code != 4 {
		t.Errorf("exit file: %d %v", code, ok)
	}
}

// TestLaunchCmdNegativeCode: a crash code (negative as a signed number)
// counts as a failure too.
func TestLaunchCmdNegativeCode(t *testing.T) {
	ws := launchWeirdHome(t)
	exe, lead := launchTestHelper(t)
	l, err := Launch(LaunchSpec{ID: "helper", Title: "Helper (EZLM)", Bin: exe, Args: append(lead, "x"), Dir: ws,
		Env: []string{"EZLM_LAUNCH_HELPER=-1073741819"}})
	if err != nil {
		t.Fatal(err)
	}
	lt := launchStartTerm(t, exec.Command(l.Launcher))
	lt.waitFor("Helper stopped", 30*time.Second)
	lt.p.Write([]byte("\r"))
	lt.waitExit(15 * time.Second)
	if code, _, ok := ReadExit(l.ExitFile); !ok || code != -1073741819 {
		t.Errorf("exit file: %d %v\n%s", code, ok, lt.text())
	}
}

// TestLaunchFindWT only checks that looking for Windows Terminal never fails
// hard; whether it is installed depends on the machine.
func TestLaunchFindWT(t *testing.T) {
	t.Logf("wt: %q", termFindWT())
}
