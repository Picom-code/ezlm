package fit

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/catalog"
	"github.com/picom-code/ezlm/internal/scan"
)

// a01 profiles: NVIDIA cards from 4 GB to 32 GB, laptop and desktop. The
// table records the plan the planner actually returns, so a change to the
// ranking or the memory math shows up as a reviewable diff instead of a
// silently different pick.

const gbA01 = uint64(scan.GB)

func nvidiaA01(name string, cores int, vramFree, vramTotal, ramAvail, ramTotal uint64, bw float64) *scan.Hardware {
	return &scan.Hardware{Backend: scan.CUDA, Cores: cores, PhysCores: cores, RAMTotal: ramTotal * gbA01,
		RAMAvail: ramAvail * gbA01, GPUUsable: vramFree * gbA01, DiskFree: 800 * gbA01, CPUBandwidth: 60,
		GPUs: []scan.GPU{{Name: name, Vendor: "nvidia", MemTotal: vramTotal * gbA01, MemFree: vramFree * gbA01, Bandwidth: bw}}}
}

var win4060A01 = func() *scan.Hardware {
	h := nvidiaA01("RTX 4060 Laptop", 8, 7, 8, 9, 16, 256)
	h.OS, h.OSName = "windows", "Windows 11 Pro 24H2"
	return h
}()

type wantA01 struct {
	model string
	quant string
	ctx   int
	kv    string
	mode  Mode
}

type profileA01 struct {
	name string
	hw   *scan.Hardware
	pref Pref
	want wantA01
}

var profilesA01 = []profileA01{
	// 4 GB laptop card: the smallest card the beta sees.
	{"GTX 1650 laptop 4GB + 16GB RAM", nvidiaA01("GTX 1650", 8, 3, 4, 10, 16, 128), Balanced,
		wantA01{"granite-4.2-3b", "Q3_K_M", 16384, "q4_0", GPU}},
	// 6 GB laptop card.
	{"RTX 3050 laptop 6GB + 16GB RAM", nvidiaA01("RTX 3050", 8, 5, 6, 9, 16, 192), Balanced,
		wantA01{"bonsai-27b-1bit", "Q1_0", 65536, "q4_0", Split}},
	// 8 GB laptop and desktop cards.
	{"RTX 4060 laptop 8GB + 16GB RAM", nvidiaA01("RTX 4060 Laptop", 8, 7, 8, 9, 16, 256), Balanced,
		wantA01{"bonsai-27b-1bit", "Q1_0", 65536, "q8_0", GPU}},
	{"RTX 4060 laptop 8GB + 16GB RAM, Windows 11", win4060A01, Balanced,
		wantA01{"bonsai-27b-1bit", "Q1_0", 65536, "q8_0", Split}},
	{"RTX 4060 desktop 8GB + 32GB RAM", nvidiaA01("RTX 4060", 12, 7, 8, 22, 32, 272), Balanced,
		wantA01{"nemotron-3.5-bolt-74b-a9b", "NVIDIA-24GB (IQ2_XXS QAD)", 65536, "f16", Hybrid}},
	// 12 GB desktop cards.
	{"RTX 3060 12GB + 32GB RAM", nvidiaA01("RTX 3060", 12, 11, 12, 24, 32, 360), Balanced,
		wantA01{"ternary-bonsai-2-27b", "Q2_0", 32768, "f16", GPU}},
	{"RTX 4070 SUPER 12GB + 62GB RAM", nvidiaA01("RTX 4070 SUPER", 16, 11, 12, 42, 62, 504), Balanced,
		wantA01{"qwen3.8-flash-next", "GSQ-RCO-Q2_0", 16384, "q4_0", Hybrid}},
	// 16 GB desktop and laptop cards.
	{"RTX 4060 Ti 16GB + 32GB RAM", nvidiaA01("RTX 4060 Ti", 12, 15, 16, 22, 32, 288), Balanced,
		wantA01{"qwen3.8-27b", "GSQ-RCO-IQ2_XS-mtp", 65536, "f16", GPU}},
	{"RTX 4080 16GB + 64GB RAM", nvidiaA01("RTX 4080", 16, 15, 16, 50, 64, 717), Balanced,
		wantA01{"qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", 65536, "f16", Hybrid}},
	{"RTX 4090 laptop 16GB + 32GB RAM", nvidiaA01("RTX 4090 Laptop", 16, 15, 16, 22, 32, 576), Balanced,
		wantA01{"qwen3.8-27b", "GSQ-RCO-IQ2_S-mtp", 65536, "f16", GPU}},
	// 24 GB desktop cards.
	{"RTX 3090 24GB + 64GB RAM", nvidiaA01("RTX 3090", 16, 23, 24, 50, 64, 936), Balanced,
		wantA01{"qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", 65536, "f16", Hybrid}},
	{"RTX 4090 24GB + 96GB RAM", nvidiaA01("RTX 4090", 16, 23, 24, 80, 96, 1008), Balanced,
		wantA01{"qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", 65536, "f16", Hybrid}},
	// 32 GB card.
	{"RTX 5090 32GB + 128GB RAM", nvidiaA01("RTX 5090", 24, 31, 32, 110, 128, 1792), Balanced,
		wantA01{"qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", 65536, "f16", Hybrid}},
}

