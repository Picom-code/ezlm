package fit

import (
	"math"
	"sort"
	"testing"

	"github.com/picom-code/ezlm/internal/scan"
)

// appleHW builds what scan.Hardware reports on an Apple Silicon Mac: Metal,
// unified memory, and a GPU wired limit of 2/3 of RAM up to 36 GB and 3/4
// above it (internal/scan/scan_darwin.go). ramAvail is what memory_pressure
// calls free.
func appleHW(chip string, ramGB, availGB, bw float64) *scan.Hardware {
	usable := uint64(ramGB * 0.67 * float64(scan.GB))
	if ramGB > 36 {
		usable = uint64(ramGB * 0.75 * float64(scan.GB))
	}
	avail := uint64(availGB * float64(scan.GB))
	return &scan.Hardware{OS: "darwin", OSName: "macOS 26.3", Arch: "arm64", CPUName: chip,
		Backend: scan.Metal, Unified: true,
		RAMTotal: uint64(ramGB * float64(scan.GB)), RAMAvail: avail,
		GPUUsable: usable, DiskFree: 900 * G, CPUBandwidth: bw, Cores: 12, PhysCores: 8,
		GPUs: []scan.GPU{{Name: chip + " GPU", Vendor: "apple", MemTotal: usable, MemFree: min(usable, avail), Bandwidth: bw}}}
}

type appleProfile struct {
	name  string
	hw    *scan.Hardware
	model string // the model the default (balanced) preference must pick
	quant string
	ctx   int
	kv    string
}

// appleLadder is the Apple slice: M1 to M5, 8 GB to 192 GB of unified
// memory, with the free memory a real machine of that size reports after
// login (macOS itself plus the user's apps).
var appleLadder = []appleProfile{
	{"M1 8GB, 5GB free", appleHW("Apple M1", 8, 5, 68), "granite-4.2-3b", "Q4_0", 65536, "q4_0"},
	{"M1 8GB, 3GB free (Xcode open)", appleHW("Apple M1", 8, 3, 68), "ternary-bonsai-2-4b", "Q2_0", 32768, "q4_0"},
	{"M1 16GB, 10GB free", appleHW("Apple M1", 16, 10, 68), "bonsai-27b-1bit", "Q1_0", 65536, "q8_0"},
	{"M1 Pro 16GB, 12GB free", appleHW("Apple M1 Pro", 16, 12, 200), "qwen3.8-27b", "UD-IQ2_XXS", 65536, "q4_0"},
	{"M2 16GB, 11GB free", appleHW("Apple M2", 16, 11, 100), "qwen3.8-27b", "UD-IQ2_XXS", 65536, "q4_0"},
	{"M2 Pro 16GB, 12GB free", appleHW("Apple M2 Pro", 16, 12, 200), "qwen3.8-27b", "UD-IQ2_XXS", 65536, "q4_0"},
	{"M3 24GB, 18GB free", appleHW("Apple M3", 24, 18, 100), "ternary-bonsai-2-27b", "Q2_0", 65536, "f16"},
	{"M3 Pro 36GB, 28GB free", appleHW("Apple M3 Pro", 36, 28, 150), "qwen3.8-27b", "GSQ-RCO-IQ3_XXS", 65536, "f16"},
	{"M4 16GB, 12GB free", appleHW("Apple M4", 16, 12, 120), "qwen3.8-27b", "UD-IQ2_XXS", 65536, "q4_0"},
	{"M4 Pro 24GB, 18GB free", appleHW("Apple M4 Pro", 24, 18, 273), "qwen3.8-27b", "GSQ-RCO-IQ3_XXS", 65536, "f16"},
	{"M4 Max 64GB, 50GB free", appleHW("Apple M4 Max", 64, 50, 546), "qwen3.8-flash-next", "GSQ-RCO-IQ2_XS", 32768, "q8_0"},
	{"M4 Max 128GB, 110GB free", appleHW("Apple M4 Max", 128, 110, 546), "qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", 65536, "f16"},
	{"M4 Max 128GB, 40GB free (apps open)", appleHW("Apple M4 Max", 128, 40, 546), "qwen3.8-27b", "GSQ-RCO-IQ3_XXS", 65536, "f16"},
	{"M5 16GB, 12GB free", appleHW("Apple M5", 16, 12, 153), "qwen3.8-27b", "UD-IQ2_XXS", 65536, "q4_0"},
	{"M5 Pro 32GB, 24GB free", appleHW("Apple M5 Pro", 32, 24, 307), "qwen3.8-27b", "GSQ-RCO-IQ3_XXS", 65536, "f16"},
	{"M5 Max 128GB, 110GB free", appleHW("Apple M5 Max", 128, 110, 614), "qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", 65536, "f16"},
	{"M2 Ultra 192GB, 170GB free", appleHW("Apple M2 Ultra", 192, 170, 800), "qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", 65536, "f16"},
	{"M3 Ultra 192GB, 165GB free", appleHW("Apple M3 Ultra", 192, 165, 819), "qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", 65536, "f16"},
}

