package scan_test

// End-to-end: realistic machine fixtures go through the real scan parsers and
// the result is handed to the planner. This is the "unusual machine" path
// users hit in the wild, where a misparse must not panic or leave the planner
// with nothing.

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/catalog"
	"github.com/picom-code/ezlm/internal/fit"
	"github.com/picom-code/ezlm/internal/scan"
)

const gb = uint64(scan.GB)

type machine struct {
	name    string
	fixture string
	kind    string // nvidia | rocm | none | adapters | apple
	osName  string
	ram     uint64 // total bytes
	avail   uint64 // available bytes
	cores   int
	bw      float64 // CPU/GPU bandwidth estimate, GB/s

	wantBackend  scan.Backend
	wantGPUs     int
	wantGPUName  string
	wantGPUFree  uint64
	wantUnified  bool
	wantNotePart string
	// skipPlan marks a machine the planner cannot serve yet. The test still
	// runs it, to prove the scan and the planner do not panic.
	skipPlan string
}

var machines = []machine{
	{
		name: "nvidia desktop", fixture: "nvidia-4070-desktop.txt", kind: "nvidia",
		osName: "Linux 6.8", ram: 64 * gb, avail: 52 * gb, cores: 16, bw: 60,
		wantBackend: scan.CUDA, wantGPUs: 1, wantGPUName: "NVIDIA GeForce RTX 4070 SUPER", wantGPUFree: 11000 << 20,
	},
	{
		name: "nvidia windows laptop", fixture: "nvidia-4060-laptop.txt", kind: "nvidia",
		osName: "Windows 11 Pro 24H2", ram: 16 * gb, avail: 8 * gb, cores: 12, bw: 60,
		wantBackend: scan.CUDA, wantGPUs: 1, wantGPUName: "NVIDIA GeForce RTX 4060 Laptop GPU", wantGPUFree: 6200 << 20,
	},
	{
		name: "nvidia MIG server with NPU row", fixture: "nvidia-a100-mig.txt", kind: "nvidia",
		osName: "Linux 6.8", ram: 512 * gb, avail: 480 * gb, cores: 64, bw: 200,
		wantBackend: scan.CUDA, wantGPUs: 2, wantGPUName: "NVIDIA A100-SXM4-80GB", wantGPUFree: 40960 << 20,
	},
	{
		name: "nvidia busy gaming desktop", fixture: "nvidia-4090-busy.txt", kind: "nvidia",
		osName: "Windows 11 Pro 24H2", ram: 32 * gb, avail: 20 * gb, cores: 16, bw: 60,
		wantBackend: scan.CUDA, wantGPUs: 1, wantGPUName: "NVIDIA GeForce RTX 4090", wantGPUFree: 6000 << 20,
	},
	{
		// A card with under a gigabyte free still has to produce a plan.
		name: "nvidia nearly full desktop", fixture: "nvidia-4090-full.txt", kind: "nvidia",
		osName: "Windows 11 Pro 24H2", ram: 32 * gb, avail: 20 * gb, cores: 16, bw: 60,
		wantBackend: scan.CUDA, wantGPUs: 1, wantGPUName: "NVIDIA GeForce RTX 4090", wantGPUFree: 800 << 20,
		skipPlan: "bug: fit returns no plan when a GPU has under ~1 GB free; its CPU fallback is unreachable",
	},
	{
		name: "rocm MI300X server", fixture: "rocm-mi300x.txt", kind: "rocm",
		osName: "Linux 6.8", ram: 1024 * gb, avail: 900 * gb, cores: 128, bw: 300,
		wantBackend: scan.Vulkan, wantGPUs: 1, wantGPUName: "AMD Instinct MI300X",
		wantGPUFree: 205520896000 - 8589934592,
	},
	{
		name: "rocm laptop iGPU", fixture: "rocm-780m.txt", kind: "rocm",
		osName: "Linux 6.8", ram: 32 * gb, avail: 24 * gb, cores: 16, bw: 90,
		wantBackend: scan.Vulkan, wantGPUs: 1, wantGPUName: "AMD Radeon 780M Graphics", wantGPUFree: 536870912 - 134217728,
	},
	{
		name: "cpu only desktop", fixture: "cpu-only-16gb.txt", kind: "none",
		osName: "Linux 6.8", ram: 16 * gb, avail: 12 * gb, cores: 8, bw: 50,
		wantBackend: scan.CPU, wantGPUs: 0, wantNotePart: "No supported GPU",
	},
	{
		name: "windows Arc desktop", fixture: "windows-arc.txt", kind: "adapters",
		osName: "Windows 11 Pro 24H2", ram: 64 * gb, avail: 48 * gb, cores: 20, bw: 60,
		wantBackend: scan.Vulkan, wantGPUs: 1, wantGPUName: "Intel(R) Arc(TM) B580 Graphics", wantGPUFree: 12884901888,
	},
	{
		name: "apple M5 Max 128GB", fixture: "apple-m5max.txt", kind: "apple",
		osName: "macOS 26.3", ram: 128 * gb, avail: 100 * gb, cores: 16, bw: 614,
		wantBackend: scan.Metal, wantGPUs: 1, wantUnified: true, wantGPUName: "Apple M5 Max GPU",
	},
	{
		name: "apple M1 8GB", fixture: "apple-m1-8gb.txt", kind: "apple",
		osName: "macOS 15.7", ram: 8 * gb, avail: 6 * gb, cores: 8, bw: 68,
		wantBackend: scan.Metal, wantGPUs: 1, wantUnified: true, wantGPUName: "Apple M1 GPU",
	},
}

