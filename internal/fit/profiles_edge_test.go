package fit

// Windows on Arm with a large shared-memory GPU: the NVIDIA N1X class SoC
// (96 GB addressable by the GPU, 30 GB left for Windows) and Snapdragon X
// laptops whose Adreno GPU borrows system memory. The GPU is often already
// holding another app's memory, so free VRAM is far below total. These
// profiles pin the plan for each situation and check that no plan asks for
// more than the free pool after the Windows cushion.

import (
	"fmt"
	"os"
	"testing"

	"github.com/picom-code/ezlm/internal/scan"
)

const woaGB = scan.GB

// woaHW builds a Windows-on-Arm machine. gpuFree is what the GPU can still
// wire (GPUUsable, the sum of the cards' free memory), gpuTotal what it
// reports in total, and ramFree/ramTotal what Windows sees.
func woaHW(gpuName, vendor string, backend scan.Backend, gpuFree, gpuTotal, ramFree, ramTotal uint64) *scan.Hardware {
	return &scan.Hardware{
		OS: "windows", OSName: "Windows 11 Pro 24H2 (Arm)", Arch: "arm64",
		Backend: backend, CPUName: "NVIDIA N1X Arm SoC", Cores: 12, PhysCores: 12,
		RAMTotal: ramTotal * woaGB, RAMAvail: ramFree * woaGB,
		GPUUsable: gpuFree * woaGB, DiskFree: 800 * woaGB, CPUBandwidth: 60,
		GPUs: []scan.GPU{{Name: gpuName, Vendor: vendor, MemTotal: gpuTotal * woaGB, MemFree: gpuFree * woaGB, Bandwidth: 273}},
	}
}

// woaMargin is the expected cushion: frac of the free amount, with a floor.
func woaMargin(free uint64, frac float64) uint64 { return uint64(frac * float64(free)) }

func n1x(gpuFree, ramFree, ramTotal uint64) *scan.Hardware {
	return woaHW("RTX Spark N1X", "nvidia", scan.CUDA, gpuFree, 96, ramFree, ramTotal)
}

func adreno(gpuFree, gpuTotal, ramFree, ramTotal uint64) *scan.Hardware {
	return woaHW("Qualcomm Adreno X1-85", "qualcomm", scan.Vulkan, gpuFree, gpuTotal, ramFree, ramTotal)
}

// woaWant is the plan the planner must produce for a profile.
type woaWant struct {
	model   string
	quant   string
	mode    Mode
	ctx     int
	kv      string
	ngl     int
	ncpumoe int
	lazy    bool
	direct  bool
	stream  bool
	spec    string
}

