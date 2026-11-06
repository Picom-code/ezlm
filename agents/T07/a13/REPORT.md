# a13: shared test harness for the engine API

## What I built

`internal/web/authtest/` starts the real engine API (`internal/web.Server`) on an
ephemeral loopback port with a stub backend, so every threat test talks to the
same server with the same token and the same helpers.

- `internal/web/authtest/authtest.go` — the harness and stub backend.
- `internal/web/authtest/authtest_test.go` — tests that the harness itself
  behaves (token required, Host checked, request reaches the backend once,
  unauthenticated POST does not, hooks work).

No production file changed. `internal/web/server.go` is untouched, so this
branch cannot conflict with a01/a02/a14 on that file.

## How the other agents use it

```go
package web_test // external test package, see the caveat below

import (
    "net/http"
    "testing"

    "github.com/picom-code/ezlm/internal/web/authtest"
)

func TestThing(t *testing.T) {
    h := authtest.New(t)                 // starts the API, cleaned up at t.Cleanup

    h.Token                              // the per-launch token the server accepts
    h.Addr                               // 127.0.0.1:port
    h.URL("/api/status")                 // absolute URL
    h.Client                             // *http.Client (10s timeout)

    resp := h.Get(t, "/api/status")      // GET with the token
    resp = h.PostJSON(t, "/api/deploy", map[string]string{"model_id": "granite-4.2-3b"})
    resp = h.Post(t, "/api/lan", "application/json", `{"on":true}`)
    resp = h.GetNoToken(t, "/api/status")
    resp = h.PostNoToken(t, "/api/deploy", "text/plain", `{"model_id":"x"}`)

    req := h.Request(t, http.MethodGet, "/api/status", nil)
    req.Host = "attacker.example"         // DNS-rebinding shape (a01)
    req.Header.Set("Origin", "https://attacker.example") // CORS shape (a03)
    resp = h.Do(t, req)

    body := authtest.Body(t, resp)        // reads and closes the body
    var out map[string]bool
    authtest.Decode(t, resp, &out)

    h.Stub.Called("Deploy")               // did the request reach the app?
    h.Stub.Calls()                         // method names, in order
    h.Stub.ResetCalls()
    h.Stub.Emit(app.Event{State: "ok"})    // push one event to /api/events (a04)
}
```

Behaviour can be changed per method with hooks (`OnStatus`, `OnScan`, `OnDeploy`,
`OnKill`, `OnSetLAN`, `OnAddKey`, `OnRotateKey`, `OnRevokeKey`, `OnUse`,
`OnRemove`, `OnLogs`, `OnReport`). For anything else, embed `*authtest.Stub` in
your own type and override the method; the rest stay recorded.

## Caveat: import cycle

A test file in `package web` must not import `internal/web/authtest`:
`authtest` imports `internal/web`, so an internal test file that imports it is an
import cycle ("import cycle not allowed in test"). Use `package web_test`, or
any other package, and import `web` explicitly when you need its types.

## Observations while writing it

- `internal/web/server.go:88` — the static file handler (`GET /`) is registered
  outside `s.guard`, so the UI assets are served with no token. Harmless for the
  token (it is not in the HTML; the page gets it from the URL), but worth knowing
  for a04/a14: the unauthenticated surface is `/` plus the Host check.
- `internal/web/server.go:232` — the Host check compares `r.Host` to
  `s.addr` exactly, so `localhost:PORT` and `127.0.0.1:PORT` are not both
  accepted. The harness always dials `s.addr`, so tests are unaffected; the UI
  URL is built from `s.addr` too. Flagging in case a01 wants the loopback-name
  list widened.
- `internal/web/server.go:248` — the token is accepted from the `t` query
  parameter on every route, not only `/api/events` (needed for `EventSource`).
  That is a04's surface; the harness lets a test build either shape.
- `internal/web/server.go:74` — `Listen` mints its own token and there is no
  exported constructor with a caller-chosen token. The harness reads
  `srv.Token()`, so "known token" means known to the test, not hardcoded. Tests
  that need to inject a specific token string would need a small change in
  `server.go`; none do so far.

## Not done

- No SSE helper (reading `/api/events` needs a streaming read); `Stub.Emit` plus
  `h.Request` is enough for a04 to write one.
- No coverage of the `internal/serve` LAN surface (a05, a06): the harness is
  web-only, as scoped.
- Not run: `GOOS=windows`/`GOOS=linux` builds and the full `go test ./...` are
  for the last commit; the package is pure stdlib plus `internal/app`, so no
  platform-specific code is involved.

## Open question

Should `authtest` also be the home of a temp-`HOME`/`EZLM_HOME` helper for a07 and
a12? Those tests live in `internal/keys`/`internal/serve` and do not need the
HTTP server; a separate small helper there may fit better.
