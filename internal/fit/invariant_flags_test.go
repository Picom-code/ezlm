package fit

// The plan is the recipe: -c, -ngl and the expert-offload flags must always
// be explicit, so llama-server never falls back to a default that could put
// more on the graphics card than the plan budgeted for. These tests generate a
// spread of machines and check every plan the planner returns, not just the
// recommended one.

import (
	"fmt"
	"testing"

	"github.com/picom-code/ezlm/internal/catalog"
	"github.com/picom-code/ezlm/internal/scan"
	"github.com/picom-code/ezlm/internal/serve"
)

// flagProfile is one generated machine: a name for the failure message and
// the hardware the planner sees.
type flagProfile struct {
	name string
	hw   *scan.Hardware
}

// flagProfiles spans the placements the planner can choose (all on GPU,
// experts in RAM, split layers, CPU only) across the backends EZLM supports.
func flagProfiles() []flagProfile {
	var out []flagProfile
	add := func(name string, hw *scan.Hardware) { out = append(out, flagProfile{name, hw}) }

	// Apple Silicon: one memory pool.
	for _, ram := range []uint64{8, 16, 24, 36, 64, 128, 192} {
		free := ram * 3 / 4
		if ram <= 16 {
			free = ram / 2
		}
		usable := uint64(float64(ram*G) * 0.67)
		if ram > 36 {
			usable = uint64(float64(ram*G) * 0.75)
		}
		bw := 100.0
		switch {
		case ram >= 128:
			bw = 546
		case ram >= 64:
			bw = 400
		}
		add(fmt.Sprintf("Mac unified %dGB (%dGB free)", ram, free), &scan.Hardware{
			OS: "darwin", Arch: "arm64", Backend: scan.Metal, Unified: true,
			RAMTotal: ram * G, RAMAvail: free * G, GPUUsable: usable, DiskFree: 800 * G,
			PhysCores: 12, CPUBandwidth: bw,
			GPUs: []scan.GPU{{Vendor: "apple", Bandwidth: bw}}})
	}

	// Discrete NVIDIA: everything from a 4GB card to a 48GB one.
	for _, vram := range []uint64{4, 6, 8, 12, 16, 24, 32, 48} {
		for _, ram := range []uint64{16, 32, 64} {
			bw := 128.0 + float64(vram)*40
			add(fmt.Sprintf("RTX %dGB + %dGB RAM", vram, ram), &scan.Hardware{
				OS: "windows", Arch: "amd64", Backend: scan.CUDA,
				RAMTotal: ram * G, RAMAvail: ram * 2 / 3 * G, GPUUsable: vram * G, DiskFree: 800 * G,
				PhysCores: 8, CPUBandwidth: 60,
				GPUs: []scan.GPU{{Vendor: "nvidia", MemTotal: vram * G, MemFree: vram * G, Bandwidth: bw}}})
		}
	}

	// AMD and Intel through Vulkan, on Windows and Linux.
	for _, os := range []string{"windows", "linux"} {
		for _, vram := range []uint64{4, 8, 16, 24} {
			add(fmt.Sprintf("%s Vulkan %dGB", os, vram), &scan.Hardware{
				OS: os, Arch: "amd64", Backend: scan.Vulkan,
				RAMTotal: 32 * G, RAMAvail: 22 * G, GPUUsable: vram * G, DiskFree: 800 * G,
				PhysCores: 8, CPUBandwidth: 60,
				GPUs: []scan.GPU{{Vendor: "amd", MemTotal: vram * G, MemFree: vram * G, Bandwidth: 500}}})
		}
	}
	add("Intel Arc 12GB, Windows", &scan.Hardware{
		OS: "windows", Arch: "amd64", Backend: scan.Vulkan,
		RAMTotal: 32 * G, RAMAvail: 24 * G, GPUUsable: 11 * G, DiskFree: 800 * G,
		PhysCores: 8, CPUBandwidth: 70,
		GPUs: []scan.GPU{{Vendor: "intel", MemTotal: 12 * G, MemFree: 11 * G, Bandwidth: 560}}})

	// Two cards of the same size and two mismatched ones.
	for _, g := range [][2]uint64{{12, 12}, {24, 24}, {24, 8}, {16, 4}} {
		hw := &scan.Hardware{OS: "linux", Arch: "amd64", Backend: scan.CUDA,
			RAMTotal: 64 * G, RAMAvail: 48 * G, DiskFree: 2000 * G, PhysCores: 16, CPUBandwidth: 80}
		for _, v := range g {
			hw.GPUs = append(hw.GPUs, scan.GPU{Vendor: "nvidia", MemTotal: v * G, MemFree: v * G, Bandwidth: 900})
			hw.GPUUsable += v * G
		}
		add(fmt.Sprintf("2x NVIDIA %dGB+%dGB", g[0], g[1]), hw)
	}

	// Windows on Arm with a big shared-memory GPU and little RAM.
	add("Windows on Arm, 96GB GPU share, 30GB RAM", &scan.Hardware{
		OS: "windows", Arch: "arm64", Backend: scan.CUDA,
		RAMTotal: 30 * G, RAMAvail: 20 * G, GPUUsable: 90 * G, DiskFree: 800 * G,
		PhysCores: 12, CPUBandwidth: 273,
		GPUs: []scan.GPU{{Vendor: "nvidia", MemTotal: 96 * G, MemFree: 90 * G, Bandwidth: 273}}})

	// Processor-only laptops and workstations.
	for _, ram := range []uint64{4, 8, 16, 32, 64} {
		add(fmt.Sprintf("CPU only, %dGB RAM", ram), &scan.Hardware{
			OS: "linux", Arch: "amd64", Backend: scan.CPU,
			RAMTotal: ram * G, RAMAvail: ram * 2 / 3 * G, DiskFree: 800 * G,
			PhysCores: 8, CPUBandwidth: 50})
	}
	return out
}

