package awake

import (
	"os"
	"os/exec"
	"strconv"
)

// holdCmd builds the child that keeps the machine awake; nil means no helper.
// The inhibitor holds while tail waits for EZLM's process, so it ends with EZLM
// even after a crash. Tests replace it to start a fake child.
var holdCmd = func(why string) *exec.Cmd {
	p, err := exec.LookPath("systemd-inhibit")
	if err != nil {
		return nil
	}
	if why == "" {
		why = "EZLM is working"
	}
	return exec.Command(p, "--what=idle", "--who=EZLM", "--why="+why, "--mode=block",
		"tail", "--pid="+strconv.Itoa(os.Getpid()), "-f", "/dev/null")
}
