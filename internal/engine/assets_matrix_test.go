package engine

import (
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/scan"
)

// The asset names published for the two releases this study compares:
//
//   - b11960: the build pinnedRelease falls back to.
//   - b12110: what releases/latest (v0.9.0, 2026-11-04) points at through its
//     nightly-tag.txt. The v0.9.0 release itself carries no binaries.
//
// Both lists are complete as of 2026-11-04; keep them sorted the way the API
// returns them so PickAssets is exercised against the real ordering.
var matrixReleases = []struct {
	tag   string
	names []string
}{
	{"b11960", []string{
		"cudart-llama-b11960-bin-ubuntu-cuda-12.8-x64.tar.gz",
		"cudart-llama-b11960-bin-ubuntu-cuda-13.4-arm64.tar.gz",
		"cudart-llama-b11960-bin-ubuntu-cuda-13.4-x64.tar.gz",
		"cudart-llama-bin-win-cuda-12.4-x64.zip",
		"cudart-llama-bin-win-cuda-13.4-arm64.zip",
		"cudart-llama-bin-win-cuda-13.4-x64.zip",
		"llama-b11960-bin-android-arm64-snapdragon.tar.gz",
		"llama-b11960-bin-android-arm64.tar.gz",
		"llama-b11960-bin-linux-arm64-snapdragon.tar.gz",
		"llama-b11960-bin-macos-arm64.tar.gz",
		"llama-b11960-bin-macos-x64.tar.gz",
		"llama-b11960-bin-ubuntu-arm64.tar.gz",
		"llama-b11960-bin-ubuntu-cuda-12.8-x64.tar.gz",
		"llama-b11960-bin-ubuntu-cuda-13.4-arm64.tar.gz",
		"llama-b11960-bin-ubuntu-cuda-13.4-x64.tar.gz",
		"llama-b11960-bin-ubuntu-openvino-2026.4.1-x64.tar.gz",
		"llama-b11960-bin-ubuntu-rocm-10.0-x64.tar.gz",
		"llama-b11960-bin-ubuntu-s390x.tar.gz",
		"llama-b11960-bin-ubuntu-sycl-fp16-x64.tar.gz",
		"llama-b11960-bin-ubuntu-sycl-fp32-x64.tar.gz",
		"llama-b11960-bin-ubuntu-vulkan-arm64.tar.gz",
		"llama-b11960-bin-ubuntu-vulkan-x64.tar.gz",
		"llama-b11960-bin-ubuntu-x64.tar.gz",
		"llama-b11960-bin-win-cpu-arm64.zip",
		"llama-b11960-bin-win-cpu-x64.zip",
		"llama-b11960-bin-win-cuda-12.4-x64.zip",
		"llama-b11960-bin-win-cuda-13.4-arm64.zip",
		"llama-b11960-bin-win-cuda-13.4-x64.zip",
		"llama-b11960-bin-win-opencl-adreno-arm64.zip",
		"llama-b11960-bin-win-openvino-2026.4.1-x64.zip",
		"llama-b11960-bin-win-rocm-10.0-x64.zip",
		"llama-b11960-bin-win-sycl-x64.zip",
		"llama-b11960-bin-win-vulkan-arm64.zip",
		"llama-b11960-bin-win-vulkan-x64.zip",
	}},
	{"b12110", []string{
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
	}},
}

