//go:build windows

package fakeproc

import "os/exec"

// prepareGroup does nothing: Windows has no process groups to set up here.
func prepareGroup(*exec.Cmd) {}

func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
