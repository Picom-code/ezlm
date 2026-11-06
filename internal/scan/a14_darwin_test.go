package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// fakeTool puts a shell script named name first on PATH, so platformScan's
// sysctl and memory_pressure calls hit the script instead of the machine.
func fakeTool(t *testing.T, name, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestGPUUsable(t *testing.T) {
	gb8, gb16, gb36, gb64 := uint64(8*GB), uint64(16*GB), uint64(36*GB), uint64(64*GB)
	tests := []struct {
		name    string
		ram     uint64
		wiredMB uint64
		want    uint64
	}{
		{"8 GB default is two thirds", gb8, 0, uint64(float64(gb8) * 0.67)},
		{"36 GB still uses two thirds", gb36, 0, uint64(float64(gb36) * 0.67)},
		{"64 GB uses three quarters", gb64, 0, uint64(float64(gb64) * 0.75)},
		{"user limit below RAM is used", gb16, 8192, 8192 << 20},
		{"user limit equal to RAM is kept", gb16, 16384, gb16},
		{"user limit above RAM is clamped", gb16, 32768, gb16},
		{"absurd user limit cannot overflow", gb16, 1 << 40, gb16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gpuUsable(tt.ram, tt.wiredMB); got != tt.want {
				t.Errorf("gpuUsable(%d, %d) = %d, want %d", tt.ram, tt.wiredMB, got, tt.want)
			}
		})
	}
}

func TestPlatformScanClampsWiredLimitToRAM(t *testing.T) {
	fakeTool(t, "sysctl", `#!/bin/sh
case "$*" in
	*iogpu.wired_limit_mb*) echo 32768 ;;
	*machdep.cpu.brand_string*) echo "Apple M3 Pro" ;;
	*) exit 1 ;;
esac
`)
	fakeTool(t, "memory_pressure", "#!/bin/sh\necho 'System-wide memory free percentage: 50%'\n")

	h := &Hardware{Arch: "arm64", RAMTotal: 16 * GB, RAMAvail: 4 * GB}
	platformScan(context.Background(), h)

	if h.GPUUsable > h.RAMTotal {
		t.Fatalf("GPU usable memory %d is more than the machine's RAM %d", h.GPUUsable, h.RAMTotal)
	}
	if h.GPUUsable != h.RAMTotal {
		t.Errorf("GPU usable memory = %d, want %d", h.GPUUsable, h.RAMTotal)
	}
	if len(h.GPUs) != 1 || h.GPUs[0].MemTotal != h.GPUUsable {
		t.Errorf("GPU memory total does not match usable memory: %+v", h.GPUs)
	}
}

func TestPlatformScanUsesMemoryPressure(t *testing.T) {
	fakeTool(t, "sysctl", `#!/bin/sh
case "$*" in
	*iogpu.wired_limit_mb*) echo 0 ;;
	*machdep.cpu.brand_string*) echo "Apple M2" ;;
	*) exit 1 ;;
esac
`)
	fakeTool(t, "memory_pressure", "#!/bin/sh\necho 'System-wide memory free percentage: 25%'\n")

	h := &Hardware{Arch: "arm64", RAMTotal: 16 * GB, RAMAvail: 2 * GB}
	platformScan(context.Background(), h)

	want := uint64(float64(16*GB) * 0.25)
	if h.RAMAvail != want {
		t.Errorf("available memory = %d, want %d from memory_pressure", h.RAMAvail, want)
	}
}

func TestPlatformScanIntelFallsBackToCPU(t *testing.T) {
	h := &Hardware{Arch: "amd64", RAMTotal: 16 * GB}
	platformScan(context.Background(), h)

	if h.Backend != CPU {
		t.Errorf("backend = %q, want %q", h.Backend, CPU)
	}
	if h.Unified {
		t.Error("an Intel Mac is not unified memory")
	}
	if len(h.Notes) == 0 {
		t.Error("an Intel Mac should explain that models run on the CPU")
	}
}

func TestAppleBandwidth(t *testing.T) {
	tests := []struct {
		chip string
		want float64
	}{
		{"Apple M1", 68},
		{"Apple M1 Max", 400},
		{"Apple M2 Pro", 200},
		{"Apple M3 Ultra", 819},
		{"Apple M4 Pro", 273},
		{"Apple M5 Max", 614},
		{"Apple M2", 100},
	}
	for _, tt := range tests {
		if got := appleBandwidth(tt.chip); got != tt.want {
			t.Errorf("appleBandwidth(%q) = %v, want %v", tt.chip, got, tt.want)
		}
	}
}
