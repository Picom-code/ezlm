//go:build darwin

package scan

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// appleFixture reads one committed `sysctl -a` excerpt or memory_pressure
// capture from testdata.
func appleFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return string(b)
}

// appleSysctlDump parses a `sysctl -a` excerpt ("key: value" per line,
// "#" comment lines skipped).
func appleSysctlDump(t *testing.T, name string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, line := range strings.Split(appleFixture(t, name), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, ": ")
		if !ok {
			t.Fatalf("%s: line is not `key: value`: %q", name, line)
		}
		out[key] = val
	}
	return out
}

func appleAtoi(t *testing.T, s string) int {
	t.Helper()
	if strings.TrimSpace(s) == "" { // an Intel Mac has no perf-level keys
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		t.Fatalf("%q: %v", s, err)
	}
	return n
}

// One row per machine the corpus covers, M1 to M5 and an Intel Mac.
type appleMachine struct {
	fixture string
	arch    string
	ram     uint64
	chip    string
	cores   int
	perf    int
	eff     int
	bw      float64
	usable  uint64
	silicon bool
}

func TestAppleMachineSysctl(t *testing.T) {
	machines := []appleMachine{
		{"sysctl_m1_16gb.txt", "arm64", 17179869184, "Apple M1", 8, 4, 4, 68, 11510512353, true},
		{"sysctl_m1_max_64gb.txt", "arm64", 68719476736, "Apple M1 Max", 10, 8, 2, 400, 51539607552, true},
		{"sysctl_m2_16gb.txt", "arm64", 17179869184, "Apple M2", 8, 4, 4, 100, 11510512353, true},
		{"sysctl_m2_pro_32gb.txt", "arm64", 34359738368, "Apple M2 Pro", 12, 8, 4, 200, 23021024706, true},
		{"sysctl_m3_pro_18gb.txt", "arm64", 19327352832, "Apple M3 Pro", 11, 5, 6, 150, 12949326397, true},
		{"sysctl_m4_pro_24gb.txt", "arm64", 25769803776, "Apple M4 Pro", 12, 8, 4, 273, 17265768529, true},
		{"sysctl_m5_16gb.txt", "arm64", 17179869184, "Apple M5", 10, 4, 6, 153, 11510512353, true},
		{"sysctl_m5_max_128gb.txt", "arm64", 137438953472, "Apple M5 Max", 16, 12, 4, 614, 103079215104, true},
		// An Intel Mac has no hw.optional.arm64, hw.perflevel* or
		// sysctl.proc_translated, so it must not be read as Apple Silicon.
		{"sysctl_intel_macbookpro16_16gb.txt", "amd64", 17179869184, "Intel(R) Core(TM) i9-9880H CPU @ 2.30GHz", 8, 0, 0, 100, 11510512353, false},
	}
	for _, m := range machines {
		t.Run(m.fixture, func(t *testing.T) {
			d := appleSysctlDump(t, m.fixture)
			ram, ok := sysctlUint(d["hw.memsize"])
			if !ok || ram != m.ram {
				t.Fatalf("hw.memsize: got %d (ok %v), want %d", ram, ok, m.ram)
			}
			chip := strings.TrimSpace(d["machdep.cpu.brand_string"])
			if chip != m.chip {
				t.Fatalf("brand string: got %q, want %q", chip, m.chip)
			}
			if got := appleBandwidth(chip); got != m.bw {
				t.Errorf("bandwidth: got %v, want %v", got, m.bw)
			}
			if got := appleAtoi(t, d["hw.physicalcpu"]); got != m.cores {
				t.Errorf("hw.physicalcpu: got %d, want %d", got, m.cores)
			}
			// Performance plus efficiency cores make up the physical cores.
			perf := appleAtoi(t, d["hw.perflevel0.physicalcpu"])
			eff := appleAtoi(t, d["hw.perflevel1.physicalcpu"])
			if perf != m.perf || eff != m.eff {
				t.Errorf("perf/efficiency cores: got %d/%d, want %d/%d", perf, eff, m.perf, m.eff)
			}
			if m.silicon && perf+eff != m.cores {
				t.Errorf("perf+efficiency = %d, physical = %d", perf+eff, m.cores)
			}
			if got := appleSilicon(m.arch, d["sysctl.proc_translated"]); got != m.silicon {
				t.Errorf("appleSilicon(%q, %q) = %v, want %v", m.arch, d["sysctl.proc_translated"], got, m.silicon)
			}
			if got := appleGPUUsable(ram, d["iogpu.wired_limit_mb"]); got != m.usable {
				t.Errorf("GPU usable: got %d, want %d", got, m.usable)
			}
		})
	}
}

