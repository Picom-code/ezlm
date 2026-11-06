package engine

import (
	"bytes"
	"context"
	"fmt"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeHF is a small stand-in for huggingface.co and its CDN, built for the
// download fault tests. It serves a body, optionally behind a 302 to a second
// server (like HF redirecting to the CDN), and can be told to misbehave:
// ignore Range, change its ETag, go silent, drip, cut the connection, or serve
// several split parts.
//
//	hf := newFakeHF(t, data)          // starts a primary and a CDN server
//	hf.Redirect = true               // 302 to the CDN, like huggingface.co
//	hf.ResetAt = []int64{1000}       // cut the first response at byte 1000
//	err := Download(ctx, hf.URL("model.gguf"), dst, uint64(len(data)), nil)
//	reqs := hf.Requests()            // what the server saw, in order
//
// Fields are read once per request. Set them before the first request, or use
// SetETag and SetData to change them while a download is running.
type fakeHF struct {
	// Data is the body served for every path when Files is nil.
	Data []byte
	// Files maps a path ("/model-00001-of-00002.gguf") to its body, for split
	// GGUFs and for parts with the wrong size. When set, a path that is not in
	// Files answers 404.
	Files map[string][]byte
	// Missing lists paths that answer 404 even when Data or Files has them.
	Missing []string

	// ETag is served as the ETag header. Empty means one derived from the body.
	ETag string

	// Redirect sends every primary request to the CDN with a 302. RedirectTo
	// overrides the target and is handed the request, the resume offset and
	// the 1-based number of primary requests; returning "" means the CDN.
	// Changing the target on the resume is what a01 needs.
	Redirect   bool
	RedirectTo func(r *http.Request, have int64, hit int) string

	// IgnoreRange answers a Range request with 200 and the whole body.
	IgnoreRange bool

	// StallAfter goes silent after this many body bytes in one response;
	// StallFor says for how long. Zero StallFor means until the client gives
	// up, which is what makes the stall watchdog fire rather than a plain drop.
	StallAfter int64
	StallFor   time.Duration
	// Drip pauses this long before each 1 KiB write: a slow but alive link.
	Drip time.Duration
	// ResetBefore closes the connection before any response bytes, this many times.
	ResetBefore int
	// ResetAt lists absolute file offsets where the connection is cut once, so
	// 0, a mid-body offset and len(Data)-1 all work.
	ResetAt []int64

	mu         sync.Mutex
	primaryHit int
	resets     map[int64]bool
	requests   []fakeHFRequest
	primary    *httptest.Server
	cdn        *httptest.Server
}

// fakeHFRequest is one request the fake saw.
type fakeHFRequest struct {
	Path   string
	Range  string
	Have   int64 // resume offset parsed from Range
	Status int
	ETag   string // ETag the response carried, when it had one
	Target string // redirect target for a 302
}

const fakeHFChunk = 1 << 10

// newFakeHF starts a primary server and a CDN server and closes both when the
// test ends.
func newFakeHF(t *testing.T, data []byte) *fakeHF {
	t.Helper()
	f := &fakeHF{Data: data, resets: map[int64]bool{}}
	f.cdn = httptest.NewServer(http.HandlerFunc(f.serve))
	f.primary = httptest.NewServer(http.HandlerFunc(f.servePrimary))
	t.Cleanup(f.Close)
	return f
}

// URL is the primary URL for a file, the one a caller would download from.
func (f *fakeHF) URL(name string) string { return f.primary.URL + fakeHFPath(name) }

// CDNURL is the CDN URL for a file, the redirect target.
func (f *fakeHF) CDNURL(name string) string { return f.cdn.URL + fakeHFPath(name) }

// Hits is how many requests both servers have seen.
func (f *fakeHF) Hits() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// Requests returns what the servers saw, in order.
func (f *fakeHF) Requests() []fakeHFRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeHFRequest(nil), f.requests...)
}

// SetETag changes the ETag served from the next request on.
func (f *fakeHF) SetETag(etag string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ETag = etag
}

// SetData changes the body served from the next request on.
func (f *fakeHF) SetData(data []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Data = data
}

// Close stops both servers. It is safe to call more than once.
func (f *fakeHF) Close() {
	f.primary.Close()
	f.cdn.Close()
}

func fakeHFPath(name string) string {
	if strings.HasPrefix(name, "/") {
		return name
	}
	return "/" + name
}

