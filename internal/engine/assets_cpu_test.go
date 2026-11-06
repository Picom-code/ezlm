package engine

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/scan"
)

// latestCPURelease is the build the current stable release points at: v0.9.0's
// nightly-tag.txt names b12110, not the newest nightly (b12167).
const latestCPURelease = "b12110"

// cpuAssets is a llama.cpp release as far as CPU builds are concerned: the
// plain CPU archives for every OS, plus the GPU and accelerator archives that
// must never be picked for a CPU backend. The order follows the release API,
// with the b12110-only s390x extras appended.
func cpuAssets(tag string) *Release {
	names := []string{
		"cudart-llama-%s-bin-ubuntu-cuda-12.8-x64.tar.gz",
		"cudart-llama-%s-bin-ubuntu-cuda-13.4-arm64.tar.gz",
		"cudart-llama-%s-bin-ubuntu-cuda-13.4-x64.tar.gz",
		"cudart-llama-bin-win-cuda-12.4-x64.zip",
		"cudart-llama-bin-win-cuda-13.4-arm64.zip",
		"cudart-llama-bin-win-cuda-13.4-x64.zip",
		"llama-%s-bin-android-arm64-snapdragon.tar.gz",
		"llama-%s-bin-android-arm64.tar.gz",
		"llama-%s-bin-linux-arm64-snapdragon.tar.gz",
		"llama-%s-bin-macos-arm64.tar.gz",
		"llama-%s-bin-macos-x64.tar.gz",
		"llama-%s-bin-ubuntu-arm64.tar.gz",
		"llama-%s-bin-ubuntu-cuda-12.8-x64.tar.gz",
		"llama-%s-bin-ubuntu-cuda-13.4-arm64.tar.gz",
		"llama-%s-bin-ubuntu-cuda-13.4-x64.tar.gz",
		"llama-%s-bin-ubuntu-openvino-2026.4.1-x64.tar.gz",
		"llama-%s-bin-ubuntu-rocm-10.0-x64.tar.gz",
		"llama-%s-bin-ubuntu-s390x.tar.gz",
		"llama-%s-bin-ubuntu-sycl-fp16-x64.tar.gz",
		"llama-%s-bin-ubuntu-sycl-fp32-x64.tar.gz",
		"llama-%s-bin-ubuntu-vulkan-arm64.tar.gz",
		"llama-%s-bin-ubuntu-vulkan-x64.tar.gz",
		"llama-%s-bin-ubuntu-x64.tar.gz",
		"llama-%s-bin-win-cpu-arm64.zip",
		"llama-%s-bin-win-cpu-x64.zip",
		"llama-%s-bin-win-cuda-12.4-x64.zip",
		"llama-%s-bin-win-cuda-13.4-arm64.zip",
		"llama-%s-bin-win-cuda-13.4-x64.zip",
		"llama-%s-bin-win-opencl-adreno-arm64.zip",
		"llama-%s-bin-win-openvino-2026.4.1-x64.zip",
		"llama-%s-bin-win-rocm-10.0-x64.zip",
		"llama-%s-bin-win-sycl-x64.zip",
		"llama-%s-bin-win-vulkan-arm64.zip",
		"llama-%s-bin-win-vulkan-x64.zip",
		"llama-%s-ui.tar.gz",
		"llama-%s-xcframework.zip",
	}
	if tag != "b11960" {
		// s390x picked up two more variants between b11960 and b12110; the
		// plain ubuntu-s390x archive is still the one a CPU backend wants.
		names = append(names,
			"llama-%s-bin-ubuntu-26.04-s390x.tar.gz",
			"llama-%s-bin-ubuntu-zdnn-s390x.tar.gz")
	}
	r := &Release{Tag: tag}
	for _, n := range names {
		r.Assets = append(r.Assets, Asset{Name: fmt.Sprintf(n, tag)})
	}
	return r
}

