package engine

import (
	"context"
	"fmt"
	"net"
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

func fastRetries(t *testing.T) {
	old := retryStep
	retryStep = time.Millisecond
	t.Cleanup(func() { retryStep = old })
}

// Connection problems end in plain words, not Go's error strings.
func TestDownloadNetErrorsArePlain(t *testing.T) {
	fastRetries(t)
	dir := t.TempDir()

	// No such host: the machine looks offline.
	err := Download(context.Background(), "http://ezlm-download-test.invalid/m.gguf", filepath.Join(dir, "a.gguf"), 0, nil)
	if err == nil || !strings.Contains(err.Error(), "looks offline") {
		t.Fatalf("unknown host: got %v", err)
	}

	// Nothing listening: the connection keeps failing.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	err = Download(context.Background(), "http://"+addr+"/m.gguf", filepath.Join(dir, "b.gguf"), 0, nil)
	if err == nil || !strings.Contains(err.Error(), "keeps dropping") {
		t.Fatalf("refused: got %v", err)
	}

	// A certificate this computer doesn't trust (what an intercepting proxy looks like).
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("x")) }))
	defer srv.Close()
	err = Download(context.Background(), srv.URL+"/m.gguf", filepath.Join(dir, "c.gguf"), 0, nil)
	if err == nil || !strings.Contains(err.Error(), "intercepting secure connections") {
		t.Fatalf("untrusted certificate: got %v", err)
	}
}

// A connection that breaks every few bytes but keeps delivering finishes,
// however many times it breaks.
func TestDownloadFlakyConnectionFinishes(t *testing.T) {
	fastRetries(t)
	data := []byte(strings.Repeat("0123456789", 12)) // 120 bytes, 10 per connection: 12 drops
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		from := 0
		if rg := r.Header.Get("Range"); rg != "" {
			from, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)-from))
		if from > 0 {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(data)-1, len(data)))
			w.WriteHeader(http.StatusPartialContent)
		}
		end := min(from+10, len(data))
		w.Write(data[from:end])
		w.(http.Flusher).Flush()
		if end < len(data) {
			conn, _, _ := w.(http.Hijacker).Hijack() // cut the connection mid-file
			conn.Close()
		}
	}))
	defer srv.Close()
	dst := filepath.Join(t.TempDir(), "m.gguf")
	if err := Download(context.Background(), srv.URL+"/m.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("flaky download gave up after %d connections: %v", hits.Load(), err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(data) {
		t.Fatalf("file is wrong: %q", got)
	}
	if hits.Load() < 12 {
		t.Fatalf("expected at least 12 connections, got %d", hits.Load())
	}
}
