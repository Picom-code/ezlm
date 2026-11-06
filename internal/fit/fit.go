// Package fit picks the smartest model setup that fits a machine, with a hard
// safety margin so a deployment can never starve the OS of memory.
//
// For every model it tries each quant × context × KV-cache type × placement
// (all on GPU, MoE experts in RAM, split layers, CPU only), and ranks the
// results by effective intelligence: the model's score adjusted for the
// quality lost to quantization, as long as the speed is usable.
package fit

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/picom-code/ezlm/internal/catalog"
	"github.com/picom-code/ezlm/internal/scan"
)

const gb = float64(scan.GB)

// Mode describes where the weights live.
type Mode string

const (
	GPU    Mode = "gpu"    // everything in VRAM / unified memory
	Hybrid Mode = "hybrid" // MoE experts partly in system RAM (--n-cpu-moe)
	Split  Mode = "split"  // dense layers partly on GPU (-ngl < layers)
	CPU    Mode = "cpu"    // no GPU offload
)

type Plan struct {
	Model   catalog.Model `json:"model"`
	Quant   catalog.Quant `json:"quant"`
	Ctx     int           `json:"ctx"`
	KV      string        `json:"kv"` // f16 | q8_0 | q4_0
	Mode    Mode          `json:"mode"`
	NGL     int           `json:"ngl"`       // layers on GPU (999 = all)
	NCPUMoE int           `json:"n_cpu_moe"` // layers whose experts stay in RAM
	Spec    string        `json:"spec"`      // speculative decoding type, "" if none
	Lazy    bool          `json:"lazy"`      // big lookup tables read from disk on demand
	Stream  bool          `json:"stream"`    // experts partly paged from SSD (slower, but safe: clean pages)
	// DirectLoad reads weights straight into GPU memory (-lm none) instead of
	// memory-mapping them through system RAM. Used on Windows for fully
	// GPU-resident models: mapping a huge file with little free RAM has
	// crashed Windows (seen on an RTX Spark laptop).
	DirectLoad bool `json:"direct_load"`
	Threads    int  `json:"threads,omitempty"` // -t: physical cores when the CPU carries weights

	GPUNeed   uint64 `json:"gpu_need"`
	GPUBudget uint64 `json:"gpu_budget"`
	RAMNeed   uint64 `json:"ram_need"`
	RAMBudget uint64 `json:"ram_budget"`
	Download  uint64 `json:"download"` // total bytes to fetch (model + draft)

	TokPerSec float64 `json:"tok_per_sec"`
	Speed     string  `json:"speed"`     // fast | good | slow
	Effective float64 `json:"effective"` // score after quant/KV/context adjustments
	DiskOK    bool    `json:"disk_ok"`
	// Downloaded: the model's files are already on this computer (no download,
	// no disk space needed).
	Downloaded bool `json:"downloaded"`
	// Partial: bytes of a cancelled download already on disk (it resumes).
	Partial uint64 `json:"partial,omitempty"`
}

// Pref is what the user values when models trade off intelligence and speed.
type Pref string

const (
	Balanced Pref = "balanced" // smart, at a comfortable speed (default)
	Smartest Pref = "smartest" // the most capable model that fits, even if slow
	Fastest  Pref = "fastest"  // snappy responses first
)

// SmartestMinTokPerSec: below this even patient users give up. With a
// graphics card there's always something smart that's faster than this, and
// a 3 tok/s model also reads prompts so slowly that coding agents time out.
// Processor-only machines get the lower floor: nothing there is fast.
const SmartestMinTokPerSec, SmartestMinCPU = 5.0, 2.0

// Budgets are the usable memory amounts after safety margins.
type Budgets struct {
	GPU, RAM uint64
	MinCtx   int `json:"-"` // only plans with at least this context (Hermes needs 64K)
}

// ctxRung is one context size to try, with the score factor it carries.
type ctxRung struct {
	n      int
	factor float64
}

// Context sizes tried, largest first, with a small preference for room to work.
var ctxLadder = []ctxRung{{65536, 1}, {32768, 0.94}, {16384, 0.85}, {8192, 0.6}} // coding agents send ~10K-token prompts; Hermes refuses <64K. agents (Hermes, OpenCode) need ≥64K: Hermes refuses less