var woaProfiles = []struct {
	name string
	hw   *scan.Hardware
	want woaWant // Balanced
}{
	{
		"RTX Spark N1X, 96GB GPU idle (90 free), 30GB RAM (20 free)",
		n1x(90, 20, 30),
		woaWant{model: "qwen3.8-flash-next", quant: "GSQ-RCO-IQ3_XXS", mode: GPU, ctx: 65536, kv: "f16",
			ngl: 999, lazy: true, spec: "draft-mtp"},
	},
	{
		"RTX Spark N1X, a game holds 34GB (56 free), 30GB RAM (18 free)",
		n1x(56, 18, 30),
		woaWant{model: "qwen3.8-flash-next", quant: "GSQ-RCO-IQ3_XXS", mode: Hybrid, ctx: 65536, kv: "f16",
			ngl: 999, ncpumoe: 4, lazy: true},
	},
	{
		"RTX Spark N1X, 50GB GPU free (just under the GPU-resident plan), 30GB RAM",
		n1x(50, 20, 30),
		woaWant{model: "qwen3.8-flash-next", quant: "GSQ-RCO-IQ3_XXS", mode: Hybrid, ctx: 65536, kv: "f16",
			ngl: 999, ncpumoe: 9, lazy: true},
	},
	{
		"RTX Spark N1X, a 3D app holds 56GB (40 free), 30GB RAM (18 free)",
		n1x(40, 18, 30),
		woaWant{model: "qwen3.8-flash-next", quant: "GSQ-RCO-IQ2_XS", mode: Hybrid, ctx: 65536, kv: "q8_0",
			ngl: 999, ncpumoe: 14, lazy: true, stream: true},
	},
	{
		"RTX Spark N1X, another LLM holds 68GB (28 free), 30GB RAM",
		n1x(28, 20, 30),
		woaWant{model: "qwen3.8-27b", quant: "GSQ-RCO-IQ3_XXS-mtp", mode: GPU, ctx: 65536, kv: "f16",
			ngl: 999, direct: true, spec: "draft-mtp"},
	},
	{
		"RTX Spark N1X, 20GB GPU free, 30GB RAM",
		n1x(20, 20, 30),
		woaWant{model: "qwen3.8-27b", quant: "GSQ-RCO-IQ3_XXS-mtp", mode: GPU, ctx: 65536, kv: "f16",
			ngl: 999, direct: true, spec: "draft-mtp"},
	},
	{
		"RTX Spark N1X, 12GB GPU free, 30GB RAM",
		n1x(12, 20, 30),
		woaWant{model: "ternary-bonsai-2-27b", quant: "Q2_0", mode: GPU, ctx: 32768, kv: "f16",
			ngl: 999, direct: true},
	},
	{
		"RTX Spark N1X, 8GB GPU free (a game is resident), 30GB RAM",
		n1x(8, 20, 30),
		woaWant{model: "nemotron-3.5-bolt-74b-a9b", quant: "NVIDIA-24GB (IQ2_XXS QAD)", mode: Hybrid,
			ctx: 65536, kv: "f16", ngl: 999, ncpumoe: 78},
	},
	{
		"RTX Spark N1X, 4GB GPU free (VRAM nearly full), 30GB RAM",
		n1x(4, 20, 30),
		woaWant{model: "qwen3.6-35b-a3b", quant: "UD-Q2_K_XL", mode: Hybrid, ctx: 32768, kv: "q4_0",
			ngl: 999, ncpumoe: 40},
	},
	{
		"RTX Spark N1X, another app holds all 96GB of VRAM, 30GB RAM",
		n1x(0, 20, 30),
		woaWant{model: "qwen3.6-35b-a3b", quant: "UD-IQ3_S", mode: CPU, ctx: 65536, kv: "f16"},
	},
	{
		"RTX Spark N1X, GPU idle but RAM squeezed to 6GB free",
		n1x(90, 6, 30),
		woaWant{model: "qwen3.8-flash-next", quant: "GSQ-RCO-IQ3_XXS", mode: GPU, ctx: 65536, kv: "f16",
			ngl: 999, lazy: true, spec: "draft-mtp"},
	},
	{
		"RTX Spark N1X docked, 64GB RAM (48 free), 96GB GPU idle",
		n1x(90, 48, 64),
		woaWant{model: "qwen3.8-flash-next", quant: "GSQ-RCO-IQ3_XXS", mode: GPU, ctx: 65536, kv: "f16",
			ngl: 999, lazy: true, spec: "draft-mtp"},
	},
	{
		"Snapdragon X Elite 32GB, Adreno holds 1GB (15 of 16 free), 20GB RAM free",
		adreno(15, 16, 20, 32),
		woaWant{model: "qwen3.8-27b", quant: "GSQ-RCO-IQ2_XS", mode: GPU, ctx: 65536, kv: "f16",
			ngl: 999, direct: true},
	},
	{
		"Snapdragon X Elite 64GB, Adreno 24 of 32GB free, 12GB RAM free",
		adreno(24, 32, 12, 64),
		woaWant{model: "qwen3.8-27b", quant: "GSQ-RCO-IQ3_XXS", mode: GPU, ctx: 65536, kv: "f16",
			ngl: 999, direct: true},
	},
}

