package engine

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
	"sync/atomic"
	"testing"
)

func shaOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// serveBytes serves data, honouring a simple byte range so a retry can resume.
func serveBytes(data []byte) (*httptest.Server, *atomic.Int32) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Write(data)
	}))
	return srv, &hits
}

// A server that sends a file of the right size but the wrong bytes must not be
// accepted: the checksum catches it, the bad file is deleted, and a retry
// starts from a clean copy.
func TestDownloadSHARejectsCorruptFile(t *testing.T) {
	fastRetries(t)
	good := []byte(strings.Repeat("ezlm-good-model-bytes-", 8))
	bad := []byte(strings.Repeat("ezlm-bad!-model-bytes-", 8)) // same size, other bytes
	if len(good) != len(bad) {
		t.Fatalf("test data must be the same size: %d vs %d", len(good), len(bad))
	}
	srv, hits := serveBytes(bad)
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "m.gguf")
	err := DownloadSHA(context.Background(), srv.URL+"/m.gguf", dst, uint64(len(good)), shaOf(good), nil)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("wrong SHA: got %v", err)
	}
	if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
		t.Fatalf("the corrupt file was kept at %s", dst)
	}
	if _, statErr := os.Stat(dst + ".part"); !os.IsNotExist(statErr) {
		t.Fatalf("the corrupt part file was kept at %s.part", dst)
	}
	// One download plus one retry, not an endless loop.
	if n := hits.Load(); n > 2 {
		t.Fatalf("kept retrying a bad checksum: %d requests", n)
	}
}

// A file left on disk at the right size but with the wrong contents is not
// trusted: Download alone returns it as-is, so the checksum path must re-fetch.
func TestDownloadSHARedownloadsCorruptFileOnDisk(t *testing.T) {
	fastRetries(t)
	good := []byte(strings.Repeat("ezlm-good-model-bytes-", 8))
	bad := []byte(strings.Repeat("ezlm-bad!-model-bytes-", 8))
	dir := t.TempDir()
	dst := filepath.Join(dir, "m.gguf")
	if err := os.WriteFile(dst, bad, 0o644); err != nil {
		t.Fatal(err)
	}
	srv, hits := serveBytes(good)
	defer srv.Close()

	if err := DownloadSHA(context.Background(), srv.URL+"/m.gguf", dst, uint64(len(good)), shaOf(good), nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(good) {
		t.Fatalf("the corrupt file on disk was not replaced")
	}
	if hits.Load() != 1 {
		t.Fatalf("expected one re-download, got %d requests", hits.Load())
	}
}

// A missing SHA is allowed (the server published none) and the size check
// still applies. A good SHA with GitHub's "sha256:" prefix is accepted.
func TestDownloadSHAMissingAndPrefixed(t *testing.T) {
	fastRetries(t)
	data := []byte(strings.Repeat("ezlm-model-bytes-", 16))
	srv, _ := serveBytes(data)
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "a.gguf")
	if err := DownloadSHA(context.Background(), srv.URL+"/a.gguf", dst, uint64(len(data)), "", nil); err != nil {
		t.Fatalf("missing SHA should still download: %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != string(data) {
		t.Fatal("missing SHA: file differs")
	}

	// Missing SHA does not turn off the size check.
	err := DownloadSHA(context.Background(), srv.URL+"/b.gguf", filepath.Join(t.TempDir(), "b.gguf"), uint64(len(data)+1), "", nil)
	if err == nil || !strings.Contains(err.Error(), "wrong size") {
		t.Fatalf("missing SHA with a wrong size: got %v", err)
	}

	dst = filepath.Join(t.TempDir(), "c.gguf")
	if err := DownloadSHA(context.Background(), srv.URL+"/c.gguf", dst, uint64(len(data)), "sha256:"+shaOf(data), nil); err != nil {
		t.Fatalf("prefixed SHA: %v", err)
	}
}

// A SHA of the wrong length (or with non-hex characters) is a metadata bug,
// not a corrupt download: it fails before a single byte is fetched, so a huge
// model is not downloaded just to fail the check at the end.
func TestDownloadSHAMalformed(t *testing.T) {
	fastRetries(t)
	data := []byte(strings.Repeat("ezlm-model-bytes-", 16))
	srv, hits := serveBytes(data)
	defer srv.Close()

	cases := map[string]string{
		"short":       "abc123",
		"sha1 length": strings.Repeat("a", 40),
		"non-hex":     strings.Repeat("z", 64),
		"too long":    strings.Repeat("a", 65),
	}
	for name, sha := range cases {
		t.Run(name, func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), "m.gguf")
			err := DownloadSHA(context.Background(), srv.URL+"/m.gguf", dst, uint64(len(data)), sha, nil)
			if err == nil || !strings.Contains(err.Error(), "checksum") {
				t.Fatalf("%s SHA: got %v", name, err)
			}
			if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
				t.Fatalf("%s SHA: a file was written anyway", name)
			}
		})
	}
	if hits.Load() != 0 {
		t.Fatalf("a malformed SHA still hit the network %d time(s)", hits.Load())
	}
}