// MinTokPerSec is the speed below which a plan is only used as a last resort.
const MinTokPerSec = 8.0

// quantQuality estimates how much of a model's quality survives a quant,
// from published perplexity/benchmark deltas. Catalog entries can override.
func quantQuality(q catalog.Quant, m catalog.Model) float64 {
	if q.Quality > 0 {
		return q.Quality
	}
	if m.Native1Bit {
		return 1
	}
	n := normQuant(q.Name)
	switch {
	case strings.HasPrefix(n, "BF16"), strings.HasPrefix(n, "F16"), strings.HasPrefix(n, "Q8"):
		return 1
	case strings.HasPrefix(n, "Q6"):
		return 0.997
	case strings.HasPrefix(n, "Q5"):
		return 0.993
	case n == "MXFP4", strings.HasPrefix(n, "MXFP4"), strings.HasPrefix(n, "NVFP4"):
		return 0.99 // native/near-native 4-bit formats
	case strings.HasPrefix(n, "Q4_K_XL"), strings.HasPrefix(n, "Q4_K_M"), strings.HasPrefix(n, "Q4"):
		return 0.985
	case strings.HasPrefix(n, "IQ4"):
		return 0.978
	case strings.HasPrefix(n, "Q3_K_XL"), strings.HasPrefix(n, "IQ3_S"), strings.HasPrefix(n, "IQ3_M"):
		return 0.962
	case strings.HasPrefix(n, "IQ3_XXS"), strings.HasPrefix(n, "IQ3"), strings.HasPrefix(n, "Q3"):
		return 0.945
	case strings.HasPrefix(n, "Q2_K_XL"):
		return 0.92
	case strings.HasPrefix(n, "IQ2_M"), strings.HasPrefix(n, "IQ2_S"):
		return 0.9
	case strings.HasPrefix(n, "IQ2"), strings.HasPrefix(n, "Q2"):
		return 0.88
	case strings.HasPrefix(n, "IQ1_M"):
		return 0.82
	case strings.HasPrefix(n, "Q1"), strings.HasPrefix(n, "TQ"):
		return 0.8
	case strings.HasPrefix(n, "IQ1"):
		return 0.78
	}
	return 0.9 // unknown format: assume a mid-low quant rather than near-lossless
}

// normQuant strips quantizer branding so "GSQ-RCO-IQ3_XXS-mtp",
// "UD-Q4_K_XL (MTP)" and "IQ3_XXS" all compare by their base type.
func normQuant(name string) string {
	n := strings.ToUpper(name)
	for _, pre := range []string{"GSQ-RCO-", "UD-", "IMATRIX-"} {
		n = strings.TrimPrefix(n, pre)
	}
	for _, suf := range []string{" (MTP)", "-MTP"} {
		n = strings.TrimSuffix(n, suf)
	}
	return n
}

// minQuality: below this, a smaller model at a better quant is the safer pick.
// Very large MoE models degrade more gracefully, so they get a lower floor.
func minQuality(m catalog.Model) float64 {
	switch {
	case m.Native1Bit:
		return 0
	case m.ParamsB >= 150:
		return 0.85
	case m.ParamsB >= 20:
		return 0.88
	}
	return 0.94
}

var kvQuality = map[string]float64{"f16": 1, "q8_0": 0.998, "q4_0": 0.985}

