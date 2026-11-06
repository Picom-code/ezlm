package fit

// FuzzFit varies the free memory a machine reports and checks two promises the
// planner makes on every machine:
//
//   - it never panics, and
//   - every plan it returns fits inside the budgets ComputeBudgets produced
//     (the safety margin), with a plan that describes itself consistently.
//
// The seed corpus is the team's real profiles plus the edge values a scan can
// report: 0, one byte, just under a gigabyte, an absurdly large number and
// the largest uint64. A second group of seeds sits one byte below and one byte
// above the free VRAM at which each catalog model's smallest and largest file
// starts to fit, because that is where the boundary arithmetic is decided.

import (
	"context"
	"math"
	"os"
	"testing"

	"github.com/picom-code/ezlm/internal/catalog"
	"github.com/picom-code/ezlm/internal/scan"
)

// fuzzProfile builds a machine of one shape with the given free memory.
type fuzzProfile struct {
	name string
	hw   func(freeRAM, freeVRAM uint64) *scan.Hardware
}

// fuzzGPU keeps MemTotal consistent with the fuzzed free memory.
func fuzzGPU(name, vendor string, total, free uint64, bw float64) scan.GPU {
	if total < free {
		total = free
	}
	return scan.GPU{Name: name, Vendor: vendor, MemTotal: total, MemFree: free, Bandwidth: bw}
}

var fuzzProfiles = []fuzzProfile{
	{"desktop CUDA 12GB", func(ram, vram uint64) *scan.Hardware {
		return &scan.Hardware{Backend: scan.CUDA, RAMTotal: 62 * G, RAMAvail: ram, GPUUsable: vram,
			DiskFree: 800 * G, CPUBandwidth: 60, PhysCores: 8,
			GPUs: []scan.GPU{fuzzGPU("RTX 4070 SUPER", "nvidia", 12*G, vram, 504)}}
	}},
	{"laptop CUDA 4GB", func(ram, vram uint64) *scan.Hardware {
		return &scan.Hardware{Backend: scan.CUDA, RAMTotal: 16 * G, RAMAvail: ram, GPUUsable: vram,
			DiskFree: 200 * G, CPUBandwidth: 34, PhysCores: 4,
			GPUs: []scan.GPU{fuzzGPU("GTX 1650", "nvidia", 4*G, vram, 128)}}
	}},
	{"Apple unified 64GB", func(ram, vram uint64) *scan.Hardware {
		return &scan.Hardware{Backend: scan.Metal, Unified: true, RAMTotal: 64 * G, RAMAvail: ram,
			GPUUsable: vram, DiskFree: 500 * G, CPUBandwidth: 546, PhysCores: 12,
			GPUs: []scan.GPU{fuzzGPU("Apple M4 Max", "apple", 64*G, vram, 546)}}
	}},
	{"Windows Vulkan 16GB", func(ram, vram uint64) *scan.Hardware {
		return &scan.Hardware{OS: "windows", Backend: scan.Vulkan, RAMTotal: 32 * G, RAMAvail: ram,
			GPUUsable: vram, DiskFree: 500 * G, CPUBandwidth: 60, PhysCores: 8,
			GPUs: []scan.GPU{fuzzGPU("AMD Radeon RX 7800 XT", "amd", 16*G, vram, 624)}}
	}},
	{"processor only 16GB", func(ram, _ uint64) *scan.Hardware {
		return &scan.Hardware{Backend: scan.CPU, RAMTotal: 16 * G, RAMAvail: ram,
			DiskFree: 500 * G, CPUBandwidth: 50, PhysCores: 6}
	}},
	{"2x CUDA 24GB", func(ram, vram uint64) *scan.Hardware {
		each := vram / 2
		return &scan.Hardware{Backend: scan.CUDA, RAMTotal: 128 * G, RAMAvail: ram, GPUUsable: vram,
			DiskFree: 2000 * G, CPUBandwidth: 200, PhysCores: 16,
			GPUs: []scan.GPU{fuzzGPU("RTX 3090", "nvidia", 24*G, each, 936), fuzzGPU("RTX 3090", "nvidia", 24*G, each, 936)}}
	}},
}

