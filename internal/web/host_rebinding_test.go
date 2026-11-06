package web

// T07/a01: DNS rebinding against the engine API.
//
// A malicious page at attacker.example is loaded in the user's browser. The
// attacker's DNS answers 127.0.0.1 on a short TTL, so the page's own origin
// now points at the loopback engine. The browser keeps sending the page's
// hostname in Host ("attacker.example:PORT"); the engine must reject it,
// because otherwise the page reads the user's status, keys and logs.
//
// The tests below drive the real handler chain with httptest-style requests
// and vary only the Host header: same server, same path, same token.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/app"
)

// rebindStub answers /api/status; every other method is unused by these
// tests and panics if called.
type rebindStub struct{ Backend }

func (rebindStub) Status(context.Context) app.Status {
	return app.Status{Deployed: true, Healthy: true, LocalURL: "http://127.0.0.1:8080/v1"}
}

// startRebindServer starts the real server on a loopback port and returns it
// with the port it listens on.
func startRebindServer(t *testing.T) (*Server, string) {
	t.Helper()
	s, l, _, err := Listen(rebindStub{}, nil)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go s.Serve(l)
	t.Cleanup(func() { l.Close() })
	return s, s.Addr()
}

// get sends one GET to the running server with an explicit Host header and
// returns the status code and body.
func get(t *testing.T, addr, host, path, token string) (int, string) {
	t.Helper()
	req, err := http.NewRequest("GET", "http://"+addr+path, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Host = host
	if token != "" {
		req.Header.Set("X-EZLM-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s (Host %q): %v", path, host, err)
	}
	defer resp.Body.Close()
	var b strings.Builder
	buf := make([]byte, 512)
	for {
		n, err := resp.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp.StatusCode, b.String()
}

// TestHostRebindingReadsAPI is the proof: a rebinding page reaches
// /api/status without a token in its URL, just by carrying its own hostname.
func TestHostRebindingReadsAPI(t *testing.T) {
	s, addr := startRebindServer(t)

	// The page's origin is attacker.example; the socket is our loopback one.
	code, body := get(t, addr, "attacker.example:"+port(addr), "/api/status", s.Token())
	if code != http.StatusForbidden {
		t.Fatalf("rebinding Host got %d, want 403 (body %q); the page can read the engine API", code, body)
	}
	if !strings.Contains(body, "forbidden") {
		t.Fatalf("403 body = %q, want %q", body, "forbidden")
	}

	// The same request with the real Host still works, so the 403 above is
	// the Host check and not a broken server.
	code, body = get(t, addr, addr, "/api/status", s.Token())
	if code != http.StatusOK {
		t.Fatalf("loopback Host got %d, want 200 (body %q)", code, body)
	}
	var st app.Status
	if err := json.Unmarshal([]byte(body), &st); err != nil || !st.Deployed {
		t.Fatalf("status body = %q (err %v), want the app status", body, err)
	}
}

// TestHostRebindingHosts is the allowlist: loopback names on our port are
// the user's own URL, everything else is an attack.
func TestHostRebindingHosts(t *testing.T) {
	s, addr := startRebindServer(t)
	p := port(addr)

	for _, tc := range []struct {
		host string
		want int
	}{
		{"127.0.0.1:" + p, http.StatusOK},
		{"localhost:" + p, http.StatusOK},
		{"LOCALHOST:" + p, http.StatusOK},
		{"attacker.example:" + p, http.StatusForbidden},
		{"attacker.example", http.StatusForbidden}, // no port
		{"127.0.0.1", http.StatusForbidden},        // no port
		{"127.0.0.1:1", http.StatusForbidden},      // another port
		{"127.0.0.1:" + p + ".evil", http.StatusForbidden},
		{"evil.example:127.0.0.1:" + p, http.StatusForbidden},
		{"127.0.0.1:" + p + " ", http.StatusForbidden},
		{"[::1]:" + p, http.StatusOK}, // any loopback name on our port is the user
	} {
		code, body := get(t, addr, tc.host, "/api/status", s.Token())
		if code != tc.want {
			t.Errorf("Host %q got %d, want %d (body %q)", tc.host, code, tc.want, body)
		}
	}
}

// TestHostCheckRunsBeforeToken: a rebinding page can guess the token (it is
// in the URL the user opened), so the Host check must not depend on it.
func TestHostCheckRunsBeforeToken(t *testing.T) {
	s, addr := startRebindServer(t)

	code, body := get(t, addr, "attacker.example:"+port(addr), "/api/status", "REDACTED-KEY")
	if code != http.StatusForbidden {
		t.Fatalf("foreign Host with a wrong token got %d, want 403 (body %q)", code, body)
	}
	code, body = get(t, addr, "attacker.example:"+port(addr), "/api/logs", s.Token())
	if code != http.StatusForbidden {
		t.Fatalf("foreign Host with the real token got %d, want 403 (body %q)", code, body)
	}
}

// port returns the port part of a host:port address.
func port(addr string) string {
	if i := strings.LastIndexByte(addr, ':'); i >= 0 {
		return addr[i+1:]
	}
	return addr
}
