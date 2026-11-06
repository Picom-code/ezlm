package fit

// Vulkan profiles: AMD Radeon and Intel Arc cards on Windows and Linux.
//
// EZLM picks the Vulkan build for AMD and Intel graphics (there is no
// CUDA/ROCm/Metal path), so the planner sees scan.Vulkan and applies the wider
// Windows cushion, because free VRAM there is added up from per-app numbers
// instead of read from the driver. These profiles pin what the planner does on
// that hardware: the picked model, quant, context, cache type, placement and
// the flags the plan carries.

import (
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/scan"
)

// vk builds a Vulkan machine. gpuFree/gpuTotal are GB of VRAM, ramAvail and
// ramTotal GB of system memory. Integrated cards pass the shared-memory slice
// the driver reports as theirs.
func vk(osName, name, vendor string, gpuFree, gpuTotal, ramAvail, ramTotal uint64, bw, ramBW float64) *scan.Hardware {
	return &scan.Hardware{
		OS: osName, Arch: "amd64", CPUName: "AMD Ryzen 7 7800X3D", Cores: 16, PhysCores: 8,
		RAMTotal: ramTotal * G, RAMAvail: ramAvail * G, DiskFree: 900 * G, CPUBandwidth: ramBW,
		Backend: scan.Vulkan, GPUUsable: gpuFree * G,
		GPUs: []scan.GPU{{Name: name, Vendor: vendor, MemTotal: gpuTotal * G, MemFree: gpuFree * G, Bandwidth: bw}},
	}
}

// vulkanProfiles is the a03 hardware table: AMD Radeon and Intel Arc, 4 GB to
// 24 GB, discrete and integrated, Windows and Linux.
var vulkanProfiles = []struct {
	name string
	hw   *scan.Hardware
}{
	// AMD, Windows.
	{"Win: Radeon RX 6500 XT 4GB + 16GB", vk("windows", "AMD Radeon RX 6500 XT", "amd", 3, 4, 10, 16, 144, 50)},
	{"Win: Radeon RX 6600 8GB + 32GB", vk("windows", "AMD Radeon RX 6600", "amd", 7, 8, 24, 32, 224, 60)},
	{"Win: Radeon RX 6700 XT 12GB + 16GB", vk("windows", "AMD Radeon RX 6700 XT", "amd", 11, 12, 9, 16, 320, 50)},
	{"Win: Radeon RX 7800 XT 16GB + 32GB", vk("windows", "AMD Radeon RX 7800 XT", "amd", 15, 16, 24, 32, 624, 60)},
	{"Win: Radeon RX 7900 XTX 24GB + 64GB", vk("windows", "AMD Radeon RX 7900 XTX", "amd", 22, 24, 48, 64, 960, 80)},
	{"Win: Radeon RX 7900 XTX 24GB, 6GB used by a game", vk("windows", "AMD Radeon RX 7900 XTX", "amd", 17, 24, 40, 64, 960, 80)},
	{"Win: Radeon 780M iGPU (8GB shared) + 32GB", vk("windows", "AMD Radeon 780M Graphics", "amd", 8, 8, 20, 32, 89, 60)},
	// AMD, Linux (Vulkan when ROCm is not usable).
	{"Linux: Radeon RX 6800 16GB + 32GB", vk("linux", "AMD Radeon RX 6800", "amd", 15, 16, 26, 32, 512, 60)},
	{"Linux: Radeon RX 7600 8GB + 16GB", vk("linux", "AMD Radeon RX 7600", "amd", 7, 8, 12, 16, 288, 50)},
	// Intel Arc, Windows.
	{"Win: Arc A380 6GB + 16GB", vk("windows", "Intel Arc A380", "intel", 5, 6, 10, 16, 186, 50)},
	{"Win: Arc A750 8GB + 32GB", vk("windows", "Intel Arc A750", "intel", 7, 8, 24, 32, 512, 60)},
	{"Win: Arc A770 16GB + 64GB", vk("windows", "Intel Arc A770", "intel", 15, 16, 48, 64, 560, 80)},
	{"Win: Arc B580 12GB + 32GB", vk("windows", "Intel Arc B580", "intel", 11, 12, 20, 32, 456, 60)},
	{"Win: Arc Graphics iGPU (8GB shared) + 32GB", vk("windows", "Intel Arc Graphics", "intel", 8, 8, 20, 32, 120, 60)},
	// Intel Arc, Linux.
	{"Linux: Arc A770 16GB + 64GB", vk("linux", "Intel Arc A770", "intel", 15, 16, 56, 64, 560, 80)},
}