// TestPickAssetsCPU checks that every OS and architecture with a CPU-only
// build gets that build, on the pinned release and on the latest one, and
// never a GPU archive or a CUDA runtime.
func TestPickAssetsCPU(t *testing.T) {
	cases := []struct {
		goos, arch string
		b          scan.Backend
		want       string // format: tag substituted
	}{
		{"darwin", "arm64", scan.Metal, "llama-%s-bin-macos-arm64.tar.gz"},
		{"darwin", "amd64", scan.Metal, "llama-%s-bin-macos-x64.tar.gz"},
		{"windows", "amd64", scan.CPU, "llama-%s-bin-win-cpu-x64.zip"},
		{"windows", "arm64", scan.CPU, "llama-%s-bin-win-cpu-arm64.zip"},
		{"linux", "amd64", scan.CPU, "llama-%s-bin-ubuntu-x64.tar.gz"},
		{"linux", "arm64", scan.CPU, "llama-%s-bin-ubuntu-arm64.tar.gz"},
		{"linux", "s390x", scan.CPU, "llama-%s-bin-ubuntu-s390x.tar.gz"},
	}
	for _, tag := range []string{"b11960", latestCPURelease} {
		r := cpuAssets(tag)
		for _, c := range cases {
			got, err := PickAssets(r, c.goos, c.arch, c.b, false)
			if err != nil {
				t.Errorf("%s %s/%s: %v", tag, c.goos, c.arch, err)
				continue
			}
			want := fmt.Sprintf(c.want, tag)
			if len(got) != 1 {
				t.Errorf("%s %s/%s: got %d assets (%v), want just %s", tag, c.goos, c.arch, len(got), names(got), want)
				continue
			}
			if got[0].Name != want {
				t.Errorf("%s %s/%s: got %s, want %s", tag, c.goos, c.arch, got[0].Name, want)
			}
			// CPU archives are self-contained; a cudart archive alongside
			// would be a GPU build in disguise.
			if strings.HasPrefix(got[0].Name, "cudart-") || strings.Contains(got[0].Name, "-cuda-") {
				t.Errorf("%s %s/%s: picked a CUDA asset %s", tag, c.goos, c.arch, got[0].Name)
			}
		}
	}
}

// TestCPUFlavorKeepsBuildsApart checks the per-build folder name, so a CPU
// install never reuses a GPU folder (and its DLLs or .so files).
func TestCPUFlavorKeepsBuildsApart(t *testing.T) {
	r := cpuAssets(latestCPURelease)
	cases := []struct {
		goos, arch string
		b          scan.Backend
		cuda13     bool
	}{
		{"windows", "amd64", scan.CPU, false},
		{"windows", "amd64", scan.CUDA, false},
		{"windows", "amd64", scan.Vulkan, false},
		{"linux", "amd64", scan.CPU, false},
		{"linux", "amd64", scan.CUDA, false},
		{"linux", "amd64", scan.Vulkan, false},
	}
	seen := map[string]string{}
	for _, c := range cases {
		got, err := PickAssets(r, c.goos, c.arch, c.b, c.cuda13)
		if err != nil {
			t.Fatalf("%s/%s %s: %v", c.goos, c.arch, c.b, err)
		}
		f := flavor(got[0].Name, r.Tag)
		key := c.goos + "/" + string(c.b)
		if prev, ok := seen[f]; ok {
			t.Errorf("%s and %s share build folder %q", prev, key, f)
		}
		seen[f] = key
	}
}

// runtimeSets is what each CPU archive must contain for llama-server to run.
// Windows and Linux x64 lists come from the published archives (zip central
// directory for Windows, streamed tar listing for Linux); the rest follow the
// same build targets at the same tags.
var runtimeSets = []struct {
	archive string
	// top is the folder the archive unpacks into, "" for the Windows zips.
	top   string
	files []string
	// links maps a symlink name to its target, as Linux and macOS ship
	// versioned libraries.
	links map[string]string
}{
	{
		archive: "llama-b12110-bin-win-cpu-x64.zip",
		files: []string{
			"llama-server.exe", "llama-server-impl.dll", "llama.dll", "llama-common.dll",
			"ggml.dll", "ggml-base.dll", "ggml-cpu-x64.dll", "ggml-cpu-haswell.dll",
			"ggml-rpc.dll", "mtmd.dll", "libomp.dll",
		},
	},
	{
		archive: "llama-b12110-bin-win-cpu-arm64.zip",
		files: []string{
			"llama-server.exe", "llama-server-impl.dll", "llama.dll", "llama-common.dll",
			"ggml.dll", "ggml-base.dll", "ggml-cpu.dll", "ggml-rpc.dll", "mtmd.dll", "libomp.dll",
		},
	},
	{
		archive: "llama-b12110-bin-ubuntu-x64.tar.gz",
		top:     "llama-b12110",
		files: []string{
			"llama-server", "libllama.so.0.9.0", "libllama-common.so.0.9.0",
			"libggml.so.0.9.0", "libggml-base.so.0.9.0", "libmtmd.so.0.9.0",
			"libggml-cpu-x64.so", "libggml-cpu-haswell.so", "libggml-rpc.so",
		},
		links: map[string]string{"libllama.so.0": "libllama.so.0.9.0"},
	},
	{
		archive: "llama-b12110-bin-ubuntu-arm64.tar.gz",
		top:     "llama-b12110",
		files: []string{
			"llama-server", "libllama.so.0.9.0", "libllama-common.so.0.9.0",
			"libggml.so.0.9.0", "libggml-base.so.0.9.0", "libmtmd.so.0.9.0",
			"libggml-cpu-armv8.0_1.so", "libggml-cpu-armv8.2_1.so", "libggml-rpc.so",
		},
	},
	{
		archive: "llama-b12110-bin-macos-arm64.tar.gz",
		top:     "llama-b12110",
		files: []string{
			"llama-server", "libllama.dylib", "libllama-common.dylib", "libggml.dylib",
			"libggml-base.dylib", "libggml-cpu.dylib", "libggml-metal.dylib",
			"libggml-rpc.dylib", "libmtmd.dylib",
		},
	},
	{
		archive: "llama-b12110-bin-macos-x64.tar.gz",
		top:     "llama-b12110",
		files: []string{
			"llama-server", "libllama.dylib", "libllama-common.dylib", "libggml.dylib",
			"libggml-base.dylib", "libggml-cpu.dylib", "libggml-rpc.dylib", "libmtmd.dylib",
		},
	},
}

