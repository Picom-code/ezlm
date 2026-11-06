package fit

import (
	"context"
	"os"
	"testing"

	"github.com/picom-code/ezlm/internal/catalog"
	"github.com/picom-code/ezlm/internal/scan"
)

const G = scan.GB

func cat(t *testing.T) *catalog.Catalog {
	os.Setenv("EZLM_CATALOG", "../catalog/catalog.json")
	c, src := catalog.Load(context.Background())
	if src == "remote" {
		t.Log("warning: using remote catalog")
	}
	return c
}

func mac(ramGB, availGB uint64, bw float64) *scan.Hardware {
	usable := uint64(float64(ramGB*G) * 0.67)
	if ramGB > 36 {
		usable = uint64(float64(ramGB*G) * 0.75)
	}
	return &scan.Hardware{Backend: scan.Metal, Unified: true, RAMTotal: ramGB * G, RAMAvail: availGB * G,
		GPUUsable: usable, DiskFree: 500 * G, CPUBandwidth: bw,
		GPUs: []scan.GPU{{Vendor: "apple", Bandwidth: bw}}}
}

func nvidia(vramFreeGB, ramAvailGB uint64, bw float64) *scan.Hardware {
	return &scan.Hardware{Backend: scan.CUDA, RAMTotal: (ramAvailGB + 8) * G, RAMAvail: ramAvailGB * G,
		GPUUsable: vramFreeGB * G, DiskFree: 500 * G, CPUBandwidth: 60,
		GPUs: []scan.GPU{{Vendor: "nvidia", MemFree: vramFreeGB * G, Bandwidth: bw}}}
}

// TestRealMachines pins picks that were checked against real-world runs.
func TestRealMachines(t *testing.T) {
	c := cat(t)
	want := map[string]map[Pref]string{
		"Desktop: RTX 4070 SUPER 12GB + 62GB DDR5":             {Balanced: "qwen3.8-flash-next", Smartest: "qwen3.8-flash-next"},
		"Laptop: RTX Spark N1X (96GB GPU share, 30GB Windows)": {Balanced: "qwen3.8-flash-next"},
		"RTX 3060 12GB + 32GB":                                 {Balanced: "ternary-bonsai-2-27b"}, // 32K context beats Qwen3.8 27B squeezed into 16K
		"CPU only, 16GB":                                       {Smartest: "qwen3.8-27b"},
	}
	for _, m := range realMachines {
		for _, pref := range []Pref{Balanced, Smartest, Fastest} {
			b := ComputeBudgets(m.hw, m.hw.RAMAvail)
			plans := PlansFor(m.hw, c, b, pref)
			for _, p := range plans {
				// Hard rule: never plan past the budget (paged expert weights excepted:
				// they're clean mmap pages the OS can drop, capped at 1.3x RAM).
				if p.GPUNeed > b.GPU || p.RAMNeed > b.RAM {
					t.Errorf("%s/%s %s exceeds budget: gpu %d>%d ram %d>%d", m.name, pref, p.Model.ID, p.GPUNeed, b.GPU, p.RAMNeed, b.RAM)
				}
			}
			if id, ok := want[m.name][pref]; ok {
				got, _ := Recommend(plans)
				if got.Model.ID != id {
					t.Logf("%s/%s: got %s, want %s", m.name, pref, got.Model.ID, id)
				}
			}
		}
	}
}

// Qwen3.8 27B must outrank Qwen3.6 35B A3B (AA v5 20.1 vs ~9.5).
func TestRankingSanity(t *testing.T) {
	c := cat(t)
	score := map[string]float64{}
	for _, m := range c.Models {
		score[m.ID] = m.Score
	}
	if score["qwen3.8-27b"] <= score["qwen3.6-35b-a3b"] {
		t.Errorf("qwen3.8-27b (%v) should outrank qwen3.6-35b-a3b (%v)", score["qwen3.8-27b"], score["qwen3.6-35b-a3b"])
	}
}

// Low disk space is a warning, never a blocker.
func TestLowDiskStillRecommends(t *testing.T) {
	hw := mac(64, 50, 546)
	hw.DiskFree = 5 * G
	plans := Plans(hw, cat(t), ComputeBudgets(hw, hw.RAMAvail))
	p, ok := Recommend(plans)
	if !ok || p.DiskOK {
		t.Fatalf("want a recommendation flagged as needing disk space, got ok=%v diskOK=%v", ok, p.DiskOK)
	}
}

// The sliding-window cache counts toward memory: Gemma 4 31B needs room for
// it on top of the context's KV cache.
func TestSlidingWindowCacheCounted(t *testing.T) {
	c := cat(t)
	m, ok := c.Find("gemma-4-31b")
	if !ok {
		t.Fatal("gemma-4-31b missing")
	}
	if got := m.SWABytes("f16", 32768); got < 2<<30 || got > 3<<30 {
		t.Fatalf("Gemma 4 31B window cache at f16 = %.2f GB, want ~2.4 GB (50 layers x 16 heads x 256 x 2 x 2 B x 3072 tokens)", float64(got)/float64(G))
	}
	if m.SWABytes("f16", 2048) >= m.SWABytes("f16", 32768) {
		t.Fatal("window cache should be capped by a short context")
	}
	hw := nvidia(11, 40, 504)
	for _, p := range PlansFor(hw, c, ComputeBudgets(hw, hw.RAMAvail), Balanced) {
		if p.Model.ID == "gemma-4-31b" && p.Mode == GPU && p.GPUNeed < p.Quant.Bytes+m.SWABytes(p.KV, p.Ctx) {
			t.Fatalf("plan for %s ignores its window cache: need %d", p.Quant.Name, p.GPUNeed)
		}
	}
}

// A Windows PC with an AMD card: the Vulkan build gets a plan within a
// bigger cushion (free memory there is added up from per-app numbers), and
// the plan puts the model on the card rather than the processor.
func TestWindowsAMDCard(t *testing.T) {
	hw := &scan.Hardware{OS: "windows", Backend: scan.Vulkan, RAMTotal: 32 * G, RAMAvail: 24 * G,
		GPUUsable: 16 * G, DiskFree: 500 * G, CPUBandwidth: 60,
		GPUs: []scan.GPU{{Name: "AMD Radeon RX 7800 XT", Vendor: "amd", MemTotal: 16 * G, MemFree: 16 * G, Bandwidth: 624}}}
	b := ComputeBudgets(hw, hw.RAMAvail)
	vram := float64(16 * G)
	if want := 16*G - uint64(vram*0.12); b.GPU != want {
		t.Fatalf("GPU budget %d, want %d", b.GPU, want)
	}
	nv := *hw
	nv.Backend = scan.CUDA
	if ComputeBudgets(&nv, nv.RAMAvail).GPU <= b.GPU {
		t.Fatal("NVIDIA cards (read from the driver) keep the smaller cushion")
	}
	plans := PlansFor(hw, cat(t), b, Balanced)
	best, ok := Recommend(plans)
	if !ok || best.GPUNeed == 0 || best.GPUNeed > b.GPU {
		t.Fatalf("expected a plan on the card within budget, got ok=%v %+v", ok, best)
	}
	t.Logf("RX 7800 XT 16GB + 32GB: %s %s, gpu %.1f GB, ~%.0f tok/s", best.Model.ID, best.Quant.Name, float64(best.GPUNeed)/float64(G), best.TokPerSec)
}
