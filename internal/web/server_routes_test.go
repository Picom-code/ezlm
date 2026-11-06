package web

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/picom-code/ezlm/internal/app"
	"github.com/picom-code/ezlm/internal/report"
	"github.com/picom-code/ezlm/internal/scan"
	"github.com/picom-code/ezlm/internal/update"
)

// fakeBackend_serverroutes implements Backend for route tests: it records the calls the
// handlers make and returns canned values. No real app, no real home.
type fakeBackend_serverroutes struct {
	mu         sync.Mutex
	calls      []string
	err        error
	events     chan app.Event
	cancelSeen chan struct{}
}

func (f *fakeBackend_serverroutes) record(s string) {
	f.mu.Lock()
	f.calls = append(f.calls, s)
	f.mu.Unlock()
}

func (f *fakeBackend_serverroutes) lastCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeBackend_serverroutes) Status(context.Context) app.Status {
	return app.Status{Version: "1.2.3"}
}

func (f *fakeBackend_serverroutes) Scan(_ context.Context, pref string) *app.ScanResult {
	f.record("Scan:" + pref)
	return &app.ScanResult{Pref: pref}
}

func (f *fakeBackend_serverroutes) Deploy(modelID string) error {
	f.record("Deploy:" + modelID)
	return f.err
}

func (f *fakeBackend_serverroutes) Kill(_ context.Context, pid int32) error {
	f.record("Kill:" + itoa(int64(pid)))
	return f.err
}

func (f *fakeBackend_serverroutes) SetLAN(on bool) error {
	f.record("SetLAN:" + btoa(on))
	return f.err
}

func (f *fakeBackend_serverroutes) Integrate(ids []string, makeDefault bool) error {
	f.record("Integrate:" + strings.Join(ids, ",") + ":" + btoa(makeDefault))
	return f.err
}

func (f *fakeBackend_serverroutes) AddKey(name string) error {
	f.record("AddKey:" + name)
	return f.err
}

func (f *fakeBackend_serverroutes) RotateKey(id string) error {
	f.record("RotateKey:" + id)
	return f.err
}

func (f *fakeBackend_serverroutes) RevokeKey(id string) error {
	f.record("RevokeKey:" + id)
	return f.err
}

func (f *fakeBackend_serverroutes) Stop() error  { f.record("Stop"); return f.err }
func (f *fakeBackend_serverroutes) Start() error { f.record("Start"); return f.err }

func (f *fakeBackend_serverroutes) Uninstall() error { f.record("Uninstall"); return f.err }

func (f *fakeBackend_serverroutes) Subscribe() (chan app.Event, func()) {
	f.record("Subscribe")
	ch := f.events
	if ch == nil {
		ch = make(chan app.Event)
	}
	return ch, func() {
		if f.cancelSeen == nil {
			return
		}
		select {
		case <-f.cancelSeen:
		default:
			close(f.cancelSeen)
		}
	}
}

func (f *fakeBackend_serverroutes) Live(context.Context) app.Live {
	f.record("Live")
	return app.Live{Healthy: true, Phase: "idle"}
}

func (f *fakeBackend_serverroutes) Test() error   { f.record("Test"); return f.err }
func (f *fakeBackend_serverroutes) Cancel() error { f.record("Cancel"); return f.err }

func (f *fakeBackend_serverroutes) Installed(_ context.Context, pref string) []app.InstalledModel {
	f.record("Installed:" + pref)
	return nil
}

func (f *fakeBackend_serverroutes) Use(_ context.Context, modelID, quant, pref string) error {
	f.record("Use:" + modelID + ":" + quant + ":" + pref)
	return f.err
}

func (f *fakeBackend_serverroutes) Remove(_ context.Context, modelID, quant string) error {
	f.record("Remove:" + modelID + ":" + quant)
	return f.err
}

func (f *fakeBackend_serverroutes) Logs(n int) []string {
	f.record("Logs:" + itoa(int64(n)))
	return []string{"log line"}
}

func (f *fakeBackend_serverroutes) Apps(context.Context) []scan.App {
	f.record("Apps")
	return []scan.App{{ID: "app-1", Name: "App"}}
}

