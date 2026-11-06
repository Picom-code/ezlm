package fit

// Arithmetic and unit invariants: the places where the planner can be wrong
// even when every profile plans "successfully" (a plan that is quietly too
// big, a model that is silently never planned, a quality factor that never
// runs). Every number here is computed from the documented unit (scan.GB is
// 1<<30, so the "GB" in comments and field names means GiB).

import (
	"math"
	"testing"

	"github.com/picom-code/ezlm/internal/catalog"
	"github.com/picom-code/ezlm/internal/scan"
)

// synth is a catalog model built directly, so a test can put one number
// (context cap, weight size, layer count) exactly on a boundary.
var gbf = float64(scan.GB)

func synth(id string, paramsB, activeB float64, moe bool, layers, ctxMax int, bytes uint64) catalog.Model {
	return catalog.Model{ID: id, Name: id, Family: id, ParamsB: paramsB, ActiveParamsB: activeB,
		MoE: moe, Score: 50, Layers: layers, KVHeads: 8, HeadDim: 128, CtxMax: ctxMax,
		Quants: []catalog.Quant{{Name: "Q8_0", Bytes: bytes}}}
}

func planOne(t *testing.T, hw *scan.Hardware, m catalog.Model, pref Pref) (Plan, bool) {
	t.Helper()
	c := &catalog.Catalog{Models: []catalog.Model{m}}
	b := ComputeBudgets(hw, hw.RAMAvail)
	plans := PlansFor(hw, c, b, pref)
	if len(plans) == 0 {
		return Plan{}, false
	}
	return plans[0], true
}

// A model whose context cap is below the shortest rung of the ladder must
// still get a plan. Today the ladder stops at 8192, so a 4K model (older
// catalog entries, and the catalog is fetched from a remote URL, so new
// entries appear without a new release) is silently dropped from every plan
// list: "nothing fits" on a machine that can run it easily.
func TestContextLadderPlansSmallContextModels(t *testing.T) {
	hw := &scan.Hardware{Backend: scan.CUDA, RAMTotal: 32 * scan.GB, RAMAvail: 24 * scan.GB,
		GPUUsable: 8 * scan.GB, DiskFree: 500 * scan.GB, CPUBandwidth: 60,
		GPUs: []scan.GPU{{Vendor: "nvidia", Bandwidth: 500}}}
	for _, ctxMax := range []int{4096, 6144, 8191, 8192, 16384} {
		m := synth("small-ctx", 7, 0, false, 32, ctxMax, 2*scan.GB)
		p, ok := planOne(t, hw, m, Balanced)
		if !ok {
			t.Errorf("ctx_max %d: no plan at all for a model that fits in 8 GB", ctxMax)
			continue
		}
		if p.Ctx > ctxMax || p.Ctx <= 0 {
			t.Errorf("ctx_max %d: planned ctx %d", ctxMax, p.Ctx)
		}
	}
}

// The speed class (and so usable()) must come from the real rate, not from
// the rate rounded for display. A plan at 7.96 tok/s rounds to 8.0 and was
// labelled "good", although MinTokPerSec is 8.
func TestSpeedClassUsesUnroundedRate(t *testing.T) {
	const bw = 50.0
	hw := &scan.Hardware{Backend: scan.CPU, RAMTotal: 32 * scan.GB, RAMAvail: 24 * scan.GB,
		DiskFree: 500 * scan.GB, CPUBandwidth: bw}
	// CPU plan: tok/s = 0.5*bw/(bytes/gb). Pick bytes for ~7.96 tok/s.
	bytes := uint64(0.5 * bw * gbf / 7.96)
	m := synth("just-under-floor", 7, 0, false, 32, 65536, bytes)
	trueTok := 0.5 * bw / (float64(bytes) / float64(scan.GB))
	if trueTok < 7.95 || trueTok >= MinTokPerSec {
		t.Fatalf("test setup: true rate %.4f, want in [7.95, %v)", trueTok, MinTokPerSec)
	}
	p, ok := planOne(t, hw, m, Balanced)
	if !ok {
		t.Fatal("no plan")
	}
	if p.Mode != CPU {
		t.Fatalf("mode %s, want cpu", p.Mode)
	}
	if p.TokPerSec != 8.0 {
		t.Fatalf("display rate %.2f, want 8.0 (rounded for display)", p.TokPerSec)
	}
	if p.Speed != "slow" {
		t.Errorf("speed %q for a true %.3f tok/s, want slow (class must use the unrounded rate)", p.Speed, trueTok)
	}
	if p.Speed != "slow" && usable(p, Balanced) {
		t.Errorf("a %.3f tok/s plan counts as usable", trueTok)
	}
}