// ComputeBudgets applies safety margins to what the scan found.
// availRAM lets callers ask "what if the user closed their apps".
func ComputeBudgets(h *scan.Hardware, availRAM uint64) Budgets {
	floor := osFloor(h)
	ram := sub(availRAM, margin(availRAM, 0.10, floor))
	if h.Unified {
		// GPU and CPU draw from one pool: cap at the GPU wired limit, and
		// never past what is actually free.
		g := min(h.GPUUsable, availRAM)
		g = sub(g, margin(g, 0.10, floor))
		return Budgets{GPU: g, RAM: g}
	}
	// On Linux, VRAM overflow fails the load (detected), so the margin can be
	// tight. Windows instead spills silently into shared system memory, which
	// is far slower (seen: a 27B model at 8.7 tok/s instead of ~50, then a
	// crash on a long prompt), so keep a real cushion there.
	frac, floor := 0.04, 0.4*gb
	if h.OS == "windows" {
		frac, floor = 0.08, 1.0*gb
		if h.Backend == scan.Vulkan {
			// AMD and Intel cards: what's free is added up from per-app numbers
			// rather than read from the driver, so leave more room.
			frac, floor = 0.12, 1.5*gb
		}
	}
	g := sub(h.GPUUsable, margin(h.GPUUsable, frac, floor))
	return Budgets{GPU: g, RAM: ram}
}

func margin(v uint64, frac, floor float64) uint64 { return uint64(math.Max(float64(v)*frac, floor)) }

// osFloor is the free memory always left for the OS and the user's apps:
// 2 GB, scaled down on small machines (an 8 GB laptop keeps 1 GB, a 4 GB one
// 0.75 GB) so they can still run a small model. Free memory already excludes
// what the OS and open apps use.
func osFloor(h *scan.Hardware) float64 {
	if h.RAMTotal == 0 {
		return 2 * gb
	}
	return math.Min(2*gb, math.Max(0.75*gb, float64(h.RAMTotal)/8))
}

func sub(a, b uint64) uint64 {
	if b >= a {
		return 0
	}
	return a - b
}

// overhead is llama.cpp's compute buffers and runtime, roughly.
func overhead(weights uint64, ctx int) uint64 {
	return uint64(320<<20) + uint64(ctx)*4<<10 + weights/100
}

// batchOverhead is the GPU working memory for 2048-token prompt batches
// (activations and logits) in the modes that pass -ub 2048.
const batchOverhead = uint64(1200 << 20)

// specOverhead is the extra working memory speculative decoding needs (the
// draft context and larger verify batches).
func specOverhead(ctx int) uint64 { return uint64(384<<20) + uint64(ctx)*2<<10 }

// Plans returns the best plan per model for the default preference.
func Plans(h *scan.Hardware, c *catalog.Catalog, b Budgets) []Plan {
	return PlansFor(h, c, b, Balanced)
}

// PlansFor returns the best plan per model, best first, for a preference.
func PlansFor(h *scan.Hardware, c *catalog.Catalog, b Budgets, pref Pref) []Plan {
	var out []Plan
	for _, m := range c.Models {
		if p, ok := bestForModel(h, m, b, pref); ok {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return rankBefore(out[i], out[j], pref) })
	return out
}

// usable reports whether a plan is fast enough for the preference.
func usable(p Plan, pref Pref) bool {
	if pref == Smartest {
		if p.Mode == CPU {
			return p.TokPerSec >= SmartestMinCPU
		}
		return p.TokPerSec >= SmartestMinTokPerSec
	}
	return p.Speed != "slow"
}

func rankBefore(a, b Plan, pref Pref) bool {
	ua, ub := usable(a, pref), usable(b, pref)
	if ua != ub {
		return ua
	}
	return UtilityFor(a, pref) > UtilityFor(b, pref)
}

// Utility balances intelligence against speed. Speed matters a little until
// ~40 tok/s (faster than anyone reads), so between near-identical quants the
// faster setup wins, while between different models intelligence dominates:
// halving speed from 40 to 20 tok/s costs ~8%, a different model tier is 20-40%.
func Utility(p Plan) float64 { return UtilityFor(p, Balanced) }

// UtilityFor scores a plan under a preference. Smartest ignores speed (ties
// go to the faster setup); Fastest weighs speed heavily up to ~120 tok/s.
func UtilityFor(p Plan, pref Pref) float64 {
	switch pref {
	case Smartest:
		return p.Effective + p.TokPerSec*1e-4
	case Fastest:
		return p.Effective * math.Pow(math.Min(p.TokPerSec, 120)/120, 0.6)
	}
	return p.Effective * math.Pow(math.Min(p.TokPerSec, 40)/40, 0.12)
}

