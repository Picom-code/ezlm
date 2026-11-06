//go:build !windows && !darwin

package integrate

import "runtime"

// MachineArch is the machine's architecture.
func MachineArch() string { return runtime.GOARCH }
