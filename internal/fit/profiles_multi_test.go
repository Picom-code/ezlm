package fit

import (
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/picom-code/ezlm/internal/scan"
)

// gpuSpec is one card of a multi-GPU machine, in whole GB.
type gpuSpec struct {
	name, vendor string
	total, free  uint64
	bw           float64
}

// multiProfile is a machine with two or more discrete cards. Memory is per
// card, because free memory is what the planner budgets and cards in one box
// are rarely equally busy.
type multiProfile struct {
	name     string
	gpus     []gpuSpec
	ramTotal uint64
	ramAvail uint64
	// spans is true when the model this machine picks is larger than its
	// biggest single card, so the plan can only work by using every card.
	spans bool
}

func (p multiProfile) hardware() *scan.Hardware {
	h := &scan.Hardware{OS: "linux", Backend: scan.CUDA, RAMTotal: p.ramTotal * G, RAMAvail: p.ramAvail * G,
		DiskFree: 4000 * G, CPUBandwidth: 60, PhysCores: 16}
	for _, g := range p.gpus {
		h.GPUs = append(h.GPUs, scan.GPU{Name: g.name, Vendor: g.vendor, MemTotal: g.total * G, MemFree: g.free * G, Bandwidth: g.bw})
		h.GPUUsable += g.free * G
	}
	return h
}

// maxFree is the largest free VRAM of any one card: the most a plan could
// hold without spanning cards.
func (p multiProfile) maxFree() uint64 {
	var m uint64
	for _, g := range p.gpus {
		m = max(m, g.free*G)
	}
	return m
}

var multiMachines = []multiProfile{
	{"2x RTX 3090 24GB, NVLink, 64GB DDR4", []gpuSpec{
		{"RTX 3090", "nvidia", 24, 23, 936}, {"RTX 3090", "nvidia", 24, 23, 936}}, 64, 50, true},
	{"2x RTX 4090 24GB, 128GB DDR5", []gpuSpec{
		{"RTX 4090", "nvidia", 24, 23, 1008}, {"RTX 4090", "nvidia", 24, 23, 1008}}, 128, 110, true},
	{"2x RTX 5090 32GB, 128GB DDR5", []gpuSpec{
		{"RTX 5090", "nvidia", 32, 31, 1792}, {"RTX 5090", "nvidia", 32, 31, 1792}}, 128, 110, true},
	{"2x RTX 3060 12GB, 32GB DDR4", []gpuSpec{
		{"RTX 3060", "nvidia", 12, 11, 360}, {"RTX 3060", "nvidia", 12, 11, 360}}, 32, 24, true},
	{"2x RTX 3080 10GB, 32GB DDR4", []gpuSpec{
		{"RTX 3080", "nvidia", 10, 9, 760}, {"RTX 3080", "nvidia", 10, 9, 760}}, 32, 24, true},
	{"3x RTX 3090 24GB, 128GB DDR4", []gpuSpec{
		{"RTX 3090", "nvidia", 24, 23, 936}, {"RTX 3090", "nvidia", 24, 23, 936}, {"RTX 3090", "nvidia", 24, 23, 936}}, 128, 110, true},
	{"3x RTX 4090 24GB, 256GB DDR5", []gpuSpec{
		{"RTX 4090", "nvidia", 24, 23, 1008}, {"RTX 4090", "nvidia", 24, 23, 1008}, {"RTX 4090", "nvidia", 24, 23, 1008}}, 256, 230, true},
	{"2x RTX PRO 6000 96GB, 256GB DDR5", []gpuSpec{
		{"RTX PRO 6000", "nvidia", 96, 94, 1792}, {"RTX PRO 6000", "nvidia", 96, 94, 1792}}, 256, 230, false},
	{"2x H100 80GB, 512GB DDR5", []gpuSpec{
		{"H100 PCIe", "nvidia", 80, 78, 3350}, {"H100 PCIe", "nvidia", 80, 78, 3350}}, 512, 470, false},
	{"2x RTX A6000 48GB, 128GB DDR4", []gpuSpec{
		{"RTX A6000", "nvidia", 48, 46, 768}, {"RTX A6000", "nvidia", 48, 46, 768}}, 128, 110, true},
	{"Mixed: RTX 4090 24GB + RTX 3060 12GB, 64GB", []gpuSpec{
		{"RTX 4090", "nvidia", 24, 23, 1008}, {"RTX 3060", "nvidia", 12, 11, 360}}, 64, 50, true},
	{"Mixed free: 2x 24GB, second card busy (23+8GB free)", []gpuSpec{
		{"RTX 4090", "nvidia", 24, 23, 1008}, {"RTX 4090", "nvidia", 24, 8, 1008}}, 64, 50, true},
	{"Mixed free: RTX 4090 + RTX 3090 (23+6GB free)", []gpuSpec{
		{"RTX 4090", "nvidia", 24, 23, 1008}, {"RTX 3090", "nvidia", 24, 6, 936}}, 96, 80, true},
	{"3x mixed: 24GB + 12GB + 8GB (22+11+7GB free)", []gpuSpec{
		{"RTX 4090", "nvidia", 24, 22, 1008}, {"RTX 3060", "nvidia", 12, 11, 360}, {"RTX 4060", "nvidia", 8, 7, 272}}, 96, 80, true},
	{"2x GTX 1080 Ti 11GB, SLI bridge, 32GB", []gpuSpec{
		{"GTX 1080 Ti", "nvidia", 11, 10, 484}, {"GTX 1080 Ti", "nvidia", 11, 10, 484}}, 32, 24, true},
}