func (f *fakeHF) servePrimary(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.primaryHit++
	hit := f.primaryHit
	f.mu.Unlock()

	have := fakeHFRange(r)
	if f.Redirect || f.RedirectTo != nil {
		target := ""
		if f.RedirectTo != nil {
			target = f.RedirectTo(r, have, hit)
		}
		if target == "" {
			target = f.cdn.URL + r.URL.RequestURI()
		}
		f.record(fakeHFRequest{Path: r.URL.Path, Range: r.Header.Get("Range"), Have: have, Status: http.StatusFound, Target: target})
		http.Redirect(w, r, target, http.StatusFound)
		return
	}
	f.serve(w, r)
}

func (f *fakeHF) serve(w http.ResponseWriter, r *http.Request) {
	data, ok := f.body(r.URL.Path)
	if !ok {
		f.record(fakeHFRequest{Path: r.URL.Path, Range: r.Header.Get("Range"), Have: fakeHFRange(r), Status: http.StatusNotFound})
		http.NotFound(w, r)
		return
	}
	etag := f.etag(data)
	have := fakeHFRange(r)
	if have > 0 && f.IgnoreRange {
		have = 0
	}
	if have > 0 {
		// A real CDN restarts when the validator no longer matches.
		if ir := r.Header.Get("If-Range"); ir != "" && ir != etag {
			have = 0
		}
	}

	req := fakeHFRequest{Path: r.URL.Path, Range: r.Header.Get("Range"), Have: fakeHFRange(r), ETag: etag}
	if have >= int64(len(data)) {
		req.Status = http.StatusRequestedRangeNotSatisfiable
		f.record(req)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", len(data)))
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}

	status := http.StatusOK
	if have > 0 {
		status = http.StatusPartialContent
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", have, len(data)-1, len(data)))
	}
	req.Status = status
	f.record(req)

	w.Header().Set("ETag", etag)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)-int(have)))

	if f.takeResetBefore() {
		fakeHFCut(w)
		return
	}
	w.WriteHeader(status)

	from := int(have)
	limit := len(data)
	cut := false
	if off, ok := f.takeReset(from); ok && off <= int64(limit) {
		limit = int(off)
		cut = true
	}
	pos := from
	for pos < limit {
		if f.StallAfter > 0 && int64(pos-from) >= f.StallAfter {
			fakeHFFlush(w)
			f.wait(r)
			return
		}
		if f.Drip > 0 {
			time.Sleep(f.Drip)
		}
		n := min(fakeHFChunk, limit-pos)
		if _, err := w.Write(data[pos : pos+n]); err != nil {
			return
		}
		fakeHFFlush(w)
		pos += n
	}
	if cut {
		fakeHFFlush(w)
		fakeHFCut(w)
	}
}

func (f *fakeHF) wait(r *http.Request) {
	if f.StallFor <= 0 {
		<-r.Context().Done()
		return
	}
	t := time.NewTimer(f.StallFor)
	defer t.Stop()
	select {
	case <-r.Context().Done():
	case <-t.C:
	}
}

func (f *fakeHF) body(p string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.Missing {
		if fakeHFPath(m) == p {
			return nil, false
		}
	}
	if f.Files != nil {
		d, ok := f.Files[p]
		return d, ok
	}
	return f.Data, true
}

func (f *fakeHF) etag(data []byte) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ETag != "" {
		return f.ETag
	}
	return fmt.Sprintf("%q", fmt.Sprintf("fake-%08x", crc32.ChecksumIEEE(data)))
}

func (f *fakeHF) takeResetBefore() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ResetBefore <= 0 {
		return false
	}
	f.ResetBefore--
	return true
}

// takeReset returns the lowest unused offset at or after from and marks it
// used, so a reset fires once and the resume makes progress.
func (f *fakeHF) takeReset(from int) (int64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	best := int64(-1)
	for _, off := range f.ResetAt {
		if f.resets[off] || off < int64(from) {
			continue
		}
		if best < 0 || off < best {
			best = off
		}
	}
	if best < 0 {
		return 0, false
	}
	f.resets[best] = true
	return best, true
}

func (f *fakeHF) record(req fakeHFRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
}

