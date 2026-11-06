package engine

// Asset selection for the backends EZLM does not scan for (SYCL, OpenCL,
// OpenVINO), Intel Macs, and llama.cpp's asset naming conventions.
// The release payloads these tests read are cached under agents/T06/a08.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/scan"
)

const assetsDir = "../../agents/T06/a08"

func loadRelease(t *testing.T, tag string) *Release {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(assetsDir, "release-"+tag+".json"))
	if err != nil {
		t.Skipf("cached release %s not available: %v", tag, err)
	}
	var r Release
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatalf("parsing release-%s.json: %v", tag, err)
	}
	if r.Tag != tag {
		t.Fatalf("release-%s.json holds tag %q", tag, r.Tag)
	}
	return &r
}

func assetNames_other(as []Asset) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.Name
	}
	return out
}

// b12110 is what releases/latest (v0.9.0) points at through nightly-tag.txt;
// b12144 is the newest build. Both carry the same asset names.
var otherTags = []string{"b11960", "b12110", "b12144"}

// TestPickAssetsOther covers the machines and backends that fall outside the
// per-platform slices: Intel Macs, SYCL, OpenCL, OpenVINO and the tagless
// Windows CUDA runtime archives.
func TestPickAssetsOther(t *testing.T) {
	cases := []struct {
		tag, goos, arch string
		b               scan.Backend
		cuda13          bool
		want            []string
	}{
		// Intel Macs: a plain Metal build, no runtime archive, at every tag.
		{"b11960", "darwin", "amd64", scan.Metal, false, []string{"llama-b11960-bin-macos-x64.tar.gz"}},
		{"b12110", "darwin", "amd64", scan.Metal, false, []string{"llama-b12110-bin-macos-x64.tar.gz"}},
		{"b12144", "darwin", "amd64", scan.Metal, false, []string{"llama-b12144-bin-macos-x64.tar.gz"}},
		// The xcframework zip is a sidecar, never the server.
		{"b12110", "darwin", "amd64", scan.CPU, false, []string{"llama-b12110-bin-macos-x64.tar.gz"}},
		// The backend does not change the macOS pick: there is one build.
		{"b12110", "darwin", "amd64", scan.Backend("sycl"), false, []string{"llama-b12110-bin-macos-x64.tar.gz"}},
		// Windows CUDA runtime archives carry no tag in their name; the pick
		// must still pair them with the tagged server archive after a bump.
		{"b12110", "windows", "amd64", scan.CUDA, false, []string{"llama-b12110-bin-win-cuda-12.4-x64.zip", "cudart-llama-bin-win-cuda-12.4-x64.zip"}},
		{"b12144", "windows", "amd64", scan.CUDA, true, []string{"llama-b12144-bin-win-cuda-13.4-x64.zip", "cudart-llama-bin-win-cuda-13.4-x64.zip"}},
		// Linux runtimes do carry the tag.
		{"b12110", "linux", "amd64", scan.CUDA, false, []string{"llama-b12110-bin-ubuntu-cuda-12.8-x64.tar.gz", "cudart-llama-b12110-bin-ubuntu-cuda-12.8-x64.tar.gz"}},
		// No backend in scan.Backend selects a SYCL, OpenCL or OpenVINO build,
		// so these names fall through to the CPU archive. Documented, not
		// fixed: Intel Arc machines get Vulkan from scan (scan.go:138).
		{"b12110", "linux", "amd64", scan.Backend("sycl"), false, []string{"llama-b12110-bin-ubuntu-x64.tar.gz"}},
		{"b12110", "windows", "amd64", scan.Backend("sycl"), false, []string{"llama-b12110-bin-win-cpu-x64.zip"}},
		{"b12110", "windows", "arm64", scan.Backend("opencl"), false, []string{"llama-b12110-bin-win-cpu-arm64.zip"}},
		{"b12110", "linux", "amd64", scan.Backend("openvino"), false, []string{"llama-b12110-bin-ubuntu-x64.tar.gz"}},
	}
	for _, c := range cases {
		r := loadRelease(t, c.tag)
		got, err := PickAssets(r, c.goos, c.arch, c.b, c.cuda13)
		if err != nil {
			t.Errorf("%s %s/%s %s cuda13=%v: %v", c.tag, c.goos, c.arch, c.b, c.cuda13, err)
			continue
		}
		if names := assetNames_other(got); !equalStrings(names, c.want) {
			t.Errorf("%s %s/%s %s cuda13=%v: got %v, want %v", c.tag, c.goos, c.arch, c.b, c.cuda13, names, c.want)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestPickAssetsUnknownArch: llama.cpp now publishes s390x assets too
// (ubuntu-26.04-s390x, ubuntu-zdnn-s390x). EZLM's apps are x64 and arm64
// only, so an unknown arch must fail loudly instead of matching the first
// "-bin-ubuntu-" archive, which could be CUDA or Vulkan.
func TestPickAssetsUnknownArch(t *testing.T) {
	r := loadRelease(t, "b12110")
	for _, arch := range []string{"ppc64le", "386"} {
		if got, err := PickAssets(r, "linux", arch, scan.CPU, false); err == nil {
			t.Errorf("linux/%s: got %v, want an error", arch, assetNames_other(got))
		}
		if got, err := PickAssets(r, "darwin", arch, scan.Metal, false); err == nil {
			t.Errorf("darwin/%s: got %v, want an error", arch, assetNames_other(got))
		}
	}
}

// TestPickAssetsSidecars checks that the non-server archives llama.cpp ships
// (ui.tar.gz, xcframework.zip) are never picked as the server.
func TestPickAssetsSidecars(t *testing.T) {
	for _, tag := range otherTags {
		r := loadRelease(t, tag)
		for _, c := range []struct {
			goos, arch string
			b          scan.Backend
		}{
			{"darwin", "arm64", scan.Metal}, {"darwin", "amd64", scan.Metal},
			{"linux", "amd64", scan.CPU}, {"windows", "amd64", scan.CPU},
		} {
			got, err := PickAssets(r, c.goos, c.arch, c.b, false)
			if err != nil {
				t.Errorf("%s %s/%s: %v", tag, c.goos, c.arch, err)
				continue
			}
			for _, a := range got {
				if !strings.HasPrefix(a.Name, "llama-"+tag+"-bin-") && !strings.HasPrefix(a.Name, "cudart-") {
					t.Errorf("%s %s/%s: picked sidecar %s", tag, c.goos, c.arch, a.Name)
				}
			}
		}
	}
}

// assetName splits a published asset name into its convention parts.
var assetName = regexp.MustCompile(`^(llama|cudart-llama)(?:-(b[0-9]+))?-bin-([a-z0-9.]+)-(.+?)(\.zip|\.tar\.gz)$`)

// TestAssetNameConventions parses every asset name in the three cached
// releases and asserts the convention that PickAssets' substring patterns
// depend on: a build tag in the name (except the Windows CUDA runtime
// archives), an OS part from a fixed set, a trailing architecture, and an
// archive extension per OS. A rename that breaks this fails here first.
func TestAssetNameConventions(t *testing.T) {
	oses := map[string]bool{"android": true, "linux": true, "macos": true, "ubuntu": true, "win": true}
	archs := map[string]bool{"x64": true, "arm64": true, "s390x": true}
	sidecars := map[string]bool{"ui.tar.gz": true, "xcframework.zip": true}
	for _, tag := range otherTags {
		r := loadRelease(t, tag)
		for _, a := range r.Assets {
			if sidecars[strings.TrimPrefix(a.Name, "llama-"+tag+"-")] {
				continue // llama-<tag>-ui.tar.gz and llama-<tag>-xcframework.zip
			}
			m := assetName.FindStringSubmatch(a.Name)
			if m == nil {
				t.Errorf("%s: %s does not follow the asset naming convention", tag, a.Name)
				continue
			}
			prefix, nameTag, osPart, rest, ext := m[1], m[2], m[3], m[4], m[5]
			if !oses[osPart] {
				t.Errorf("%s: unknown OS part %q in %s", tag, osPart, a.Name)
			}
			if nameTag == "" {
				if prefix != "cudart-llama" || osPart != "win" {
					t.Errorf("%s: %s has no build tag; only Windows cudart archives may omit it", tag, a.Name)
				}
			} else if nameTag != tag {
				t.Errorf("%s: asset %s names tag %s", tag, a.Name, nameTag)
			}
			if want := map[bool]string{true: ".zip", false: ".tar.gz"}[osPart == "win"]; ext != want {
				t.Errorf("%s: %s has extension %s, want %s", tag, a.Name, ext, want)
			}
			// The architecture is the last dash-separated token, except for
			// the Snapdragon builds, which add a trailing qualifier.
			tokens := strings.Split(rest, "-")
			arch := tokens[len(tokens)-1]
			if !archs[arch] && arch == "snapdragon" && len(tokens) > 1 {
				arch = tokens[len(tokens)-2]
			}
			if !archs[arch] {
				t.Errorf("%s: %s does not name a known architecture", tag, a.Name)
			}
		}
	}
}

// TestRenamedS390xAssets records the one naming change in the range: the
// plain s390x build gained Ubuntu-release-qualified siblings. Nothing was
// removed, so the x64 and arm64 names PickAssets matches are unchanged.
func TestRenamedS390xAssets(t *testing.T) {
	want := map[string][]string{
		"b11960": {"llama-b11960-bin-ubuntu-s390x.tar.gz"},
		"b12110": {"llama-b12110-bin-ubuntu-26.04-s390x.tar.gz", "llama-b12110-bin-ubuntu-s390x.tar.gz", "llama-b12110-bin-ubuntu-zdnn-s390x.tar.gz"},
		"b12144": {"llama-b12144-bin-ubuntu-26.04-s390x.tar.gz", "llama-b12144-bin-ubuntu-s390x.tar.gz", "llama-b12144-bin-ubuntu-zdnn-s390x.tar.gz"},
	}
	for _, tag := range otherTags {
		r := loadRelease(t, tag)
		var got []string
		for _, a := range r.Assets {
			if strings.Contains(a.Name, "-s390x") {
				got = append(got, a.Name)
			}
		}
		if !equalStrings(got, want[tag]) {
			t.Errorf("%s: s390x assets = %v, want %v", tag, got, want[tag])
		}
	}
}

// TestUbuntuReleaseQualifierIsUnsupported: if llama.cpp ever qualifies the
// x64 or arm64 Linux names the way it qualified s390x, "-bin-ubuntu-x64"
// stops matching and Linux installs fail. This test pins that behaviour so
// the upgrade plan can watch for it.
func TestUbuntuReleaseQualifierIsUnsupported(t *testing.T) {
	r := &Release{Tag: "b99999", Assets: []Asset{
		{Name: "llama-b99999-bin-ubuntu-26.04-x64.tar.gz"},
		{Name: "llama-b99999-bin-macos-x64.tar.gz"},
	}}
	if got, err := PickAssets(r, "linux", "amd64", scan.CPU, false); err == nil {
		t.Fatalf("got %v, want an error until the pattern learns the qualifier", assetNames_other(got))
	}
}

// TestChecksumFileNaming: llama.cpp publishes no checksum file. The only
// checksum is GitHub's per-asset "digest" field, which engine.Asset.Digest
// reads; a missing digest is accepted on size alone (verifyDigest).
func TestChecksumFileNaming(t *testing.T) {
	sidecar := regexp.MustCompile(`(?i)(sha256sums|checksums?\.(txt|json)|\.sha256|\.sha512|\.md5|\.sig|\.asc|\.minisig)$`)
	digest := regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	for _, tag := range otherTags {
		r := loadRelease(t, tag)
		for _, a := range r.Assets {
			if sidecar.MatchString(a.Name) {
				t.Errorf("%s: unexpected checksum file asset %s", tag, a.Name)
			}
			if !digest.MatchString(a.Digest) {
				t.Errorf("%s: asset %s has no sha256 digest (%q)", tag, a.Name, a.Digest)
			}
		}
	}
}

// TestHasBinaries: a stable release may carry only sidecars and the
// nightly-tag.txt pointer, which LatestRelease follows. Windows CUDA runtime
// archives contain "-bin-" too, so the prefix check matters.
func TestHasBinaries(t *testing.T) {
	cases := []struct {
		name   string
		assets []Asset
		want   bool
	}{
		{"empty", nil, false},
		{"nightly pointer only", []Asset{{Name: "nightly-tag.txt"}}, false},
		{"sidecars only", []Asset{{Name: "llama-b12110-ui.tar.gz"}, {Name: "llama-b12110-xcframework.zip"}}, false},
		{"cudart only", []Asset{{Name: "cudart-llama-bin-win-cuda-13.4-x64.zip"}}, false},
		{"server archive", []Asset{{Name: "llama-b12110-bin-ubuntu-x64.tar.gz"}}, true},
		{"stable payload", []Asset{{Name: "nightly-tag.txt"}, {Name: "llama-b12110-bin-win-cpu-x64.zip"}}, true},
	}
	for _, c := range cases {
		if got := hasBinaries(&Release{Tag: "v0.9.0", Assets: c.assets}); got != c.want {
			t.Errorf("%s: hasBinaries = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestFlavorMacX64: the install folder is per architecture, so an Intel Mac
// and an Apple Silicon Mac do not reuse each other's server.
func TestFlavorMacX64(t *testing.T) {
	cases := []struct{ asset, tag, want string }{
		{"llama-b12110-bin-macos-x64.tar.gz", "b12110", "macos-x64"},
		{"llama-b12110-bin-macos-arm64.tar.gz", "b12110", "macos-arm64"},
		{"llama-b12110-bin-win-sycl-x64.zip", "b12110", "win-sycl-x64"},
	}
	for _, c := range cases {
		if got := flavor(c.asset, c.tag); got != c.want {
			t.Errorf("flavor(%s) = %q, want %q", c.asset, got, c.want)
		}
	}
}