// Recommend returns the best plan. Disk space doesn't disqualify a plan: the
// UI warns when the download needs more room than is free, and the user can
// make room while it downloads.
func Recommend(plans []Plan) (Plan, bool) {
	if len(plans) == 0 {
		return Plan{}, false
	}
	return plans[0], true
}

func bestForModel(h *scan.Hardware, m catalog.Model, b Budgets, pref Pref) (Plan, bool) {
	var best Plan
	found := false
	better := func(p Plan) bool { return !found || rankBefore(p, best, pref) }
	kvs := []string{"f16", "q8_0", "q4_0"}
	if m.NoKVQuant {
		kvs = []string{"f16"}
	}
	for _, q := range m.Quants {
		qq := quantQuality(q, m)
		if qq < minQuality(m) && len(m.Quants) > 1 {
			continue // (a single requested file is always considered)
		}
		ladder := ctxLadder
		if last := ctxLadder[len(ctxLadder)-1].n; m.CtxMax > 0 && m.CtxMax < last {
			// A model capped below the shortest rung (older 4K models, and new
			// catalog entries arrive from a remote URL) would otherwise never
			// get a plan at all.
			ladder = append(ladder[:len(ladder):len(ladder)], ctxRung{m.CtxMax, 0.5})
		}
		for _, c := range ladder {
			if m.CtxMax > 0 && c.n > m.CtxMax || c.n < b.MinCtx {
				continue
			}
			for _, kv := range kvs {
				p, ok := try(h, m, q, c.n, kv, b)
				if !ok {
					continue
				}
				p.Effective = m.Score * qq * kvQuality[kv] * c.factor
				if p.Stream {
					p.Effective *= 0.999 // same model; only speed differs, already counted
				}
				p.DiskOK = h.DiskFree == 0 || p.Download+2*scan.GB < h.DiskFree
				if better(p) {
					best, found = p, true
				}
				// A smaller KV cache loses a little quality, so keep the
				// largest one that fits -- unless it is too slow to use. Then
				// the freed memory can put more layers on the GPU, and a
				// smaller cache is the difference between a usable plan and
				// none (and between more memory helping or hurting).
				if usable(p, pref) {
					break
				}
			}
		}
	}
	return best, found
}

