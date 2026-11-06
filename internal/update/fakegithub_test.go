package update

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGitHub stands in for GitHub's releases API and the release assets, so a
// test can make the updater see one thing wrong without a network. It starts
// with one good release (v0.1.2, both installers, a correct sha256 digest) and
// a test then changes a single knob:
//
//	fg := newFakeGitHub(t)
//	fg.Use()                                  // point Check at the fake
//	fg.Installer().Digest = "sha256:" + strings.Repeat("0", 64)
//	info := Check(context.Background(), "0.1.0")
//	Download(context.Background(), info.Latest, nil) // must fail
//
// The returned releases and assets are live: change them between requests.
// Tests that need a fault the knobs don't express (truncation, slow drip,
// redirects) serve the bytes themselves with fakeAsset.Serve or OnAsset.
type fakeGitHub struct {
	t   *testing.T
	URL string // base URL of the fake server

	mu      sync.Mutex
	rels    []*fakeRelease
	apiHook http.HandlerFunc
	hits    map[string]int
	srv     *httptest.Server
}

// apiPath is where the fake serves the release list.
const apiPath = "/repos/picom-code/ezlm/releases"

// newFakeGitHub starts a server with one good release, v0.1.2.
func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()
	fg := &fakeGitHub{t: t, hits: map[string]int{}}
	fg.srv = httptest.NewServer(http.HandlerFunc(fg.serve))
	t.Cleanup(fg.srv.Close)
	fg.URL = fg.srv.URL
	fg.Release("v0.1.2", []byte("EZLM 0.1.2 installer"))
	return fg
}

// Use points the updater at the fake and gives Download a private temp dir
// (Download writes under os.TempDir). It calls t.Setenv, so the test must not
// be parallel.
func (fg *fakeGitHub) Use() {
	fg.t.Helper()
	fg.t.Setenv("TMPDIR", fg.t.TempDir())
	fg.t.Setenv("EZLM_UPDATE_URL", fg.APIURL())
}

// APIURL is the URL Check fetches.
func (fg *fakeGitHub) APIURL() string { return fg.URL + apiPath }

// AssetURL is where the fake serves the named asset's bytes.
func (fg *fakeGitHub) AssetURL(name string) string { return fg.URL + "/download/" + name }

// Release adds a release with an installer for each platform, both carrying
// body, and returns it for tweaking.
func (fg *fakeGitHub) Release(tag string, body []byte) *fakeRelease {
	fg.t.Helper()
	v := strings.TrimPrefix(tag, "v")
	r := &fakeRelease{
		Tag:  tag,
		Name: "EZLM " + tag,
		Body: "release notes for " + tag,
		Page: "https://example.invalid/ezlm/releases/tag/" + tag,
		Assets: []*fakeAsset{
			{Name: "EZLM-Setup-" + v + ".exe", Body: fakePE(body)},
			{Name: "EZLM-" + v + ".dmg", Body: fakeDMG(body)},
		},
	}
	return fg.Add(r)
}

// Add appends a release as given.
func (fg *fakeGitHub) Add(r *fakeRelease) *fakeRelease {
	fg.mu.Lock()
	defer fg.mu.Unlock()
	fg.rels = append(fg.rels, r)
	return r
}

// Reset drops every release and every request count.
func (fg *fakeGitHub) Reset() {
	fg.mu.Lock()
	defer fg.mu.Unlock()
	fg.rels, fg.apiHook, fg.hits = nil, nil, map[string]int{}
}

// Asset returns the named asset of the most recent release that has one.
func (fg *fakeGitHub) Asset(name string) *fakeAsset {
	fg.t.Helper()
	if a := fg.asset(name); a != nil {
		return a
	}
	fg.t.Fatalf("fakeGitHub: no asset %q", name)
	return nil
}

// Installer returns the asset Check picks on this computer in the most recent
// release, so a test can break the file the updater will actually choose.
func (fg *fakeGitHub) Installer() *fakeAsset {
	fg.t.Helper()
	fg.mu.Lock()
	defer fg.mu.Unlock()
	for i := len(fg.rels) - 1; i >= 0; i-- {
		for _, a := range fg.rels[i].Assets {
			if assetFor(runtime.GOOS, a.Name) {
				return a
			}
		}
	}
	fg.t.Fatalf("fakeGitHub: no installer for %s", runtime.GOOS)
	return nil
}

// OnAPI replaces the release list response (rate limits, redirects, a body
// that is not JSON).
func (fg *fakeGitHub) OnAPI(h http.HandlerFunc) {
	fg.mu.Lock()
	defer fg.mu.Unlock()
	fg.apiHook = h
}

