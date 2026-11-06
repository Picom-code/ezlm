//go:build !windows && !darwin

package scan

import "context"

// displayScan: AMD cards on Linux are read with rocm-smi (amdScan).
func displayScan(ctx context.Context, h *Hardware) bool { return false }
