package engine

// Vulkan: the Windows and Linux archives EZLM installs when the GPU backend is
// Vulkan. The asset lists below are the complete asset set of each llama.cpp
// release, copied from the GitHub releases API (raw listings in
// agents/T06/a05/). They are full lists on purpose: if a release gains or
// drops an archive, the test data changes and the selection test notices.

import (
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/scan"
)

// vulkanAssetsB12110 is b12110, the build the latest stable release (v0.9.0)
// names in nightly-tag.txt. This is what LatestRelease resolves to today.
var vulkanAssetsB12110 = []string{
	"cudart-llama-b12110-bin-ubuntu-cuda-12.8-x64.tar.gz",
	"cudart-llama-b12110-bin-ubuntu-cuda-13.4-arm64.tar.gz",
	"cudart-llama-b12110-bin-ubuntu-cuda-13.4-x64.tar.gz",
	"cudart-llama-bin-win-cuda-12.4-x64.zip",
	"cudart-llama-bin-win-cuda-13.4-arm64.zip",
	"cudart-llama-bin-win-cuda-13.4-x64.zip",
	"llama-b12110-bin-android-arm64-snapdragon.tar.gz",
	"llama-b12110-bin-android-arm64.tar.gz",
	"llama-b12110-bin-linux-arm64-snapdragon.tar.gz",
	"llama-b12110-bin-macos-arm64.tar.gz",
	"llama-b12110-bin-macos-x64.tar.gz",
	"llama-b12110-bin-ubuntu-26.04-s390x.tar.gz",
	"llama-b12110-bin-ubuntu-arm64.tar.gz",
	"llama-b12110-bin-ubuntu-cuda-12.8-x64.tar.gz",
	"llama-b12110-bin-ubuntu-cuda-13.4-arm64.tar.gz",
	"llama-b12110-bin-ubuntu-cuda-13.4-x64.tar.gz",
	"llama-b12110-bin-ubuntu-openvino-2026.4.1-x64.tar.gz",
	"llama-b12110-bin-ubuntu-rocm-10.0-x64.tar.gz",
	"llama-b12110-bin-ubuntu-s390x.tar.gz",
	"llama-b12110-bin-ubuntu-sycl-fp16-x64.tar.gz",
	"llama-b12110-bin-ubuntu-sycl-fp32-x64.tar.gz",
	"llama-b12110-bin-ubuntu-vulkan-arm64.tar.gz",
	"llama-b12110-bin-ubuntu-vulkan-x64.tar.gz",
	"llama-b12110-bin-ubuntu-x64.tar.gz",
	"llama-b12110-bin-ubuntu-zdnn-s390x.tar.gz",
	"llama-b12110-bin-win-cpu-arm64.zip",
	"llama-b12110-bin-win-cpu-x64.zip",
	"llama-b12110-bin-win-cuda-12.4-x64.zip",
	"llama-b12110-bin-win-cuda-13.4-arm64.zip",
	"llama-b12110-bin-win-cuda-13.4-x64.zip",
	"llama-b12110-bin-win-opencl-adreno-arm64.zip",
	"llama-b12110-bin-win-openvino-2026.4.1-x64.zip",
	"llama-b12110-bin-win-rocm-10.0-x64.zip",
	"llama-b12110-bin-win-sycl-x64.zip",
	"llama-b12110-bin-win-vulkan-arm64.zip",
	"llama-b12110-bin-win-vulkan-x64.zip",
	"llama-b12110-ui.tar.gz",
	"llama-b12110-xcframework.zip",
}

// vulkanAssetsB12167 is the newest bNNNNN build published at the time of
// writing (2026-11-06), newer than what releases/latest resolves to.
var vulkanAssetsB12167 = []string{
	"cudart-llama-b12167-bin-ubuntu-cuda-12.8-x64.tar.gz",
	"cudart-llama-b12167-bin-ubuntu-cuda-13.4-arm64.tar.gz",
	"cudart-llama-b12167-bin-ubuntu-cuda-13.4-x64.tar.gz",
	"cudart-llama-bin-win-cuda-12.4-x64.zip",
	"cudart-llama-bin-win-cuda-13.4-arm64.zip",
	"cudart-llama-bin-win-cuda-13.4-x64.zip",
	"llama-b12167-bin-android-arm64-snapdragon.tar.gz",
	"llama-b12167-bin-android-arm64.tar.gz",
	"llama-b12167-bin-linux-arm64-snapdragon.tar.gz",
	"llama-b12167-bin-macos-arm64.tar.gz",
	"llama-b12167-bin-macos-x64.tar.gz",
	"llama-b12167-bin-ubuntu-26.04-s390x.tar.gz",
	"llama-b12167-bin-ubuntu-arm64.tar.gz",
	"llama-b12167-bin-ubuntu-cuda-12.8-x64.tar.gz",
	"llama-b12167-bin-ubuntu-cuda-13.4-arm64.tar.gz",
	"llama-b12167-bin-ubuntu-cuda-13.4-x64.tar.gz",
	"llama-b12167-bin-ubuntu-openvino-2026.4.1-x64.tar.gz",
	"llama-b12167-bin-ubuntu-rocm-10.0-x64.tar.gz",
	"llama-b12167-bin-ubuntu-s390x.tar.gz",
	"llama-b12167-bin-ubuntu-sycl-fp16-x64.tar.gz",
	"llama-b12167-bin-ubuntu-sycl-fp32-x64.tar.gz",
	"llama-b12167-bin-ubuntu-vulkan-arm64.tar.gz",
	"llama-b12167-bin-ubuntu-vulkan-x64.tar.gz",
	"llama-b12167-bin-ubuntu-x64.tar.gz",
	"llama-b12167-bin-ubuntu-zdnn-s390x.tar.gz",
	"llama-b12167-bin-win-cpu-arm64.zip",
	"llama-b12167-bin-win-cpu-x64.zip",
	"llama-b12167-bin-win-cuda-12.4-x64.zip",
	"llama-b12167-bin-win-cuda-13.4-arm64.zip",
	"llama-b12167-bin-win-cuda-13.4-x64.zip",
	"llama-b12167-bin-win-opencl-adreno-arm64.zip",
	"llama-b12167-bin-win-openvino-2026.4.1-x64.zip",
	"llama-b12167-bin-win-rocm-10.0-x64.zip",
	"llama-b12167-bin-win-sycl-x64.zip",
	"llama-b12167-bin-win-vulkan-arm64.zip",
	"llama-b12167-bin-win-vulkan-x64.zip",
	"llama-b12167-ui.tar.gz",
	"llama-b12167-xcframework.zip",
}

