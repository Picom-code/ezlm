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

// pathServer serves data with Range support and remembers the request paths it
// saw, so a test can prove the name survived the trip through the URL.
func pathServer(t *testing.T, data []byte, seen *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			*seen = append(*seen, r.URL.Path)
		}
		from := 0
		if rg := r.Header.Get("Range"); rg != "" {
			from, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(data)-1, len(data)))
			w.Header().Set("Content-Length", strconv.Itoa(len(data)-from))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		}
		w.Write(data[from:])
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A model name that isn't ASCII, with spaces in it, and a model folder that
// does not exist yet: the first download must create the folder and land the
// bytes under the exact name.
func TestDownloadNonASCIINameInNewFolder(t *testing.T) {
	data := []byte(strings.Repeat("ezlm-模型-bytes-", 512))
	var seen []string
	srv := pathServer(t, data, &seen)

	const name = "Qwen2.5 7B 日本語 モデル v1.gguf"
	dst := filepath.Join(t.TempDir(), "模型 目录", name)
	if err := Download(context.Background(), srv.URL+"/"+name, dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download to %q: %v", dst, err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read %q: %v", dst, err)
	}
	if string(got) != string(data) {
		t.Fatalf("file differs: %d bytes, want %d", len(got), len(data))
	}
	if len(seen) != 1 || seen[0] != "/"+name {
		t.Fatalf("server saw %q, want the decoded name", seen)
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Fatalf("part file left behind after a clean download: %v", err)
	}
}

// A resume must work when the URL and the destination are non-ASCII: the Range
// request has to reach the same file the first request started.
func TestDownloadNonASCIIResume(t *testing.T) {
	fastRetries(t)
	data := []byte(strings.Repeat("0123456789模型", 400))
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		from := 0
		if rg := r.Header.Get("Range"); rg != "" {
			from, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"))
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(data)-1, len(data)))
			w.Header().Set("Content-Length", strconv.Itoa(len(data)-from))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		}
		if n == 1 { // cut the connection halfway through
			w.Write(data[:len(data)/2])
			w.(http.Flusher).Flush()
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		w.Write(data[from:])
	}))
	defer srv.Close()

	const name = "Mistral 小模型 Q4_K_M.gguf"
	dst := filepath.Join(t.TempDir(), "模型 目录", name)
	if err := Download(context.Background(), srv.URL+"/"+name, dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != string(data) {
		t.Fatalf("file differs after resume: %d bytes, want %d", len(got), len(data))
	}
	if hits.Load() != 2 {
		t.Fatalf("expected one cut and one resumed request, got %d", hits.Load())
	}
}

// Windows stops at 260 characters unless a path is handled as a long path, and
// model folders nest model name plus file name. A long path, partly non-ASCII
// and with spaces, must download the same as a short one.
func TestDownloadLongPath(t *testing.T) {
	data := []byte(strings.Repeat("long-path-bytes-", 256))
	srv := pathServer(t, data, nil)

	dir := t.TempDir()
	for len(dir) < 300 {
		dir = filepath.Join(dir, "models 模型", "a-rather-long-model-folder-name")
	}
	dst := filepath.Join(dir, "Model 7B Q4_K_M.gguf")
	if len(dst) < 260 {
		t.Fatalf("test path is only %d characters: %q", len(dst), dst)
	}
	if err := Download(context.Background(), srv.URL+"/Model.gguf", dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download to a %d character path: %v", len(dst), err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read %q: %v", dst, err)
	}
	if string(got) != string(data) {
		t.Fatalf("file differs: %d bytes, want %d", len(got), len(data))
	}
}

// A path this computer can't create is this computer's problem, not the
// network's: it must fail at once instead of being retried like a dropped
// connection, and the message must name the path.
func TestDownloadUnwritableFolderFailsFast(t *testing.T) {
	srv := pathServer(t, []byte("x"), nil)

	dir := t.TempDir()
	blocked := filepath.Join(dir, "模型 目录")
	if err := os.WriteFile(blocked, []byte("not a folder"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(blocked, "m.gguf")
	t0 := time.Now()
	err := Download(context.Background(), srv.URL+"/m.gguf", dst, 1, nil)
	if err == nil {
		t.Fatal("expected a failure when the model folder is a file")
	}
	if !strings.Contains(err.Error(), blocked) {
		t.Fatalf("error does not name the path: %v", err)
	}
	if d := time.Since(t0); d > 2*time.Second {
		t.Fatalf("took %s: a local folder problem was retried like a dropped connection", d)
	}
}
