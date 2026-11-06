package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A missing file fails at once with a plain message; a busy server is retried.
func TestDownloadStatusHandling(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if strings.Contains(r.URL.Path, "missing") {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	dir := t.TempDir()

	start := time.Now()
	err := Download(context.Background(), srv.URL+"/models/missing-Q4.gguf", filepath.Join(dir, "a.gguf"), 0, nil)
	if err == nil || !strings.Contains(err.Error(), "missing-Q4.gguf isn't on the download server") {
		t.Fatalf("404: got %v", err)
	}
	if time.Since(start) > time.Second || hits.Load() != 1 {
		t.Fatalf("404 was retried: %d requests in %s", hits.Load(), time.Since(start))
	}

	hits.Store(0)
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	Download(ctx, srv.URL+"/models/busy.gguf", filepath.Join(dir, "b.gguf"), 0, nil)
	if hits.Load() < 2 {
		t.Fatalf("503 should be retried, got %d request(s)", hits.Load())
	}
}