// vulkanAssetsB12050 is the only release in the b11960..b12167 range that
// shipped no Windows Vulkan archive.
var vulkanAssetsB12050 = []string{
	"cudart-llama-b12050-bin-ubuntu-cuda-12.8-x64.tar.gz",
	"cudart-llama-b12050-bin-ubuntu-cuda-13.4-arm64.tar.gz",
	"cudart-llama-b12050-bin-ubuntu-cuda-13.4-x64.tar.gz",
	"cudart-llama-bin-win-cuda-12.4-x64.zip",
	"cudart-llama-bin-win-cuda-13.4-arm64.zip",
	"cudart-llama-bin-win-cuda-13.4-x64.zip",
	"llama-b12050-bin-android-arm64-snapdragon.tar.gz",
	"llama-b12050-bin-android-arm64.tar.gz",
	"llama-b12050-bin-linux-arm64-snapdragon.tar.gz",
	"llama-b12050-bin-macos-arm64.tar.gz",
	"llama-b12050-bin-macos-x64.tar.gz",
	"llama-b12050-bin-ubuntu-26.04-s390x.tar.gz",
	"llama-b12050-bin-ubuntu-arm64.tar.gz",
	"llama-b12050-bin-ubuntu-cuda-12.8-x64.tar.gz",
	"llama-b12050-bin-ubuntu-cuda-13.4-arm64.tar.gz",
	"llama-b12050-bin-ubuntu-cuda-13.4-x64.tar.gz",
	"llama-b12050-bin-ubuntu-openvino-2026.4.1-x64.tar.gz",
	"llama-b12050-bin-ubuntu-rocm-10.0-x64.tar.gz",
	"llama-b12050-bin-ubuntu-s390x.tar.gz",
	"llama-b12050-bin-ubuntu-sycl-fp16-x64.tar.gz",
	"llama-b12050-bin-ubuntu-sycl-fp32-x64.tar.gz",
	"llama-b12050-bin-ubuntu-vulkan-arm64.tar.gz",
	"llama-b12050-bin-ubuntu-vulkan-x64.tar.gz",
	"llama-b12050-bin-ubuntu-x64.tar.gz",
	"llama-b12050-bin-ubuntu-zdnn-s390x.tar.gz",
	"llama-b12050-bin-win-cpu-arm64.zip",
	"llama-b12050-bin-win-cpu-x64.zip",
	"llama-b12050-bin-win-cuda-12.4-x64.zip",
	"llama-b12050-bin-win-cuda-13.4-arm64.zip",
	"llama-b12050-bin-win-cuda-13.4-x64.zip",
}

func vulkanRelease(tag string, names []string) *Release {
	r := &Release{Tag: tag}
	for _, n := range names {
		r.Assets = append(r.Assets, Asset{Name: n})
	}
	return r
}

func vulkanNames(as []Asset) []string {
	out := make([]string, 0, len(as))
	for _, a := range as {
		out = append(out, a.Name)
	}
	return out
}

