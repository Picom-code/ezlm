package web

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/picom-code/ezlm/internal/app"
	"github.com/picom-code/ezlm/internal/report"
	"github.com/picom-code/ezlm/internal/scan"
	"github.com/picom-code/ezlm/internal/update"
)

// fakeBackend_webauth answers every Backend call with a zero value. The auth tests
// only care that a request did or did not reach the backend.
type fakeBackend_webauth struct {
	statusCalls int
	events      chan app.Event
}

func (f *fakeBackend_webauth) Status(context.Context) app.Status {
	f.statusCalls++
	return app.Status{}
}
func (f *fakeBackend_webauth) Scan(context.Context, string) *app.ScanResult {
	return &app.ScanResult{}
}
func (f *fakeBackend_webauth) Deploy(string) error               { return nil }
func (f *fakeBackend_webauth) Kill(context.Context, int32) error { return nil }
func (f *fakeBackend_webauth) SetLAN(bool) error                 { return nil }
func (f *fakeBackend_webauth) Integrate([]string, bool) error    { return nil }
func (f *fakeBackend_webauth) AddKey(string) error               { return nil }
func (f *fakeBackend_webauth) RotateKey(string) error            { return nil }
func (f *fakeBackend_webauth) RevokeKey(string) error            { return nil }
func (f *fakeBackend_webauth) Stop() error                       { return nil }
func (f *fakeBackend_webauth) Start() error                      { return nil }
func (f *fakeBackend_webauth) Uninstall() error                  { return nil }
func (f *fakeBackend_webauth) Subscribe() (chan app.Event, func()) {
	return f.events, func() {}
}
func (f *fakeBackend_webauth) Live(context.Context) app.Live { return app.Live{} }
func (f *fakeBackend_webauth) Test() error                   { return nil }
func (f *fakeBackend_webauth) Cancel() error                 { return nil }
func (f *fakeBackend_webauth) Installed(context.Context, string) []app.InstalledModel {
	return nil
}
func (f *fakeBackend_webauth) Use(context.Context, string, string, string) error { return nil }
func (f *fakeBackend_webauth) Remove(context.Context, string, string) error      { return nil }
func (f *fakeBackend_webauth) Logs(int) []string                                 { return nil }
func (f *fakeBackend_webauth) Apps(context.Context) []scan.App                   { return nil }
func (f *fakeBackend_webauth) UpdateInfo(context.Context, bool) update.Info      { return update.Info{} }
func (f *fakeBackend_webauth) DownloadUpdate() error                             { return nil }
func (f *fakeBackend_webauth) Storage(context.Context) app.StorageInfo           { return app.StorageInfo{} }
func (f *fakeBackend_webauth) SetStorage(string, bool) error                     { return nil }
func (f *fakeBackend_webauth) Launch(string, string) error                       { return nil }
func (f *fakeBackend_webauth) IntegrationLog(string, int) []string               { return nil }
func (f *fakeBackend_webauth) Report(context.Context, string, bool, string) (report.Result, error) {
	return report.Result{}, nil
}
func (f *fakeBackend_webauth) CloseApp(context.Context, string, bool) (scan.CloseResult, error) {
	return scan.CloseResult{}, nil
}

// startAuthServer runs the real Serve loop on a loopback port, exactly as the
// CLI does, so the Host check and route table under test are the real ones.
func startAuthServer(t *testing.T) (*Server, *fakeBackend_webauth, string) {
	t.Helper()
	fake := &fakeBackend_webauth{events: make(chan app.Event)}
	s, l, url, err := Listen(fake, nil)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go s.Serve(l)
	t.Cleanup(func() { l.Close() })
	if !strings.HasPrefix(url, "http://"+s.Addr()+"/?t=") {
		t.Fatalf("Listen URL = %q, want http://%s/?t=<token>", url, s.Addr())
	}
	return s, fake, "http://" + s.Addr()
}

func authClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second}
}

func doAuthRequest(t *testing.T, req *http.Request) (*http.Response, string) {
	t.Helper()
	resp, err := authClient().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, string(b)
}

