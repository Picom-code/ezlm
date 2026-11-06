package scan

import (
	"context"
	"encoding/json"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// nvidiaScan reads GPUs and GPU processes from nvidia-smi.
func nvidiaScan(ctx context.Context, h *Hardware) bool {
	out, err := run(ctx, "nvidia-smi", "--query-gpu=name,memory.total,memory.free,driver_version", "--format=csv,noheader,nounits")
	if err != nil {
		return false
	}
	h.GPUs = append(h.GPUs, parseNvidiaGPUs(out)...)
	if len(h.GPUs) == 0 {
		return false
	}
	h.Backend = CUDA
	// The plain nvidia-smi header reports the newest CUDA the driver supports:
	// "CUDA Version: 12.8" on older drivers, "CUDA UMD Version: 13.4" on newer.
	if hdr, err := run(ctx, "nvidia-smi"); err == nil {
		h.CUDAMax = parseCudaVersion(hdr)
	}
	if out, err := run(ctx, "nvidia-smi", "--query-compute-apps=pid,process_name,used_memory", "--format=csv,noheader,nounits"); err == nil {
		for _, p := range parseNvidiaProcs(out) {
			h.GPUProcs = append(h.GPUProcs, procFor(ctx, p.PID, p.Name, p.Mem))
		}
	}
	if runtime.GOOS == "windows" {
		windowsGPUMem(ctx, h)
	}
	return true
}

// parseNvidiaGPUs reads the CSV rows from
// nvidia-smi --query-gpu=name,memory.total,memory.free,driver_version.
// A row with no usable memory (an "NVIDIA NPU" reporting [N/A]) is skipped.
func parseNvidiaGPUs(out string) []GPU {
	var gpus []GPU
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := splitCSV(line)
		if len(f) < 3 {
			continue
		}
		total, err1 := strconv.ParseUint(f[1], 10, 64)
		free, err2 := strconv.ParseUint(f[2], 10, 64)
		if err1 != nil || err2 != nil || total == 0 || total > maxMiB || free > maxMiB {
			continue
		}
		if free > total {
			free = total
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

// nvidiaProcRow is one row of
// nvidia-smi --query-compute-apps=pid,process_name,used_memory.
type nvidiaProcRow struct {
	PID  int32
	Name string
	Mem  uint64 // bytes
}

func parseNvidiaProcs(out string) []nvidiaProcRow {
	var rows []nvidiaProcRow
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := splitCSV(line)
		if len(f) < 3 {
			continue
		}
		pid, _ := strconv.ParseInt(f[0], 10, 32)
		used, _ := strconv.ParseUint(f[2], 10, 64)
		if pid > 0 && used <= maxMiB {
			rows = append(rows, nvidiaProcRow{PID: int32(pid), Name: f[1], Mem: used << 20})
		}
	}
	return rows
}

// maxMiB is the largest MiB count that still fits after the shift to bytes.
const maxMiB = ^uint64(0) >> 20

// parseCudaVersion reads the newest CUDA version the driver supports from the
// nvidia-smi header: "CUDA Version: 12.8" or "CUDA UMD Version: 13.4".
func parseCudaVersion(hdr string) string {
	if m := cudaVersionRe.FindStringSubmatch(hdr); m != nil {
		return m[1]
	}
	return ""
}

// windowsGPUMem fills in per-process VRAM on Windows, where nvidia-smi
// reports "[N/A]" under the WDDM driver model. The "GPU Process Memory"
// performance counters have the numbers; they also reveal graphics apps
// (games, OBS) that nvidia-smi's compute list leaves out.
func windowsGPUMem(ctx context.Context, h *Hardware) map[int32]uint64 {
	usage := windowsGPUUsage(ctx)
	if usage == nil {
		return nil
	}
	var vram uint64
	for _, g := range h.GPUs {
		vram += g.MemTotal
	}
	for pid, b := range usage {
		if b > vram { // some processes (e.g. NVIDIA Overlay) report garbage
			delete(usage, pid)
		}
	}
	seen := map[int32]bool{}
	for i := range h.GPUProcs {
		seen[h.GPUProcs[i].PID] = true
		if b := usage[h.GPUProcs[i].PID]; b > h.GPUProcs[i].Mem {
			h.GPUProcs[i].Mem = b
		}
	}
	for pid, b := range usage {
		if !seen[pid] && b >= 256<<20 {
			h.GPUProcs = append(h.GPUProcs, procFor(ctx, pid, "", b))
		}
	}
	kept := h.GPUProcs[:0]
	for _, p := range h.GPUProcs {
		if p.Mem >= 64<<20 { // ignore apps holding a sliver (overlays, recorders idling)
			kept = append(kept, p)
		}
	}
	h.GPUProcs = kept
	sort.Slice(h.GPUProcs, func(i, j int) bool { return h.GPUProcs[i].Mem > h.GPUProcs[j].Mem })
	return usage
}

// windowsGPUUsage reads dedicated graphics memory per process (Windows).
// The WMI class has the numbers under the same English names on every
// Windows (and answers faster); the counter path is the fallback, but
// Windows translates counter names, so in German or Japanese Windows it
// finds nothing.
func windowsGPUUsage(ctx context.Context) map[int32]uint64 {
	out, err := run(ctx, "powershell", "-NoProfile", "-Command",
		`Get-CimInstance Win32_PerfRawData_GPUPerformanceCounters_GPUProcessMemory -ErrorAction SilentlyContinue | ForEach-Object { "$($_.Name) $($_.DedicatedUsage)" }`)
	if err != nil || !strings.Contains(out, "pid_") {
		out, err = run(ctx, "powershell", "-NoProfile", "-Command",
			`(Get-Counter '\GPU Process Memory(*)\Dedicated Usage' -ErrorAction SilentlyContinue).CounterSamples | ForEach-Object { "$($_.InstanceName) $([int64]$_.CookedValue)" }`)
		if err != nil {
			return nil
		}
	}
	return parseGPUUsage(out)
}

// parseGPUUsage reads "pid_<pid>_luid_..._phys_0 <bytes>" lines, adding up a
// process's use across graphics cards.
func parseGPUUsage(out string) map[int32]uint64 {
	usage := map[int32]uint64{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 || !strings.HasPrefix(f[0], "pid_") {
			continue
		}
		parts := strings.SplitN(strings.TrimPrefix(f[0], "pid_"), "_", 2)
		pid, err1 := strconv.ParseInt(parts[0], 10, 32)
		b, err2 := strconv.ParseUint(strings.TrimSpace(f[1]), 10, 64)
		if err1 == nil && err2 == nil && b < 1<<40 {
			usage[int32(pid)] += b
		}
	}
	return usage
}

var cudaVersionRe = regexp.MustCompile(`CUDA (?:UMD )?Version:\s*([0-9]+(?:\.[0-9]+)*)`)

// amdScan reads AMD GPUs from rocm-smi.
func amdScan(ctx context.Context, h *Hardware) bool {
	out, err := run(ctx, "rocm-smi", "--showmeminfo", "vram", "--showproductname", "--json")
	if err != nil {
		return false
	}
	h.GPUs = append(h.GPUs, parseRocmCards(out)...)
	if len(h.GPUs) == 0 {
		return false
	}
	h.Backend = Vulkan
	return true
}

// parseRocmCards reads rocm-smi --showmeminfo vram --showproductname --json.
// The JSON shape has changed between ROCm releases, but both keys are bytes.
func parseRocmCards(out string) []GPU {
	var cards map[string]map[string]string
	if json.Unmarshal([]byte(out), &cards) != nil {
		return nil
	}
	var gpus []GPU
	for _, c := range cards {
		total, _ := strconv.ParseUint(c["VRAM Total Memory (B)"], 10, 64)
		used, _ := strconv.ParseUint(c["VRAM Total Used Memory (B)"], 10, 64)
		if total == 0 {
			continue
		}
		if used > total {
			used = total
		}
		name := c["Card Series"]
		if name == "" {
			name = c["Card series"]
		}
		gpus = append(gpus, GPU{Name: name, Vendor: "amd", MemTotal: total, MemFree: total - used})
	}
	return gpus
}

func at(f []string, i int) string {
	if i < len(f) {
		return f[i]
	}
	return ""
}

func splitCSV(line string) []string {
	f := strings.Split(line, ",")
	for i := range f {
		f[i] = strings.TrimSpace(f[i])
	}
	return f
}

// displayAdapter is a graphics card as Windows' display drivers list it.
type displayAdapter struct {
	Name, Provider string
	VRAM           uint64
}

// usableAdapters picks the AMD or Intel graphics models can run on with the
// Vulkan build: discrete cards (Radeon RX and Pro, Arc A and B series) with
// 2 GB or more of their own memory, or else built-in graphics with a big
// share of memory set aside (8 GB or more, as on Ryzen AI Max). Everyday
// built-in graphics (a desktop Ryzen's 2 GB, Iris Xe) are about as fast as
// the processor, so they're left out. Like llama.cpp, built-in graphics only
// count when there's no discrete card. NVIDIA cards are read with nvidia-smi.
func usableAdapters(list []displayAdapter) []GPU {
	var discrete, builtIn []GPU
	for _, a := range list {
		v := vendorOf(a.Name + " " + a.Provider)
		switch {
		case v != "amd" && v != "intel":
		case discreteRe.MatchString(a.Name) && a.VRAM >= 2*GB:
			discrete = append(discrete, GPU{Name: a.Name, Vendor: v, MemTotal: a.VRAM, MemFree: a.VRAM, Bandwidth: otherBandwidth(a.Name)})
		case a.VRAM >= 8*GB:
			builtIn = append(builtIn, GPU{Name: a.Name, Vendor: v, MemTotal: a.VRAM, MemFree: a.VRAM}) // speed: the processor's memory
		}
	}
	gpus := discrete
	if len(gpus) == 0 {
		gpus = builtIn
	}
	sort.SliceStable(gpus, func(i, j int) bool { return gpus[i].MemTotal > gpus[j].MemTotal })
	if len(discrete) == 0 && len(gpus) > 1 {
		gpus = gpus[:1]
	}
	return gpus
}

// discreteRe: "Radeon RX 7800 XT", "Radeon RX Vega 64", "Radeon PRO W7900", "Radeon AI
// PRO R9700", "Arc(TM) B580", "Arc(TM) Pro B60", but not "Radeon(TM) Graphics",
// "Radeon RX Vega 10" (built in) or "Arc(TM) 140V".
var discreteRe = regexp.MustCompile(`(?i)\brx\s*\d|\brx\s*vega\s*(56|64)\b|radeon\s+(ai\s+)?pro\b|radeon\s+vii|\barc(\(tm\))?\s+(pro\s+)?[ab]\d{2,3}\b`)

func vendorOf(s string) string {
	l := strings.ToLower(s)
	switch {
	case strings.Contains(l, "nvidia"):
		return "nvidia"
	case strings.Contains(l, "amd") || strings.Contains(l, "advanced micro devices") || strings.Contains(l, "radeon"):
		return "amd"
	case strings.Contains(l, "intel"):
		return "intel"
	}
	return ""
}

// otherBandwidth is a rough GB/s lookup for AMD and Intel cards, used only to
// estimate speed (longer names first, so "7900 xtx" isn't read as "7900 xt").
func otherBandwidth(name string) float64 {
	n := strings.ToLower(name)
	table := []struct {
		key string
		bw  float64
	}{
		{"9070 xt", 640}, {"9070", 640}, {"9060 xt", 322},
		{"7900 xtx", 960}, {"7900 xt", 800}, {"7900 gre", 576}, {"7800 xt", 624}, {"7700 xt", 432}, {"7600 xt", 288}, {"7600", 288},
		{"6950 xt", 576}, {"6900 xt", 512}, {"6800 xt", 512}, {"6800", 512}, {"6750 xt", 432}, {"6700 xt", 384}, {"6650 xt", 280},
		{"6600 xt", 256}, {"6600", 224}, {"6500 xt", 144}, {"w7900", 864}, {"w7800", 576},
		{"r9700", 640}, {"vega 64", 484}, {"vega 56", 410}, {"radeon vii", 1024},
		{"b580", 456}, {"b570", 380}, {"b60", 456}, {"b50", 224}, {"a770", 560}, {"a750", 512}, {"a580", 512}, {"a380", 186}, {"a60", 384}, {"a50", 224}, {"a40", 192},
	}
	for _, t := range table {
		if strings.Contains(n, t.key) {
			return t.bw
		}
	}
	return 300
}

// nvidiaBandwidth is a rough GB/s lookup used only to estimate speed.
func nvidiaBandwidth(name string) float64 {
	n := strings.ToLower(name)
	table := []struct {
		key string
		bw  float64
	}{
		{"5090", 1790}, {"5080", 960}, {"5070 ti", 896}, {"5070", 672}, {"5060 ti", 448}, {"5060", 448},
		{"4090", 1008}, {"4080", 717}, {"4070 ti super", 672}, {"4070 ti", 504}, {"4070 super", 504}, {"4070", 504},
		{"4060 ti", 288}, {"4060", 272},
		{"3090", 936}, {"3080 ti", 912}, {"3080", 760}, {"3070", 448}, {"3060 ti", 448}, {"3060", 360},
		{"a6000", 768}, {"6000 ada", 960}, {"pro 6000", 1790}, {"a100", 1935}, {"h100", 3350}, {"h200", 4800},
		{"l40", 864}, {"a10", 600}, {"t4", 320}, {"gb10", 273}, {"n1x", 273}, {"spark", 273},
	}
	for _, t := range table {
		if strings.Contains(n, t.key) {
			return t.bw
		}
	}
	return 400
}
