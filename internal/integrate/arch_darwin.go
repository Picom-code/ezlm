package integrate

import (
	"runtime"

	"golang.org/x/sys/unix"
)

// MachineArch is the machine's own architecture, not this build's: an x64
// EZLM running under Rosetta still says "arm64".
func MachineArch() string {
	if v, err := unix.SysctlUint32("sysctl.proc_translated"); err == nil && v == 1 {
		return "arm64"
	}
	return runtime.GOARCH
}
