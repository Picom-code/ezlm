package engine

import (
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/scan"
)

// HIP / ROCm assets as published by llama.cpp for the pinned build and the
// builds the upgrade targets. b12110 is the build that releases/latest
// (v0.9.0) names in nightly-tag.txt; b12167 is the newest build by date.
// Names are copied from the GitHub releases API, trimmed to the assets this
// slice cares about.
var rocmReleaseAssets = []struct {
	tag    string
	assets []string
}{
	{"b11960", []string{
		"llama-b11960-bin-ubuntu-x64.tar.gz",
		"llama-b11960-bin-ubuntu-arm64.tar.gz",
		"llama-b11960-bin-ubuntu-vulkan-x64.tar.gz",
		"llama-b11960-bin-ubuntu-vulkan-arm64.tar.gz",
		"llama-b11960-bin-ubuntu-rocm-10.0-x64.tar.gz",
		"llama-b11960-bin-win-cpu-x64.zip",
		"llama-b11960-bin-win-cpu-arm64.zip",
		"llama-b11960-bin-win-vulkan-x64.zip",
		"llama-b11960-bin-win-vulkan-arm64.zip",
		"llama-b11960-bin-win-rocm-10.0-x64.zip",
	}},
	{"b12110", []string{
		"llama-b12110-bin-ubuntu-x64.tar.gz",
		"llama-b12110-bin-ubuntu-arm64.tar.gz",
		"llama-b12110-bin-ubuntu-vulkan-x64.tar.gz",
		"llama-b12110-bin-ubuntu-vulkan-arm64.tar.gz",
		"llama-b12110-bin-ubuntu-rocm-10.0-x64.tar.gz",
		"llama-b12110-bin-win-cpu-x64.zip",
		"llama-b12110-bin-win-cpu-arm64.zip",
		"llama-b12110-bin-win-vulkan-x64.zip",
		"llama-b12110-bin-win-vulkan-arm64.zip",
		"llama-b12110-bin-win-rocm-10.0-x64.zip",
	}},
	{"b12167", []string{
		"llama-b12167-bin-ubuntu-x64.tar.gz",
		"llama-b12167-bin-ubuntu-arm64.tar.gz",
		"llama-b12167-bin-ubuntu-vulkan-x64.tar.gz",
		"llama-b12167-bin-ubuntu-vulkan-arm64.tar.gz",
		"llama-b12167-bin-ubuntu-rocm-10.0-x64.tar.gz",
		"llama-b12167-bin-win-cpu-x64.zip",
		"llama-b12167-bin-win-cpu-arm64.zip",
		"llama-b12167-bin-win-vulkan-x64.zip",
		"llama-b12167-bin-win-vulkan-arm64.zip",
		"llama-b12167-bin-win-rocm-10.0-x64.zip",
	}},
}

func releaseFor(tag string) *Release {
	for _, r := range rocmReleaseAssets {
		if r.tag == tag {
			var assets []Asset
			for _, n := range r.assets {
				assets = append(assets, Asset{Name: n})
			}
			return &Release{Tag: tag, Assets: assets}
		}
	}
	return nil
}

// The ROCm backend installs the Vulkan build: llama.cpp's ROCm archives need a
// ROCm 10.0 runtime on the machine and EZLM has no way to check for it, so the
// self-contained Vulkan build stays the pick. That holds at the pinned build
// and at both upgrade candidates.
func TestPickAssetsROCm(t *testing.T) {
	cases := []struct {
		goos, arch string
		want       string
	}{
		{"linux", "amd64", "llama-%s-bin-ubuntu-vulkan-x64.tar.gz"},
		{"linux", "arm64", "llama-%s-bin-ubuntu-vulkan-arm64.tar.gz"},
		{"windows", "amd64", "llama-%s-bin-win-vulkan-x64.zip"},
		{"windows", "arm64", "llama-%s-bin-win-vulkan-arm64.zip"},
	}
	for _, r := range rocmReleaseAssets {
		rel := releaseFor(r.tag)
		for _, c := range cases {
			got, err := PickAssets(rel, c.goos, c.arch, scan.ROCm, false)
			if err != nil {
				t.Errorf("%s: %s/%s rocm: %v", r.tag, c.goos, c.arch, err)
				continue
			}
			if len(got) != 1 {
				t.Errorf("%s: %s/%s rocm: got %d assets, want 1", r.tag, c.goos, c.arch, len(got))
				continue
			}
			if want := strings.Replace(c.want, "%s", r.tag, 1); got[0].Name != want {
				t.Errorf("%s: %s/%s rocm: got %q, want %q", r.tag, c.goos, c.arch, got[0].Name, want)
			}
		}
	}
}

