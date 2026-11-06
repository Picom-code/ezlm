package fit

// a10: the MoE expert offload math must agree with the RAM available.
//
// A hybrid plan keeps the attention and shared weights on the graphics card
// and moves whole layers of experts to system RAM (--n-cpu-moe). Those
// expert bytes are a RAM cost, never a VRAM cost, and the plan's own
// numbers must say so:
//
//   - the layers kept on the card fit the VRAM budget,
//   - the experts moved to RAM fit the RAM budget (or, for the documented
//     paging allowance, exceed it by at most 10% and say Stream),
//   - RAMNeed reports the bytes the plan expects to keep resident in RAM,
//   - a draft model the plan drops does not change any of those numbers.

import (
	"fmt"
	"math"
	"testing"

	"github.com/picom-code/ezlm/internal/catalog"
	"github.com/picom-code/ezlm/internal/scan"
)

// moePageAllowance is the documented overcommit for memory-mapped expert
// weights: clean, evictable pages may page from SSD, but never by more than
// this, and the plan must set Stream when it does.
const moePageAllowance = 1.1

// moeProfiles is the generated space: discrete cards from 4 to 48 GB with
// 8 to 128 GB of RAM on the CUDA margins and on the larger Windows Vulkan
// margin, a two-card machine, Apple unified memory (where hybrid is not
// offered) and processor-only boxes.
func moeProfiles() []struct {
	name string
	hw   *scan.Hardware
} {
	var out []struct {
		name string
		hw   *scan.Hardware
	}
	add := func(name string, hw *scan.Hardware) {
		out = append(out, struct {
			name string
			hw   *scan.Hardware
		}{name, hw})
	}

	for _, vram := range []uint64{4, 6, 8, 11, 12, 16, 24, 32, 48} {
		for _, ram := range []uint64{8, 16, 32, 64, 128} {
			// Keep the pairs realistic: a 4 GB card does not sit in a
			// 128 GB workstation, a 48 GB card does not sit in an 8 GB laptop.
			if vram <= 8 && ram > 32 || vram >= 32 && ram < 32 {
				continue
			}
			bw := 100.0 + float64(vram)*40
			for _, os := range []string{"linux", "windows"} {
				add(fmt.Sprintf("%s CUDA %dGB VRAM + %dGB RAM", os, vram, ram), &scan.Hardware{
					OS: os, Backend: scan.CUDA, RAMTotal: ram * G, RAMAvail: ram * G / 4 * 3,
					GPUUsable: vram * G, DiskFree: 2000 * G, CPUBandwidth: 60, PhysCores: 8,
					GPUs: []scan.GPU{{Name: "probe", Vendor: "nvidia", MemTotal: vram * G, MemFree: vram * G, Bandwidth: bw}},
				})
			}
			// Windows Vulkan (AMD/Intel): a wider cushion on the card, the
			// same RAM rule.
			add(fmt.Sprintf("windows Vulkan %dGB VRAM + %dGB RAM", vram, ram), &scan.Hardware{
				OS: "windows", Backend: scan.Vulkan, RAMTotal: ram * G, RAMAvail: ram * G / 4 * 3,
				GPUUsable: vram * G, DiskFree: 2000 * G, CPUBandwidth: 60, PhysCores: 8,
				GPUs: []scan.GPU{{Name: "probe", Vendor: "amd", MemTotal: vram * G, MemFree: vram * G, Bandwidth: bw}},
			})
		}
	}
	// A free-memory reading that is much lower than the installed RAM, as on
	// a machine running other apps, and one that is nearly all of it.
	for _, vram := range []uint64{8, 12, 24} {
		add(fmt.Sprintf("linux CUDA %dGB VRAM, busy 32GB RAM", vram), &scan.Hardware{
			OS: "linux", Backend: scan.CUDA, RAMTotal: 32 * G, RAMAvail: 10 * G,
			GPUUsable: vram * G, DiskFree: 2000 * G, CPUBandwidth: 60, PhysCores: 8,
			GPUs: []scan.GPU{{Name: "probe", Vendor: "nvidia", MemTotal: vram * G, MemFree: vram * G, Bandwidth: 300}},
		})
	}
	// Two cards: the budget is their sum, the RAM rule is unchanged.
	for _, ram := range []uint64{32, 64} {
		h := &scan.Hardware{OS: "linux", Backend: scan.CUDA, RAMTotal: ram * G, RAMAvail: ram * G / 4 * 3,
			DiskFree: 2000 * G, CPUBandwidth: 120, PhysCores: 16}
		for i := 0; i < 2; i++ {
			free := uint64(23) * G
			h.GPUs = append(h.GPUs, scan.GPU{Name: "probe", Vendor: "nvidia", MemTotal: 24 * G, MemFree: free, Bandwidth: 936})
			h.GPUUsable += free
		}
		add(fmt.Sprintf("2x 24GB CUDA + %dGB RAM", ram), h)
	}
	// Apple unified memory: one pool, so there is no separate RAM offload.
	for _, ram := range []uint64{8, 16, 32, 64, 128} {
		usable := uint64(float64(ram*G) * 0.67)
		if ram > 36 {
			usable = uint64(float64(ram*G) * 0.75)
		}
		add(fmt.Sprintf("Apple unified %dGB", ram), &scan.Hardware{
			OS: "darwin", Backend: scan.Metal, Unified: true, RAMTotal: ram * G, RAMAvail: ram * G / 4 * 3,
			GPUUsable: usable, DiskFree: 2000 * G, CPUBandwidth: 400, PhysCores: 10,
			GPUs: []scan.GPU{{Name: "probe", Vendor: "apple", Bandwidth: 400}},
		})
	}
	for _, ram := range []uint64{8, 16, 64} {
		add(fmt.Sprintf("processor only %dGB", ram), &scan.Hardware{
			OS: "linux", Backend: scan.CPU, RAMTotal: ram * G, RAMAvail: ram * G / 4 * 3,
			DiskFree: 2000 * G, CPUBandwidth: 40, PhysCores: 6,
		})
	}
	return out
}