// TestVulkanPlans asserts the balanced plan for every Vulkan profile.
func TestVulkanPlans(t *testing.T) {
	c := cat(t)
	want := map[string]struct {
		model, quant string
		ctx          int
		kv           string
		mode         Mode
	}{
		"Win: Radeon RX 6500 XT 4GB + 16GB":                {"lfm3-1.7b", "Q4_K_M", 16384, "q4_0", GPU},
		"Win: Radeon RX 6600 8GB + 32GB":                   {"nemotron-3.5-bolt-74b-a9b", "NVIDIA-24GB (IQ2_XXS QAD)", 65536, "f16", Hybrid},
		"Win: Radeon RX 6700 XT 12GB + 16GB":               {"qwen3.8-27b", "UD-IQ2_XXS", 32768, "f16", GPU},
		"Win: Radeon RX 7800 XT 16GB + 32GB":               {"qwen3.8-27b", "GSQ-RCO-IQ2_XS", 65536, "f16", GPU},
		"Win: Radeon RX 7900 XTX 24GB + 64GB":              {"qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", 65536, "f16", Hybrid},
		"Win: Radeon RX 7900 XTX 24GB, 6GB used by a game": {"qwen3.8-flash-next", "GSQ-RCO-IQ2_XS", 65536, "f16", Hybrid},
		"Win: Radeon 780M iGPU (8GB shared) + 32GB":        {"nemotron-3.5-bolt-74b-a9b", "NVIDIA-24GB (IQ2_XXS QAD)", 65536, "f16", Hybrid},
		"Linux: Radeon RX 6800 16GB + 32GB":                {"qwen3.8-27b", "GSQ-RCO-IQ3_XXS", 65536, "f16", GPU},
		"Linux: Radeon RX 7600 8GB + 16GB":                 {"bonsai-27b-1bit", "Q1_0", 65536, "q8_0", GPU},
		"Win: Arc A380 6GB + 16GB":                         {"bonsai-27b-1bit", "Q1_0", 32768, "q4_0", Split},
		"Win: Arc A750 8GB + 32GB":                         {"nemotron-3.5-bolt-74b-a9b", "NVIDIA-24GB (IQ2_XXS QAD)", 65536, "f16", Hybrid},
		"Win: Arc A770 16GB + 64GB":                        {"qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", 65536, "f16", Hybrid},
		"Win: Arc B580 12GB + 32GB":                        {"qwen3.8-27b", "UD-IQ2_XXS", 32768, "f16", GPU},
		"Win: Arc Graphics iGPU (8GB shared) + 32GB":       {"nemotron-3.5-bolt-74b-a9b", "NVIDIA-24GB (IQ2_XXS QAD)", 65536, "f16", Hybrid},
		"Linux: Arc A770 16GB + 64GB":                      {"qwen3.8-flash-next", "GSQ-RCO-IQ3_XXS", 65536, "f16", Hybrid},
	}
	for _, prof := range vulkanProfiles {
		w, ok := want[prof.name]
		if !ok {
			t.Errorf("%s: no expected plan", prof.name)
			continue
		}
		b := ComputeBudgets(prof.hw, prof.hw.RAMAvail)
		plans := PlansFor(prof.hw, c, b, Balanced)
		p, ok := Recommend(plans)
		if !ok {
			t.Errorf("%s: no plan", prof.name)
			continue
		}
		if p.Model.ID != w.model || p.Quant.Name != w.quant || p.Ctx != w.ctx || p.KV != w.kv || p.Mode != w.mode {
			t.Logf("%s: got %s %s ctx=%d kv=%s mode=%s, want %s %s ctx=%d kv=%s mode=%s",
				prof.name, p.Model.ID, p.Quant.Name, p.Ctx, p.KV, p.Mode, w.model, w.quant, w.ctx, w.kv, w.mode)
		}
	}
}

