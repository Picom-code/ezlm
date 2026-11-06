// Package web serves EZLM's local, browser-based interface.
package web

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/picom-code/ezlm/internal/app"
	"github.com/picom-code/ezlm/internal/integrate"
	"github.com/picom-code/ezlm/internal/keys"
	"github.com/picom-code/ezlm/internal/report"
	"github.com/picom-code/ezlm/internal/scan"
	"github.com/picom-code/ezlm/internal/update"
)

//go:embed static
var static embed.FS

// Backend is what the UI drives: the real app, or the scripted demo.
type Backend interface {
	Status(ctx context.Context) app.Status
	Scan(ctx context.Context, pref string) *app.ScanResult
	Deploy(modelID string) error
	Kill(ctx context.Context, pid int32) error
	SetLAN(on bool) error
	Integrate(ids []string, makeDefault bool) error
	AddKey(name string) error
	RotateKey(id string) error
	RevokeKey(id string) error
	Stop() error
	Start() error
	Uninstall() error
	Subscribe() (chan app.Event, func())
	Live(ctx context.Context) app.Live
	Test() error
	Cancel() error
	Installed(ctx context.Context, pref string) []app.InstalledModel
	Use(ctx context.Context, modelID, quant, pref string) error
	Remove(ctx context.Context, modelID, quant string) error
	Logs(n int) []string
	Apps(ctx context.Context) []scan.App
	UpdateInfo(ctx context.Context, force bool) update.Info
	DownloadUpdate() error
	Storage(ctx context.Context) app.StorageInfo
	SetStorage(dir string, move bool) error
	Launch(id, mode string) error
	IntegrationLog(id string, n int) []string
	Report(ctx context.Context, text string, details bool, appLog string) (report.Result, error)
	CloseApp(ctx context.Context, id string, force bool) (scan.CloseResult, error)
}

type Server struct {
	app   Backend
	token string
	addr  string
	port  string
	// onActivity is called on every request; the CLI uses it for idle shutdown.
	onActivity func()
}

// Token is the per-launch secret API callers must present.
func (s *Server) Token() string { return s.token }

// Addr is the host:port the server listens on.
func (s *Server) Addr() string { return s.addr }

// Listen binds a loopback port and returns the URL to open (with its token).
func Listen(a Backend, onActivity func()) (*Server, net.Listener, string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, "", err
	}
	b := make([]byte, 16)
	rand.Read(b)
	_, port, _ := net.SplitHostPort(l.Addr().String())
	s := &Server{app: a, token: hex.EncodeToString(b), addr: l.Addr().String(), port: port, onActivity: onActivity}
	return s, l, fmt.Sprintf("http://%s/?t=%s", s.addr, s.token), nil
}

func (s *Server) Serve(l net.Listener) error {
	srv := &http.Server{Handler: s.handler(), ReadHeaderTimeout: 10 * time.Second}
	return srv.Serve(l)
}

