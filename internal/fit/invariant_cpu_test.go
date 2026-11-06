package fit

// a12: the CPU fallback invariant.
//
// A plan may put the whole model on the processor only when the processor can
// hold it: weights, KV and sliding-window caches and the llama.cpp runtime
// must fit the RAM budget after the safety margin. And it must not be picked
// while a plan that uses the graphics card would fit instead.
//
// The planner reaches CPU mode when it has no usable GPU: a processor-only
// machine, or a GPU machine whose free VRAM after the safety margin is zero
// (the card is busy with another app). The profile space below generates
// both shapes, plus forced-CPU builds, unified memory and odd scans.
//
// The tests check the plan's own numbers against an independent recount, so
// a mistake in try's arithmetic cannot hide behind Plan.RAMNeed.

import (
	"fmt"
	"testing"

	"github.com/picom-code/ezlm/internal/scan"
)

type a12Profile struct {
	name string
	hw   *scan.Hardware
}

// a12Base builds a machine with no GPU yet; a12GPU adds one.
func a12Base(backend scan.Backend, ramTotalGB, ramAvailGB uint64) *scan.Hardware {
	return &scan.Hardware{
		OS: "linux", Arch: "amd64", Cores: 16, PhysCores: 8,
		RAMTotal: ramTotalGB * G, RAMAvail: ramAvailGB * G,
		DiskFree: 2000 * G, CPUBandwidth: 60, Backend: backend,
	}
}

func a12GPU(hw *scan.Hardware, vendor, name string, freeGB, totalGB uint64, bw float64) *scan.Hardware {
	hw.GPUs = append(hw.GPUs, scan.GPU{Name: name, Vendor: vendor, MemFree: freeGB * G, MemTotal: totalGB * G, Bandwidth: bw})
	hw.GPUUsable += freeGB * G
	return hw
}

// a12Unified builds an Apple-like machine: the GPU may wire a share of RAM.
func a12Unified(ramTotalGB, ramAvailGB uint64, bw float64) *scan.Hardware {
	usable := uint64(float64(ramTotalGB*G) * 0.67)
	if ramTotalGB > 36 {
		usable = uint64(float64(ramTotalGB*G) * 0.75)
	}
	return &scan.Hardware{OS: "darwin", Arch: "arm64", Cores: 12, PhysCores: 8, Backend: scan.Metal, Unified: true,
		RAMTotal: ramTotalGB * G, RAMAvail: ramAvailGB * G, GPUUsable: usable, DiskFree: 1000 * G, CPUBandwidth: bw,
		GPUs: []scan.GPU{{Vendor: "apple", MemTotal: usable, MemFree: min(usable, ramAvailGB*G), Bandwidth: bw}}}
}

