package engine

import (
	"testing"

	"github.com/picom-code/ezlm/internal/scan"
)

// windowsAssetsB12110 lists every Windows asset published in llama.cpp
// b12110, the build that release v0.9.0 (2026-11-04) points at through
// nightly-tag.txt. Names come from
// https://api.github.com/repos/ggml-org/llama.cpp/releases/tags/b12110.
var windowsAssetsB12110 = []string{
	"cudart-llama-bin-win-cuda-12.4-x64.zip",
	"cudart-llama-bin-win-cuda-13.4-arm64.zip",
	"cudart-llama-bin-win-cuda-13.4-x64.zip",
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
}

func namedRelease(tag string, names []string) *Release {
	r := &Release{Tag: tag}
	for _, n := range names {
		r.Assets = append(r.Assets, Asset{Name: n, URL: "https://example.invalid/" + n, Size: 1})
	}
	return r
}

// assetNames is the selection result as names, for table comparisons.
func assetNames_wincuda(as []Asset) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.Name
	}
	return out
}

func equalNames(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// Windows x64 CUDA 12 must resolve to the same two archives at the pinned
// build and at the build the latest release points at. The CUDA runtime
// archive is not tagged with the build number, so it is matched by suffix.
func TestPickAssetsWindowsCUDA12X64(t *testing.T) {
	pinned := pinnedRelease
	cases := []struct {
		name string
		rel  *Release
		want []string
	}{
		{
			name: "b11960",
			rel:  &pinned,
			want: []string{
				"llama-b11960-bin-win-cuda-12.4-x64.zip",
				"cudart-llama-bin-win-cuda-12.4-x64.zip",
			},
		},
		{
			name: "b12110 (latest release v0.9.0)",
			rel:  namedRelease("b12110", windowsAssetsB12110),
			want: []string{
				"llama-b12110-bin-win-cuda-12.4-x64.zip",
				"cudart-llama-bin-win-cuda-12.4-x64.zip",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := PickAssets(c.rel, "windows", "amd64", scan.CUDA, false)
			if err != nil {
				t.Fatalf("PickAssets: %v", err)
			}
			if names := assetNames_wincuda(got); !equalNames(names, c.want) {
				t.Fatalf("got %v, want %v", names, c.want)
			}
			// Both names must be real assets of the release, not invented.
			for _, n := range c.want {
				if _, ok := findAsset(c.rel, n); !ok {
					t.Errorf("%s is not an asset of %s", n, c.rel.Tag)
				}
			}
			// The runtime must be the Windows one: Linux runtimes share the
			// same -cuda-12.4-x64 suffix.
			if !contains(got[1].Name, "-win-") {
				t.Errorf("runtime %s is not a Windows archive", got[1].Name)
			}
			if got[0].Size == 0 || got[0].URL == "" {
				t.Errorf("main asset missing size or URL: %+v", got[0])
			}
		})
	}
}

func findAsset(r *Release, name string) (Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// A CUDA 12 machine must never be handed a CUDA 13 archive as a fallback:
// its driver is older than 13.0, so the build would not start. Vulkan is the
// next best flavor that still runs.
func TestPickAssetsWindowsCUDA12FallbackIsVulkan(t *testing.T) {
	// A release that dropped the CUDA 12 x64 build, as upstream eventually will.
	rel := namedRelease("b99999", []string{
		"llama-b99999-bin-win-cuda-13.4-x64.zip",
		"cudart-llama-bin-win-cuda-13.4-x64.zip",
		"llama-b99999-bin-win-vulkan-x64.zip",
		"llama-b99999-bin-win-cpu-x64.zip",
	})
	got, err := PickAssets(rel, "windows", "amd64", scan.CUDA, false)
	if err != nil {
		t.Fatalf("PickAssets: %v", err)
	}
	if names := assetNames_wincuda(got); !equalNames(names, []string{"llama-b99999-bin-win-vulkan-x64.zip"}) {
		t.Fatalf("got %v, want the Vulkan archive", names)
	}
}
