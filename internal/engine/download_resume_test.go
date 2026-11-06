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
	"testing"
)

// restartServer serves one file the way a CDN does: it honours Range, sends an
// ETag, and answers If-Range with the whole body when the tag doesn't match.
// It records what each request asked for so a test can see the resume offset.
type restartServer struct {
	mu      sync.Mutex
	data    []byte
	etag    string
	ranges  []string
	ifRange []string
}

func (s *restartServer) setFile(data []byte, etag string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data, s.etag = data, etag
}

func (s *restartServer) requests() (ranges, ifRange []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.ranges...), append([]string(nil), s.ifRange...)
}

func (s *restartServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	data, etag := s.data, s.etag
	rg, ir := r.Header.Get("Range"), r.Header.Get("If-Range")
	s.ranges = append(s.ranges, rg)
	s.ifRange = append(s.ifRange, ir)
	s.mu.Unlock()

	w.Header().Set("ETag", etag)
	w.Header().Set("Accept-Ranges", "bytes")
	if rg != "" && (ir == "" || ir == etag) {
		start, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
		if start >= len(data) {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", len(data)))
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(data)-1, len(data)))
		w.Header().Set("Content-Length", strconv.Itoa(len(data)-start))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(data[start:])
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

func newRestartServer(t *testing.T, data []byte, etag string) (*restartServer, *httptest.Server) {
	t.Helper()
	s := &restartServer{data: data, etag: etag}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	return s, srv
}

// A download killed halfway through is resumed from the bytes already on disk
// after the app starts again, not restarted from zero.
func TestDownloadResumeAfterRestart(t *testing.T) {
	fastRetries(t)
	data := []byte(strings.Repeat("ezlm-model-bytes-", 4096))
	half := len(data) / 2
	srvState, srv := newRestartServer(t, data, `"v1"`)

	dst := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(dst+".part", data[:half], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("resume after restart: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("resumed file differs: %d bytes, want %d", len(got), len(data))
	}
	ranges, _ := srvState.requests()
	if len(ranges) != 1 || ranges[0] != fmt.Sprintf("bytes=%d-", half) {
		t.Fatalf("resume asked for %q, want one request from byte %d", ranges, half)
	}
	for _, leftover := range []string{dst + ".part", dst + ".part.state"} {
		if _, err := os.Stat(leftover); !os.IsNotExist(err) {
			t.Fatalf("%s survived a finished download", filepath.Base(leftover))
		}
	}
}

// The .part left by an interrupted download of an older, larger file is longer
// than the file on the server now. The resume must not keep the stale bytes.
func TestDownloadStalePartLongerThanFile(t *testing.T) {
	fastRetries(t)
	data := []byte(strings.Repeat("new-file-", 64))
	stale := append(append([]byte(nil), data...), []byte(strings.Repeat("old-tail-", 64))...)
	srvState, srv := newRestartServer(t, data, `"v2"`)

	for _, tc := range []struct {
		name string
		want uint64
	}{
		{"size unknown (split part)", 0},
		{"size known", uint64(len(data))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prior, _ := srvState.requests()
			dst := filepath.Join(t.TempDir(), "model.gguf")
			if err := os.WriteFile(dst+".part", stale, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := Download(context.Background(), srv.URL+"/model.gguf", dst, tc.want, nil); err != nil {
				t.Fatalf("download with a stale partial: %v", err)
			}
			got, err := os.ReadFile(dst)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(data) {
				t.Fatalf("stale partial corrupted the file: %d bytes, want %d", len(got), len(data))
			}
			if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
				t.Fatal("partial file survived the finished download")
			}
			// At most a probe with the stale offset, then a full request.
			ranges, _ := srvState.requests()
			asked := ranges[len(prior):]
			if len(asked) == 0 || len(asked) > 2 || asked[len(asked)-1] != "" {
				t.Fatalf("stale partial was not replaced by a full request: %q", asked)
			}
		})
	}
}

// The server's file changed while the app was closed. The saved revision in
// the state file must make the resume start over instead of appending new
// bytes to old ones.
func TestDownloadStaleStateRestartsWhenFileChanged(t *testing.T) {
	fastRetries(t)
	oldData := []byte(strings.Repeat("old-revision-", 32))
	newData := []byte(strings.Repeat("new-revision-", 32))
	if len(oldData) != len(newData) {
		t.Fatal("test needs two revisions of the same length")
	}
	srvState, srv := newRestartServer(t, newData, `"v2"`)

	dst := filepath.Join(t.TempDir(), "model.gguf")
	half := len(oldData) / 2
	if err := os.WriteFile(dst+".part", oldData[:half], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst+".part.state", []byte(`"v1"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(newData)), nil); err != nil {
		t.Fatalf("download after the server file changed: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(newData) {
		t.Fatalf("resume appended to a stale partial: %q", got)
	}
	ranges, ifRange := srvState.requests()
	if len(ranges) != 1 || ranges[0] != fmt.Sprintf("bytes=%d-", half) {
		t.Fatalf("expected one resume request from byte %d, got %q", half, ranges)
	}
	if len(ifRange) != 1 || ifRange[0] != `"v1"` {
		t.Fatalf("resume sent If-Range %q, want the saved revision", ifRange)
	}
	if _, err := os.Stat(dst + ".part.state"); !os.IsNotExist(err) {
		t.Fatal("state file survived the finished download")
	}
}

// A state file the app can't read is not a reason to fail or to corrupt the
// download: the resume falls back to the bytes on disk.
func TestDownloadUnreadableStateFileIgnored(t *testing.T) {
	fastRetries(t)
	data := []byte(strings.Repeat("ezlm-model-bytes-", 256))
	half := len(data) / 2
	_, srv := newRestartServer(t, data, `"v1"`)

	dst := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(dst+".part", data[:half], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst+".part.state", []byte("\x00\x00garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download with a corrupt state file: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("corrupt state file changed the result: %d bytes, want %d", len(got), len(data))
	}
}

// The app died after the last byte reached the partial but before the rename.
// The next start must finish the file, not download it again.
func TestDownloadCompletePartAfterRestart(t *testing.T) {
	fastRetries(t)
	data := []byte(strings.Repeat("ezlm-model-bytes-", 256))
	srvState, srv := newRestartServer(t, data, `"v1"`)

	dst := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(dst+".part", data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst+".part.state", []byte(`"v1"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Download(context.Background(), srv.URL+"/model.gguf", dst, 0, nil); err != nil {
		t.Fatalf("finishing a complete partial: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("finished file differs: %d bytes, want %d", len(got), len(data))
	}
	if ranges, _ := srvState.requests(); len(ranges) != 1 {
		t.Fatalf("expected one request for a complete partial, got %q", ranges)
	}
}