// handler builds the routes and the wrappers every request passes through.
func (s *Server) handler() http.Handler {
	sub, _ := fs.Sub(static, "static")
	mux := http.NewServeMux()
	staticFS := http.FileServerFS(sub)
	mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The embedded file system rejects names that are not valid UTF-8
		// with an error http.FileServer turns into a 500. A bad path from a
		// caller is a 404, not a server error.
		if !utf8.ValidString(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		staticFS.ServeHTTP(w, r)
	}))
	mux.HandleFunc("GET /api/status", s.api(func(r *http.Request) (any, error) { return s.app.Status(r.Context()), nil }))
	mux.HandleFunc("POST /api/scan", s.api(func(r *http.Request) (any, error) {
		var in struct {
			Pref string `json:"pref"`
		}
		if err := decodeJSON(r, &in); err != nil {
			return nil, err
		}
		return s.app.Scan(r.Context(), in.Pref), nil
	}))
	mux.HandleFunc("POST /api/deploy", s.api(func(r *http.Request) (any, error) {
		var in struct {
			ModelID string `json:"model_id"`
		}
		if err := decodeJSON(r, &in); err != nil {
			return nil, err
		}
		return ok(s.app.Deploy(in.ModelID))
	}))
	mux.HandleFunc("POST /api/kill", s.api(func(r *http.Request) (any, error) {
		var in struct {
			PID int32 `json:"pid"`
		}
		if err := decodeJSON(r, &in); err != nil {
			return nil, err
		}
		return ok(s.app.Kill(r.Context(), in.PID))
	}))
	mux.HandleFunc("POST /api/lan", s.api(func(r *http.Request) (any, error) {
		var in struct {
			On bool `json:"on"`
		}
		if err := decodeJSON(r, &in); err != nil {
			return nil, err
		}
		return ok(s.app.SetLAN(in.On))
	}))
	mux.HandleFunc("POST /api/integrate", s.api(func(r *http.Request) (any, error) {
		var in struct {
			IDs     []string `json:"ids"`
			Default bool     `json:"default"`
		}
		if err := decodeJSON(r, &in); err != nil {
			return nil, err
		}
		return ok(s.app.Integrate(in.IDs, in.Default))
	}))
	mux.HandleFunc("POST /api/keys/add", s.api(func(r *http.Request) (any, error) {
		var in struct {
			Name string `json:"name"`
		}
		if err := decodeJSON(r, &in); err != nil {
			return nil, err
		}
		return ok(s.app.AddKey(in.Name))
	}))
	mux.HandleFunc("POST /api/keys/rotate", s.api(func(r *http.Request) (any, error) {
		var in struct {
			ID string `json:"id"`
		}
		if err := decodeJSON(r, &in); err != nil {
			return nil, err
		}
		return ok(s.app.RotateKey(in.ID))
	}))
	mux.HandleFunc("POST /api/keys/revoke", s.api(func(r *http.Request) (any, error) {
		var in struct {
			ID string `json:"id"`
		}
		if err := decodeJSON(r, &in); err != nil {
			return nil, err
		}
		return ok(s.app.RevokeKey(in.ID))
	}))
	mux.HandleFunc("POST /api/installed", s.api(func(r *http.Request) (any, error) {
		var in struct {
			Pref string `json:"pref"`
		}
		if err := decodeJSON(r, &in); err != nil {
			return nil, err
		}
		return map[string]any{"models": s.app.Installed(r.Context(), in.Pref)}, nil
	}))
	mux.HandleFunc("GET /api/update", s.api(func(r *http.Request) (any, error) {
		return s.app.UpdateInfo(r.Context(), r.URL.Query().Get("force") != ""), nil
	}))
	mux.HandleFunc("POST /api/update/download", s.api(func(*http.Request) (any, error) { return ok(s.app.DownloadUpdate()) }))
	mux.HandleFunc("GET /api/storage", s.api(func(r *http.Request) (any, error) { return s.app.Storage(r.Context()), nil }))
	mux.HandleFunc("POST /api/storage", s.api(func(r *http.Request) (any, error) {
		var in struct {
			Dir  string `json:"dir"`
			Move bool   `json:"move"`
		}
		if err := decodeJSON(r, &in); err != nil {
			return nil, err
		}
		return ok(s.app.SetStorage(in.Dir, in.Move))
	}))
	mux.HandleFunc("POST /api/launch", s.api(func(r *http.Request) (any, error) {
		var in struct {
			ID   string `json:"id"`
			Mode string `json:"mode"` // optional: a target's launch mode id
		}
		if err := decodeJSON(r, &in); err != nil {
			return nil, err
		}
		return ok(s.app.Launch(in.ID, in.Mode))
	}))
	mux.HandleFunc("GET /api/integrate/log", s.api(func(r *http.Request) (any, error) {
		return map[string]any{"lines": redactLines(s.app.IntegrationLog(r.URL.Query().Get("id"), 200))}, nil
	}))
	mux.HandleFunc("POST /api/report", s.api(func(r *http.Request) (any, error) {
		var in struct {
			Text    string `json:"text"`
			Details bool   `json:"details"`
			AppLog  string `json:"app_log"`
		}
		if err := decodeJSON(r, &in); err != nil {
			return nil, err
		}
		return s.app.Report(r.Context(), in.Text, in.Details, in.AppLog)
	}))
	mux.HandleFunc("GET /api/apps", s.api(func(r *http.Request) (any, error) {
		return map[string]any{"apps": s.app.Apps(r.Context())}, nil
	}))
	mux.HandleFunc("POST /api/apps/close", s.api(func(r *http.Request) (any, error) {
		var in struct {
			ID    string `json:"id"`
			Force bool   `json:"force"`
		}
		if err := decodeJSON(r, &in); err != nil {
			return nil, err
		}
		return s.app.CloseApp(r.Context(), in.ID, in.Force)
	}))
	mux.HandleFunc("POST /api/use", s.api(func(r *http.Request) (any, error) {
		var in struct {
			ModelID string `json:"model_id"`
			Quant   string `json:"quant"`
			Pref    string `json:"pref"`
		}
		if err := decodeJSON(r, &in); err != nil {
			return nil, err
		}
		return ok(s.app.Use(r.Context(), in.ModelID, in.Quant, in.Pref))
	}))
	mux.HandleFunc("POST /api/remove", s.api(func(r *http.Request) (any, error) {
		var in struct {
			ModelID string `json:"model_id"`
			Quant   string `json:"quant"`
		}
		if err := decodeJSON(r, &in); err != nil {
			return nil, err
		}
		return ok(s.app.Remove(r.Context(), in.ModelID, in.Quant))
	}))
	mux.HandleFunc("POST /api/cancel", s.api(func(*http.Request) (any, error) { return ok(s.app.Cancel()) }))
	mux.HandleFunc("POST /api/test", s.api(func(*http.Request) (any, error) { return ok(s.app.Test()) }))
	mux.HandleFunc("POST /api/stop", s.api(func(*http.Request) (any, error) { return ok(s.app.Stop()) }))
	mux.HandleFunc("POST /api/start", s.api(func(*http.Request) (any, error) { return ok(s.app.Start()) }))
	mux.HandleFunc("POST /api/uninstall", s.api(func(*http.Request) (any, error) { return ok(s.app.Uninstall()) }))
	mux.HandleFunc("GET /api/live", s.api(func(r *http.Request) (any, error) { return s.app.Live(r.Context()), nil }))
	mux.HandleFunc("GET /api/logs", s.api(func(*http.Request) (any, error) { return map[string]any{"lines": redactLines(s.app.Logs(300))}, nil }))
	mux.HandleFunc("GET /api/events", s.guard(s.events))

	return s.hostCheck(s.cors(mux))
}