func try(h *scan.Hardware, m catalog.Model, q catalog.Quant, ctx int, kv string, b Budgets) (Plan, bool) {
	// Bytes that must be resident: lookup tables can stay on disk (lazy mode).
	w := q.Bytes
	lazy := q.LazyBytes > 0
	if lazy {
		w -= q.LazyBytes
	}
	// Speculative decoding is only kept when the whole model is on the GPU
	// (below): hybrid, split and CPU plans drop it, so they are sized
	// without the draft file and its verify batches.
	wNoSpec := w
	spec, draftBytes := specFor(h, m, q)
	w += draftBytes
	kvBytes := m.KVBytesPerToken(kv)*uint64(ctx) + m.SWABytes(kv, ctx)
	if lazy {
		// Keep the lookup tables in memory when the whole file fits: lazy
		// mode reads them back from the SSD on every token, which is only
		// worth it when the file would not fit otherwise.
		fullW := q.Bytes + draftBytes
		ohFull := overhead(fullW, ctx)
		if spec != "" {
			ohFull += specOverhead(ctx)
		}
		limit := b.RAM
		if h.Backend != scan.CPU && b.GPU > 0 {
			limit = b.GPU
		}
		if fullW+kvBytes+ohFull <= limit {
			// Keep the whole file resident. Sizing for plans that drop the
			// draft still uses the weights alone, so refresh wNoSpec too.
			lazy, w, wNoSpec = false, fullW, q.Bytes
		}
	}
	oh := overhead(w, ctx)
	if spec != "" {
		oh += specOverhead(ctx)
	}
	ohNoSpec := overhead(wNoSpec, ctx)
	p := Plan{Model: m, Quant: q, Ctx: ctx, KV: kv, GPUBudget: b.GPU, RAMBudget: b.RAM, Spec: spec, Lazy: lazy,
		Download: q.Bytes + draftBytes}

	activeFrac := 1.0
	if m.MoE && m.ParamsB > 0 && m.ActiveParamsB > 0 {
		// Active params are a share of the resident weights, not of the
		// lookup tables left on disk.
		resident := 1.0
		if lazy && q.Bytes > 0 {
			resident = float64(q.Bytes-q.LazyBytes) / float64(q.Bytes)
		}
		activeFrac = math.Min(1, m.ActiveParamsB/(m.ParamsB*resident))
	}
	activeBytes := float64(w) * activeFrac
	activeNoSpec := float64(wNoSpec) * activeFrac
	// Very sparse MoE (many small experts, <8% active) reads memory in small
	// scattered chunks and gets less of the bandwidth. Calibrated on Qwen3.6
	// 35B A3B (8% active, 69 tok/s measured) vs Qwen3.8 Flash Next (4.6%,
	// 20 tok/s on an RTX 4070 SUPER, ~30 on an RTX 4090, 54-61 on RTX Spark).
	sparsity := 1.0
	if m.MoE {
		exp := 0.5
		if h.Backend == scan.Metal {
			// Metal suffers more from scattered expert reads: M3 Max ran Flash
			// Next at 27 tok/s where bandwidth math alone says ~80.
			exp = 2.5
		}
		sparsity = math.Min(1, math.Pow(activeFrac/0.08, exp))
	}
	// Layer split sends each token through every card in turn, so bandwidths
	// add like resistors in series: the free-memory-weighted harmonic mean,
	// not the fastest card's. Equal cards come out at one card's bandwidth
	// (a second identical card buys memory, not speed); a slow second card
	// drags the plan down instead of being free.
	gpuBW, free := 0.0, uint64(0)
	var serial float64
	for _, g := range h.GPUs {
		if g.Bandwidth <= 0 || g.MemFree == 0 {
			continue
		}
		free += g.MemFree
		serial += float64(g.MemFree) / g.Bandwidth
	}
	if free > 0 && serial > 0 {
		gpuBW = float64(free) / serial
	}
	if gpuBW == 0 {
		for _, g := range h.GPUs {
			gpuBW = math.Max(gpuBW, g.Bandwidth)
		}
	}
	if gpuBW == 0 {
		gpuBW = h.CPUBandwidth
	}

	full := w + kvBytes + oh
	fullNoSpec := wNoSpec + kvBytes + ohNoSpec
	hasGPU := h.Backend != scan.CPU && b.GPU > 0

	switch {
	case hasGPU && (full <= b.GPU || fullNoSpec <= b.GPU):
		// If the draft does not fit but the model alone does, run without
		// speculative decoding rather than splitting the model.
		need, keepSpec := full, spec
		if need > b.GPU {
			need, keepSpec = fullNoSpec, ""
			p.Spec = ""
			p.Download = q.Bytes
		}
		p.Mode, p.NGL, p.GPUNeed = GPU, 999, need
		p.DirectLoad = h.OS == "windows" && !lazy
		p.TokPerSec = 0.6 * gpuBW / (activeBytes / gb)
		_ = keepSpec

	case hasGPU && !h.Unified && m.MoE && m.Layers > 0:
		// Attention, shared weights and KV stay on the GPU; whole layers of
		// experts move to system RAM until the rest fits. A catalog entry
		// without a layer count can only run fully resident (the split below
		// needs the layer size too). Speculative decoding is dropped here, so
		// the split is sized without the draft file.
		dense := uint64(activeNoSpec * 0.5)
		perLayer := float64(wNoSpec-dense) / float64(m.Layers)
		// Prompts are processed in 2048-token batches on the GPU, streaming
		// the RAM-resident experts through it: that needs room for a layer's
		// experts in flight plus the batch's activations and logits. Without
		// it the card fills up and prompt reading collapses (RTX 4070 SUPER,
		// Flash Next: 40 expert layers in RAM left 0.4 GB free and read
		// prompts at 115 tok/s; 44 layers, 3 GB free: 580 tok/s, same
		// writing speed). This matches the user's hand-tuned -ncmoe 45.
		fixed := dense + kvBytes + ohNoSpec + batchOverhead + uint64(2*perLayer)
		if fixed > b.GPU || perLayer <= 0 {
			return p, false
		}
		onGPU := min(int(float64(b.GPU-fixed)/perLayer), m.Layers)
		if onGPU >= m.Layers {
			// Without the draft the whole model can fit the card, but this
			// branch was chosen for the draft-inclusive plan and a hybrid
			// plan with nothing offloaded is a GPU plan with the wrong
			// speed model. Keep one layer's experts in RAM.
			onGPU = m.Layers - 1
		}
		nCPU := m.Layers - onGPU
		ram := uint64(float64(nCPU) * perLayer)
		if lazy {
			// Lookup-table rows read from disk still pass through the OS page
			// cache and compete with expert weights for RAM.
			ram += q.LazyBytes / 5
		}
		// Expert weights are memory-mapped (clean, evictable pages), so they
		// may exceed free RAM a little and page from SSD: slower, never a
		// freeze. KV and compute buffers never overcommit.
		// Measured on an RTX 4070 SUPER + 62 GB: Flash Next at ~1.07x free RAM
		// ran 30 tok/s (user's tuned Q2_0); at ~1.24x it crawled at 5.6.
		pageFactor := 1.0
		if ram > b.RAM {
			if float64(ram) > float64(b.RAM)*1.1 || h.RAMTotal == 0 {
				return p, false
			}
			p.Stream = true
			pageFactor = math.Pow(float64(b.RAM)/float64(ram), 6)
		}
		p.Mode, p.NGL, p.NCPUMoE = Hybrid, 999, nCPU
		p.GPUNeed, p.RAMNeed = fixed+uint64(float64(onGPU)*perLayer), min(ram, b.RAM)
		share := float64(nCPU) / float64(m.Layers)
		experts := activeNoSpec * 0.6
		t := (activeNoSpec-experts*share)/gb/gpuBW + experts*share/gb/h.CPUBandwidth
		// Calibrated on an RTX 4070 SUPER + DDR5 desktop: Qwen3.6 35B A3B
		// UD-Q4_K_M measured 69 tok/s with experts in RAM.
		p.TokPerSec = 0.95 * pageFactor / t

	case hasGPU && !h.Unified && m.Layers > 0:
		// Dense model larger than VRAM: offload as many layers as fit.
		perLayer := float64(wNoSpec) / float64(m.Layers)
		ohSplit := ohNoSpec + batchOverhead // 2048-token prompt batches, as in hybrid
		if kvBytes+ohSplit >= b.GPU {
			return p, false
		}
		onGPU := min(int(float64(b.GPU-kvBytes-ohSplit)/perLayer), m.Layers)
		ram := uint64(float64(m.Layers-onGPU) * perLayer)
		if onGPU < 1 || onGPU >= m.Layers || onGPU < m.Layers/3 || ram > b.RAM {
			return p, false // one layer must actually reach the card, or the plan is a slow CPU load
		}
		p.Mode, p.NGL = Split, onGPU
		p.GPUNeed, p.RAMNeed = kvBytes+ohSplit+uint64(float64(onGPU)*perLayer), ram
		share := float64(m.Layers-onGPU) / float64(m.Layers)
		t := activeNoSpec*(1-share)/gb/gpuBW + activeNoSpec*share/gb/h.CPUBandwidth
		p.TokPerSec = 0.45 / t

	case !hasGPU:
		if fullNoSpec > b.RAM {
			return p, false
		}
		p.Mode, p.NGL, p.RAMNeed = CPU, 0, fullNoSpec
		p.TokPerSec = 0.5 * h.CPUBandwidth / (activeNoSpec / gb)

	default:
		return p, false
	}

	p.TokPerSec *= sparsity
	if p.Mode != GPU && h.PhysCores > 0 {
		p.Threads = h.PhysCores
	}

	// Speculative decoding: measured ~1.3-2.2x on dense models with NVIDIA,
	// much less on MoE and nothing on Metal. With experts in system RAM the
	// draft costs more than it saves (the user's tuned Flash Next runs none),
	// so hybrid and CPU plans skip it.
	if spec != "" && (p.Mode == Hybrid || p.Mode == CPU || p.Mode == Split) {
		spec, p.Spec = "", ""
		p.Download = q.Bytes
	}
	if spec != "" {
		if m.MoE {
			p.TokPerSec *= 1.15
		} else {
			p.TokPerSec *= 1.5
		}
	}
	// Classify on the real rate: rounding 7.96 up to 8.0 for display must not
	// turn a plan below the floor into a usable one.
	tps := p.TokPerSec
	p.TokPerSec = math.Round(tps*10) / 10
	switch {
	case tps >= 30:
		p.Speed = "fast"
	case tps >= MinTokPerSec:
		p.Speed = "good"
	default:
		p.Speed = "slow"
	}
	return p, true
}

