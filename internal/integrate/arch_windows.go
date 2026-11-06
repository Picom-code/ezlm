package integrate

import (
	"runtime"

	"golang.org/x/sys/windows"
)

// MachineArch is the machine's own architecture, not this build's: an x64
// EZLM running under emulation on Windows on Arm still says "arm64".
func MachineArch() string {
	var proc, native uint16
	if err := windows.IsWow64Process2(windows.CurrentProcess(), &proc, &native); err == nil {
		switch native {
		case 0xAA64: // IMAGE_FILE_MACHINE_ARM64
			return "arm64"
		case 0x8664: // IMAGE_FILE_MACHINE_AMD64
			return "amd64"
		}
	}
	return runtime.GOARCH
}