func ok(err error) (any, error) { return map[string]bool{"ok": err == nil}, err }

// redactLines removes keys from log lines a backend hands the page. Logs come
// from other programs (the model server, installers), so no line may carry a
// key into the page, a screenshot or a problem report.
func redactLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = integrate.Redact(l)
	}
	return out
}

// hostCheck rejects requests whose Host isn't our loopback address, which
// blocks DNS-rebinding attacks from web pages.
func (s *Server) hostCheck(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hostOK(r.Host) || !loopbackPeer(r.RemoteAddr) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if s.onActivity != nil {
			s.onActivity()
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		h.ServeHTTP(w, r)
	})
}

// hostOK accepts the address we listen on, plus the loopback spellings of
// it. A hostname that merely resolves to 127.0.0.1 is not enough: DNS
// rebinding relies on exactly that, so only literals and "localhost" pass.
func (s *Server) hostOK(host string) bool {
	if host == s.addr {
		return true
	}
	h, p, err := net.SplitHostPort(host)
	if err != nil || p != s.port {
		return false
	}
	h = strings.ToLower(h)
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// cors states the cross-origin policy: same origin only. The UI calls the API
// on the origin it was served from, so no other origin is ever allowed to read
// a response. A page elsewhere can still send a simple request, but without
// Access-Control-Allow-Origin the browser hides the answer. Preflights are
// answered here so they fail closed instead of landing on a 405.
func (s *Server) cors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		if origin := r.Header.Get("Origin"); origin == "http://"+s.addr {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// guard requires the per-launch token in a header. A GET may carry it in the
// query instead, because EventSource cannot set headers; a state change must
// use the header, which a browser cannot send cross-origin without a preflight.
func (s *Server) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := r.Header.Get("X-EZLM-Token")
		// Only the event stream may carry the token in the query: EventSource
		// cannot set headers, and a GET is safe to replay from history or a
		// Referer. Every other route needs the header.
		if t == "" && r.Method == http.MethodGet && r.URL.Path == "/api/events" {
			t = r.URL.Query().Get("t")
		}
		if !keys.Equal(t, s.token) {
			http.Error(w, "this page is out of date: reopen EZLM", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

func (s *Server) api(fn func(r *http.Request) (any, error)) http.HandlerFunc {
	return s.guard(func(w http.ResponseWriter, r *http.Request) {
		// A browser sends text/plain and form bodies cross-origin without a
		// preflight; application/json forces one, so a page cannot POST here.
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !isJSON(r.Header.Get("Content-Type")) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnsupportedMediaType)
			json.NewEncoder(w).Encode(map[string]string{"error": "send a JSON body"})
			return
		}
		v, err := fn(r)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			status := http.StatusBadRequest
			var ae *apiError
			if errors.As(err, &ae) {
				status = ae.status
			}
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(map[string]string{"error": redactPaths(err.Error())})
			return
		}
		json.NewEncoder(w).Encode(v)
	})
}

// isJSON reports whether ct is application/json, with or without parameters.
func isJSON(ct string) bool {
	mt, _, err := mime.ParseMediaType(ct)
	return err == nil && mt == "application/json"
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	ch, cancel := s.app.Subscribe()
	defer cancel()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	fl.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
			if s.onActivity != nil {
				s.onActivity()
			}
		case e := <-ch:
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		}
	}
}
