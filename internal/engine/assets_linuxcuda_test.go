package engine

import (
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/scan"
)

// linuxCUDABuilds holds the asset names published for the pinned build
// (b11960, via pinnedRelease) and for the two builds the upgrade could land
// on: b12110 is the build that releases/latest (v0.9.0) names in its
// nightly-tag.txt, b12167 is the newest nightly build at the time of writing.
//
// The Linux CUDA 12.8 x64, CUDA 13.4 x64 and CUDA 13.4 arm64 archives kept
// the same names, sizes and .tar.gz format across all 117 builds from b11960
// to b12167; see agents/T06/a04/linux-cuda-assets.json.
var linuxCUDABuilds = map[string]*Release{
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
	"b12167": {Tag: "b12167", Assets: []Asset{
		{Name: "cudart-llama-b12167-bin-ubuntu-cuda-12.8-x64.tar.gz"},
		{Name: "cudart-llama-b12167-bin-ubuntu-cuda-13.4-arm64.tar.gz"},
		{Name: "cudart-llama-b12167-bin-ubuntu-cuda-13.4-x64.tar.gz"},
		{Name: "cudart-llama-bin-win-cuda-12.4-x64.zip"},
		{Name: "cudart-llama-bin-win-cuda-13.4-arm64.zip"},
		{Name: "cudart-llama-bin-win-cuda-13.4-x64.zip"},
		{Name: "llama-b12167-bin-android-arm64-snapdragon.tar.gz"},
		{Name: "llama-b12167-bin-android-arm64.tar.gz"},
		{Name: "llama-b12167-bin-linux-arm64-snapdragon.tar.gz"},
		{Name: "llama-b12167-bin-macos-arm64.tar.gz"},
		{Name: "llama-b12167-bin-macos-x64.tar.gz"},
		{Name: "llama-b12167-bin-ubuntu-26.04-s390x.tar.gz"},
		{Name: "llama-b12167-bin-ubuntu-arm64.tar.gz"},
		{Name: "llama-b12167-bin-ubuntu-cuda-12.8-x64.tar.gz"},
		{Name: "llama-b12167-bin-ubuntu-cuda-13.4-arm64.tar.gz"},
		{Name: "llama-b12167-bin-ubuntu-cuda-13.4-x64.tar.gz"},
		{Name: "llama-b12167-bin-ubuntu-openvino-2026.4.1-x64.tar.gz"},
		{Name: "llama-b12167-bin-ubuntu-rocm-10.0-x64.tar.gz"},
		{Name: "llama-b12167-bin-ubuntu-s390x.tar.gz"},
		{Name: "llama-b12167-bin-ubuntu-sycl-fp16-x64.tar.gz"},
		{Name: "llama-b12167-bin-ubuntu-sycl-fp32-x64.tar.gz"},
		{Name: "llama-b12167-bin-ubuntu-vulkan-arm64.tar.gz"},
		{Name: "llama-b12167-bin-ubuntu-vulkan-x64.tar.gz"},
		{Name: "llama-b12167-bin-ubuntu-x64.tar.gz"},
		{Name: "llama-b12167-bin-ubuntu-zdnn-s390x.tar.gz"},
		{Name: "llama-b12167-bin-win-cpu-arm64.zip"},
		{Name: "llama-b12167-bin-win-cpu-x64.zip"},
		{Name: "llama-b12167-bin-win-cuda-12.4-x64.zip"},
		{Name: "llama-b12167-bin-win-cuda-13.4-arm64.zip"},
		{Name: "llama-b12167-bin-win-cuda-13.4-x64.zip"},
		{Name: "llama-b12167-bin-win-opencl-adreno-arm64.zip"},
		{Name: "llama-b12167-bin-win-openvino-2026.4.1-x64.zip"},
		{Name: "llama-b12167-bin-win-rocm-10.0-x64.zip"},
		{Name: "llama-b12167-bin-win-sycl-x64.zip"},
		{Name: "llama-b12167-bin-win-vulkan-arm64.zip"},
		{Name: "llama-b12167-bin-win-vulkan-x64.zip"},
		{Name: "llama-b12167-ui.tar.gz"},
		{Name: "llama-b12167-xcframework.zip"},
	}},
}

// buildFor returns the release fixture for a tag. The pinned build is read
// from pinned.go so the test fails if the pin and the selection drift apart.
func buildFor(t *testing.T, tag string) *Release {
	t.Helper()
	if tag == pinnedRelease.Tag {
		r := pinnedRelease
		return &r
	}
	r, ok := linuxCUDABuilds[tag]
	if !ok {
		t.Fatalf("no fixture for %s", tag)
	}
	return r
}

