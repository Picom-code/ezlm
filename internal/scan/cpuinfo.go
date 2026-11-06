package scan

import (
	"context"
	"os"
	"strconv"
	"strings"
)

// cpuInfo is the processor name and physical core count as the kernel reports
// them, for machines where gopsutil cannot read them: a container with a
// cut-down /proc (no "physical id" or "cpu cores") or one where /sys is not
// mounted.
type cpuInfo struct {
	Name  string
	Cores int
}

// linuxCPUInfo fills CPU fields that gopsutil left empty on Linux, first from
// /proc/cpuinfo and then from lscpu. procPath is a parameter so tests can
// point it at a fixture.
func linuxCPUInfo(ctx context.Context, h *Hardware, procPath string) {
	proc, _ := os.ReadFile(procPath)
	out, _ := run(ctx, "lscpu")
	fillCPUInfo(h, string(proc), out)
}

// fillCPUInfo fills only the empty fields, so a reading from gopsutil wins.
func fillCPUInfo(h *Hardware, procText, lscpuText string) {
	ci := parseProcCPUInfo(procText)
	li := parseLscpu(lscpuText)
	if ci.Name == "" {
		ci.Name = li.Name
	}
	if ci.Cores == 0 {
		ci.Cores = li.Cores
	}
	if h.CPUName == "" {
		h.CPUName = ci.Name
	}
	if h.PhysCores == 0 {
		h.PhysCores = ci.Cores
	}
	if h.Cores > 0 && h.PhysCores > h.Cores {
		h.PhysCores = h.Cores // a cut-down /proc can over-report
	}
}

// parseProcCPUInfo reads the processor name and physical core count from
// /proc/cpuinfo. x86 lists a block per logical processor, so cores are
// counted from the (physical id, core id) pairs; arm64 lists a block per core
// and often has no model name, so the CPU part is mapped for the Neoverse
// cores cloud machines use.
func parseProcCPUInfo(text string) cpuInfo {
	var ci cpuInfo
	var hardware, processor, part string
	coresPerSocket := 0
	physIDs := map[string]bool{}
	pairs := map[string]bool{}
	processors := 0
	paired := true // every processor block names its physical and core ids
	for _, b := range procBlocks(text) {
		if _, ok := b["processor"]; !ok {
			continue
		}
		processors++
		if ci.Name == "" {
			ci.Name = b["model name"]
		}
		if hardware == "" {
			hardware = b["Hardware"]
		}
		if processor == "" {
			processor = b["Processor"]
		}
		if part == "" {
			part = b["CPU part"]
		}
		if n, err := strconv.Atoi(b["cpu cores"]); err == nil && n > coresPerSocket {
			coresPerSocket = n
		}
		p, hasP := b["physical id"]
		c, hasC := b["core id"]
		if hasP {
			physIDs[p] = true
		}
		if !hasP || !hasC {
			paired = false
			continue
		}
		pairs[p+"\x00"+c] = true
	}
	switch {
	case paired && len(pairs) > 0:
		ci.Cores = len(pairs)
	case coresPerSocket > 0:
		ci.Cores = coresPerSocket * max(1, len(physIDs))
	default:
		ci.Cores = processors // no topology in this /proc: logical is the best bound
	}
	if ci.Name == "" {
		ci.Name = hardware
	}
	if ci.Name == "" && !strings.HasPrefix(processor, "AArch64") && !strings.HasPrefix(processor, "ARM") {
		ci.Name = processor
	}
	if ci.Name == "" {
		ci.Name = armCPUParts[strings.ToLower(strings.TrimSpace(part))]
	}
	return ci
}

// parseLscpu reads the model name and core count from lscpu. lscpu reads
// /sys, so it still answers when /proc/cpuinfo is not mounted.
func parseLscpu(text string) cpuInfo {
	var ci cpuInfo
	coresPerSocket, sockets := 0, 0
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "model name", "bios model name":
			if ci.Name == "" {
				ci.Name = v
			}
		case "core(s) per socket", "core(s) per cluster":
			coresPerSocket, _ = strconv.Atoi(v)
		case "socket(s)", "cluster(s)":
			sockets, _ = strconv.Atoi(v)
		}
	}
	if coresPerSocket > 0 {
		ci.Cores = coresPerSocket * max(1, sockets)
	}
	return ci
}

// procBlocks splits /proc/cpuinfo into its blank-line separated records.
func procBlocks(text string) []map[string]string {
	var blocks []map[string]string
	cur := map[string]string{}
	flush := func() {
		if len(cur) > 0 {
			blocks = append(blocks, cur)
			cur = map[string]string{}
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		cur[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	flush()
	return blocks
}

// armCPUParts names the cores whose "CPU part" numbers appear in arm64
// /proc/cpuinfo on cloud machines (Linux arch/arm64/include/asm/cputype.h).
var armCPUParts = map[string]string{
	"0xd0c": "Neoverse-N1",
	"0xd40": "Neoverse-V1",
	"0xd49": "Neoverse-N2",
	"0xd4f": "Neoverse-V2",
}