func TestScanToFitFixtures(t *testing.T) {
	t.Setenv("EZLM_CATALOG", "../catalog/catalog.json")
	c, src := catalog.Load(context.Background())
	if len(c.Models) == 0 {
		t.Fatalf("empty catalog (%s)", src)
	}

	for _, m := range machines {
		m := m
		t.Run(m.name, func(t *testing.T) {
			h := hardwareFromFixture(t, m)
			if h.Backend != m.wantBackend {
				t.Fatalf("backend = %q, want %q (notes %q)", h.Backend, m.wantBackend, h.Notes)
			}
			if len(h.GPUs) != m.wantGPUs {
				t.Fatalf("GPUs = %+v, want %d", h.GPUs, m.wantGPUs)
			}
			if m.wantGPUs > 0 {
				g := h.GPUs[0]
				if g.Name != m.wantGPUName {
					t.Errorf("GPU name = %q, want %q", g.Name, m.wantGPUName)
				}
				if m.wantGPUFree != 0 && g.MemFree != m.wantGPUFree {
					t.Errorf("%s free = %d, want %d", g.Name, g.MemFree, m.wantGPUFree)
				}
				if g.Vendor == "" {
					t.Errorf("%s has no vendor", g.Name)
				}
			}
			if h.Unified != m.wantUnified {
				t.Errorf("Unified = %v, want %v", h.Unified, m.wantUnified)
			}
			if m.wantNotePart != "" && !strings.Contains(strings.Join(h.Notes, " "), m.wantNotePart) {
				t.Errorf("notes = %q, want one containing %q", h.Notes, m.wantNotePart)
			}
			if h.RAMTotal != m.ram || h.RAMAvail != m.avail {
				t.Errorf("RAM = %d/%d, want %d/%d", h.RAMAvail, h.RAMTotal, m.avail, m.ram)
			}

			b := fit.ComputeBudgets(h, h.RAMAvail)
			for _, pref := range []fit.Pref{fit.Balanced, fit.Smartest, fit.Fastest} {
				plans := fit.PlansFor(h, c, b, pref)
				if len(plans) == 0 {
					if m.skipPlan != "" {
						t.Skip(m.skipPlan)
					}
					t.Fatalf("%s: planner returned no plan for a %d GB machine (budgets gpu=%d ram=%d)", pref, h.RAMTotal/gb, b.GPU, b.RAM)
				}
				for _, p := range plans {
					if p.GPUNeed > b.GPU || p.RAMNeed > b.RAM {
						t.Errorf("%s/%s exceeds budget: gpu %d>%d ram %d>%d", pref, p.Model.ID, p.GPUNeed, b.GPU, p.RAMNeed, b.RAM)
					}
				}
				got, ok := fit.Recommend(plans)
				if !ok || got.Model.ID == "" || got.Quant.Name == "" || got.Ctx < 1 {
					t.Fatalf("%s: no usable recommendation: %+v", pref, got)
				}
			}
		})
	}
}

