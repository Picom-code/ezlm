package scan

import (
	"strconv"
	"strings"
)

func sysctlUint(s string) (uint64, bool) {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	return n, err == nil
}

func appleSilicon(arch, procTranslated string) bool {
	if arch == "arm64" {
		return true
	}
	n, ok := sysctlUint(procTranslated)
	return ok && n == 1
}

func appleGPUUsable(ramTotal uint64, wiredLimitMB string) uint64 {
	usable := uint64(float64(ramTotal) * 0.67)
	if ramTotal >= 36*GB {
		usable = uint64(float64(ramTotal) * 0.75)
	}
	if mb, ok := sysctlUint(wiredLimitMB); ok && mb > 0 {
		usable = mb << 20
	}
	return usable
}

func parseMemoryPressure(out string) (float64, bool) {
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
