package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// Hugging Face answers a model URL with a 302 to a signed CDN URL. The CDN
// URL changes on every request, so a resume must go back through the
// original URL rather than reuse the first target.

func rangeStart(t *testing.T, r *http.Request) int {
	t.Helper()
	rg := r.Header.Get("Range")
	if rg == "" {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
	if err != nil {
		t.Fatalf("unparsable Range %q: %v", rg, err)
	}
	return n
}

func serveCDNRange(t *testing.T, w http.ResponseWriter, r *http.Request, data []byte) int {
	t.Helper()
	from := rangeStart(t, r)
	w.Header().Set("Accept-Ranges", "bytes")
	if from > 0 {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(data)-1, len(data)))
		w.Header().Set("Content-Length", strconv.Itoa(len(data)-from))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	}
	return from
}

// A plain 302 to another host downloads the whole file.
func TestDownloadFollowsRedirectToCDN(t *testing.T) {
	data := []byte(strings.Repeat("ezlm-redirect-bytes-", 512))
	var cdnHits atomic.Int32

	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cdnHits.Add(1)
		from := serveCDNRange(t, w, r, data)
		w.Write(data[from:])
	}))
	defer cdn.Close()

	var hfHits atomic.Int32
	hf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hfHits.Add(1)
		http.Redirect(w, r, cdn.URL+"/blob/model.gguf?sig=1", http.StatusFound)
	}))
	defer hf.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	var total uint64
	if err := Download(context.Background(), hf.URL+"/resolve/model.gguf", dst, uint64(len(data)), func(_, tot uint64, _ float64) {
		if tot > total {
			total = tot
		}
	}); err != nil {
		t.Fatalf("download through redirect: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(data) {
		t.Fatalf("file differs: %d bytes, want %d", len(got), len(data))
	}
	if hfHits.Load() != 1 || cdnHits.Load() != 1 {
		t.Fatalf("expected one request to each server, got hf=%d cdn=%d", hfHits.Load(), cdnHits.Load())
	}
	if total != uint64(len(data)) {
		t.Fatalf("progress total = %d, want %d", total, len(data))
	}
}

// The connection to the CDN drops partway. The retry goes back to the
// original URL, which hands out a different CDN target, and the resume must
// follow the new target with its Range intact.
func TestDownloadResumeFollowsChangedRedirect(t *testing.T) {
	data := []byte(strings.Repeat("ezlm-redirect-bytes-", 4096)) // ~82 KB
	cutAt := len(data) / 3

	var (
		mu       sync.Mutex
		hfHits   int
		hfRange  []string
		cdnSigs  []string
		cdnRange []string
		cut      atomic.Bool
	)
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		cdnSigs = append(cdnSigs, r.URL.Query().Get("sig"))
		cdnRange = append(cdnRange, r.Header.Get("Range"))
		mu.Unlock()

		from := serveCDNRange(t, w, r, data)
		if !cut.Swap(true) {
			w.Write(data[from:cutAt])
			w.(http.Flusher).Flush()
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		w.Write(data[from:])
	}))
	defer cdn.Close()

	hf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hfHits++
		n := hfHits
		hfRange = append(hfRange, r.Header.Get("Range"))
		mu.Unlock()
		// A fresh, expiring target on every request, like the real CDN.
		http.Redirect(w, r, fmt.Sprintf("%s/blob/model.gguf?sig=%d", cdn.URL, n), http.StatusFound)
	}))
	defer hf.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	if err := Download(context.Background(), hf.URL+"/resolve/model.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("resume through a changed redirect: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(data) {
		t.Fatalf("file differs after resume: %d bytes, want %d", len(got), len(data))
	}

	mu.Lock()
	defer mu.Unlock()
	if hfHits < 2 {
		t.Fatalf("expected a second request to the original URL, got %d", hfHits)
	}
	if len(cdnSigs) < 2 {
		t.Fatalf("expected a second CDN request, got %d", len(cdnSigs))
	}
	if cdnSigs[0] == cdnSigs[len(cdnSigs)-1] {
		t.Fatalf("redirect target did not change between requests: %v", cdnSigs)
	}
	if cdnRange[len(cdnRange)-1] != fmt.Sprintf("bytes=%d-", cutAt) {
		t.Fatalf("resume sent Range %q, want bytes=%d-", cdnRange[len(cdnRange)-1], cutAt)
	}
	if len(hfRange) < 2 || hfRange[1] != fmt.Sprintf("bytes=%d-", cutAt) {
		t.Fatalf("resume asked the original URL with Range %v, want bytes=%d-", hfRange, cutAt)
	}
}

// A part file left by an earlier run (app restart) resumes through the
// original URL, which now points at a different CDN target.
func TestDownloadResumeAcrossRestartFollowsNewRedirect(t *testing.T) {
	data := []byte(strings.Repeat("ezlm-restart-bytes-", 2048))
	have := len(data) / 2

	var (
		mu     sync.Mutex
		sigs   []string
		ranges []string
	)
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sigs = append(sigs, r.URL.Query().Get("sig"))
		ranges = append(ranges, r.Header.Get("Range"))
		mu.Unlock()
		from := serveCDNRange(t, w, r, data)
		w.Write(data[from:])
	}))
	defer cdn.Close()

	hf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, fmt.Sprintf("%s/blob/model.gguf?sig=%d", cdn.URL, len(sigs)+1), http.StatusFound)
	}))
	defer hf.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(dst+".part", data[:have], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Download(context.Background(), hf.URL+"/resolve/model.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("resume from a part file: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(data) {
		t.Fatalf("file differs after restart resume: %d bytes, want %d", len(got), len(data))
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ranges) != 1 || ranges[0] != fmt.Sprintf("bytes=%d-", have) {
		t.Fatalf("CDN saw ranges %v, want one bytes=%d-", ranges, have)
	}
	if sigs[0] != "1" {
		t.Fatalf("resume used redirect target %q, want the freshly issued one", sigs[0])
	}
}

// A relative Location works too: the redirect target can be same-host.
func TestDownloadFollowsRelativeRedirect(t *testing.T) {
	data := []byte(strings.Repeat("relative-", 256))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/resolve/model.gguf" {
			http.Redirect(w, r, "/cdn/blob", http.StatusFound)
			return
		}
		from := serveCDNRange(t, w, r, data)
		w.Write(data[from:])
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	if err := Download(context.Background(), srv.URL+"/resolve/model.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("relative redirect: %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != string(data) {
		t.Fatalf("file differs: %d bytes", len(got))
	}
}