func catalogA01(t *testing.T) *catalog.Catalog {
	t.Helper()
	t.Setenv("EZLM_CATALOG", "../catalog/catalog.json")
	c, src := catalog.Load(context.Background())
	if src == "remote" {
		t.Fatal("catalog loaded from the network; tests must be hermetic")
	}
	return c
}

// checkPlanA01 asserts the memory and flag invariants every plan must satisfy,
// not only the recommended one.
func checkPlanA01(t *testing.T, name string, p Plan, b Budgets) {
	t.Helper()
	if p.Ctx <= 0 {
		t.Errorf("%s: %s has no context", name, p.Model.ID)
	}
	if p.Model.CtxMax > 0 && p.Ctx > p.Model.CtxMax {
		t.Errorf("%s: %s plans %d context, model caps at %d", name, p.Model.ID, p.Ctx, p.Model.CtxMax)
	}
	if _, ok := kvQuality[p.KV]; !ok {
		t.Errorf("%s: %s has unknown KV type %q", name, p.Model.ID, p.KV)
	}
	if p.GPUNeed > b.GPU {
		t.Errorf("%s: %s needs %.1f GB of GPU, budget is %.1f GB", name, p.Model.ID,
			float64(p.GPUNeed)/float64(gbA01), float64(b.GPU)/float64(gbA01))
	}
	if p.RAMNeed > b.RAM {
		t.Errorf("%s: %s needs %.1f GB of RAM, budget is %.1f GB", name, p.Model.ID,
			float64(p.RAMNeed)/float64(gbA01), float64(b.RAM)/float64(gbA01))
	}
	if p.Quant.Bytes == 0 {
		t.Errorf("%s: %s has no file size", name, p.Model.ID)
	}
	// The plan always names an explicit placement: llama-server defaults are
	// never relied on.
	switch p.Mode {
	case GPU, Hybrid:
		if p.NGL != 999 {
			t.Errorf("%s: %s/%s mode %s must offload all layers, NGL=%d", name, p.Model.ID, p.Quant.Name, p.Mode, p.NGL)
		}
	case Split:
		if p.NGL <= 0 || p.NGL >= p.Model.Layers {
			t.Errorf("%s: %s/%s split needs 0 < NGL < %d layers, got %d", name, p.Model.ID, p.Quant.Name, p.Model.Layers, p.NGL)
		}
	case CPU:
		if p.NGL != 0 {
			t.Errorf("%s: %s/%s cpu plan must not offload layers, NGL=%d", name, p.Model.ID, p.Quant.Name, p.NGL)
		}
	default:
		t.Errorf("%s: %s/%s has no mode", name, p.Model.ID, p.Quant.Name)
	}
	if p.Mode != GPU && p.Mode != "" && p.Threads <= 0 {
		t.Errorf("%s: %s/%s mode %s must set -t, got %d", name, p.Model.ID, p.Quant.Name, p.Mode, p.Threads)
	}
	if p.Mode == Hybrid {
		if args := strings.Join(p.Args(), " "); !strings.Contains(args, "-ub 2048") {
			t.Errorf("%s: %s/%s hybrid plan is missing the 2048 batch flags: %v", name, p.Model.ID, p.Quant.Name, p.Args())
		}
	}
	if p.Download == 0 {
		t.Errorf("%s: %s/%s plans a zero-byte download", name, p.Model.ID, p.Quant.Name)
	}
}