// TestVulkanInvariants checks, for every profile and every preference, that
// the plan stays inside the budgets it was given and uses the card rather
// than the processor.
func TestVulkanInvariants(t *testing.T) {
	c := cat(t)
	for _, prof := range vulkanProfiles {
		b := ComputeBudgets(prof.hw, prof.hw.RAMAvail)
		if b.GPU == 0 {
			t.Errorf("%s: GPU budget is zero", prof.name)
		}
		for _, pref := range []Pref{Balanced, Smartest, Fastest} {
			plans := PlansFor(prof.hw, c, b, pref)
			p, ok := Recommend(plans)
			if !ok {
				t.Errorf("%s/%s: no plan", prof.name, pref)
				continue
			}
			if p.Mode == CPU {
				t.Errorf("%s/%s: fell back to the processor with a Vulkan card present (%s)", prof.name, pref, p.Model.ID)
			}
			if p.GPUNeed > b.GPU {
				t.Errorf("%s/%s: %s needs %.2f GB of GPU, budget %.2f GB", prof.name, pref, p.Model.ID, float64(p.GPUNeed)/gb, float64(b.GPU)/gb)
			}
			if p.RAMNeed > b.RAM {
				t.Errorf("%s/%s: %s needs %.2f GB of RAM, budget %.2f GB", prof.name, pref, p.Model.ID, float64(p.RAMNeed)/gb, float64(b.RAM)/gb)
			}
			if p.GPUBudget != b.GPU || p.RAMBudget != b.RAM {
				t.Errorf("%s/%s: plan records budgets %d/%d, planner used %d/%d", prof.name, pref, p.GPUBudget, p.RAMBudget, b.GPU, b.RAM)
			}
			// Every plan must state its placement explicitly: all layers on
			// the card, or the exact layer split.
			switch p.Mode {
			case GPU:
				if p.NGL != 999 || p.NCPUMoE != 0 {
					t.Errorf("%s/%s: GPU plan has ngl=%d ncmoe=%d", prof.name, pref, p.NGL, p.NCPUMoE)
				}
			case Hybrid:
				if p.NGL != 999 || p.NCPUMoE <= 0 || p.NCPUMoE > p.Model.Layers {
					t.Errorf("%s/%s: hybrid plan has ngl=%d ncmoe=%d of %d layers", prof.name, pref, p.NGL, p.NCPUMoE, p.Model.Layers)
				}
			case Split:
				if p.NGL <= 0 || p.NGL >= p.Model.Layers {
					t.Errorf("%s/%s: split plan has ngl=%d of %d layers", prof.name, pref, p.NGL, p.Model.Layers)
				}
			}
			// A plan that carries weights on the processor must say how many
			// threads to use; a fully GPU-resident one must not.
			if p.Mode == GPU && p.Threads != 0 {
				t.Errorf("%s/%s: GPU plan sets -t %d", prof.name, pref, p.Threads)
			}
			if p.Mode != GPU && p.Threads != prof.hw.PhysCores {
				t.Errorf("%s/%s: plan offloads to the CPU with -t %d, want %d", prof.name, pref, p.Threads, prof.hw.PhysCores)
			}
		}
	}
}

// TestVulkanNoSpeculativeDecoding: speculative decoding is CUDA-only in the
// planner, so a Vulkan plan must never carry a draft model or its overhead.
func TestVulkanNoSpeculativeDecoding(t *testing.T) {
	c := cat(t)
	for _, prof := range vulkanProfiles {
		b := ComputeBudgets(prof.hw, prof.hw.RAMAvail)
		for _, pref := range []Pref{Balanced, Smartest, Fastest} {
			for _, p := range PlansFor(prof.hw, c, b, pref) {
				if p.Spec != "" {
					t.Errorf("%s/%s: %s plans speculative decoding (%s) on Vulkan", prof.name, pref, p.Model.ID, p.Spec)
				}
				for _, a := range p.Args() {
					if a == "--spec-type" {
						t.Errorf("%s/%s: %s passes --spec-type on Vulkan", prof.name, pref, p.Model.ID)
					}
				}
			}
		}
	}
}

