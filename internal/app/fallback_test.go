package app

import (
	"slices"
	"testing"

	"github.com/picom-code/ezlm/internal/fit"
	"github.com/picom-code/ezlm/internal/scan"
	"github.com/picom-code/ezlm/internal/serve"
)

// When an AMD or Intel card can't load a model, the same model runs on the
// processor (with no graphics device in the way) if it fits in memory there.
func TestCPUInstead(t *testing.T) {
	const G = scan.GB
	amd := &scan.Hardware{OS: "windows", Backend: scan.Vulkan, RAMTotal: 32 * G, RAMAvail: 24 * G, GPUUsable: 16 * G}
	cfg := &serve.Config{NGL: 999, NCPUMoE: 4, ExtraArgs: []string{"-ctk", "q8_0", "-ctv", "q8_0"}}
	plan := fit.Plan{GPUNeed: 9 * G, RAMNeed: 1 * G}

	cpu, ok := cpuInstead(cfg, amd, plan)
	if !ok || cpu.NGL != 0 || cpu.NCPUMoE != 0 || !slices.Equal(cpu.ExtraArgs, []string{"-ctk", "q8_0", "-ctv", "q8_0", "--device", "none"}) {
		t.Fatalf("got ok=%v %+v", ok, cpu)
	}
	if cfg.NGL != 999 || len(cfg.ExtraArgs) != 4 {
		t.Fatal("the original setup must stay as it was")
	}
	if _, ok := cpuInstead(cfg, amd, fit.Plan{GPUNeed: 15 * G, RAMNeed: 8 * G}); ok {
		t.Fatal("a model too big for system memory has no processor fallback")
	}
	if _, ok := cpuInstead(cpu, amd, plan); ok {
		t.Fatal("already on the processor: nothing to fall back to")
	}
	nvidia := *amd
	nvidia.Backend = scan.CUDA
	if _, ok := cpuInstead(cfg, &nvidia, plan); ok {
		t.Fatal("NVIDIA failures are reported, not hidden behind a slow fallback")
	}
	if got := firstLine("the model server stopped while loading. Last log lines:\nfoo"); got != "the model server stopped while loading" {
		t.Fatalf("firstLine: %q", got)
	}
}

// Only failures that point at the graphics card fall back to the processor:
// a slow load, a busy port or a damaged model file would fail there too.
func TestDriverFailure(t *testing.T) {
	for msg, want := range map[string]bool{
		serve.ErrStopped + ". Last log lines:\nfoo":                                      true,
		"alloc_tensor_range: failed to allocate Vulkan0 buffer of size 123":              true,
		"ggml_vulkan: Device memory allocation of size 1 failed: ErrorOutOfDeviceMemory": true,
		"the model did not finish loading in 10m0s. Last log lines:\n...":                false,
		"couldn't bind HTTP server socket, hostname: 127.0.0.1, port: 8080":              false,
		"error loading model: tensor data is not within the file bounds":                 false,
	} {
		if got := driverFailure(msg); got != want {
			t.Errorf("driverFailure(%q) = %v, want %v", msg, got, want)
		}
	}
}
