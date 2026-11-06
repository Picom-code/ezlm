//go:build windows

package engine

import (
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/scan"
)

// The Windows archives llama.cpp publishes for b11960, for both architectures.
var winArchRelease = &Release{Tag: "b11960", Assets: []Asset{
	{Name: "llama-b11960-bin-win-cpu-x64.zip"},
	{Name: "llama-b11960-bin-win-cpu-arm64.zip"},
	{Name: "llama-b11960-bin-win-vulkan-x64.zip"},
	{Name: "llama-b11960-bin-win-vulkan-arm64.zip"},
	{Name: "llama-b11960-bin-win-cuda-12.4-x64.zip"},
	{Name: "cudart-llama-bin-win-cuda-12.4-x64.zip"},
	{Name: "llama-b11960-bin-win-cuda-13.4-x64.zip"},
	{Name: "cudart-llama-bin-win-cuda-13.4-x64.zip"},
	{Name: "llama-b11960-bin-win-cuda-13.4-arm64.zip"},
	{Name: "cudart-llama-bin-win-cuda-13.4-arm64.zip"},
}}

// A build for the other architecture either won't start at all (arm64 on x64)
// or runs under emulation (x64 on Arm, with no NEON and about half the speed),
// so every archive EZLM downloads must be the machine's own architecture. The
// llama-server archive and its CUDA runtime must also agree with each other.
func TestPickAssetsWindowsArch(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		want := map[string]string{"amd64": "x64", "arm64": "arm64"}[arch]
		other := map[string]string{"amd64": "arm64", "arm64": "x64"}[arch]
		for _, b := range []scan.Backend{scan.CPU, scan.Vulkan, scan.ROCm, scan.CUDA} {
			for _, cuda13 := range []bool{false, true} {
				got, err := PickAssets(winArchRelease, "windows", arch, b, cuda13)
				if err != nil {
					t.Fatalf("%s/%s cuda13=%v: %v", arch, b, cuda13, err)
				}
				for _, a := range got {
					if strings.Contains(a.Name, other) {
						t.Errorf("%s/%s cuda13=%v: picked the %s archive %s", arch, b, cuda13, other, a.Name)
					}
					if !strings.Contains(a.Name, want) {
						t.Errorf("%s/%s cuda13=%v: %s is not a %s archive", arch, b, cuda13, a.Name, want)
					}
				}
			}
		}
	}
}

// The install folder is named after the archive's flavor, so an arm64 and an
// x64 build of the same release never share a folder: an update that switches
// architecture (or an x64 EZLM on Arm that starts using the Arm build) cannot
// pick up the other architecture's llama-server.
func TestFlavorSeparatesWindowsArch(t *testing.T) {
	x64 := flavor("llama-b11960-bin-win-cpu-x64.zip", "b11960")
	arm := flavor("llama-b11960-bin-win-cpu-arm64.zip", "b11960")
	if x64 == arm {
		t.Fatalf("x64 and arm64 installs share the folder %q", x64)
	}
	if x64 != "win-cpu-x64" || arm != "win-cpu-arm64" {
		t.Errorf("flavor = %q and %q", x64, arm)
	}
}
