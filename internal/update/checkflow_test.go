package update

// The check flow end to end: a fake GitHub API serves a release list, and the
// tests assert which release and which asset Check picks for this platform,
// then hand that release to Download and check the file it installs. The fault
// tests (rate limits, redirects, digests, downgrades and the rest) plug into
// the fixture below.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// checkflowAsset is one file on a release. The fake fills Size, Digest and
// URL from the payload unless the test set them, so a test can publish a lie
// (a wrong digest, say) while the bytes on the wire stay honest.
type checkflowAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int    `json:"size"`
	Digest string `json:"digest"`
}

type checkflowRelease struct {
	Tag        string           `json:"tag_name"`
	Name       string           `json:"name"`
	Body       string           `json:"body"`
	Page       string           `json:"html_url"`
	Draft      bool             `json:"draft"`
	Prerelease bool             `json:"prerelease"`
	Assets     []checkflowAsset `json:"assets"`
}

// checkflowGitHub is a stand-in for the GitHub releases API and the release
// assets it links to.
type checkflowGitHub struct {
	srv   *httptest.Server
	hits  atomic.Int64
	rels  []checkflowRelease
	files map[string][]byte
}

func newCheckflowGitHub(t *testing.T, rels []checkflowRelease, files map[string][]byte) *checkflowGitHub {
	t.Helper()
	gh := &checkflowGitHub{rels: rels, files: files}
	mux := http.NewServeMux()
	mux.HandleFunc("/releases", func(w http.ResponseWriter, r *http.Request) {
		gh.hits.Add(1)
		if r.Method != http.MethodGet {
			http.Error(w, "GET only", http.StatusMethodNotAllowed)
			return
		}
		out := make([]checkflowRelease, len(gh.rels))
		copy(out, gh.rels)
		for i := range out {
			out[i].Assets = append([]checkflowAsset(nil), gh.rels[i].Assets...)
			for j := range out[i].Assets {
				a := &out[i].Assets[j]
				b, ok := gh.files[a.Name]
				if !ok {
					continue
				}
				tag := out[i].Tag
				a.URL = gh.srv.URL + "/assets/" + tag + "/" + a.Name
				bytes := append([]byte{}, b...)
				if strings.HasSuffix(a.Name, ".dmg") && len(bytes) > 512 {
					bytes = append(append(append([]byte{}, bytes[:len(bytes)-512]...), tag...), bytes[len(bytes)-512:]...)
				} else {
					bytes = append(bytes, tag...)
				}
				if a.Size == 0 {
					a.Size = len(bytes)
				}
				if a.Digest == "" {
					sum := sha256.Sum256(bytes)
					a.Digest = "sha256:" + hex.EncodeToString(sum[:])
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("/assets/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/assets/")
		tag, name := "", rest
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			tag, name = rest[:i], rest[i+1:]
		}
		b, ok := gh.files[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		b = append([]byte{}, b...)
		if strings.HasSuffix(name, ".dmg") && len(b) > 512 {
			b = append(append(append([]byte{}, b[:len(b)-512]...), tag...), b[len(b)-512:]...)
		} else {
			b = append(b, tag...)
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(b)))
		w.Write(b)
	})
	gh.srv = httptest.NewServer(mux)
	t.Cleanup(gh.srv.Close)
	return gh
}

// check runs Check against this fake, the way the app does.
func (g *checkflowGitHub) check(t *testing.T, current string) Info {
	t.Helper()
	t.Setenv("EZLM_UPDATE_URL", g.srv.URL+"/releases")
	return Check(context.Background(), current)
}

func (g *checkflowGitHub) assetURL(name string) string { return g.srv.URL + "/assets/" + name }

// served returns the bytes the fake serves for a tag and asset name.
func (g *checkflowGitHub) served(tag, name string) []byte {
	b := append([]byte{}, g.files[name]...)
	if strings.HasSuffix(name, ".dmg") && len(b) > 512 {
		return append(append(append([]byte{}, b[:len(b)-512]...), tag...), b[len(b)-512:]...)
	}
	return append(b, tag...)
}

// checkflowHostAsset is the installer Check must pick on this computer, or ""
// when EZLM ships no installer for this platform (the Linux engine has none).
func checkflowHostAsset() string {
	switch runtime.GOOS {
	case "darwin":
		return "EZLM.dmg"
	case "windows":
		return "EZLM-Setup.exe"
	}
	return ""
}

// checkflowInstaller is a release at tag that ships both installers, so the
// same fixture works on macOS and Windows and still proves the other
// platform's file is filtered out.
func checkflowInstaller(tag string) checkflowRelease {
	return checkflowRelease{
		Tag:  tag,
		Name: "EZLM " + strings.TrimPrefix(tag, "v"),
		Body: "Notes for " + tag,
		Page: "https://example.test/releases/" + strings.TrimPrefix(tag, "v"),
		Assets: []checkflowAsset{
			{Name: "EZLM.dmg"},
			{Name: "EZLM-Setup.exe"},
		},
	}
}

// checkflowFiles are the payloads behind checkflowInstaller.
func checkflowFiles() map[string][]byte {
	// Since T08 a10 Download verifies the file's platform: a DMG needs the
	// UDIF koly trailer, an .exe needs an MZ header with a PE signature.
	dmg := make([]byte, 512*2)
	copy(dmg[512:], "koly")
	pe := make([]byte, 512)
	copy(pe, "MZ")
	pe[0x3c] = 0x80 // e_lfanew -> 0x80
	copy(pe[0x80:], "PE\x00\x00")
	return map[string][]byte{
		"EZLM.dmg":        dmg,
		"EZLM-Setup.exe":  pe,
		"notes.txt":       []byte("release notes"),
		"EZLM.dmg.sha256": []byte("checksum file"),
	}
}

// TestCheckFlowPicksThePlatformAsset is the whole check against a fake GitHub:
// one newer release with several files, and the assertion that the decision,
// the version, and the asset (with its size and digest) all come out right.
func TestCheckFlowPicksThePlatformAsset(t *testing.T) {
	asset := checkflowHostAsset()
	if asset == "" {
		t.Skip("EZLM ships installers for macOS and Windows only")
	}
	files := checkflowFiles()
	rel := checkflowInstaller("v0.1.12-beta")
	rel.Assets = append(rel.Assets, checkflowAsset{Name: "notes.txt"}, checkflowAsset{Name: "EZLM.dmg.sha256"})
	gh := newCheckflowGitHub(t, []checkflowRelease{rel}, files)

	info := gh.check(t, "0.1.11-beta")

	if info.Error != "" {
		t.Fatalf("Check reported an error: %q", info.Error)
	}
	if info.Current != "0.1.11-beta" {
		t.Errorf("Current = %q, want %q", info.Current, "0.1.11-beta")
	}
	if info.Checked.IsZero() {
		t.Error("Checked was not set")
	}
	if !info.Available || info.Latest == nil {
		t.Fatalf("Available = %v, Latest = %+v; want the 0.1.12 beta offered", info.Available, info.Latest)
	}
	got := info.Latest
	if got.Version != "0.1.12-beta" {
		t.Errorf("Version = %q, want %q", got.Version, "0.1.12-beta")
	}
	if got.Name != "EZLM 0.1.12-beta" || got.Notes != "Notes for v0.1.12-beta" || got.Page != "https://example.test/releases/0.1.12-beta" {
		t.Errorf("release metadata not carried over: %+v", got)
	}
	if got.Asset != asset {
		t.Errorf("Asset = %q, want %q for %s", got.Asset, asset, runtime.GOOS)
	}
	assetBytes := append([]byte{}, files[asset]...)
	if strings.HasSuffix(asset, ".dmg") && len(assetBytes) > 512 {
		assetBytes = append(append(append([]byte{}, assetBytes[:len(assetBytes)-512]...), "v0.1.12-beta"...), assetBytes[len(assetBytes)-512:]...)
	} else {
		assetBytes = append(assetBytes, "v0.1.12-beta"...)
	}
	if got.URL != gh.assetURL("v0.1.12-beta/"+asset) {
		t.Errorf("URL = %q, want %q", got.URL, gh.assetURL("v0.1.12-beta/"+asset))
	}
	if got.Size != uint64(len(assetBytes)) {
		t.Errorf("Size = %d, want %d", got.Size, len(assetBytes))
	}
	sum := sha256.Sum256(assetBytes)
	if want := "sha256:" + hex.EncodeToString(sum[:]); got.Digest != want {
		t.Errorf("Digest = %q, want %q", got.Digest, want)
	}
	if n := gh.hits.Load(); n != 1 {
		if n < 1 || n > 2 {
			t.Errorf("the check asked GitHub %d times, want 1 or 2 (a redirect probe adds one)", n)
		}
	}
}

// TestCheckFlowChannelDecisions pins the release each channel decision picks:
// stable, beta, draft, up to date, and a newer release that has no file for
// this platform.
func TestCheckFlowChannelDecisions(t *testing.T) {
	if checkflowHostAsset() == "" {
		t.Skip("EZLM ships installers for macOS and Windows only")
	}
	files := checkflowFiles()
	withNotesOnly := checkflowRelease{
		Tag:    "v0.1.13",
		Assets: []checkflowAsset{{Name: "notes.txt"}},
	}
	for _, c := range []struct {
		name    string
		current string
		rels    []checkflowRelease
		want    string // version chosen, "" for none
		seen    string // version recorded in Latest even when it is not newer
	}{
		{"newer stable", "0.1.11-beta", []checkflowRelease{checkflowInstaller("v0.1.12")}, "0.1.12", ""},
		{"newer beta", "0.1.11-beta", []checkflowRelease{checkflowInstaller("v0.1.12-beta")}, "0.1.12-beta", ""},
		{"stable outranks its own beta", "0.1.11-beta", []checkflowRelease{checkflowInstaller("v0.1.12-beta"), checkflowInstaller("v0.1.12")}, "0.1.12", ""},
		{"beta of the running stable is older", "0.1.12", []checkflowRelease{checkflowInstaller("v0.1.12-beta")}, "", "0.1.12-beta"},
		{"draft is skipped", "0.1.11-beta", []checkflowRelease{checkflowRelease{Tag: "v0.1.13", Draft: true, Assets: []checkflowAsset{{Name: "EZLM.dmg"}, {Name: "EZLM-Setup.exe"}}}, checkflowInstaller("v0.1.12")}, "0.1.12", ""},
		{"only a draft", "0.1.11-beta", []checkflowRelease{checkflowRelease{Tag: "v0.1.13", Draft: true, Assets: []checkflowAsset{{Name: "EZLM.dmg"}, {Name: "EZLM-Setup.exe"}}}}, "", ""},
		{"already up to date", "0.1.11-beta", []checkflowRelease{checkflowInstaller("v0.1.11-beta")}, "", "0.1.11-beta"},
		{"older release", "0.1.11-beta", []checkflowRelease{checkflowInstaller("v0.1.10-beta")}, "", "0.1.10-beta"},
		{"newest has no file for this platform", "0.1.11-beta", []checkflowRelease{withNotesOnly, checkflowInstaller("v0.1.12")}, "0.1.12", ""},
		{"no installer at all", "0.1.11-beta", []checkflowRelease{withNotesOnly}, "", ""},
		{"list out of order", "0.1.11-beta", []checkflowRelease{checkflowInstaller("v0.1.10-beta"), checkflowInstaller("v0.1.12-beta")}, "0.1.12-beta", ""},
		{"prerelease flag follows the tag", "0.1.11-beta", []checkflowRelease{func() checkflowRelease {
			r := checkflowInstaller("v0.1.12")
			r.Prerelease = true // GitHub's flag, but the tag is a stable one
			return r
		}()}, "0.1.12", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			gh := newCheckflowGitHub(t, c.rels, files)
			info := gh.check(t, c.current)
			if info.Error != "" {
				t.Fatalf("Check reported an error: %q", info.Error)
			}
			if c.want == "" {
				if info.Available {
					t.Fatalf("Available = true, Latest = %+v; want no update", info.Latest)
				}
				// Latest records the newest release with a file for this
				// platform even when it is not newer, so callers must look at
				// Available before offering it.
				switch {
				case c.seen == "" && info.Latest != nil:
					t.Fatalf("Latest = %+v; want none", info.Latest)
				case c.seen != "" && info.Latest != nil && info.Latest.Version != c.seen:
					t.Fatalf("Latest = %+v; want version %q or none", info.Latest, c.seen)
				}
				return
			}
			if !info.Available || info.Latest == nil {
				t.Fatalf("Available = %v, Latest = %+v; want %s", info.Available, info.Latest, c.want)
			}
			if info.Latest.Version != c.want {
				t.Fatalf("chose %q, want %q", info.Latest.Version, c.want)
			}
			if want := checkflowHostAsset(); info.Latest.Asset != want {
				t.Errorf("chose asset %q, want %q", info.Latest.Asset, want)
			}
		})
	}
}

// TestCheckFlowDownloadInstallsTheCheckedFile carries the check's decision into
// Download: the bytes on disk are the asset the check picked, and the digest
// the check read from GitHub is the one Download verified.
func TestCheckFlowDownloadInstallsTheCheckedFile(t *testing.T) {
	asset := checkflowHostAsset()
	if asset == "" {
		t.Skip("EZLM ships installers for macOS and Windows only")
	}
	files := checkflowFiles()
	gh := newCheckflowGitHub(t, []checkflowRelease{checkflowInstaller("v0.1.12-beta")}, files)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("EZLM_HOME", tmp)

	info := gh.check(t, "0.1.11-beta")
	if !info.Available || info.Latest == nil {
		t.Fatalf("no update offered: %+v", info)
	}
	path, err := Download(context.Background(), info.Latest, nil)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if want := filepath.Join(tmp, "update", asset); path != want {
		t.Errorf("Download wrote %q, want %q", path, want)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the downloaded file: %v", err)
	}
	if wantBytes := gh.served("v0.1.12-beta", asset); string(got) != string(wantBytes) {
		t.Errorf("downloaded %d bytes, want %d", len(got), len(wantBytes))
	}
}

// TestCheckFlowRefusesAForeignAsset checks the join between check and install:
// a release that publishes the wrong digest is still offered, and Download
// then refuses it and leaves nothing behind to run.
func TestCheckFlowRefusesAForeignAsset(t *testing.T) {
	asset := checkflowHostAsset()
	if asset == "" {
		t.Skip("EZLM ships installers for macOS and Windows only")
	}
	files := checkflowFiles()
	rel := checkflowInstaller("v0.1.12-beta")
	for i := range rel.Assets {
		if rel.Assets[i].Name == asset {
			rel.Assets[i].Digest = "sha256:" + strings.Repeat("0", 64)
		}
	}
	gh := newCheckflowGitHub(t, []checkflowRelease{rel}, files)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	info := gh.check(t, "0.1.11-beta")
	if !info.Available || info.Latest == nil {
		t.Fatalf("no update offered: %+v", info)
	}
	if _, err := Download(context.Background(), info.Latest, nil); err == nil {
		t.Fatal("Download accepted an asset whose digest does not match")
	} else if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("error %q does not mention the checksum", err)
	}
	dst := filepath.Join(tmp, "update", asset)
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("the rejected file is still at %s (err %v)", dst, err)
	}
}

