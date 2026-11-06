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
)

var rangeData = []byte(strings.Repeat("ezlm-model-bytes-", 4096)) // ~69 KB

// A resume request can meet a server that ignores Range and answers 200 with
// the whole file. The part file must be replaced, not appended to, and the
// download must still finish in one request.
func TestDownloadRestartsWhenServerIgnoresRange(t *testing.T) {
	var hits, ranged atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Range") != "" {
			ranged.Add(1)
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(rangeData)))
		w.WriteHeader(http.StatusOK) // ignores Range
		w.Write(rangeData)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(dst+".part", rangeData[:1000], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(rangeData)), nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(rangeData) {
		t.Fatalf("file is %d bytes, want %d", len(got), len(rangeData))
	}
	if ranged.Load() == 0 {
		t.Fatal("the resume request carried no Range header")
	}
	if hits.Load() != 1 {
		t.Fatalf("expected one request, got %d", hits.Load())
	}
}

// The same fault with the wrong status: some servers answer a Range request
// with 206 but still send the whole file from byte 0. Appending that to the
// part file corrupts the download.
func TestDownloadRestartsWhenPartialContentIgnoresRange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(rangeData)))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(rangeData)-1, len(rangeData)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(rangeData)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(dst+".part", rangeData[:1000], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Download(context.Background(), srv.URL+"/model.gguf", dst, 0, nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(rangeData) {
		t.Fatalf("file is %d bytes, want %d", len(got), len(rangeData))
	}
}

// The same again with a known size: before the fix the size check threw the
// whole part file away instead of restarting it.
func TestDownloadRestartsWhenPartialContentIgnoresRangeSized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(rangeData)))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(rangeData)-1, len(rangeData)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(rangeData)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(dst+".part", rangeData[:1000], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(rangeData)), nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(rangeData) {
		t.Fatalf("file is %d bytes, want %d", len(got), len(rangeData))
	}
}

// A 206 from an offset EZLM didn't ask for can't be appended either, and a
// retry can't fix it: the download fails at once and keeps the part file.
func TestDownloadWrongContentRangeFailsFast(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body := rangeData[500:]
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 500-%d/%d", len(rangeData)-1, len(rangeData)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(body)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "model.gguf")
	part := dst + ".part"
	if err := os.WriteFile(part, rangeData[:1000], 0o644); err != nil {
		t.Fatal(err)
	}
	err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(rangeData)), nil)
	if err == nil || !strings.Contains(err.Error(), "byte 500") {
		t.Fatalf("wrong Content-Range: got %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("a wrong range should not be retried, got %d requests", hits.Load())
	}
	if st, statErr := os.Stat(part); statErr != nil || st.Size() != 1000 {
		t.Fatalf("the part file was lost: %v", statErr)
	}
}
