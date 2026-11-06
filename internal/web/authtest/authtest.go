// Package authtest starts EZLM's engine API on a loopback port with a stub
// backend, so a test can drive the real handler with a token it knows.
//
// A test in internal/web imports it from the external test package:
//
//	package web_test
//
//	func TestSomething(t *testing.T) {
//		h := authtest.New(t)
//		resp := h.Get(t, "/api/status")
//		...
//	}
//
// An internal test file (package web) cannot import this package: authtest
// imports web, and that would be an import cycle.
package authtest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/picom-code/ezlm/internal/app"
	"github.com/picom-code/ezlm/internal/report"
	"github.com/picom-code/ezlm/internal/scan"
	"github.com/picom-code/ezlm/internal/update"
	"github.com/picom-code/ezlm/internal/web"
)

// Harness is a running engine API and the token it accepts.
type Harness struct {
	Server *web.Server
	Stub   *Stub
	// Addr is the host:port the API listens on; Token is the per-launch secret.
	Addr  string
	Token string
	// Client is what the helpers use. Set it to model a browser (cookies,
	// redirect policy) before the first request.
	Client *http.Client
}

// New starts the engine API on an ephemeral loopback port and stops it when
// the test ends.
func New(t testing.TB) *Harness {
	t.Helper()
	stub := &Stub{events: make(chan app.Event, 64)}
	srv, l, _, err := web.Listen(stub, nil)
	if err != nil {
		t.Fatalf("authtest: listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.Serve(l)
	}()
	h := &Harness{
		Server: srv,
		Stub:   stub,
		Addr:   srv.Addr(),
		Token:  srv.Token(),
		Client: &http.Client{Timeout: 10 * time.Second},
	}
	t.Cleanup(func() {
		h.Client.CloseIdleConnections()
		l.Close()
		<-done
	})
	return h
}

// URL is the absolute URL of path on the test server.
func (h *Harness) URL(path string) string { return "http://" + h.Addr + path }

// Request builds a request carrying the token. Change r.Host, r.Header or the
// body to model an attacker before passing it to Do.
func (h *Harness) Request(t testing.TB, method, path string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, h.URL(path), body)
	if err != nil {
		t.Fatalf("authtest: new request %s %s: %v", method, path, err)
	}
	req.Header.Set("X-EZLM-Token", h.Token)
	return req
}

// NoToken builds the same request without the token, the way a hostile page
// sends it.
func (h *Harness) NoToken(t testing.TB, method, path string, body io.Reader) *http.Request {
	t.Helper()
	req := h.Request(t, method, path, body)
	req.Header.Del("X-EZLM-Token")
	return req
}

// Do sends req and fails the test if the transport fails.
func (h *Harness) Do(t testing.TB, req *http.Request) *http.Response {
	t.Helper()
	resp, err := h.Client.Do(req)
	if err != nil {
		t.Fatalf("authtest: %s %s: %v", req.Method, req.URL, err)
	}
	return resp
}

// Get sends a GET with the token.
func (h *Harness) Get(t testing.TB, path string) *http.Response {
	t.Helper()
	return h.Do(t, h.Request(t, http.MethodGet, path, nil))
}

// Post sends body with contentType and the token.
func (h *Harness) Post(t testing.TB, path, contentType, body string) *http.Response {
	t.Helper()
	req := h.Request(t, http.MethodPost, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", contentType)
	return h.Do(t, req)
}

// PostJSON sends v as a JSON body with the token.
func (h *Harness) PostJSON(t testing.TB, path string, v any) *http.Response {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("authtest: marshal body: %v", err)
	}
	req := h.Request(t, http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	return h.Do(t, req)
}

// GetNoToken and PostNoToken send a request without the token.
func (h *Harness) GetNoToken(t testing.TB, path string) *http.Response {
	t.Helper()
	return h.Do(t, h.NoToken(t, http.MethodGet, path, nil))
}

func (h *Harness) PostNoToken(t testing.TB, path, contentType, body string) *http.Response {
	t.Helper()
	req := h.NoToken(t, http.MethodPost, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", contentType)
	return h.Do(t, req)
}

// Body reads resp's body and closes it.
func Body(t testing.TB, resp *http.Response) []byte {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("authtest: read body: %v", err)
	}
	return b
}

// Decode reads resp's JSON body into v and closes it.
func Decode(t testing.TB, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("authtest: decode body: %v", err)
	}
}

// Stub is the Backend the test server drives. It records every call so a test
// can assert a request reached the app, and returns zero values otherwise.
// Set a hook to change one method's result, or embed *Stub in your own type to
// replace a method outright.
type Stub struct {
	mu     sync.Mutex
	calls  []string
	events chan app.Event

	OnStatus    func(context.Context) app.Status
	OnScan      func(context.Context, string) *app.ScanResult
	OnDeploy    func(string) error
	OnKill      func(context.Context, int32) error
	OnSetLAN    func(bool) error
	OnAddKey    func(string) error
	OnRotateKey func(string) error
	OnRevokeKey func(string) error
	OnUse       func(context.Context, string, string, string) error
	OnRemove    func(context.Context, string, string) error
	OnLogs      func(int) []string
	OnReport    func(context.Context, string, bool, string) (report.Result, error)
}

