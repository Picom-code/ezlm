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

// progressTick is one call of a Progress callback, with when it arrived.
type progressTick struct {
	done, total uint64
	bps         float64
	at          time.Time
}

// progressRecorder collects every Progress tick, in order.
type progressRecorder struct {
	mu    sync.Mutex
	ticks []progressTick
}

func (r *progressRecorder) prog(done, total uint64, bps float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ticks = append(r.ticks, progressTick{done: done, total: total, bps: bps, at: time.Now()})
}

func (r *progressRecorder) all() []progressTick {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]progressTick(nil), r.ticks...)
}

// rangeFrom reads the first byte offset of a Range header.
func rangeFrom(r *http.Request) int {
	rg := r.Header.Get("Range")
	if rg == "" {
		return 0
	}
	from, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
	return from
}

// A normal download gives the caller a usable stream: a known total, a done
// that never passes it, a positive speed while bytes arrive, and a final tick
// that matches the file on disk.
func TestProgressTicksAreUsable(t *testing.T) {
	fastRetries(t)
	data := []byte(strings.Repeat("ezlm-model-bytes-", 4096)) // ~69 KB
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "m.gguf", time.Time{}, strings.NewReader(string(data)))
	}))
	defer srv.Close()

	rec := &progressRecorder{}
	dst := filepath.Join(t.TempDir(), "m.gguf")
	if err := Download(context.Background(), srv.URL+"/m.gguf", dst, uint64(len(data)), rec.prog); err != nil {
		t.Fatalf("download: %v", err)
	}
	ticks := rec.all()
	if len(ticks) == 0 {
		t.Fatal("a download reported no progress at all")
	}
	var fast bool
	for _, tk := range ticks {
		if tk.total != uint64(len(data)) {
			t.Errorf("tick reported total %d, want the known size %d", tk.total, len(data))
		}
		if tk.done > tk.total {
			t.Errorf("tick reported %d of %d: more done than total", tk.done, tk.total)
		}
		if tk.bps > 0 {
			fast = true
		}
	}
	if !fast {
		t.Error("no tick carried a speed while bytes were arriving")
	}
	if last := ticks[len(ticks)-1]; last.done != uint64(len(data)) {
		t.Errorf("last tick reported %d of %d, want the whole file", last.done, len(data))
	}
}

// A file that is already there is reported once, at 100%, with no speed and
// no request to the server.
func TestProgressWhenFileIsAlreadyComplete(t *testing.T) {
	data := []byte(strings.Repeat("ezlm-model-bytes-", 128))
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.NotFound(w, r)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "m.gguf")
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
	rec := &progressRecorder{}
	if err := Download(context.Background(), srv.URL+"/m.gguf", dst, uint64(len(data)), rec.prog); err != nil {
		t.Fatalf("download: %v", err)
	}
	ticks := rec.all()
	if len(ticks) != 1 {
		t.Fatalf("want one tick for an already-complete file, got %d", len(ticks))
	}
	if tk := ticks[0]; tk.done != uint64(len(data)) || tk.total != uint64(len(data)) || tk.bps != 0 {
		t.Fatalf("tick is %+v, want done=total=%d and no speed", tk, len(data))
	}
	if hits.Load() != 0 {
		t.Fatalf("the file was already complete, but the server was asked %d time(s)", hits.Load())
	}
}

// A stall must be visible in the progress stream. The watchdog cuts a silent
// connection; the caller should then get a zero-speed tick with the bytes it
// has, so a stalled download doesn't look like a frozen speed and ETA.
func TestProgressReportsStallBeforeRetry(t *testing.T) {
	fastRetries(t)
	old := stallTimeout
	stallTimeout = 300 * time.Millisecond
	defer func() { stallTimeout = old }()

	data := []byte(strings.Repeat("ezlm-model-bytes-", 4096))
	half := len(data) / 2
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		from := rangeFrom(r)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)-from))
		if from > 0 {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(data)-1, len(data)))
			w.WriteHeader(http.StatusPartialContent)
		}
		if n == 1 { // first attempt: half the file, then silence
			w.Write(data[:half])
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		w.Write(data[from:])
	}))
	defer srv.Close()

	rec := &progressRecorder{}
	dst := filepath.Join(t.TempDir(), "m.gguf")
	if err := Download(context.Background(), srv.URL+"/m.gguf", dst, uint64(len(data)), rec.prog); err != nil {
		t.Fatalf("download: %v", err)
	}
	if got, _ := os.ReadFile(dst); len(got) != len(data) {
		t.Fatalf("file is %d bytes, want %d", len(got), len(data))
	}

	ticks := rec.all()
	stall := -1
	for i, tk := range ticks {
		if tk.bps == 0 {
			stall = i
			break
		}
	}
	if stall < 0 {
		t.Fatalf("the stall was invisible to the caller: %d ticks, last %+v, none with speed 0", len(ticks), ticks[len(ticks)-1])
	}
	if got := ticks[stall].done; got != uint64(half) {
		t.Errorf("stall tick reported %d bytes, want the %d bytes on disk", got, half)
	}
	if ticks[stall].total != uint64(len(data)) {
		t.Errorf("stall tick reported total %d, want %d", ticks[stall].total, len(data))
	}
	if last := ticks[len(ticks)-1]; last.done != uint64(len(data)) || last.bps <= 0 {
		t.Errorf("after the resume the last tick is %+v, want the whole file at a positive speed", last)
	}
}