func TestVulkanAssetSelection(t *testing.T) {
	cases := []struct {
		tag, goos, arch string
		assets          []string
		cuda13          bool
		want            string
	}{
		{"b11960", "linux", "amd64", nil, false, "llama-b11960-bin-ubuntu-vulkan-x64.tar.gz"},
		{"b11960", "linux", "arm64", nil, false, "llama-b11960-bin-ubuntu-vulkan-arm64.tar.gz"},
		{"b11960", "windows", "amd64", nil, false, "llama-b11960-bin-win-vulkan-x64.zip"},
		{"b11960", "windows", "arm64", nil, false, "llama-b11960-bin-win-vulkan-arm64.zip"},

		{"b12110", "linux", "amd64", vulkanAssetsB12110, false, "llama-b12110-bin-ubuntu-vulkan-x64.tar.gz"},
		{"b12110", "linux", "arm64", vulkanAssetsB12110, false, "llama-b12110-bin-ubuntu-vulkan-arm64.tar.gz"},
		{"b12110", "windows", "amd64", vulkanAssetsB12110, false, "llama-b12110-bin-win-vulkan-x64.zip"},
		{"b12110", "windows", "arm64", vulkanAssetsB12110, false, "llama-b12110-bin-win-vulkan-arm64.zip"},

		{"b12167", "linux", "amd64", vulkanAssetsB12167, false, "llama-b12167-bin-ubuntu-vulkan-x64.tar.gz"},
		{"b12167", "linux", "arm64", vulkanAssetsB12167, false, "llama-b12167-bin-ubuntu-vulkan-arm64.tar.gz"},
		{"b12167", "windows", "amd64", vulkanAssetsB12167, false, "llama-b12167-bin-win-vulkan-x64.zip"},
		{"b12167", "windows", "arm64", vulkanAssetsB12167, false, "llama-b12167-bin-win-vulkan-arm64.zip"},

		// Install passes cuda13=true on arm64 and when the GPU wants CUDA 13,
		// then drops back to Vulkan if the driver is too old. The Vulkan
		// patterns must not change because of that flag.
		{"b12110", "linux", "amd64", vulkanAssetsB12110, true, "llama-b12110-bin-ubuntu-vulkan-x64.tar.gz"},
		{"b12167", "windows", "arm64", vulkanAssetsB12167, true, "llama-b12167-bin-win-vulkan-arm64.zip"},
	}
	for _, c := range cases {
		var r *Release
		if c.assets == nil {
			r = &pinnedRelease
			if r.Tag != c.tag {
				t.Fatalf("pinned release is %s, want %s", r.Tag, c.tag)
			}
		} else {
			r = vulkanRelease(c.tag, c.assets)
		}
		got, err := PickAssets(r, c.goos, c.arch, scan.Vulkan, c.cuda13)
		if err != nil {
			t.Errorf("%s %s/%s cuda13=%v: %v", c.tag, c.goos, c.arch, c.cuda13, err)
			continue
		}
		if len(got) != 1 {
			t.Errorf("%s %s/%s: picked %d assets %v, want exactly the Vulkan archive", c.tag, c.goos, c.arch, len(got), vulkanNames(got))
			continue
		}
		if got[0].Name != c.want {
			t.Errorf("%s %s/%s cuda13=%v: got %q, want %q", c.tag, c.goos, c.arch, c.cuda13, got[0].Name, c.want)
		}
	}
}

// The archive format is part of the contract: Linux unpacks tar.gz, Windows
// unpacks zip.
func TestVulkanAssetArchiveFormat(t *testing.T) {
	for _, c := range []struct{ goos, want string }{
		{"linux", ".tar.gz"},
		{"windows", ".zip"},
	} {
		got, err := PickAssets(vulkanRelease("b12167", vulkanAssetsB12167), c.goos, "amd64", scan.Vulkan, false)
		if err != nil {
			t.Fatalf("%s: %v", c.goos, err)
		}
		if !strings.HasSuffix(got[0].Name, c.want) {
			t.Errorf("%s: %q does not end in %s", c.goos, got[0].Name, c.want)
		}
	}
}

// b12050 published no Windows Vulkan archive. PickAssets falls back to the
// Windows CPU build, so the install succeeds but silently loses the GPU.
func TestVulkanMissingWindowsAssetFallsBackToCPU(t *testing.T) {
	r := vulkanRelease("b12050", vulkanAssetsB12050)
	got, err := PickAssets(r, "windows", "amd64", scan.Vulkan, false)
	if err != nil {
		t.Fatalf("windows: %v", err)
	}
	if want := "llama-b12050-bin-win-cpu-x64.zip"; got[0].Name != want {
		t.Errorf("windows: got %q, want %q", got[0].Name, want)
	}
	// Linux kept its Vulkan archive in the same release.
	got, err = PickAssets(r, "linux", "amd64", scan.Vulkan, false)
	if err != nil {
		t.Fatalf("linux: %v", err)
	}
	if want := "llama-b12050-bin-ubuntu-vulkan-x64.tar.gz"; got[0].Name != want {
		t.Errorf("linux: got %q, want %q", got[0].Name, want)
	}
}

// The pin itself must carry the Vulkan archives, so offline installs work.
func TestPinnedVulkanAssets(t *testing.T) {
	want := []string{
		"llama-b11960-bin-ubuntu-vulkan-x64.tar.gz",
		"llama-b11960-bin-ubuntu-vulkan-arm64.tar.gz",
		"llama-b11960-bin-win-vulkan-x64.zip",
		"llama-b11960-bin-win-vulkan-arm64.zip",
	}
	have := map[string]bool{}
	for _, a := range pinnedRelease.Assets {
		have[a.Name] = true
	}
	for _, n := range want {
		if !have[n] {
			t.Errorf("pinned release is missing %s", n)
		}
	}
}
