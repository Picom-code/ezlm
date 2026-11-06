# T11 a08: internal/web auth and token handling

Slice: `internal/web` handlers, auth and token handling. New test file only:
`internal/web/web_auth_test.go`. No production code changed.

## What I covered

The tests run the real `Serve` loop (`Listen` plus `go s.Serve(l)`) on a loopback
port with a fake `Backend`, so the route table, `hostCheck` and `guard` under test
are the production ones. No model, no network, no files.

| Test | What it pins |
|---|---|
| `TestAuthTokenRequired` | table: no token, wrong token in header and query, token in header, token in query, header wins over query, an empty header falls back to the query. 401 body is the "page is out of date" text, does not leak the token, and keeps `Cache-Control: no-store`. |
| `TestAuthTokenHeaderIsExact` | the token is compared whole: longer, shorter, upper-cased and prefixed tokens are rejected. |
| `TestAuthRejectedRequestDoesNotReachBackend` | `guard` runs before the handler, so a bad token never calls `Backend.Status`. |
| `TestAuthQueryTokenOnPOST` | a query token is accepted on POST routes, not just GET. |
| `TestAuthEventsQueryToken` | `/api/events?t=...` returns 200 `text/event-stream` (the reason the query token exists); without a token it is 401. |
| `TestAuthTokenIsPerLaunch` | two servers get different tokens and each rejects the other's token. |
| `TestAuthHostCheck` | a foreign `Host` is 403 even with the right token; the loopback `Host` passes. |
| `TestAuthStaticShellNeedsNoToken` | `GET /` serves the page shell without a token; the API is the guarded boundary. |

## Coverage

`go test -cover ./internal/web/`, before: no test files, **0.0% of statements**.
After: **52.4% of statements**.

Per function (`go tool cover -func`):

| Function | Coverage |
|---|---|
| `guard` (server.go:248) | 100.0% |
| `hostCheck` (server.go:232) | 88.9% |
| `Listen` (server.go:74) | 85.7% |
| `api` (server.go:262) | 62.5% |
| `events` (server.go:275) | 50.0% |
| `Serve` (server.go:85) | 41.3% |

The remaining uncovered statements are the other route closures (a07, a09, a10)
and the error path of `net.Listen` (not reachable hermetically).

## Bugs found

No hard auth bug: nothing was worth a `t.Skip("bug: ...")`. The auth path
behaved as intended in every case I tried. Findings below are observations, not
skips, and none of them needs a test change.

1. **minor (T07, `server.go:250-253`)**: the token is accepted from the query
   string on every route, although the page only needs that for `/api/events`
   (`static/app.js:232` sends the header everywhere else). The token therefore
   lives in the URL that the browser keeps in history, and in same-origin
   `Referer` headers. Limiting the query-token fallback to `/api/events` would
   shrink that surface. Pinned by `TestAuthQueryTokenOnPOST`; no failing test,
   because accepting the query token on POST is the current contract.
2. **minor (T07, `server.go:79-81`)**: `Listen` ignores the error from
   `rand.Read`. If the system entropy source ever fails, the server keeps a
   token of 32 zero characters, which is predictable. Checking the error and
   failing the launch would be safer. There is no seam to inject a failing
   reader, so this has no test.
3. **info (T07, `server.go:255` + `static/app.js:19-20`)**: 401 replies are
   plain text, not JSON. The page falls back to a status message when
   `res.json()` fails, so this works today; it would break if the page ever
   starts requiring a JSON error body.
4. **info (T07, `server.go:250`)**: `guard` treats an empty `X-EZLM-Token`
   header as absent and falls back to the query token. Pinned as the "empty
   header falls back to query" case. Harmless (no caller can set the header),
   but worth knowing.

## Open questions

- Should the query-token fallback be limited to `/api/events` (and maybe
  `/api/update`), so the token stays out of URLs for the rest of the API?
- Should `GET /` and the static assets require a token? Today any local process
  can load the shell, but every `/api` call is guarded, and the shell holds no
  data.

## Checks

`gofmt -l .` empty, `go build ./...`, `go vet ./...` and
`go test -p 2 ./internal/web/` all pass.