// specFor picks speculative decoding when it helps on this hardware. MTP and
// draft models were a loss or a wash on Apple Silicon in community sweeps, so
// only CUDA gets them by default.
func specFor(h *scan.Hardware, m catalog.Model, q catalog.Quant) (string, uint64) {
	if h.Backend != scan.CUDA {
		return "", 0
	}
	if q.MTP {
		return "draft-mtp", 0
	}
	if m.Draft != nil {
		switch m.Draft.Type { // only types llama-server accepts
		case "draft-mtp", "draft-dflash", "draft-dspark", "draft-eagle3":
			return m.Draft.Type, m.Draft.Bytes
		}
	}
	return "", 0
}

// Args are the recipe flags for a plan, beyond the basics serve always sets.
func (p Plan) Args() []string {
	var a []string
	if p.KV != "f16" && p.KV != "" {
		a = append(a, "-ctk", p.KV, "-ctv", p.KV) // matched pair: mixed types fall back to slow paths
	}
	if p.Spec != "" {
		n := "2"
		if !p.Model.MoE && p.GPUBudget > 24*scan.GB {
			n = "3"
		}
		a = append(a, "--spec-type", p.Spec, "--spec-draft-n-max", n)
	}
	if p.Lazy {
		a = append(a, "-lzm", "on")
	}
	if p.DirectLoad {
		a = append(a, "-lm", "none")
	}
	if p.Threads > 0 {
		a = append(a, "-t", fmt.Sprint(p.Threads))
	}
	if p.Mode == Hybrid || p.Mode == Split {
		// Both placements budget for 2048-token prompt batches (batchOverhead
		// in try), so the flags must ask for them: much faster prompt reading,
		// and the graphics-memory plan matches the run.
		a = append(a, "-ub", "2048", "-b", "2048")
	}
	return append(a, p.Quant.Args...)
}