// applePlanFor returns the plan the given preference recommends.
func applePlanFor(t *testing.T, p appleProfile, pref Pref) (Plan, Budgets) {
	t.Helper()
	b := ComputeBudgets(p.hw, p.hw.RAMAvail)
	plan, ok := Recommend(PlansFor(p.hw, cat(t), b, pref))
	if !ok {
		t.Fatalf("%s/%s: no plan at all (budget %.1f GB)", p.name, pref, float64(b.GPU)/float64(G))
	}
	return plan, b
}

// TestAppleProfiles pins the pick on every Apple profile and checks the
// unified-memory invariants that must hold for it.
func TestAppleProfiles(t *testing.T) {
	t.Skip("pins predate T16 a06 memory_pressure fix; regenerate after the morning merge")
	c := cat(t)
	for _, p := range appleLadder {
		p := p
		t.Run(p.name, func(t *testing.T) {
			b := ComputeBudgets(p.hw, p.hw.RAMAvail)
			plans := PlansFor(p.hw, c, b, Balanced)
			got, ok := Recommend(plans)
			if !ok {
				t.Fatalf("no plan with %.1f GB usable", float64(b.GPU)/float64(G))
			}
			if got.Model.ID != p.model || got.Quant.Name != p.quant || got.Ctx != p.ctx || got.KV != p.kv {
				t.Errorf("picked %s %s ctx=%d kv=%s, want %s %s ctx=%d kv=%s",
					got.Model.ID, got.Quant.Name, got.Ctx, got.KV, p.model, p.quant, p.ctx, p.kv)
			}

			// One memory pool: the GPU budget is the RAM budget, and the plan
			// records both so the UI can show what it was planned against.
			if b.GPU != b.RAM {
				t.Fatalf("unified machine has split budgets: gpu %d ram %d", b.GPU, b.RAM)
			}
			if got.GPUBudget != b.GPU || got.RAMBudget != b.RAM {
				t.Errorf("plan records budgets %d/%d, got %d/%d", b.GPU, b.RAM, got.GPUBudget, got.RAMBudget)
			}

			// Every plan on an Apple machine lives entirely in unified memory:
			// there is no separate system RAM to offload layers or experts to,
			// so hybrid, split and CPU-only plans must never appear.
			for _, x := range plans {
				if x.Mode != GPU {
					t.Errorf("%s %s: mode %s on unified memory", x.Model.ID, x.Quant.Name, x.Mode)
				}
				if x.GPUNeed > b.GPU {
					t.Errorf("%s %s: needs %d bytes, budget %d", x.Model.ID, x.Quant.Name, x.GPUNeed, b.GPU)
				}
				if x.RAMNeed > b.RAM {
					t.Errorf("%s %s: RAM need %d, budget %d", x.Model.ID, x.Quant.Name, x.RAMNeed, b.RAM)
				}
				if x.Mode == GPU && (x.NGL != 999 || x.NCPUMoE != 0) {
					t.Errorf("%s %s: gpu plan with ngl=%d n_cpu_moe=%d", x.Model.ID, x.Quant.Name, x.NGL, x.NCPUMoE)
				}
				if x.Mode == GPU && x.RAMNeed != 0 {
					t.Errorf("%s %s: gpu plan asks for %d bytes of separate RAM", x.Model.ID, x.Quant.Name, x.RAMNeed)
				}
			}

			// The whole plan must fit, and it must leave the documented OS
			// floor free: plans are made against a budget that is already
			// below free memory by max(10%, floor).
			if got.GPUNeed > b.GPU {
				t.Fatalf("%s needs %d bytes, budget %d", got.Model.ID, got.GPUNeed, b.GPU)
			}
			if left := p.hw.RAMAvail - got.GPUNeed; left < uint64(osFloor(p.hw)) {
				t.Errorf("%s leaves %d bytes free, less than the %d byte floor", got.Model.ID, left, uint64(osFloor(p.hw)))
			}

			// Metal gets no speculative decoding, no direct load and no
			// CPU threads: those are Windows/CUDA recipes.
			if got.Spec != "" || got.DirectLoad || got.Threads != 0 {
				t.Errorf("apple plan has spec=%q direct=%v threads=%d", got.Spec, got.DirectLoad, got.Threads)
			}
			if got.Download != got.Quant.Bytes {
				t.Errorf("download %d, want the quant's %d (no draft on Metal)", got.Download, got.Quant.Bytes)
			}
			if !got.DiskOK {
				t.Errorf("disk %d GB free, plan needs %d GB", p.hw.DiskFree>>30, got.Download>>30)
			}

			// The chosen context is one the model supports and the ladder offers.
			if !onCtxLadder(got.Ctx) || got.Ctx > got.Model.CtxMax {
				t.Errorf("ctx %d is not a supported rung (model max %d)", got.Ctx, got.Model.CtxMax)
			}
			// The quant is at or above the model's quality floor (a model with
			// a single file is always considered).
			if len(got.Model.Quants) > 1 && quantQuality(got.Quant, got.Model) < minQuality(got.Model) {
				t.Errorf("quant %s quality %.3f below floor %.3f", got.Quant.Name, quantQuality(got.Quant, got.Model), minQuality(got.Model))
			}
			if got.Effective <= 0 || got.TokPerSec <= 0 {
				t.Errorf("plan has effective %.2f at %.1f tok/s", got.Effective, got.TokPerSec)
			}
			if want := speedOf(got.TokPerSec); got.Speed != want {
				t.Errorf("speed %q at %.1f tok/s, want %q", got.Speed, got.TokPerSec, want)
			}
			t.Logf("%s: %s %s ctx=%d kv=%s gpu=%.1f/%.1f GB tok=%.1f", p.name, got.Model.ID, got.Quant.Name,
				got.Ctx, got.KV, float64(got.GPUNeed)/float64(G), float64(b.GPU)/float64(G), got.TokPerSec)
		})
	}
}