// argValue returns the value that follows the first occurrence of flag, and
// how many times the flag appears.
func argValue(argv []string, flag string) (string, int) {
	val, n := "", 0
	for i, a := range argv {
		if a != flag {
			continue
		}
		n++
		if i+1 < len(argv) {
			val = argv[i+1]
		}
	}
	return val, n
}

// planArgv is the llama-server command line for a plan, as serve builds it.
func planArgv(p Plan) []string {
	c := serve.Config{ModelPath: "/models/model.gguf", ModelID: p.Model.ID,
		Ctx: p.Ctx, NGL: p.NGL, NCPUMoE: p.NCPUMoE, ExtraArgs: p.Args(), Port: 18080}
	return c.Args()
}

// checkPlanFlags asserts one plan sets every memory-relevant flag explicitly
// and that the flags agree with the plan's own mode.
func checkPlanFlags(t *testing.T, profile string, p Plan) {
	t.Helper()
	where := fmt.Sprintf("%s: %s %s/%s %s", profile, p.Model.ID, p.Quant.Name, p.Mode, p.KV)

	// -c: always a context the planner actually tried, never zero.
	onLadder := false
	for _, c := range ctxLadder {
		if c.n == p.Ctx {
			onLadder = true
		}
	}
	if p.Ctx <= 0 || !onLadder {
		t.Errorf("%s: ctx %d is not one of the planned contexts", where, p.Ctx)
	}
	if p.Model.CtxMax > 0 && p.Ctx > p.Model.CtxMax {
		t.Errorf("%s: ctx %d is past the model's %d", where, p.Ctx, p.Model.CtxMax)
	}

	// -ngl and --n-cpu-moe: exactly what the mode means.
	switch p.Mode {
	case GPU:
		if p.NGL < p.Model.Layers {
			t.Errorf("%s: GPU plan offloads only %d of %d layers", where, p.NGL, p.Model.Layers)
		}
		if p.NCPUMoE != 0 {
			t.Errorf("%s: GPU plan keeps %d layers' experts in RAM", where, p.NCPUMoE)
		}
	case Hybrid:
		if p.NGL < p.Model.Layers {
			t.Errorf("%s: hybrid plan offloads only %d of %d layers", where, p.NGL, p.Model.Layers)
		}
		if p.NCPUMoE <= 0 || p.NCPUMoE > p.Model.Layers {
			t.Errorf("%s: hybrid plan offloads %d of %d layers' experts", where, p.NCPUMoE, p.Model.Layers)
		}
	case Split:
		if p.NGL <= 0 || p.NGL >= p.Model.Layers {
			t.Errorf("%s: split plan offloads %d of %d layers", where, p.NGL, p.Model.Layers)
		}
		if p.NCPUMoE != 0 {
			t.Errorf("%s: split plan keeps %d layers' experts in RAM", where, p.NCPUMoE)
		}
	case CPU:
		if p.NGL != 0 || p.NCPUMoE != 0 {
			t.Errorf("%s: CPU plan sets ngl %d, n_cpu_moe %d", where, p.NGL, p.NCPUMoE)
		}
	default:
		t.Errorf("%s: unknown mode %q", where, p.Mode)
	}

	// The recipe must not repeat a flag serve already sets: serve puts its
	// own flags first, so a later copy would silently win.
	for _, a := range p.Args() {
		switch a {
		case "-c", "--ctx-size", "-ngl", "--n-gpu-layers", "--n-cpu-moe", "-np", "--parallel", "--device":
			t.Errorf("%s: recipe repeats %s, which overrides the plan's own value", where, a)
		}
	}

	// The effective command line: exactly one of each flag, with the plan's
	// value, so nothing is left to llama-server's defaults.
	argv := planArgv(p)
	if v, n := argValue(argv, "-c"); n != 1 || v != fmt.Sprint(p.Ctx) {
		t.Errorf("%s: command line has -c %q x%d, want %d once", where, v, n, p.Ctx)
	}
	if v, n := argValue(argv, "-ngl"); n != 1 || v != fmt.Sprint(p.NGL) {
		t.Errorf("%s: command line has -ngl %q x%d, want %d once", where, v, n, p.NGL)
	}
	v, n := argValue(argv, "--n-cpu-moe")
	if p.NCPUMoE > 0 {
		if n != 1 || v != fmt.Sprint(p.NCPUMoE) {
			t.Errorf("%s: command line has --n-cpu-moe %q x%d, want %d once", where, v, n, p.NCPUMoE)
		}
	} else if n != 0 {
		t.Errorf("%s: command line has --n-cpu-moe %q, but the plan offloads no experts", where, v)
	}

	// A plan that keeps experts in RAM reads prompts in 2048-token batches
	// and budgets 1.2GB of graphics memory for them: the flags must ask for
	// that batch size, or the budget and the run disagree.
	if p.Mode == Hybrid || p.Mode == Split {
		for _, flag := range []string{"-ub", "-b"} {
			if v, n := argValue(argv, flag); n != 1 || v != "2048" {
				t.Errorf("%s: command line has %s %q x%d, want 2048 once (the plan budgets 2048-token batches)", where, flag, v, n)
			}
		}
	}
}