// TestCPUArchiveRuntimeSet unpacks a synthetic archive shaped like each CPU
// release and checks that every runtime file survives, that libraries stay
// executable and that llama-server is still found after extraction.
func TestCPUArchiveRuntimeSet(t *testing.T) {
	for _, set := range runtimeSets {
		t.Run(set.archive, func(t *testing.T) {
			dst := t.TempDir()
			arc := filepath.Join(t.TempDir(), set.archive)
			if strings.HasSuffix(set.archive, ".zip") {
				writeTestZip(t, arc, set.top, set.files)
			} else {
				writeTestTar(t, arc, set.top, set.files, set.links)
			}
			if err := Extract(arc, dst); err != nil {
				t.Fatalf("extract: %v", err)
			}
			dir := filepath.Join(dst, set.top)
			for _, f := range set.files {
				p := filepath.Join(dir, f)
				st, err := os.Stat(p)
				if err != nil {
					t.Errorf("%s missing after extraction: %v", f, err)
					continue
				}
				if !strings.HasSuffix(f, ".dll") && st.Mode().Perm()&0o111 == 0 {
					t.Errorf("%s not executable after extraction: %v", f, st.Mode())
				}
			}
			for link, target := range set.links {
				got, err := os.Readlink(filepath.Join(dir, link))
				if err != nil || got != target {
					t.Errorf("%s -> %q (%v), want %q", link, got, err, target)
				}
			}
			// findServer looks for the host's server name, so only the
			// archive for this OS can be checked here.
			if strings.HasSuffix(set.archive, ".tar.gz") {
				if srv := findServer(dst); srv == "" {
					t.Error("llama-server not found after extraction")
				}
			}
		})
	}
}

func names(as []Asset) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.Name
	}
	return out
}

func writeTestZip(t *testing.T, path, top string, files []string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for _, name := range files {
		if top != "" {
			name = top + "/" + name
		}
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(w, "test")
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeTestTar(t *testing.T, path, top string, files []string, links map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	add := func(h *tar.Header) {
		t.Helper()
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
	}
	if top != "" {
		add(&tar.Header{Name: top + "/", Typeflag: tar.TypeDir, Mode: 0o755})
	}
	for _, name := range files {
		mode := int64(0o644)
		if strings.Contains(name, ".so") || strings.Contains(name, ".dylib") || strings.HasPrefix(name, "llama-") {
			mode = 0o755
		}
		p := name
		if top != "" {
			p = top + "/" + name
		}
		add(&tar.Header{Name: p, Typeflag: tar.TypeReg, Mode: mode, Size: 4})
		if _, err := tw.Write([]byte("test")); err != nil {
			t.Fatal(err)
		}
	}
	for link, target := range links {
		p := link
		if top != "" {
			p = top + "/" + link
		}
		add(&tar.Header{Name: p, Typeflag: tar.TypeSymlink, Linkname: target, Mode: 0o777})
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}
