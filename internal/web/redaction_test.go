package web

// Redaction: keys and tokens must not reach a log, a report, an error string
// or the page. These tests drive the engine API and the model-server log tail
// with secrets in them and check that none survives.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/integrate"
	"github.com/picom-code/ezlm/internal/serve"
)

const (
	testKey    = "ezlm-AbCdEfGhIjKlMnOpQrStUvWxYz0123456789ab"
	testSK     = "sk-proj-1234567890abcdef"
	testBearer = "abcdefghijklmnop"
)

// logBackend answers the two log endpoints; every other method is nil and
// must not be called by the routes under test.
type logBackend struct {
	Backend
	logs  []string
	ilogs []string
}

func (b_redaction *logBackend) Logs(int) []string                   { return b_redaction.logs }
func (b_redaction *logBackend) IntegrationLog(string, int) []string { return b_redaction.ilogs }

// startEngine serves the real engine API on a loopback port.
func startEngine(t *testing.T, b_redaction Backend) (*Server, string) {
	t.Helper()
	s, l, _, err := Listen(b_redaction, nil)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		s.Serve(l)
		close(done)
	}()
	t.Cleanup(func() {
		l.Close()
		<-done
	})
	return s, "http://" + s.Addr()
}

func getJSON(t *testing.T, s *Server, base, path string, v any) {
	t.Helper()
	req, err := http.NewRequest("GET", base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-EZLM-Token", s.Token())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %s", path, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
}

func assertNoSecret(t *testing.T, where, text string) {
	t.Helper()
	for _, secret := range []string{testKey, testSK, testBearer} {
		if strings.Contains(text, secret) {
			t.Errorf("%s leaked %q: %s", where, secret, text)
		}
	}
}

// A line the model server wrote with a key in it must not reach the page.
func TestAPILogsRedactKeys(t *testing.T) {
	b_redaction := &logBackend{logs: []string{
		"main: server is listening on http://127.0.0.1:8081",
		"main: args --api-key " + testKey,
		"auth: Bearer " + testBearer,
		"client sent " + testSK,
	}}
	s, base := startEngine(t, b_redaction)
	var out struct {
		Lines []string `json:"lines"`
	}
	getJSON(t, s, base, "/api/logs", &out)
	if len(out.Lines) != len(b_redaction.logs) {
		t.Fatalf("lines = %d, want %d", len(out.Lines), len(b_redaction.logs))
	}
	assertNoSecret(t, "/api/logs", strings.Join(out.Lines, "\n"))
}

// Integration logs are written with keys removed, but the endpoint is the
// last line of defence and must not pass one through either.
func TestIntegrationLogRedactsKeys(t *testing.T) {
	b_redaction := &logBackend{ilogs: []string{
		"run: opencode open with key " + testKey,
		"exit after 1.0 s: ok",
	}}
	s, base := startEngine(t, b_redaction)
	var out struct {
		Lines []string `json:"lines"`
	}
	getJSON(t, s, base, "/api/integrate/log?id=opencode", &out)
	if len(out.Lines) != len(b_redaction.ilogs) {
		t.Fatalf("lines = %d, want %d", len(out.Lines), len(b_redaction.ilogs))
	}
	assertNoSecret(t, "/api/integrate/log", strings.Join(out.Lines, "\n"))
}

// TailLog feeds the log view, the "last log lines" of a load error and the
// report. A key in the file must not survive it.
func TestTailLogRedactsKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("EZLM_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := strings.Join([]string{
		"main: args --api-key " + testKey,
		"auth: Bearer " + testBearer,
		"client sent " + testSK,
	}, "\n") + "\n"
	if err := os.WriteFile(serve.LogPath(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got := serve.TailLog(8)
	if strings.TrimSpace(got) == "" {
		t.Fatal("TailLog returned nothing")
	}
	assertNoSecret(t, "serve.TailLog", got)
	// The lines still have to be readable: the key is shortened, not cut out.
	if !strings.Contains(got, "ezlm-AbCd…") {
		t.Errorf("TailLog lost the key prefix: %s", got)
	}
}

// A fatal line from the server log becomes an error the user sees, so it is
// redacted too.
func TestFatalInLogRedactsKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("EZLM_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	line := "error loading model with key " + testKey
	if err := os.WriteFile(serve.LogPath(), []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := serve.FatalInLog()
	if got == "" {
		t.Fatal("FatalInLog found nothing")
	}
	assertNoSecret(t, "serve.FatalInLog", got)
}

// The redactor used for reports removes the literal key, any EZLM key, any
// sk- key and any Bearer token.
func TestRedactCoversReportSecrets(t *testing.T) {
	body := strings.Join([]string{
		"key=" + testKey,
		"another=ezlm-ZZZZZZZZZZZZ",
		"Authorization: Bearer " + testBearer,
		"client " + testSK,
	}, "\n")
	got := integrate.Redact(body, testKey)
	assertNoSecret(t, "integrate.Redact", got)
	if strings.Contains(got, "ZZZZZZZZZZZZ") {
		t.Errorf("an unlisted EZLM key survived: %s", got)
	}
}

// The per-launch web token is 32 hex characters, which no redaction pattern
// matches. This test pins the gap reported in agents/T07/a08/REPORT.md: the
// token is only in the setup URL and ui.url today, but a report that quotes
// the URL would publish it. The redactor lives in the frozen internal/
// integrate tree, so this is report-only.
func TestRedactGapPerLaunchToken(t *testing.T) {
	token := strings.Repeat("a1", 16) // as hex.EncodeToString(16 bytes) makes it
	got := integrate.Redact("Setup page: http://127.0.0.1:52757/?t=" + token)
	if !strings.Contains(got, token) {
		t.Fatalf("token now redacted; update REPORT.md and delete this test: %s", got)
	}
}