// IQ1_M has its own quality factor (0.82); the earlier case that lists
// IQ1_M with Q1/TQ (0.8) used to win, so the 0.82 line never ran.
func TestQuantQualityIQ1M(t *testing.T) {
	m := catalog.Model{Score: 50, ParamsB: 30}
	got := quantQuality(catalog.Quant{Name: "UD-IQ1_M"}, m)
	if got != 0.82 {
		t.Errorf("UD-IQ1_M quality %.2f, want 0.82", got)
	}
	if gs := quantQuality(catalog.Quant{Name: "GSQ-RCO-IQ1_M-mtp"}, m); gs != got {
		t.Errorf("branded IQ1_M %.2f, want %.2f", gs, got)
	}
	if s := quantQuality(catalog.Quant{Name: "UD-IQ1_S"}, m); s != 0.78 {
		t.Errorf("UD-IQ1_S quality %.2f, want 0.78", s)
	}
	if q := quantQuality(catalog.Quant{Name: "UD-Q4_K_XL"}, m); q != 0.985 {
		t.Errorf("UD-Q4_K_XL quality %.2f, want 0.985", q)
	}
}

// The budgets are GiB and the margin floors are GiB, so a 16 GiB card keeps
// exactly 4% (0.64 GiB, above the 0.4 GiB floor) and a Windows Vulkan card
// keeps 12% (above the 1.5 GiB floor).
func TestBudgetUnitsAreGiB(t *testing.T) {
	if scan.GB != 1<<30 {
		t.Fatalf("scan.GB = %d, want 1<<30", scan.GB)
	}
	if gb != float64(scan.GB) {
		t.Fatalf("gb = %v, want %d", gb, scan.GB)
	}
	// osFloor: an eighth of total RAM, clamped to [0.75 GiB, 2 GiB].
	for _, tc := range []struct {
		ramTotal uint64
		want     float64
	}{
		{0, 2 * gb},
		{4 * scan.GB, 0.75 * gb},
		{8 * scan.GB, 1 * gb},
		{16 * scan.GB, 2 * gb},
		{1024 * scan.GB, 2 * gb},
	} {
		if got := osFloor(&scan.Hardware{RAMTotal: tc.ramTotal}); got != tc.want {
			t.Errorf("osFloor(%d GiB) = %.3f GiB, want %.3f", tc.ramTotal>>30, got/gb, tc.want/gb)
		}
	}
	// margin is the larger of the fraction and the floor, in bytes.
	sixteen := float64(16 * scan.GB)
	if got, want := margin(16*scan.GB, 0.04, 0.4*gb), uint64(sixteen*0.04); got != want {
		t.Errorf("margin(16 GiB, 4%%) = %d, want %d", got, want)
	}
	if got := margin(4*scan.GB, 0.04, 0.4*gb); got != uint64(0.4*gbf) {
		t.Errorf("margin(4 GiB, 4%%) = %d, want the 0.4 GiB floor", got)
	}

	base := func() *scan.Hardware {
		return &scan.Hardware{Backend: scan.CUDA, RAMTotal: 32 * scan.GB, RAMAvail: 24 * scan.GB,
			GPUUsable: 16 * scan.GB, DiskFree: 500 * scan.GB, CPUBandwidth: 60,
			GPUs: []scan.GPU{{Vendor: "nvidia", Bandwidth: 500}}}
	}
	b := ComputeBudgets(base(), base().RAMAvail)
	if want := uint64(16*scan.GB) - uint64(sixteen*0.04); b.GPU != want {
		t.Errorf("Linux CUDA GPU budget %d, want %d (4%%)", b.GPU, want)
	}
	win := base()
	win.OS = "windows"
	if got := ComputeBudgets(win, win.RAMAvail); got.GPU != uint64(16*scan.GB)-uint64(sixteen*0.08) {
		t.Errorf("Windows CUDA GPU budget %d, want 8%% off", got.GPU)
	}
	vk := win
	vk.Backend = scan.Vulkan
	if got := ComputeBudgets(vk, vk.RAMAvail); got.GPU != uint64(16*scan.GB)-uint64(sixteen*0.12) {
		t.Errorf("Windows Vulkan GPU budget %d, want 12%% off", got.GPU)
	}
	// Unified memory: one pool, RAM budget mirrors the GPU one, never above
	// what is free.
	mac := &scan.Hardware{Backend: scan.Metal, Unified: true, RAMTotal: 64 * scan.GB,
		RAMAvail: 50 * scan.GB, GPUUsable: 43 * scan.GB, CPUBandwidth: 546,
		GPUs: []scan.GPU{{Vendor: "apple", Bandwidth: 546}}}
	ub := ComputeBudgets(mac, mac.RAMAvail)
	if ub.GPU != ub.RAM {
		t.Errorf("unified budgets differ: gpu %d ram %d", ub.GPU, ub.RAM)
	}
	if ub.GPU > mac.GPUUsable || ub.GPU > mac.RAMAvail {
		t.Errorf("unified budget %d above what is usable (%d, %d free)", ub.GPU, mac.GPUUsable, mac.RAMAvail)
	}
}

