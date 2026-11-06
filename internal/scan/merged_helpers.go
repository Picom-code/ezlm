package scan

import (
	"context"
	"strconv"
	"strings"
)

func parseNVIDIAComputeApps(out string) []Proc {
	var procs []Proc
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := splitCSV(line)
		if len(f) < 3 {
			continue
		}
		pid, err := strconv.ParseInt(f[0], 10, 32)
		if err != nil || pid <= 0 {
			continue
		}
		used, _ := strconv.ParseUint(f[2], 10, 64)
		procs = append(procs, Proc{PID: int32(pid), Name: f[1], Mem: used << 20})
	}
	return procs
}

func parseNVIDIAGPUs(out string, ramTotal, ramAvail uint64) []GPU {
	var gpus []GPU
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := splitCSV(line)
		if len(f) < 3 || f[0] == "" {
			continue
		}
		total, err := strconv.ParseUint(f[1], 10, 64)
		var free uint64
		if err != nil || total == 0 {
			if !unifiedNVIDIA(f[0]) || ramTotal == 0 {
				continue // e.g. an "NVIDIA NPU" row reporting [N/A]
			}
			total, free = ramTotal>>20, ramAvail>>20
		} else if free, err = strconv.ParseUint(f[2], 10, 64); err != nil {
			free = total // free memory unknown: assume the card is idle
		}
		gpus = append(gpus, GPU{
			Name: f[0], Vendor: "nvidia",
			MemTotal: total << 20, MemFree: free << 20,
			Bandwidth: nvidiaBandwidth(f[0]),
			Driver:    at(f, 3),
		})
	}
	return gpus
}

func unifiedNVIDIA(name string) bool {
	n := strings.ToLower(name)
	for _, key := range []string{"gb10", "n1x", "spark", "jetson", "tegra"} {
		if strings.Contains(n, key) {
			return true
		}
	}
	return false
}

var rocmSMI = func(ctx context.Context) (string, error) {
	return run(ctx, "rocm-smi", "--showmeminfo", "vram", "--showproductname", "--json")
}
