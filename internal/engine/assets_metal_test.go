package engine

import (
	"testing"

	"github.com/picom-code/ezlm/internal/scan"
)

// macOS Metal builds for both architectures, as published by llama.cpp.
// Asset names are copied from the release listings, not shortened: the
// selection rules match on substrings of exactly these strings.
//
//	b11960: https://api.github.com/repos/ggml-org/llama.cpp/releases/tags/b11960
//	b12110: .../tags/b12110, the build named by nightly-tag.txt in v0.9.0
var metalReleases = map[string]*Release{
	"b11960": {Tag: "b11960", Assets: []Asset{
		{Name: "cudart-llama-b11960-bin-ubuntu-cuda-12.8-x64.tar.gz"},
		{Name: "cudart-llama-b11960-bin-ubuntu-cuda-13.4-arm64.tar.gz"},
		{Name: "cudart-llama-b11960-bin-ubuntu-cuda-13.4-x64.tar.gz"},
		{Name: "cudart-llama-bin-win-cuda-12.4-x64.zip"},
		{Name: "cudart-llama-bin-win-cuda-13.4-arm64.zip"},
		{Name: "cudart-llama-bin-win-cuda-13.4-x64.zip"},
		{Name: "llama-b11960-bin-android-arm64-snapdragon.tar.gz"},
		{Name: "llama-b11960-bin-android-arm64.tar.gz"},
		{Name: "llama-b11960-bin-linux-arm64-snapdragon.tar.gz"},
		{Name: "llama-b11960-bin-macos-arm64.tar.gz"},
		{Name: "llama-b11960-bin-macos-x64.tar.gz"},
		{Name: "llama-b11960-bin-ubuntu-arm64.tar.gz"},
		{Name: "llama-b11960-bin-ubuntu-cuda-12.8-x64.tar.gz"},
		{Name: "llama-b11960-bin-ubuntu-cuda-13.4-arm64.tar.gz"},
		{Name: "llama-b11960-bin-ubuntu-cuda-13.4-x64.tar.gz"},
		{Name: "llama-b11960-bin-ubuntu-openvino-2026.4.1-x64.tar.gz"},
		{Name: "llama-b11960-bin-ubuntu-rocm-10.0-x64.tar.gz"},
		{Name: "llama-b11960-bin-ubuntu-s390x.tar.gz"},
		{Name: "llama-b11960-bin-ubuntu-sycl-fp16-x64.tar.gz"},
		{Name: "llama-b11960-bin-ubuntu-sycl-fp32-x64.tar.gz"},
		{Name: "llama-b11960-bin-ubuntu-vulkan-arm64.tar.gz"},
		{Name: "llama-b11960-bin-ubuntu-vulkan-x64.tar.gz"},
		{Name: "llama-b11960-bin-ubuntu-x64.tar.gz"},
		{Name: "llama-b11960-bin-win-cpu-arm64.zip"},
		{Name: "llama-b11960-bin-win-cpu-x64.zip"},
		{Name: "llama-b11960-bin-win-cuda-12.4-x64.zip"},
		{Name: "llama-b11960-bin-win-cuda-13.4-arm64.zip"},
		{Name: "llama-b11960-bin-win-cuda-13.4-x64.zip"},
		{Name: "llama-b11960-bin-win-opencl-adreno-arm64.zip"},
		{Name: "llama-b11960-bin-win-openvino-2026.4.1-x64.zip"},
		{Name: "llama-b11960-bin-win-rocm-10.0-x64.zip"},
		{Name: "llama-b11960-bin-win-sycl-x64.zip"},
		{Name: "llama-b11960-bin-win-vulkan-arm64.zip"},
		{Name: "llama-b11960-bin-win-vulkan-x64.zip"},
	}},
	"b12110": {Tag: "b12110", Assets: []Asset{
		{Name: "cudart-llama-b12110-bin-ubuntu-cuda-12.8-x64.tar.gz"},
		{Name: "cudart-llama-b12110-bin-ubuntu-cuda-13.4-arm64.tar.gz"},
		{Name: "cudart-llama-b12110-bin-ubuntu-cuda-13.4-x64.tar.gz"},
		{Name: "cudart-llama-bin-win-cuda-12.4-x64.zip"},
		{Name: "cudart-llama-bin-win-cuda-13.4-arm64.zip"},
		{Name: "cudart-llama-bin-win-cuda-13.4-x64.zip"},
		{Name: "llama-b12110-bin-android-arm64-snapdragon.tar.gz"},
		{Name: "llama-b12110-bin-android-arm64.tar.gz"},
		{Name: "llama-b12110-bin-linux-arm64-snapdragon.tar.gz"},
		{Name: "llama-b12110-bin-macos-arm64.tar.gz"},
		{Name: "llama-b12110-bin-macos-x64.tar.gz"},
		{Name: "llama-b12110-bin-ubuntu-26.04-s390x.tar.gz"},
		{Name: "llama-b12110-bin-ubuntu-arm64.tar.gz"},
		{Name: "llama-b12110-bin-ubuntu-cuda-12.8-x64.tar.gz"},
		{Name: "llama-b12110-bin-ubuntu-cuda-13.4-arm64.tar.gz"},
		{Name: "llama-b12110-bin-ubuntu-cuda-13.4-x64.tar.gz"},
		{Name: "llama-b12110-bin-ubuntu-openvino-2026.4.1-x64.tar.gz"},
		{Name: "llama-b12110-bin-ubuntu-rocm-10.0-x64.tar.gz"},
		{Name: "llama-b12110-bin-ubuntu-s390x.tar.gz"},
		{Name: "llama-b12110-bin-ubuntu-sycl-fp16-x64.tar.gz"},
		{Name: "llama-b12110-bin-ubuntu-sycl-fp32-x64.tar.gz"},
		{Name: "llama-b12110-bin-ubuntu-vulkan-arm64.tar.gz"},
		{Name: "llama-b12110-bin-ubuntu-vulkan-x64.tar.gz"},
		{Name: "llama-b12110-bin-ubuntu-x64.tar.gz"},
		{Name: "llama-b12110-bin-ubuntu-zdnn-s390x.tar.gz"},
		{Name: "llama-b12110-bin-win-cpu-arm64.zip"},
		{Name: "llama-b12110-bin-win-cpu-x64.zip"},
		{Name: "llama-b12110-bin-win-cuda-12.4-x64.zip"},
		{Name: "llama-b12110-bin-win-cuda-13.4-arm64.zip"},
		{Name: "llama-b12110-bin-win-cuda-13.4-x64.zip"},
		{Name: "llama-b12110-bin-win-opencl-adreno-arm64.zip"},
		{Name: "llama-b12110-bin-win-openvino-2026.4.1-x64.zip"},
		{Name: "llama-b12110-bin-win-rocm-10.0-x64.zip"},
		{Name: "llama-b12110-bin-win-sycl-x64.zip"},
		{Name: "llama-b12110-bin-win-vulkan-arm64.zip"},
		{Name: "llama-b12110-bin-win-vulkan-x64.zip"},
		{Name: "llama-b12110-ui.tar.gz"},
		{Name: "llama-b12110-xcframework.zip"},
	}},
}