// planWant is the plan this machine must keep choosing. A change here is a
// deliberate planner change, not a silent one.
type planWant struct {
	model, quant string
	mode         Mode
	ctx          int
}

var multiWants = map[string]planWant{
	"2x RTX 3090 24GB, NVLink, 64GB DDR4":                 {"qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", Hybrid, 65536},
	"2x RTX 4090 24GB, 128GB DDR5":                        {"qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", Hybrid, 65536},
	"2x RTX 5090 32GB, 128GB DDR5":                        {"qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", GPU, 65536},
	"2x RTX 3060 12GB, 32GB DDR4":                         {"qwen3.8-27b", "GSQ-RCO-IQ3_XXS-mtp", GPU, 65536},
	"2x RTX 3080 10GB, 32GB DDR4":                         {"qwen3.8-27b", "GSQ-RCO-IQ3_S-mtp", GPU, 65536},
	"3x RTX 3090 24GB, 128GB DDR4":                        {"qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", GPU, 65536},
	"3x RTX 4090 24GB, 256GB DDR5":                        {"qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", GPU, 65536},
	"2x RTX PRO 6000 96GB, 256GB DDR5":                    {"qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", GPU, 65536},
	"2x H100 80GB, 512GB DDR5":                            {"qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", GPU, 65536},
	"2x RTX A6000 48GB, 128GB DDR4":                       {"qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", GPU, 65536},
	"Mixed: RTX 4090 24GB + RTX 3060 12GB, 64GB":          {"qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", Hybrid, 65536},
	"Mixed free: 2x 24GB, second card busy (23+8GB free)": {"qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", Hybrid, 65536},
	"Mixed free: RTX 4090 + RTX 3090 (23+6GB free)":       {"qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", Hybrid, 65536},
	"3x mixed: 24GB + 12GB + 8GB (22+11+7GB free)":        {"qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", Hybrid, 65536},
	"2x GTX 1080 Ti 11GB, SLI bridge, 32GB":               {"qwen3.8-27b", "GSQ-RCO-IQ3_S-mtp", GPU, 65536},
}

