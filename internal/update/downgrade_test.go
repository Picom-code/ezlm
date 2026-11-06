package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

// releaseJSON is the part of GitHub's release JSON the updater reads.
type releaseJSON struct {
	Tag    string      `json:"tag_name"`
	Name   string      `json:"name"`
	Body   string      `json:"body"`
	Page   string      `json:"html_url"`
	Draft  bool        `json:"draft"`
	Assets []assetJSON `json:"assets"`
}

type assetJSON struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   uint64 `json:"size"`
	Digest string `json:"digest"`
}

// fakeReleases serves rels to Check through EZLM_UPDATE_URL.
func fakeReleases(t *testing.T, rels []releaseJSON) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(rels); err != nil {
			t.Errorf("write release list: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("EZLM_UPDATE_URL", srv.URL)
}

// platformAsset is the installer name assetFor accepts on this computer.
func platformAsset() string {
	switch runtime.GOOS {
	case "windows":
		return "EZLM-Setup.exe"
	case "darwin":
		return "EZLM.dmg"
	}
	return ""
}

// release is one release with this platform's installer, served from url.
func release(tag, url, digest string) releaseJSON {
	return releaseJSON{Tag: tag, Name: "EZLM " + tag, Assets: []assetJSON{{
		Name: platformAsset(), URL: url, Size: 1024, Digest: digest,
	}}}
}

// A release older than the running version must not come back as the
// "latest" release, even though Check still has to know it exists.
func TestDowngradeOlderReleaseNotOffered(t *testing.T) {
	if platformAsset() == "" {
		t.Skip("no installer for " + runtime.GOOS)
	}
	fakeReleases(t, []releaseJSON{
		release("v0.1.10-beta", "https://example.test/download/v0.1.10-beta/EZLM", "sha256:1010"),
		release("v0.1.9-beta", "https://example.test/download/v0.1.9-beta/EZLM", "sha256:0909"),
	})
	info := Check(context.Background(), "0.1.11-beta")
	if info.Available {
		t.Errorf("Check offered %s over the running 0.1.11-beta", info.Latest.Version)
	}
	if info.Latest != nil {
		t.Errorf("Check returned older release %s as Latest; a caller that downloads Latest would downgrade", info.Latest.Version)
	}
}

// 1.10 is newer than 1.9: numbers, not text. And 1.9 must not replace a
// running 1.10.
func TestDowngradeNumericVersionOrder(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"1.10.0", "1.9.0", true},
		{"1.9.0", "1.10.0", false},
		{"v1.10", "v1.9", true},
		{"1.10.0-beta", "1.9.0", true},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}

	if platformAsset() == "" {
		t.Skip("no installer for " + runtime.GOOS)
	}
	fakeReleases(t, []releaseJSON{
		release("v1.9.0", "https://example.test/download/v1.9.0/EZLM", "sha256:1900"),
		release("v1.10.0", "https://example.test/download/v1.10.0/EZLM", "sha256:1100"),
	})
	if info := Check(context.Background(), "1.9.0"); !info.Available || info.Latest == nil || info.Latest.Version != "1.10.0" {
		t.Fatalf("Check(1.9.0) = %+v, want the newer 1.10.0", info)
	}

	fakeReleases(t, []releaseJSON{
		release("v1.9.0", "https://example.test/download/v1.9.0/EZLM", "sha256:1900"),
	})
	if info := Check(context.Background(), "1.10.0"); info.Available || info.Latest != nil {
		t.Errorf("Check(1.10.0) offered the older 1.9.0: %+v", info)
	}
}

// A new release whose asset is the same file as an older release's asset is a
// replayed build: installing it replaces the running version with an old one.
func TestDowngradeReplayedOldAsset(t *testing.T) {
	if platformAsset() == "" {
		t.Skip("no installer for " + runtime.GOOS)
	}
	const oldURL = "https://example.test/download/v0.1.9-beta/EZLM"
	const oldDigest = "sha256:0909"
	for _, c := range []struct {
		name      string
		newURL    string
		newDigest string
	}{
		{"same asset URL", oldURL, "sha256:1212"},
		{"same asset digest", "https://example.test/download/v0.1.12-beta/EZLM", oldDigest},
	} {
		t.Run(c.name, func(t *testing.T) {
			fakeReleases(t, []releaseJSON{
				release("v0.1.12-beta", c.newURL, c.newDigest),
				release("v0.1.9-beta", oldURL, oldDigest),
			})
			info := Check(context.Background(), "0.1.11-beta")
			if info.Available || info.Latest != nil {
				t.Fatalf("Check offered a replayed old asset as %s: %+v", info.Latest.Version, info)
			}
			if info.Error == "" {
				t.Error("Check refused the replay silently; the user cannot tell why there is no update")
			}
		})
	}
}

// Distinct assets are still offered: the replay check must not block updates.
func TestDowngradeDistinctAssetOffered(t *testing.T) {
	if platformAsset() == "" {
		t.Skip("no installer for " + runtime.GOOS)
	}
	fakeReleases(t, []releaseJSON{
		release("v0.1.12-beta", "https://example.test/download/v0.1.12-beta/EZLM", "sha256:1212"),
		release("v0.1.9-beta", "https://example.test/download/v0.1.9-beta/EZLM", "sha256:0909"),
	})
	info := Check(context.Background(), "0.1.11-beta")
	if !info.Available || info.Latest == nil || info.Latest.Version != "0.1.12-beta" {
		t.Fatalf("Check = %+v, want 0.1.12-beta available", info)
	}
	if info.Error != "" {
		t.Errorf("Check reported an error for a normal release: %q", info.Error)
	}
}
