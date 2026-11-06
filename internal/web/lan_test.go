package web

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/picom-code/ezlm/internal/app"
	"github.com/picom-code/ezlm/internal/fakellm"
	"github.com/picom-code/ezlm/internal/serve"
)

// lanBackend satisfies Backend; only Status is ever called here.
type lanBackend struct{ Backend }

func (lanBackend) Status(context.Context) app.Status { return app.Status{} }

// lanClient is a client that sends a Host header naming another machine,
// the way a LAN peer would when it connects to this machine's address.
func lanClient(t *testing.T, host string) *http.Client {
	t.Helper()
	return &http.Client{
		Timeout: 2 * time.Second,
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			r.Host = host
			return http.DefaultTransport.RoundTrip(r)
		}),
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// lanIP is this machine's address on the local network, or "" when it has none.
func lanIP(t *testing.T) string {
	t.Helper()
	conn, err := net.Dial("udp", "192.0.2.1:9") // no packets are sent
	if err != nil {
		return ""
	}
	defer conn.Close()
	ip := conn.LocalAddr().(*net.UDPAddr).IP
	if ip == nil || ip.IsLoopback() {
		return ""
	}
	return ip.String()
}

// TestEngineAPIIsLoopbackOnly proves that the setup API listens on loopback
// and that, even if a LAN peer reached the port, the Host check turns it away.
func TestEngineAPIIsLoopbackOnly(t *testing.T) {
	s, l, _, err := Listen(lanBackend{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go s.Serve(l)

	if ip := l.Addr().(*net.TCPAddr).IP; !ip.IsLoopback() {
		t.Fatalf("engine API listens on %s, not loopback", ip)
	}
	base := "http://" + s.Addr()

	local := func(path string, token string) *http.Request {
		req, _ := http.NewRequest("GET", base+path, nil)
		if token != "" {
			req.Header.Set("X-EZLM-Token", token)
		}
		return req
	}

	// The local page works.
	resp, err := http.DefaultClient.Do(local("/api/status", s.Token()))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("local request with token: got %d, want 200", resp.StatusCode)
	}

	// The token is required.
	resp, err = http.DefaultClient.Do(local("/api/status", ""))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("local request without token: got %d, want 401", resp.StatusCode)
	}

	// A LAN peer sends its own Host. Even with a valid token it gets nothing:
	// the Host check is the second line of defence behind the loopback bind.
	peer := lanClient(t, "192.168.1.20:"+portOf(t, s.Addr()))
	resp, err = peer.Do(local("/api/status", s.Token()))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("LAN Host with token: got %d, want 403", resp.StatusCode)
	}

	// And the port really is unreachable through this machine's LAN address.
	if ip := lanIP(t); ip != "" {
		c, err := net.DialTimeout("tcp", net.JoinHostPort(ip, portOf(t, s.Addr())), 500*time.Millisecond)
		if err == nil {
			c.Close()
			t.Fatalf("engine API answered on the LAN address %s", ip)
		}
	}
}