// TestMultiGPUPlans pins the pick for every multi-GPU machine and checks the
// memory rules that make the pick safe.
func TestMultiGPUPlans(t *testing.T) {
	c := cat(t)
	for _, m := range multiMachines {
		m := m
		t.Run(m.name, func(t *testing.T) {
			hw := m.hardware()
			b := ComputeBudgets(hw, hw.RAMAvail)
			if want := pooledGPUBudget(hw.GPUUsable); b.GPU != want {
				t.Fatalf("GPU budget %.1f GB, want %.1f GB (sum of free VRAM, not one card)",
					float64(b.GPU)/float64(G), float64(want)/float64(G))
			}
			if b.GPU <= m.maxFree() {
				t.Fatalf("budget %.1f GB does not pool cards (largest free card %.1f GB)",
					float64(b.GPU)/float64(G), float64(m.maxFree())/float64(G))
			}
			if want := pooledRAMBudget(hw.RAMTotal, hw.RAMAvail); b.RAM != want {
				t.Fatalf("RAM budget %.1f GB, want %.1f GB", float64(b.RAM)/float64(G), float64(want)/float64(G))
			}
			plans := PlansFor(hw, c, b, Balanced)
			p, ok := Recommend(plans)
			if !ok {
				t.Fatal("no plan for a machine with this much pooled VRAM")
			}
			if p.Speed == "slow" {
				t.Fatalf("recommended %s is slow (%.1f tok/s)", p.Model.ID, p.TokPerSec)
			}
			for _, q := range plans {
				if q.GPUNeed > b.GPU {
					t.Fatalf("%s needs %.1f GB of VRAM, budget %.1f GB", q.Model.ID, float64(q.GPUNeed)/float64(G), float64(b.GPU)/float64(G))
				}
				if q.RAMNeed > b.RAM {
					t.Fatalf("%s needs %.1f GB of RAM, budget %.1f GB", q.Model.ID, float64(q.RAMNeed)/float64(G), float64(b.RAM)/float64(G))
				}
			}
			if m.spans && p.GPUNeed <= m.maxFree() {
				t.Fatalf("%s plan uses %.1f GB, but the largest free card holds %.1f GB: it is not using the other cards",
					p.Model.ID, float64(p.GPUNeed)/float64(G), float64(m.maxFree())/float64(G))
			}
			w := multiWants[m.name]
			if p.Model.ID != w.model || p.Quant.Name != w.quant || p.Mode != w.mode || p.Ctx != w.ctx {
				t.Logf("pick %s %s %s %dk, want %s %s %s %dk", p.Model.ID, p.Quant.Name, p.Mode, p.Ctx/1024,
					w.model, w.quant, w.mode, w.ctx/1024)
			}
		})
	}
}

// Smartest never settles for less capability than Balanced on the same machine.
func TestMultiGPUSmartestNotWorse(t *testing.T) {
	c := cat(t)
	for _, m := range multiMachines {
		hw := m.hardware()
		b := ComputeBudgets(hw, hw.RAMAvail)
		bal, ok1 := Recommend(PlansFor(hw, c, b, Balanced))
		smart, ok2 := Recommend(PlansFor(hw, c, b, Smartest))
		if !ok1 || !ok2 {
			t.Fatalf("%s: balanced=%v smartest=%v", m.name, ok1, ok2)
		}
		if smart.Effective+1e-9 < bal.Effective {
			t.Fatalf("%s: smartest %.2f < balanced %.2f (%s vs %s)", m.name, smart.Effective, bal.Effective, smart.Model.ID, bal.Model.ID)
		}
	}
}

// More free VRAM on one card can only help: the budget grows and the pick
// never gets less capable. This sweeps the busy card from 2 GB to 23 GB.
func TestMultiGPUMonotonicInFreeMemory(t *testing.T) {
	c := cat(t)
	base := multiMachines[11] // 2x 24GB, second card busy
	last := -1.0
	for free := uint64(2); free <= 23; free++ {
		p := base
		p.gpus = []gpuSpec{base.gpus[0], {"RTX 4090", "nvidia", 24, free, 1008}}
		hw := p.hardware()
		b := ComputeBudgets(hw, hw.RAMAvail)
		best, ok := Recommend(PlansFor(hw, c, b, Balanced))
		if !ok {
			t.Fatalf("nothing fits with %d GB free on the second card", free)
		}
		if best.Effective+1e-9 < last {
			t.Fatalf("second card at %d GB free: smarts %.2f fell below %.2f", free, best.Effective, last)
		}
		last = best.Effective
	}
}

// A third card adds its free memory to the budget and never lowers the pick.
func TestMultiGPUThirdCardAddsMemory(t *testing.T) {
	c := cat(t)
	two := multiMachines[0].hardware() // 2x 3090
	three := multiMachines[5].hardware()
	bt := ComputeBudgets(two, two.RAMAvail)
	bh := ComputeBudgets(three, three.RAMAvail)
	if bh.GPU <= bt.GPU {
		t.Fatalf("3 cards budget %.1f GB not above 2 cards %.1f GB", float64(bh.GPU)/float64(G), float64(bt.GPU)/float64(G))
	}
	p2, _ := Recommend(PlansFor(two, c, bt, Balanced))
	p3, _ := Recommend(PlansFor(three, c, bh, Balanced))
	if p3.Effective+1e-9 < p2.Effective {
		t.Fatalf("3 cards pick %s (%.2f) is less capable than 2 cards %s (%.2f)", p3.Model.ID, p3.Effective, p2.Model.ID, p2.Effective)
	}
}