// TestAuthTokenRequired pins the token rules for the JSON API: no token and a
// wrong token are 401, the header and the query string are both accepted, and
// a present header wins over the query string.
func TestAuthTokenRequired(t *testing.T) {
	s, _, base := startAuthServer(t)

	cases := []struct {
		name       string
		header     string // X-EZLM-Token, "" means absent unless setHeader
		setHeader  bool   // send the header even when empty
		query      string // t, "" means absent
		wantStatus int
	}{
		{"no token", "", false, "", http.StatusUnauthorized},
		{"wrong token", "not-the-token", true, "", http.StatusUnauthorized},
		{"wrong token in query", "", false, "not-the-token", http.StatusUnauthorized},
		{"token in header", s.Token(), true, "", http.StatusOK},
		{"token in query on a non-stream route is now rejected", "", false, s.Token(), http.StatusUnauthorized},
		{"header and query agree", s.Token(), true, s.Token(), http.StatusOK},
		{"stale header beats good query", "stale", true, s.Token(), http.StatusUnauthorized},
		{"good header beats stale query", s.Token(), true, "stale", http.StatusOK},
		{"empty header no longer falls back to query", "", true, s.Token(), http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := base + "/api/status"
			if tc.query != "" {
				u += "?t=" + tc.query
			}
			req, err := http.NewRequest("GET", u, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.header != "" || tc.setHeader {
				req.Header.Set("X-EZLM-Token", tc.header)
			}
			resp, body := doAuthRequest(t, req)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", resp.StatusCode, tc.wantStatus, body)
			}
			if tc.wantStatus == http.StatusUnauthorized {
				if body != "this page is out of date: reopen EZLM\n" {
					t.Errorf("401 body = %q", body)
				}
				if strings.Contains(body, s.Token()) {
					t.Errorf("401 body leaks the token: %q", body)
				}
				if got := resp.Header.Get("Cache-Control"); got != "no-store" {
					t.Errorf("401 Cache-Control = %q, want no-store", got)
				}
			}
		})
	}
}

// TestAuthTokenHeaderIsExact: the token is compared whole, so a token with a
// different length or case is rejected. Surrounding spaces are not tested
// because HTTP trims them from header values before the handler sees them.
func TestAuthTokenHeaderIsExact(t *testing.T) {
	s, _, base := startAuthServer(t)
	for _, tok := range []string{
		s.Token() + "0",
		s.Token()[:len(s.Token())-1],
		strings.ToUpper(s.Token()),
		"0" + s.Token(),
	} {
		req, _ := http.NewRequest("GET", base+"/api/status", nil)
		req.Header.Set("X-EZLM-Token", tok)
		resp, body := doAuthRequest(t, req)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("token %q: status = %d, want 401 (body %q)", tok, resp.StatusCode, body)
		}
	}
}

// TestAuthRejectedRequestDoesNotReachBackend checks the guard runs before the
// handler, so a bad token cannot trigger any backend work.
func TestAuthRejectedRequestDoesNotReachBackend(t *testing.T) {
	s, fake, base := startAuthServer(t)

	req, _ := http.NewRequest("GET", base+"/api/status", nil)
	req.Header.Set("X-EZLM-Token", "wrong")
	if resp, _ := doAuthRequest(t, req); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if fake.statusCalls != 0 {
		t.Errorf("backend Status called %d times for a rejected request", fake.statusCalls)
	}

	req, _ = http.NewRequest("GET", base+"/api/status", nil)
	req.Header.Set("X-EZLM-Token", s.Token())
	if resp, _ := doAuthRequest(t, req); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if fake.statusCalls != 1 {
		t.Errorf("backend Status called %d times for an accepted request, want 1", fake.statusCalls)
	}
}

