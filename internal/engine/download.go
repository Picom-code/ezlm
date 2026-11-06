package engine

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// stallTimeout: a download that receives nothing for this long is cut and
// resumed. A connection can stay open while the server sends nothing (seen
// with Hugging Face's CDN); without this the download would wait forever.
var stallTimeout = 45 * time.Second

// dlClient gives up on servers that don't connect or answer in time; the
// stall watchdog in fetch covers transfers that stop midway.
var dlClient = &http.Client{Transport: &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	TLSHandshakeTimeout:   20 * time.Second,
	ResponseHeaderTimeout: 60 * time.Second,
	IdleConnTimeout:       90 * time.Second,
	ForceAttemptHTTP2:     true,
}}

var errStalled = errors.New("the download stopped receiving data")

// errStalePart: the bytes on disk came from a different version of the file.
var errStalePart = errors.New("the partial download is from an older version of the file")

// retryStep paces the retries after a failed attempt (2, 4, 6, 8 steps).
var retryStep = time.Second

// Progress reports download progress. Total is 0 when unknown.
type Progress func(done, total uint64, bytesPerSec float64)

// dlLocks serialises downloads that write the same destination file. Two
// callers asking for one model at the same time would otherwise truncate and
// append to the same .part file and both try to rename it.
var (
	dlLocksMu sync.Mutex
	dlLocks   = map[string]*dlLock{}
)

type dlLock struct {
	sem  chan struct{} // one token: held by the caller doing the download
	refs int           // callers using the entry, held or waiting
}

// lockDownload blocks until the caller may write dst. The returned release
// must be called. A caller whose context ends while waiting gives up without
// disturbing the download in progress.
func lockDownload(ctx context.Context, dst string) (release func(), err error) {
	key, err := filepath.Abs(dst)
	if err != nil {
		key = dst
	}
	dlLocksMu.Lock()
	l := dlLocks[key]
	if l == nil {
		l = &dlLock{sem: make(chan struct{}, 1)}
		dlLocks[key] = l
	}
	l.refs++
	dlLocksMu.Unlock()

	drop := func() {
		dlLocksMu.Lock()
		if l.refs--; l.refs == 0 {
			delete(dlLocks, key)
		}
		dlLocksMu.Unlock()
	}
	select {
	case l.sem <- struct{}{}:
		return func() { <-l.sem; drop() }, nil
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	}
}