// Calls returns the backend method names called so far, in order.
func (s *Stub) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

// Called reports whether name has been called.
func (s *Stub) Called(name string) bool {
	for _, c := range s.Calls() {
		if c == name {
			return true
		}
	}
	return false
}

// ResetCalls forgets the calls recorded so far.
func (s *Stub) ResetCalls() {
	s.mu.Lock()
	s.calls = nil
	s.mu.Unlock()
}

// Emit sends e to the API's event stream.
func (s *Stub) Emit(e app.Event) {
	s.mu.Lock()
	if s.events == nil {
		s.events = make(chan app.Event, 64)
	}
	ch := s.events
	s.mu.Unlock()
	ch <- e
}

func (s *Stub) record(name string) {
	s.mu.Lock()
	s.calls = append(s.calls, name)
	s.mu.Unlock()
}

func (s *Stub) Status(ctx context.Context) app.Status {
	s.record("Status")
	if s.OnStatus != nil {
		return s.OnStatus(ctx)
	}
	return app.Status{}
}

func (s *Stub) Scan(ctx context.Context, pref string) *app.ScanResult {
	s.record("Scan")
	if s.OnScan != nil {
		return s.OnScan(ctx, pref)
	}
	return &app.ScanResult{Pref: pref}
}

func (s *Stub) Deploy(modelID string) error {
	s.record("Deploy")
	if s.OnDeploy != nil {
		return s.OnDeploy(modelID)
	}
	return nil
}

func (s *Stub) Kill(ctx context.Context, pid int32) error {
	s.record("Kill")
	if s.OnKill != nil {
		return s.OnKill(ctx, pid)
	}
	return nil
}

func (s *Stub) SetLAN(on bool) error {
	s.record("SetLAN")
	if s.OnSetLAN != nil {
		return s.OnSetLAN(on)
	}
	return nil
}

func (s *Stub) Integrate(ids []string, makeDefault bool) error {
	s.record("Integrate")
	return nil
}

func (s *Stub) AddKey(name string) error {
	s.record("AddKey")
	if s.OnAddKey != nil {
		return s.OnAddKey(name)
	}
	return nil
}

func (s *Stub) RotateKey(id string) error {
	s.record("RotateKey")
	if s.OnRotateKey != nil {
		return s.OnRotateKey(id)
	}
	return nil
}

func (s *Stub) RevokeKey(id string) error {
	s.record("RevokeKey")
	if s.OnRevokeKey != nil {
		return s.OnRevokeKey(id)
	}
	return nil
}

func (s *Stub) Stop() error {
	s.record("Stop")
	return nil
}

func (s *Stub) Start() error {
	s.record("Start")
	return nil
}

func (s *Stub) Uninstall() error {
	s.record("Uninstall")
	return nil
}

func (s *Stub) Subscribe() (chan app.Event, func()) {
	s.record("Subscribe")
	s.mu.Lock()
	if s.events == nil {
		s.events = make(chan app.Event, 64)
	}
	ch := s.events
	s.mu.Unlock()
	return ch, func() {}
}

func (s *Stub) Live(context.Context) app.Live {
	s.record("Live")
	return app.Live{}
}

func (s *Stub) Test() error {
	s.record("Test")
	return nil
}

func (s *Stub) Cancel() error {
	s.record("Cancel")
	return nil
}

func (s *Stub) Installed(context.Context, string) []app.InstalledModel {
	s.record("Installed")
	return nil
}

func (s *Stub) Use(ctx context.Context, modelID, quant, pref string) error {
	s.record("Use")
	if s.OnUse != nil {
		return s.OnUse(ctx, modelID, quant, pref)
	}
	return nil
}

func (s *Stub) Remove(ctx context.Context, modelID, quant string) error {
	s.record("Remove")
	if s.OnRemove != nil {
		return s.OnRemove(ctx, modelID, quant)
	}
	return nil
}

func (s *Stub) Logs(n int) []string {
	s.record("Logs")
	if s.OnLogs != nil {
		return s.OnLogs(n)
	}
	return nil
}

func (s *Stub) Apps(context.Context) []scan.App {
	s.record("Apps")
	return nil
}

func (s *Stub) UpdateInfo(context.Context, bool) update.Info {
	s.record("UpdateInfo")
	return update.Info{}
}

func (s *Stub) DownloadUpdate() error {
	s.record("DownloadUpdate")
	return nil
}

func (s *Stub) Storage(context.Context) app.StorageInfo {
	s.record("Storage")
	return app.StorageInfo{}
}

func (s *Stub) SetStorage(dir string, move bool) error {
	s.record("SetStorage")
	return nil
}

func (s *Stub) Launch(id, mode string) error {
	s.record("Launch")
	return nil
}

func (s *Stub) IntegrationLog(string, int) []string {
	s.record("IntegrationLog")
	return nil
}

func (s *Stub) Report(ctx context.Context, text string, details bool, appLog string) (report.Result, error) {
	s.record("Report")
	if s.OnReport != nil {
		return s.OnReport(ctx, text, details, appLog)
	}
	return report.Result{}, nil
}

func (s *Stub) CloseApp(context.Context, string, bool) (scan.CloseResult, error) {
	s.record("CloseApp")
	return scan.CloseResult{}, nil
}