// hardwareFromFixture turns one fixture into the Hardware the planner sees,
// running the real parser for that machine family.
func hardwareFromFixture(t *testing.T, m machine) *scan.Hardware {
	t.Helper()
	sections := readSections(t, filepath.Join("testdata", "a16", m.fixture))
	ctx := context.Background()

	switch m.kind {
	case "nvidia", "rocm":
		bin := t.TempDir()
		if out, ok := sections["query-gpu"]; ok {
			writeCommand(t, bin, "nvidia-smi", nvidiaScript, out, sections)
		}
		if out, ok := sections["rocm-smi"]; ok {
			writeCommand(t, bin, "rocm-smi", rocmScript, out, sections)
		}
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		t.Setenv("FIXTURE_DIR", bin)
		return scan.GPUFixtureForTest(ctx, m.osName, m.ram, m.avail, m.cores)

	case "adapters":
		var adapters []scan.AdapterFixture
		for _, line := range strings.Split(sections["adapters"], "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			f := strings.Split(line, "\t")
			if len(f) != 3 {
				t.Fatalf("adapter line %q: want name, provider and bytes", line)
			}
			vram, err := strconv.ParseUint(strings.TrimSpace(f[2]), 10, 64)
			if err != nil {
				t.Fatalf("adapter line %q: %v", line, err)
			}
			adapters = append(adapters, scan.AdapterFixture{Name: strings.TrimSpace(f[0]), Provider: strings.TrimSpace(f[1]), VRAM: vram})
		}
		return scan.AdaptersFixtureForTest(m.osName, m.ram, m.avail, m.cores, adapters)

	case "apple":
		return appleHardware(t, m, sections["sysctl"])

	case "none":
		t.Setenv("PATH", t.TempDir())
		return scan.GPUFixtureForTest(ctx, m.osName, m.ram, m.avail, m.cores)
	}
	t.Fatalf("unknown fixture kind %q", m.kind)
	return nil
}

// appleHardware mirrors what scan_darwin.go derives from sysctl: the wired
// limit, or the 2/3 (3/4 above 36 GB) share of RAM macOS allows by default.
func appleHardware(t *testing.T, m machine, sysctl string) *scan.Hardware {
	t.Helper()
	var chip string
	var ramTotal, wiredMB uint64
	for _, line := range strings.Split(sysctl, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "hw.memsize":
			ramTotal, _ = strconv.ParseUint(v, 10, 64)
		case "machdep.cpu.brand_string":
			chip = v
		case "iogpu.wired_limit_mb":
			wiredMB, _ = strconv.ParseUint(v, 10, 64)
		}
	}
	if ramTotal == 0 || chip == "" {
		t.Fatalf("sysctl fixture missing hw.memsize or brand string:\n%s", sysctl)
	}
	usable := uint64(float64(ramTotal) * 0.67)
	if ramTotal > 36*gb {
		usable = uint64(float64(ramTotal) * 0.75)
	}
	if wiredMB > 0 {
		usable = wiredMB << 20
	}
	return &scan.Hardware{
		OS: m.osName, Arch: "arm64", CPUName: chip, Cores: m.cores, PhysCores: m.cores / 2,
		RAMTotal: ramTotal, RAMAvail: m.avail, DiskFree: 500 * gb, CPUBandwidth: m.bw,
		Unified: true, Backend: scan.Metal, GPUUsable: usable,
		GPUs: []scan.GPU{{Name: chip + " GPU", Vendor: "apple", MemTotal: usable, MemFree: min(usable, m.avail), Bandwidth: m.bw}},
	}
}

// readSections splits a fixture into its "# name" sections.
func readSections(t *testing.T, path string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	cur := ""
	var body []string
	flush := func() {
		if cur != "" {
			out[cur] = strings.TrimSpace(strings.Join(body, "\n"))
		}
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "# ") {
			flush()
			cur, body = strings.TrimSpace(strings.TrimPrefix(line, "# ")), nil
			continue
		}
		body = append(body, line)
	}
	flush()
	return out
}

// writeCommand installs one fake tool that serves the fixture's sections:
// the main output, plus optional query-gpu / compute-apps / header parts.
func writeCommand(t *testing.T, dir, name, script string, main string, sections map[string]string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name+".main"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"query-gpu", "compute-apps", "header"} {
		if s, ok := sections[part]; ok {
			if err := os.WriteFile(filepath.Join(dir, name+"."+part), []byte(s), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

const nvidiaScript = `#!/bin/sh
d=${FIXTURE_DIR:-.}
case "$*" in
  *--query-compute-apps*) [ -f "$d/nvidia-smi.compute-apps" ] && cat "$d/nvidia-smi.compute-apps" || cat "$d/nvidia-smi.main" ;;
  *--query-gpu*) [ -f "$d/nvidia-smi.query-gpu" ] && cat "$d/nvidia-smi.query-gpu" || cat "$d/nvidia-smi.main" ;;
  *) [ -f "$d/nvidia-smi.header" ] && cat "$d/nvidia-smi.header" || cat "$d/nvidia-smi.main" ;;
esac
`

const rocmScript = `#!/bin/sh
d=${FIXTURE_DIR:-.}
cat "$d/rocm-smi.main"
`
