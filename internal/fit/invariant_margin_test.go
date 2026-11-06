package fit

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/picom-code/ezlm/internal/scan"
)

// The safety margin invariant: a plan may never need more memory than the
// budget left after ComputeBudgets' documented margins. This file proves it
// over a generated profile space, not just a handful of hand-picked machines.

// marginProfiles generates hardware in the shapes scan.Scan produces: unified
// Apple machines, discrete NVIDIA/AMD/Intel cards (including several cards and
// cards partly used by another app), and processor-only laptops and servers.
// Zero and absurd values are included on purpose: a scan can miss fields.
func marginProfiles(r *rand.Rand, n int) []*scan.Hardware {
	out := make([]*scan.Hardware, 0, n+8)
	gbs := func(v float64) uint64 { return uint64(math.Max(0, v) * float64(scan.GB)) }
	pick := func(xs ...float64) float64 { return xs[r.Intn(len(xs))] }

	for i := 0; i < n; i++ {
		ramTotal := gbs(pick(4, 8, 12, 16, 24, 32, 48, 64, 96, 128, 192, 256, 512, 1536))
		ramAvail := uint64(float64(ramTotal) * (0.2 + 0.75*r.Float64()))
		h := &scan.Hardware{
			OS:           []string{"linux", "windows", "darwin"}[r.Intn(3)],
			RAMTotal:     ramTotal,
			RAMAvail:     ramAvail,
			DiskFree:     gbs(pick(0, 20, 100, 500, 2000)),
			CPUBandwidth: pick(0, 17, 34, 50, 60, 100, 200, 460),
			PhysCores:    []int{0, 2, 4, 6, 8, 16, 32, 96}[r.Intn(8)],
		}
		switch r.Intn(10) {
		case 0, 1, 2: // processor only
			h.Backend = scan.CPU
		case 3, 4: // Apple Silicon, unified
			h.Backend = scan.Metal
			h.Unified = true
			h.OS = "darwin"
			h.GPUUsable = uint64(float64(ramTotal) * pick(0.5, 0.67, 0.75))
			h.GPUs = []scan.GPU{{Vendor: "apple", MemTotal: ramTotal, MemFree: h.GPUUsable, Bandwidth: pick(68, 150, 273, 546, 819)}}
		default: // one or more discrete cards
			h.Backend = []scan.Backend{scan.CUDA, scan.Vulkan, scan.ROCm}[r.Intn(3)]
			if h.Backend == scan.CUDA {
				h.OS = []string{"linux", "windows"}[r.Intn(2)]
			}
			cards := []int{1, 1, 1, 2, 3}[r.Intn(5)]
			for c := 0; c < cards; c++ {
				total := gbs(pick(4, 6, 8, 11, 12, 16, 24, 32, 48, 80, 96, 141))
				// A card can be partly occupied by another app.
				free := uint64(float64(total) * pick(0.0, 0.3, 0.7, 0.9, 1.0))
				h.GPUs = append(h.GPUs, scan.GPU{
					Vendor:   []string{"nvidia", "amd", "intel"}[r.Intn(3)],
					MemTotal: total, MemFree: free,
					Bandwidth: pick(0, 128, 192, 360, 504, 1008, 1792, 4800),
				})
				h.GPUUsable += free
			}
		}
		out = append(out, h)
	}

	// Edge values a scan can produce when a probe fails or a machine is tiny.
	out = append(out,
		&scan.Hardware{Backend: scan.CPU},
		&scan.Hardware{Backend: scan.CUDA},
		&scan.Hardware{Backend: scan.Metal, Unified: true},
		&scan.Hardware{Backend: scan.CUDA, RAMTotal: scan.GB, RAMAvail: 0, GPUUsable: 0},
		&scan.Hardware{Backend: scan.CPU, RAMTotal: 1 << 20, RAMAvail: 1 << 20},
		&scan.Hardware{Backend: scan.CUDA, RAMTotal: 1 << 40, RAMAvail: 1 << 40, GPUUsable: 1 << 40,
			GPUs: []scan.GPU{{Vendor: "nvidia", MemTotal: 1 << 40, MemFree: 1 << 40, Bandwidth: 1e6}}},
	)
	return out
}