// TestFlagsExplicitOnEveryPlan checks every plan for every preference over the
// generated machines: no plan may leave -c, -ngl or the offload flags to a
// llama-server default.
func TestFlagsExplicitOnEveryPlan(t *testing.T) {
	t.Setenv("EZLM_TEST_GPU_FAIL", "")
	c := cat(t)
	plans := 0
	for _, prof := range flagProfiles() {
		b := ComputeBudgets(prof.hw, prof.hw.RAMAvail)
		if b.GPU == 0 && b.RAM == 0 {
			continue
		}
		for _, pref := range []Pref{Balanced, Smartest, Fastest} {
			for _, p := range PlansFor(prof.hw, c, b, pref) {
				plans++
				checkPlanFlags(t, prof.name+"/"+string(pref), p)
			}
		}
	}
	if plans == 0 {
		t.Fatal("no plans generated")
	}
	t.Logf("checked %d plans over %d profiles", plans, len(flagProfiles()))
}

// TestFlagsModeCoverage proves the generated machines reach every placement,
// so the invariant above is not only tested for the easy GPU case.
func TestFlagsModeCoverage(t *testing.T) {
	c := cat(t)
	seen := map[Mode]int{}
	for _, prof := range flagProfiles() {
		b := ComputeBudgets(prof.hw, prof.hw.RAMAvail)
		for _, p := range PlansFor(prof.hw, c, b, Balanced) {
			seen[p.Mode]++
		}
	}
	for _, m := range []Mode{GPU, Hybrid, Split, CPU} {
		if seen[m] == 0 {
			t.Errorf("generated profiles never produce a %s plan; widen the spread", m)
		}
	}
	t.Logf("plans per mode: %v", seen)
}

// TestFlagsDegenerateLayerCounts covers a catalog entry whose layer count is
// missing or tiny: the mode must still agree with the flags, so a "hybrid" or
// "split" plan never reaches llama-server as a plain default load.
func TestFlagsDegenerateLayerCounts(t *testing.T) {
	hw := &scan.Hardware{OS: "linux", Arch: "amd64", Backend: scan.CUDA,
		RAMTotal: 64 * G, RAMAvail: 48 * G, GPUUsable: 8 * G, DiskFree: 800 * G,
		PhysCores: 8, CPUBandwidth: 60,
		GPUs: []scan.GPU{{Vendor: "nvidia", MemTotal: 8 * G, MemFree: 8 * G, Bandwidth: 500}}}
	b := ComputeBudgets(hw, hw.RAMAvail)
	for _, layers := range []int{0, 1, 2, 3} {
		for _, moe := range []bool{false, true} {
			for _, bytes := range []uint64{4 * G, 20 * G} {
				m := catalog.Model{ID: "synthetic", Name: "synthetic", Layers: layers, MoE: moe,
					ParamsB: 50, ActiveParamsB: 5, Score: 10,
					Quants: []catalog.Quant{{Name: "Q4_K_M", Bytes: bytes}}}
				c := &catalog.Catalog{Models: []catalog.Model{m}}
				name := fmt.Sprintf("layers=%d moe=%v %dGB", layers, moe, bytes/G)
				for _, p := range PlansFor(hw, c, b, Balanced) {
					checkPlanFlags(t, name, p)
				}
			}
		}
	}
}
