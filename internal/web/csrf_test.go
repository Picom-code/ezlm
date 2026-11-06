package web

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/picom-code/ezlm/internal/app"
)

// testToken_csrf stands in for the per-launch token. It is not a real key.
const testToken_csrf = "REDACTED-KEY"

// stubBackend records the state-changing calls the handlers make. It embeds
// Backend so a test only spells out the methods it exercises; a handler that
// reaches an unset method panics, which is the point.
type stubBackend struct {
	Backend
	mu    sync.Mutex
	calls []string
}

func (b *stubBackend) record(s string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, s)
}

func (b *stubBackend) Deploy(modelID string) error { b.record("deploy " + modelID); return nil }
func (b *stubBackend) Stop() error                 { b.record("stop"); return nil }
func (b *stubBackend) AddKey(name string) error    { b.record("addkey " + name); return nil }

func (b *stubBackend) Subscribe() (chan app.Event, func()) {
	return make(chan app.Event), func() {}
}

func (b *stubBackend) callList() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.calls...)
}

// newTestServer starts the real engine API on a loopback port with a known
// token, the way the CLI starts it.
func newTestServer(t *testing.T) (*Server, *stubBackend, string) {
	t.Helper()
	b := &stubBackend{}
	s, l, _, err := Listen(b, nil)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s.token = testToken_csrf
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Serve(l)
	}()
	t.Cleanup(func() {
		l.Close()
		<-done
	})
	return s, b, "http://" + l.Addr().String()
}

func do(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	io.Copy(io.Discard, resp.Body)
	return resp
}

// TestCSRFSimplePostFromWebPage is the attack a page at https://evil.example
// can run with no preflight: a POST with a CORS-safelisted content type
// (text/plain) and the token in the query string, which is where the UI keeps
// it (the address bar URL is http://host:port/?t=token). Before the fix the
// request reached the handler, the JSON body was decoded whatever the content
// type said, and the state changed.
func TestCSRFSimplePostFromWebPage(t *testing.T) {
	s, b, base := newTestServer(t)
	req, err := http.NewRequest(http.MethodPost, base+"/api/deploy?t="+s.Token(), strings.NewReader(`{"model_id":"evil"}`))
	if err != nil {
		t.Fatal(err)
	}
	// A browser sets these itself for a simple request; no preflight is sent.
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Origin", "https://evil.example")

	resp := do(t, req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("simple cross-origin POST: got %d, want 401 (the query token alone must not authorize a state change)", resp.StatusCode)
	}
	if got := b.callList(); len(got) != 0 {
		t.Fatalf("backend calls = %v, want none: the page changed state", got)
	}
}

// TestCSRFPostWithSafelistedContentType keeps the header token but sends a
// body in a type a form can produce. A browser can send this cross-origin with
// no preflight, so the server must reject it and force application/json, which
// does trigger a preflight.
func TestCSRFPostWithSafelistedContentType(t *testing.T) {
	s, b, base := newTestServer(t)
	for _, ct := range []string{"text/plain", "application/x-www-form-urlencoded", "multipart/form-data; boundary=x"} {
		req, err := http.NewRequest(http.MethodPost, base+"/api/stop", strings.NewReader("name=evil"))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-EZLM-Token", s.Token())
		req.Header.Set("Content-Type", ct)
		req.Header.Set("Origin", "https://evil.example")

		resp := do(t, req)
		if resp.StatusCode != http.StatusUnsupportedMediaType {
			t.Errorf("Content-Type %q: got %d, want 415", ct, resp.StatusCode)
		}
	}
	if got := b.callList(); len(got) != 0 {
		t.Fatalf("backend calls = %v, want none: a safelisted content type must not reach the handler", got)
	}
}

// TestJSONPostStillWorks is the control: the real clients (the UI, the Windows
// and Mac apps) send the token in the header and a JSON body.
func TestJSONPostStillWorks(t *testing.T) {
	s, b, base := newTestServer(t)
	req, err := http.NewRequest(http.MethodPost, base+"/api/deploy", strings.NewReader(`{"model_id":"llama"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-EZLM-Token", s.Token())
	req.Header.Set("Content-Type", "application/json")

	resp := do(t, req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("JSON POST with header token: got %d, want 200", resp.StatusCode)
	}
	if got := b.callList(); len(got) != 1 || got[0] != "deploy llama" {
		t.Fatalf("backend calls = %v, want [deploy llama]", got)
	}
}

// TestJSONPostWithCharsetStillWorks covers a client that adds a charset
// parameter, which is still application/json.
func TestJSONPostWithCharsetStillWorks(t *testing.T) {
	s, _, base := newTestServer(t)
	req, err := http.NewRequest(http.MethodPost, base+"/api/keys/add", strings.NewReader(`{"name":"laptop"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-EZLM-Token", s.Token())
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	if resp := do(t, req); resp.StatusCode != http.StatusOK {
		t.Fatalf("JSON POST with charset: got %d, want 200", resp.StatusCode)
	}
}

// TestPostWithoutTokenIsRejected is the baseline: the token is still required.
func TestPostWithoutTokenIsRejected(t *testing.T) {
	_, b, base := newTestServer(t)
	req, err := http.NewRequest(http.MethodPost, base+"/api/stop", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	if resp := do(t, req); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("POST without a token: got %d, want 401", resp.StatusCode)
	}
	if got := b.callList(); len(got) != 0 {
		t.Fatalf("backend calls = %v, want none", got)
	}
}

// TestEventSourceKeepsQueryToken covers the one client that cannot set a
// header: EventSource opens GET /api/events?t=..., so the query fallback must
// survive there.
func TestEventSourceKeepsQueryToken(t *testing.T) {
	s, _, base := newTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/events?t="+s.Token(), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("event stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("event stream: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("event stream: Content-Type = %q, want text/event-stream", ct)
	}
	cancel()
}