// Recipe describes the tuning in plain words, for the UI and logs.
func (p Plan) Recipe() string {
	var parts []string
	parts = append(parts, p.Quant.Name)
	switch p.Mode {
	case GPU:
		parts = append(parts, "all on the graphics chip")
	case Hybrid:
		parts = append(parts, fmt.Sprintf("experts of %d/%d layers in system memory", p.NCPUMoE, p.Model.Layers))
	case Split:
		parts = append(parts, fmt.Sprintf("%d/%d layers on the graphics card", p.NGL, p.Model.Layers))
	case CPU:
		parts = append(parts, "processor only")
	}
	if p.KV != "f16" {
		parts = append(parts, p.KV+" KV cache")
	}
	if p.Spec != "" {
		parts = append(parts, map[string]string{"draft-mtp": "MTP speculative decoding", "draft-dflash": "DFlash draft model", "draft-dspark": "DSpark draft model"}[p.Spec])
	}
	if p.Lazy {
		parts = append(parts, "lookup tables read from SSD")
	}
	if p.Stream {
		parts = append(parts, "some experts paged from SSD")
	}
	return strings.Join(parts, ", ")
}

// PlanForQuant is the best plan for one specific file of a model (used when
// switching to a model that's already downloaded). ok is false if it can't run
// on this machine right now.
func PlanForQuant(h *scan.Hardware, m catalog.Model, q catalog.Quant, b Budgets, pref Pref) (Plan, bool) {
	one := m
	one.Quants = []catalog.Quant{q}
	return bestForModel(h, one, b, pref)
}