// The same for a connection that breaks mid-body: the caller sees data stop
// moving (speed 0) before the retry, not only when the whole download fails.
func TestProgressReportsDropBeforeRetry(t *testing.T) {
	fastRetries(t)
	data := []byte(strings.Repeat("ezlm-model-bytes-", 4096))
	half := len(data) / 2
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		from := rangeFrom(r)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)-from))
		if from > 0 {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(data)-1, len(data)))
			w.WriteHeader(http.StatusPartialContent)
		}
		if from == 0 { // first attempt: cut the connection after half the file
			w.Write(data[:half])
			w.(http.Flusher).Flush()
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		w.Write(data[from:])
	}))
	defer srv.Close()

	rec := &progressRecorder{}
	dst := filepath.Join(t.TempDir(), "m.gguf")
	if err := Download(context.Background(), srv.URL+"/m.gguf", dst, uint64(len(data)), rec.prog); err != nil {
		t.Fatalf("download: %v", err)
	}
	ticks := rec.all()
	var stall *progressTick
	for i := range ticks {
		if ticks[i].bps == 0 {
			stall = &ticks[i]
			break
		}
	}
	if stall == nil {
		t.Fatalf("the dropped connection was invisible to the caller: %d ticks, last %+v", len(ticks), ticks[len(ticks)-1])
	}
	if stall.done != uint64(half) {
		t.Errorf("drop tick reported %d bytes, want the %d bytes on disk", stall.done, half)
	}
	if hits.Load() < 2 {
		t.Fatalf("expected a resumed request, got %d", hits.Load())
	}
}

// With no size argument and no Content-Length the caller gets total 0, which
// the apps treat as "unknown" (no bar, no ETA), never a made-up total.
func TestProgressWithUnknownTotal(t *testing.T) {
	data := []byte(strings.Repeat("ezlm-model-bytes-", 256))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush() // no Content-Length: chunked
		w.Write(data)
	}))
	defer srv.Close()

	rec := &progressRecorder{}
	dst := filepath.Join(t.TempDir(), "m.gguf")
	if err := Download(context.Background(), srv.URL+"/m.gguf", dst, 0, rec.prog); err != nil {
		t.Fatalf("download: %v", err)
	}
	ticks := rec.all()
	if len(ticks) == 0 {
		t.Fatal("no progress reported")
	}
	for _, tk := range ticks {
		if tk.total != 0 {
			t.Errorf("tick invented a total of %d with no length known", tk.total)
		}
	}
	if last := ticks[len(ticks)-1]; last.done != uint64(len(data)) {
		t.Errorf("last tick reported %d of an unknown total, want %d", last.done, len(data))
	}
}

// A server that ignores Range restarts the transfer from zero, which the
// caller sees as done dropping back near zero (the first tick after the
// restart is the first read, not a tidy 0). That is the only backward step a
// download may make: the ticks after it must grow to the whole file.
func TestProgressRestartIsFromZero(t *testing.T) {
	fastRetries(t)
	data := []byte(strings.Repeat("ezlm-model-bytes-", 4096))
	half := len(data) / 2
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		if n == 1 { // half the file, then cut, and ignore Range on the retry
			w.Write(data[:half])
			w.(http.Flusher).Flush()
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		w.Write(data)
	}))
	defer srv.Close()

	rec := &progressRecorder{}
	dst := filepath.Join(t.TempDir(), "m.gguf")
	if err := Download(context.Background(), srv.URL+"/m.gguf", dst, uint64(len(data)), rec.prog); err != nil {
		t.Fatalf("download: %v", err)
	}
	ticks := rec.all()
	high, restarts, lastBack := uint64(0), 0, -1
	for i, tk := range ticks {
		if tk.done < high {
			restarts++
			lastBack = i
		}
		if tk.done > high {
			high = tk.done
		}
	}
	if restarts > 1 {
		t.Fatalf("progress went backwards %d times, want at most one restart: %+v", restarts, ticks)
	}
	if lastBack >= 0 {
		for i := lastBack + 1; i < len(ticks); i++ {
			if ticks[i].done < ticks[i-1].done {
				t.Fatalf("progress still went backwards after the restart: %+v", ticks)
			}
		}
	}
	if last := ticks[len(ticks)-1]; last.done != uint64(len(data)) {
		t.Fatalf("last tick reported %d of %d", last.done, len(data))
	}
}