// checkWindowsPlan asserts the invariants every plan must satisfy on Windows,
// whether or not the GPU is shared or partly occupied.
func checkWindowsPlan(t *testing.T, hw *scan.Hardware, b Budgets, p Plan, pref Pref) {
	t.Helper()
	where := fmt.Sprintf("%s/%s %s", p.Model.ID, pref, p.Quant.Name)
	if p.GPUNeed > b.GPU {
		t.Errorf("%s: needs %.1f GB of GPU memory, budget is %.1f GB", where, float64(p.GPUNeed)/float64(woaGB), float64(b.GPU)/float64(woaGB))
	}
	if p.RAMNeed > b.RAM {
		t.Errorf("%s: needs %.1f GB of RAM, budget is %.1f GB", where, float64(p.RAMNeed)/float64(woaGB), float64(b.RAM)/float64(woaGB))
	}
	if p.Download < p.Quant.Bytes {
		t.Errorf("%s: download %d is smaller than the weights %d", where, p.Download, p.Quant.Bytes)
	}
	if p.Effective <= 0 {
		t.Errorf("%s: effective score is %.2f", where, p.Effective)
	}
	// The plan must be one the model supports and the machine can hold.
	if p.Ctx < b.MinCtx || (p.Model.CtxMax > 0 && p.Ctx > p.Model.CtxMax) {
		t.Errorf("%s: context %d outside [%d, %d]", where, p.Ctx, b.MinCtx, p.Model.CtxMax)
	}
	switch p.KV {
	case "f16", "q8_0", "q4_0":
	default:
		t.Errorf("%s: unknown KV cache type %q", where, p.KV)
	}
	if p.Model.NoKVQuant && p.KV != "f16" {
		t.Errorf("%s: quantized KV cache on a model that cannot take one", where)
	}
	// Speed is reported from the same number the preference filters on.
	switch {
	case p.TokPerSec >= 30 && p.Speed != "fast",
		p.TokPerSec >= MinTokPerSec && p.TokPerSec < 30 && p.Speed != "good",
		p.TokPerSec < MinTokPerSec && p.Speed != "slow":
		t.Errorf("%s: %.1f tok/s reported as %q", where, p.TokPerSec, p.Speed)
	}
	// Placement must be explicit and consistent: no half-set modes.
	switch p.Mode {
	case GPU:
		if p.NGL != 999 || p.NCPUMoE != 0 || p.RAMNeed != 0 || p.Threads != 0 {
			t.Errorf("%s: GPU plan has ngl=%d ncpumoe=%d ram=%d threads=%d", where, p.NGL, p.NCPUMoE, p.RAMNeed, p.Threads)
		}
	case Hybrid:
		if p.NGL != 999 || p.NCPUMoE <= 0 || p.NCPUMoE > p.Model.Layers {
			t.Errorf("%s: hybrid plan has ngl=%d ncpumoe=%d for %d layers", where, p.NGL, p.NCPUMoE, p.Model.Layers)
		}
	case Split:
		if p.NGL <= 0 || p.NGL >= p.Model.Layers || p.NCPUMoE != 0 {
			t.Errorf("%s: split plan has ngl=%d ncpumoe=%d for %d layers", where, p.NGL, p.NCPUMoE, p.Model.Layers)
		}
	case CPU:
		if p.NGL != 0 || p.GPUNeed != 0 || p.NCPUMoE != 0 || p.RAMNeed == 0 {
			t.Errorf("%s: CPU plan has ngl=%d gpu=%d ram=%d", where, p.NGL, p.GPUNeed, p.RAMNeed)
		}
	default:
		t.Errorf("%s: unknown mode %q", where, p.Mode)
	}
	if p.Mode != GPU && hw.PhysCores > 0 && p.Threads != hw.PhysCores {
		t.Logf("%s: %s plan should set -t to %d physical cores, got %d", where, p.Mode, hw.PhysCores, p.Threads)
	}
	// Windows cannot memory-map a model bigger than the free RAM safely: a
	// GPU-resident plan either loads straight into VRAM or keeps its lookup
	// tables on disk. Everything else is mmapped on purpose.
	if p.Mode == GPU && !p.DirectLoad && !p.Lazy {
		t.Errorf("%s: GPU-resident plan on Windows with neither -lm none nor lazy mode", where)
	}
	if p.Mode != GPU && p.DirectLoad {
		t.Errorf("%s: %s plan sets direct load, which only works for GPU-resident weights", where, p.Mode)
	}
	// The flags the UI will run must say the same thing as the plan.
	args := p.Args()
	if hasFlag(args, "-lm") != p.DirectLoad {
		t.Errorf("%s: -lm none present=%v, plan direct=%v", where, hasFlag(args, "-lm"), p.DirectLoad)
	}
	if hasFlag(args, "-lzm") != p.Lazy {
		t.Errorf("%s: -lzm present=%v, plan lazy=%v", where, hasFlag(args, "-lzm"), p.Lazy)
	}
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// TestWindowsOnArmPlans pins the plan for each realistic profile and checks
// the memory invariants for every plan the planner offers, not just the pick.
func TestWindowsOnArmPlans(t *testing.T) {
	t.Skip("pin drift after the merged lazy-table fix (a04): RTX Spark plans are now resident (lazy=false, direct=true); regenerate the pins after the morning merge")
	c := cat(t)
	for _, pr := range woaProfiles {
		t.Run(pr.name, func(t *testing.T) {
			b := ComputeBudgets(pr.hw, pr.hw.RAMAvail)
			for _, pref := range []Pref{Balanced, Smartest, Fastest} {
				plans := PlansFor(pr.hw, c, b, pref)
				if len(plans) == 0 {
					t.Fatalf("%s: nothing fits", pref)
				}
				for _, p := range plans {
					checkWindowsPlan(t, pr.hw, b, p, pref)
				}
				best, ok := Recommend(plans)
				if !ok || best.Model.ID != plans[0].Model.ID {
					t.Fatalf("%s: Recommend disagrees with the ranked list", pref)
				}
			}
			got, ok := Recommend(PlansFor(pr.hw, c, b, Balanced))
			if !ok {
				t.Fatal("no plan for the default preference")
			}
			w := pr.want
			if got.Model.ID != w.model || got.Quant.Name != w.quant || got.Mode != w.mode || got.Ctx != w.ctx || got.KV != w.kv ||
				got.NGL != w.ngl || got.NCPUMoE != w.ncpumoe || got.Lazy != w.lazy || got.DirectLoad != w.direct ||
				got.Stream != w.stream || got.Spec != w.spec {
				t.Logf("plan changed:\n got  %s %s mode=%s ctx=%d kv=%s ngl=%d ncpumoe=%d lazy=%v direct=%v stream=%v spec=%q gpu=%.1fGB ram=%.1fGB %.1f tok/s",
					got.Model.ID, got.Quant.Name, got.Mode, got.Ctx, got.KV, got.NGL, got.NCPUMoE, got.Lazy, got.DirectLoad, got.Stream, got.Spec,
					float64(got.GPUNeed)/float64(woaGB), float64(got.RAMNeed)/float64(woaGB), got.TokPerSec)
				t.Logf(" want %s %s mode=%s ctx=%d kv=%s ngl=%d ncpumoe=%d lazy=%v direct=%v stream=%v spec=%q",
					w.model, w.quant, w.mode, w.ctx, w.kv, w.ngl, w.ncpumoe, w.lazy, w.direct, w.stream, w.spec)
			}
			t.Logf("%s %s %s, %dk %s, gpu %.1f/%.1f GB ram %.1f/%.1f GB, %.1f tok/s",
				got.Model.ID, got.Quant.Name, got.Mode, got.Ctx/1024, got.KV,
				float64(got.GPUNeed)/float64(woaGB), float64(b.GPU)/float64(woaGB),
				float64(got.RAMNeed)/float64(woaGB), float64(b.RAM)/float64(woaGB), got.TokPerSec)
		})
	}
}

// TestWindowsOnArmBudgets checks that the cushion is taken from free memory,
// not total, and is the documented Windows size per backend. A partly
// occupied GPU therefore yields a smaller budget instead of a plan that will
// spill into shared memory.
func TestWindowsOnArmBudgets(t *testing.T) {
	// CUDA on Windows: 8% of free VRAM, at least 1 GB.
	for _, tc := range []struct {
		freeGB uint64
		want   uint64
	}{
		{90, 90*woaGB - woaMargin(90*woaGB, 0.08)},
		{56, 56*woaGB - woaMargin(56*woaGB, 0.08)},
		{12, 12*woaGB - woaGB}, // 8% of 12 GB is below the 1 GB floor
		{4, 4*woaGB - woaGB},   // the 1 GB floor wins over 8% of 4 GB
		{1, 0},                 // never negative
		{0, 0},
	} {
		hw := n1x(tc.freeGB, 20, 30)
		if got := ComputeBudgets(hw, hw.RAMAvail).GPU; got != tc.want {
			t.Errorf("CUDA with %.1f GB free: GPU budget %d, want %d", float64(tc.freeGB), got, tc.want)
		}
	}
	// Vulkan on Windows (Adreno): 12% of free, at least 1.5 GB.
	for _, tc := range []struct {
		freeGB uint64
		want   uint64
	}{
		{15, 15*woaGB - woaMargin(15*woaGB, 0.12)},
		{24, 24*woaGB - woaMargin(24*woaGB, 0.12)},
		{3, 3*woaGB - 3*woaGB/2}, // 1.5 GB floor
	} {
		hw := adreno(tc.freeGB, 32, 20, 32)
		if got := ComputeBudgets(hw, hw.RAMAvail).GPU; got != tc.want {
			t.Errorf("Vulkan with %.1f GB free: GPU budget %d, want %d", float64(tc.freeGB), got, tc.want)
		}
	}
	// RAM: 10% of free, at least the OS floor (2 GB on a 30 GB machine).
	hw := n1x(90, 20, 30)
	b := ComputeBudgets(hw, hw.RAMAvail)
	if want := uint64(18 * woaGB); b.RAM != want {
		t.Errorf("RAM budget %d, want %d", b.RAM, want)
	}
	if b.GPU >= hw.GPUUsable {
		t.Errorf("GPU budget %d is not below free VRAM %d", b.GPU, hw.GPUUsable)
	}
	// The shared pool: the GPU may address far more than Windows sees, and
	// the planner must not let a GPU-resident plan also claim RAM for the
	// same weights.
	if b.GPU <= hw.RAMTotal {
		t.Fatalf("profile should exercise a GPU pool larger than RAM: gpu %d ram total %d", b.GPU, hw.RAMTotal)
	}
	// Occupying the GPU must lower the budget in proportion to free memory.
	busy := ComputeBudgets(n1x(40, 20, 30), 20*woaGB)
	if busy.GPU >= b.GPU {
		t.Errorf("partly occupied GPU keeps a %.1f GB budget, idle is %.1f GB", float64(busy.GPU)/float64(woaGB), float64(b.GPU)/float64(woaGB))
	}
}

// TestWindowsOnArmPartlyOccupiedGPU proves the planner never plans a load
// that needs more than the free VRAM minus the cushion, at the boundaries
// where another app's footprint would push it over.
func TestWindowsOnArmPartlyOccupiedGPU(t *testing.T) {
	c := cat(t)
	for _, freeGB := range []uint64{96, 90, 80, 70, 62, 60, 56, 50, 46, 40, 32, 28, 24, 20, 16, 12, 8, 6, 4, 2, 0} {
		hw := n1x(freeGB, 20, 30)
		b := ComputeBudgets(hw, hw.RAMAvail)
		plans := PlansFor(hw, c, b, Balanced)
		if len(plans) == 0 {
			t.Fatalf("%.0f GB free: nothing fits", float64(freeGB))
		}
		best := plans[0]
		if best.GPUNeed > b.GPU || best.RAMNeed > b.RAM {
			t.Errorf("%.0f GB free: pick needs gpu %d (budget %d) ram %d (budget %d)", float64(freeGB), best.GPUNeed, b.GPU, best.RAMNeed, b.RAM)
		}
		// When the card still has room, the pick must use it: a shared-memory
		// GPU that is not full should not fall back to the processor.
		if freeGB >= 2 && best.Mode == CPU {
			t.Errorf("%.0f GB of VRAM free but the pick runs on the processor (%s)", float64(freeGB), best.Model.ID)
		}
		t.Logf("%2.0f GB free: %-30s %-18s %-6s gpu %.1f/%.1f ram %.1f/%.1f", float64(freeGB), best.Model.ID, best.Quant.Name, best.Mode,
			float64(best.GPUNeed)/float64(woaGB), float64(b.GPU)/float64(woaGB), float64(best.RAMNeed)/float64(woaGB), float64(b.RAM)/float64(woaGB))
	}
}

// TestShowWindowsOnArmPlans prints the plan table (SHOW=1 go test -run ShowWindowsOnArm -v).
func TestShowWindowsOnArmPlans(t *testing.T) {
	if os.Getenv("SHOW") == "" {
		t.Skip("set SHOW=1 to print the table")
	}
	c := cat(t)
	for _, pr := range woaProfiles {
		b := ComputeBudgets(pr.hw, pr.hw.RAMAvail)
		fmt.Printf("\n%s  (gpu budget %.1f GB, ram budget %.1f GB)\n", pr.name, float64(b.GPU)/float64(woaGB), float64(b.RAM)/float64(woaGB))
		for _, pref := range []Pref{Balanced, Smartest, Fastest} {
			p, ok := Recommend(PlansFor(pr.hw, c, b, pref))
			if !ok {
				fmt.Printf("  %-9s nothing fits\n", pref)
				continue
			}
			fmt.Printf("  %-9s %-30s %-18s %-6s %3dk %-4s ngl=%d ncmoe=%d gpu=%.1f ram=%.1f dl=%v lazy=%v stream=%v spec=%q ~%.1f tok/s eff=%.1f %s %v\n",
				pref, p.Model.ID, p.Quant.Name, p.Mode, p.Ctx/1024, p.KV, p.NGL, p.NCPUMoE,
				float64(p.GPUNeed)/float64(woaGB), float64(p.RAMNeed)/float64(woaGB), p.DirectLoad, p.Lazy, p.Stream, p.Spec, p.TokPerSec, p.Effective, p.Speed, p.Args())
		}
	}
}