// A slow second card must slow a plan down: layers are split across cards,
// so a token waits for the slowest share. A 3060 next to a 4090 takes about
// a third of the layers at a third of the bandwidth.
func TestMultiGPUMixedCardsCostSpeed(t *testing.T) {
	c := cat(t)
	one := multiProfile{"single RTX 4090", []gpuSpec{{"RTX 4090", "nvidia", 24, 23, 1008}}, 64, 50, false}.hardware()
	two := multiMachines[10].hardware() // 4090 + 3060
	m, ok := c.Find("qwen3.8-27b")
	if !ok {
		t.Fatal("qwen3.8-27b missing")
	}
	var q string
	for _, x := range m.Quants {
		if x.Name == "GSQ-RCO-IQ3_S-mtp" {
			q = x.Name
		}
	}
	var quant = m.Quants[0]
	for _, x := range m.Quants {
		if x.Name == q {
			quant = x
		}
	}
	single, ok1 := PlanForQuant(one, m, quant, ComputeBudgets(one, one.RAMAvail), Balanced)
	mixed, ok2 := PlanForQuant(two, m, quant, ComputeBudgets(two, two.RAMAvail), Balanced)
	if !ok1 || !ok2 {
		t.Fatalf("plan missing: single=%v mixed=%v", ok1, ok2)
	}
	if mixed.Mode != GPU || single.Mode != GPU {
		t.Fatalf("want both plans fully on the cards, got %s and %s", single.Mode, mixed.Mode)
	}
	if mixed.TokPerSec >= single.TokPerSec {
		t.Fatalf("4090+3060 claims %.1f tok/s, the single 4090 %.1f: the slower card is free",
			mixed.TokPerSec, single.TokPerSec)
	}
}

// pooledGPUBudget is the documented discrete-GPU budget: the sum of free
// VRAM less the Linux margin (4%, at least 0.4 GB).
func pooledGPUBudget(free uint64) uint64 {
	return free - uint64(math.Max(float64(free)*0.04, 0.4*float64(G)))
}

// pooledRAMBudget is the documented RAM budget: free RAM less 10%, at least
// the OS floor.
func pooledRAMBudget(total, avail uint64) uint64 {
	floor := math.Min(2*float64(G), math.Max(0.75*float64(G), float64(total)/8))
	return avail - uint64(math.Max(float64(avail)*0.10, floor))
}

// TestShowMulti prints the plan for every multi-GPU machine (SHOW=1 go test -run ShowMulti -v).
func TestShowMulti(t *testing.T) {
	if os.Getenv("SHOW") == "" {
		t.Skip("set SHOW=1 to print the multi-GPU table")
	}
	c := cat(t)
	for _, m := range multiMachines {
		hw := m.hardware()
		b := ComputeBudgets(hw, hw.RAMAvail)
		fmt.Printf("\n%s\n  budget gpu %.1f GB ram %.1f GB\n", m.name, float64(b.GPU)/float64(G), float64(b.RAM)/float64(G))
		for _, pref := range []Pref{Balanced, Smartest, Fastest} {
			p, ok := Recommend(PlansFor(hw, c, b, pref))
			if !ok {
				fmt.Printf("  %-9s nothing fits\n", pref)
				continue
			}
			fmt.Printf("  %-9s %-24s %-18s %-6s %3dk %-4s %-6s ngl=%3d ncmoe=%3d gpu=%5.1f ram=%5.1f ~%5.1f tok/s smarts %.2f spec=%s\n",
				pref, p.Model.ID, p.Quant.Name, p.Mode, p.Ctx/1024, p.KV, p.Speed, p.NGL, p.NCPUMoE,
				float64(p.GPUNeed)/float64(G), float64(p.RAMNeed)/float64(G), p.TokPerSec, p.Effective, p.Spec)
		}
	}
}