// Download fetches url to dst, resuming from dst+".part" if present.
// If want > 0 the final size must match exactly.
func Download(ctx context.Context, url, dst string, want uint64, prog Progress) error {
	release, err := lockDownload(ctx, dst)
	if err != nil {
		return err
	}
	defer release()

	if st, err := os.Stat(dst); err == nil && (want == 0 || uint64(st.Size()) == want) {
		if prog != nil {
			prog(uint64(st.Size()), uint64(st.Size()), 0)
		}
		return nil
	}
	part := dst + ".part"
	// A model name can be new to this computer (and can hold spaces or other
	// scripts), so its folder may not exist yet. Creating it here keeps the
	// path rules in one place instead of in every caller.
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	var have int64
	if st, err := os.Stat(part); err == nil {
		have = st.Size()
	}
	if want > 0 && uint64(have) > want {
		// A partial left by an interrupted download of a larger file.
		os.Remove(part)
		os.Remove(statePath(part))
		have = 0
	}

	etag := readState(part)
	var restarts int
	var restarted bool
	for fails := 0; ; {
		// The last body byte can reach the part file before the connection
		// drops; if the part is already complete, finish instead of retrying.
		if want > 0 && uint64(have) == want {
			break
		}
		next, err := fetch(ctx, url, part, have, want, prog, etag)
		etag = next
		if err == nil {
			if want > 0 {
				st, err := os.Stat(part)
				if err != nil {
					return err
				}
				if uint64(st.Size()) != want {
					// An oversized part is worth one clean restart; any other
					// mismatch means the server can't deliver the size it
					// promised, so the bytes are no good.
					if uint64(st.Size()) > want && restarts < 2 {
						restarts++
						if err := os.Truncate(part, 0); err != nil {
							return err
						}
						have = 0
						continue
					}
					os.Remove(part)
					return fmt.Errorf("the downloaded file came out the wrong size (%d bytes instead of %d), so EZLM deleted it. Try again, and if it happens again, report this problem", st.Size(), want)
				}
			}
			break
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, errStalePart) && !restarted {
			// The file on the server isn't the one the partial came from:
			// drop the stale bytes and start over, once.
			restarted = true
			os.Remove(part)
			os.Remove(statePath(part))
			have = 0
			continue
		}
		var p permanent
		if errors.As(err, &p) {
			return err // retrying can't fix a missing or forbidden file
		}
		var d diskErr
		if errors.As(err, &d) {
			// A full or unwritable disk won't fix itself while we retry, and
			// the half-written file would only take more space.
			os.Remove(part)
			return saveErr(err)
		}
		// A connection that drops now and then but keeps delivering never runs
		// out of retries: only five failures in a row without progress give up.
		prev := have
		if st, err := os.Stat(part); err == nil {
			have = st.Size()
		}
		if have > prev {
			fails = 0
		}
		// Tell the caller that data has stopped moving before the wait. Without
		// this a stall or a dropped connection looks like a download frozen at
		// its last speed and ETA; the zero speed makes the apps drop both.
		if prog != nil {
			prog(uint64(have), want, 0)
		}
		if fails++; fails >= 5 {
			return plainNetErr(err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(fails*2) * retryStep):
		}
	}
	if want > 0 {
		st, err := os.Stat(part)
		if err != nil {
			return err
		}
		if uint64(st.Size()) != want {
			os.Remove(part)
			os.Remove(statePath(part))
			return fmt.Errorf("the downloaded file came out the wrong size (%d bytes instead of %d), so EZLM deleted it. Try again, and if it happens again, report this problem", st.Size(), want)
		}
	}
	os.Remove(statePath(part))
	return os.Rename(part, dst)
}

