package awake

import (
	"os"
	"os/exec"
	"strconv"
)

// holdCmd builds the child that keeps the machine awake; nil means no helper.
// caffeinate -w ends by itself when EZLM does, so a crash never leaves the Mac
// unable to sleep. Tests replace it to start a fake child.
var holdCmd = func(string) *exec.Cmd {
	return exec.Command("/usr/bin/caffeinate", "-i", "-w", strconv.Itoa(os.Getpid()))
}