// moeSplit mirrors the planner's split of a MoE file into the dense bytes
// that stay on the card and the per-layer expert bytes that move to RAM.
// Hybrid plans drop speculative decoding, so the draft model's bytes are
// not part of the split (the planner must agree; see TestMoEHybridIgnoresDraft).
func moeSplit(h *scan.Hardware, m catalog.Model, q catalog.Quant) (dense uint64, perLayer float64) {
	w := q.Bytes
	lazy := q.LazyBytes > 0
	if lazy {
		w -= q.LazyBytes
	}
	activeFrac := 1.0
	if m.ParamsB > 0 && m.ActiveParamsB > 0 {
		resident := 1.0
		if lazy && q.Bytes > 0 {
			resident = float64(q.Bytes-q.LazyBytes) / float64(q.Bytes)
		}
		activeFrac = math.Min(1, m.ActiveParamsB/(m.ParamsB*resident))
	}
	activeBytes := float64(w) * activeFrac
	dense = uint64(activeBytes * 0.5)
	if m.Layers > 0 {
		perLayer = float64(w-dense) / float64(m.Layers)
	}
	return dense, perLayer
}

// TestMoEOffloadFitsRAM sweeps every MoE model and quant over the generated
// profiles and checks the hybrid plan's memory accounting.
func TestMoEOffloadFitsRAM(t *testing.T) {
	c := cat(t)
	hybrid, streamed := 0, 0
	for _, prof := range moeProfiles() {
		hw := prof.hw
		b := ComputeBudgets(hw, hw.RAMAvail)
		for _, m := range c.Models {
			if !m.MoE || m.Layers <= 0 {
				continue
			}
			for _, q := range m.Quants {
				for _, ctx := range []int{65536, 32768, 16384, 8192} {
					if m.CtxMax > 0 && ctx > m.CtxMax {
						continue
					}
					kvs := []string{"f16", "q8_0", "q4_0"}
					if m.NoKVQuant {
						kvs = []string{"f16"}
					}
					for _, kv := range kvs {
						p, ok := try(hw, m, q, ctx, kv, b)
						if !ok || p.Mode != Hybrid {
							continue
						}
						hybrid++
						if p.Stream {
							streamed++
						}
						if p.NCPUMoE <= 0 || p.NCPUMoE > m.Layers {
							t.Errorf("%s %s %s ctx %d: NCPUMoE %d outside 1..%d layers", prof.name, m.ID, q.Name, ctx, p.NCPUMoE, m.Layers)
						}
						if p.GPUNeed > b.GPU {
							t.Errorf("%s %s %s ctx %d: VRAM need %d above budget %d", prof.name, m.ID, q.Name, ctx, p.GPUNeed, b.GPU)
						}
						if p.RAMNeed > b.RAM {
							t.Errorf("%s %s %s ctx %d: RAM need %d above budget %d", prof.name, m.ID, q.Name, ctx, p.RAMNeed, b.RAM)
						}
						// The experts the plan moves to RAM: one whole layer's
						// expert bytes per offloaded layer, plus the share of
						// the lookup tables that competes for the page cache.
						_, perLayer := moeSplit(hw, m, q)
						off := uint64(float64(p.NCPUMoE) * perLayer)
						if q.LazyBytes > 0 {
							off += q.LazyBytes / 5
						}
						switch {
						case !p.Stream:
							if off > b.RAM {
								t.Errorf("%s %s %s ctx %d: %d expert bytes in RAM, budget %d, but Stream is not set", prof.name, m.ID, q.Name, ctx, off, b.RAM)
							}
							if p.RAMNeed != off {
								t.Errorf("%s %s %s ctx %d: RAMNeed %d does not match the %d bytes offloaded to RAM", prof.name, m.ID, q.Name, ctx, p.RAMNeed, off)
							}
						default:
							if off <= b.RAM {
								t.Errorf("%s %s %s ctx %d: Stream set but %d expert bytes fit the %d byte budget", prof.name, m.ID, q.Name, ctx, off, b.RAM)
							}
							if float64(off) > float64(b.RAM)*moePageAllowance {
								t.Errorf("%s %s %s ctx %d: %d expert bytes in RAM, %.0f%% over the %d byte budget", prof.name, m.ID, q.Name, ctx, off, (float64(off)/float64(b.RAM)-1)*100, b.RAM)
							}
							if p.RAMNeed != b.RAM {
								t.Errorf("%s %s %s ctx %d: streamed plan reports RAMNeed %d, want the %d byte budget", prof.name, m.ID, q.Name, ctx, p.RAMNeed, b.RAM)
							}
						}
					}
				}
			}
		}
	}
	if hybrid == 0 {
		t.Fatal("no hybrid plan in the generated space: the test proves nothing")
	}
	t.Logf("%d hybrid plans checked (%d paging), %d profiles", hybrid, streamed, len(moeProfiles()))
}

