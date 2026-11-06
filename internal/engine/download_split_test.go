package engine

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// splitSet serves a fake split GGUF: three parts of equal size, with Range
// support, the way Hugging Face's CDN serves them.
type splitSet struct {
	parts [][]byte
	srv   *httptest.Server
	hits  atomic.Int32
}

func newSplitSet(t *testing.T, n, size int) *splitSet {
	t.Helper()
	s := &splitSet{}
	for k := 0; k < n; k++ {
		b := make([]byte, size)
		for i := range b {
			b[i] = byte('a' + (k+i)%26)
		}
		s.parts = append(s.parts, b)
	}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		var k int
		if _, err := fmt.Sscanf(filepath.Base(r.URL.Path), "m-%05d-of-%05d.gguf", &k, &n); err != nil || k < 1 || k > len(s.parts) {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, filepath.Base(r.URL.Path), time.Time{}, bytes.NewReader(s.parts[k-1]))
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *splitSet) name(k int) string {
	return fmt.Sprintf("m-%05d-of-%05d.gguf", k, len(s.parts))
}

func (s *splitSet) url(k int) string { return s.srv.URL + "/" + s.name(k) }

// downloadAll fetches every part, as DownloadModel does for a split quant.
func (s *splitSet) downloadAll(t *testing.T, dir string, want bool) {
	t.Helper()
	for k := range s.parts {
		var w uint64
		if want {
			w = uint64(len(s.parts[k]))
		}
		dst := filepath.Join(dir, s.name(k+1))
		if err := Download(context.Background(), s.url(k+1), dst, w, nil); err != nil {
			t.Fatalf("part %d: %v", k+1, err)
		}
	}
}

func (s *splitSet) checkAll(t *testing.T, dir string) {
	t.Helper()
	for k := range s.parts {
		got, err := os.ReadFile(filepath.Join(dir, s.name(k+1)))
		if err != nil {
			t.Fatalf("part %d: %v", k+1, err)
		}
		if !bytes.Equal(got, s.parts[k]) {
			t.Fatalf("part %d is wrong: %d bytes, want %d", k+1, len(got), len(s.parts[k]))
		}
	}
}

// Parts already on disk are not fetched again, whatever order they arrived in.
func TestDownloadSplitAllPartsPresent(t *testing.T) {
	s := newSplitSet(t, 3, 512)
	dir := t.TempDir()
	for _, k := range []int{3, 1, 2} { // out of order
		dst := filepath.Join(dir, s.name(k))
		if err := Download(context.Background(), s.url(k), dst, uint64(len(s.parts[k-1])), nil); err != nil {
			t.Fatalf("part %d: %v", k, err)
		}
	}
	s.checkAll(t, dir)

	s.hits.Store(0)
	s.downloadAll(t, dir, true)
	if n := s.hits.Load(); n != 0 {
		t.Fatalf("complete parts were fetched again: %d requests", n)
	}
	s.checkAll(t, dir)
}

// A missing part is the only one fetched; the others stay untouched.
func TestDownloadSplitMissingPart(t *testing.T) {
	s := newSplitSet(t, 3, 512)
	dir := t.TempDir()
	for _, k := range []int{1, 3} {
		if err := Download(context.Background(), s.url(k), filepath.Join(dir, s.name(k)), uint64(len(s.parts[k-1])), nil); err != nil {
			t.Fatalf("part %d: %v", k, err)
		}
	}
	s.hits.Store(0)
	if err := Download(context.Background(), s.url(2), filepath.Join(dir, s.name(2)), uint64(len(s.parts[1])), nil); err != nil {
		t.Fatalf("missing part: %v", err)
	}
	if n := s.hits.Load(); n != 1 {
		t.Fatalf("expected one request for the missing part, got %d", n)
	}
	s.checkAll(t, dir)
}

// A stale .part left by an older, larger file is bigger than the part the
// server has now. Resuming from past its end gets 416; the old bytes must be
// thrown away and the part fetched from the start, not accepted as complete.
func TestDownloadSplitPartWrongSize(t *testing.T) {
	for _, wantSize := range []bool{false, true} {
		t.Run(fmt.Sprintf("want=%v", wantSize), func(t *testing.T) {
			s := newSplitSet(t, 3, 400)
			dir := t.TempDir()
			dst := filepath.Join(dir, s.name(2))
			stale := bytes.Repeat([]byte("x"), len(s.parts[1])+50)
			if err := os.WriteFile(dst+".part", stale, 0o644); err != nil {
				t.Fatal(err)
			}
			var want uint64
			if wantSize {
				want = uint64(len(s.parts[1]))
			}
			if err := Download(context.Background(), s.url(2), dst, want, nil); err != nil {
				t.Fatalf("stale oversized part: %v", err)
			}
			got, err := os.ReadFile(dst)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, s.parts[1]) {
				t.Fatalf("part kept the stale bytes: %d bytes, want %d", len(got), len(s.parts[1]))
			}
		})
	}
}

// A part cut short by a dropped connection resumes from where it stopped.
func TestDownloadSplitPartTruncatedResume(t *testing.T) {
	s := newSplitSet(t, 3, 400)
	dir := t.TempDir()
	dst := filepath.Join(dir, s.name(3))
	if err := os.WriteFile(dst+".part", s.parts[2][:150], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Download(context.Background(), s.url(3), dst, 0, nil); err != nil {
		t.Fatalf("resume: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, s.parts[2]) {
		t.Fatalf("resumed part is wrong: %d bytes, want %d", len(got), len(s.parts[2]))
	}
}
