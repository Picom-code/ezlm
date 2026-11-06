package serve

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// A port something is listening on is never free, even where the OS would
// let a second listener bind it (Windows with llama-server's SO_REUSEADDR).
func TestPortFreeSeesListener(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if PortFree(port) {
		t.Fatalf("port %d has a listener but was reported free", port)
	}
	l.Close()
	if !PortFree(port) {
		t.Fatalf("port %d was closed but is still reported busy", port)
	}
}

// Ours tells EZLM's server apart from another llama-server on the same port.
func TestOursChecksKeyAndModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ezlm-right" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"data":[{"id":"qwen3.6-35b-a3b"}]}`)
	}))
	defer srv.Close()
	port, _ := strconv.Atoi(srv.URL[strings.LastIndex(srv.URL, ":")+1:])
	ctx := context.Background()
	for _, tc := range []struct {
		key, model string
		want       bool
	}{
		{"ezlm-right", "qwen3.6-35b-a3b", true},
		{"ezlm-wrong", "qwen3.6-35b-a3b", false}, // another server with its own key
		{"ezlm-right", "granite-4.2-3b", false},  // same key, different model: not ours
	} {
		c := &Config{Port: port, Key: tc.key, ModelID: tc.model}
		if got := Ours(ctx, c); got != tc.want {
			t.Errorf("Ours(key %s, model %s) = %v, want %v", tc.key, tc.model, got, tc.want)
		}
	}
}

// A test instance (own EZLM_HOME) never shares the real install's service name.
func TestTestInstanceServiceName(t *testing.T) {
	t.Setenv("EZLM_HOME", "")
	if s := testSuffix(); s != "" {
		t.Fatalf("real install got suffix %q", s)
	}
	t.Setenv("EZLM_HOME", "/tmp/ezlm-test-home-a")
	a := testSuffix()
	t.Setenv("EZLM_HOME", "/tmp/ezlm-test-home-b")
	if b := testSuffix(); a == "" || a == b || !strings.HasPrefix(a, "-test-") {
		t.Fatalf("suffixes %q and %q should be distinct test names", a, b)
	}
}
