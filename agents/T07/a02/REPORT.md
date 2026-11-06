# T07 / a02: CSRF and simple-request POSTs

**Threat.** A malicious page in the victim's browser POSTs to the engine API at
`127.0.0.1:<port>`. A browser sends a *simple request* (no CORS preflight) when
the method is POST and the content type is `text/plain`,
`application/x-www-form-urlencoded` or `multipart/form-data`. The engine API
accepts the per-launch token from the query string, and the UI's own URL
carries it there (`http://host:port/?t=<token>`, see `internal/web/server.go`
`Listen`). So one URL leak (a04's threat) turns into a one-shot form POST from
any page, and the body is JSON-decoded whatever the content type says.

**Reproduced: yes. Fixed: yes. Severity: major** (defense in depth: needs the
token to have leaked, but then the attack is a plain form POST with no
preflight and no custom header).

## Proof of concept

`internal/web/csrf_test.go` starts the real server with `Listen` + `Serve` on a
loopback port and a stub backend that records state changes.

Before the fix (`go test -run 'CSRF' ./internal/web/`):

```
--- FAIL: TestCSRFSimplePostFromWebPage (0.00s)
    csrf_test.go:98: simple cross-origin POST: got 200, want 401 (the query token alone must not authorize a state change)
--- FAIL: TestCSRFPostWithSafelistedContentType (0.00s)
    csrf_test.go:122: Content-Type "text/plain": got 200, want 415
    csrf_test.go:122: Content-Type "application/x-www-form-urlencoded": got 200, want 415
    csrf_test.go:122: Content-Type "multipart/form-data; boundary=x": got 200, want 415
    csrf_test.go:126: backend calls = [stop stop stop], want none: a safelisted content type must not reach the handler
```

That is the attack: `POST /api/deploy?t=<token>` with `Content-Type: text/plain`
and `Origin: https://evil.example` returned 200 and called `Deploy("evil")`; and
`POST /api/stop` with the header token and a form content type returned 200 and
stopped the engine. After the fix both fail closed (401 and 415) and the backend
is not called. The control tests (JSON POST with the header token, with and
without `; charset=utf-8`, and the EventSource GET with `?t=`) pass before and
after.

## Findings

1. **Query-string token authorizes state-changing requests (major).**
   `internal/web/server.go:250-253` (before the fix): `guard` falls back to
   `r.URL.Query().Get("t")` for every method. A page can put `?t=<token>` in a
   form action or a `no-cors` fetch, which needs no preflight and no header.
   Evidence: `TestCSRFSimplePostFromWebPage`.
2. **No content-type check on the API (major).**
   `internal/web/server.go:262-273` (before the fix): every handler runs
   `json.NewDecoder(r.Body)` and ignores the error, so a `text/plain` or
   form-encoded body is accepted. Those are exactly the types a browser can
   send cross-origin without a preflight. Evidence:
   `TestCSRFPostWithSafelistedContentType`.
3. **The query token still works on GET (minor, deliberate).** `EventSource`
   cannot set headers, so `GET /api/events?t=` must keep working
   (`internal/web/static/app.js:232`). GETs do not change state, so this is
   accepted. Evidence: `TestEventSourceKeepsQueryToken`.

## Fix (shared file, minimal)

`internal/web/server.go`, two changes:

- `guard`: the query-string token is accepted only for `GET`. A state change
  must present `X-EZLM-Token`, a header a browser cannot send cross-origin
  without a preflight.
- `api`: a non-GET request must carry `Content-Type: application/json`
  (parameters allowed, via the new `isJSON` helper with `mime.ParseMediaType`),
  else 415 with a JSON error. `application/json` is not a CORS-safelisted
  content type, so a cross-origin POST that sends it triggers a preflight, and
  the mux has no OPTIONS route, so the browser blocks the real request.

Compatibility checked: the browser UI (`internal/web/static/app.js:14-18`), the
Windows client (`windows/EZLM/Engine.cs:114-119`, `StringContent` with
`application/json`), the Mac client (`macos/Sources/EZLM/Engine.swift:117`) and
`ChatModel.swift:245` all send the header token and a JSON content type. No
other Go caller POSTs to the API. The CLI only prints the URL and token.

## Residual risk

- The token is still in the address-bar URL (`Listen`), so it still leaks
  through history, logs and `Referer` (a04). This fix means a leaked token no
  longer turns a cross-origin *form* into a state change, but a page that can
  make a preflighted request succeed could still use it.
- The fix relies on the server not answering preflights. If a03 adds a
  permissive CORS policy (`Access-Control-Allow-Origin: *` and
  `Allow-Headers: X-EZLM-Token`), a cross-origin JSON POST with the header
  token becomes possible again. Keep the CORS policy same-origin, or add an
  `Origin` check on non-GET requests.
- No `Origin` check was added here (that is a03's file area and would conflict
  with its patch). It is the natural next layer: reject a non-GET request whose
  `Origin` is present and not the server's own origin.

## Next steps

- a03: keep the preflight answer restrictive, or add the `Origin` check.
- a04: remove the token from the URL (fragment or one-time exchange), which
  removes the precondition for this attack entirely.
- a13: the `authtest` harness can carry these request shapes so the remaining
  threat tests share them.
