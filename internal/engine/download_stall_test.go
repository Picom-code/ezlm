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
	"sync/atomic"
	"testing"
	"time"
)

// TestMain shortens the stall timeout once, before any test starts. Setting
// it inside a test races with the watchdog goroutine of a download started by
// an earlier test, which reads stallTimeout as it starts (download.go:109).
func TestMain(m *testing.M) {
	stallTimeout = 300 * time.Millisecond
	os.Exit(m.Run())
}

// withStallTimeout shortens the watchdog for one test, then restores the
// package default from TestMain.
func withStallTimeout(t *testing.T, d time.Duration) {
	old := stallTimeout
	stallTimeout = d
	t.Cleanup(func() { stallTimeout = old })
}

// A server that sends part of a file and then goes silent (the connection
// stays open) must not hang the download: it's cut and resumed.
func TestDownloadResumesAfterStall(t *testing.T) {
	withStallTimeout(t, 300*time.Millisecond)

	data := []byte(strings.Repeat("ezlm-model-bytes-", 4096)) // ~69 KB
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		start := 0
		if rg := r.Header.Get("Range"); rg != "" {
			start, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(data)-1, len(data)))
			w.Header().Set("Content-Length", strconv.Itoa(len(data)-start))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		}
		if n == 1 { // first attempt: half the file, then silence
			w.Write(data[:len(data)/2])
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
			case <-time.After(10 * time.Second):
			}
			return
		}
		w.Write(data[start:])
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	t0 := time.Now()
	if err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(data) {
		t.Fatalf("file differs after resume: %d bytes, want %d", len(got), len(data))
	}
	if hits.Load() != 2 || time.Since(t0) > 8*time.Second {
		t.Fatalf("expected one stall then a resumed request: %d requests in %s", hits.Load(), time.Since(t0))
	}
}

// The same over HTTP/2 (Hugging Face's CDN speaks it): a silent stream must
// still be cut and resumed.
func TestDownloadResumesAfterStallHTTP2(t *testing.T) {
	withStallTimeout(t, 300*time.Millisecond)

	data := []byte(strings.Repeat("ezlm-model-bytes-", 4096))
	var hits atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		start := 0
		if rg := r.Header.Get("Range"); rg != "" {
			start, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(data)-1, len(data)))
			w.Header().Set("Content-Length", strconv.Itoa(len(data)-start))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		}
		if n == 1 {
			w.Write(data[:len(data)/2])
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
			case <-time.After(20 * time.Second):
			}
			return
		}
		w.Write(data[start:])
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	tr := dlClient.Transport.(*http.Transport)
	oldTLS := tr.TLSClientConfig
	tr.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig
	defer func() { tr.TLSClientConfig = oldTLS; tr.CloseIdleConnections() }()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	done := make(chan error, 1)
	go func() { done <- Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(data)), nil) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("download: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("download hung on a silent HTTP/2 stream")
	}
	if got, _ := os.ReadFile(dst); string(got) != string(data) {
		t.Fatalf("file differs after resume: %d bytes", len(got))
	}
}

// dripServer sends body in small pieces, pausing between them, so the
// connection is slow but never silent for long. It answers Range requests.
func dripServer(t *testing.T, data []byte, pieces int, gap time.Duration) *httptest.Server {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		start := 0
		if rg := r.Header.Get("Range"); rg != "" {
			start, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(data)-1, len(data)))
			w.Header().Set("Content-Length", strconv.Itoa(len(data)-start))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		}
		rest := data[start:]
		step := (len(rest) + pieces - 1) / pieces
		for off := 0; off < len(rest); off += step {
			end := min(off+step, len(rest))
			if _, err := w.Write(rest[off:end]); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			if end < len(rest) {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(gap):
				}
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A slow connection that keeps delivering must not trip the watchdog: the
// download takes far longer than the stall timeout, with a request for each
// piece, and must still be a single request.
func TestDownloadSlowDripDoesNotStall(t *testing.T) {
	withStallTimeout(t, 400*time.Millisecond)
	fastRetries(t)

	data := []byte(strings.Repeat("slow-but-alive-", 2048)) // ~30 KB
	// 30 pieces, 120ms apart: 3.6s in total, nine stall timeouts long, and
	// every gap is well under the 400ms threshold.
	srv := dripServer(t, data, 30, 120*time.Millisecond)

	dst := filepath.Join(t.TempDir(), "model.gguf")
	t0 := time.Now()
	if err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download of a slow but alive connection failed: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(data) {
		t.Fatalf("file differs: %d bytes, want %d", len(got), len(data))
	}
	if time.Since(t0) < 3*time.Second {
		t.Fatalf("test finished too fast to prove anything: %s", time.Since(t0))
	}
}

// A gap just under the threshold is still alive: the watchdog must use the
// time since the last byte, not the length of the transfer.
func TestDownloadDripWithGapsJustUnderTimeout(t *testing.T) {
	withStallTimeout(t, 300*time.Millisecond)
	fastRetries(t)

	data := []byte(strings.Repeat("x", 12*1024))
	srv := dripServer(t, data, 12, 220*time.Millisecond) // 2.4s, each gap 0.73x the timeout

	dst := filepath.Join(t.TempDir(), "model.gguf")
	if err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != string(data) {
		t.Fatalf("file differs: %d bytes", len(got))
	}
}

// A server that sends headers and part of the body and then goes silent must
// be cut even when the body is chunked and has no Content-Length, as Hugging
// Face's CDN sometimes does.
func TestDownloadStallOnChunkedBodyFires(t *testing.T) {
	withStallTimeout(t, 250*time.Millisecond)
	fastRetries(t)

	data := []byte(strings.Repeat("chunked-body-", 2048))
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		start := 0
		if rg := r.Header.Get("Range"); rg != "" {
			start, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(data)-1, len(data)))
			w.WriteHeader(http.StatusPartialContent)
		}
		if n == 1 {
			w.Write(data[:len(data)/2]) // no Content-Length: chunked
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
			case <-time.After(10 * time.Second):
			}
			return
		}
		w.Write(data[start:])
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	done := make(chan error, 1)
	go func() { done <- Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(data)), nil) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("download: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("watchdog did not fire on a silent chunked body")
	}
	if got, _ := os.ReadFile(dst); string(got) != string(data) {
		t.Fatalf("file differs after resume: %d bytes", len(got))
	}
	if hits.Load() < 2 {
		t.Fatalf("expected the stalled request to be retried, got %d request(s)", hits.Load())
	}
}

// A server that accepts the connection and then sends nothing at all is
// reported as stalling, in plain words, instead of hanging.
func TestDownloadStallBeforeBodyReportsStalling(t *testing.T) {
	withStallTimeout(t, 150*time.Millisecond)
	fastRetries(t)

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		<-r.Context().Done() // never write a byte
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	done := make(chan error, 1)
	go func() { done <- Download(context.Background(), srv.URL+"/model.gguf", dst, 1024, nil) }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "keeps stalling") {
			t.Fatalf("silent server: got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("download hung on a server that never sends a byte")
	}
	if hits.Load() < 2 {
		t.Fatalf("a stalled attempt must be retried, got %d request(s)", hits.Load())
	}
}