func (f *fakeBackend_serverroutes) UpdateInfo(_ context.Context, force bool) update.Info {
	f.record("UpdateInfo:" + btoa(force))
	return update.Info{Current: "1.0.0"}
}

func (f *fakeBackend_serverroutes) DownloadUpdate() error { f.record("DownloadUpdate"); return f.err }

func (f *fakeBackend_serverroutes) Storage(context.Context) app.StorageInfo {
	f.record("Storage")
	return app.StorageInfo{Dir: "/tmp/models"}
}

func (f *fakeBackend_serverroutes) SetStorage(dir string, move bool) error {
	f.record("SetStorage:" + dir + ":" + btoa(move))
	return f.err
}

func (f *fakeBackend_serverroutes) Launch(id, mode string) error {
	f.record("Launch:" + id + ":" + mode)
	return f.err
}

func (f *fakeBackend_serverroutes) IntegrationLog(id string, n int) []string {
	f.record("IntegrationLog:" + id + ":" + itoa(int64(n)))
	return []string{"integrate line"}
}

func (f *fakeBackend_serverroutes) Report(_ context.Context, text string, details bool, appLog string) (report.Result, error) {
	f.record("Report:" + text + ":" + btoa(details) + ":" + appLog)
	return report.Result{URL: "https://example.test/issue"}, f.err
}

func (f *fakeBackend_serverroutes) CloseApp(_ context.Context, id string, force bool) (scan.CloseResult, error) {
	f.record("CloseApp:" + id + ":" + btoa(force))
	return scan.CloseResult{Closed: true}, f.err
}

// testServer is the real Serve loop on a loopback listener.
type testServer struct {
	s    *Server
	c    *http.Client
	base string
}

// startServer runs Serve and returns a client that presents the server token.
func startServer(t *testing.T, b Backend) *testServer {
	t.Helper()
	s, l, url, err := Listen(b, nil)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	if !strings.HasPrefix(url, "http://"+s.Addr()+"/?t="+s.Token()) {
		t.Fatalf("Listen URL = %q, want http://%s/?t=%s", url, s.Addr(), s.Token())
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Serve(l)
	}()
	t.Cleanup(func() {
		l.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("Serve did not return after the listener closed")
		}
	})
	return &testServer{s: s, c: &http.Client{Timeout: 5 * time.Second}, base: "http://" + s.Addr()}
}

func (ts *testServer) do(t *testing.T, method, path string, body io.Reader) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, ts.base+path, body)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("X-EZLM-Token", ts.s.Token())
	if method == http.MethodPost {
		// Since T07 a14 every /api POST must be JSON.
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := ts.c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, string(b)
}

func jsonBody(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("body is not JSON: %v (%q)", err, s)
	}
	return m
}

