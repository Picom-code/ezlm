package engine

import (
	"testing"

	"github.com/picom-code/ezlm/internal/scan"
)

func TestFlavor(t *testing.T) {
	if got := flavor("llama-b11960-bin-win-cuda-13.4-x64.zip", "b11960"); got != "win-cuda-13.4-x64" {
		t.Errorf("flavor = %q", got)
	}
}

// Asset names as published in llama.cpp b11960.
var release = &Release{Tag: "b11960", Assets: []Asset{
	{Name: "llama-b11960-bin-macos-arm64.tar.gz"},
	{Name: "llama-b11960-bin-ubuntu-x64.tar.gz"},
	{Name: "llama-b11960-bin-ubuntu-vulkan-x64.tar.gz"},
	{Name: "llama-b11960-bin-ubuntu-cuda-12.8-x64.tar.gz"},
	{Name: "llama-b11960-bin-ubuntu-cuda-13.4-x64.tar.gz"},
	{Name: "cudart-llama-b11960-bin-ubuntu-cuda-12.8-x64.tar.gz"},
	{Name: "cudart-llama-b11960-bin-ubuntu-cuda-13.4-x64.tar.gz"},
	{Name: "llama-b11960-bin-win-cuda-12.4-x64.zip"},
	{Name: "llama-b11960-bin-win-cuda-13.4-x64.zip"},
	{Name: "cudart-llama-bin-win-cuda-12.4-x64.zip"},
	{Name: "cudart-llama-bin-win-cuda-13.4-x64.zip"},
	{Name: "llama-b11960-bin-win-vulkan-x64.zip"},
	{Name: "llama-b11960-bin-win-cpu-x64.zip"},
	{Name: "llama-b11960-bin-win-cuda-13.4-arm64.zip"},
	{Name: "cudart-llama-bin-win-cuda-13.4-arm64.zip"},
	{Name: "llama-b11960-bin-win-vulkan-arm64.zip"},
	{Name: "llama-b11960-bin-win-cpu-arm64.zip"},
}}

func TestPickAssets(t *testing.T) {
	cases := []struct {
		goos, arch string
		b          scan.Backend
		cuda13     bool
		want       []string
	}{
		{"darwin", "arm64", scan.Metal, false, []string{"llama-b11960-bin-macos-arm64.tar.gz"}},
		{"windows", "amd64", scan.CUDA, false, []string{"llama-b11960-bin-win-cuda-12.4-x64.zip", "cudart-llama-bin-win-cuda-12.4-x64.zip"}},
		{"windows", "amd64", scan.CUDA, true, []string{"llama-b11960-bin-win-cuda-13.4-x64.zip", "cudart-llama-bin-win-cuda-13.4-x64.zip"}},
		{"windows", "arm64", scan.CUDA, true, []string{"llama-b11960-bin-win-cuda-13.4-arm64.zip", "cudart-llama-bin-win-cuda-13.4-arm64.zip"}},
		{"windows", "amd64", scan.Vulkan, false, []string{"llama-b11960-bin-win-vulkan-x64.zip"}},
		{"linux", "amd64", scan.CUDA, false, []string{"llama-b11960-bin-ubuntu-cuda-12.8-x64.tar.gz", "cudart-llama-b11960-bin-ubuntu-cuda-12.8-x64.tar.gz"}},
		{"linux", "amd64", scan.CPU, false, []string{"llama-b11960-bin-ubuntu-x64.tar.gz"}},
	}
	for _, c := range cases {
		got, err := PickAssets(release, c.goos, c.arch, c.b, c.cuda13)
		if err != nil {
			t.Errorf("%s/%s %s: %v", c.goos, c.arch, c.b, err)
			continue
		}
		var names []string
		for _, a := range got {
			names = append(names, a.Name)
		}
		if len(names) != len(c.want) || names[0] != c.want[0] || (len(names) > 1 && names[1] != c.want[1]) {
			t.Errorf("%s/%s %s cuda13=%v: got %v, want %v", c.goos, c.arch, c.b, c.cuda13, names, c.want)
		}
	}
}

// The pinned fallback must cover every platform EZLM supports.
func TestPinnedReleaseCoversPlatforms(t *testing.T) {
	cases := []struct {
		goos, arch string
		b          scan.Backend
		cuda13     bool
	}{
		{"windows", "amd64", scan.CUDA, false}, {"windows", "amd64", scan.CUDA, true}, {"windows", "amd64", scan.Vulkan, false},
		{"windows", "amd64", scan.CPU, false}, {"windows", "arm64", scan.CUDA, true}, {"windows", "arm64", scan.CPU, false},
		{"darwin", "arm64", scan.Metal, false}, {"linux", "amd64", scan.Vulkan, false}, {"linux", "amd64", scan.CPU, false},
	}
	for _, c := range cases {
		pr := pinnedRelease
		if _, err := PickAssets(&pr, c.goos, c.arch, c.b, c.cuda13); err != nil {
			t.Errorf("%s/%s %s cuda13=%v: %v", c.goos, c.arch, c.b, c.cuda13, err)
		}
	}
}
