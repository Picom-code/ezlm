//go:build !windows && !linux

package proc

import "syscall"

func setPdeathsig(a *syscall.SysProcAttr) {}