// a12Profiles is the generated profile space. It is a deterministic grid, not
// a random sample, so a failure names the same machine every time.
func a12Profiles() []a12Profile {
	var ps []a12Profile
	add := func(name string, hw *scan.Hardware) { ps = append(ps, a12Profile{name, hw}) }

	// Processor-only machines across the RAM sizes beta testers have.
	for _, r := range []struct{ total, avail uint64 }{{4, 2}, {4, 3}, {8, 4}, {8, 6}, {16, 9}, {16, 12}, {32, 24}, {64, 48}, {128, 100}, {256, 230}, {1536, 1450}} {
		add(fmt.Sprintf("cpu-only %dGB RAM (%dGB free)", r.total, r.avail), a12Base(scan.CPU, r.total, r.avail))
	}
	// A forced processor-only build (EZLM_BACKEND=cpu) still sees the card in
	// the scan; the planner must not plan GPU offload for it.
	add("cpu backend, 32GB RAM, RTX 4090 present", a12GPU(a12Base(scan.CPU, 32, 24), "nvidia", "RTX 4090", 23, 24, 1008))
	// A GPU machine whose card is fully occupied by another app: no usable
	// VRAM, so the CPU fallback is the right answer where RAM fits.
	add("cuda, 32GB RAM, 8GB card with 0GB free", a12GPU(a12Base(scan.CUDA, 32, 24), "nvidia", "RTX 4060", 0, 8, 272))
	add("cuda, 64GB RAM, 24GB card with 0GB free", a12GPU(a12Base(scan.CUDA, 64, 48), "nvidia", "RTX 4090", 0, 24, 1008))
	// Windows CUDA leaves a 1GB floor: 0.5GB free means no GPU budget at all.
	add("windows cuda, 32GB RAM, 0.5GB free of 8GB", a12GPU(&scan.Hardware{OS: "windows", Arch: "amd64", Cores: 16, PhysCores: 8, RAMTotal: 32 * G, RAMAvail: 24 * G, DiskFree: 1000 * G, CPUBandwidth: 60, Backend: scan.CUDA}, "nvidia", "RTX 4060", 0, 8, 272))
	// Windows Vulkan keeps a 1.5GB floor: 1.4GB free leaves no GPU budget.
	// This is the AMD/Intel "card is busy" shape.
	add("windows vulkan, 32GB RAM, 1.4GB free of 16GB", a12GPU(&scan.Hardware{OS: "windows", Arch: "amd64", Cores: 16, PhysCores: 8, RAMTotal: 32 * G, RAMAvail: 24 * G, DiskFree: 1000 * G, CPUBandwidth: 60, Backend: scan.Vulkan}, "amd", "Radeon RX 7800 XT", 1, 16, 624))
	// Normal GPU machines, so the same profile space also proves no CPU plan
	// appears while the card has room.
	for _, g := range []struct {
		backend scan.Backend
		os      string
		vendor  string
		name    string
		free    uint64
		total   uint64
		bw      float64
		ram     uint64
	}{
		{scan.CUDA, "linux", "nvidia", "RTX 3060", 11, 12, 360, 32},
		{scan.CUDA, "linux", "nvidia", "RTX 4070 SUPER", 11, 12, 504, 62},
		{scan.CUDA, "linux", "nvidia", "RTX 4090", 23, 24, 1008, 96},
		{scan.CUDA, "windows", "nvidia", "RTX 5090", 31, 32, 1792, 128},
		{scan.Vulkan, "windows", "amd", "Radeon RX 7800 XT", 15, 16, 624, 32},
		{scan.Vulkan, "linux", "amd", "Radeon RX 7900 XTX", 23, 24, 960, 64},
		{scan.Vulkan, "windows", "intel", "Arc B580", 11, 12, 456, 32},
		{scan.ROCm, "linux", "amd", "Radeon VII", 15, 16, 1024, 64},
		{scan.CUDA, "linux", "nvidia", "H200", 141, 141, 4800, 512},
	} {
		hw := a12Base(g.backend, g.ram, g.ram*3/4)
		hw.OS = g.os
		add(fmt.Sprintf("%s %s %dGB card, %dGB RAM", g.backend, g.name, g.free, g.ram),
			a12GPU(hw, g.vendor, g.name, g.free, g.total, g.bw))
	}
	// Unified memory machines: GPU and CPU draw from one pool, so a plan is
	// never "CPU only" there, but the RAM budget must still be respected.
	for _, r := range []struct{ total, avail uint64 }{{8, 4}, {16, 10}, {24, 16}, {64, 50}, {128, 110}, {192, 170}, {512, 470}} {
		add(fmt.Sprintf("unified %dGB (%dGB free)", r.total, r.avail), a12Unified(r.total, r.avail, 400))
	}
	// Odd scans: no free memory, more free than total, no total at all, a
	// unified machine with no wired limit, and a GPU bigger than the RAM.
	add("cpu-only 16GB RAM with 0GB free", a12Base(scan.CPU, 16, 0))
	add("cpu-only, free > total", a12Base(scan.CPU, 16, 24))
	add("cpu-only, total unknown", a12Base(scan.CPU, 0, 8))
	add("cuda, 16GB RAM, free > total", a12GPU(a12Base(scan.CUDA, 16, 24), "nvidia", "RTX 4060", 8, 8, 272))
	add("cuda, GPU bigger than RAM", a12GPU(a12Base(scan.CUDA, 16, 12), "nvidia", "RTX PRO 6000", 94, 96, 1792))
	add("unified with no wired limit", func() *scan.Hardware {
		hw := a12Unified(64, 50, 546)
		hw.GPUUsable = 0
		return hw
	}())
	return ps
}