// Every release in the range publishes one HIP/ROCm archive per OS, x64 only,
// with the same name shape and archive format. There is no arm64 or HIP-named
// variant, and no ROCm runtime archive next to it (unlike cudart- for CUDA).
func TestROCmAssetNames(t *testing.T) {
	want := map[string]string{
		"linux":   "llama-%s-bin-ubuntu-rocm-10.0-x64.tar.gz",
		"windows": "llama-%s-bin-win-rocm-10.0-x64.zip",
	}
	for _, r := range rocmReleaseAssets {
		var rocm []string
		for _, a := range r.assets {
			n := strings.ToLower(a)
			if strings.Contains(n, "rocm") || strings.Contains(n, "hip") {
				rocm = append(rocm, a)
			}
		}
		if len(rocm) != 2 {
			t.Errorf("%s: got %d ROCm/HIP assets %v, want 2", r.tag, len(rocm), rocm)
			continue
		}
		for goos, pat := range want {
			name := strings.Replace(pat, "%s", r.tag, 1)
			found := false
			for _, a := range rocm {
				if a == name {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: missing %s asset %q", r.tag, goos, name)
			}
			if !strings.HasSuffix(name, map[string]string{"linux": ".tar.gz", "windows": ".zip"}[goos]) {
				t.Errorf("%s: %s asset %q has the wrong archive format", r.tag, goos, name)
			}
		}
	}
}

// PickAssets must never return a ROCm archive: the Windows ROCm zip carries
// only ggml-hip.dll and three HIP runtime DLLs, and the Linux one packs
// build/bin alone, so both depend on a ROCm 10.0 install EZLM does not check.
func TestPickAssetsNeverPicksROCm(t *testing.T) {
	for _, r := range rocmReleaseAssets {
		rel := releaseFor(r.tag)
		for _, goos := range []string{"linux", "windows"} {
			for _, arch := range []string{"amd64", "arm64"} {
				for _, b := range []scan.Backend{scan.ROCm, scan.Vulkan, scan.CPU} {
					got, err := PickAssets(rel, goos, arch, b, false)
					if err != nil {
						t.Errorf("%s: %s/%s %s: %v", r.tag, goos, arch, b, err)
						continue
					}
					for _, a := range got {
						if strings.Contains(a.Name, "-rocm-") {
							t.Errorf("%s: %s/%s %s picked ROCm asset %q", r.tag, goos, arch, b, a.Name)
						}
					}
				}
			}
		}
	}
}

// If a release ever drops Vulkan, the ROCm backend still falls back to the
// plain CPU build rather than the ROCm archive. This pins the current
// behaviour so a future change to prefer ROCm is a deliberate one.
func TestPickAssetsROCmFallbackWithoutVulkan(t *testing.T) {
	rel := &Release{Tag: "b12110", Assets: []Asset{
		{Name: "llama-b12110-bin-ubuntu-x64.tar.gz"},
		{Name: "llama-b12110-bin-ubuntu-rocm-10.0-x64.tar.gz"},
	}}
	got, err := PickAssets(rel, "linux", "amd64", scan.ROCm, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := "llama-b12110-bin-ubuntu-x64.tar.gz"; got[0].Name != want {
		t.Errorf("got %q, want %q", got[0].Name, want)
	}
}
