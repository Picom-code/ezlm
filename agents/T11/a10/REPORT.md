# T11 / a10: `internal/web` remaining routes, static files, 404/405

Slice: the routes not covered by a07 (health/status), a08 (auth) and a09 (settings):
scan, deploy, kill, lan, integrate, keys, installed, update, launch, integrate/log,
report, apps, apps/close, use, remove, cancel, test, stop, start, uninstall, live,
logs, events, plus static file serving and unknown-path/method handling.

Deliverable: one new test file, `internal/web/server_routes_test.go`. No logic changes.

## Tests added

`server_routes_test.go` runs the real `Serve` loop on a loopback listener with a
hand-written `fakeBackend` that records each call and returns canned values. The
client presents the per-launch token from `Server.Token()`. Hermetic: no real app,
no home directory, no network beyond 127.0.0.1.

- `TestListenURLAndToken` - `Addr` matches the listener, token is 32 hex characters,
  returned URL ends in `/?t=<token>`.
- `TestStaticFiles` - `/` (HTML), `/style.css`, `/app.js`, `/logos/app-dsh.png`
  (PNG magic), missing asset 404, `HEAD /` has an empty body, and successful
  responses carry `Cache-Control: no-store` and `X-Frame-Options: DENY`.
- `TestUnknownRoute` - GET on an unknown API path serves the file server's 404;
  `/../server.go` does not escape the embedded root.
- `TestMethodNotAllowed` - POST on GET-only routes and DELETE on POST-only routes
  return 405 with an `Allow` header.
- `TestPostRoutes` - 21 POST routes: status 200, `application/json`, response
  body, and exactly one recorded backend call with the decoded arguments
  (pref, pid, ids/default, key id/name, dir/move, mode, report fields, force, quant).
- `TestGetRoutes` - update (with and without `force`), storage, integrate/log,
  apps, live, logs: 200, JSON, body, and the call arguments (including the
  hard-coded `n=200` for integration logs and `n=300` for logs).
- `TestAPIError` - a backend error becomes 400 with `{"error": ...}` and JSON
  content type.
- `TestEventStream` - `/api/events` sends `text/event-stream`, forwards a
  backend event as an SSE `data:` line plus a blank separator, and the
  subscription's cancel function runs after the request context ends.
- Skipped bug tests: `TestPostToUnknownPath`, `TestGetOnPostOnlyRoute`,
  `TestMalformedJSONBody` (see findings).

## Coverage (`go test -cover ./internal/web/`)

| | statements |
|---|---|
| before | 0.0% (no test files) |
| after | 91.2% |

Still uncovered: `Listen`'s `net.Listen` error path (needs a bind failure),
`hostCheck`'s nil-`onActivity` branch and activity callback (a07's slice),
`guard`'s empty-header fallback (a08's slice), `events`'s non-`http.Flusher`
branch and the 15-second ping ticker (not worth a 15 s test).

## Findings

1. **Malformed JSON bodies are silently accepted.** `internal/web/server.go:94`
   (and every other POST handler, e.g. `server.go:101`, `:108`, `:114`) ignores
   the error from `json.NewDecoder(r.Body).Decode(&in)`. A truncated body on
   `/api/deploy` returns 200 `{"ok":true}` and calls `Deploy("")`.
   Test: `TestMalformedJSONBody` (skipped).
   Severity: **major** (a client cannot tell a rejected request from a bad one).
   Owner: T07.

2. **POST to an unknown path returns 405 instead of 404.** `internal/web/server.go:88`
   registers `GET /` as a catch-all file server, so every path matches it. A
   non-GET method on a path that exists nowhere yields
   `405 Method Not Allowed` with `Allow: GET, HEAD`, not 404.
   Test: `TestPostToUnknownPath` (skipped).
   Severity: **minor**. Owner: T07.

3. **GET on a POST-only API route returns the static 404 page instead of 405.**
   Same catch-all: `GET /api/deploy` matches `GET /` and the file server answers
   404, while `DELETE /api/scan` (no `GET /` method match) correctly answers 405.
   Test: `TestGetOnPostOnlyRoute` (skipped).
   Severity: **minor**. Owner: T07. A single fallback handler that splits `/api/`
   from static paths would fix findings 2 and 3 together.

## Observations (no test, no bug claimed)

- Go 1.27's `ServeContent` strips `Cache-Control` from its own error responses,
  so the file server's 404 page lacks the `no-store` header `hostCheck` set. This
  is standard library behaviour, not an EZLM bug; the static tests only assert
  the header on 200s.
- `Listen` ignores the `crypto/rand.Read` error (`server.go:80`). Practically
  unreachable, so no test.

## Open questions

- Should `/api/events` cap concurrent streams? Nothing in the handler does; a
  browser refresh leaves the old stream until its context is cancelled.
- The API decodes request bodies with no size limit; a very large body is read
  into memory. Not tested here (needs a handler-level limit to be meaningful).

## Checks

`gofmt -l internal/web` empty; `go vet ./internal/web` clean;
`go test -p 2 -count=1 -race ./internal/web` passes (91.2% coverage).
