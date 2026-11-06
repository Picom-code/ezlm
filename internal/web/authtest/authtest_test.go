package authtest_test

import (
	"net/http"
	"testing"

	"github.com/picom-code/ezlm/internal/web/authtest"
)

// The token the server minted is the one it accepts.
func TestTokenIsRequired(t *testing.T) {
	h := authtest.New(t)

	resp := h.Get(t, "/api/status")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("with token: status %d, want 200", resp.StatusCode)
	}
	authtest.Body(t, resp)

	if resp := h.GetNoToken(t, "/api/status"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("without token: status %d, want 401", resp.StatusCode)
	}
	req := h.NoToken(t, http.MethodGet, "/api/status", nil)
	req.Header.Set("X-EZLM-Token", "REDACTED-KEY")
	if resp := h.Do(t, req); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong token: status %d, want 401", resp.StatusCode)
	}
}

// A request from a rebinding page carries its own Host header.
func TestHostMustBeLoopback(t *testing.T) {
	h := authtest.New(t)

	req := h.Request(t, http.MethodGet, "/api/status", nil)
	req.Host = "attacker.example"
	if resp := h.Do(t, req); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign Host: status %d, want 403", resp.StatusCode)
	}
	if len(h.Stub.Calls()) != 0 {
		t.Fatalf("foreign Host reached the backend: %v", h.Stub.Calls())
	}
}

// A request that passes the guard reaches the backend once.
func TestRequestReachesBackend(t *testing.T) {
	h := authtest.New(t)

	resp := h.PostJSON(t, "/api/deploy", map[string]string{"model_id": "granite-4.2-3b"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	authtest.Body(t, resp)
	if !h.Stub.Called("Deploy") {
		t.Fatalf("backend calls %v, want Deploy", h.Stub.Calls())
	}
	if n := len(h.Stub.Calls()); n != 1 {
		t.Fatalf("backend calls %v, want exactly one", h.Stub.Calls())
	}
}

// A form-encoded POST from a page with no token never reaches the backend.
func TestPostWithoutTokenIsRejected(t *testing.T) {
	h := authtest.New(t)

	resp := h.PostNoToken(t, "/api/deploy", "text/plain", `{"model_id":"x"}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", resp.StatusCode)
	}
	authtest.Body(t, resp)
	if h.Stub.Called("Deploy") {
		t.Fatalf("unauthenticated POST reached the backend: %v", h.Stub.Calls())
	}
}

// A hook changes one method's result without replacing the stub.
func TestHookChangesResult(t *testing.T) {
	h := authtest.New(t)
	h.Stub.OnDeploy = func(modelID string) error {
		if modelID != "granite-4.2-3b" {
			t.Errorf("Deploy(%q), want the model id from the body", modelID)
		}
		return nil
	}

	resp := h.PostJSON(t, "/api/deploy", map[string]string{"model_id": "granite-4.2-3b"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	var out map[string]bool
	authtest.Decode(t, resp, &out)
	if !out["ok"] {
		t.Fatalf("body %v, want ok true", out)
	}
}
