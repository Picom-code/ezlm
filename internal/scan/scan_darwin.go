package scan

import (
	"context"
	"os"
	"strconv"
	"strings"
)

func platformScan(ctx context.Context, h *Hardware) {
	if h.Arch != "arm64" {
		h.Backend = CPU
		h.Notes = append(h.Notes, "Intel Mac detected: models will run on the CPU.")
		return
	}
	// memory_pressure reports the kernel's own estimate. Its "free percentage"
	// is the share of RAM that is neither wired nor compressed (xnu's
	// AVAILABLE_NON_COMPRESSED_MEMORY: active+inactive+free+speculative), so
	// it counts the active working sets of open apps as free. Take the active
	// pages back out: they are reusable only by compressing or swapping them.
	if out, err := run(ctx, "memory_pressure"); err == nil {
		if avail, ok := pressureAvailable(out, h.RAMTotal); ok {
			h.RAMAvail = max(h.RAMAvail, avail)
		}
	}

	chip, _ := run(ctx, "sysctl", "-n", "machdep.cpu.brand_string")
	chip = strings.TrimSpace(chip)
	if chip != "" {
		h.CPUName = chip
	}
	h.Unified = true
	h.Backend = Metal

	// macOS lets the GPU wire roughly 2/3 of RAM on small machines and 3/4 on
	// larger ones, unless the user raised iogpu.wired_limit_mb.
	var wiredMB uint64
	if s, err := run(ctx, "sysctl", "-n", "iogpu.wired_limit_mb"); err == nil {
		wiredMB, _ = strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	}
	usable := gpuUsable(h.RAMTotal, wiredMB)
	h.GPUUsable = usable

	bw := appleBandwidth(chip)
	h.CPUBandwidth = bw
	h.GPUs = []GPU{{Name: chip + " GPU", Vendor: "apple", MemTotal: h.GPUUsable, MemFree: min(h.GPUUsable, h.RAMAvail), Bandwidth: bw}}
}

// defaultGPUUsable is how much memory macOS lets the GPU wire when the user has
// not set iogpu.wired_limit_mb: 2/3 of RAM up to 36 GB, 3/4 above it.
func defaultGPUUsable(ramTotal uint64) uint64 {
	if ramTotal > 36*GB {
		return uint64(float64(ramTotal) * 0.75)
	}
	return uint64(float64(ramTotal) * 0.67)
}

// parseWiredLimitMB reads the iogpu.wired_limit_mb sysctl value. Zero is the
// "unset, use the default" value and is not a limit.
func parseWiredLimitMB(s string) (uint64, bool) {
	mb, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	if err != nil || mb == 0 {
		return 0, false
	}
	return mb, true
}

// pressureAvailable turns memory_pressure's output into bytes of memory that
// can be reused without paging. The kernel's free percentage counts active
// pages as free, so subtract the active pages the same output lists.
func pressureAvailable(out string, ramTotal uint64) (uint64, bool) {
	i := strings.Index(out, "free percentage:")
	if i < 0 {
		return 0, false
	}
	f := strings.Fields(out[i+len("free percentage:"):])
	if len(f) == 0 {
		return 0, false
	}
	pct, err := strconv.ParseFloat(strings.TrimSuffix(f[0], "%"), 64)
	if err != nil {
		return 0, false
	}
	avail := uint64(float64(ramTotal) * pct / 100)
	if active, ok := pageCount(out, "Pages active:"); ok {
		if used := active * pageSizeBytes(out); used < avail {
			avail -= used
		} else {
			avail = 0
		}
	}
	return avail, true
}

// pageCount reads one of memory_pressure's "Pages ...: N" counters.
func pageCount(out, label string) (uint64, bool) {
	i := strings.Index(out, label)
	if i < 0 {
		return 0, false
	}
	f := strings.Fields(out[i+len(label):])
	if len(f) == 0 {
		return 0, false
	}
	n, err := strconv.ParseUint(strings.TrimSpace(f[0]), 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// pageSizeBytes reads the page size from memory_pressure's header, falling
// back to the running system's page size when the header has no size.
func pageSizeBytes(out string) uint64 {
	const marker = "page size of "
	if i := strings.Index(out, marker); i >= 0 {
		f := strings.Fields(out[i+len(marker):])
		if len(f) > 0 {
			if n, err := strconv.ParseUint(strings.TrimSuffix(f[0], ")"), 10, 64); err == nil && n > 0 {
				return n
			}
		}
	}
	return uint64(os.Getpagesize())
}

// gpuUsable is how much memory the GPU may wire. The sysctl stores whatever
// value was written to it, so a limit above physical RAM is clamped: the GPU
// cannot use more memory than the machine has.
func gpuUsable(ramTotal, wiredMB uint64) uint64 {
	if wiredMB > 0 {
		if wiredMB > ramTotal>>20 {
			return ramTotal
		}
		return wiredMB << 20
	}
	if ramTotal > 36*GB {
		return uint64(float64(ramTotal) * 0.75)
	}
	return uint64(float64(ramTotal) * 0.67)
}

// appleBandwidth is the unified-memory bandwidth (GB/s) of Apple Silicon chips.
func appleBandwidth(chip string) float64 {
	c := strings.ToLower(chip)
	gen := ""
	for _, g := range []string{"m5", "m4", "m3", "m2", "m1"} {
		if strings.Contains(c, g) {
			gen = g
			break
		}
	}
	tier := "base"
	for _, t := range []string{"ultra", "max", "pro"} {
		if strings.Contains(c, t) {
			tier = t
			break
		}
	}
	table := map[string]map[string]float64{
		"m1": {"base": 68, "pro": 200, "max": 400, "ultra": 800},
		"m2": {"base": 100, "pro": 200, "max": 400, "ultra": 800},
		"m3": {"base": 100, "pro": 150, "max": 400, "ultra": 819},
		"m4": {"base": 120, "pro": 273, "max": 546, "ultra": 1092},
		"m5": {"base": 153, "pro": 307, "max": 614, "ultra": 1228},
	}
	if t, ok := table[gen]; ok {
		return t[tier]
	}
	return 100
}