func assetNames(as []Asset) []string {
	names := make([]string, len(as))
	for i, a := range as {
		names[i] = a.Name
	}
	return names
}

// TestLinuxCUDAAssetSelection covers the two Linux CUDA builds EZLM can pick:
// CUDA 12.8 x64 for ordinary NVIDIA drivers, and the CUDA 13.4 builds when a
// Blackwell card needs them (arm64 only ever has CUDA 13). The expected names
// are the ones llama.cpp actually published at each tag.
func TestLinuxCUDAAssetSelection(t *testing.T) {
	cases := []struct {
		tag    string
		arch   string
		cuda13 bool
		want   []string
	}{
		{"b11960", "amd64", false, []string{
			"llama-b11960-bin-ubuntu-cuda-12.8-x64.tar.gz",
			"cudart-llama-b11960-bin-ubuntu-cuda-12.8-x64.tar.gz",
		}},
		{"b11960", "amd64", true, []string{
			"llama-b11960-bin-ubuntu-cuda-13.4-x64.tar.gz",
			"cudart-llama-b11960-bin-ubuntu-cuda-13.4-x64.tar.gz",
		}},
		{"b11960", "arm64", true, []string{
			"llama-b11960-bin-ubuntu-cuda-13.4-arm64.tar.gz",
			"cudart-llama-b11960-bin-ubuntu-cuda-13.4-arm64.tar.gz",
		}},
		{"b12110", "amd64", false, []string{
			"llama-b12110-bin-ubuntu-cuda-12.8-x64.tar.gz",
			"cudart-llama-b12110-bin-ubuntu-cuda-12.8-x64.tar.gz",
		}},
		{"b12110", "amd64", true, []string{
			"llama-b12110-bin-ubuntu-cuda-13.4-x64.tar.gz",
			"cudart-llama-b12110-bin-ubuntu-cuda-13.4-x64.tar.gz",
		}},
		{"b12110", "arm64", true, []string{
			"llama-b12110-bin-ubuntu-cuda-13.4-arm64.tar.gz",
			"cudart-llama-b12110-bin-ubuntu-cuda-13.4-arm64.tar.gz",
		}},
		{"b12167", "amd64", false, []string{
			"llama-b12167-bin-ubuntu-cuda-12.8-x64.tar.gz",
			"cudart-llama-b12167-bin-ubuntu-cuda-12.8-x64.tar.gz",
		}},
		{"b12167", "amd64", true, []string{
			"llama-b12167-bin-ubuntu-cuda-13.4-x64.tar.gz",
			"cudart-llama-b12167-bin-ubuntu-cuda-13.4-x64.tar.gz",
		}},
		{"b12167", "arm64", true, []string{
			"llama-b12167-bin-ubuntu-cuda-13.4-arm64.tar.gz",
			"cudart-llama-b12167-bin-ubuntu-cuda-13.4-arm64.tar.gz",
		}},
	}
	for _, c := range cases {
		got, err := PickAssets(buildFor(t, c.tag), "linux", c.arch, scan.CUDA, c.cuda13)
		if err != nil {
			t.Errorf("%s linux/%s cuda13=%v: %v", c.tag, c.arch, c.cuda13, err)
			continue
		}
		names := assetNames(got)
		if len(names) != len(c.want) || names[0] != c.want[0] || (len(names) > 1 && names[1] != c.want[1]) {
			t.Errorf("%s linux/%s cuda13=%v: got %v, want %v", c.tag, c.arch, c.cuda13, names, c.want)
		}
	}
}

// TestLinuxCUDAAssetShapeUnchanged strips the build tag from every expected
// name. All 117 builds from b11960 to b12167 published exactly these six
// names, so a rename upstream would show up here as a single diff.
func TestLinuxCUDAAssetShapeUnchanged(t *testing.T) {
	want := map[string]bool{
		"bin-ubuntu-cuda-12.8-x64":                true,
		"bin-ubuntu-cuda-13.4-x64":                true,
		"bin-ubuntu-cuda-13.4-arm64":              true,
		"cudart-llama-bin-ubuntu-cuda-12.8-x64":   true,
		"cudart-llama-bin-ubuntu-cuda-13.4-x64":   true,
		"cudart-llama-bin-ubuntu-cuda-13.4-arm64": true,
	}
	seen := map[string]bool{}
	for tag, r := range linuxCUDABuilds {
		for _, a := range r.Assets {
			if !strings.Contains(a.Name, "ubuntu-cuda") {
				continue
			}
			shape := strings.ReplaceAll(a.Name, "-"+tag, "")
			shape = strings.TrimSuffix(strings.TrimPrefix(shape, "llama-"), ".tar.gz")
			seen[shape] = true
		}
	}
	for shape := range seen {
		if !want[shape] {
			t.Errorf("unexpected Linux CUDA asset shape %q", shape)
		}
	}
	for shape := range want {
		if !seen[shape] {
			t.Errorf("missing Linux CUDA asset shape %q", shape)
		}
	}
}