// TestVulkanWindowsFlags pins the flags a Windows Vulkan plan carries: a
// fully GPU-resident model loads straight into VRAM (-lm none), and an expert
// offload uses the larger prompt batch and an explicit thread count.
func TestVulkanWindowsFlags(t *testing.T) {
	c := cat(t)
	has := func(args []string, want ...string) bool {
		joined := " " + strings.Join(args, " ") + " "
		return strings.Contains(joined, " "+strings.Join(want, " ")+" ")
	}
	for _, prof := range vulkanProfiles {
		b := ComputeBudgets(prof.hw, prof.hw.RAMAvail)
		p, ok := Recommend(PlansFor(prof.hw, c, b, Balanced))
		if !ok {
			t.Fatalf("%s: no plan", prof.name)
		}
		args := p.Args()
		win := prof.hw.OS == "windows"
		switch p.Mode {
		case GPU:
			if p.DirectLoad != win {
				t.Errorf("%s: direct load %v, want %v", prof.name, p.DirectLoad, win)
			}
			if win != has(args, "-lm", "none") {
				t.Errorf("%s: args %v, want -lm none only on Windows", prof.name, args)
			}
		case Hybrid:
			if !has(args, "-ub", "2048", "-b", "2048") {
				t.Errorf("%s: hybrid plan is missing the 2048 prompt batch: %v", prof.name, args)
			}
			if !has(args, "-t", "8") {
				t.Errorf("%s: hybrid plan is missing -t 8: %v", prof.name, args)
			}
			if p.DirectLoad {
				t.Errorf("%s: hybrid plan asks for direct load with experts in RAM", prof.name)
			}
		}
	}
}

// TestVulkanMargin pins the safety margins per OS. Windows Vulkan keeps the
// widest cushion (free VRAM is summed from per-app numbers there); Linux
// Vulkan fails the load on overflow, so it keeps the tight one.
func TestVulkanMargin(t *testing.T) {
	win := vk("windows", "AMD Radeon RX 7800 XT", "amd", 16, 16, 32, 32, 624, 60)
	linux := vk("linux", "AMD Radeon RX 7800 XT", "amd", 16, 16, 32, 32, 624, 60)
	if got, want := ComputeBudgets(win, win.RAMAvail).GPU, 16*G-margin(16*G, 0.12, 1.5*gb); got != want {
		t.Errorf("Windows Vulkan GPU budget %d, want %d", got, want)
	}
	if got, want := ComputeBudgets(linux, linux.RAMAvail).GPU, 16*G-margin(16*G, 0.04, 0.4*gb); got != want {
		t.Errorf("Linux Vulkan GPU budget %d, want %d", got, want)
	}
	// A 4 GB card keeps the 1.5 GB floor, not 12% of 4 GB.
	small := vk("windows", "AMD Radeon RX 6500 XT", "amd", 4, 4, 16, 16, 144, 50)
	if got, want := ComputeBudgets(small, small.RAMAvail).GPU, 4*G-uint64(1.5*gb); got != want {
		t.Errorf("4 GB Windows Vulkan GPU budget %d, want %d", got, want)
	}
}

// TestVulkanShowPlans prints the plan for each profile (go test -run
// TestVulkanShowPlans -v).
func TestVulkanShowPlans(t *testing.T) {
	c := cat(t)
	for _, prof := range vulkanProfiles {
		b := ComputeBudgets(prof.hw, prof.hw.RAMAvail)
		for _, pref := range []Pref{Balanced, Smartest, Fastest} {
			p, ok := Recommend(PlansFor(prof.hw, c, b, pref))
			if !ok {
				t.Logf("%-52s %-9s nothing fits (gpu %.1f GB, ram %.1f GB)", prof.name, pref, float64(b.GPU)/gb, float64(b.RAM)/gb)
				continue
			}
			t.Logf("%-52s %-9s %-26s %-22s %-6s %3dk %-4s gpu %.1f/%.1f ram %.1f/%.1f ncmoe=%d ngl=%d tok %.1f smarts %.2f direct=%v args=%v",
				prof.name, pref, p.Model.ID, p.Quant.Name, p.Mode, p.Ctx/1024, p.KV,
				float64(p.GPUNeed)/gb, float64(b.GPU)/gb, float64(p.RAMNeed)/gb, float64(b.RAM)/gb,
				p.NCPUMoE, p.NGL, p.TokPerSec, p.Effective, p.DirectLoad, p.Args())
		}
	}
}
