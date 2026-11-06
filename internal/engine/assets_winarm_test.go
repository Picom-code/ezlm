package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/scan"
)

// Windows CUDA assets, as published from b11960 (the pin) through b12167
// (2026-11-06). All 119 releases in that range carry exactly this set: three
// llama archives and three cudart archives, same names, same .zip format.
// There is no CUDA 12 build for Windows arm64 at any tag.
const (
	winCUDA12X64   = "llama-%s-bin-win-cuda-12.4-x64.zip"
	winCUDA13Arm64 = "llama-%s-bin-win-cuda-13.4-arm64.zip"
	winCUDA13X64   = "llama-%s-bin-win-cuda-13.4-x64.zip"
	cudart12X64    = "cudart-llama-bin-win-cuda-12.4-x64.zip"
	cudart13Arm64  = "cudart-llama-bin-win-cuda-13.4-arm64.zip"
	cudart13X64    = "cudart-llama-bin-win-cuda-13.4-x64.zip"
)

// winCUDARelease builds the asset list GitHub returns for tag, in the order
// GitHub returns it: cudart archives first, then the llama archives.
func winCUDARelease(tag string, extra ...string) *Release {
	names := []string{
		cudart12X64, cudart13Arm64, cudart13X64,
		fmt.Sprintf(winCUDA12X64, tag),
		fmt.Sprintf(winCUDA13Arm64, tag),
		fmt.Sprintf(winCUDA13X64, tag),
		"llama-" + tag + "-bin-win-cpu-arm64.zip",
		"llama-" + tag + "-bin-win-vulkan-arm64.zip",
		"llama-" + tag + "-bin-win-vulkan-x64.zip",
	}
	names = append(names, extra...)
	r := &Release{Tag: tag}
	for _, n := range names {
		r.Assets = append(r.Assets, Asset{Name: n, Size: 1, URL: "https://example.invalid/" + n, Digest: "sha256:00"})
	}
	return r
}

// TestPickAssetsWindowsCUDA covers the Windows CUDA arm64 and CUDA 13 cases for
// the pinned tag and for the builds after it. The latest stable release,
// v0.9.0, names b12110 in nightly-tag.txt; b12167 is the newest build.
func TestPickAssetsWindowsCUDA(t *testing.T) {
	tags := []string{"b11960", "b12110", "b12167"}
	cases := []struct {
		name   string
		arch   string
		cuda13 bool
		want   []string
	}{
		{
			// Windows arm64 only ships CUDA 13 builds, so Install always
			// passes cuda13=true on arm64.
			name: "arm64 cuda13", arch: "arm64", cuda13: true,
			want: []string{winCUDA13Arm64, cudart13Arm64},
		},
		{
			// Without the flag the CUDA 12 pattern finds nothing on arm64 and
			// the generic CUDA pattern falls through to the CUDA 13 build.
			// Same answer, so arm64 is never left without an engine.
			name: "arm64 without cuda13 flag", arch: "arm64", cuda13: false,
			want: []string{winCUDA13Arm64, cudart13Arm64},
		},
		{
			name: "x64 cuda13", arch: "amd64", cuda13: true,
			want: []string{winCUDA13X64, cudart13X64},
		},
		{
			name: "x64 cuda12", arch: "amd64", cuda13: false,
			want: []string{winCUDA12X64, cudart12X64},
		},
	}
	for _, tag := range tags {
		for _, c := range cases {
			t.Run(tag+"/"+c.name, func(t *testing.T) {
				got, err := PickAssets(winCUDARelease(tag), "windows", c.arch, scan.CUDA, c.cuda13)
				if err != nil {
					t.Fatalf("PickAssets: %v", err)
				}
				var names []string
				for _, a := range got {
					names = append(names, a.Name)
				}
				want := make([]string, len(c.want))
				for i, w := range c.want {
					if strings.HasPrefix(w, "llama-") {
						want[i] = fmt.Sprintf(w, tag)
					} else {
						want[i] = w // cudart archives carry no tag
					}
				}
				if len(names) != len(want) {
					t.Fatalf("got %v, want %v", names, want)
				}
				for i := range want {
					if names[i] != want[i] {
						t.Errorf("asset %d = %q, want %q", i, names[i], want[i])
					}
				}
				// The CUDA runtime must match the build's architecture, or
				// the install gets x64 DLLs on an arm64 machine.
				arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[c.arch]
				if len(got) > 1 && !strings.HasSuffix(strings.TrimSuffix(got[1].Name, ".zip"), "-"+arch) {
					t.Errorf("cudart %q does not match %s", got[1].Name, arch)
				}
			})
		}
	}
}

// TestWindowsArm64HasNoCUDA12Build records the gap the selection relies on: no
// release between b11960 and b12167 ships a CUDA 12 build for Windows arm64.
// If one appears, arm64 would need a cuda12 case in assetPatterns.
func TestWindowsArm64HasNoCUDA12Build(t *testing.T) {
	for _, tag := range []string{"b11960", "b12110", "b12167"} {
		for _, a := range winCUDARelease(tag).Assets {
			if strings.Contains(a.Name, "-cuda-12") && strings.Contains(a.Name, "arm64") {
				t.Errorf("%s: unexpected CUDA 12 arm64 asset %q", tag, a.Name)
			}
		}
	}
}

// TestPickAssetsWindowsArmCUDA13FallsBackToVulkan proves the CUDA 13 arm64
// request degrades to Vulkan, not to an x64 archive, when the build is missing.
func TestPickAssetsWindowsArmCUDA13FallsBackToVulkan(t *testing.T) {
	r := &Release{Tag: "b12110", Assets: []Asset{
		{Name: "llama-b12110-bin-win-cuda-13.4-x64.zip"},
		{Name: cudart13X64},
		{Name: "llama-b12110-bin-win-vulkan-arm64.zip"},
	}}
	got, err := PickAssets(r, "windows", "arm64", scan.CUDA, true)
	if err != nil {
		t.Fatalf("PickAssets: %v", err)
	}
	if len(got) != 1 || got[0].Name != "llama-b12110-bin-win-vulkan-arm64.zip" {
		t.Fatalf("got %v, want the arm64 Vulkan build", got)
	}
}

// TestFlavorWindowsArmCUDA13 keeps CUDA 13 arm64 installs in their own folder,
// so a later switch to Vulkan cannot reuse the CUDA binaries.
func TestFlavorWindowsArmCUDA13(t *testing.T) {
	cases := []struct{ asset, want string }{
		{"llama-b12110-bin-win-cuda-13.4-arm64.zip", "win-cuda-13.4-arm64"},
		{"llama-b12110-bin-win-cuda-13.4-x64.zip", "win-cuda-13.4-x64"},
		{"llama-b12110-bin-win-vulkan-arm64.zip", "win-vulkan-arm64"},
	}
	for _, c := range cases {
		if got := flavor(c.asset, "b12110"); got != c.want {
			t.Errorf("flavor(%q) = %q, want %q", c.asset, got, c.want)
		}
	}
}
