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

// resetConn cuts the connection with a TCP reset instead of a clean close,
// the way a proxy or a CDN drops a long download. Anything already flushed
// reaches the client first.
func resetConn(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	conn, buf, err := w.(http.Hijacker).Hijack()
	if err != nil {
		t.Errorf("hijack: %v", err)
		return
	}
	buf.Flush()
	// Give the bytes time to reach the client: an immediate close can race
	// the request and the client then waits for a keepalive to notice.
	time.Sleep(30 * time.Millisecond)
	if tc, ok := conn.(*net.TCPConn); ok {
		tc.SetLinger(0)
	}
	conn.Close()
}

// serveRange answers a GET or a Range request with the rest of data.
func serveRange(t *testing.T, w http.ResponseWriter, r *http.Request, data []byte) {
	t.Helper()
	from := 0
	if rg := r.Header.Get("Range"); rg != "" {
		from, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
	}
	if from >= len(data) {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", len(data)))
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)-from))
	if from > 0 {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(data)-1, len(data)))
		w.WriteHeader(http.StatusPartialContent)
	}
	w.Write(data[from:])
}

func resetData() []byte {
	return []byte(strings.Repeat("ezlm-model-bytes-", 4096))
}

// A reset before the server sends a single byte of the response is retried,
// and the resumed request still gets the whole file.
func TestDownloadResetBeforeHeaders(t *testing.T) {
	fastRetries(t)
	data := resetData()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) <= 2 {
			resetConn(t, w) // no status line, no headers, just a reset
			return
		}
		serveRange(t, w, r, data)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	if err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download after two resets at offset 0: %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != string(data) {
		t.Fatalf("file is %d bytes, want %d", len(got), len(data))
	}
	if hits.Load() != 3 {
		t.Fatalf("expected 3 requests, got %d", hits.Load())
	}
}

// A reset in the middle of the response headers is retried the same way.
func TestDownloadResetMidHeader(t *testing.T) {
	fastRetries(t)
	data := resetData()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			conn, buf, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			fmt.Fprintf(buf, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\nContent-Ty", len(data))
			buf.Flush()
			if tc, ok := conn.(*net.TCPConn); ok {
				tc.SetLinger(0)
			}
			conn.Close()
			return
		}
		serveRange(t, w, r, data)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	if err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download after a reset mid-header: %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != string(data) {
		t.Fatalf("file is %d bytes, want %d", len(got), len(data))
	}
	if hits.Load() < 2 {
		t.Fatalf("expected a retry, got %d request(s)", hits.Load())
	}
}

// A reset in the middle of the body keeps the bytes that arrived and resumes
// from there, so a reset every few KB still finishes with the right file.
func TestDownloadResetMidBody(t *testing.T) {
	fastRetries(t)
	data := resetData()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		from := 0
		if rg := r.Header.Get("Range"); rg != "" {
			from, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
		}
		if from >= len(data) {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		rest := data[from:]
		w.Header().Set("Content-Length", strconv.Itoa(len(rest)))
		if from > 0 {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(data)-1, len(data)))
			w.WriteHeader(http.StatusPartialContent)
		}
		// Every connection dies after 8 KB: the file takes several resets.
		cut := min(8192, len(rest))
		w.Write(rest[:cut])
		w.(http.Flusher).Flush()
		if cut < len(rest) {
			resetConn(t, w)
		}
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	if err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download with a reset every 8 KB: %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != string(data) {
		t.Fatalf("file is %d bytes, want %d", len(got), len(data))
	}
	if hits.Load() < 2 {
		t.Fatalf("expected several connections, got %d", hits.Load())
	}
}

// The connection dies after the last byte but before the end of the chunked
// body. The file is already complete, so the resume must not download it
// again from zero.
func TestDownloadResetAtLastByte(t *testing.T) {
	fastRetries(t)
	data := resetData()
	var hits atomic.Int32
	var rangeAtResume atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			// No Content-Length, so the response is chunked and the client
			// needs the closing chunk to know the body ended. Send every
			// byte, then reset before the closing chunk.
			for off := 0; off < len(data); off += 4096 {
				w.Write(data[off:min(off+4096, len(data))])
				w.(http.Flusher).Flush()
			}
			resetConn(t, w)
			return
		}
		if rg := r.Header.Get("Range"); rg != "" {
			from, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
			rangeAtResume.Store(int64(from))
		}
		serveRange(t, w, r, data)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	if err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download after a reset at the last byte: %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != string(data) {
		t.Fatalf("file is %d bytes, want %d", len(got), len(data))
	}
	if got := rangeAtResume.Load(); got > 0 && got < int64(len(data))-1 {
		t.Fatalf("resume started at %d, want %d (the file was already complete)", got, len(data))
	}
}

// The last byte arrives and then the server resets every later request. The
// finished file is already on disk, so the download must keep it instead of
// failing after a pointless round of retries.
func TestDownloadResetAtLastByteKeepsFile(t *testing.T) {
	fastRetries(t)
	data := resetData()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			for off := 0; off < len(data); off += 4096 {
				w.Write(data[off:min(off+4096, len(data))])
				w.(http.Flusher).Flush()
			}
		}
		resetConn(t, w) // every request ends in a reset
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	if err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("complete file thrown away because the server keeps resetting: %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != string(data) {
		t.Fatalf("file is %d bytes, want %d", len(got), len(data))
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("asked a dead server %d times for a file already complete", n)
	}
}

// A server that resets every request must fail with a plain message after a
// bounded number of attempts, not hang and not loop forever.
func TestDownloadResetEveryRequestFailsCleanly(t *testing.T) {
	fastRetries(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		resetConn(t, w)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	err := Download(context.Background(), srv.URL+"/model.gguf", dst, 100, nil)
	if err == nil || !strings.Contains(err.Error(), "keeps dropping") {
		t.Fatalf("got %v, want a plain connection error", err)
	}
	if n := hits.Load(); n < 2 || n > 10 {
		t.Fatalf("expected a few attempts, got %d", n)
	}
	if _, statErr := os.Stat(dst); statErr == nil {
		t.Fatal("a failed download left a file at the destination")
	}
}