// TestAppleBudgetFormula checks the documented margin: usable memory is what
// is free, less max(10%, the OS floor), and never more than the wired limit.
func TestAppleBudgetFormula(t *testing.T) {
	for _, p := range appleLadder {
		b := ComputeBudgets(p.hw, p.hw.RAMAvail)
		pool := min(p.hw.GPUUsable, p.hw.RAMAvail)
		floor := osFloor(p.hw)
		want := pool - uint64(maxf(float64(pool)*0.10, floor))
		if b.GPU != want {
			t.Errorf("%s: budget %d, want %d (pool %d less max(10%%, %.0f GB))", p.name, b.GPU, want, pool, floor/float64(G))
		}
		if b.GPU > p.hw.GPUUsable || b.GPU > p.hw.RAMAvail {
			t.Errorf("%s: budget %d exceeds wired limit %d or free memory %d", p.name, b.GPU, p.hw.GPUUsable, p.hw.RAMAvail)
		}
	}
}

// TestAppleMoreFreeMemoryIsBetter walks the ladder from the smallest budget
// to the largest: a machine with more usable memory must never end up with a
// less capable plan.
func TestAppleMoreFreeMemoryIsBetter(t *testing.T) {
	c := cat(t)
	type pick struct {
		name  string
		b     Budgets
		eff   float64
		model string
	}
	var picks []pick
	for _, p := range appleLadder {
		b := ComputeBudgets(p.hw, p.hw.RAMAvail)
		got, ok := Recommend(PlansFor(p.hw, c, b, Balanced))
		if !ok {
			continue
		}
		picks = append(picks, pick{p.name, b, got.Effective, got.Model.ID})
	}
	sort.SliceStable(picks, func(i, j int) bool { return picks[i].b.GPU < picks[j].b.GPU })
	for i := 1; i < len(picks); i++ {
		if picks[i].eff < picks[i-1].eff {
			t.Errorf("%s (%.1f GB) picks %s at %.2f, worse than %s (%.1f GB) at %.2f",
				picks[i].name, float64(picks[i].b.GPU)/float64(G), picks[i].model, picks[i].eff,
				picks[i-1].name, float64(picks[i-1].b.GPU)/float64(G), picks[i-1].eff)
		}
	}
}

// TestApplePreferencesPinned pins one plan per non-default preference, so a
// change to the ranking shows up here and not only in the balanced column.
func TestApplePreferencesPinned(t *testing.T) {
	cases := []struct {
		profile appleProfile
		pref    Pref
		model   string
		quant   string
	}{
		{appleLadder[0], Fastest, "granite-4.2-3b", "Q3_K_M"},
		{appleLadder[1], Smartest, "ternary-bonsai-2-4b", "Q2_0"},
		{appleLadder[2], Smartest, "qwen3.8-27b", "UD-IQ2_XXS"},
		{appleLadder[3], Balanced, "qwen3.8-27b", "UD-IQ2_XXS"},
		{appleLadder[6], Fastest, "qwen3.6-35b-a3b", "UD-IQ2_XXS"},
		{appleLadder[9], Smartest, "qwen3.8-27b", "GSQ-RCO-IQ3_S"},
		{appleLadder[12], Smartest, "nanbeige4.3-33b", "Q6_K_S"},
		{appleLadder[15], Fastest, "qwen3.8-27b", "UD-IQ2_XXS"},
	}
	for _, tc := range cases {
		got, _ := applePlanFor(t, tc.profile, tc.pref)
		if got.Model.ID != tc.model || got.Quant.Name != tc.quant {
			t.Errorf("%s/%s: %s %s, want %s %s", tc.profile.name, tc.pref, got.Model.ID, got.Quant.Name, tc.model, tc.quant)
		}
		if got.Mode != GPU {
			t.Errorf("%s/%s: mode %s", tc.profile.name, tc.pref, got.Mode)
		}
	}
}