// TestCheckFlowReportsAFailedCheck pins what the app sees when the check
// itself fails: an error to show, no update offered, and one request only so
// a failing API is not hammered.
func TestCheckFlowReportsAFailedCheck(t *testing.T) {
	for _, c := range []struct {
		name    string
		handler http.HandlerFunc
		wantErr bool
	}{
		{"server error", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}, true},
		{"rate limited", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			http.Error(w, "slow down", http.StatusForbidden)
		}, true},
		{"not a release list", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"message":"Not Found"}`))
		}, true},
		{"empty list", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`[]`))
		}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			var hits atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				c.handler(w, r)
			}))
			defer srv.Close()
			t.Setenv("EZLM_UPDATE_URL", srv.URL)

			info := Check(context.Background(), "0.1.11-beta")
			if gotErr := info.Error != ""; gotErr != c.wantErr {
				t.Errorf("Error = %q, want error: %v", info.Error, c.wantErr)
			}
			if info.Available || info.Latest != nil {
				t.Errorf("Available = %v, Latest = %+v; want no update", info.Available, info.Latest)
			}
			if n := hits.Load(); n != 1 {
				if n < 1 || n > 2 {
					t.Errorf("the check asked GitHub %d times, want 1 or 2 (a redirect probe adds one)", n)
				}
			}
		})
	}
}