func fakeHFRange(r *http.Request) int64 {
	rg := r.Header.Get("Range")
	if !strings.HasPrefix(rg, "bytes=") {
		return 0
	}
	start := strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-")
	n, err := strconv.ParseInt(start, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func fakeHFFlush(w http.ResponseWriter) {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func fakeHFCut(w http.ResponseWriter) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		return
	}
	conn.Close()
}

// The fake serves a file, honours Range, and follows the HF redirect to its CDN.
func TestFakeHFServesRangeThroughRedirect(t *testing.T) {
	data := bytes.Repeat([]byte("ezlm-model-"), 2048)
	hf := newFakeHF(t, data)
	hf.Redirect = true
	dst := filepath.Join(t.TempDir(), "m.gguf")

	if err := Download(context.Background(), hf.URL("model.gguf"), dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	if got, _ := os.ReadFile(dst); !bytes.Equal(got, data) {
		t.Fatalf("file differs: %d bytes", len(got))
	}
	reqs := hf.Requests()
	if len(reqs) != 2 || reqs[0].Status != http.StatusFound || reqs[1].Status != http.StatusOK {
		t.Fatalf("expected a redirect and one CDN hit, got %+v", reqs)
	}
	if reqs[0].Target == "" || !strings.HasPrefix(reqs[0].Target, hf.cdn.URL) {
		t.Fatalf("redirect target = %q", reqs[0].Target)
	}
}

// A cut in the middle is resumed with a Range request through the redirect,
// and the redirect target can change between the two requests.
func TestFakeHFResumeThroughRedirect(t *testing.T) {
	fakeHFFastRetries(t)
	data := bytes.Repeat([]byte("ezlm-model-"), 2048)
	hf := newFakeHF(t, data)
	hf.Redirect = true
	hf.ResetAt = []int64{int64(len(data) / 2)}
	var targets []string
	hf.RedirectTo = func(r *http.Request, have int64, hit int) string {
		targets = append(targets, fmt.Sprintf("%d:%d", hit, have))
		return ""
	}
	dst := filepath.Join(t.TempDir(), "m.gguf")

	if err := Download(context.Background(), hf.URL("model.gguf"), dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	if got, _ := os.ReadFile(dst); !bytes.Equal(got, data) {
		t.Fatalf("file differs: %d bytes", len(got))
	}
	if len(targets) < 2 || targets[0] != "1:0" || !strings.HasPrefix(targets[1], "2:") {
		t.Fatalf("redirects = %v", targets)
	}
	var resumed bool
	for _, req := range hf.Requests() {
		if req.Status == http.StatusPartialContent && req.Have > 0 {
			resumed = true
		}
	}
	if !resumed {
		t.Fatalf("no resumed request: %+v", hf.Requests())
	}
}

// IgnoreRange answers a resume with the whole body, which the download must
// survive by starting over.
func TestFakeHFIgnoreRange(t *testing.T) {
	fakeHFFastRetries(t)
	data := bytes.Repeat([]byte("ezlm-model-"), 2048)
	hf := newFakeHF(t, data)
	hf.IgnoreRange = true
	hf.ResetAt = []int64{int64(len(data) / 2)}
	dst := filepath.Join(t.TempDir(), "m.gguf")

	if err := Download(context.Background(), hf.URL("model.gguf"), dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	if got, _ := os.ReadFile(dst); !bytes.Equal(got, data) {
		t.Fatalf("file differs: %d bytes", len(got))
	}
	var sawRange, sawFull bool
	for _, req := range hf.Requests() {
		if req.Have > 0 && req.Status == http.StatusOK {
			sawRange, sawFull = true, true
		}
	}
	if !sawRange || !sawFull {
		t.Fatalf("no resume answered 200: %+v", hf.Requests())
	}
}

// A response that goes silent is cut by the watchdog and resumed.
func TestFakeHFStallIsResumed(t *testing.T) {
	fakeHFFastRetries(t)
	old := stallTimeout
	stallTimeout = 250 * time.Millisecond
	defer func() { stallTimeout = old }()

	data := bytes.Repeat([]byte("ezlm-model-"), 2048)
	hf := newFakeHF(t, data)
	hf.StallAfter = int64(len(data) / 2)
	hf.StallFor = 5 * time.Second // longer than the watchdog
	dst := filepath.Join(t.TempDir(), "m.gguf")

	start := time.Now()
	if err := Download(context.Background(), hf.URL("model.gguf"), dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	if got, _ := os.ReadFile(dst); !bytes.Equal(got, data) {
		t.Fatalf("file differs: %d bytes", len(got))
	}
	if hf.Hits() < 2 {
		t.Fatalf("expected a retry after the stall, got %d request(s)", hf.Hits())
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("stall was not cut promptly: %s", time.Since(start))
	}
}

// A slow but alive link is not a stall.
func TestFakeHFDripIsNotAStall(t *testing.T) {
	fakeHFFastRetries(t)
	old := stallTimeout
	stallTimeout = 500 * time.Millisecond
	defer func() { stallTimeout = old }()

	data := bytes.Repeat([]byte("x"), 4*fakeHFChunk) // 4 writes
	hf := newFakeHF(t, data)
	hf.Drip = 100 * time.Millisecond // 100ms between 1 KiB writes, under the watchdog
	dst := filepath.Join(t.TempDir(), "m.gguf")

	if err := Download(context.Background(), hf.URL("model.gguf"), dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	if got, _ := os.ReadFile(dst); !bytes.Equal(got, data) {
		t.Fatalf("file differs: %d bytes", len(got))
	}
	if hf.Hits() != 1 {
		t.Fatalf("slow link was retried %d times", hf.Hits()-1)
	}
}

// Resets at the first byte, mid-body and the last byte all resume cleanly.
func TestFakeHFResetsAtOffsets(t *testing.T) {
	fakeHFFastRetries(t)
	data := bytes.Repeat([]byte("ezlm-model-"), 2048)
	hf := newFakeHF(t, data)
	hf.ResetBefore = 1
	hf.ResetAt = []int64{0, int64(len(data) / 2), int64(len(data) - 1)}
	dst := filepath.Join(t.TempDir(), "m.gguf")

	if err := Download(context.Background(), hf.URL("model.gguf"), dst, uint64(len(data)), nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	if got, _ := os.ReadFile(dst); !bytes.Equal(got, data) {
		t.Fatalf("file differs: %d bytes", len(got))
	}
	if hf.Hits() < 5 {
		t.Fatalf("expected four cuts and a final request, got %d", hf.Hits())
	}
}

// Split parts are served per path, and a missing part answers 404.
func TestFakeHFSplitParts(t *testing.T) {
	first := bytes.Repeat([]byte("a"), 3000)
	second := bytes.Repeat([]byte("b"), 1500)
	hf := newFakeHF(t, nil)
	hf.Files = map[string][]byte{
		"/m-00001-of-00002.gguf": first,
		"/m-00002-of-00002.gguf": second,
	}
	hf.Missing = []string{"/m-00003-of-00002.gguf"}
	dir := t.TempDir()

	for i, part := range []struct {
		name string
		data []byte
	}{{"m-00001-of-00002.gguf", first}, {"m-00002-of-00002.gguf", second}} {
		dst := filepath.Join(dir, strconv.Itoa(i)+".gguf")
		if err := Download(context.Background(), hf.URL(part.name), dst, uint64(len(part.data)), nil); err != nil {
			t.Fatalf("%s: %v", part.name, err)
		}
		if got, _ := os.ReadFile(dst); !bytes.Equal(got, part.data) {
			t.Fatalf("%s differs", part.name)
		}
	}

	err := Download(context.Background(), hf.URL("m-00003-of-00002.gguf"), filepath.Join(dir, "missing.gguf"), 0, nil)
	if err == nil || !strings.Contains(err.Error(), "isn't on the download server") {
		t.Fatalf("missing part: got %v", err)
	}
}

// The ETag can change between requests, for the mid-download change case.
func TestFakeHFSetETag(t *testing.T) {
	data := []byte("ezlm")
	hf := newFakeHF(t, data)
	first := fakeHFGet(t, hf.CDNURL("m.gguf"))
	if first.Header.Get("ETag") == "" {
		t.Fatal("no ETag on the first response")
	}
	hf.SetETag(`"changed"`)
	second := fakeHFGet(t, hf.CDNURL("m.gguf"))
	if got := second.Header.Get("ETag"); got != `"changed"` {
		t.Fatalf("ETag after SetETag = %q", got)
	}
}

func fakeHFGet(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func fakeHFFastRetries(t *testing.T) {
	t.Helper()
	old := retryStep
	retryStep = time.Millisecond
	t.Cleanup(func() { retryStep = old })
}