// OnAsset replaces how one asset's bytes are served (truncation, drip,
// redirects). The default serves the whole body and honours Range.
func (fg *fakeGitHub) OnAsset(name string, h http.HandlerFunc) { fg.Asset(name).Serve = h }

// APIHits is how many times the release list was fetched.
func (fg *fakeGitHub) APIHits() int { return fg.Hits(apiPath) }

// AssetHits is how many times the named asset's URL was fetched.
func (fg *fakeGitHub) AssetHits(name string) int { return fg.Hits("/download/" + name) }

// Hits is how many requests a path received.
func (fg *fakeGitHub) Hits(path string) int {
	fg.mu.Lock()
	defer fg.mu.Unlock()
	return fg.hits[path]
}

func (fg *fakeGitHub) asset(name string) *fakeAsset {
	fg.mu.Lock()
	defer fg.mu.Unlock()
	for i := len(fg.rels) - 1; i >= 0; i-- {
		for _, a := range fg.rels[i].Assets {
			if a.Name == name {
				return a
			}
		}
	}
	return nil
}

func (fg *fakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	fg.mu.Lock()
	fg.hits[r.URL.Path]++
	fg.mu.Unlock()
	switch {
	case r.URL.Path == apiPath:
		fg.serveAPI(w, r)
	case strings.HasPrefix(r.URL.Path, "/download/"):
		fg.serveAsset(w, r, strings.TrimPrefix(r.URL.Path, "/download/"))
	default:
		http.NotFound(w, r)
	}
}