// fuzzFreeVRAMForNeed is the smallest free VRAM whose budget covers need on
// the desktop profile (CUDA, non-unified: the margin is 4%, at least 0.4 GB).
func fuzzFreeVRAMForNeed(need uint64) uint64 {
	lo := need
	hi := need + need/20 + G
	for lo < hi {
		mid := lo + (hi-lo)/2
		if ComputeBudgets(fuzzProfiles[0].hw(16*G, mid), 16*G).GPU >= need {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo
}

// FuzzFit seeds the corpus and checks every plan against the budgets.
func FuzzFit(f *testing.F) {
	os.Setenv("EZLM_CATALOG", "../catalog/catalog.json")
	c, src := catalog.Load(context.Background())
	if len(c.Models) == 0 {
		f.Fatalf("empty catalog (source %q)", src)
	}

	// The team's real profiles.
	for _, s := range []struct {
		kind      uint8
		ram, vram uint64
	}{
		{0, 42 * G, 11 * G}, // RTX 4070 SUPER + 62GB
		{1, 10 * G, 3 * G},  // GTX 1650 laptop + 16GB
		{2, 50 * G, 43 * G}, // Mac M4 Max 64GB
		{3, 24 * G, 16 * G}, // Windows AMD RX 7800 XT + 32GB
		{4, 12 * G, 0},      // CPU only, 16GB
		{5, 50 * G, 23 * G}, // 2x RTX 3090
	} {
		f.Add(s.kind, int64(s.ram), int64(s.vram))
	}

	// Edge values a scan can report, for every profile shape. Negative int64
	// values are read back as uint64, so -1 is the absurd case (2^64-1).
	const maxU64 = ^uint64(0)
	edges := []uint64{0, 1, G - 1, G, 1 << 40, maxU64}
	for kind := range fuzzProfiles {
		for _, v := range edges {
			f.Add(uint8(kind), int64(16*G), int64(v)) // free VRAM
			f.Add(uint8(kind), int64(v), int64(4*G))  // free RAM
		}
	}

	// One byte either side of the free VRAM where a model's smallest and
	// largest file starts to fit: the boundary arithmetic is decided there.
	for _, m := range c.Models {
		if len(m.Quants) == 0 {
			continue
		}
		smallest, largest := m.Quants[0].Bytes, m.Quants[0].Bytes
		for _, q := range m.Quants {
			smallest = min(smallest, q.Bytes)
			largest = max(largest, q.Bytes)
		}
		for _, need := range []uint64{smallest, largest} {
			at := fuzzFreeVRAMForNeed(need)
			f.Add(uint8(0), int64(16*G), int64(at-1))
			f.Add(uint8(0), int64(16*G), int64(at))
		}
	}

	f.Fuzz(func(t *testing.T, kind uint8, freeRAM, freeVRAM int64) {
		p := fuzzProfiles[int(kind)%len(fuzzProfiles)]
		ram, vram := uint64(freeRAM), uint64(freeVRAM)
		hw := p.hw(ram, vram)
		b := ComputeBudgets(hw, ram)
		if b.GPU > hw.GPUUsable {
			t.Fatalf("%s: GPU budget %d above usable %d", p.name, b.GPU, hw.GPUUsable)
		}
		if b.RAM > ram {
			t.Fatalf("%s: RAM budget %d above free RAM %d", p.name, b.RAM, ram)
		}
		for _, pref := range []Pref{Balanced, Smartest, Fastest} {
			plans := PlansFor(hw, c, b, pref)
			for _, pl := range plans {
				checkFuzzPlan(t, p.name, pref, pl, b)
			}
			if len(plans) > 0 {
				if best, ok := Recommend(plans); !ok || best.Model.ID != plans[0].Model.ID {
					t.Fatalf("%s/%s: Recommend disagrees with the first plan", p.name, pref)
				}
			}
		}
	})
}

// checkFuzzPlan asserts one plan stays inside the budgets and describes itself
// consistently with the model it names.
func checkFuzzPlan(t *testing.T, profile string, pref Pref, p Plan, b Budgets) {
	t.Helper()
	where := profile + "/" + string(pref) + "/" + p.Model.ID + "/" + p.Quant.Name
	if p.GPUNeed > b.GPU {
		t.Fatalf("%s: GPU need %d above budget %d", where, p.GPUNeed, b.GPU)
	}
	if p.RAMNeed > b.RAM {
		t.Fatalf("%s: RAM need %d above budget %d", where, p.RAMNeed, b.RAM)
	}
	if p.Ctx <= 0 {
		t.Fatalf("%s: non-positive context %d", where, p.Ctx)
	}
	if p.Model.CtxMax > 0 && p.Ctx > p.Model.CtxMax {
		t.Fatalf("%s: context %d above the model's %d", where, p.Ctx, p.Model.CtxMax)
	}
	if math.IsNaN(p.TokPerSec) || math.IsInf(p.TokPerSec, 0) || p.TokPerSec < 0 {
		t.Fatalf("%s: bad speed %v", where, p.TokPerSec)
	}
	if math.IsNaN(p.Effective) || math.IsInf(p.Effective, 0) || p.Effective < 0 {
		t.Fatalf("%s: bad effective score %v", where, p.Effective)
	}
	switch p.Mode {
	case GPU, Hybrid:
		if p.NGL != 999 {
			t.Fatalf("%s: mode %s with -ngl %d", where, p.Mode, p.NGL)
		}
	case Split:
		if p.NGL < 0 || p.NGL >= p.Model.Layers {
			t.Fatalf("%s: split with %d of %d layers on the GPU", where, p.NGL, p.Model.Layers)
		}
	case CPU:
		if p.NGL != 0 {
			t.Fatalf("%s: processor plan with -ngl %d", where, p.NGL)
		}
	default:
		t.Fatalf("%s: unknown mode %q", where, p.Mode)
	}
	if p.NCPUMoE < 0 || p.NCPUMoE > p.Model.Layers {
		t.Fatalf("%s: %d of %d layers with experts in RAM", where, p.NCPUMoE, p.Model.Layers)
	}
	// A fully GPU-resident plan must count at least the weights it keeps
	// resident (lazy lookup tables excepted). This is the check that would
	// catch a plan that "fits" because it forgot the weights.
	if p.Mode == GPU && p.Quant.Bytes >= p.Quant.LazyBytes {
		if resident := p.Quant.Bytes - p.Quant.LazyBytes; p.GPUNeed < resident {
			t.Fatalf("%s: GPU need %d below resident weights %d", where, p.GPUNeed, resident)
		}
	}
}
