package web

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/picom-code/ezlm/internal/app"
	"github.com/picom-code/ezlm/internal/report"
	"github.com/picom-code/ezlm/internal/scan"
	"github.com/picom-code/ezlm/internal/update"
)

// a07Token is the per-launch token the test server expects. Test keys look
// like REDACTED-KEY so nothing here can be mistaken for a real secret.
const a07Token = "REDACTED-KEY"

// a07Backend is a scripted Backend. Only Status and Live return data; the
// rest exist to satisfy the interface and are never called by these tests.
type a07Backend struct {
	status app.Status
	live   app.Live
}

var _ Backend = (*a07Backend)(nil)

func (b *a07Backend) Status(context.Context) app.Status { return b.status }
func (b *a07Backend) Live(context.Context) app.Live     { return b.live }

func (b *a07Backend) Scan(context.Context, string) *app.ScanResult { return nil }
func (b *a07Backend) Deploy(string) error                          { return nil }
func (b *a07Backend) Kill(context.Context, int32) error            { return nil }
func (b *a07Backend) SetLAN(bool) error                            { return nil }
func (b *a07Backend) Integrate([]string, bool) error               { return nil }
func (b *a07Backend) AddKey(string) error                          { return nil }
func (b *a07Backend) RotateKey(string) error                       { return nil }
func (b *a07Backend) RevokeKey(string) error                       { return nil }
func (b *a07Backend) Stop() error                                  { return nil }
func (b *a07Backend) Start() error                                 { return nil }
func (b *a07Backend) Uninstall() error                             { return nil }

func (b *a07Backend) Subscribe() (chan app.Event, func()) { return nil, func() {} }
func (b *a07Backend) Test() error                         { return nil }
func (b *a07Backend) Cancel() error                       { return nil }
func (b *a07Backend) Installed(context.Context, string) []app.InstalledModel {
	return nil
}
func (b *a07Backend) Use(context.Context, string, string, string) error { return nil }
func (b *a07Backend) Remove(context.Context, string, string) error      { return nil }
func (b *a07Backend) Logs(int) []string                                 { return nil }
func (b *a07Backend) Apps(context.Context) []scan.App                   { return nil }
func (b *a07Backend) UpdateInfo(context.Context, bool) update.Info      { return update.Info{} }
func (b *a07Backend) DownloadUpdate() error                             { return nil }
func (b *a07Backend) Storage(context.Context) app.StorageInfo           { return app.StorageInfo{} }
func (b *a07Backend) SetStorage(string, bool) error                     { return nil }
func (b *a07Backend) Launch(string, string) error                       { return nil }
func (b *a07Backend) IntegrationLog(string, int) []string               { return nil }
func (b *a07Backend) Report(context.Context, string, bool, string) (report.Result, error) {
	return report.Result{}, nil
}
func (b *a07Backend) CloseApp(context.Context, string, bool) (scan.CloseResult, error) {
	return scan.CloseResult{}, nil
}

// a07Serve starts the real Server (mux, host check, guard) on a loopback
// listener and returns its base URL.
func a07Serve(t *testing.T, b *a07Backend) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &Server{app: b, token: a07Token, addr: l.Addr().String()}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = s.Serve(l)
	}()
	t.Cleanup(func() {
		_ = l.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("server did not stop")
		}
	})
	return "http://" + s.addr
}

// a07Do sends a request with the test token and returns the response and the
// body it read.
func a07Do(t *testing.T, method, url string, body io.Reader) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Close = true
	req.Header.Set("X-EZLM-Token", a07Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, data
}

// TestA07StatusOK covers the success contract of GET /api/status: method,
// content type, cache and framing headers, and the JSON body.
func TestA07StatusOK(t *testing.T) {
	want := app.Status{
		Deployed: true,
		Healthy:  true,
		LocalURL: "http://127.0.0.1:8080/v1",
		LANURL:   "http://192.168.1.5:8080/v1",
		Version:  "1.2.3",
		Targets:  nil,
	}
	base := a07Serve(t, &a07Backend{status: want})

	resp, body := a07Do(t, http.MethodGet, base+"/api/status", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := resp.Header.Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", got)
	}
	if !strings.HasSuffix(string(body), "\n") {
		t.Errorf("body %q does not end with a newline", body)
	}
	var got app.Status
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not JSON: %v (body %q)", err, body)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("status body = %+v, want %+v", got, want)
	}
}

