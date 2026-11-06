//go:build !darwin

package scan

import "context"

func platformScan(ctx context.Context, h *Hardware) {
	if nvidiaScan(ctx, h) {
		return
	}
	if amdScan(ctx, h) {
		return
	}
	if displayScan(ctx, h) { // AMD and Intel cards on Windows (no rocm-smi there)
		return
	}
	h.Backend = CPU
	h.Notes = append(h.Notes, "No supported GPU found: models will run on the CPU, which is slower.")
}