// a12CPUNeed recounts what a CPU plan actually keeps in RAM, from the plan
// alone: the quant file (minus lookup tables left on disk), the KV and
// sliding-window caches, and the runtime overhead. CPU plans never use
// speculative decoding, so no draft file is added.
func a12CPUNeed(p Plan) uint64 {
	w := p.Quant.Bytes
	if p.Lazy && p.Quant.LazyBytes > 0 {
		w -= p.Quant.LazyBytes
	}
	kv := p.Model.KVBytesPerToken(p.KV)*uint64(p.Ctx) + p.Model.SWABytes(p.KV, p.Ctx)
	return w + kv + overhead(w, p.Ctx)
}

// TestCPUPlanFitsAndIsNotPickedOverAGPU is the invariant over the generated
// profile space, for every model and every preference.
func TestCPUPlanFitsAndIsNotPickedOverAGPU(t *testing.T) {
	c := cat(t)
	cpuPlans := 0
	for _, prof := range a12Profiles() {
		b := ComputeBudgets(prof.hw, prof.hw.RAMAvail)
		for _, pref := range []Pref{Balanced, Smartest, Fastest} {
			for _, p := range PlansFor(prof.hw, c, b, pref) {
				if p.Mode != CPU {
					if prof.hw.Backend == scan.CPU {
						t.Errorf("%s/%s: %s planned on the graphics card (%s) on a processor-only machine", prof.name, pref, p.Model.ID, p.Mode)
					}
					continue
				}
				cpuPlans++
				need := a12CPUNeed(p)
				if need > b.RAM {
					t.Errorf("%s/%s: CPU plan for %s needs %d bytes, RAM budget is %d", prof.name, pref, p.Model.ID, need, b.RAM)
				}
				if p.RAMNeed != need {
					t.Errorf("%s/%s: CPU plan for %s reports RAMNeed %d, actual need is %d (draft bytes counted?)", prof.name, pref, p.Model.ID, p.RAMNeed, need)
				}
				if p.RAMNeed > b.RAM {
					t.Errorf("%s/%s: CPU plan for %s reports %d bytes, RAM budget is %d", prof.name, pref, p.Model.ID, p.RAMNeed, b.RAM)
				}
				if p.GPUNeed != 0 || p.NGL != 0 || p.NCPUMoE != 0 {
					t.Errorf("%s/%s: CPU plan for %s reserves the GPU (gpu_need=%d ngl=%d ncpumoe=%d)", prof.name, pref, p.Model.ID, p.GPUNeed, p.NGL, p.NCPUMoE)
				}
				if prof.hw.Backend != scan.CPU && b.GPU > 0 {
					t.Errorf("%s/%s: CPU plan for %s although the GPU budget is %d bytes", prof.name, pref, p.Model.ID, b.GPU)
				}
				if prof.hw.Backend != scan.CPU && need <= b.GPU {
					t.Errorf("%s/%s: CPU plan for %s although a GPU plan would fit (%d <= %d)", prof.name, pref, p.Model.ID, need, b.GPU)
				}
			}
		}
	}
	if cpuPlans == 0 {
		t.Fatal("profile space produced no CPU plan at all; the invariant is not exercised")
	}
	t.Logf("%d CPU plans checked over %d profiles", cpuPlans, len(a12Profiles()))
}

