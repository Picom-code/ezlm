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
	"time"
)

// serveTrickle writes data from start to the client in small chunks so that a
// second request has time to overlap with the first one.
func serveTrickle(w http.ResponseWriter, r *http.Request, data []byte, served *atomic.Int64) {
	start := 0
	if rg := r.Header.Get("Range"); rg != "" {
		start, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(data)-1, len(data)))
		w.Header().Set("Content-Length", strconv.Itoa(len(data)-start))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	}
	for i := start; i < len(data); i += 32 << 10 {
		end := min(i+32<<10, len(data))
		n, _ := w.Write(data[i:end])
		served.Add(int64(n))
		w.(http.Flusher).Flush()
		time.Sleep(200 * time.Microsecond)
	}
}

// Two callers asking for the same file at the same time must get one download,
// one intact file, and success for both. Today they share the same .part file
// and truncate/append over each other, and the second rename fails.
func TestConcurrentDownloadsOfSameFile(t *testing.T) {
	data := []byte(strings.Repeat("ezlm-concurrent-bytes-", 40000)) // ~840 KB
	var reqs, served atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs.Add(1)
		serveTrickle(w, r, data, &served)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	url := srv.URL + "/model.gguf"

	type result struct {
		err  error
		last uint64
	}
	run := func() result {
		var (
			mu   sync.Mutex
			last uint64
		)
		err := Download(context.Background(), url, dst, uint64(len(data)), func(done, total uint64, bps float64) {
			mu.Lock()
			last = done
			mu.Unlock()
		})
		mu.Lock()
		defer mu.Unlock()
		return result{err: err, last: last}
	}

	start := make(chan struct{})
	results := make([]result, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i] = run()
		}()
	}
	close(start)
	wg.Wait()

	for i, r := range results {
		if r.err != nil {
			t.Fatalf("caller %d: %v", i, r.err)
		}
		if r.last != uint64(len(data)) {
			t.Fatalf("caller %d saw %d of %d bytes", i, r.last, len(data))
		}
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read finished file: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("file is corrupt: %d bytes, want %d", len(got), len(data))
	}
	if reqs.Load() != 1 {
		t.Fatalf("expected one transfer, server got %d requests", reqs.Load())
	}
	if served.Load() != int64(len(data)) {
		t.Fatalf("expected %d bytes served once, got %d", len(data), served.Load())
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Fatalf("partial file left behind: %v", err)
	}
}

// When the shared download fails, both callers get the same clear answer and
// neither leaves a file behind.
func TestConcurrentDownloadsShareFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	url := srv.URL + "/models/missing.gguf"

	errs := make([]error, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = Download(context.Background(), url, dst, 0, nil)
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err == nil || !strings.Contains(err.Error(), "missing.gguf isn't on the download server") {
			t.Fatalf("caller %d: got %v", i, err)
		}
	}
	if errs[0].Error() != errs[1].Error() {
		t.Fatalf("callers got different errors:\n%v\n%v", errs[0], errs[1])
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("a failed download left a file behind: %v", err)
	}
}

// A caller waiting for the same file can give up without disturbing the
// download already in progress.
func TestConcurrentDownloadWaiterCancels(t *testing.T) {
	data := []byte(strings.Repeat("ezlm-concurrent-bytes-", 1000))
	release := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once
	var served atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(started) })
		<-release
		serveTrickle(w, r, data, &served)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	url := srv.URL + "/model.gguf"

	firstDone := make(chan error, 1)
	go func() { firstDone <- Download(context.Background(), url, dst, uint64(len(data)), nil) }()
	<-started // the first request is in flight

	ctx, cancel := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() { secondDone <- Download(ctx, url, dst, uint64(len(data)), nil) }()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-secondDone:
		if err != context.Canceled {
			t.Fatalf("waiting caller: got %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled caller stayed blocked")
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first download: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(data) {
		t.Fatalf("first download was disturbed: %d bytes, want %d", len(got), len(data))
	}
}