// assetMatrix is the index of every platform and backend EZLM can choose a
// build for. "{tag}" is replaced with the release tag. Each row lists the
// exact archives PickAssets must return, in order: the server build first and
// its matching CUDA runtime archive second.
//
// Rows whose result looks surprising are intentional and cross-referenced:
//   - ROCm is routed to Vulkan (see a06); the rocm-10.0 archives are published
//     but never selected.
//   - arm64 has no CUDA 12 build, so the CUDA fallback picks CUDA 13.
var assetMatrix = []struct {
	platform string
	goos     string
	arch     string
	backend  scan.Backend
	cuda13   bool
	want     []string
}{
	{"macos-arm64-metal", "darwin", "arm64", scan.Metal, false,
		[]string{"llama-{tag}-bin-macos-arm64.tar.gz"}},
	// Intel Macs report CPU, but selection is the same either way.
	{"macos-x64-metal", "darwin", "amd64", scan.Metal, false,
		[]string{"llama-{tag}-bin-macos-x64.tar.gz"}},
	{"macos-x64-cpu", "darwin", "amd64", scan.CPU, false,
		[]string{"llama-{tag}-bin-macos-x64.tar.gz"}},

	{"linux-x64-cuda12", "linux", "amd64", scan.CUDA, false,
		[]string{"llama-{tag}-bin-ubuntu-cuda-12.8-x64.tar.gz", "cudart-llama-{tag}-bin-ubuntu-cuda-12.8-x64.tar.gz"}},
	{"linux-x64-cuda13", "linux", "amd64", scan.CUDA, true,
		[]string{"llama-{tag}-bin-ubuntu-cuda-13.4-x64.tar.gz", "cudart-llama-{tag}-bin-ubuntu-cuda-13.4-x64.tar.gz"}},
	{"linux-x64-vulkan", "linux", "amd64", scan.Vulkan, false,
		[]string{"llama-{tag}-bin-ubuntu-vulkan-x64.tar.gz"}},
	{"linux-x64-rocm", "linux", "amd64", scan.ROCm, false,
		[]string{"llama-{tag}-bin-ubuntu-vulkan-x64.tar.gz"}},
	{"linux-x64-cpu", "linux", "amd64", scan.CPU, false,
		[]string{"llama-{tag}-bin-ubuntu-x64.tar.gz"}},
	{"linux-arm64-cuda13", "linux", "arm64", scan.CUDA, true,
		[]string{"llama-{tag}-bin-ubuntu-cuda-13.4-arm64.tar.gz", "cudart-llama-{tag}-bin-ubuntu-cuda-13.4-arm64.tar.gz"}},
	{"linux-arm64-cuda12", "linux", "arm64", scan.CUDA, false,
		[]string{"llama-{tag}-bin-ubuntu-cuda-13.4-arm64.tar.gz", "cudart-llama-{tag}-bin-ubuntu-cuda-13.4-arm64.tar.gz"}},
	{"linux-arm64-vulkan", "linux", "arm64", scan.Vulkan, false,
		[]string{"llama-{tag}-bin-ubuntu-vulkan-arm64.tar.gz"}},
	{"linux-arm64-cpu", "linux", "arm64", scan.CPU, false,
		[]string{"llama-{tag}-bin-ubuntu-arm64.tar.gz"}},

	{"windows-x64-cuda12", "windows", "amd64", scan.CUDA, false,
		[]string{"llama-{tag}-bin-win-cuda-12.4-x64.zip", "cudart-llama-bin-win-cuda-12.4-x64.zip"}},
	{"windows-x64-cuda13", "windows", "amd64", scan.CUDA, true,
		[]string{"llama-{tag}-bin-win-cuda-13.4-x64.zip", "cudart-llama-bin-win-cuda-13.4-x64.zip"}},
	{"windows-x64-vulkan", "windows", "amd64", scan.Vulkan, false,
		[]string{"llama-{tag}-bin-win-vulkan-x64.zip"}},
	{"windows-x64-rocm", "windows", "amd64", scan.ROCm, false,
		[]string{"llama-{tag}-bin-win-vulkan-x64.zip"}},
	{"windows-x64-cpu", "windows", "amd64", scan.CPU, false,
		[]string{"llama-{tag}-bin-win-cpu-x64.zip"}},
	{"windows-arm64-cuda13", "windows", "arm64", scan.CUDA, true,
		[]string{"llama-{tag}-bin-win-cuda-13.4-arm64.zip", "cudart-llama-bin-win-cuda-13.4-arm64.zip"}},
	{"windows-arm64-cuda12", "windows", "arm64", scan.CUDA, false,
		[]string{"llama-{tag}-bin-win-cuda-13.4-arm64.zip", "cudart-llama-bin-win-cuda-13.4-arm64.zip"}},
	{"windows-arm64-vulkan", "windows", "arm64", scan.Vulkan, false,
		[]string{"llama-{tag}-bin-win-vulkan-arm64.zip"}},
	{"windows-arm64-cpu", "windows", "arm64", scan.CPU, false,
		[]string{"llama-{tag}-bin-win-cpu-arm64.zip"}},
}

// unselectedFlavors lists published archives that no platform/backend row can
// pick, with the reason. Anything not here must be covered by assetMatrix, so
// a new upstream asset that EZLM ought to use fails
// TestAssetMatrixCoversEveryReleaseAsset instead of being noticed by hand.
var unselectedFlavors = map[string]string{
	"android-arm64-snapdragon.tar.gz":     "no Android target",
	"android-arm64.tar.gz":                "no Android target",
	"linux-arm64-snapdragon.tar.gz":       "Snapdragon builds are not offered",
	"ubuntu-26.04-s390x.tar.gz":           "no s390x target",
	"ubuntu-openvino-2026.4.1-x64.tar.gz": "OpenVINO is not offered",
	"ubuntu-rocm-10.0-x64.tar.gz":         "ROCm routes to Vulkan (a06)",
	"ubuntu-s390x.tar.gz":                 "no s390x target",
	"ubuntu-sycl-fp16-x64.tar.gz":         "SYCL is not offered",
	"ubuntu-sycl-fp32-x64.tar.gz":         "SYCL is not offered",
	"ubuntu-zdnn-s390x.tar.gz":            "no s390x target",
	"win-opencl-adreno-arm64.zip":         "OpenCL is not offered",
	"win-openvino-2026.4.1-x64.zip":       "OpenVINO is not offered",
	"win-rocm-10.0-x64.zip":               "ROCm routes to Vulkan (a06)",
	"win-sycl-x64.zip":                    "SYCL is not offered",
	"ui.tar.gz":                           "web UI bundle, not a server build",
	"xcframework.zip":                     "Apple framework bundle, not a server build",
}

