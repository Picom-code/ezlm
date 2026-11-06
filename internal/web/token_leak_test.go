package web

import (
	"context"

	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/app"
	"github.com/picom-code/ezlm/internal/report"
	"github.com/picom-code/ezlm/internal/scan"
	"github.com/picom-code/ezlm/internal/update"
)

// fakeBackend_tokenleak is the smallest thing that satisfies Backend, so the tests
// exercise the HTTP layer only and never touch the real app, disk or network.
type fakeBackend_tokenleak struct{}

func (fakeBackend_tokenleak) Status(context.Context) app.Status                 { return app.Status{} }
func (fakeBackend_tokenleak) Scan(context.Context, string) *app.ScanResult      { return &app.ScanResult{} }
func (fakeBackend_tokenleak) Deploy(string) error                               { return nil }
func (fakeBackend_tokenleak) Kill(context.Context, int32) error                 { return nil }
func (fakeBackend_tokenleak) SetLAN(bool) error                                 { return nil }
func (fakeBackend_tokenleak) Integrate([]string, bool) error                    { return nil }
func (fakeBackend_tokenleak) AddKey(string) error                               { return nil }
func (fakeBackend_tokenleak) RotateKey(string) error                            { return nil }
func (fakeBackend_tokenleak) RevokeKey(string) error                            { return nil }
func (fakeBackend_tokenleak) Stop() error                                       { return nil }
func (fakeBackend_tokenleak) Start() error                                      { return nil }
func (fakeBackend_tokenleak) Uninstall() error                                  { return nil }
func (fakeBackend_tokenleak) Test() error                                       { return nil }
func (fakeBackend_tokenleak) Cancel() error                                     { return nil }
func (fakeBackend_tokenleak) Use(context.Context, string, string, string) error { return nil }
func (fakeBackend_tokenleak) Remove(context.Context, string, string) error      { return nil }
func (fakeBackend_tokenleak) Logs(int) []string                                 { return nil }
func (fakeBackend_tokenleak) Apps(context.Context) []scan.App                   { return nil }
func (fakeBackend_tokenleak) DownloadUpdate() error                             { return nil }
func (fakeBackend_tokenleak) SetStorage(string, bool) error                     { return nil }
func (fakeBackend_tokenleak) Launch(string, string) error                       { return nil }
func (fakeBackend_tokenleak) IntegrationLog(string, int) []string               { return nil }
func (fakeBackend_tokenleak) Installed(context.Context, string) []app.InstalledModel {
	return nil
}
func (fakeBackend_tokenleak) UpdateInfo(context.Context, bool) update.Info { return update.Info{} }
func (fakeBackend_tokenleak) Storage(context.Context) app.StorageInfo      { return app.StorageInfo{} }
func (fakeBackend_tokenleak) Live(context.Context) app.Live                { return app.Live{} }
func (fakeBackend_tokenleak) Subscribe() (chan app.Event, func()) {
	ch := make(chan app.Event)
	return ch, func() {}
}
func (fakeBackend_tokenleak) Report(context.Context, string, bool, string) (report.Result, error) {
	return report.Result{}, nil
}
func (fakeBackend_tokenleak) CloseApp(context.Context, string, bool) (scan.CloseResult, error) {
	return scan.CloseResult{}, nil
}

// startAPI serves the engine API on a loopback port and returns its base URL
// and the per-launch token from the page URL, the way Listen hands them out.
func startAPI(t *testing.T) (base, token string) {
	t.Helper()
	s, l, pageURL, err := Listen(fakeBackend_tokenleak{}, nil)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go s.Serve(l)
	t.Cleanup(func() { l.Close() })
	u, err := url.Parse(pageURL)
	if err != nil {
		t.Fatalf("parse %q: %v", pageURL, err)
	}
	if got := u.Query().Get("t"); got == "" {
		t.Fatalf("page URL %q carries no token", pageURL)
	}
	return "http://" + u.Host, u.Query().Get("t")
}