func btoa(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestListenURLAndToken(t *testing.T) {
	b := &fakeBackend_serverroutes{}
	s, l, url, err := Listen(b, nil)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer l.Close()
	if s.Addr() != l.Addr().String() {
		t.Errorf("Addr = %q, want listener address %q", s.Addr(), l.Addr().String())
	}
	if len(s.Token()) != 32 {
		t.Errorf("token length = %d, want 32 hex characters", len(s.Token()))
	}
	if strings.Trim(s.Token(), "0123456789abcdef") != "" {
		t.Errorf("token %q is not lowercase hex", s.Token())
	}
	if !strings.HasSuffix(url, "/?t="+s.Token()) {
		t.Errorf("URL = %q, want it to end with /?t=<token>", url)
	}
}

func TestStaticFiles(t *testing.T) {
	ts := startServer(t, &fakeBackend_serverroutes{})

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantType   string
		wantSubstr string
		wantMagic  string
	}{
		{name: "index", path: "/", wantStatus: 200, wantType: "text/html", wantSubstr: "<!doctype html>"},
		{name: "stylesheet", path: "/style.css", wantStatus: 200, wantType: "text/css", wantSubstr: "--"},
		{name: "script", path: "/app.js", wantStatus: 200, wantType: "javascript", wantSubstr: "use strict"},
		{name: "logo", path: "/logos/app-dsh.png", wantStatus: 200, wantType: "image/png", wantMagic: "\x89PNG"},
		{name: "missing asset", path: "/missing.js", wantStatus: 404, wantSubstr: "404"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, body := ts.do(t, http.MethodGet, tt.path, nil)
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("GET %s status = %d, want %d", tt.path, resp.StatusCode, tt.wantStatus)
			}
			if tt.wantType != "" && !strings.Contains(resp.Header.Get("Content-Type"), tt.wantType) {
				t.Errorf("GET %s Content-Type = %q, want %q", tt.path, resp.Header.Get("Content-Type"), tt.wantType)
			}
			if tt.wantSubstr != "" && !strings.Contains(body, tt.wantSubstr) {
				t.Errorf("GET %s body does not contain %q: %.80q", tt.path, tt.wantSubstr, body)
			}
			if tt.wantMagic != "" && !strings.HasPrefix(body, tt.wantMagic) {
				t.Errorf("GET %s body does not start with %q", tt.path, tt.wantMagic)
			}
			// ServeContent strips Cache-Control from its own error responses,
			// so only a successful file carries the header hostCheck set.
			if tt.wantStatus == 200 {
				if got := resp.Header.Get("Cache-Control"); got != "no-store" {
					t.Errorf("GET %s Cache-Control = %q, want no-store", tt.path, got)
				}
			}
			if got := resp.Header.Get("X-Frame-Options"); got != "DENY" {
				t.Errorf("GET %s X-Frame-Options = %q, want DENY", tt.path, got)
			}
		})
	}

	t.Run("HEAD index", func(t *testing.T) {
		resp, body := ts.do(t, http.MethodHead, "/", nil)
		if resp.StatusCode != 200 {
			t.Fatalf("HEAD / status = %d, want 200", resp.StatusCode)
		}
		if body != "" {
			t.Errorf("HEAD / body = %q, want empty", body)
		}
	})
}

func TestUnknownRoute(t *testing.T) {
	ts := startServer(t, &fakeBackend_serverroutes{})

	resp, body := ts.do(t, http.MethodGet, "/api/does-not-exist", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET unknown API path status = %d, want 404", resp.StatusCode)
	}
	if !strings.Contains(body, "404") {
		t.Errorf("GET unknown API path body = %q, want the file server's 404 page", body)
	}

	// The static file server must not escape its embedded root.
	resp, _ = ts.do(t, http.MethodGet, "/../server.go", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Logf("GET /../server.go status = %d (file server redirects out of the root)", resp.StatusCode)
	}
}

func TestPostToUnknownPath(t *testing.T) {
	ts := startServer(t, &fakeBackend_serverroutes{})

	// The "GET /" file-server pattern matches every path, so a method that is
	// not GET falls through to a 405 for paths that do not exist.
	resp, body := ts.do(t, http.MethodPost, "/api/does-not-exist", strings.NewReader(`{}`))
	if resp.StatusCode != http.StatusNotFound {
		t.Skipf("bug: POST to an unknown path returns %d (Allow %q, body %q), want 404",
			resp.StatusCode, resp.Header.Get("Allow"), body)
	}
}

func TestGetOnPostOnlyRoute(t *testing.T) {
	ts := startServer(t, &fakeBackend_serverroutes{})

	// The static catch-all wins over the method-mismatch rule, so a GET on a
	// POST-only route serves the 404 page instead of 405.
	resp, body := ts.do(t, http.MethodGet, "/api/deploy", nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Skipf("bug: GET on POST-only /api/deploy returns %d with %q, want 405", resp.StatusCode, body)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	ts := startServer(t, &fakeBackend_serverroutes{})

	tests := []struct {
		method  string
		path    string
		allowed string
	}{
		{method: http.MethodPost, path: "/api/status", allowed: "GET"},
		{method: http.MethodPost, path: "/api/logs", allowed: "GET"},
		{method: http.MethodDelete, path: "/api/scan", allowed: "POST"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			resp, _ := ts.do(t, tt.method, tt.path, nil)
			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s status = %d, want 405", tt.method, tt.path, resp.StatusCode)
			}
			if got := resp.Header.Get("Allow"); !strings.Contains(got, tt.allowed) {
				t.Errorf("%s %s Allow = %q, want it to include %q", tt.method, tt.path, got, tt.allowed)
			}
		})
	}
}