// assetFlavor is the part of an asset name after the tag, e.g.
// "ubuntu-cuda-12.8-x64.tar.gz" for either the llama- or cudart- prefix.
func assetFlavor(name, tag string) string {
	for _, p := range []string{"cudart-llama-" + tag + "-", "llama-" + tag + "-", "cudart-llama-"} {
		if s, ok := strings.CutPrefix(name, p); ok {
			return strings.TrimPrefix(s, "bin-")
		}
	}
	return name
}

func matrixRelease(tag string, names []string) *Release {
	r := &Release{Tag: tag}
	for _, n := range names {
		r.Assets = append(r.Assets, Asset{Name: n})
	}
	return r
}

// TestAssetMatrix is the single table that maps every platform and backend
// EZLM supports to the exact archives it must pick, for the pinned build and
// for the latest release.
func TestAssetMatrix(t *testing.T) {
	for _, rel := range matrixReleases {
		r := matrixRelease(rel.tag, rel.names)
		for _, row := range assetMatrix {
			t.Run(rel.tag+"/"+row.platform, func(t *testing.T) {
				got, err := PickAssets(r, row.goos, row.arch, row.backend, row.cuda13)
				if err != nil {
					t.Fatalf("PickAssets: %v", err)
				}
				var names []string
				for _, a := range got {
					names = append(names, a.Name)
				}
				var want []string
				for _, w := range row.want {
					want = append(want, strings.ReplaceAll(w, "{tag}", rel.tag))
				}
				if len(names) != len(want) {
					t.Fatalf("got %v, want %v", names, want)
				}
				for i := range want {
					if names[i] != want[i] {
						t.Fatalf("got %v, want %v", names, want)
					}
				}
			})
		}
	}
}

// TestAssetMatrixCoversEveryReleaseAsset fails when a published archive is
// neither selected by a matrix row nor listed as intentionally unused. That
// keeps this file the index for every platform agent: a renamed or new asset
// shows up here first.
func TestAssetMatrixCoversEveryReleaseAsset(t *testing.T) {
	for _, rel := range matrixReleases {
		r := matrixRelease(rel.tag, rel.names)
		selected := map[string]bool{}
		for _, row := range assetMatrix {
			got, err := PickAssets(r, row.goos, row.arch, row.backend, row.cuda13)
			if err != nil {
				t.Fatalf("%s %s: %v", rel.tag, row.platform, err)
			}
			for _, a := range got {
				selected[a.Name] = true
			}
		}
		for _, a := range r.Assets {
			if selected[a.Name] {
				continue
			}
			if _, ok := unselectedFlavors[assetFlavor(a.Name, rel.tag)]; ok {
				continue
			}
			t.Errorf("%s: %s is published but no matrix row selects it; add a row or an unselectedFlavors entry", rel.tag, a.Name)
		}
	}
}

// TestAssetMatrixCUDAHasRuntime checks the invariant behind every CUDA row: a
// CUDA server build is useless without the matching cudart archive, and the
// runtime must match the build's CUDA version, architecture and OS.
func TestAssetMatrixCUDAHasRuntime(t *testing.T) {
	for _, rel := range matrixReleases {
		r := matrixRelease(rel.tag, rel.names)
		for _, row := range assetMatrix {
			if row.backend != scan.CUDA {
				continue
			}
			got, err := PickAssets(r, row.goos, row.arch, row.backend, row.cuda13)
			if err != nil {
				t.Fatalf("%s %s: %v", rel.tag, row.platform, err)
			}
			if len(got) != 2 || !strings.HasPrefix(got[1].Name, "cudart-") {
				t.Errorf("%s %s: got %v, want the build plus one cudart archive", rel.tag, row.platform, got)
				continue
			}
			if cudaFlavor(got[0].Name) != cudaFlavor(got[1].Name) {
				t.Errorf("%s %s: build %s but runtime %s", rel.tag, row.platform, got[0].Name, got[1].Name)
			}
		}
	}
}