// TestCPUPlanIgnoresDraftBytes: a CUDA machine whose card is busy has no GPU
// budget, so the processor fallback is used. A model with a separate
// speculative draft file must be sized without that draft: CPU plans drop
// speculative decoding (try clears Plan.Spec), so the file is never loaded.
// Counting it both overstates RAMNeed and rejects models that do fit.
func TestCPUPlanIgnoresDraftBytes(t *testing.T) {
	c := cat(t)
	m, ok := c.Find("qwen3.8-27b") // has a 3.16 GB MTP draft file
	if !ok || m.Draft == nil {
		t.Skip("qwen3.8-27b with a draft file is not in the catalog")
	}
	q := m.Quants[0]
	for _, x := range m.Quants {
		if x.Bytes < q.Bytes {
			q = x
		}
	}
	if qq := quantQuality(q, m); qq < minQuality(m) {
		t.Skipf("smallest quant %s is below the quality floor", q.Name)
	}
	const ctx = 8192
	w := q.Bytes
	need := w + m.KVBytesPerToken("f16")*ctx + m.SWABytes("f16", ctx) + overhead(w, ctx)
	if need+m.Draft.Bytes <= need {
		t.Fatal("draft file has no size")
	}

	// Windows CUDA with a 1GB safety floor and a card fully used by another
	// app: the GPU budget is zero, so this machine must get CPU plans.
	hw := &scan.Hardware{OS: "windows", Arch: "amd64", Cores: 16, PhysCores: 8, Backend: scan.CUDA,
		RAMTotal: 32 * G, DiskFree: 1000 * G, CPUBandwidth: 60}
	hw = a12GPU(hw, "nvidia", "RTX 4060", 0, 8, 272)
	// Leave room for the weights but not for the weights plus the draft.
	hw.RAMAvail = need + 2*G + G/2 // osFloor is 2GB on a 32GB machine
	b := ComputeBudgets(hw, hw.RAMAvail)
	if b.GPU != 0 {
		t.Fatalf("GPU budget = %d, want 0", b.GPU)
	}
	if need > b.RAM || need+m.Draft.Bytes <= b.RAM {
		t.Fatalf("profile is not on the boundary: need %d, need+draft %d, RAM budget %d", need, need+m.Draft.Bytes, b.RAM)
	}

	var got *Plan
	for _, p := range PlansFor(hw, c, b, Smartest) {
		if p.Model.ID == m.ID {
			p := p
			got = &p
		}
	}
	if got == nil {
		t.Fatalf("%s fits the processor (%d of %d bytes) but got no plan", m.ID, need, b.RAM)
	}
	if got.Mode != CPU {
		t.Fatalf("%s planned as %s, want CPU", m.ID, got.Mode)
	}
	if got.Spec != "" {
		t.Fatalf("CPU plan for %s keeps speculative decoding %q", m.ID, got.Spec)
	}
	if want := a12CPUNeed(*got); got.RAMNeed != want {
		t.Fatalf("CPU plan for %s reports RAMNeed %d, actual need is %d (draft file counted)", m.ID, got.RAMNeed, want)
	}
	if got.RAMNeed > b.RAM {
		t.Fatalf("CPU plan for %s needs %d, RAM budget is %d", m.ID, got.RAMNeed, b.RAM)
	}
	t.Logf("%s %s on the processor: ctx %d, %.1f GB of %.1f GB RAM, %.1f tok/s",
		m.ID, got.Quant.Name, got.Ctx, float64(got.RAMNeed)/float64(G), float64(b.RAM)/float64(G), got.TokPerSec)
}

// TestCPUPlansAreNotPickedWhileAGPUPlanFits pins the second half of the
// invariant on the machines where both are conceivable: a GPU with a tiny
// budget must still not produce CPU plans if any model would fit the card.
func TestCPUPlansAreNotPickedWhileAGPUPlanFits(t *testing.T) {
	c := cat(t)
	for _, prof := range a12Profiles() {
		b := ComputeBudgets(prof.hw, prof.hw.RAMAvail)
		if prof.hw.Backend == scan.CPU || b.GPU == 0 {
			continue
		}
		for _, pref := range []Pref{Balanced, Smartest, Fastest} {
			for _, p := range PlansFor(prof.hw, c, b, pref) {
				if p.Mode == CPU {
					t.Errorf("%s/%s: CPU plan for %s while the GPU budget is %.1f GB", prof.name, pref, p.Model.ID, float64(b.GPU)/float64(G))
				}
			}
		}
	}
}
