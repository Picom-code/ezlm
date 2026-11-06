package update

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

// Channel policy, as documented on ReleasesURL and Newer: EZLM ships betas, so
// a release marked prerelease is a candidate (Check reads /releases, which
// includes them). Drafts are never candidates, not even when they are also
// marked prerelease, and a final release beats a pre-release of the same
// version.

type a06Asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   uint64 `json:"size"`
	Digest string `json:"digest"`
}

type a06Release struct {
	Tag        string     `json:"tag_name"`
	Name       string     `json:"name"`
	Body       string     `json:"body"`
	Page       string     `json:"html_url"`
	Draft      bool       `json:"draft"`
	Prerelease bool       `json:"prerelease"`
	Assets     []a06Asset `json:"assets"`
}

// a06Installer is the asset name Check should pick on this machine.
func a06Installer(t *testing.T) string {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		return "EZLM-Setup.exe"
	case "darwin":
		return "EZLM.dmg"
	}
	t.Skipf("update.Check picks no asset on %s", runtime.GOOS)
	return ""
}

func a06Rel(tag string, draft, prerelease bool) a06Release {
	return a06Release{Tag: tag, Name: tag, Draft: draft, Prerelease: prerelease}
}

// a06Serve stands in for the GitHub releases API and returns the check result
// for current. No request leaves the process.
func a06Serve(t *testing.T, current string, rels ...a06Release) Info {
	t.Helper()
	name := a06Installer(t)
	for i := range rels {
		rels[i].Assets = []a06Asset{{
			Name:   name,
			URL:    "https://example.invalid/" + rels[i].Tag + "/" + name,
			Size:   1 << 20,
			Digest: fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(rels[i].Tag))),
		}}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(rels); err != nil {
			t.Errorf("encode releases: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("EZLM_UPDATE_URL", srv.URL)
	return Check(t.Context(), current)
}

func TestChannelOffersPrerelease(t *testing.T) {
	info := a06Serve(t, "0.1.11-beta", a06Rel("v0.1.12-beta", false, true))
	if !info.Available || info.Latest == nil || info.Latest.Version != "0.1.12-beta" {
		t.Fatalf("prerelease not offered: %+v", info)
	}
}

func TestChannelSkipsDraft(t *testing.T) {
	info := a06Serve(t, "0.1.11-beta", a06Rel("v0.1.12", true, false))
	if info.Available || info.Latest != nil {
		t.Fatalf("draft offered: %+v", info)
	}
}

func TestChannelSkipsDraftPrerelease(t *testing.T) {
	info := a06Serve(t, "0.1.11-beta", a06Rel("v0.1.12-beta", true, true))
	if info.Available || info.Latest != nil {
		t.Fatalf("draft prerelease offered: %+v", info)
	}
}

func TestChannelDraftDoesNotHidePublished(t *testing.T) {
	info := a06Serve(t, "0.1.11-beta",
		a06Rel("v0.2.0", true, false),
		a06Rel("v0.1.12-beta", false, true))
	if info.Latest == nil || info.Latest.Version != "0.1.12-beta" {
		t.Fatalf("published prerelease hidden by draft: %+v", info.Latest)
	}
}

// A final release and its own pre-release can both be in the list. The final
// must win whichever order GitHub returns them in.
func TestChannelFinalBeatsPrerelease(t *testing.T) {
	for _, order := range []struct {
		name string
		rels []a06Release
	}{
		{"prerelease first", []a06Release{a06Rel("v0.1.12-beta", false, true), a06Rel("v0.1.12", false, false)}},
		{"final first", []a06Release{a06Rel("v0.1.12", false, false), a06Rel("v0.1.12-beta", false, true)}},
	} {
		t.Run(order.name, func(t *testing.T) {
			info := a06Serve(t, "0.1.11-beta", order.rels...)
			if info.Latest == nil || info.Latest.Version != "0.1.12" {
				t.Fatalf("final release of the same version lost: %+v", info.Latest)
			}
		})
	}
}

// Upgrading from a beta to the final release of the same version is allowed.
func TestChannelFinalOfCurrentVersionIsOffered(t *testing.T) {
	info := a06Serve(t, "0.1.11-beta", a06Rel("v0.1.11", false, false))
	if !info.Available || info.Latest == nil || info.Latest.Version != "0.1.11" {
		t.Fatalf("final release of the running beta not offered: %+v", info)
	}
}

// The channel flag must survive Check: the apps label an update from it, and a
// prerelease whose tag has no "-beta" suffix is otherwise indistinguishable
// from a final release.
func TestChannelKeepsPrereleaseFlag(t *testing.T) {
	info := a06Serve(t, "0.1.11-beta",
		a06Rel("v0.1.12", false, true))
	if info.Latest == nil {
		t.Fatal("prerelease-flagged release not offered")
	}
	if !info.Latest.Prerelease {
		t.Fatalf("prerelease flag lost for %s", info.Latest.Version)
	}

	final := a06Serve(t, "0.1.11-beta", a06Rel("v0.1.12", false, false))
	if final.Latest == nil || final.Latest.Prerelease {
		t.Fatalf("final release marked as a prerelease: %+v", final.Latest)
	}
}
