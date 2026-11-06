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

// A resume must notice that the server swapped the file underneath it. The old
// head and the new tail are the same length here, so a size check cannot catch
// the mix: only the ETag can, and the download must start over.
func TestDownloadRestartsWhenETagChanges(t *testing.T) {
	oldData := []byte(strings.Repeat("old-model-bytes-", 32)) // 512 bytes
	newData := []byte(strings.Repeat("NEW-MODEL-BYTES-", 32)) // 512 bytes: same length

	for _, tc := range []struct {
		name           string
		honoursIfRange bool
	}{
		{"server honours If-Range", true},
		{"server ignores If-Range", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fastRetries(t)
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := hits.Add(1)
				if n == 1 { // first attempt: half of the old file, then a drop
					w.Header().Set("ETag", `"v1"`)
					w.Header().Set("Content-Length", strconv.Itoa(len(oldData)))
					w.Write(oldData[:len(oldData)/2])
					w.(http.Flusher).Flush()
					conn, _, _ := w.(http.Hijacker).Hijack()
					conn.Close()
					return
				}
				// By the resume the server holds a different file.
				w.Header().Set("ETag", `"v2"`)
				if tc.honoursIfRange && r.Header.Get("If-Range") == `"v1"` {
					// The validator no longer matches: send the whole new file.
					w.Header().Set("Content-Length", strconv.Itoa(len(newData)))
					w.Write(newData)
					return
				}
				from := 0
				if rg := r.Header.Get("Range"); rg != "" {
					from, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
					w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(newData)-1, len(newData)))
					w.Header().Set("Content-Length", strconv.Itoa(len(newData)-from))
					w.WriteHeader(http.StatusPartialContent)
				} else {
					w.Header().Set("Content-Length", strconv.Itoa(len(newData)))
				}
				w.Write(newData[from:])
			}))
			defer srv.Close()

			dst := filepath.Join(t.TempDir(), "m.gguf")
			if err := Download(context.Background(), srv.URL+"/m.gguf", dst, uint64(len(newData)), nil); err != nil {
				t.Fatalf("download after the file changed: %v", err)
			}
			got, err := os.ReadFile(dst)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(newData) {
				t.Fatalf("file mixes the old and the new copy: got %q", got)
			}
		})
	}
}

// An unchanged ETag keeps the resume: no byte is fetched twice.
func TestDownloadResumesWhenETagMatches(t *testing.T) {
	fastRetries(t)
	data := []byte(strings.Repeat("same-model-bytes", 32)) // 512 bytes
	var sent atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		from := 0
		if rg := r.Header.Get("Range"); rg != "" {
			from, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(data)-1, len(data)))
		}
		end := min(from+len(data)/2, len(data))
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)-from)) // half of it, then a drop
		if from > 0 {
			w.WriteHeader(http.StatusPartialContent)
		}
		n, _ := w.Write(data[from:end])
		sent.Add(int32(n))
		if end < len(data) {
			w.(http.Flusher).Flush()
			conn, _, _ := w.(http.Hijacker).Hijack() // cut the connection mid-file
			conn.Close()
		}
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "m.gguf")
	if err := Download(context.Background(), srv.URL+"/m.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download with an unchanged ETag: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("file is wrong: %q", got)
	}
	if got, want := sent.Load(), int32(len(data)); got != want {
		t.Fatalf("an unchanged ETag refetched bytes: sent %d, file is %d", got, want)
	}
}
