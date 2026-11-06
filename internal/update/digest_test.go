package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// assetServer stands in for the GitHub release asset. It serves body to any
// GET, which is all Download needs for a fresh temp directory.
func assetServer(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sha256Of(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// digestCase runs one Download against a fake asset server and reports whether
// the file was installed and where it landed.
func digestCase(t *testing.T, body []byte, digest string) (installed bool, dst string, err error) {
	t.Helper()
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("EZLM_HOME", t.TempDir())
	srv := assetServer(t, body)
	r := &Release{
		Version: "0.1.2",
		Asset:   "EZLM-0.1.2.dmg",
		URL:     srv.URL + "/EZLM-0.1.2.dmg",
		Size:    uint64(len(body)),
		Digest:  digest,
	}
	dst, err = Download(context.Background(), r, nil)
	if err != nil {
		return false, dst, err
	}
	if _, statErr := os.Stat(dst); statErr != nil {
		return false, dst, statErr
	}
	return true, dst, nil
}

func TestDownloadAcceptsMatchingDigest(t *testing.T) {
	body := make([]byte, 1024)
	copy(body[512:], "koly")
	ok, dst, err := digestCase(t, body, sha256Of(body))
	if err != nil {
		t.Fatalf("Download with a matching digest: %v", err)
	}
	if !ok {
		t.Fatal("Download with a matching digest did not install the file")
	}
	if got, err := os.ReadFile(dst); err != nil || string(got) != string(body) {
		t.Fatalf("installed file = %q, %v; want %q", got, err, body)
	}
}

func TestDownloadRefusesDigestMismatch(t *testing.T) {
	body := make([]byte, 1024)
	copy(body[512:], "koly")
	wrong := sha256Of([]byte("a different installer"))
	ok, _, err := digestCase(t, body, wrong)
	if err == nil {
		t.Fatal("Download installed an asset whose digest does not match")
	}
	if ok {
		t.Fatal("Download reported success for a digest mismatch")
	}
}

func TestDownloadRefusesMissingDigest(t *testing.T) {
	body := make([]byte, 1024)
	copy(body[512:], "koly")
	for _, digest := range []string{"", "   "} {
		ok, _, err := digestCase(t, body, digest)
		if err == nil {
			t.Errorf("Download installed an asset with digest %q; want a refusal", digest)
		}
		if ok {
			t.Errorf("Download reported success with digest %q", digest)
		}
	}
}

func TestDownloadRefusesWrongAlgorithm(t *testing.T) {
	body := make([]byte, 1024)
	copy(body[512:], "koly")
	sum512 := sha256.Sum256(body)
	for _, digest := range []string{
		"sha512:" + hex.EncodeToString(sum512[:]) + strings.Repeat("ab", 32),
		"md5:d41d8cd98f00b204e9800998ecf8427e",
		"sha1:da39a3ee5e6b4b0d3255bfef95601890afd80709",
		"sha256", // no colon at all
	} {
		ok, _, err := digestCase(t, body, digest)
		if err == nil {
			t.Errorf("Download installed an asset with digest %q; want a refusal", digest)
		}
		if ok {
			t.Errorf("Download reported success with digest %q", digest)
		}
	}
}

func TestDownloadRefusesWrongLengthDigest(t *testing.T) {
	body := make([]byte, 1024)
	copy(body[512:], "koly")
	good := strings.TrimPrefix(sha256Of(body), "sha256:")
	for _, digest := range []string{
		"sha256:",
		"sha256:abc",
		"sha256:" + good[:63],
		"sha256:" + good + "0",
		"sha256:" + strings.Repeat("z", 64), // right length, not hex
	} {
		ok, _, err := digestCase(t, body, digest)
		if err == nil {
			t.Errorf("Download installed an asset with digest %q; want a refusal", digest)
		}
		if ok {
			t.Errorf("Download reported success with digest %q", digest)
		}
	}
}

// A refused digest must not leave the file where the app can run it.
func TestDownloadLeavesNoFileOnDigestMismatch(t *testing.T) {
	body := make([]byte, 1024)
	copy(body[512:], "koly")
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	srv := assetServer(t, body)
	r := &Release{
		Version: "0.1.2",
		Asset:   "EZLM-0.1.2.dmg",
		URL:     srv.URL + "/EZLM-0.1.2.dmg",
		Size:    uint64(len(body)),
		Digest:  sha256Of([]byte("tampered")),
	}
	if _, err := Download(context.Background(), r, nil); err == nil {
		t.Fatal("Download accepted a tampered asset")
	}
	dst := filepath.Join(tmp, "update", r.Asset)
	if _, err := os.Stat(dst); err == nil {
		t.Fatalf("a rejected download is still at %s", dst)
	}
}
