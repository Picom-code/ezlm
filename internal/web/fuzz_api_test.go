package web_test

// Fuzz target for the engine API's request parsing: method, path, Host
// header, token header, content type and body. The invariants are:
//
//   - the server never panics, whatever it is sent;
//   - malformed input never produces a 5xx (a bad request is a 4xx);
//   - no response leaks a path from this computer (a working directory,
//     a home directory or a temp directory).
//
// The server is started through web.Listen and web.Server.Serve, so the
// fuzz corpus goes through the real listener, host check, token guard, mux
// and static file server rather than a copy of the routing table.

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/picom-code/ezlm/internal/app"
	"github.com/picom-code/ezlm/internal/report"
	"github.com/picom-code/ezlm/internal/scan"
	"github.com/picom-code/ezlm/internal/update"
	"github.com/picom-code/ezlm/internal/web"
)

// fuzzBackend is a Backend that answers with cheap, fixed values. Nothing it
// does touches the filesystem or the network.
type fuzzBackend struct {
	events chan app.Event
}

func (b_fuzzapi *fuzzBackend) Status(context.Context) app.Status { return app.Status{Version: "test"} }
func (b_fuzzapi *fuzzBackend) Scan(context.Context, string) *app.ScanResult {
	return &app.ScanResult{Pref: "balanced"}
}
func (b_fuzzapi *fuzzBackend) Deploy(string) error               { return nil }
func (b_fuzzapi *fuzzBackend) Kill(context.Context, int32) error { return nil }
func (b_fuzzapi *fuzzBackend) SetLAN(bool) error                 { return nil }
func (b_fuzzapi *fuzzBackend) Integrate([]string, bool) error    { return nil }
func (b_fuzzapi *fuzzBackend) AddKey(string) error               { return nil }
func (b_fuzzapi *fuzzBackend) RotateKey(string) error            { return nil }
func (b_fuzzapi *fuzzBackend) RevokeKey(string) error            { return nil }
func (b_fuzzapi *fuzzBackend) Stop() error                       { return nil }
func (b_fuzzapi *fuzzBackend) Start() error                      { return nil }
func (b_fuzzapi *fuzzBackend) Uninstall() error                  { return nil }
func (b_fuzzapi *fuzzBackend) Subscribe() (chan app.Event, func()) {
	return b_fuzzapi.events, func() {}
}
func (b_fuzzapi *fuzzBackend) Live(context.Context) app.Live                          { return app.Live{} }
func (b_fuzzapi *fuzzBackend) Test() error                                            { return nil }
func (b_fuzzapi *fuzzBackend) Cancel() error                                          { return nil }
func (b_fuzzapi *fuzzBackend) Installed(context.Context, string) []app.InstalledModel { return nil }
func (b_fuzzapi *fuzzBackend) Use(context.Context, string, string, string) error      { return nil }
func (b_fuzzapi *fuzzBackend) Remove(context.Context, string, string) error           { return nil }
func (b_fuzzapi *fuzzBackend) Logs(int) []string                                      { return []string{"line"} }
func (b_fuzzapi *fuzzBackend) Apps(context.Context) []scan.App                        { return nil }
func (b_fuzzapi *fuzzBackend) UpdateInfo(context.Context, bool) update.Info {
	return update.Info{Current: "test"}
}
func (b_fuzzapi *fuzzBackend) DownloadUpdate() error { return nil }
func (b_fuzzapi *fuzzBackend) Storage(context.Context) app.StorageInfo {
	return app.StorageInfo{Dir: "/models"}
}
func (b_fuzzapi *fuzzBackend) SetStorage(string, bool) error       { return nil }
func (b_fuzzapi *fuzzBackend) Launch(string, string) error         { return nil }
func (b_fuzzapi *fuzzBackend) IntegrationLog(string, int) []string { return nil }
func (b_fuzzapi *fuzzBackend) Report(context.Context, string, bool, string) (report.Result, error) {
	return report.Result{}, nil
}
func (b_fuzzapi *fuzzBackend) CloseApp(context.Context, string, bool) (scan.CloseResult, error) {
	return scan.CloseResult{}, nil
}

// syncBuffer collects the standard logger's output while the fuzz server
// runs, so a panic recovered by net/http can be told apart from a client-side
// error.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b_fuzzapi *syncBuffer) Write(p []byte) (int, error) {
	b_fuzzapi.mu.Lock()
	defer b_fuzzapi.mu.Unlock()
	return b_fuzzapi.buf.Write(p)
}

func (b_fuzzapi *syncBuffer) reset() {
	b_fuzzapi.mu.Lock()
	defer b_fuzzapi.mu.Unlock()
	b_fuzzapi.buf.Reset()
}

func (b_fuzzapi *syncBuffer) String() string {
	b_fuzzapi.mu.Lock()
	defer b_fuzzapi.mu.Unlock()
	return b_fuzzapi.buf.String()
}