func do_tokenleak(t *testing.T, method, target, token string, body io.Reader) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if token != "" {
		req.Header.Set("X-EZLM-Token", token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, string(b)
}

// The page URL carries the token in its query string. That string ends up in
// browser history, in Referer headers and in anything that copies the URL, so
// it must not also be a full API credential: a leaked URL should at most open
// the read-only event stream. Every other call has to present the header.
func TestTokenInQueryOnlyOpensTheEventStream(t *testing.T) {
	base, token := startAPI(t)

	// A GET that needs a token, called with the query form only.
	resp, _ := do_tokenleak(t, http.MethodGet, base+"/api/status?t="+token, "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET /api/status?t=TOKEN = %d, want 401 (header only)", resp.StatusCode)
	}

	// The dangerous one: a state-changing POST.
	resp, _ = do_tokenleak(t, http.MethodPost, base+"/api/deploy?t="+token, "", strings.NewReader(`{"model_id":"x"}`))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("POST /api/deploy?t=TOKEN = %d, want 401 (header only)", resp.StatusCode)
	}

	// The same calls with the header still work.
	resp, _ = do_tokenleak(t, http.MethodGet, base+"/api/status", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /api/status with header = %d, want 200", resp.StatusCode)
	}
	resp, _ = do_tokenleak(t, http.MethodPost, base+"/api/deploy", token, strings.NewReader(`{"model_id":"x"}`))
	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST /api/deploy with header = %d, want 200", resp.StatusCode)
	}
}

// EventSource cannot set request headers, so the event stream is the one
// place where the query form has to keep working.
func TestEventStreamAcceptsQueryToken(t *testing.T) {
	base, token := startAPI(t)
	resp, err := http.Get(base + "/api/events?t=" + token)
	if err != nil {
		t.Fatalf("GET /api/events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/events?t=TOKEN = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
}

// A header always wins over the query, so a stale URL token in the address
// bar cannot override the header a client sends.
func TestHeaderWinsOverQuery(t *testing.T) {
	base, token := startAPI(t)
	resp, _ := do_tokenleak(t, http.MethodGet, base+"/api/status?t=stale", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("right header with a stale query = %d, want 200", resp.StatusCode)
	}
	resp, _ = do_tokenleak(t, http.MethodGet, base+"/api/status?t="+token, "wrong", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong header with a right query = %d, want 401", resp.StatusCode)
	}
}

// With no Referrer-Policy the browser decides what to send, and the old
// default (no-referrer-when-downgrade) puts the whole page URL, token
// included, in the Referer of every external link. Pin it to no-referrer.
func TestResponsesForbidReferer(t *testing.T) {
	base, token := startAPI(t)
	for _, tc := range []struct{ name, target, hdr string }{
		{"document", base + "/?t=" + token, ""},
		{"api", base + "/api/status", token},
		{"error page", base + "/api/status", "wrong"},
	} {
		resp, _ := do_tokenleak(t, http.MethodGet, tc.target, tc.hdr, nil)
		if got := resp.Header.Get("Referrer-Policy"); got != "no-referrer" {
			t.Errorf("%s: Referrer-Policy = %q, want no-referrer", tc.name, got)
		}
	}
}

// A stale or wrong token must not be echoed back on the error page: the page
// is shown in a browser, and a shared screenshot or a report would carry it.
func TestErrorPagesDoNotEchoTheToken(t *testing.T) {
	base, token := startAPI(t)
	for _, target := range []string{
		base + "/api/status?t=" + token,
		base + "/api/status?t=not-the-token",
		base + "/api/nope?t=" + token,
	} {
		resp, body := do_tokenleak(t, http.MethodGet, target, "", nil)
		if strings.Contains(body, token) || strings.Contains(body, "not-the-token") {
			t.Errorf("%s: error body echoes the token: %q", target, body)
		}
		for k, vs := range resp.Header {
			for _, v := range vs {
				if strings.Contains(v, token) {
					t.Errorf("%s: header %s echoes the token", target, k)
				}
			}
		}
	}
}

// The page URL holds the token, so nothing may cache the document or an API
// reply and replay it later, from another tab or after a restart.
func TestTokenBearingRepliesAreNotCached(t *testing.T) {
	base, token := startAPI(t)
	for _, tc := range []struct{ target, hdr string }{
		{base + "/?t=" + token, ""},
		{base + "/api/status", token},
	} {
		resp, _ := do_tokenleak(t, http.MethodGet, tc.target, tc.hdr, nil)
		if got := resp.Header.Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: Cache-Control = %q, want no-store", tc.target, got)
		}
	}
}

// The document is served with the token still in the URL. Nothing in the
// server strips it, so it stays in the address bar and in browser history.
// This test documents the current behaviour: it is the leak the report
// proposes to close in the page script.
func TestDocumentKeepsTheTokenInTheURL(t *testing.T) {
	base, token := startAPI(t)
	resp, err := http.Get(base + "/?t=" + token)
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /?t=TOKEN = %d, want 200", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "" {
		t.Fatalf("document redirects to %q: the test's assumption changed", loc)
	}
}