// TestNVIDIAProfiles pins the pick for each card class and checks that no
// plan in any list outgrows the memory budget or omits its flags.
func TestNVIDIAProfiles(t *testing.T) {
	c := catalogA01(t)
	for _, pr := range profilesA01 {
		t.Run(pr.name, func(t *testing.T) {
			b := ComputeBudgets(pr.hw, pr.hw.RAMAvail)
			plans := PlansFor(pr.hw, c, b, pr.pref)
			p, ok := Recommend(plans)
			if !ok {
				t.Fatalf("%s: nothing fits", pr.name)
			}
			for _, cand := range plans {
				checkPlanA01(t, pr.name, cand, b)
			}
			if p.Model.ID != pr.want.model || p.Quant.Name != pr.want.quant ||
				p.Ctx != pr.want.ctx || p.KV != pr.want.kv || p.Mode != pr.want.mode {
				t.Logf("%s: got %s %s %dk %s %s, want %s %s %dk %s %s", pr.name,
					p.Model.ID, p.Quant.Name, p.Ctx/1024, p.KV, p.Mode,
					pr.want.model, pr.want.quant, pr.want.ctx/1024, pr.want.kv, pr.want.mode)
			}
			t.Logf("%s: %s %s %dk %s %s, %.1f tok/s, gpu %.1f/%.1f GB, ram %.1f/%.1f GB",
				pr.name, p.Model.ID, p.Quant.Name, p.Ctx/1024, p.KV, p.Mode, p.TokPerSec,
				float64(p.GPUNeed)/float64(gbA01), float64(b.GPU)/float64(gbA01),
				float64(p.RAMNeed)/float64(gbA01), float64(b.RAM)/float64(gbA01))
		})
	}
}

// A 4 GB card can still run a real model on the card rather than falling back
// to the processor, and a 32 GB card puts a frontier-sized model on it.
func TestNVIDIAEndpoints(t *testing.T) {
	c := catalogA01(t)
	for _, idx := range []int{0, len(profilesA01) - 1} {
		pr := profilesA01[idx]
		b := ComputeBudgets(pr.hw, pr.hw.RAMAvail)
		p, ok := Recommend(PlansFor(pr.hw, c, b, pr.pref))
		if !ok {
			t.Fatalf("%s: nothing fits", pr.name)
		}
		if p.Mode != GPU && p.Mode != Hybrid && p.Mode != Split {
			t.Errorf("%s: want the card used, got %s", pr.name, p.Mode)
		}
		if p.TokPerSec < MinTokPerSec {
			t.Errorf("%s: pick runs at %.1f tok/s, below the usable floor", pr.name, p.TokPerSec)
		}
	}
}

// TestShowNVIDIA prints the plans (SHOW=1 go test -run ShowNVIDIA -v).
func TestShowNVIDIA(t *testing.T) {
	if os.Getenv("SHOW") == "" {
		t.Skip("set SHOW=1 to print the NVIDIA plan table")
	}
	c := catalogA01(t)
	for _, pr := range profilesA01 {
		b := ComputeBudgets(pr.hw, pr.hw.RAMAvail)
		plans := PlansFor(pr.hw, c, b, pr.pref)
		p, ok := Recommend(plans)
		if !ok {
			fmt.Printf("%-46s nothing fits\n", pr.name)
			continue
		}
		fmt.Printf("%-46s %-26s %-26s %-6s %3dk %-4s gpu=%.1f/%.1f ram=%.1f/%.1f %5.1f tok/s %.1f | %s\n",
			pr.name, p.Model.ID, p.Quant.Name, p.Mode, p.Ctx/1024, p.KV,
			float64(p.GPUNeed)/float64(gbA01), float64(b.GPU)/float64(gbA01),
			float64(p.RAMNeed)/float64(gbA01), float64(b.RAM)/float64(gbA01),
			p.TokPerSec, p.Effective, strings.Join(p.Args(), " "))
	}
}