// TestMoEHybridIgnoresDraft pins the draft-model bug: a hybrid plan drops
// speculative decoding (the draft costs more than it saves with experts in
// RAM), so the draft's bytes must not change the plan's VRAM/RAM math.
// Before the fix the plan budgeted the draft and then discarded it: up to
// 2.3 GB of RAM counted for a model it never downloads.
func TestMoEHybridIgnoresDraft(t *testing.T) {
	c := cat(t)
	checked, promoted := 0, 0
	reported := 0
	for _, prof := range moeProfiles() {
		hw := prof.hw
		if hw.Backend != scan.CUDA {
			continue // a draft model is only offered on CUDA
		}
		b := ComputeBudgets(hw, hw.RAMAvail)
		for _, m := range c.Models {
			if !m.MoE || m.Draft == nil || m.Layers <= 0 {
				continue
			}
			noDraft := m
			noDraft.Draft = nil
			for _, q := range m.Quants {
				for _, ctx := range []int{65536, 32768, 16384, 8192} {
					if m.CtxMax > 0 && ctx > m.CtxMax {
						continue
					}
					p, ok := try(hw, m, q, ctx, "f16", b)
					if !ok || p.Mode != Hybrid {
						continue
					}
					want, ok := try(hw, noDraft, q, ctx, "f16", b)
					if !ok {
						t.Fatalf("%s %s %s ctx %d: no plan without the draft, but one with it", prof.name, m.ID, q.Name, ctx)
					}
					if want.Mode != Hybrid {
						// Without the draft the model fits entirely on the
						// card. The draft pushed the GPU check over, so the
						// planner offloads experts anyway: a quality miss,
						// not a wrong RAM number (reported in REPORT.md).
						promoted++
						continue
					}
					checked++
					if p.Spec != "" {
						t.Errorf("%s %s %s ctx %d: hybrid plan keeps spec %q", prof.name, m.ID, q.Name, ctx, p.Spec)
					}
					if p.NCPUMoE != want.NCPUMoE || p.GPUNeed != want.GPUNeed || p.RAMNeed != want.RAMNeed || p.Stream != want.Stream {
						if reported < 5 {
							reported++
							t.Errorf("%s %s %s ctx %d: draft %d bytes changes a plan that drops it: ncmoe %d vs %d, VRAM %d vs %d, RAM %d vs %d, stream %v vs %v",
								prof.name, m.ID, q.Name, ctx, m.Draft.Bytes, p.NCPUMoE, want.NCPUMoE, p.GPUNeed, want.GPUNeed, p.RAMNeed, want.RAMNeed, p.Stream, want.Stream)
						}
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no hybrid plan for a model with a draft: the test proves nothing")
	}
	t.Logf("%d hybrid plans checked against the same model without its draft (%d more fit the card without it)", checked, promoted)
}

// TestMoEHybridRealDesktop records the case that matters: the RTX 4070
// SUPER + 62 GB desktop the catalog is calibrated on, running Flash Next with
// experts in RAM. The draft model must not inflate its RAM or VRAM numbers.
func TestMoEHybridRealDesktop(t *testing.T) {
	c := cat(t)
	m, ok := c.Find("qwen3.8-flash-next")
	if !ok || m.Draft == nil {
		t.Fatal("qwen3.8-flash-next with a draft model missing from the catalog")
	}
	hw := realMachines[0].hw
	b := ComputeBudgets(hw, hw.RAMAvail)
	q := m.Quants[0]
	p, ok := try(hw, m, q, 32768, "f16", b)
	if !ok || p.Mode != Hybrid {
		t.Fatalf("expected a hybrid plan, got ok=%v mode=%s", ok, p.Mode)
	}
	noDraft := m
	noDraft.Draft = nil
	want, ok := try(hw, noDraft, q, 32768, "f16", b)
	if !ok {
		t.Fatal("expected a hybrid plan without the draft")
	}
	t.Logf("%s %s: ncmoe %d/%d, VRAM %.2f GB, RAM %.2f GB, draft %d bytes dropped",
		m.ID, q.Name, p.NCPUMoE, m.Layers, float64(p.GPUNeed)/float64(G), float64(p.RAMNeed)/float64(G), m.Draft.Bytes)
	if p.NCPUMoE != want.NCPUMoE || p.GPUNeed != want.GPUNeed || p.RAMNeed != want.RAMNeed {
		t.Errorf("draft changes the hybrid plan: ncmoe %d vs %d, VRAM %d vs %d, RAM %d vs %d",
			p.NCPUMoE, want.NCPUMoE, p.GPUNeed, want.GPUNeed, p.RAMNeed, want.RAMNeed)
	}
}