// macOS ships one Metal archive per architecture and nothing else: no CUDA
// runtime sidecar, and the same names at both tags. Install() also rewrites
// the backend to Vulkan on arm64 Macs (internal/engine/engine.go:218), so the
// darwin rule must not depend on the backend at all.
func TestPickAssetsMacOSMetal(t *testing.T) {
	cases := []struct {
		tag, arch string
		b         scan.Backend
		want      string
	}{
		{"b11960", "arm64", scan.Metal, "llama-b11960-bin-macos-arm64.tar.gz"},
		{"b11960", "amd64", scan.Metal, "llama-b11960-bin-macos-x64.tar.gz"},
		{"b12110", "arm64", scan.Metal, "llama-b12110-bin-macos-arm64.tar.gz"},
		{"b12110", "amd64", scan.Metal, "llama-b12110-bin-macos-x64.tar.gz"},

		// What Install() actually passes: backend rewritten to Vulkan.
		{"b11960", "arm64", scan.Vulkan, "llama-b11960-bin-macos-arm64.tar.gz"},
		{"b11960", "amd64", scan.Vulkan, "llama-b11960-bin-macos-x64.tar.gz"},
		{"b12110", "arm64", scan.Vulkan, "llama-b12110-bin-macos-arm64.tar.gz"},
		{"b12110", "amd64", scan.Vulkan, "llama-b12110-bin-macos-x64.tar.gz"},

		// CPU-only Macs, and a stale CUDA hint, still get the Metal build.
		{"b11960", "arm64", scan.CPU, "llama-b11960-bin-macos-arm64.tar.gz"},
		{"b12110", "amd64", scan.CPU, "llama-b12110-bin-macos-x64.tar.gz"},
		{"b12110", "arm64", scan.CUDA, "llama-b12110-bin-macos-arm64.tar.gz"},
		{"b12110", "amd64", scan.ROCm, "llama-b12110-bin-macos-x64.tar.gz"},
	}
	for _, c := range cases {
		got, err := PickAssets(metalReleases[c.tag], "darwin", c.arch, c.b, c.arch == "arm64")
		if err != nil {
			t.Errorf("%s darwin/%s %s: %v", c.tag, c.arch, c.b, err)
			continue
		}
		if len(got) != 1 {
			t.Errorf("%s darwin/%s %s: got %d assets, want 1", c.tag, c.arch, c.b, len(got))
			continue
		}
		if got[0].Name != c.want {
			t.Errorf("%s darwin/%s %s: got %q, want %q", c.tag, c.arch, c.b, got[0].Name, c.want)
		}
	}
}