// fetch transfers one response into part and returns the file's ETag, if the
// server sent one. The tag is empty when the server has no validator.
func fetch(ctx context.Context, url, part string, have int64, want uint64, prog Progress, etag string) (string, error) {
	// The watchdog cancels this request (not the caller's job) when nothing
	// arrives for stallTimeout; Download then retries and resumes. The timeout
	// is read once so a test that changes it can't race with the goroutine.
	stall := stallTimeout
	rctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var last atomic.Int64
	last.Store(time.Now().UnixNano())
	var stalled atomic.Bool
	go func() {
		t := time.NewTicker(stall / 9)
		defer t.Stop()
		for {
			select {
			case <-rctx.Done():
				return
			case <-t.C:
				if time.Since(time.Unix(0, last.Load())) > stall {
					stalled.Store(true)
					cancel()
					return
				}
			}
		}
	}()
	req, err := http.NewRequestWithContext(rctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "ezlm")
	saved := ""
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
		if saved = readState(part); saved != "" {
			// If the server's file changed since the partial was written, it
			// sends the whole file instead of the rest of the old one.
			req.Header.Set("If-Range", saved)
		}
	}
	resp, err := dlClient.Do(req)
	if err != nil {
		if stalled.Load() {
			return "", errStalled
		}
		return "", err
	}
	defer resp.Body.Close()
	// The headers arriving counts as activity: the body gets its own
	// stallTimeout. Otherwise a slow handshake and a slow first byte each
	// under the limit add up to a stall that never happened.
	last.Store(time.Now().UnixNano())

	got := resp.Header.Get("ETag")
	flags := os.O_CREATE | os.O_WRONLY
	switch resp.StatusCode {
	case http.StatusPartialContent:
		// A server can answer a Range request with 206 and still send the
		// whole file from byte 0. Appending that to the part file would
		// corrupt it, so start the file over.
		start, ok := contentRangeStart(resp.Header.Get("Content-Range"))
		if ok && start != have {
			if start != 0 {
				file := path.Base(strings.SplitN(url, "?", 2)[0])
				return "", permanent{fmt.Errorf("the download server sent %s from byte %d when EZLM asked for byte %d, so EZLM can't resume it. Try again, and if it happens again, report this problem", file, start, have)}
			}
			have = 0
			flags |= os.O_TRUNC
			break
		}
		flags |= os.O_APPEND
	case http.StatusOK:
		have = 0
		flags |= os.O_TRUNC
	case http.StatusRequestedRangeNotSatisfiable:
		// The server says the partial already covers the file, or that it is
		// longer than the file now is. Only the first is a finished download.
		if total, ok := contentRangeTotal(resp); ok {
			if uint64(have) == total {
				return got, nil
			}
		} else if want > 0 && uint64(have) == want {
			return got, nil
		}
		return got, errStalePart
	default:
		return got, statusError(resp, url)
	}

	if resp.StatusCode == http.StatusPartialContent && saved != "" {
		if v := validator(resp); v != "" && v != saved {
			return got, errStalePart
		}
	}

	total := want
	if total == 0 && resp.ContentLength > 0 {
		total = uint64(have + resp.ContentLength)
	}
	f, err := openPart(part, flags, 0o644)
	if err != nil {
		return got, diskErr{err}
	}
	defer f.Close()
	// Remember which revision the bytes on disk belong to, so a resume after
	// the app restarts can tell when the server's file has changed.
	writeState(part, validator(resp))

	buf := make([]byte, 1<<20)
	done := uint64(have)
	start, lastReport := time.Now(), time.Time{}
	var sinceStart uint64
	// rate is this attempt's speed so far. Zero when no time has passed yet,
	// so a caller never sees an infinite speed (which JSON can't carry).
	rate := func() float64 {
		if el := time.Since(start).Seconds(); el > 0 {
			return float64(sinceStart) / el
		}
		return 0
	}
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			last.Store(time.Now().UnixNano())
			if _, err := f.Write(buf[:n]); err != nil {
				return got, diskErr{err}
			}
			done += uint64(n)
			sinceStart += uint64(n)
			if prog != nil && time.Since(lastReport) > 200*time.Millisecond {
				lastReport = time.Now()
				prog(done, total, rate())
			}
		}
		if rerr == io.EOF {
			if prog != nil {
				prog(done, total, rate())
			}
			return got, nil
		}
		if rerr != nil {
			if stalled.Load() {
				return got, errStalled
			}
			return got, dropped{rerr}
		}
	}
}

// statePath is the sidecar that names the file revision a partial belongs to.
func statePath(part string) string { return part + ".state" }

// readState returns the saved revision, or "" when there is none to trust.
func readState(part string) string {
	b, err := os.ReadFile(statePath(part))
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(string(b))
	if v == "" || len(v) > 256 || strings.ContainsAny(v, "\r\n") || strings.ContainsFunc(v, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return ""
	}
	return v
}

func writeState(part, v string) {
	if v == "" {
		os.Remove(statePath(part))
		return
	}
	os.WriteFile(statePath(part), []byte(v+"\n"), 0o644)
}

// validator names the version of the file a response carries.
func validator(resp *http.Response) string {
	if v := resp.Header.Get("ETag"); v != "" {
		return v
	}
	return resp.Header.Get("Last-Modified")
}

