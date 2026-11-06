package scan

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func profilerFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Apple Silicon: one Apple GPU, no VRAM of its own, and a display section
// that must not be mistaken for another card.
func TestProfilerDisplaysAppleSilicon(t *testing.T) {
	gpus := parseProfilerDisplays(profilerFixture(t, "system_profiler_displays_apple_m3_pro.txt"))
	if len(gpus) != 1 {
		t.Fatalf("got %d cards: %+v", len(gpus), gpus)
	}
	g := gpus[0]
	if g.Name != "Apple M3 Pro" || g.Vendor != "apple" || g.Bus != "Built-In" || !g.Metal {
		t.Fatalf("card: %+v", g)
	}
	if g.MemTotal != 0 {
		t.Fatalf("Apple Silicon shares RAM, so no VRAM of its own: %d", g.MemTotal)
	}
	if got := metalGPUs(gpus); len(got) != 0 {
		t.Fatalf("the Apple GPU is handled by the unified-memory path, not as a discrete card: %+v", got)
	}
}

// Intel Mac with built-in graphics, a discrete Radeon and an external GPU,
// with no display attached: the discrete cards must still be found.
func TestProfilerDisplaysIntelEGPUHeadless(t *testing.T) {
	all := parseProfilerDisplays(profilerFixture(t, "system_profiler_displays_intel_egpu_headless.txt"))
	if len(all) != 3 {
		t.Fatalf("got %d cards: %+v", len(all), all)
	}
	if all[0].Name != "Intel UHD Graphics 630" || all[0].Vendor != "intel" || all[0].MemTotal != 1536<<20 {
		t.Fatalf("built-in graphics: %+v", all[0])
	}
	if all[2].Name != "Radeon RX 580" || all[2].Vendor != "amd" || all[2].MemTotal != 8*GB || all[2].Bus != "PCIe" {
		t.Fatalf("external GPU: %+v", all[2])
	}

	gpus := metalGPUs(all)
	if len(gpus) != 2 {
		t.Fatalf("got %d usable cards: %+v", len(gpus), gpus)
	}
	if gpus[0].Name != "Radeon RX 580" || gpus[0].MemTotal != 8*GB || gpus[0].MemFree != 8*GB {
		t.Fatalf("biggest card first: %+v", gpus[0])
	}
	if gpus[1].Name != "Radeon Pro 5500M" || gpus[1].MemTotal != 4*GB {
		t.Fatalf("discrete card: %+v", gpus[1])
	}
}

// A headless Mac mini: no "Displays:" section at all.
func TestProfilerDisplaysHeadless(t *testing.T) {
	gpus := parseProfilerDisplays(profilerFixture(t, "system_profiler_displays_apple_headless.txt"))
	if len(gpus) != 1 || gpus[0].Name != "Apple M2" || gpus[0].Vendor != "apple" {
		t.Fatalf("got %+v", gpus)
	}
}

// Built-in Intel graphics are no faster than the processor, so a Mac with
// only those stays on the CPU build.
func TestProfilerDisplaysIntelBuiltInOnly(t *testing.T) {
	gpus := parseProfilerDisplays(profilerFixture(t, "system_profiler_displays_intel_igpu_only.txt"))
	if len(gpus) != 1 || gpus[0].MemTotal != 1536<<20 {
		t.Fatalf("got %+v", gpus)
	}
	if got := metalGPUs(gpus); len(got) != 0 {
		t.Fatalf("built-in graphics must stay with the processor: %+v", got)
	}
}

// macOS 10.14 to 11 spelled Metal support differently and printed VRAM in MB.
func TestProfilerDisplaysLegacyMetal(t *testing.T) {
	all := parseProfilerDisplays(profilerFixture(t, "system_profiler_displays_intel_legacy_metal.txt"))
	if len(all) != 2 {
		t.Fatalf("got %d cards: %+v", len(all), all)
	}
	if all[1].Vendor != "amd" || all[1].MemTotal != 4096<<20 || !all[1].Metal {
		t.Fatalf("ATI vendor and 4096 MB must still parse: %+v", all[1])
	}
	gpus := metalGPUs(all)
	if len(gpus) != 1 || gpus[0].Name != "Radeon Pro 560X" {
		t.Fatalf("got %+v", gpus)
	}
}

