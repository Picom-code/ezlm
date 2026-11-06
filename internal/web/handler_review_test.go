package web

// Handler review (T07/a14): request body limits, JSON decode errors,
// content-type confusion, path traversal and absolute paths in error text.
// Every test drives the production wiring from Listen/Serve over loopback.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/app"
	"github.com/picom-code/ezlm/internal/report"
	"github.com/picom-code/ezlm/internal/scan"
	"github.com/picom-code/ezlm/internal/update"
)

// reviewBackend records the calls that matter to the tests and returns
// zero values for everything else.
type reviewBackend struct {
	revoked    []string
	reported   int
	stopped    int
	storageErr error
}

func (b_handlerreview *reviewBackend) Status(context.Context) app.Status { return app.Status{} }
func (b_handlerreview *reviewBackend) Scan(context.Context, string) *app.ScanResult {
	return &app.ScanResult{}
}
func (b_handlerreview *reviewBackend) Deploy(string) error               { return nil }
func (b_handlerreview *reviewBackend) Kill(context.Context, int32) error { return nil }
func (b_handlerreview *reviewBackend) SetLAN(bool) error                 { return nil }
func (b_handlerreview *reviewBackend) Integrate([]string, bool) error    { return nil }
func (b_handlerreview *reviewBackend) AddKey(string) error               { return nil }
func (b_handlerreview *reviewBackend) RotateKey(string) error            { return nil }
func (b_handlerreview *reviewBackend) RevokeKey(id string) error {
	b_handlerreview.revoked = append(b_handlerreview.revoked, id)
	return nil
}
func (b_handlerreview *reviewBackend) Stop() error      { return nil }
func (b_handlerreview *reviewBackend) Start() error     { return nil }
func (b_handlerreview *reviewBackend) Uninstall() error { return nil }
func (b_handlerreview *reviewBackend) Subscribe() (chan app.Event, func()) {
	return make(chan app.Event), func() {}
}
func (b_handlerreview *reviewBackend) Live(context.Context) app.Live { return app.Live{} }
func (b_handlerreview *reviewBackend) Test() error                   { return nil }
func (b_handlerreview *reviewBackend) Cancel() error                 { return nil }
func (b_handlerreview *reviewBackend) Installed(context.Context, string) []app.InstalledModel {
	return nil
}
func (b_handlerreview *reviewBackend) Use(context.Context, string, string, string) error { return nil }
func (b_handlerreview *reviewBackend) Remove(context.Context, string, string) error      { return nil }
func (b_handlerreview *reviewBackend) Logs(int) []string                                 { return nil }
func (b_handlerreview *reviewBackend) Apps(context.Context) []scan.App                   { return nil }
func (b_handlerreview *reviewBackend) UpdateInfo(context.Context, bool) update.Info {
	return update.Info{}
}
func (b_handlerreview *reviewBackend) DownloadUpdate() error { return nil }
func (b_handlerreview *reviewBackend) Storage(context.Context) app.StorageInfo {
	return app.StorageInfo{}
}
func (b_handlerreview *reviewBackend) SetStorage(string, bool) error {
	return b_handlerreview.storageErr
}
func (b_handlerreview *reviewBackend) Launch(string, string) error         { return nil }
func (b_handlerreview *reviewBackend) IntegrationLog(string, int) []string { return nil }
func (b_handlerreview *reviewBackend) Report(context.Context, string, bool, string) (report.Result, error) {
	b_handlerreview.reported++
	return report.Result{}, nil
}
func (b_handlerreview *reviewBackend) CloseApp(context.Context, string, bool) (scan.CloseResult, error) {
	return scan.CloseResult{}, nil
}

// reviewServer starts the real server on a loopback port and returns its base
// URL and token.
func reviewServer(t *testing.T, b_handlerreview Backend) (base, token string) {
	t.Helper()
	s, l, _, err := Listen(b_handlerreview, nil)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	go func() { _ = s.Serve(l) }()
	return "http://" + s.addr, s.Token()
}

