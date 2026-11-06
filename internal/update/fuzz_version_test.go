package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/picom-code/ezlm/internal/version"
)

// fuzzTags are the shapes a release tag has taken, or could take, on GitHub.
var fuzzTags = []string{
	"v0.1.11-beta", "0.1.11-beta", "v1.0.0", "1.2.3", "1.2.3.4", "v1", "v",
	"", " ", "v1.2.3-beta+build", "1.2.3+build.5", "1.2.3-beta.10", "1.2.3-beta.2",
	"1.10.0", "1.9.0", "V1.2.3", "β1.2.3", "1.2.3-", "1.2.3+", "01.2.3",
	strings.Repeat("9", 400), "1.2.3-01", "1.2.3-alpha.1", "1.2.3-Alpha",
}

func TestParseVersion(t *testing.T) {
	valid := map[string]Version{
		"v0.1.11-beta":      {Major: 0, Minor: 1, Patch: 11, Pre: []string{"beta"}},
		"1.2.3":             {Major: 1, Minor: 2, Patch: 3},
		"v1.2.3-beta+build": {Major: 1, Minor: 2, Patch: 3, Pre: []string{"beta"}},
		"1.2.3+build.5":     {Major: 1, Minor: 2, Patch: 3},
	}
	for tag, want := range valid {
		got, err := ParseVersion(tag)
		if err != nil {
			t.Errorf("ParseVersion(%q) error: %v", tag, err)
			continue
		}
		if got.Compare(want) != 0 || len(got.Pre) != len(want.Pre) {
			t.Errorf("ParseVersion(%q) = %+v, want %+v", tag, got, want)
		}
	}
	for _, tag := range []string{"v", "1.2.3.4", "", " ", "β1.2.3", "1.2.3-", "1.2.3+", "01.2.3", "1.2.3-01", "V1.2.3", strings.Repeat("9", 400)} {
		if v, err := ParseVersion(tag); err == nil {
			t.Errorf("ParseVersion(%q) = %+v, want an error", tag, v)
		}
	}
}

// FuzzVersion fuzzes tag parsing and comparison. It asserts that nothing
// panics, that ordering is total and transitive, that comparison agrees with
// ParseVersion, and that a malformed tag never counts as newer than anything.
func FuzzVersion(f *testing.F) {
	for _, a := range fuzzTags {
		for _, b := range fuzzTags {
			f.Add(a, b, "1.2.3")
		}
	}
	f.Fuzz(func(t *testing.T, a, b, c string) {
		va, ea := ParseVersion(a)
		vb, eb := ParseVersion(b)
		_, ec := ParseVersion(c)

		ab, ba := Newer(a, b), Newer(b, a)
		if ab && ba {
			t.Fatalf("Newer(%q, %q) and Newer(%q, %q) are both true", a, b, b, a)
		}
		if Newer(a, a) {
			t.Fatalf("Newer(%q, %q) is true: ordering is not irreflexive", a, a)
		}
		if ea != nil && (ab || ba) {
			t.Fatalf("malformed tag %q takes part in ordering: Newer(a,b)=%v Newer(b,a)=%v", a, ab, ba)
		}
		if ea == nil && eb == nil {
			if want := va.Compare(vb) > 0; ab != want {
				t.Fatalf("Newer(%q, %q) = %v, Compare says %d", a, b, ab, va.Compare(vb))
			}
		}
		if ea == nil && eb == nil && ec == nil {
			if ab && Newer(b, c) && !Newer(a, c) {
				t.Fatalf("ordering is not transitive: %q > %q > %q but not > %q", a, b, c, c)
			}
		}
	})
}

// fuzzReleases is a fake GitHub release list whose body the fuzz target swaps
// between iterations.
type fuzzReleases struct {
	mu   sync.Mutex
	body []byte
}

func (f *fuzzReleases) set(t *testing.T, rels []map[string]any) {
	t.Helper()
	b, err := json.Marshal(rels)
	if err != nil {
		t.Fatalf("marshal releases: %v", err)
	}
	f.mu.Lock()
	f.body = b
	f.mu.Unlock()
}

func (f *fuzzReleases) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	b := f.body
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

// FuzzCheckChannel fuzzes channel selection: with two releases and their draft
// flags, Check must pick the newest well-formed, non-draft release that has a
// file for this platform, and never one whose tag does not parse.
func FuzzCheckChannel(f *testing.F) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		f.Skip("the updater only picks assets on windows and darwin")
	}
	asset := "EZLM.dmg"
	if runtime.GOOS == "windows" {
		asset = "EZLM-Setup.exe"
	}
	for _, a := range fuzzTags {
		for _, b := range fuzzTags {
			f.Add(a, b, false, false)
			f.Add(a, b, true, false)
			f.Add(a, b, false, true)
		}
	}

	fake := &fuzzReleases{}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	oldURL := ReleasesURL
	ReleasesURL = srv.URL
	defer func() { ReleasesURL = oldURL }()
	oldEnv, hadEnv := os.LookupEnv("EZLM_UPDATE_URL")
	os.Unsetenv("EZLM_UPDATE_URL")
	defer func() {
		if hadEnv {
			os.Setenv("EZLM_UPDATE_URL", oldEnv)
		}
	}()

	f.Fuzz(func(t *testing.T, tagA, tagB string, draftA, draftB bool) {
		rels := []map[string]any{
			{"tag_name": tagA, "name": "A", "draft": draftA, "assets": []map[string]any{{"name": asset, "browser_download_url": "http://example.invalid/A", "size": 1}}},
			{"tag_name": tagB, "name": "B", "draft": draftB, "assets": []map[string]any{{"name": asset, "browser_download_url": "http://example.invalid/B", "size": 1}}},
		}
		fake.set(t, rels)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		info := Check(ctx, version.Version)
		if info.Error != "" {
			t.Fatalf("Check reported %q for %+v", info.Error, rels)
		}

		cur, err := ParseVersion(version.Version)
		if err != nil {
			t.Fatalf("the running version %q does not parse: %v", version.Version, err)
		}
		var best *Version
		for _, rel := range []struct {
			tag   string
			draft bool
		}{{tagA, draftA}, {tagB, draftB}} {
			if rel.draft {
				continue
			}
			v, err := ParseVersion(rel.tag)
			if err != nil {
				continue
			}
			if best == nil || v.Compare(*best) > 0 {
				vv := v
				best = &vv
			}
		}
		switch {
		case best == nil:
			if info.Latest != nil || info.Available {
				t.Fatalf("Check picked %+v, but neither release is well formed and not a draft", info.Latest)
			}
		default:
			if info.Latest == nil {
				// Since T08 a05 Check only offers a release that is newer
				// than the running version.
				if best.Compare(cur) <= 0 {
					return
				}
				t.Fatalf("Check picked nothing, want version %+v from %+v", *best, rels)
			}
			got, err := ParseVersion(info.Latest.Version)
			if err != nil {
				t.Fatalf("Check picked unparsable version %q", info.Latest.Version)
			}
			if got.Compare(*best) != 0 {
				t.Fatalf("Check picked %q, want the newest %+v", info.Latest.Version, *best)
			}
			if want := best.Compare(cur) > 0; info.Available != want {
				t.Fatalf("Available = %v for %q against %q, want %v", info.Available, info.Latest.Version, version.Version, want)
			}
		}
	})
}