// Extreme inputs must not wrap: a budget is never larger than the memory it
// was computed from, and nothing panics or converts an out-of-range float.
func TestBudgetsNoOverflow(t *testing.T) {
	for _, avail := range []uint64{0, 1, 1 << 20, 1 * scan.GB, 1 << 40, math.MaxUint64} {
		for _, total := range []uint64{0, 1, 8 * scan.GB, 1 << 40, math.MaxUint64} {
			for _, backend := range []scan.Backend{scan.CUDA, scan.Vulkan, scan.CPU, scan.Metal} {
				hw := &scan.Hardware{Backend: backend, RAMTotal: total, RAMAvail: avail,
					GPUUsable: avail, Unified: backend == scan.Metal, CPUBandwidth: 60}
				b := ComputeBudgets(hw, avail)
				if b.GPU > hw.GPUUsable || b.RAM > avail {
					t.Fatalf("backend %s total %d avail %d: budgets gpu %d ram %d exceed the input",
						backend, total, avail, b.GPU, b.RAM)
				}
				if hw.GPUUsable > 8*scan.GB && b.GPU < hw.GPUUsable/2 {
					// A wrap or an out-of-range float conversion shows up as a
					// tiny budget on a huge machine.
					t.Fatalf("GPU budget %d far below %d usable bytes", b.GPU, hw.GPUUsable)
				}
			}
		}
	}
}