func reviewDo(t *testing.T, method, url, ctype, token, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if token != "" {
		req.Header.Set("X-EZLM-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func reviewBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b_handlerreview, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b_handlerreview)
}

// A POST body is read without a limit, so one request can make the engine
// allocate arbitrarily much memory.
func TestHandlerRejectsOversizedBody(t *testing.T) {
	b_handlerreview := &reviewBackend{}
	base, token := reviewServer(t, b_handlerreview)
	big := `{"text":"` + strings.Repeat("x", 9<<20) + `"}`

	resp := reviewDo(t, "POST", base+"/api/report", "application/json", token, big)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body: status = %d, want 413", resp.StatusCode)
	}
	if b_handlerreview.reported != 0 {
		t.Fatalf("oversized body reached the backend %d times", b_handlerreview.reported)
	}
}

// Malformed JSON is decoded into a zero value and the action still runs.
func TestHandlerRejectsMalformedJSON(t *testing.T) {
	b_handlerreview := &reviewBackend{}
	base, token := reviewServer(t, b_handlerreview)

	resp := reviewDo(t, "POST", base+"/api/keys/revoke", "application/json", token, `{"id":`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed body: status = %d, want 400", resp.StatusCode)
	}
	if len(b_handlerreview.revoked) != 0 {
		t.Fatalf("malformed body revoked %q", b_handlerreview.revoked)
	}
}

// A browser can send text/plain without a preflight, so the API must not
// accept it as JSON.
func TestHandlerRejectsNonJSONContentType(t *testing.T) {
	b_handlerreview := &reviewBackend{}
	base, token := reviewServer(t, b_handlerreview)

	resp := reviewDo(t, "POST", base+"/api/keys/revoke", "text/plain", token, `{"id":"k1"}`)
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain body: status = %d, want 415", resp.StatusCode)
	}
	if len(b_handlerreview.revoked) != 0 {
		t.Fatalf("text/plain body revoked %q", b_handlerreview.revoked)
	}
}

// Backend errors are returned verbatim, including absolute home paths.
func TestHandlerRedactsHomePathFromError(t *testing.T) {
	t.Setenv("EZLM_HOME", "/Users/example/.ezlm")

	b_handlerreview := &reviewBackend{storageErr: errors.New("/Users/example/.ezlm/models: permission denied")}
	base, token := reviewServer(t, b_handlerreview)

	resp := reviewDo(t, "POST", base+"/api/storage", "application/json", token, `{"dir":"/Volumes/Models"}`)
	got := reviewBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("storage error: status = %d, want 400", resp.StatusCode)
	}
	if strings.Contains(got, "/Users/example") {
		t.Fatalf("error leaks the absolute home path: %s", got)
	}
	if !strings.Contains(got, "~/models") {
		t.Fatalf("error lost the useful path: %s", got)
	}
}

// The embedded file server must not serve files outside the static tree.
func TestFileServerNoPathTraversal(t *testing.T) {
	b_handlerreview := &reviewBackend{}
	base, token := reviewServer(t, b_handlerreview)

	// The UI itself still works.
	resp := reviewDo(t, "GET", base+"/", "", token, "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(reviewBody(t, resp), "<!doctype html>") {
		t.Fatalf("GET / = %d, want the UI", resp.StatusCode)
	}

	for _, p := range []string{
		"/../server.go",
		"/%2e%2e/server.go",
		"/..%2fserver.go",
		"/static/../../server.go",
		"/../../../../etc/passwd",
		"//etc/passwd",
		"/%2e%2e%2f%2e%2e%2fetc/passwd",
	} {
		resp := reviewDo(t, "GET", base+p, "", token, "")
		got := reviewBody(t, resp)
		if strings.Contains(got, "package web") || strings.Contains(got, "root:") {
			t.Fatalf("GET %s served a file outside static: %d %.80q", p, resp.StatusCode, got)
		}
	}
}

// Method-specific routes must not fall through to another handler.
func TestMethodConfusion(t *testing.T) {
	b_handlerreview := &reviewBackend{}
	base, token := reviewServer(t, b_handlerreview)

	cases := []struct {
		method, path string
		want         int
	}{
		{"POST", "/api/status", http.StatusMethodNotAllowed},
		{"GET", "/api/stop", http.StatusNotFound}, // falls to the file server
		{"PUT", "/api/report", http.StatusMethodNotAllowed},
		{"DELETE", "/api/keys/revoke", http.StatusMethodNotAllowed},
		{"HEAD", "/api/status", http.StatusOK},
	}
	for _, c := range cases {
		resp := reviewDo(t, c.method, base+c.path, "application/json", token, "{}")
		if resp.StatusCode != c.want {
			t.Fatalf("%s %s = %d, want %d", c.method, c.path, resp.StatusCode, c.want)
		}
	}
	if b_handlerreview.stopped != 0 || b_handlerreview.reported != 0 || len(b_handlerreview.revoked) != 0 {
		t.Fatalf("wrong-method request reached a handler: stopped=%d reported=%d revoked=%q", b_handlerreview.stopped, b_handlerreview.reported, b_handlerreview.revoked)
	}
}

// An unknown API path must not fall back to the UI or a directory listing.
func TestUnknownAPIPathIs404(t *testing.T) {
	b_handlerreview := &reviewBackend{}
	base, token := reviewServer(t, b_handlerreview)

	resp := reviewDo(t, "GET", base+"/api/nope", "", token, "")
	got := reviewBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /api/nope = %d, want 404", resp.StatusCode)
	}
	if strings.Contains(got, "<!doctype html>") {
		t.Fatalf("unknown API path served the UI")
	}
}
