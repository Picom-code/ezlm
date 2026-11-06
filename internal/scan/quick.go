package scan

import (
	"context"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/mem"
)

// Memory is a fast snapshot of memory pools, for live dashboards.
type Memory struct {
	RAMTotal, RAMAvail uint64
	GPUTotal, GPUFree  uint64
	Unified            bool
}

// QuickMemory reads RAM and GPU memory without the full scan (no process
// listing, no catalog), so it is cheap to call every couple of seconds.
func QuickMemory(ctx context.Context) Memory {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var m Memory
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		m.RAMTotal, m.RAMAvail = vm.Total, vm.Available
	}
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		m.Unified = true
		if out, err := run(ctx, "memory_pressure"); err == nil {
			if pct, ok := parseFreePercentage(out); ok {
				m.RAMAvail = max(m.RAMAvail, uint64(float64(m.RAMTotal)*pct/100))
			}
		}
		m.GPUTotal, m.GPUFree = m.RAMTotal, m.RAMAvail
		return m
	}
	if out, err := run(ctx, "nvidia-smi", "--query-gpu=memory.total,memory.free", "--format=csv,noheader,nounits"); err == nil {
		m.GPUTotal, m.GPUFree = parseNvidiaMem(out)
	}
	if m.GPUTotal == 0 {
		m.GPUTotal, m.GPUFree = otherGPUMemory() // an AMD or Intel card on Windows
	}
	return m
}

// parseFreePercentage reads memory_pressure's "free percentage: 62%".
func parseFreePercentage(out string) (float64, bool) {
	i := strings.Index(out, "free percentage:")
	if i < 0 {
		return 0, false
	}
	f := strings.Fields(out[i+len("free percentage:"):])
	if len(f) == 0 {
		return 0, false
	}
	pct, err := strconv.ParseFloat(strings.TrimSuffix(f[0], "%"), 64)
	if err != nil || pct < 0 || pct > 100 {
		return 0, false
	}
	return pct, true
}

// parseNvidiaMem adds up nvidia-smi --query-gpu=memory.total,memory.free rows
// (MiB, no units) into bytes.
func parseNvidiaMem(out string) (total, free uint64) {
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := splitCSV(line)
		if len(f) < 2 {
			continue
		}
		t, e1 := strconv.ParseUint(f[0], 10, 64)
		fr, e2 := strconv.ParseUint(f[1], 10, 64)
		if e1 == nil && e2 == nil && t <= maxMiB && fr <= maxMiB {
			total += t << 20
			free += fr << 20
		}
	}
	if free > total {
		free = total
	}
	return total, free
}
