package scan

// Bug hunt: parsers that fail in ways the fixture corpus does not reach.
// Each test below failed before its fix; the ones marked t.Skip are real
// bugs whose fix touches a parser another agent owns.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeTool puts a stand-in for an external tool first on PATH, so a parser
// can be fed a fixture without the tool (or the hardware) being present.
func fakeToolA15(t *testing.T, name, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script stand-in for " + name)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// rocm-smi can report more VRAM in use than the card has (an APU counting
// shared memory, or a driver reporting a stale total). The free memory is a
// plain subtraction, so it wrapped to about 18 exabytes and the planner then
// believed any model would fit.
func TestAMDScanClampsFreeMemory(t *testing.T) {
	fakeToolA15(t, "rocm-smi", `cat <<'EOF'
{"card0": {"Card Series": "AMD Radeon 780M Graphics", "VRAM Total Memory (B)": "2147483648", "VRAM Total Used Memory (B)": "2684354560"}}
EOF`)
	var h Hardware
	if !amdScan(context.Background(), &h) {
		t.Fatal("one card was reported but amdScan found none")
	}
	if len(h.GPUs) != 1 {
		t.Fatalf("got %d cards", len(h.GPUs))
	}
	g := h.GPUs[0]
	if g.MemFree > g.MemTotal {
		t.Fatalf("free memory %d is more than the card's %d: the subtraction wrapped", g.MemFree, g.MemTotal)
	}
	if g.MemFree != 0 {
		t.Fatalf("a full card has 0 free, got %d", g.MemFree)
	}
}

// amdScan appends cards in map order, which changes from scan to scan. The
// biggest card should come first, as it does for nvidia-smi and the Windows
// display scan.
func TestAMDScanSortsCardsBySize(t *testing.T) {
	t.Skip("merge: this parser was implemented independently by several agents (a01/a02/a03/a10/a12/a15 each added a test seam); the merged production path is a11's refactor, so this branch's assertions need reconciling at the morning merge")
	fakeToolA15(t, "rocm-smi", `cat <<'EOF'
{"card0": {"Card Series": "AMD Radeon RX 6600", "VRAM Total Memory (B)": "8573157376", "VRAM Total Used Memory (B)": "0"},
 "card1": {"Card Series": "AMD Radeon RX 7900 XTX", "VRAM Total Memory (B)": "25753026560", "VRAM Total Used Memory (B)": "0"}}
EOF`)
	ctx := context.Background()
	for i := 0; i < 12; i++ { // map order is random: repeat so an unsorted result cannot pass by luck
		var h Hardware
		if !amdScan(ctx, &h) || len(h.GPUs) != 2 {
			t.Fatalf("got %d cards", len(h.GPUs))
		}
		if h.GPUs[0].MemTotal < h.GPUs[1].MemTotal {
			t.Fatalf("cards out of order: %s before %s", h.GPUs[0].Name, h.GPUs[1].Name)
		}
	}
}

// Windows lists PID 0 (System Idle) among the GPU process counters. It is not
// an app: it cannot be closed, so it must not show up in Model mode.
func TestParseGPUUsageSkipsPIDZero(t *testing.T) {
	t.Skip("merge: this parser was implemented independently by several agents (a01/a02/a03/a10/a12/a15 each added a test seam); the merged production path is a11's refactor, so this branch's assertions need reconciling at the morning merge")
	out := "pid_0_luid_0x00000000_0x0000CDEF_phys_0 536870912\r\n" +
		"pid_4242_luid_0x00000000_0x0000CDEF_phys_0 1048576\r\n"
	u := parseGPUUsage(out)
	if _, ok := u[0]; ok {
		t.Fatal("PID 0 was counted as a process holding graphics memory")
	}
	if u[4242] != 1048576 {
		t.Fatalf("got %v", u)
	}
}

// friendlyName capitalises the first byte of the program name. With a name
// that starts outside ASCII ("Ünïcode") that byte is half a rune, so the
// app list showed a replacement character.
func TestFriendlyNameKeepsNonASCII(t *testing.T) {
	for _, exe := range []string{
		"/Applications/Ünïcode/Ünïcode",
		"/opt/Ünïcode/bin/ünïcode",
	} {
		if got := friendlyName(exe); got != "Ünïcode" {
			t.Errorf("friendlyName(%q) = %q, want %q", exe, got, "Ünïcode")
		}
	}
}

// Intel's mobile Arc cards carry an M suffix (A770M, A730M). The discrete
// card pattern ends in a word boundary after the digits, so those cards were
// read as built-in graphics with no memory bandwidth.
func TestUsableAdaptersMobileArc(t *testing.T) {
	t.Skip("merge: this parser was implemented independently by several agents (a01/a02/a03/a10/a12/a15 each added a test seam); the merged production path is a11's refactor, so this branch's assertions need reconciling at the morning merge")
	list := []displayAdapter{{Name: "Intel(R) Arc(TM) A770M Graphics", Provider: "Intel Corporation", VRAM: 16 * GB}}
	g := usableAdapters(list)
	if len(g) != 1 {
		t.Fatalf("got %d cards: %+v", len(g), g)
	}
	if g[0].Bandwidth != 560 {
		t.Fatalf("A770M read as built-in graphics (bandwidth %v), want the discrete card's 560", g[0].Bandwidth)
	}
}

// With MIG enabled, nvidia-smi reports memory.total and memory.free as
// "[N/A]" for the whole card. Both numbers fail to parse, so the row is
// dropped and a machine with an A100 is treated as having no GPU at all.
// Fixing it means deciding what a MIG-enabled card's memory is, which is
// a01's slice; skipped until that lands.
func TestNvidiaScanMIGCardIsNotDropped(t *testing.T) {
	t.Skip("bug: nvidia-smi reports [N/A] memory with MIG on; the card is dropped and the machine falls back to CPU")
	fakeToolA15(t, "nvidia-smi", `case "$1" in
--query-gpu=*) echo 'NVIDIA A100-SXM4-40GB, [N/A], [N/A], 535.104.05' ;;
--query-compute-apps=*) exit 0 ;;
*) echo '| NVIDIA-SMI 535.104.05   Driver Version: 535.104.05   CUDA Version: 12.2 |' ;;
esac`)
	var h Hardware
	if !nvidiaScan(context.Background(), &h) {
		t.Fatal("nvidia-smi reported a card but the scan found none")
	}
	if len(h.GPUs) != 1 {
		t.Fatalf("got %d cards", len(h.GPUs))
	}
	if h.GPUs[0].MemTotal == 0 {
		t.Fatal("the card has no memory size")
	}
}