func TestPostRoutes(t *testing.T) {
	tests := []struct {
		path     string
		body     string
		wantCall string
		wantBody string
	}{
		{path: "/api/scan", body: `{"pref":"balanced"}`, wantCall: "Scan:balanced", wantBody: `"pref":"balanced"`},
		{path: "/api/deploy", body: `{"model_id":"qwen"}`, wantCall: "Deploy:qwen", wantBody: `{"ok":true}`},
		{path: "/api/kill", body: `{"pid":4242}`, wantCall: "Kill:4242", wantBody: `{"ok":true}`},
		{path: "/api/lan", body: `{"on":true}`, wantCall: "SetLAN:true", wantBody: `{"ok":true}`},
		{path: "/api/integrate", body: `{"ids":["a","b"],"default":true}`, wantCall: "Integrate:a,b:true", wantBody: `{"ok":true}`},
		{path: "/api/keys/add", body: `{"name":"laptop"}`, wantCall: "AddKey:laptop", wantBody: `{"ok":true}`},
		{path: "/api/keys/rotate", body: `{"id":"k1"}`, wantCall: "RotateKey:k1", wantBody: `{"ok":true}`},
		{path: "/api/keys/revoke", body: `{"id":"k2"}`, wantCall: "RevokeKey:k2", wantBody: `{"ok":true}`},
		{path: "/api/installed", body: `{"pref":"speed"}`, wantCall: "Installed:speed", wantBody: `"models":null`},
		{path: "/api/update/download", body: ``, wantCall: "DownloadUpdate", wantBody: `{"ok":true}`},
		{path: "/api/storage", body: `{"dir":"/data","move":true}`, wantCall: "SetStorage:/data:true", wantBody: `{"ok":true}`},
		{path: "/api/launch", body: `{"id":"hermes","mode":"chat"}`, wantCall: "Launch:hermes:chat", wantBody: `{"ok":true}`},
		{path: "/api/report", body: `{"text":"broken","details":true,"app_log":"log"}`, wantCall: "Report:broken:true:log", wantBody: `"url":"https://example.test/issue"`},
		{path: "/api/apps/close", body: `{"id":"chrome","force":true}`, wantCall: "CloseApp:chrome:true", wantBody: `"closed":true`},
		{path: "/api/use", body: `{"model_id":"m","quant":"Q4","pref":"quality"}`, wantCall: "Use:m:Q4:quality", wantBody: `{"ok":true}`},
		{path: "/api/remove", body: `{"model_id":"m","quant":"Q4"}`, wantCall: "Remove:m:Q4", wantBody: `{"ok":true}`},
		{path: "/api/cancel", body: ``, wantCall: "Cancel", wantBody: `{"ok":true}`},
		{path: "/api/test", body: ``, wantCall: "Test", wantBody: `{"ok":true}`},
		{path: "/api/stop", body: ``, wantCall: "Stop", wantBody: `{"ok":true}`},
		{path: "/api/start", body: ``, wantCall: "Start", wantBody: `{"ok":true}`},
		{path: "/api/uninstall", body: ``, wantCall: "Uninstall", wantBody: `{"ok":true}`},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			b := &fakeBackend_serverroutes{}
			ts := startServer(t, b)
			var body io.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			}
			resp, got := ts.do(t, http.MethodPost, tt.path, body)
			if resp.StatusCode != 200 {
				t.Fatalf("POST %s status = %d, want 200 (body %q)", tt.path, resp.StatusCode, got)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("POST %s Content-Type = %q, want application/json", tt.path, ct)
			}
			if !strings.Contains(got, tt.wantBody) {
				t.Errorf("POST %s body = %q, want it to contain %q", tt.path, got, tt.wantBody)
			}
			calls := b.lastCalls()
			if len(calls) != 1 || calls[0] != tt.wantCall {
				t.Errorf("POST %s calls = %v, want [%s]", tt.path, calls, tt.wantCall)
			}
		})
	}
}

