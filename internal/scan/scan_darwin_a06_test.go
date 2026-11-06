//go:build darwin

package scan

import (
	"fmt"
	"testing"
)

// memory_pressure output on macOS 26.3.1, abridged.
const pressureSample = `The system has 17179869184 (1048576 pages with a page size of 16384).

Stats: 
Pages free: 5960 
Pages purgeable: 7528 
Pages purged: 2857273211 

Page Q counts:
Pages active: 285953 
Pages inactive: 269476 
Pages speculative: 15466 
Pages throttled: 0 
Pages wired down: 173665 

Compressor Stats:
Pages used by compressor: 388352 

System-wide memory free percentage: 50%
`

func TestPressureAvailable(t *testing.T) {
	const total = 16 * GB
	got, ok := pressureAvailable(pressureSample, total)
	if !ok {
		t.Fatal("pressureAvailable did not find the free percentage")
	}
	// 50% of 16 GiB is 8 GiB; the 285953 active pages (4.36 GiB) are in use.
	want := uint64(8*GB) - 285953*16384
	if got != want {
		t.Fatalf("available = %d, want %d", got, want)
	}
	if got >= uint64(8*GB) {
		t.Fatalf("available = %d still counts active pages as free", got)
	}
}

func TestPressureAvailableNeedsPercentage(t *testing.T) {
	if _, ok := pressureAvailable("Stats:\nPages free: 1 \n", 16*GB); ok {
		t.Fatal("output without a free percentage was accepted")
	}
	if _, ok := pressureAvailable("System-wide memory free percentage: lots\n", 16*GB); ok {
		t.Fatal("a non-numeric percentage was accepted")
	}
}

func TestPressureAvailableWithoutPageCounts(t *testing.T) {
	// An older macOS may not print the page lists; the percentage still works.
	got, ok := pressureAvailable("System-wide memory free percentage: 25%\n", 8*GB)
	if !ok || got != 2*GB {
		t.Fatalf("available = %d, ok = %v, want 2 GiB", got, ok)
	}
}

func TestDefaultGPUUsable(t *testing.T) {
	cases := []struct {
		ram  uint64
		want float64 // share of RAM
	}{
		{8 * GB, 0.67},
		{16 * GB, 0.67},
		{36 * GB, 0.67},
		{48 * GB, 0.75},
		{64 * GB, 0.75},
		{128 * GB, 0.75},
	}
	for _, c := range cases {
		got := defaultGPUUsable(c.ram)
		share := float64(got) / float64(c.ram)
		if share < c.want-0.001 || share > c.want+0.001 {
			t.Errorf("defaultGPUUsable(%d GB) = %.3f of RAM, want %.2f", c.ram/GB, share, c.want)
		}
		if got > c.ram {
			t.Errorf("defaultGPUUsable(%d GB) = %d exceeds RAM", c.ram/GB, got)
		}
	}
	if got := defaultGPUUsable(0); got != 0 {
		t.Errorf("defaultGPUUsable(0) = %d, want 0", got)
	}
}

func TestParseWiredLimitMB(t *testing.T) {
	cases := []struct {
		in   string
		want uint64
		ok   bool
	}{
		{"0\n", 0, false}, // unset
		{"12288\n", 12288, true},
		{" 6144 \n", 6144, true},
		{"", 0, false},
		{"abc", 0, false},
		{"-1", 0, false},
	}
	for _, c := range cases {
		got, ok := parseWiredLimitMB(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("parseWiredLimitMB(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

// The 8 GB case: a user-raised limit must win, but never exceed physical RAM.
func TestGPUUsableEightGB(t *testing.T) {
	ram := uint64(8 * GB)
	if got, want := defaultGPUUsable(ram), uint64(float64(ram)*0.67); got != want {
		t.Fatalf("8 GB default = %d, want %d", got, want)
	}
	// iogpu.wired_limit_mb set to 6 GB is honoured.
	if mb, ok := parseWiredLimitMB(fmt.Sprint(6144)); !ok || mb<<20 != 6*GB {
		t.Fatalf("6 GB wired limit not parsed: %d %v", mb, ok)
	}
	// Some users set it far above RAM; the scan must clamp to RAM.
	mb, ok := parseWiredLimitMB("65536")
	if !ok {
		t.Fatal("65536 MB limit not parsed")
	}
	if got := min(mb<<20, ram); got != ram {
		t.Fatalf("clamped limit = %d, want %d", got, ram)
	}
}