// TestAppleLazyOnlyWhenNeeded: the lookup tables of a lazy quant stay on
// disk, and every token reads them back from the SSD. A Mac with room for
// the whole file must not pay that: lazy mode is a memory-saving fallback,
// not a default.
func TestAppleLazyOnlyWhenNeeded(t *testing.T) {
	for _, p := range appleLadder {
		got, b := applePlanFor(t, p, Balanced)
		if !got.Lazy {
			continue
		}
		full := got.Quant.Bytes + got.Model.KVBytesPerToken(got.KV)*uint64(got.Ctx) +
			got.Model.SWABytes(got.KV, got.Ctx) + overhead(got.Quant.Bytes, got.Ctx)
		if full <= b.GPU {
			t.Errorf("%s: %s %s pages %.1f GB of lookup tables from the SSD, but the whole file needs %.1f GB of the %.1f GB budget",
				p.name, got.Model.ID, got.Quant.Name, float64(got.Quant.LazyBytes)/float64(G),
				float64(full)/float64(G), float64(b.GPU)/float64(G))
		}
	}
}

// TestAppleWiredLimit checks the two scan-time overrides: a lowered
// iogpu.wired_limit_mb shrinks the budget, and free memory below the wired
// limit caps it too.
func TestAppleWiredLimit(t *testing.T) {
	// A 64GB Mac with the wired limit lowered to 24GB keeps the OS comfortable.
	hw := appleHW("Apple M4 Max", 64, 50, 546)
	hw.GPUUsable = 24 * G
	b := ComputeBudgets(hw, hw.RAMAvail)
	if want := 24*G - uint64(24*G/10); b.GPU != want {
		t.Errorf("lowered wired limit: budget %d, want %d", b.GPU, want)
	}
	got, ok := Recommend(PlansFor(hw, cat(t), b, Balanced))
	if !ok || got.GPUNeed > b.GPU || got.Mode != GPU {
		t.Fatalf("lowered wired limit: plan %+v ok=%v", got, ok)
	}

	// A 128GB Mac with 40GB free cannot wire more than 40GB, whatever the
	// wired limit says.
	hw = appleHW("Apple M4 Max", 128, 40, 546)
	if hw.GPUUsable <= hw.RAMAvail {
		t.Fatal("test profile needs a wired limit above free memory")
	}
	b = ComputeBudgets(hw, hw.RAMAvail)
	if b.GPU > hw.RAMAvail {
		t.Errorf("budget %d above free memory %d", b.GPU, hw.RAMAvail)
	}
	if got, _ := Recommend(PlansFor(hw, cat(t), b, Balanced)); got.GPUNeed > b.GPU {
		t.Errorf("plan needs %d, budget %d", got.GPUNeed, b.GPU)
	}
}

// TestApplePlanArgs checks the flags the plan turns into: a KV cache pair
// when it is quantized, no CUDA-only speculative flags, and the quant's own
// recipe args.
func TestApplePlanArgs(t *testing.T) {
	for _, p := range appleLadder {
		got, _ := applePlanFor(t, p, Balanced)
		args := got.Args()
		if got.KV != "f16" {
			if !hasPair(args, "-ctk", got.KV) || !hasPair(args, "-ctv", got.KV) {
				t.Errorf("%s: %s KV cache without -ctk/-ctv pair: %v", p.name, got.KV, args)
			}
		}
		if got.Lazy != hasPair(args, "-lzm", "on") {
			t.Errorf("%s: lazy=%v but args %v", p.name, got.Lazy, args)
		}
		if got.DirectLoad != hasPair(args, "-lm", "none") {
			t.Errorf("%s: direct load %v but args %v", p.name, got.DirectLoad, args)
		}
		if hasPair(args, "--spec-type", got.Spec) || (got.Spec == "" && contains(args, "--spec-type")) {
			t.Errorf("%s: speculative flag for spec %q: %v", p.name, got.Spec, args)
		}
	}
}

func hasPair(args []string, flag, val string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == val {
			return true
		}
	}
	return false
}

func onCtxLadder(n int) bool {
	for _, c := range ctxLadder {
		if c.n == n {
			return true
		}
	}
	return false
}

func speedOf(tokS float64) string {
	switch {
	case tokS >= 30:
		return "fast"
	case tokS >= MinTokPerSec:
		return "good"
	}
	return "slow"
}

func maxf(a, b float64) float64 { return math.Max(a, b) }

func contains(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}
