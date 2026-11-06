//go:build !windows

package fakeproc

import (
	"os/exec"
	"syscall"
)

// prepareGroup puts the helper in its own process group, so the group can be
// killed as a unit and nothing outside it is ever signalled.
func prepareGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