// The per-token overheads are exactly 4 KiB (compute buffers) and 2 KiB
// (speculative verify) per context token, and the KV cache maths stays
// within one byte per token of the exact fractional size.
func TestOverheadAndKVPerTokenUnits(t *testing.T) {
	w := uint64(4 * scan.GB)
	for _, ctx := range []int{2048, 8192, 65536} {
		if got, want := overhead(w, ctx+1)-overhead(w, ctx), uint64(4096); got != want {
			t.Errorf("overhead step at ctx %d = %d bytes/token, want %d", ctx, got, want)
		}
		if got, want := specOverhead(ctx+1)-specOverhead(ctx), uint64(2048); got != want {
			t.Errorf("specOverhead step at ctx %d = %d bytes/token, want %d", ctx, got, want)
		}
	}
	m := synth("kv", 7, 0, false, 40, 262144, 4*scan.GB)
	m.KVLayers = 0
	elems := float64(2 * 40 * 8 * 128) // K+V, every layer
	f16 := float64(m.KVBytesPerToken("f16"))
	q8 := float64(m.KVBytesPerToken("q8_0"))
	q4 := float64(m.KVBytesPerToken("q4_0"))
	if f16 != 2*elems {
		t.Errorf("f16 KV %.0f bytes/token, want %.0f", f16, 2*elems)
	}
	if math.Abs(q8-elems*34/32) >= 1 {
		t.Errorf("q8_0 KV %.2f bytes/token, want ~%.2f (34/32)", q8, elems*34/32)
	}
	if math.Abs(q4-elems*18/32) >= 1 {
		t.Errorf("q4_0 KV %.2f bytes/token, want ~%.2f (18/32)", q4, elems*18/32)
	}
	if !(f16 > q8 && q8 > q4) {
		t.Errorf("KV sizes not ordered: f16 %.0f q8 %.0f q4 %.0f", f16, q8, q4)
	}
	// The KV cache for a whole context is exactly per-token times tokens.
	p, ok := planOne(t, &scan.Hardware{Backend: scan.CUDA, RAMTotal: 64 * scan.GB, RAMAvail: 48 * scan.GB,
		GPUUsable: 40 * scan.GB, DiskFree: 900 * scan.GB, CPUBandwidth: 60,
		GPUs: []scan.GPU{{Vendor: "nvidia", Bandwidth: 900}}}, m, Balanced)
	if !ok || p.Mode != GPU {
		t.Fatalf("want a GPU plan, got ok=%v mode=%s", ok, p.Mode)
	}
	if want := m.KVBytesPerToken(p.KV)*uint64(p.Ctx) + m.SWABytes(p.KV, p.Ctx); p.GPUNeed < want {
		t.Errorf("plan for ctx %d counts %d bytes of KV, want at least %d", p.Ctx, p.GPUNeed, want)
	}
	// The sliding-window cache is capped at the window plus one batch, and a
	// short context cannot ask for more than it holds.
	g := synth("swa", 7, 0, false, 40, 262144, 4*scan.GB)
	g.SWALayers, g.SWAKVHeads, g.SWAHeadDim, g.SWAWindow = 20, 8, 128, 4096
	if g.SWABytes("f16", 2048) > g.SWABytes("f16", g.SWAWindow+2048) {
		t.Error("window cache grows past the cap")
	}
	if g.SWABytes("f16", g.SWAWindow+2048) != g.SWABytes("f16", 65536) {
		t.Error("window cache is not capped at window+2048")
	}
}

// Hybrid plans clamp RAMNeed to the RAM budget, so a plan that streams
// experts from SSD reports less memory than it uses. fit_test.go's "never
// past the budget" check is therefore vacuous for hybrid plans; this test
// pins the clamp and measures the under-report.
func TestHybridRAMNeedIsClampedToBudget(t *testing.T) {
	// Sparse MoE, 24 GiB of weights in 40 layers: most of it can go to RAM.
	m := synth("moe", 100, 4, true, 40, 65536, 24*scan.GB)
	found := false
	for gpuGB := 8; gpuGB <= 40; gpuGB++ {
		hw := &scan.Hardware{Backend: scan.CUDA, RAMTotal: 64 * scan.GB, RAMAvail: 10 * scan.GB,
			GPUUsable: uint64(gpuGB) * scan.GB, DiskFree: 900 * scan.GB, CPUBandwidth: 60,
			GPUs: []scan.GPU{{Vendor: "nvidia", Bandwidth: 900}}}
		b := ComputeBudgets(hw, hw.RAMAvail)
		p, ok := planOne(t, hw, m, Balanced)
		if !ok || p.Mode != Hybrid || !p.Stream {
			continue
		}
		found = true
		if p.RAMNeed != b.RAM {
			t.Errorf("gpu %d GiB: streaming plan RAMNeed %d, want it clamped to the %d budget", gpuGB, p.RAMNeed, b.RAM)
		}
		// What the experts of the offloaded layers really weigh.
		w := float64(m.Quants[0].Bytes)
		dense := w * m.ActiveParamsB / m.ParamsB * 0.5
		perLayer := (w - dense) / float64(m.Layers)
		trueRAM := uint64(float64(p.NCPUMoE) * perLayer)
		if trueRAM <= p.RAMNeed {
			t.Fatalf("gpu %d GiB: no under-report (true %d, reported %d)", gpuGB, trueRAM, p.RAMNeed)
		}
		t.Logf("gpu %d GiB: %d layers of experts in RAM weigh %.2f GiB, plan reports %.2f GiB (%.2fx free RAM)",
			gpuGB, p.NCPUMoE, float64(trueRAM)/gb, float64(p.RAMNeed)/gb, float64(trueRAM)/float64(p.RAMNeed))
		break
	}
	if !found {
		t.Skip("no streaming hybrid plan in the searched range")
	}
}
