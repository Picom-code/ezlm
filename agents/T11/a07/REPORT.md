# T11 a07: `internal/web` health and status handlers

## Scope

New test file only: `internal/web/web_status_a07_test.go` (`package web`, `httptest`-style
loopback server, no edits to `server.go`).

Covered:

- `GET /api/status`: 200, `Content-Type: application/json`, `Cache-Control: no-store`,
  `X-Frame-Options: DENY`, body is one JSON object ending in a newline, full `app.Status`
  round-trip, and the fresh-install/zero-status shape (`omitempty` keys absent, always-present
  keys keep their JSON names).
- `GET /api/status` method contract: POST/PUT/DELETE/PATCH are 405 with `Allow: GET` and the
  `Method Not Allowed` body; HEAD is 200 with an empty body.
- `GET /api/live`: 200, JSON content type, full `app.Live` round-trip.
- `POST /api/live`: 405.
- Foreign `Host` header on `/api/status`: 403 `forbidden` with `text/plain`, proving the
  host check runs before the status handler.

Not covered here (other slices): token/auth semantics (a08), settings/config routes (a09),
remaining routes and static/404/405 behaviour (a10).

## Coverage (`go test -cover ./internal/web/`)

| | Statements |
|---|---|
| Before (no test files) | 0.0% |
| After | 36.7% |

The remainder of the package is the other ~25 routes, `events`, static serving, and the
`guard`/`api` helpers on paths this slice does not drive.

## Bugs found

None. No skipped tests in this slice. The status and live handlers are thin wrappers over the
backend: no error path exists for either route (`Status` and `Live` return values, not errors),
so the only failure modes are method, host and token, and those behave correctly.

## Notes and open questions

- `hostCheck` sets `Cache-Control`/`X-Frame-Options` only after the Host check, so a 403
  response carries neither header. Not a bug (the body is a one-line error), but it is the one
  response on these routes without the framing headers.
- `HEAD /api/status` answers 200 with an empty body through the same JSON encoder; this is
  standard `net/http` behaviour for GET patterns, and is asserted rather than skipped.

## Verification

`gofmt -l .` empty, `go build ./...`, `go vet ./internal/web/`,
`go test -p 2 -cover ./internal/web/` all pass on this branch.