// The JSON GitHub returns, as much of it as the updater reads.
type ghRelease struct {
	TagName    string    `json:"tag_name"`
	Name       string    `json:"name"`
	Body       string    `json:"body"`
	HTMLURL    string    `json:"html_url"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	Assets     []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string  `json:"name"`
	BrowserDownloadURL string  `json:"browser_download_url"`
	Size               uint64  `json:"size"`
	Digest             *string `json:"digest"` // null when GitHub has none
}

func (fg *fakeGitHub) serveAPI(w http.ResponseWriter, r *http.Request) {
	fg.mu.Lock()
	hook := fg.apiHook
	rels := make([]ghRelease, 0, len(fg.rels))
	for _, rel := range fg.rels {
		g := ghRelease{TagName: rel.Tag, Name: rel.Name, Body: rel.Body, HTMLURL: rel.Page, Draft: rel.Draft, Prerelease: rel.Prerelease}
		for _, a := range rel.Assets {
			u := a.URL
			if u == "" {
				u = fg.AssetURL(a.Name)
			}
			ga := ghAsset{Name: a.Name, BrowserDownloadURL: u, Size: a.size()}
			if d := a.digest(); d != "" {
				ga.Digest = &d
			}
			g.Assets = append(g.Assets, ga)
		}
		rels = append(rels, g)
	}
	fg.mu.Unlock()
	if hook != nil {
		hook(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(rels); err != nil {
		fg.t.Errorf("fakeGitHub: writing release list: %v", err)
	}
}

func (fg *fakeGitHub) serveAsset(w http.ResponseWriter, r *http.Request, name string) {
	a := fg.asset(name)
	if a == nil {
		http.NotFound(w, r)
		return
	}
	if a.Serve != nil {
		a.Serve(w, r)
		return
	}
	if a.Status != 0 && a.Status != http.StatusOK {
		w.WriteHeader(a.Status)
		return
	}
	http.ServeContent(w, r, a.Name, time.Time{}, bytes.NewReader(a.Body))
}

// fakeRelease is one entry in the release list. Change the fields between
// requests to model a bad release.
type fakeRelease struct {
	Tag        string
	Name       string
	Body       string
	Page       string
	Draft      bool
	Prerelease bool
	Assets     []*fakeAsset
}

// fakeAsset is one file in a release. The defaults are a correct file:
// sha256 of Body, size len(Body), served whole with Range support.
type fakeAsset struct {
	Name   string
	Body   []byte
	Size   uint64 // release-list size; 0 means len(Body)
	Digest string // release-list digest; "" means the sha256 of Body
	URL    string // release-list URL; "" means this fake's download path

	OmitDigest bool             // send "digest": null, as GitHub does when it has none
	Status     int              // serve this status instead of 200
	Serve      http.HandlerFunc // serve the bytes yourself (truncation, drip, redirects)
}

func (a *fakeAsset) size() uint64 {
	if a.Size != 0 {
		return a.Size
	}
	return uint64(len(a.Body))
}

func (a *fakeAsset) digest() string {
	switch {
	case a.OmitDigest:
		return ""
	case a.Digest != "":
		return a.Digest
	}
	sum := sha256.Sum256(a.Body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// TestFakeGitHub checks the fixture itself: the default release checks and
// downloads cleanly, and each knob changes what the updater sees.
func TestFakeGitHub(t *testing.T) {
	fg := newFakeGitHub(t)
	fg.Use()
	ctx := t.Context()

	inst := fg.Installer()
	info := Check(ctx, "0.1.0")
	if info.Error != "" {
		t.Fatalf("Check on the good release: %q", info.Error)
	}
	if !info.Available || info.Latest == nil {
		t.Fatalf("Check on the good release: available = %v", info.Available)
	}
	if info.Latest.Version != "0.1.2" || info.Latest.Asset != inst.Name {
		t.Fatalf("Check picked %q %q, want 0.1.2 %q", info.Latest.Version, info.Latest.Asset, inst.Name)
	}
	if info.Latest.Size != uint64(len(inst.Body)) || info.Latest.Digest != inst.digest() {
		t.Fatalf("release list size/digest = %d/%q, want %d/%q", info.Latest.Size, info.Latest.Digest, len(inst.Body), inst.digest())
	}

	// A wrong digest in the release list: the bytes download, then are refused.
	inst.Digest = "sha256:" + strings.Repeat("0", 64)
	bad := Check(ctx, "0.1.0")
	if bad.Latest == nil || bad.Latest.Digest != inst.Digest {
		t.Fatalf("Check did not carry the changed digest: %+v", bad.Latest)
	}
	if path, err := Download(ctx, bad.Latest, nil); err == nil {
		t.Fatalf("Download accepted a wrong digest, wrote %q", path)
	}

	inst.Digest = ""
	info = Check(ctx, "0.1.0")
	path, err := Download(ctx, info.Latest, nil)
	if err != nil {
		t.Fatalf("Download of the good asset: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, inst.Body) {
		t.Fatalf("downloaded %q (%v), want %q", got, err, inst.Body)
	}
	if n := fg.APIHits(); n < 3 || n > 6 {
		t.Errorf("release list fetched %d times, want 3 to 6 (a redirect probe may double it)", n)
	}
	if n := fg.AssetHits(inst.Name); n != 2 {
		t.Errorf("asset fetched %d times, want 2", n) // one refused, one good
	}

	// A release-list fault reaches the updater through OnAPI.
	fg.OnAPI(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	})
	if info := Check(ctx, "0.1.0"); info.Error == "" {
		t.Error("Check accepted a 403 from the release list")
	}
	if n := fg.APIHits(); n < 4 || n > 8 {
		t.Errorf("release list fetched %d times after the 403, want 4 to 8", n)
	}
}

// TestFakeGitHubAssetHooks checks the asset side: the default honours Range
// (the downloader resumes with it) and OnAsset replaces the bytes.
func TestFakeGitHubAssetHooks(t *testing.T) {
	fg := newFakeGitHub(t)
	inst := fg.Installer()

	req, _ := http.NewRequest("GET", fg.AssetURL(inst.Name), nil)
	req.Header.Set("Range", "bytes=2-")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusPartialContent || !bytes.Equal(body, inst.Body[2:]) {
		t.Fatalf("range request = %d %q, want 206 %q", resp.StatusCode, body, inst.Body[2:])
	}

	fg.OnAsset(inst.Name, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "999")
		w.Write([]byte("short"))
	})
	resp, err = http.Get(fg.AssetURL(inst.Name))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	if string(body) != "short" || resp.Header.Get("Content-Length") != "999" {
		t.Fatalf("custom asset = %q, Content-Length %q", body, resp.Header.Get("Content-Length"))
	}
}

// fakePE wraps b in a minimal MZ/PE header so the platform check accepts it.
func fakePE(b []byte) []byte {
	out := make([]byte, 512+len(b))
	copy(out, "MZ")
	out[0x3c] = 0x80
	copy(out[0x80:], "PE\x00\x00")
	copy(out[512:], b)
	return out
}

// fakeDMG wraps b in a minimal UDIF image: 512 bytes of body and a koly
// trailer, which is what the platform check looks for.
func fakeDMG(b []byte) []byte {
	out := append([]byte{}, b...)
	if len(out) < 512 {
		out = append(out, make([]byte, 512-len(out))...)
	}
	trailer := make([]byte, 512)
	copy(trailer, "koly")
	return append(out, trailer...)
}