// startFuzzServer binds the real engine API on a loopback port and returns
// its address and token. The server and the panic log are stopped when the
// test (or the fuzz run) ends.
func startFuzzServer(f *testing.F) (addr, token string, logs *syncBuffer) {
	f.Helper()
	srv, l, _, err := web.Listen(&fuzzBackend{events: make(chan app.Event, 1)}, nil)
	if err != nil {
		f.Fatalf("listen: %v", err)
	}
	logs = &syncBuffer{}
	log.SetOutput(io.MultiWriter(log.Writer(), logs))
	f.Cleanup(func() {
		l.Close()
		log.SetOutput(os.Stderr)
	})
	go func() { _ = srv.Serve(l) }()
	return srv.Addr(), srv.Token(), logs
}

// leakRoots are the paths that must never appear in a response: the source
// tree, the user's home and the temp directory. A response that names one of
// them tells a remote caller where EZLM lives.
func leakRoots() []string {
	var roots []string
	if wd, err := os.Getwd(); err == nil {
		roots = append(roots, wd)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		roots = append(roots, home)
	}
	if tmp := os.TempDir(); tmp != "" {
		roots = append(roots, tmp)
	}
	return roots
}

func FuzzAPIRequest(f *testing.F) {
	addr, token, logs := startFuzzServer(f)
	client := &http.Client{Timeout: 5 * time.Second}
	leaks := leakRoots()

	// Seeds: valid calls, unauthorised calls, wrong methods and hosts, broken
	// JSON and paths that the mux or the file server have to reject.
	f.Add("GET", "/api/status", "", "valid", "", []byte(nil))
	f.Add("POST", "/api/scan", "", "valid", "application/json", []byte(`{"pref":"fastest"}`))
	f.Add("POST", "/api/deploy", "", "valid", "text/plain", []byte(`{"model_id":"x"}`))
	f.Add("POST", "/api/keys/add", "", "valid", "application/json", []byte(`{`))
	f.Add("POST", "/api/report", "", "valid", "application/json", []byte(`{"text":"hi","details":true}`))
	f.Add("GET", "/", "", "", "", []byte(nil))
	f.Add("GET", "/app.js", "", "", "", []byte(nil))
	f.Add("GET", "/api/events", "", "valid", "", []byte(nil))
	f.Add("GET", "/api/logs", "", "", "", []byte(nil))
	f.Add("DELETE", "/api/status", "", "valid", "", []byte(nil))
	f.Add("PUT", "/api/status", "", "valid", "application/x-www-form-urlencoded", []byte("a=b_fuzzapi"))
	f.Add("GET", "/api/status", "evil.example", "valid", "", []byte(nil))
	f.Add("GET", "/api/status", "localhost:1234", "valid", "", []byte(nil))
	f.Add("POST", "/api/status", "", "valid", "", []byte(nil))
	f.Add("GET", "/../../../../etc/passwd", "", "valid", "", []byte(nil))
	f.Add("GET", "/%00", "", "valid", "", []byte(nil))
	f.Add("GET", "/api/status/", "", "valid", "", []byte(nil))
	f.Add("GET", "/api/status?t=valid", "", "", "", []byte(nil))
	f.Add("OPTIONS", "/api/status", "", "valid", "", []byte(nil))
	f.Add("HEAD", "/", "", "", "", []byte(nil))

	f.Fuzz(func(t *testing.T, method, path, host, tok, ctype string, body []byte) {
		if len(body) > 32<<10 {
			return // keep each iteration cheap; the parser sees plenty
		}
		if method == "" {
			method = http.MethodGet
		}
		if path == "" {
			path = "/api/status"
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		req, err := http.NewRequest(method, "http://"+addr+path, bytes.NewReader(body))
		if err != nil {
			return // not a request net/http can even build; nothing reaches us
		}
		if host != "" {
			req.Host = host
		}
		switch tok {
		case "valid":
			req.Header.Set("X-EZLM-Token", token)
		case "":
			// no token: the guard must answer 401, not crash
		default:
			req.Header.Set("X-EZLM-Token", tok)
		}
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		req = req.WithContext(ctx)

		logs.reset()
		resp, err := client.Do(req)
		if p := logs.String(); strings.Contains(p, "panic") {
			t.Fatalf("%s %s: server panic: %s", method, path, p)
		}
		if err != nil {
			if ctx.Err() != nil || strings.Contains(err.Error(), "stopped after") {
				return // client gave up (redirect loop, stream, slow read)
			}
			return // malformed request that never reached the handler
		}
		defer resp.Body.Close()

		var out []byte
		if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
			out, _ = io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		}

		if resp.StatusCode >= 500 {
			t.Fatalf("%s %s (host %q, body %q): status %d: %s",
				method, path, host, body, resp.StatusCode, out)
		}
		for _, root := range leaks {
			if root != "" && bytes.Contains(out, []byte(root)) {
				t.Fatalf("%s %s: response leaks path %q: %s", method, path, root, out)
			}
		}
		loc := resp.Header.Get("Location")
		if loc != "" && !strings.Contains(loc, path) {
			for _, root := range leaks {
				if root != "" && strings.Contains(loc, root) {
					t.Fatalf("%s %s: redirect leaks path %q: %s", method, path, root, loc)
				}
			}
		}
	})
}