// documentedRAMMargin is ComputeBudgets' rule for system memory: 10% of what
// is free, but at least osFloor(h). The result never goes below zero.
func documentedRAMMargin(h *scan.Hardware, avail uint64) uint64 {
	m := margin(avail, 0.10, osFloor(h))
	if m > avail {
		return avail
	}
	return m
}

// documentedGPUMargin is the discrete-GPU rule: 4% of usable VRAM with a
// 0.4 GB floor, 8%/1 GB on Windows, 12%/1.5 GB for Windows Vulkan.
func documentedGPUMargin(h *scan.Hardware) uint64 {
	frac, floor := 0.04, 0.4*gb
	if h.OS == "windows" {
		frac, floor = 0.08, 1.0*gb
		if h.Backend == scan.Vulkan {
			frac, floor = 0.12, 1.5*gb
		}
	}
	m := margin(h.GPUUsable, frac, floor)
	if m > h.GPUUsable {
		return h.GPUUsable
	}
	return m
}

// TestMarginBudgets checks ComputeBudgets leaves exactly the documented
// margin, never more memory than the machine actually has.
func TestMarginBudgets(t *testing.T) {
	r := rand.New(rand.NewSource(20261106))
	for i, h := range marginProfiles(r, 200) {
		for _, avail := range []uint64{h.RAMAvail, h.RAMTotal} {
			b := ComputeBudgets(h, avail)
			if b.RAM > avail {
				t.Errorf("profile %d (%s): RAM budget %d > available %d", i, h.Backend, b.RAM, avail)
			}
			wantRAM := avail - documentedRAMMargin(h, avail)
			if h.Unified {
				// Unified memory is one pool: capped at the wired limit and
				// at what is free, then the same 10% margin applies.
				g := h.GPUUsable
				if avail < g {
					g = avail
				}
				wantRAM = g - documentedRAMMargin(h, g)
			}
			if got, want := b.RAM, wantRAM; got != want {
				t.Errorf("profile %d (%s): RAM budget %d, want %d (margin %d)", i, h.Backend, got, want, documentedRAMMargin(h, avail))
			}
			if h.Unified {
				if b.GPU != b.RAM {
					t.Errorf("profile %d: unified GPU budget %d != RAM budget %d", i, b.GPU, b.RAM)
				}
				continue
			}
			if b.GPU > h.GPUUsable {
				t.Errorf("profile %d (%s): GPU budget %d > usable %d", i, h.Backend, b.GPU, h.GPUUsable)
			}
			if got, want := b.GPU, h.GPUUsable-documentedGPUMargin(h); got != want {
				t.Errorf("profile %d (%s): GPU budget %d, want %d (margin %d)", i, h.Backend, got, want, documentedGPUMargin(h))
			}
		}
	}
}