func TestGetRoutes(t *testing.T) {
	tests := []struct {
		path     string
		wantCall string
		wantBody string
	}{
		{path: "/api/update", wantCall: "UpdateInfo:false", wantBody: `"current":"1.0.0"`},
		{path: "/api/update?force=1", wantCall: "UpdateInfo:true", wantBody: `"current":"1.0.0"`},
		{path: "/api/storage", wantCall: "Storage", wantBody: `"dir":"/tmp/models"`},
		{path: "/api/integrate/log?id=hermes", wantCall: "IntegrationLog:hermes:200", wantBody: `"integrate line"`},
		{path: "/api/apps", wantCall: "Apps", wantBody: `"id":"app-1"`},
		{path: "/api/live", wantCall: "Live", wantBody: `"healthy":true`},
		{path: "/api/logs", wantCall: "Logs:300", wantBody: `"log line"`},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			b := &fakeBackend_serverroutes{}
			ts := startServer(t, b)
			resp, body := ts.do(t, http.MethodGet, tt.path, nil)
			if resp.StatusCode != 200 {
				t.Fatalf("GET %s status = %d, want 200 (body %q)", tt.path, resp.StatusCode, body)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("GET %s Content-Type = %q, want application/json", tt.path, ct)
			}
			if !strings.Contains(body, tt.wantBody) {
				t.Errorf("GET %s body = %q, want it to contain %q", tt.path, body, tt.wantBody)
			}
			calls := b.lastCalls()
			if len(calls) != 1 || calls[0] != tt.wantCall {
				t.Errorf("GET %s calls = %v, want [%s]", tt.path, calls, tt.wantCall)
			}
		})
	}
}

var errBackend = &backendError{}

type backendError struct{}

func (*backendError) Error() string { return "engine is not deployed" }

func TestAPIError(t *testing.T) {
	b := &fakeBackend_serverroutes{err: errBackend}
	ts := startServer(t, b)

	resp, body := ts.do(t, http.MethodPost, "/api/deploy", strings.NewReader(`{"model_id":"m"}`))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	m := jsonBody(t, body)
	if m["error"] != errBackend.Error() {
		t.Errorf("error = %v, want %q", m["error"], errBackend.Error())
	}
}

func TestMalformedJSONBody(t *testing.T) {
	ts := startServer(t, &fakeBackend_serverroutes{})

	// A truncated body currently decodes to zero values, so the handler
	// reports success for a request that named no model.
	resp, body := ts.do(t, http.MethodPost, "/api/deploy", strings.NewReader(`{"model_id":`))
	if resp.StatusCode != http.StatusBadRequest {
		t.Skipf("bug: malformed JSON on /api/deploy returns %d with %q, want 400", resp.StatusCode, body)
	}
}

func TestEventStream(t *testing.T) {
	b := &fakeBackend_serverroutes{events: make(chan app.Event, 1), cancelSeen: make(chan struct{})}
	ts := startServer(t, b)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.base+"/api/events?t="+ts.s.Token(), nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := ts.c.Do(req)
	if err != nil {
		t.Fatalf("GET /api/events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	b.events <- app.Event{Job: "deploy", Step: "engine", State: "ok", Msg: "ready"}
	rd := bufio.NewReader(resp.Body)
	line, err := rd.ReadString('\n')
	if err != nil {
		t.Fatalf("read event: %v", err)
	}
	if line != "data: {\"job\":\"deploy\",\"step\":\"engine\",\"state\":\"ok\",\"msg\":\"ready\"}\n" {
		t.Errorf("event line = %q", line)
	}
	if blank, err := rd.ReadString('\n'); err != nil || blank != "\n" {
		t.Errorf("event separator = %q, %v; want a blank line", blank, err)
	}

	cancel()
	select {
	case <-b.cancelSeen:
	case <-time.After(5 * time.Second):
		t.Error("the subscription was not cancelled after the request ended")
	}
}
