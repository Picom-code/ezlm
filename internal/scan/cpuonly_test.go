package scan

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fixture reads one of this package's testdata files.
func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// fakeCommands puts small shell scripts on PATH for the test, so the scan's
// exec calls see a CPU-only machine and never the host.
func fakeCommands(t *testing.T, cmds map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range cmds {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

// scriptCommand returns a command body that prints text, using /bin/cat so it
// still works with PATH limited to the fake directory.
func scriptCommand(text string) string {
	return "/bin/cat <<'EOF'\n" + text + "EOF\n"
}

// /proc/cpuinfo on CPU-only machines: an Intel N100 mini PC (four cores, no
// SMT), a cut-down container copy with no physical id or cpu cores, and an
// arm64 cloud server with no model name at all.
func TestParseProcCPUInfo(t *testing.T) {
	cases := []struct {
		file  string
		name  string
		cores int
	}{
		{"proc_cpuinfo_n100.txt", "Intel(R) N100", 4},
		{"proc_cpuinfo_container_partial.txt", "Intel(R) Xeon(R) Platinum 8375C CPU @ 2.90GHz", 2},
		{"proc_cpuinfo_graviton2.txt", "Neoverse-N1", 4}, // named from CPU part 0xd0c
	}
	for _, c := range cases {
		ci := parseProcCPUInfo(fixture(t, c.file))
		if ci.Name != c.name || ci.Cores != c.cores {
			t.Errorf("%s: got %+v, want %q with %d cores", c.file, ci, c.name, c.cores)
		}
	}

	// A two-socket VM whose /proc/cpuinfo keeps "cpu cores" but drops core id.
	twoSockets := "processor : 0\nphysical id : 0\ncpu cores : 8\n\n" +
		"processor : 1\nphysical id : 1\ncpu cores : 8\n"
	if ci := parseProcCPUInfo(twoSockets); ci.Cores != 16 {
		t.Errorf("two sockets of eight cores: got %d, want 16", ci.Cores)
	}

	// An old arm64 kernel names the board in Hardware, not model name.
	board := "processor : 0\nHardware : BCM2835\nCPU part : 0xd03\n"
	if ci := parseProcCPUInfo(board); ci.Name != "BCM2835" || ci.Cores != 1 {
		t.Errorf("board name from Hardware: %+v", ci)
	}

	// A truly partial /proc: no processor records at all.
	for _, in := range []string{"", "\n\n", "cpu MHz : 800\n"} {
		if ci := parseProcCPUInfo(in); ci.Name != "" || ci.Cores != 0 {
			t.Errorf("%q: got %+v, want nothing", in, ci)
		}
	}
}

func TestParseLscpu(t *testing.T) {
	if ci := parseLscpu(fixture(t, "lscpu_n100.txt")); ci.Name != "Intel(R) N100" || ci.Cores != 4 {
		t.Errorf("N100: %+v", ci)
	}
	if ci := parseLscpu(fixture(t, "lscpu_graviton2.txt")); ci.Name != "Neoverse-N1" || ci.Cores != 4 {
		t.Errorf("Graviton: %+v", ci)
	}
	two := "Architecture: x86_64\n  Model name: AMD EPYC 7763 64-Core Processor\n" +
		"    Core(s) per socket: 32\n    Socket(s): 2\n"
	if ci := parseLscpu(two); ci.Cores != 64 {
		t.Errorf("two sockets: %+v", ci)
	}
	if ci := parseLscpu(""); ci.Name != "" || ci.Cores != 0 {
		t.Errorf("nothing in, nothing out: %+v", ci)
	}
}

// fillCPUInfo only fills what gopsutil left empty.
func TestFillCPUInfo(t *testing.T) {
	proc, lscpu := fixture(t, "proc_cpuinfo_n100.txt"), fixture(t, "lscpu_n100.txt")

	var h Hardware
	fillCPUInfo(&h, proc, lscpu)
	if h.CPUName != "Intel(R) N100" || h.PhysCores != 4 {
		t.Fatalf("filled from /proc/cpuinfo: %+v", h)
	}

	var fromLscpu Hardware
	fillCPUInfo(&fromLscpu, "", lscpu)
	if fromLscpu.CPUName != "Intel(R) N100" || fromLscpu.PhysCores != 4 {
		t.Fatalf("filled from lscpu: %+v", fromLscpu)
	}

	known := Hardware{CPUName: "AMD Ryzen 7 9800X3D 8-Core Processor", Cores: 16, PhysCores: 8}
	fillCPUInfo(&known, proc, lscpu)
	if known.CPUName != "AMD Ryzen 7 9800X3D 8-Core Processor" || known.PhysCores != 8 {
		t.Fatalf("a reading from gopsutil must win: %+v", known)
	}

	// A cut-down /proc can report more physical cores than the machine shows.
	few := Hardware{Cores: 2}
	fillCPUInfo(&few, proc, lscpu)
	if few.PhysCores != 2 {
		t.Fatalf("physical cores capped at logical: %+v", few)
	}

	var nothing Hardware
	fillCPUInfo(&nothing, "", "")
	if nothing.CPUName != "" || nothing.PhysCores != 0 {
		t.Fatalf("nothing in, nothing out: %+v", nothing)
	}
}

// linuxCPUInfo is what Scan runs on a Linux machine whose CPU reads failed:
// /proc/cpuinfo first, lscpu when that is not mounted.
func TestLinuxCPUInfo(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	procPath := filepath.Join(dir, "cpuinfo")
	if err := os.WriteFile(procPath, []byte(fixture(t, "proc_cpuinfo_n100.txt")), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeCommands(t, nil) // no lscpu in this container
	var h Hardware
	linuxCPUInfo(ctx, &h, procPath)
	if h.CPUName != "Intel(R) N100" || h.PhysCores != 4 {
		t.Fatalf("from /proc/cpuinfo: %+v", h)
	}

	fakeCommands(t, map[string]string{"lscpu": scriptCommand(fixture(t, "lscpu_graviton2.txt"))})
	var onlyLscpu Hardware
	linuxCPUInfo(ctx, &onlyLscpu, filepath.Join(dir, "missing"))
	if onlyLscpu.CPUName != "Neoverse-N1" || onlyLscpu.PhysCores != 4 {
		t.Fatalf("from lscpu when /proc is partial: %+v", onlyLscpu)
	}

	fakeCommands(t, nil)
	var neither Hardware
	linuxCPUInfo(ctx, &neither, filepath.Join(dir, "missing"))
	if neither.CPUName != "" || neither.PhysCores != 0 {
		t.Fatalf("a machine with neither still scans: %+v", neither)
	}
}

// A Linux CPU-only machine: no nvidia-smi, no rocm-smi, no display adapters.
func TestCPUOnlyLinuxPlatformScan(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux platformScan")
	}
	fakeCommands(t, nil)
	h := &Hardware{Arch: "amd64", Cores: 4, CPUBandwidth: 50}
	platformScan(context.Background(), h)
	if h.Backend != CPU || len(h.GPUs) != 0 {
		t.Fatalf("CPU-only machine: backend %q, GPUs %+v", h.Backend, h.GPUs)
	}
	if !hasNote(h, "No supported GPU found") {
		t.Fatalf("the CPU-only note is missing: %v", h.Notes)
	}
}

// An Intel Mac is CPU-only too: no Metal, no GPU, and the brand string comes
// from sysctl when gopsutil could not read it.
func TestIntelMacCPUOnly(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS platformScan")
	}
	const brand = "Intel(R) Core(TM) i7-9750H CPU @ 2.60GHz"
	fakeCommands(t, map[string]string{"sysctl": "echo '" + brand + "'\n"})
	h := &Hardware{Arch: "amd64", Cores: 12, RAMTotal: 16 * GB, RAMAvail: 8 * GB, CPUBandwidth: 50}
	platformScan(context.Background(), h)
	if h.Backend != CPU || len(h.GPUs) != 0 || h.Unified {
		t.Fatalf("Intel Mac: backend %q, GPUs %+v, unified %v", h.Backend, h.GPUs, h.Unified)
	}
	if !hasNote(h, "Intel Mac detected") {
		t.Fatalf("the Intel Mac note is missing: %v", h.Notes)
	}
	if h.CPUName == "" {
		t.Fatalf("brand string from sysctl: %q", h.CPUName)
	}

	// An existing name is not overwritten by a second sysctl read.
	fakeCommands(t, map[string]string{"sysctl": "echo 'wrong'\n"})
	known := &Hardware{Arch: "amd64", Cores: 12, CPUName: brand}
	platformScan(context.Background(), known)
	if known.CPUName != brand {
		t.Fatalf("brand string overwritten: %q", known.CPUName)
	}
}

func hasNote(h *Hardware, want string) bool {
	for _, n := range h.Notes {
		if strings.Contains(n, want) {
			return true
		}
	}
	return false
}