// TestMarginPlansNeverExceedBudgets is the core invariant: every plan the
// planner emits, for every preference, stays inside the budget it was given.
func TestMarginPlansNeverExceedBudgets(t *testing.T) {
	c := cat(t)
	r := rand.New(rand.NewSource(20261106))
	profiles := marginProfiles(r, 200)
	checked := 0
	for i, h := range profiles {
		for _, avail := range []uint64{h.RAMAvail, h.RAMTotal} {
			b := ComputeBudgets(h, avail)
			for _, pref := range []Pref{Balanced, Smartest, Fastest} {
				for _, p := range PlansFor(h, c, b, pref) {
					checked++
					if p.GPUBudget != b.GPU || p.RAMBudget != b.RAM {
						t.Errorf("profile %d (%s) %s/%s: plan budgets gpu=%d ram=%d, want %d/%d",
							i, h.Backend, p.Model.ID, pref, p.GPUBudget, p.RAMBudget, b.GPU, b.RAM)
					}
					if p.GPUNeed > b.GPU {
						t.Errorf("profile %d (%s, %s) %s/%s %s: GPU need %d > budget %d (margin violated)",
							i, h.Backend, h.OS, p.Model.ID, pref, p.Mode, p.GPUNeed, b.GPU)
					}
					if p.RAMNeed > b.RAM {
						t.Errorf("profile %d (%s, %s) %s/%s %s: RAM need %d > budget %d (margin violated)",
							i, h.Backend, h.OS, p.Model.ID, pref, p.Mode, p.RAMNeed, b.RAM)
					}
					// Needs must also fit the machine's real memory, not just the budget.
					if p.GPUNeed > h.GPUUsable && !h.Unified {
						t.Errorf("profile %d (%s): %s/%s GPU need %d > usable %d", i, h.Backend, p.Model.ID, pref, p.GPUNeed, h.GPUUsable)
					}
					if p.RAMNeed > avail {
						t.Errorf("profile %d (%s): %s/%s RAM need %d > available %d", i, h.Backend, p.Model.ID, pref, p.RAMNeed, avail)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no plans generated; the profile space or catalog is empty")
	}
	t.Logf("checked %d plans over %d profiles", checked, len(profiles))
}

// TestMarginStreamStaysNearBudget: the one documented exception is MoE expert
// weights paged from SSD. try() lets those reach 1.1x the RAM budget because
// they are clean, evictable pages; anything past that is a bug.
func TestMarginStreamStaysNearBudget(t *testing.T) {
	c := cat(t)
	r := rand.New(rand.NewSource(20261106))
	streams := 0
	var streamedNeed, streamedBudget uint64
	for i, h := range marginProfiles(r, 200) {
		b := ComputeBudgets(h, h.RAMAvail)
		for _, pref := range []Pref{Balanced, Smartest, Fastest} {
			for _, p := range PlansFor(h, c, b, pref) {
				if !p.Stream {
					continue
				}
				streams++
				if p.Mode != Hybrid {
					t.Errorf("profile %d %s: Stream set on a %s plan", i, p.Model.ID, p.Mode)
				}
				if p.RAMBudget == 0 {
					t.Errorf("profile %d %s: streamed plan with a zero RAM budget", i, p.Model.ID)
				}
				// RAMNeed is clamped to the budget. Recompute the expert
				// bytes from the plan so the clamp cannot hide a plan that
				// is far past the documented 1.1x paging cap.
				// RAMNeed is deliberately clamped to the budget, so the
				// recomputation is checked against the documented cap, not
				// against RAMNeed.
				need := hybridRAMNeed(p)
				if float64(need) > float64(p.RAMBudget)*1.1 {
					t.Errorf("profile %d %s: streamed expert bytes %d > 1.1x budget %d", i, p.Model.ID, need, p.RAMBudget)
				}
				streamedNeed += need
				streamedBudget += p.RAMBudget
			}
		}
	}
	t.Logf("checked %d streamed plans (%.1f GB expert bytes over %.1f GB budget)", streams,
		float64(streamedNeed)/float64(scan.GB), float64(streamedBudget)/float64(scan.GB))
}

// TestMarginLargestPlan checks the worst case directly: for every generated
// profile, the recommended plan (the one EZLM actually deploys) is inside the
// budget, and the budget is inside the machine.
func TestMarginLargestPlan(t *testing.T) {
	c := cat(t)
	r := rand.New(rand.NewSource(7))
	for i, h := range marginProfiles(r, 200) {
		b := ComputeBudgets(h, h.RAMAvail)
		for _, pref := range []Pref{Balanced, Smartest, Fastest} {
			p, ok := Recommend(PlansFor(h, c, b, pref))
			if !ok {
				continue
			}
			if p.GPUNeed > b.GPU || p.RAMNeed > b.RAM {
				t.Errorf("profile %d (%s %s, gpu %.1fG free, ram %.1fG free) %s: %s needs gpu %.1fG/%.1fG ram %.1fG/%.1fG",
					i, h.OS, h.Backend, float64(h.GPUUsable)/float64(scan.GB), float64(h.RAMAvail)/float64(scan.GB),
					pref, p.Model.ID, float64(p.GPUNeed)/float64(scan.GB), float64(b.GPU)/float64(scan.GB),
					float64(p.RAMNeed)/float64(scan.GB), float64(b.RAM)/float64(scan.GB))
			}
		}
	}
}

// formatProfile is used in failure messages from the property tests.
func formatProfile(h *scan.Hardware) string {
	return fmt.Sprintf("os=%s backend=%s unified=%v ram=%d/%d gpu_usable=%d gpus=%d",
		h.OS, h.Backend, h.Unified, h.RAMAvail, h.RAMTotal, h.GPUUsable, len(h.GPUs))
}

// hybridRAMNeed recomputes the system-RAM bytes a hybrid plan's offloaded
// experts need, following try()'s math, so tests can check the real number
// instead of the clamped RAMNeed.
func hybridRAMNeed(p Plan) uint64 {
	if p.Mode != Hybrid || p.Model.Layers <= 0 {
		return 0
	}
	w := p.Quant.Bytes
	if p.Lazy {
		w -= p.Quant.LazyBytes
	}
	// try() adds the draft bytes to w before choosing the mode; hybrid plans
	// then drop speculative decoding but keep the bytes in the math.
	// (specFor is CUDA-only and a draft's bytes are not on Plan, so a
	// draft-bearing model's recomputation is a lower bound.)
	activeFrac := 1.0
	if p.Model.MoE && p.Model.ParamsB > 0 && p.Model.ActiveParamsB > 0 {
		resident := 1.0
		if p.Lazy && p.Quant.Bytes > 0 {
			resident = float64(p.Quant.Bytes-p.Quant.LazyBytes) / float64(p.Quant.Bytes)
		}
		activeFrac = math.Min(1, p.Model.ActiveParamsB/(p.Model.ParamsB*resident))
	}
	dense := uint64(float64(w) * activeFrac * 0.5)
	perLayer := float64(w-dense) / float64(p.Model.Layers)
	ram := uint64(float64(p.NCPUMoE) * perLayer)
	if p.Lazy {
		ram += p.Quant.LazyBytes / 5
	}
	return ram
}

// TestMarginHybridIgnoresUnusedDraft: hybrid plans drop speculative decoding
// (try() clears p.Spec), so the draft file's bytes must not stay in the
// plan's memory need. Counting them overstates GPUNeed/RAMNeed by 0.5-3.5 GB
// and can reject a model that would fit.
func TestMarginHybridIgnoresUnusedDraft(t *testing.T) {
	c := cat(t)
	hw := nv("RTX 4090", 23, 24, 80, 96, 1008)
	b := ComputeBudgets(hw, hw.RAMAvail)
	tried := 0
	for _, m := range c.Models {
		if m.Draft == nil {
			continue
		}
		noDraft := m
		noDraft.Draft = nil
		for _, q := range m.Quants {
			for _, ctx := range []int{32768, 16384, 8192} {
				p, ok := try(hw, m, q, ctx, "f16", b)
				if !ok || p.Mode != Hybrid {
					continue
				}
				p2, ok2 := try(hw, noDraft, q, ctx, "f16", b)
				if !ok2 || p2.Mode != Hybrid {
					// Dropping the draft can legitimately make the model
					// fully GPU-resident; only hybrid layouts are comparable.
					continue
				}
				tried++
				if p.GPUNeed != p2.GPUNeed || p.RAMNeed != p2.RAMNeed {
					t.Errorf("%s %s ctx=%d: hybrid plan reserves unused draft bytes: gpu %d vs %d, ram %d vs %d",
						m.ID, q.Name, ctx, p.GPUNeed, p2.GPUNeed, p.RAMNeed, p2.RAMNeed)
				}
			}
		}
	}
	if tried == 0 {
		t.Fatal("no hybrid plan for a draft-bearing model; profile needs updating")
	}
	t.Logf("checked %d hybrid plans", tried)
}