// A card without Metal support, or with less than 2 GB of its own memory,
// cannot run the Metal build usefully.
func TestProfilerDisplaysUnusableCards(t *testing.T) {
	const out = `Graphics/Displays:

    NVIDIA GeForce GTX 1080:

      Chipset Model: NVIDIA GeForce GTX 1080
      Type: GPU
      Bus: PCIe
      VRAM (Total): 8 GB
      Vendor: NVIDIA (0x10de)

    Radeon RX 560:

      Chipset Model: Radeon RX 560
      Type: GPU
      Bus: PCIe
      VRAM (Total): 1 GB
      Vendor: AMD (0x1002)
      Metal Support: Metal 3
`
	if got := metalGPUs(parseProfilerDisplays(out)); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestProfilerDisplaysEmpty(t *testing.T) {
	for _, in := range []string{"", "Graphics/Displays:\n\n", "    Truncated Header:\n"} {
		_ = parseProfilerDisplays(in) // must not panic
	}
	if gpus := parseProfilerDisplays(""); len(gpus) != 0 {
		t.Fatalf("nothing in, nothing out: %+v", gpus)
	}
}

func TestProfilerHardware(t *testing.T) {
	apple := parseProfilerHardware(profilerFixture(t, "system_profiler_hardware_apple_m3_pro.txt"))
	if apple.Chip != "Apple M3 Pro" || apple.RAMTotal != 18*GB || apple.Cores != 11 {
		t.Fatalf("Apple Silicon: %+v", apple)
	}
	if apple.PerfCores != 5 || apple.EffCores != 6 {
		t.Fatalf("core split: %+v", apple)
	}

	intel := parseProfilerHardware(profilerFixture(t, "system_profiler_hardware_intel.txt"))
	if intel.Chip != "6-Core Intel Core i7" || intel.RAMTotal != 16*GB || intel.Cores != 6 {
		t.Fatalf("Intel: %+v", intel)
	}
	if intel.PerfCores != 0 || intel.EffCores != 0 {
		t.Fatalf("Intel Macs have no performance/efficiency split: %+v", intel)
	}
}

// macOS 26 capitalised "Performance" and "Efficiency"; earlier versions did
// not, so both must read.
func TestProfilerCoreCounts(t *testing.T) {
	for in, want := range map[string][2]int{
		"12 (8 Performance and 4 Efficiency)": {8, 4},
		"12 (8 performance and 4 efficiency)": {8, 4},
		"6":                                   {0, 0},
		"10 (4 performance and 6 efficiency)": {4, 6},
	} {
		if p, e := profilerCoreCounts(in); p != want[0] || e != want[1] {
			t.Errorf("%q: got %d/%d, want %d/%d", in, p, e, want[0], want[1])
		}
	}
}

func TestProfilerBytes(t *testing.T) {
	for in, want := range map[string]uint64{
		"1536 MB": 1536 << 20,
		"4096 MB": 4096 << 20,
		"8 GB":    8 * GB,
		"18 GB":   18 * GB,
		"1.5 GB":  1610612736,
		"512 KB":  512 << 10,
		"1 TB":    1 << 40,
		"0 MB":    0,
		"8GB":     0, // the profiler always separates the number and unit
		"":        0,
	} {
		if got := profilerBytes(in); got != want {
			t.Errorf("%q: got %d, want %d", in, got, want)
		}
	}
}

// Reads this Mac's own report, so a change in Apple's text format shows up
// here. Read-only; skipped away from macOS.
func TestReadsThisMacsProfiler(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("system_profiler is macOS only")
	}
	ctx := context.Background()
	out, err := run(ctx, "system_profiler", "SPDisplaysDataType")
	if err != nil {
		t.Fatalf("SPDisplaysDataType: %v", err)
	}
	gpus := parseProfilerDisplays(out)
	if len(gpus) == 0 {
		t.Fatalf("no graphics cards read from %q", out)
	}
	for _, g := range gpus {
		t.Logf("card: %q by %q, %d GB, bus %q, metal %v", g.Name, g.Vendor, g.MemTotal/GB, g.Bus, g.Metal)
	}

	hwOut, err := run(ctx, "system_profiler", "SPHardwareDataType")
	if err != nil {
		t.Fatalf("SPHardwareDataType: %v", err)
	}
	hw := parseProfilerHardware(hwOut)
	if hw.Chip == "" || hw.RAMTotal == 0 {
		t.Fatalf("hardware not read: %+v", hw)
	}
	t.Logf("Mac: %s, chip %q, %d GB, %d cores (%d performance, %d efficiency)",
		hw.Model, hw.Chip, hw.RAMTotal/GB, hw.Cores, hw.PerfCores, hw.EffCores)
}