// Both architectures exist at both tags, and the archive format did not
// change: macOS is still .tar.gz, never .zip.
func TestMacOSAssetsExistAtBothTags(t *testing.T) {
	for tag, r := range metalReleases {
		have := map[string]bool{}
		for _, a := range r.Assets {
			have[a.Name] = true
		}
		for _, arch := range []string{"arm64", "x64"} {
			name := "llama-" + tag + "-bin-macos-" + arch + ".tar.gz"
			if !have[name] {
				t.Errorf("release %s is missing %s", tag, name)
			}
		}
	}
}

// The archive name decides the install folder (engine.go:228), and the folder
// name must stay distinct per architecture and per tag so a switch between
// them never reuses the other's binaries.
func TestMacOSFlavorPerTagAndArch(t *testing.T) {
	cases := []struct{ tag, arch, want string }{
		{"b11960", "arm64", "macos-arm64"},
		{"b11960", "amd64", "macos-x64"},
		{"b12110", "arm64", "macos-arm64"},
		{"b12110", "amd64", "macos-x64"},
	}
	for _, c := range cases {
		assets, err := PickAssets(metalReleases[c.tag], "darwin", c.arch, scan.Metal, false)
		if err != nil {
			t.Errorf("%s darwin/%s: %v", c.tag, c.arch, err)
			continue
		}
		if got := flavor(assets[0].Name, c.tag); got != c.want {
			t.Errorf("%s darwin/%s: flavor = %q, want %q", c.tag, c.arch, got, c.want)
		}
	}
}

// A release with no macOS archive must fail loudly rather than fall through to
// a Linux or Windows build.
func TestPickAssetsMacOSMissing(t *testing.T) {
	r := &Release{Tag: "b99999", Assets: []Asset{
		{Name: "llama-b99999-bin-ubuntu-x64.tar.gz"},
		{Name: "llama-b99999-bin-win-cpu-x64.zip"},
	}}
	for _, arch := range []string{"arm64", "amd64"} {
		if _, err := PickAssets(r, "darwin", arch, scan.Metal, false); err == nil {
			t.Errorf("darwin/%s: expected an error for a release without macOS builds", arch)
		}
	}
}