// TestLinuxCUDARTStaysOnLinuxAndArch guards the runtime archive pairing: the
// Windows and Linux runtimes share the "-cuda-13.4-x64" suffix, so the Linux
// pick must never pull a Windows cudart (or the other architecture's).
func TestLinuxCUDARTStaysOnLinuxAndArch(t *testing.T) {
	r := &Release{Tag: "b99999", Assets: []Asset{
		{Name: "llama-b99999-bin-ubuntu-cuda-13.4-x64.tar.gz"},
		{Name: "cudart-llama-bin-win-cuda-13.4-x64.zip"},
		{Name: "cudart-llama-b99999-bin-ubuntu-cuda-13.4-arm64.tar.gz"},
		{Name: "cudart-llama-b99999-bin-ubuntu-cuda-13.4-x64.tar.gz"},
	}}
	got, err := PickAssets(r, "linux", "amd64", scan.CUDA, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Name != "cudart-llama-b99999-bin-ubuntu-cuda-13.4-x64.tar.gz" {
		t.Errorf("cudart pairing = %v", assetNames(got))
	}
}

// TestLinuxCUDAAssetFormat pins the archive format: Linux CUDA builds ship as
// .tar.gz, and a .zip with the same name would still be accepted by matches().
// The test states the format so a future zip-only change is noticed.
func TestLinuxCUDAAssetFormat(t *testing.T) {
	for tag := range linuxCUDABuilds {
		got, err := PickAssets(buildFor(t, tag), "linux", "amd64", scan.CUDA, false)
		if err != nil {
			t.Errorf("%s: %v", tag, err)
			continue
		}
		for _, a := range got {
			if !strings.HasSuffix(a.Name, ".tar.gz") {
				t.Errorf("%s: picked %s, want a .tar.gz", tag, a.Name)
			}
		}
	}
}

// TestLinuxCUDAFallbackWhenNoCUDABuild checks the documented fallback chain
// for a Linux CUDA machine on a release without CUDA archives: Vulkan first,
// then the plain CPU build.
func TestLinuxCUDAFallbackWhenNoCUDABuild(t *testing.T) {
	r := &Release{Tag: "b99999", Assets: []Asset{
		{Name: "llama-b99999-bin-ubuntu-x64.tar.gz"},
		{Name: "llama-b99999-bin-ubuntu-vulkan-x64.tar.gz"},
	}}
	got, err := PickAssets(r, "linux", "amd64", scan.CUDA, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "llama-b99999-bin-ubuntu-vulkan-x64.tar.gz" {
		t.Errorf("fallback = %v", assetNames(got))
	}

	r = &Release{Tag: "b99999", Assets: []Asset{{Name: "llama-b99999-bin-ubuntu-x64.tar.gz"}}}
	got, err = PickAssets(r, "linux", "amd64", scan.CUDA, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "llama-b99999-bin-ubuntu-x64.tar.gz" {
		t.Errorf("fallback = %v", assetNames(got))
	}
}

// TestPinnedReleaseHasLinuxCUDA keeps the offline fallback usable on Linux:
// pinned.go must carry both the CUDA 12.8 x64 build and the CUDA 13.4
// builds, each with its matching cudart archive.
func TestPinnedReleaseHasLinuxCUDA(t *testing.T) {
	for _, c := range []struct {
		arch   string
		cuda13 bool
		want   string
	}{
		{"amd64", false, "llama-b11960-bin-ubuntu-cuda-12.8-x64.tar.gz"},
		{"amd64", true, "llama-b11960-bin-ubuntu-cuda-13.4-x64.tar.gz"},
		{"arm64", true, "llama-b11960-bin-ubuntu-cuda-13.4-arm64.tar.gz"},
	} {
		pr := pinnedRelease
		got, err := PickAssets(&pr, "linux", c.arch, scan.CUDA, c.cuda13)
		if err != nil {
			t.Errorf("linux/%s cuda13=%v: %v", c.arch, c.cuda13, err)
			continue
		}
		if len(got) != 2 || got[0].Name != c.want {
			t.Errorf("linux/%s cuda13=%v: got %v, want %s plus cudart", c.arch, c.cuda13, assetNames(got), c.want)
		}
	}
}
