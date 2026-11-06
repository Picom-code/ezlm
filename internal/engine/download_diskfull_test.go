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
	"syscall"
	"testing"
)

// fullDisk fakes a disk that runs out of space after remain bytes and stays
// full for every later open, like a real one.
type fullDisk struct {
	remain int64
}

func (d *fullDisk) open(name string, flags int, perm os.FileMode) (partFile, error) {
	f, err := os.OpenFile(name, flags, perm)
	if err != nil {
		return nil, err
	}
	return &fullDiskFile{f: f, disk: d}, nil
}

type fullDiskFile struct {
	f    *os.File
	disk *fullDisk
}

func (w *fullDiskFile) Write(p []byte) (int, error) {
	if w.disk.remain <= 0 {
		return 0, &os.PathError{Op: "write", Path: w.f.Name(), Err: syscall.ENOSPC}
	}
	if int64(len(p)) > w.disk.remain {
		p = p[:w.disk.remain]
	}
	n, err := w.f.Write(p)
	w.disk.remain -= int64(n)
	if err == nil && w.disk.remain == 0 {
		err = &os.PathError{Op: "write", Path: w.f.Name(), Err: syscall.ENOSPC}
	}
	return n, err
}

func (w *fullDiskFile) Close() error { return w.f.Close() }

// diskFullRangeServer serves data and honours Range requests, like a CDN.
func diskFullRangeServer(t *testing.T, data []byte, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
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

// A disk that fills up partway through the model: the download fails with a
// plain message, the half-written .part file is removed, the previous model
// file is untouched, and a full disk is not retried five times.
func TestDownloadDiskFullPartway(t *testing.T) {
	fastRetries(t)
	data := []byte(strings.Repeat("ezlm-model-bytes-", 200000)) // ~3.4 MB
	var hits atomic.Int32
	srv := diskFullRangeServer(t, data, &hits)

	dir := t.TempDir()
	dst := filepath.Join(dir, "model.gguf")
	old := []byte("the previous model, which must survive a failed download")
	if err := os.WriteFile(dst, old, 0o644); err != nil {
		t.Fatal(err)
	}

	disk := &fullDisk{remain: 1 << 20} // fills up after the first megabyte
	oldOpen := openPart
	openPart = disk.open
	t.Cleanup(func() { openPart = oldOpen })

	err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(data)), nil)
	if err == nil {
		t.Fatal("download succeeded on a full disk")
	}
	if !strings.Contains(err.Error(), "free space") || !errors.Is(err, syscall.ENOSPC) {
		t.Errorf("error doesn't explain the full disk: %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != string(old) {
		t.Errorf("the old file was damaged: %q", got)
	}
	if _, statErr := os.Stat(dst + ".part"); !os.IsNotExist(statErr) {
		t.Errorf("the partial file was left behind: %v", statErr)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("a full disk was retried %d times", n)
	}
}

// A disk too full to even create the .part file fails the same way.
func TestDownloadDiskFullAtOpen(t *testing.T) {
	fastRetries(t)
	data := []byte(strings.Repeat("ezlm-model-bytes-", 2000))
	var hits atomic.Int32
	srv := diskFullRangeServer(t, data, &hits)

	dir := t.TempDir()
	dst := filepath.Join(dir, "model.gguf")
	old := []byte("the previous model")
	if err := os.WriteFile(dst, old, 0o644); err != nil {
		t.Fatal(err)
	}

	oldOpen := openPart
	openPart = func(name string, flags int, perm os.FileMode) (partFile, error) {
		return nil, &os.PathError{Op: "open", Path: name, Err: syscall.ENOSPC}
	}
	t.Cleanup(func() { openPart = oldOpen })

	err := Download(context.Background(), srv.URL+"/model.gguf", dst, uint64(len(data)), nil)
	if err == nil {
		t.Fatal("download succeeded on a full disk")
	}
	if !strings.Contains(err.Error(), "free space") || !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("error doesn't explain the full disk: %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != string(old) {
		t.Fatalf("the old file was damaged: %q", got)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("a full disk was retried %d times", n)
	}
}