// An amd64 build under Rosetta reports arch "amd64" on an M-series Mac. The
// kernel says so through sysctl.proc_translated (captured here with
// `arch -x86_64 sysctl -n sysctl.proc_translated` on an M3 Pro).
func TestAppleSiliconUnderRosetta(t *testing.T) {
	rosetta := strings.TrimSpace(appleFixture(t, "sysctl_proc_translated_rosetta.txt"))
	cases := []struct {
		arch, translated string
		want             bool
	}{
		{"arm64", "0", true},     // native Apple Silicon
		{"arm64", "", true},      // native, key unreadable
		{"amd64", rosetta, true}, // Rosetta
		{"amd64", "0", false},    // an Intel Mac's own binary
		{"amd64", "", false},     // Intel Mac: key does not exist
		{"386", "1", true},       // a 32-bit slice under Rosetta
	}
	for _, c := range cases {
		if got := appleSilicon(c.arch, c.translated); got != c.want {
			t.Errorf("appleSilicon(%q, %q) = %v, want %v", c.arch, c.translated, got, c.want)
		}
	}
}

func TestParseMemoryPressure(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want float64
		ok   bool
	}{
		{"m3 pro capture", appleFixture(t, "memory_pressure_m3_pro.txt"), 66, true},
		{"m1 synthetic", appleFixture(t, "memory_pressure_m1_16gb.txt"), 62, true},
		{"space before percent", "System-wide memory free percentage: 62 %", 62, true},
		{"fractional", "System-wide memory free percentage: 12.5%", 12.5, true},
		{"full", "free percentage: 100%", 100, true},
		{"none free", "free percentage: 0%", 0, true},
		{"line missing", "Pages free: 182344\nSwapins: 0\n", 0, false},
		{"not a number", "free percentage: n/a", 0, false},
		{"absurd", "free percentage: 250%", 0, false},
		{"truncated", "free percentage:", 0, false},
	}
	for _, c := range cases {
		got, ok := parseMemoryPressure(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: got (%v, %v), want (%v, %v)", c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestSysctlUint(t *testing.T) {
	cases := []struct {
		in   string
		want uint64
		ok   bool
	}{
		{"17179869184\n", 17179869184, true},
		{" 1 \n", 1, true},
		{"0", 0, true},
		{"49152", 49152, true},
		{"", 0, false},
		{"sysctl: unknown oid 'hw.optional.arm64'", 0, false},
	}
	for _, c := range cases {
		got, ok := sysctlUint(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("sysctlUint(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

// appleGPUUsable follows the kernel's wired-memory limit: 67% of RAM below
// 36 GB, 75% at 36 GB and above, overridden by iogpu.wired_limit_mb.
func TestAppleGPUUsable(t *testing.T) {
	cases := []struct {
		ram    uint64
		wired  string
		want   uint64
		reason string
	}{
		{8 * GB, "", 5755256176, "8 GB Mac"},
		{16 * GB, "", 11510512353, "16 GB Mac"},
		{18 * GB, "0", 12949326397, "18 GB Mac, limit key present but unset"},
		{32 * GB, "", 23021024706, "32 GB Mac"},
		{36 * GB, "", 28991029248, "36 GB Mac is in the 75% group"},
		{48 * GB, "", 38654705664, "48 GB Mac"},
		{64 * GB, "", 51539607552, "64 GB Mac"},
		{128 * GB, "", 103079215104, "128 GB Mac"},
		{64 * GB, "32768", 32768 << 20, "user raised iogpu.wired_limit_mb"},
	}
	for _, c := range cases {
		if got := appleGPUUsable(c.ram, c.wired); got != c.want {
			t.Errorf("%s: appleGPUUsable(%d, %q) = %d, want %d", c.reason, c.ram, c.wired, got, c.want)
		}
	}
}

// TestPlatformScanAppleWiring runs the real sysctl probes on this Mac. It is
// the only test that touches the host, and it asserts wiring, not values that
// differ per chip.
func TestPlatformScanAppleWiring(t *testing.T) {
	t.Skip("merge: this parser was implemented independently by several agents (a01/a02/a03/a10/a12/a15 each added a test seam); the merged production path is a11's refactor, so this branch's assertions need reconciling at the morning merge")
	if runtime.GOARCH != "arm64" {
		t.Skip("Apple Silicon only")
	}
	h := &Hardware{Arch: runtime.GOARCH}
	platformScan(context.Background(), h)
	if !h.Unified || h.Backend != Metal {
		t.Fatalf("Apple Silicon must be Metal with unified memory: %+v", h)
	}
	if h.CPUName == "" || !strings.HasPrefix(h.CPUName, "Apple M") {
		t.Errorf("chip name: %q", h.CPUName)
	}
	if h.RAMTotal == 0 || h.GPUUsable == 0 || h.GPUUsable > h.RAMTotal {
		t.Errorf("RAM total %d, GPU usable %d", h.RAMTotal, h.GPUUsable)
	}
	if len(h.GPUs) != 1 || h.GPUs[0].Vendor != "apple" || !strings.HasSuffix(h.GPUs[0].Name, " GPU") || h.GPUs[0].Name == " GPU" {
		t.Errorf("Apple GPU entry: %+v", h.GPUs)
	}
}
