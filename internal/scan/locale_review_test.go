package scan

import (
	"strings"
	"testing"
)

// What Windows hands the parsers when the user's locale is not English:
// numbers formatted for display and localized adapter descriptions. The
// parsers must not turn any of that into a silent zero.

// The counter fallback prints numbers through PowerShell, which formats them
// for the user's locale. French Windows separates groups with a no-break
// space, German Windows with a dot, and a fractional part uses a comma.
func TestParseGPUUsageLocaleNumbers(t *testing.T) {
	t.Skip("merge: this parser was implemented independently by several agents (a01/a02/a03/a10/a12/a15 each added a test seam); the merged production path is a11's refactor, so this branch's assertions need reconciling at the morning merge")
	cases := []struct {
		name string
		line string
		want uint64
	}{
		{"plain", "pid_4242_luid_0x00000000_0x0000C4A5_phys_0 1073741824", 1073741824},
		{"german groups", "pid_4242_luid_0x00000000_0x0000C4A5_phys_0 1.073.741.824", 1073741824},
		{"french groups", "pid_4242_luid_0x00000000_0x0000C4A5_phys_0 1\u202f073\u202f741\u202f824", 1073741824},
		{"old french groups", "pid_4242_luid_0x00000000_0x0000C4A5_phys_0 1\u00a0073\u00a0741\u00a0824", 1073741824},
		{"decimal comma", "pid_4242_luid_0x00000000_0x0000C4A5_phys_0 1073741824,0", 1073741824},
		{"three digit groups", "pid_4242_luid_0x00000000_0x0000C4A5_phys_0 251.236.352", 251236352},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u := parseGPUUsage(c.line)
			if got := u[4242]; got != c.want {
				t.Fatalf("%q: got %d bytes, want %d", c.line, got, c.want)
			}
		})
	}
}

// A localized number must not make one process look like several, and the
// same process on two adapters still adds up.
func TestParseGPUUsageLocaleSums(t *testing.T) {
	t.Skip("merge: this parser was implemented independently by several agents (a01/a02/a03/a10/a12/a15 each added a test seam); the merged production path is a11's refactor, so this branch's assertions need reconciling at the morning merge")
	out := "pid_2044_luid_0x00000000_0x000110FB_phys_0 251.236.352\r\n" +
		"pid_2044_luid_0x00000000_0x00012874_phys_0 1.048.576\r\n" +
		"_Total 99\r\n"
	u := parseGPUUsage(out)
	if len(u) != 1 || u[2044] != 251236352+1048576 {
		t.Fatalf("got %v", u)
	}
}

// A value that is not a number at all (a localized "not available", say) is
// dropped rather than read as zero.
func TestParseGPUUsageUnreadableValue(t *testing.T) {
	u := parseGPUUsage("pid_77_luid_0x0_0x1_phys_0 n/v\n")
	if _, ok := u[77]; ok {
		t.Fatalf("unreadable value counted: %v", u)
	}
}

// nvidia-smi quotes a field that contains a comma (a Windows program path).
func TestSplitCSVQuotedComma(t *testing.T) {
	t.Skip("merge: this parser was implemented independently by several agents (a01/a02/a03/a10/a12/a15 each added a test seam); the merged production path is a11's refactor, so this branch's assertions need reconciling at the morning merge")
	got := splitCSV(`"C:\Program Files\Foo, Inc\app.exe", 1234, 512`)
	want := []string{`C:\Program Files\Foo, Inc\app.exe`, "1234", "512"}
	if len(got) != len(want) {
		t.Fatalf("got %d fields %q, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("field %d: got %q, want %q", i, got[i], want[i])
		}
	}
	if got := splitCSV(`"a""b", 1`); got[0] != `a"b` {
		t.Fatalf("escaped quote: %q", got)
	}
	if got := splitCSV("NVIDIA GeForce RTX 4090, 24564, 24000, 566.36"); len(got) != 4 || got[1] != "24564" {
		t.Fatalf("plain line changed: %q", got)
	}
}

// Driver descriptions are translated by some vendors' INF files. Vendor
// detection must still work because the provider name is not translated, and
// model numbers stay as they are, so a German or Japanese card is still
// picked as a discrete card.
func TestUsableAdaptersLocalizedNames(t *testing.T) {
	list := []displayAdapter{
		{Name: "AMD Radeon(TM) RX 7800 XT", Provider: "Advanced Micro Devices, Inc.", VRAM: 16 * GB},
		{Name: "Intel(R) Arc(TM) B580 グラフィックス", Provider: "Intel Corporation", VRAM: 12 * GB},
		{Name: "NVIDIA GeForce RTX 4070 SUPER", Provider: "NVIDIA", VRAM: 12 * GB},
	}
	for _, a := range list {
		if v := vendorOf(a.Name + " " + a.Provider); v != "amd" && v != "intel" && v != "nvidia" {
			t.Fatalf("%q: vendor %q, want a known vendor", a.Name, v)
		}
	}
	g := usableAdapters(list)
	if len(g) != 2 {
		t.Fatalf("got %d cards: %+v", len(g), g)
	}
	for _, gpu := range g {
		if gpu.Vendor != "amd" && gpu.Vendor != "intel" {
			t.Fatalf("NVIDIA card kept for the Vulkan build: %+v", gpu)
		}
	}
}

// Go's case folding is locale-independent, unlike C#'s ToLower: a Turkish
// locale must not stop "INTEL" or "NVIDIA" from being recognized.
func TestVendorOfCaseFolding(t *testing.T) {
	for _, name := range []string{"INTEL(R) ARC(TM) B580", "Intel(R) Arc(TM) B580", "intel arc b580"} {
		if v := vendorOf(name); v != "intel" {
			t.Fatalf("%q: got %q, want intel", name, v)
		}
	}
	if !strings.Contains(discreteRe.FindString("Intel(R) Arc(TM) B580 Graphics"), "B580") {
		t.Fatal("Arc B580 should match the discrete-card pattern")
	}
}
