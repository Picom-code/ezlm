# a10: fuzz test of the engine API request parsing

**Slice:** `internal/web/fuzz_api_test.go`, `testing.F` over method, path, headers
(`Host`, `X-EZLM-Token`, `Content-Type`) and body. Invariants: no panic, no 5xx on
malformed input, and no response that leaks a path from the machine (source tree,
home directory or temp directory). Run for 30s and record.

**Deliverable:** `internal/web/fuzz_api_test.go`, one minimal fix in
`internal/web/server.go`, a regression corpus seed
`internal/web/testdata/fuzz/FuzzAPIRequest/166f97167d949288`, this report.

## How the test works

- Starts the real server through `web.Listen` + `Server.Serve` on a loopback port,
  so the corpus goes through the listener, the `Host` check, the token guard, the
  `ServeMux` patterns and the static `FileServerFS`, not a copy of the routing.
- A `fuzzBackend` implements `web.Backend` with cheap fixed answers, so nothing
  touches the real home directory, the network or a model.
- Panics are detected through the standard logger (`http: panic serving ...`),
  because `net/http` recovers a handler panic and the client only sees a dropped
  connection.
- Any response `>= 500` fails the fuzz target. Any response body or `Location`
  header containing the test's working directory, home directory or temp directory
  fails it too.
- Seeds cover: valid GET and POST calls, a valid token, a missing token, a wrong
  token, wrong methods (`DELETE`, `PUT`, `OPTIONS`, `HEAD`), a foreign `Host`,
  `localhost:1234`, broken JSON, `text/plain` and form content types, `..`
  traversal, `%00`, a trailing slash and `/api/events`.

## Finding

**Invalid-UTF-8 request path returns 500 from the static file server.**

- Severity: minor. No token is needed and the response body is a generic
  `500 Internal Server Error`, so nothing sensitive leaks, but a caller can make
  the engine log a 5xx and the UI's static handler misbehaves on a malformed URL.
- File:line: `internal/web/server.go:88` (`mux.Handle("GET /", http.FileServerFS(sub))`).
- Evidence (found by the fuzzer in the first 0.4s, seed
  `testdata/fuzz/FuzzAPIRequest/166f97167d949288`):
  `GET /<0xd7>` (raw invalid UTF-8 byte in the path) answered `500 Internal
  Server Error`.
- Cause: `fs.Sub(static, "static")` wraps the embedded FS; `subFS.fullName`
  rejects names that are not valid UTF-8 with `fs.ErrInvalid`, and
  `http.toHTTPError` maps anything other than `ErrNotExist`/`ErrPermission`/
  `errInvalidUnsafePath` to 500. A malformed path from a caller is a 404, not a
  server error.
- Before: `GET /\xd7` -> 500. After: 404 `404 page not found`; the seed is part
  of the normal `go test ./internal/web/` run, so it is a regression test.

### Fix (`internal/web/server.go`)

The static handler now checks `utf8.ValidString(r.URL.Path)` and answers
`http.NotFound` before delegating to the file server. Everything else is
unchanged; the `Host` check and token guard are untouched.

## Run record

```
$ go test -fuzz=FuzzAPIRequest -fuzztime=30s -parallel=2 ./internal/web/
fuzz: elapsed: 0s, gathering baseline coverage: 20/20 completed, now fuzzing with 2 workers
...
fuzz: elapsed: 30s, execs: 268816, new interesting: 104 (total: 133)
PASS
ok  github.com/picom-code/ezlm/internal/web  31.240s
```

- Before the fix, the same command failed after 0.4s with the seed above.
- 268,816 executions, 133 corpus entries, no panic, no other 5xx, no leaked path.
- Seeds alone (`go test -run FuzzAPIRequest ./internal/web/`) pass in 0.5s.

## Residual risk / open questions

- The fuzz target only fuzzes the header token, not duplicate `X-EZLM-Token`
  headers or the `?t=` query token (the query is fuzzed only as part of the raw
  path, and `?t=valid` is not substituted with the real token). a02/a04 own the
  token-transport questions.
- `http.Client` follows redirects; a request that ends in a redirect loop is
  skipped rather than asserted on. The static server's redirects are relative, so
  this does not hide a path leak in practice.
- Backend errors are returned verbatim in JSON (`internal/web/server.go:268`),
  so any real backend error that embeds an absolute path reaches the browser.
  The fake backend in this test returns path-free errors, so that class is a14's
  finding, not this test's. Noted for a14/a16.
- The static handler fix is in a shared file; it is three lines plus the
  `unicode/utf8` import. Merge conflict risk with a01/a14 is small and the
  morning merge should keep both.
