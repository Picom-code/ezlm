# T07/a14: handler review

Agent a14. Threat: the engine API handlers in `internal/web/server.go` accept
request bodies, methods and content types they should not, and echo error text
that contains absolute paths. Attacker is a malicious web page in the user's
browser, or a LAN peer once LAN mode exposes the engine API.

Worktree: `~/ezlm-agents/work/subs/T07/a14`, branch `agent/T07-a14`.
Commit: `3012670` (fix and tests), this report in the next commit.

## What I did

Reviewed every route in `internal/web/server.go` (baseline commit `5213370`)
for path traversal, method confusion, content-type confusion, unbounded request
bodies and absolute paths in error messages. Wrote
`internal/web/handler_review_test.go` (the only test file I touched), proved
each real issue by running it against the baseline, then fixed it with one
small helper and one redaction function.

Before/after evidence, baseline `server.go` with the new test file:

```
--- FAIL: TestHandlerRejectsOversizedBody       status = 200, want 413
--- FAIL: TestHandlerRejectsMalformedJSON       status = 200, want 400
--- FAIL: TestHandlerRejectsNonJSONContentType  status = 200, want 415
--- FAIL: TestHandlerRedactsHomePathFromError   {"error":"/Users/example/.ezlm/models: permission denied"}
```

After the fix, `go test -p 2 ./internal/web/` passes all of them. The two
verified-safe areas (traversal, methods) passed before the fix too, so they
are regression tests rather than proofs of a bug.

## Findings

### F1 (major, fixed): request bodies have no size limit

`internal/web/server.go:94` and the other 14 handlers call
`json.NewDecoder(r.Body).Decode(&in)` with no limit. A single authenticated
request can make the engine allocate as much memory as the client sends; the
`/api/report` endpoint happily took a 9 MiB body (test:
`TestHandlerRejectsOversizedBody`). A page that has the token (URL token,
`Referer` leak, a04) or a LAN peer can repeat that.

Fix: `decodeJSON` wraps the body in `http.MaxBytesReader` with an 8 MiB cap
(`server.go:267`, `server.go:271`). Over the cap answers 413 with
`{"error":"the request is too large"}`. 8 MiB is above the largest client
body (the report carries the app log; the Windows engine log is capped at
2 MB in `windows/EZLM/Engine.cs:88`).

### F2 (major, fixed): malformed JSON silently runs the action with zero values

Every decode error was discarded. `POST /api/keys/revoke` with `{"id":`
returned 200 and called `RevokeKey("")`; the same applies to `use`, `remove`,
`launch`, `storage`, `apps/close` and the rest, so a truncated or wrong-typed
body turns into a destructive call with empty fields (test:
`TestHandlerRejectsMalformedJSON`).

Fix: `decodeJSON` returns a 400 for a non-empty body that is not valid JSON
(`server.go:271`). An empty body still leaves the struct at its zero value,
which is what the bodyless POST endpoints and older clients rely on.

### F3 (minor, fixed; overlaps a02): any content type is accepted as JSON

The handlers never looked at `Content-Type`, so `POST /api/keys/revoke` with
`Content-Type: text/plain` and a valid JSON body succeeded (test:
`TestHandlerRejectsNonJSONContentType`). `text/plain` is a browser simple
request, so it needs no CORS preflight. This is the request shape a02 targets;
a02 should keep its token/header fix, and this check is defence in depth
rather than a replacement. a02 and a14 both touch `server.go`; the merge
should keep both.

Fix: when a body is present it must be `application/json` (media type parsed,
parameters allowed), otherwise 415 `{"error":"send the request as JSON"}`
(`server.go:275`). All three clients already send `application/json` for
every body: `internal/web/static/app.js:16`, `macos/Sources/EZLM/Engine.swift:114`,
`windows/EZLM/Engine.cs:117`.

### F4 (minor, fixed): error responses leak absolute paths

`api` wrote `err.Error()` to the client verbatim (`server.go:262` baseline).
Backend errors can contain absolute paths, for example
`internal/app/app.go:1541` includes `paths.Bin()` (`/Users/<name>/.ezlm/bin`)
in the "model server program is missing" message, and
`internal/app/settings.go:264` wraps `os` errors that name the full path.
Anyone who can read a response (the page itself, or a cross-origin reader if
a03 finds one) learns the user name and folder layout. Test:
`TestHandlerRedactsHomePathFromError` with `EZLM_HOME=/Users/example/.ezlm`
expects `~/models`, not the absolute path.

Fix: `redactPaths` (`server.go:298`) replaces the EZLM home and the user home
with `~` in every error string before it is encoded.

### F5 (verified, no bug): path traversal

The static handler is `mux.Handle("GET /", http.FileServerFS(sub))`
(`server.go:88`). The embedded FS has no `..` entries, and the Go 1.22 mux
redirects unclean paths before the file server sees them.
`TestFileServerNoPathTraversal` requests `/../server.go`, `/%2e%2e/server.go`,
`/..%2fserver.go`, `/static/../../server.go`, `/../../../../etc/passwd`,
`//etc/passwd` and `/%2e%2e%2f%2e%2e%2fetc/passwd` and asserts no source or
`/etc/passwd` content is returned. The UI at `/` still serves.

### F6 (verified, no bug): method confusion

Routes are method-scoped (`"GET /api/status"`, `"POST /api/stop"`). Wrong
methods do not reach a handler: `POST /api/status`, `PUT /api/report` and
`DELETE /api/keys/revoke` get 405, `GET /api/stop` falls through to the file
server and gets 404, `HEAD /api/status` runs the GET handler with no body
(`TestMethodConfusion`, asserts the backend saw no calls). This is correct;
no change.

## What changed

- `internal/web/server.go`: added `maxBody`, `decodeJSON`, `apiError` and
  `redactPaths`; replaced the 15 unchecked decodes; `api` now uses the status
  from `apiError` (400 for backend errors as before) and redacts the message.
  No route, auth, host-check or event code changed.
- `internal/web/handler_review_test.go`: new, 7 tests over the production
  `Listen`/`Serve` wiring on loopback. No other agent's test file touched.

## Checks

`gofmt -l .` empty, `go build ./...`, `go vet ./...`,
`go test -p 2 ./internal/web/ ./internal/keys/ ./internal/serve/`,
`GOOS=windows GOARCH=amd64 go build ./...`, `GOOS=windows GOARCH=arm64 go build ./...`,
`GOOS=linux go build ./...`: all pass at commit `3012670`.

## Residual risk and open questions

- No `ReadTimeout` on the server (`server.go:224`), so a slow-body POST can
  hold a connection open; `ReadHeaderTimeout` only covers headers. Adding
  `ReadTimeout` is safe for SSE because the request body is read before the
  handler starts, but I left it out to keep the diff small. Minor.
- Every backend error is still answered 400, even an internal failure. The
  client only sees "bad request"; mapping needs an error taxonomy in
  `internal/app` (frozen for edits). Minor.
- The query token is still accepted for POST (`guard`, `server.go:250`), so
  the content-type gate is what stops a token-in-URL CSRF. a02 owns making
  the header mandatory for non-GET.
- The 8 MiB cap is a judgement call; if the report endpoint is meant to carry
  bigger logs, raise `maxBody`. Test keys in this file look like
  `REDACTED-KEY`; no real key is printed.
