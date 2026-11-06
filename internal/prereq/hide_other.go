//go:build !windows

package prereq

import "os/exec"

func hide(*exec.Cmd) {}