// contentRangeTotal reads the file's full size from a "bytes */N" header.
func contentRangeTotal(resp *http.Response) (uint64, bool) {
	v := resp.Header.Get("Content-Range")
	i := strings.LastIndex(v, "/")
	if i < 0 {
		return 0, false
	}
	n, err := strconv.ParseUint(strings.TrimSpace(v[i+1:]), 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// permanent marks download errors that retrying can't fix: the server says
// the file is missing or off-limits, or this computer won't allow the write.
type permanent struct{ error }

func (p permanent) Unwrap() error { return p.error }

// dropped marks a connection that broke in the middle of a transfer.
type dropped struct{ error }

func (d dropped) Unwrap() error { return d.error }

// diskErr marks a failure to save the model on disk. Retrying the transfer
// can't fix it, and the partial file should not be left behind.
type diskErr struct{ error }

func (d diskErr) Unwrap() error { return d.error }

// saveErr explains a failure to save the model in plain words.
func saveErr(err error) error {
	if errors.Is(err, syscall.ENOSPC) {
		return fmt.Errorf("there isn't enough free space on this computer to save the model. Free up some space and try again (%w)", err)
	}
	return fmt.Errorf("EZLM couldn't save the model on this computer. Check that the folder is writable and try again (%w)", err)
}

// plainNetErr explains a connection problem in plain words. The original
// error stays wrapped for callers that look for a specific one.
func plainNetErr(err error) error {
	const again = "Check the internet connection and try again: EZLM picks up where it stopped"
	var (
		dns       *net.DNSError
		op        *net.OpError
		ne        net.Error
		cut       dropped
		invalid   x509.CertificateInvalidError
		unknownCA x509.UnknownAuthorityError
		hostname  x509.HostnameError
	)
	switch {
	case errors.Is(err, errStalePart):
		return fmt.Errorf("the file on the download server changed while it was being downloaded. Try again: EZLM starts the download over (%w)", err)
	case errors.Is(err, errStalled):
		return fmt.Errorf("the download keeps stalling (no data arrives for a long time). %s (%w)", again, err)
	case errors.As(err, &dns):
		return fmt.Errorf("this computer can't find the download server (%s), so it looks offline. %s (%w)", dns.Name, again, err)
	case errors.As(err, &invalid) && invalid.Reason == x509.Expired:
		return fmt.Errorf("the download server's security certificate looks expired, which usually means this computer's date and time are wrong. Set the clock right in your system settings and try again (%w)", err)
	case errors.As(err, &unknownCA), errors.As(err, &hostname), errors.As(err, &invalid):
		return fmt.Errorf("something on this network (a security app, antivirus or company proxy) is intercepting secure connections, so the download can't be trusted. Try another network, or allow huggingface.co and github.com in that app (%w)", err)
	case errors.As(err, &cut), errors.As(err, &op), errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF), errors.As(err, &ne) && ne.Timeout():
		return fmt.Errorf("the connection to the download server keeps dropping. %s (%w)", again, err)
	}
	return err
}

// contentRangeStart reads the first byte offset from a Content-Range header
// such as "bytes 100-199/200". It reports false when the header is missing or
// doesn't parse.
func contentRangeStart(v string) (int64, bool) {
	v = strings.TrimSpace(strings.TrimPrefix(v, "bytes"))
	v = strings.TrimSpace(strings.TrimPrefix(v, "="))
	dash := strings.IndexByte(v, '-')
	if dash <= 0 {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(v[:dash]), 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// statusError explains a failed download in plain words.
func statusError(resp *http.Response, url string) error {
	file := path.Base(strings.SplitN(url, "?", 2)[0])
	switch c := resp.StatusCode; {
	case c == http.StatusNotFound || c == http.StatusGone:
		return permanent{fmt.Errorf("%s isn't on the download server any more (it may have been moved or renamed). Pick another model, or report this problem", file)}
	case c == http.StatusUnauthorized || c == http.StatusForbidden:
		return permanent{fmt.Errorf("the download server won't hand out %s (it may now need an account). Pick another model, or report this problem", file)}
	case c >= 400 && c < 500 && c != http.StatusRequestTimeout && c != http.StatusTooManyRequests:
		return permanent{fmt.Errorf("downloading %s failed: %s", file, resp.Status)}
	}
	return fmt.Errorf("the download server is having trouble (%s). Try again in a few minutes: EZLM resumes where it stopped", resp.Status)
}
