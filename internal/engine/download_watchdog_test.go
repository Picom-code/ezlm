package engine

import (
	"context"
	"errors"
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

// watchdogSrv serves data over HTTP with Range support. It waits headerDelay
// before sending the response headers, then calls body, which decides what to
// send and when. The handler keeps the connection open until body returns, so
// a body that writes nothing is a stall rather than a short body.
func watchdogSrv(data []byte, headerDelay time.Duration, body func(w http.ResponseWriter, r *http.Request, start int, hit int32)) (*httptest.Server, *atomic.Int32) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit := hits.Add(1)
		start := 0
		if rg := r.Header.Get("Range"); rg != "" {
			start, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
		}
		if start >= len(data) {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", len(data)))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if headerDelay > 0 {
			time.Sleep(headerDelay)
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)-start))
		if start > 0 {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(data)-1, len(data)))
			w.WriteHeader(http.StatusPartialContent)
		}
		w.(http.Flusher).Flush()
		body(w, r, start, hit)
	}))
	return srv, &hits
}

// shortStall lowers the watchdog threshold for a test.
func shortStall(t *testing.T, d time.Duration) {
	old := stallTimeout
	stallTimeout = d
	t.Cleanup(func() { stallTimeout = old })
}

// writeThenWait sends b, flushes it, then holds the connection open until the
// client gives up on it. The next byte would never come.
func writeThenWait(w http.ResponseWriter, r *http.Request, b []byte) {
	if len(b) > 0 {
		w.Write(b)
		w.(http.Flusher).Flush()
	}
	<-r.Context().Done()
}

// A link that is slow but keeps delivering must never trip the watchdog, even
// when the whole transfer takes several times the stall threshold.
func TestWatchdogSlowButAliveLink(t *testing.T) {
	const threshold = 300 * time.Millisecond
	shortStall(t, threshold)

	data := []byte(strings.Repeat("ezlm-slow-link-", 512)) // ~8 KB
	srv, hits := watchdogSrv(data, 0, func(w http.ResponseWriter, r *http.Request, start int, hit int32) {
		// Ten chunks, one every third of the threshold: every gap is well
		// under it, but the transfer lasts three times the threshold.
		chunk := (len(data) + 9) / 10
		for i := start; i < len(data); i += chunk {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(threshold / 3):
			}
			w.Write(data[i:min(i+chunk, len(data))])
			w.(http.Flusher).Flush()
		}
	})
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	begin := time.Now()
	if err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("slow but alive link: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("a slow but alive link was cut %d times, want one request", hits.Load()-1)
	}
	if time.Since(begin) < threshold {
		t.Fatalf("transfer finished in %s, too fast to test the watchdog", time.Since(begin))
	}
	if got, _ := os.ReadFile(dst); string(got) != string(data) {
		t.Fatalf("file differs: %d bytes, want %d", len(got), len(data))
	}
}

// A gap longer than the threshold is a stall: the attempt is cut, and the
// resumed request starts where the first one stopped.
func TestWatchdogGapOverThresholdResumes(t *testing.T) {
	fastRetries(t)
	const threshold = 200 * time.Millisecond
	shortStall(t, threshold)

	data := []byte(strings.Repeat("ezlm-stall-", 300))
	srv, hits := watchdogSrv(data, 0, func(w http.ResponseWriter, r *http.Request, start int, hit int32) {
		if hit == 1 {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(2 * threshold):
			}
		}
		w.Write(data[start:])
	})
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	if err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("expected a stall then one resumed request, got %d requests", hits.Load())
	}
	if got, _ := os.ReadFile(dst); string(got) != string(data) {
		t.Fatalf("file differs after resume: %d bytes, want %d", len(got), len(data))
	}
}

// The watchdog clock must restart when the response headers arrive. A slow
// handshake followed by a slow first byte is not a stall, even when the two
// waits together are longer than the threshold.
func TestWatchdogNoFalseStallBetweenHeadersAndFirstByte(t *testing.T) {
	const threshold = time.Second
	shortStall(t, threshold)

	data := []byte(strings.Repeat("ezlm-handoff-", 200))
	srv, hits := watchdogSrv(data, 8*threshold/10, func(w http.ResponseWriter, r *http.Request, start int, hit int32) {
		// The first body byte arrives another 0.8 thresholds after the
		// headers: each wait is under the threshold on its own.
		select {
		case <-r.Context().Done():
			return
		case <-time.After(8 * threshold / 10):
		}
		w.Write(data[start:])
	})
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	if err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("slow handoff was taken for a stall: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("slow handoff was cut %d times, want one request", hits.Load()-1)
	}
}

// A server that sends headers and then nothing at all, over and over, runs out
// of retries and says so in plain words. The .part file is kept for a resume.
func TestWatchdogSilentStallsGiveUp(t *testing.T) {
	fastRetries(t)
	shortStall(t, 100*time.Millisecond)

	data := []byte(strings.Repeat("ezlm-silent-", 100))
	srv, hits := watchdogSrv(data, 0, func(w http.ResponseWriter, r *http.Request, start int, hit int32) {
		<-r.Context().Done()
	})
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(data)), nil)
	if err == nil || !strings.Contains(err.Error(), "keeps stalling") {
		t.Fatalf("silent server: got %v", err)
	}
	if hits.Load() != 5 {
		t.Fatalf("expected 5 stalled attempts, got %d", hits.Load())
	}
	if _, statErr := os.Stat(dst + ".part"); statErr != nil {
		t.Fatalf("the partial file was removed, so a resume is impossible: %v", statErr)
	}
	if _, statErr := os.Stat(dst); statErr == nil {
		t.Fatal("the partial file was renamed to the final name")
	}
}

// Each attempt that stalls still delivers some bytes. That counts as progress,
// so the retry budget resets and a long download over a stalling link can take
// more than five attempts.
func TestWatchdogStallProgressResetsRetryBudget(t *testing.T) {
	fastRetries(t)
	shortStall(t, 80*time.Millisecond)

	data := []byte(strings.Repeat("ezlm-chunk-", 100)) // 1000 bytes
	chunk := len(data) / 8
	srv, hits := watchdogSrv(data, 0, func(w http.ResponseWriter, r *http.Request, start int, hit int32) {
		end := min(start+chunk, len(data))
		if end == len(data) {
			w.Write(data[start:end]) // last bytes: the body ends cleanly
			return
		}
		writeThenWait(w, r, data[start:end])
	})
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	if err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download gave up after %d stalled attempts: %v", hits.Load(), err)
	}
	if hits.Load() < 6 {
		t.Fatalf("expected more than five stalled attempts, got %d", hits.Load())
	}
	if got, _ := os.ReadFile(dst); string(got) != string(data) {
		t.Fatalf("file differs: %d bytes, want %d", len(got), len(data))
	}
}

// Cancelling the caller's context stops the download at once; it is not a
// stall and must not be retried.
func TestWatchdogCallerCancelIsNotARetry(t *testing.T) {
	fastRetries(t)
	shortStall(t, 10*time.Second)

	data := []byte(strings.Repeat("ezlm-cancel-", 100))
	srv, hits := watchdogSrv(data, 0, func(w http.ResponseWriter, r *http.Request, start int, hit int32) {
		<-r.Context().Done()
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := Download(ctx, srv.URL+"/model.gguf", filepath.Join(t.TempDir(), "model.gguf"), uint64(len(data)), nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled download: got %v, want the context error", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("a cancelled download was retried: %d requests", hits.Load())
	}
}
