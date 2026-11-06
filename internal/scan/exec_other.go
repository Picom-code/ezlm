//go:build !windows

package scan

import "os/exec"

func hide(*exec.Cmd) {}