// TestA07StatusZero covers a fresh install: omitempty fields must be absent
// and the always-present fields must keep their JSON names.
func TestA07StatusZero(t *testing.T) {
	base := a07Serve(t, &a07Backend{})

	resp, body := a07Do(t, http.MethodGet, base+"/api/status", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", resp.StatusCode, body)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("body is not a JSON object: %v (body %q)", err, body)
	}
	for _, key := range []string{"config", "local_url", "lan_url", "busy", "tune", "start_error"} {
		if _, ok := raw[key]; ok {
			t.Errorf("zero status should omit %q, body = %s", key, body)
		}
	}
	for _, key := range []string{"deployed", "healthy", "starting", "user_stopped", "version", "targets"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("status is missing %q, body = %s", key, body)
		}
	}
	if string(raw["version"]) != `""` {
		t.Errorf("version = %s, want empty string", raw["version"])
	}
	if string(raw["deployed"]) != "false" {
		t.Errorf("deployed = %s, want false", raw["deployed"])
	}
}

// TestA07StatusMethods covers the method contract: GET and HEAD answer, every
// other method is a 405 that names the allowed methods.
func TestA07StatusMethods(t *testing.T) {
	base := a07Serve(t, &a07Backend{})
	url := base + "/api/status"

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			resp, body := a07Do(t, method, url, strings.NewReader("{}"))
			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Fatalf("%s status = %d, want 405 (body %q)", method, resp.StatusCode, body)
			}
			if got := resp.Header.Get("Allow"); !strings.Contains(got, http.MethodGet) {
				t.Errorf("Allow = %q, want it to contain GET", got)
			}
			if got := strings.TrimSpace(string(body)); got != "Method Not Allowed" {
				t.Errorf("%s body = %q, want %q", method, got, "Method Not Allowed")
			}
		})
	}

	t.Run(http.MethodHead, func(t *testing.T) {
		resp, body := a07Do(t, http.MethodHead, url, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("HEAD status = %d, want 200 (body %q)", resp.StatusCode, body)
		}
		if len(body) != 0 {
			t.Errorf("HEAD body = %q, want empty", body)
		}
	})
}

// TestA07Live covers GET /api/live, the cheap status snapshot the UI polls.
func TestA07Live(t *testing.T) {
	want := app.Live{
		RAMTotal:  34359738368,
		RAMAvail:  17179869184,
		Unified:   true,
		Healthy:   true,
		TokPerSec: 42.5,
		Requests:  2,
		Phase:     "writing",
		Written:   128,
	}
	base := a07Serve(t, &a07Backend{live: want})

	resp, body := a07Do(t, http.MethodGet, base+"/api/live", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	var got app.Live
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body is not JSON: %v (body %q)", err, body)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("live body = %+v, want %+v", got, want)
	}
}

// TestA07LiveMethod covers the method contract for /api/live.
func TestA07LiveMethod(t *testing.T) {
	base := a07Serve(t, &a07Backend{})

	resp, body := a07Do(t, http.MethodPost, base+"/api/live", strings.NewReader("{}"))
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405 (body %q)", resp.StatusCode, body)
	}
}

// TestA07StatusForeignHost covers the host check that wraps every route: a
// request for another Host never reaches the status handler.
func TestA07StatusForeignHost(t *testing.T) {
	base := a07Serve(t, &a07Backend{})

	req, err := http.NewRequest(http.MethodGet, base+"/api/status", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = "evil.example"
	req.Close = true
	req.Header.Set("X-EZLM-Token", a07Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (body %q)", resp.StatusCode, body)
	}
	if got := strings.TrimSpace(string(body)); got != "forbidden" {
		t.Errorf("body = %q, want %q", got, "forbidden")
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", got)
	}
}
