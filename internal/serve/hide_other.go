//go:build !windows

package serve

import "os/exec"

func hideChild(*exec.Cmd) {}