func portOf(t *testing.T, addr string) string {
	t.Helper()
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

// lanConn reports a non-loopback peer for a connection accepted on loopback,
// standing in for a LAN client if the engine API were ever bound wider.
type lanConn struct{ net.Conn }

func (lanConn) RemoteAddr() net.Addr { return &net.TCPAddr{IP: net.IPv4(192, 168, 1, 50), Port: 5555} }

type lanListener struct{ net.Listener }

func (l lanListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return lanConn{c}, nil
}

// TestEngineAPIRejectsNonLoopbackPeer is the proof for the fix: a request
// that reaches the engine API from another machine's address is refused even
// when it knows the Host and the token.
func TestEngineAPIRejectsNonLoopbackPeer(t *testing.T) {
	s := &Server{addr: "127.0.0.1:1234", token: "REDACTED-TOKEN"}
	h := s.hostCheck(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ts := httptest.NewUnstartedServer(h)
	ts.Listener = lanListener{ts.Listener}
	ts.Start()
	defer ts.Close()

	req, _ := http.NewRequest("GET", ts.URL, nil)
	req.Host = s.addr
	req.Header.Set("X-EZLM-Token", s.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("request from a LAN address: got %d, want 403", resp.StatusCode)
	}

	// A request from this computer still passes.
	local := httptest.NewServer(h)
	defer local.Close()
	req, _ = http.NewRequest("GET", local.URL, nil)
	req.Host = s.addr
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("local request: got %d, want 200", resp.StatusCode)
	}
}

// TestLANModeExposesModelServerOnly checks what LAN mode actually opens: the
// model server on every interface, protected by the key; the setup API stays
// on loopback. The fake model server stands in for llama-server.
func TestLANModeExposesModelServerOnly(t *testing.T) {
	const key = "REDACTED-KEY"
	fake := fakellm.New(fakellm.Options{Key: key, Alias: "lan-model"})

	l, err := net.Listen("tcp", "0.0.0.0:0") // what --host 0.0.0.0 does
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: fake}
	go srv.Serve(l)
	defer srv.Close()

	base := "http://" + l.Addr().String()
	peerHost := "192.168.1.20:" + portOf(t, l.Addr().String())

	get := func(client *http.Client, auth string) int {
		t.Helper()
		req, _ := http.NewRequest("GET", base+"/v1/models", nil)
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	// A LAN peer without the key gets nothing.
	if got := get(lanClient(t, peerHost), ""); got != http.StatusUnauthorized {
		t.Fatalf("LAN peer without key: got %d, want 401", got)
	}
	// With a key it is served. The Host header is not part of the control:
	// the model server has to accept a LAN address.
	if got := get(lanClient(t, peerHost), key); got != http.StatusOK {
		t.Fatalf("LAN peer with key: got %d, want 200", got)
	}
	// The same is true on this machine.
	if got := get(http.DefaultClient, key); got != http.StatusOK {
		t.Fatalf("local client with key: got %d, want 200", got)
	}
}

// TestLANConfigExposure pins down what LAN mode passes to llama-server: the
// wildcard host and every named key.
func TestLANConfigExposure(t *testing.T) {
	c := &serve.Config{
		Port: 8080,
		LAN:  true,
		Key:  "ezlm-main",
		Keys: []serve.APIKey{
			{ID: "k1", Name: "Default", Key: "ezlm-main"},
			{ID: "k2", Name: "Phone", Key: "ezlm-phone"},
		},
	}
	if got := c.Host(); got != "0.0.0.0" {
		t.Fatalf("LAN host = %q, want 0.0.0.0", got)
	}
	args := strings.Join(c.Args(), " ")
	if !strings.Contains(args, "--host 0.0.0.0") {
		t.Fatalf("args do not bind the LAN: %s", args)
	}
	// Since T07 a06 the keys travel in a 0600 file instead of on the command
	// line, so assert the file form and that every key is in it.
	if !strings.Contains(args, "--api-key-file ") {
		t.Fatalf("args do not carry the key file: %s", args)
	}
	joined := strings.Join(c.Args(), " ")
	if i := strings.Index(joined, "--api-key-file "); i >= 0 {
		rest := joined[i+len("--api-key-file "):]
		f := strings.Fields(rest)[0]
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read key file %s: %v", f, err)
		}
		for _, want := range []string{"ezlm-main", "ezlm-phone"} {
			if !strings.Contains(string(b), want) {
				t.Fatalf("key file misses %q: %s", want, string(b))
			}
		}
	}
	if !strings.Contains(args, "--metrics") {
		t.Fatalf("args do not enable metrics: %s", args)
	}

	c.LAN = false
	if got := c.Host(); got != "127.0.0.1" {
		t.Fatalf("local host = %q, want 127.0.0.1", got)
	}
	if strings.Contains(strings.Join(c.Args(), " "), "0.0.0.0") {
		t.Fatalf("local args bind the LAN: %v", c.Args())
	}
}