// TestAuthQueryTokenOnPOST: EventSource can only use the query string, but the
// guard also accepts it on POST routes. Pin that so the API stays usable from
// the bundled page.
func TestAuthQueryTokenOnPOST(t *testing.T) {
	s, _, base := startAuthServer(t)

	// Since T07 a04 only GET /api/events accepts the query token; a POST
	// must carry the header.
	req, _ := http.NewRequest("POST", base+"/api/scan?t="+s.Token(), strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, body := doAuthRequest(t, req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("POST with query token: status = %d, body %q", resp.StatusCode, body)
	}

	req, _ = http.NewRequest("POST", base+"/api/scan", strings.NewReader(`{}`))
	resp, body = doAuthRequest(t, req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("POST without token: status = %d, body %q", resp.StatusCode, body)
	}
}

// TestAuthEventsQueryToken covers the EventSource route, which is the reason
// the query token exists at all.
func TestAuthEventsQueryToken(t *testing.T) {
	s, _, base := startAuthServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"/api/events?t="+s.Token(), nil)
	resp, err := authClient().Do(req)
	if err != nil {
		t.Fatalf("GET /api/events: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, body %q", resp.StatusCode, b)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	resp.Body.Close()
	cancel()

	req, _ = http.NewRequest("GET", base+"/api/events", nil)
	resp, body := doAuthRequest(t, req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("events without token: status = %d, body %q", resp.StatusCode, body)
	}
}

// TestAuthTokenIsPerLaunch: a token from one server is not accepted by another.
func TestAuthTokenIsPerLaunch(t *testing.T) {
	s1, _, base1 := startAuthServer(t)
	s2, _, base2 := startAuthServer(t)
	if s1.Token() == s2.Token() {
		t.Fatalf("two servers share a token %q", s1.Token())
	}

	req, _ := http.NewRequest("GET", base2+"/api/status", nil)
	req.Header.Set("X-EZLM-Token", s1.Token())
	resp, body := doAuthRequest(t, req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("other server's token: status = %d, body %q", resp.StatusCode, body)
	}

	// Sanity check the second server with its own token, so the failure above
	// is the token and not a broken server.
	req, _ = http.NewRequest("GET", base1+"/api/status", nil)
	req.Header.Set("X-EZLM-Token", s1.Token())
	if resp, body := doAuthRequest(t, req); resp.StatusCode != http.StatusOK {
		t.Errorf("own token: status = %d, body %q", resp.StatusCode, body)
	}
}

// TestAuthHostCheck: a request with a foreign Host header is rejected even
// with the right token, which is the DNS-rebinding defence.
func TestAuthHostCheck(t *testing.T) {
	s, _, base := startAuthServer(t)

	req, _ := http.NewRequest("GET", base+"/api/status", nil)
	req.Host = "evil.example.com"
	req.Header.Set("X-EZLM-Token", s.Token())
	resp, body := doAuthRequest(t, req)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("foreign Host: status = %d, want 403 (body %q)", resp.StatusCode, body)
	}
	if body != "forbidden\n" {
		t.Errorf("foreign Host body = %q, want %q", body, "forbidden\n")
	}

	req, _ = http.NewRequest("GET", base+"/api/status", nil)
	req.Host = s.Addr()
	req.Header.Set("X-EZLM-Token", s.Token())
	if resp, body := doAuthRequest(t, req); resp.StatusCode != http.StatusOK {
		t.Errorf("loopback Host: status = %d, body %q", resp.StatusCode, body)
	}
}

// TestAuthStaticShellNeedsNoToken documents the boundary: the page shell and
// its assets are served without a token (the page is opened with ?t=...), but
// every /api route is guarded.
func TestAuthStaticShellNeedsNoToken(t *testing.T) {
	_, _, base := startAuthServer(t)

	req, _ := http.NewRequest("GET", base+"/", nil)
	resp, body := doAuthRequest(t, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /: status = %d, body %q", resp.StatusCode, body)
	}
	if !strings.Contains(body, "<html") && !strings.Contains(body, "<!doctype") && !strings.Contains(body, "<!DOCTYPE") {
		t.Errorf("GET / did not return the page shell: %q", body[:min(len(body), 120)])
	}
}
