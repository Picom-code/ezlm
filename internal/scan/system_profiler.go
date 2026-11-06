package scan

import (
	"sort"
	"strconv"
	"strings"
)

// profilerGPU is one graphics card as `system_profiler SPDisplaysDataType`
// describes it. Apple Silicon lists a single Apple GPU with no VRAM of its
// own; Intel Macs list the built-in Intel graphics plus any discrete or
// external card.
type profilerGPU struct {
	GPU
	Bus   string
	Metal bool
}

// parseProfilerDisplays reads the text output of
// `system_profiler SPDisplaysDataType`. Each card is a four-space indented
// line ending in a colon; its fields sit one level deeper. A headless Mac
// simply has no "Displays:" section, which changes nothing here.
func parseProfilerDisplays(out string) []profilerGPU {
	var list []profilerGPU
	var cur *profilerGPU
	flush := func() {
		if cur != nil {
			list = append(list, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent == 4 && strings.HasSuffix(trimmed, ":") {
			flush()
			cur = &profilerGPU{GPU: GPU{Name: strings.TrimSuffix(trimmed, ":")}}
			continue
		}
		if cur == nil || indent != 6 {
			continue
		}
		key, val, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch key {
		case "Chipset Model":
			if val != "" {
				cur.Name = val
			}
		case "Vendor":
			cur.Vendor = profilerVendor(val)
		case "Bus":
			cur.Bus = val
		case "VRAM (Total)":
			cur.MemTotal = profilerBytes(val)
		case "VRAM (Dynamic, Max)":
			// Built-in Intel graphics share system memory; only a card with
			// its own VRAM (Total) is worth counting as discrete.
			if cur.MemTotal == 0 {
				cur.MemTotal = profilerBytes(val)
			}
		case "Metal Support":
			cur.Metal = strings.HasPrefix(val, "Metal")
		case "Metal": // macOS 10.14 to 11: "Metal: Supported, feature set macOS GPUFamily2 v1"
			cur.Metal = strings.HasPrefix(val, "Supported")
		}
	}
	flush()
	return list
}

// profilerVendor maps the profiler's vendor line ("AMD (0x1002)") to the
// vendor names the rest of the scan uses.
func profilerVendor(s string) string {
	l := strings.ToLower(s)
	switch {
	case strings.Contains(l, "apple"):
		return "apple"
	case strings.Contains(l, "nvidia"):
		return "nvidia"
	case strings.Contains(l, "amd"), strings.Contains(l, "ati"), strings.Contains(l, "advanced micro devices"):
		return "amd"
	case strings.Contains(l, "intel"):
		return "intel"
	}
	return ""
}

// profilerBytes reads a size as the profiler prints it ("1536 MB", "8 GB").
func profilerBytes(s string) uint64 {
	f := strings.Fields(s)
	if len(f) < 2 {
		return 0
	}
	n, err := strconv.ParseFloat(f[0], 64)
	if err != nil || n <= 0 {
		return 0
	}
	switch strings.ToUpper(f[1]) {
	case "KB":
		return uint64(n * (1 << 10))
	case "MB":
		return uint64(n * (1 << 20))
	case "GB":
		return uint64(n * (1 << 30))
	case "TB":
		return uint64(n * (1 << 40))
	}
	return 0
}

// metalGPUs picks the cards on an Intel Mac that the Metal build can use: a
// discrete or external card with its own memory and Metal support. Built-in
// Intel graphics are left to the processor, as on Windows.
func metalGPUs(list []profilerGPU) []GPU {
	var out []GPU
	for _, p := range list {
		if !p.Metal || p.Vendor == "" || p.Vendor == "apple" {
			continue
		}
		if strings.EqualFold(p.Bus, "Built-In") || p.MemTotal < 2*GB {
			continue
		}
		g := p.GPU
		g.MemFree = g.MemTotal
		if g.Vendor == "nvidia" {
			g.Bandwidth = nvidiaBandwidth(g.Name)
		} else {
			g.Bandwidth = otherBandwidth(g.Name)
		}
		out = append(out, g)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].MemTotal > out[j].MemTotal })
	return out
}

// profilerHardware is what `system_profiler SPHardwareDataType` says about
// the Mac. Intel Macs name the processor in "Processor Name"; Apple Silicon
// uses "Chip".
type profilerHardware struct {
	Model     string
	Chip      string
	RAMTotal  uint64
	Cores     int
	PerfCores int
	EffCores  int
}

// parseProfilerHardware reads the text output of
// `system_profiler SPHardwareDataType`.
func parseProfilerHardware(out string) profilerHardware {
	var hw profilerHardware
	for _, line := range strings.Split(out, "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch key {
		case "Model Name":
			hw.Model = val
		case "Chip":
			hw.Chip = val
		case "Processor Name":
			if hw.Chip == "" {
				hw.Chip = val
			}
		case "Memory":
			hw.RAMTotal = profilerBytes(val)
		case "Total Number of Cores":
			// "11 (5 Performance and 6 Efficiency)" on Apple Silicon, "6" on Intel.
			f := strings.Fields(val)
			if len(f) == 0 {
				continue
			}
			hw.Cores, _ = strconv.Atoi(f[0])
			hw.PerfCores, hw.EffCores = profilerCoreCounts(val)
		}
	}
	return hw
}

// profilerCoreCounts reads the "(5 Performance and 6 Efficiency)" note. The
// wording is lower case before macOS 26, so match it without case.
func profilerCoreCounts(val string) (perf, eff int) {
	open, close := strings.Index(val, "("), strings.Index(val, ")")
	if open < 0 || close < open {
		return 0, 0
	}
	f := strings.Fields(strings.ToLower(val[open+1 : close]))
	for i := 0; i+1 < len(f); i++ {
		n, err := strconv.Atoi(f[i])
		if err != nil {
			continue
		}
		switch {
		case strings.HasPrefix(f[i+1], "performance"):
			perf = n
		case strings.HasPrefix(f[i+1], "efficiency"):
			eff = n
		}
	}
	return perf, eff
}
