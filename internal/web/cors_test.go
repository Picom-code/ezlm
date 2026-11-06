package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/picom-code/ezlm/internal/app"
)

// fakeBackend answers only the calls these tests make; an embedded nil
// interface satisfies the rest of Backend.
type fakeBackend struct{ Backend }

func (fakeBackend) Status(context.Context) app.Status { return app.Status{} }

const testToken = "REDACTED-KEY"

// corsServer returns a server with the real middleware chain around a fake app.
func corsServer(t *testing.T) *Server {
	t.Helper()
	return &Server{app: fakeBackend{}, token: testToken, addr: "127.0.0.1:43210"}
}

func corsRequest(t *testing.T, s *Server, method, target string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	r.Host = s.addr
	r.RemoteAddr = "127.0.0.1:1234" // hostCheck only accepts the address we listen on
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	s.handler().ServeHTTP(w, r)
	return w
}

// A page on another origin can send a request to the loopback API, but without
// Access-Control-Allow-Origin the browser hides the response body. That header
// absence is the whole read control, so it must be asserted, not assumed.
func TestCrossOriginReadIsDenied(t *testing.T) {
	s := corsServer(t)
	w := corsRequest(t, s, http.MethodGet, "/api/status",
		map[string]string{"Origin": "https://evil.example", "X-EZLM-Token": testToken})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %q", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q for a foreign origin, want empty", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("Access-Control-Allow-Credentials = %q, want empty", got)
	}
	if got := w.Header().Values("Vary"); len(got) == 0 || got[0] != "Origin" {
		t.Errorf("Vary = %q, want Origin so caches key on the request origin", got)
	}
}

// The UI is served from the same origin it calls, so its own origin may be
// echoed back. This keeps the policy explicit instead of relying on absence.
func TestSameOriginIsEchoed(t *testing.T) {
	s := corsServer(t)
	origin := "http://" + s.addr
	w := corsRequest(t, s, http.MethodGet, "/api/status?t="+testToken,
		map[string]string{"Origin": origin})

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, origin)
	}
}

// A preflight from a foreign origin must fail closed: no wildcard, no
// credentials, no allow-listed method or header.
func TestPreflightFailsClosed(t *testing.T) {
	s := corsServer(t)
	w := corsRequest(t, s, http.MethodOptions, "/api/status",
		map[string]string{
			"Origin":                         "https://evil.example",
			"Access-Control-Request-Method":  "POST",
			"Access-Control-Request-Headers": "x-ezlm-token, content-type",
		})

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204 so the browser sees a failed preflight", w.Code)
	}
	for _, h := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Methods",
		"Access-Control-Allow-Headers",
		"Access-Control-Allow-Credentials",
	} {
		if got := w.Header().Get(h); got != "" {
			t.Errorf("%s = %q for a foreign preflight, want empty", h, got)
		}
	}
}

// The token is accepted in the query string, so a foreign page can send a
// simple request with no preflight. It must still not be able to read the
// answer, and a wildcard would hand it the answer.
func TestNoWildcardOnSimpleRequest(t *testing.T) {
	s := corsServer(t)
	w := corsRequest(t, s, http.MethodGet, "/api/status?t="+testToken,
		map[string]string{"Origin": "null"})

	if got := w.Header().Get("Access-Control-Allow-Origin"); got == "*" {
		t.Errorf("Access-Control-Allow-Origin = * ; any page can read the API")
	}
}

// Without nosniff a browser may run a JSON response loaded with <script src>,
// which is a cross-origin read that needs no CORS header at all.
func TestAPIResponseIsNotSniffed(t *testing.T) {
	s := corsServer(t)
	w := corsRequest(t, s, http.MethodGet, "/api/status?t="+testToken, nil)

	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
}
