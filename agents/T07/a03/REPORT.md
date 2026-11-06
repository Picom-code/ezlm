# T07 / a03 — CORS on the engine API

**Threat.** A malicious page in the user's browser reaches the loopback engine API
(`http://127.0.0.1:<port>`) with a cross-origin `fetch`. The question is whether it can
*read* the JSON answer (status, logs, model list, storage paths), not just send the request.

**Scope.** `internal/web/server.go` (owned), test `internal/web/cors_test.go` (mine).
No other agent's test file was touched.

## What the code did (before)

`internal/web/server.go` set no CORS header anywhere, and `ServeMux` answered a
preflight `OPTIONS /api/status` with `405 Method Not Allowed` (method-specific patterns).

PoC (`internal/web/cors_test.go`, run against the unfixed tree):

```
--- FAIL: TestCrossOriginReadIsDenied    cors_test.go:56: Vary = [], want Origin ...
--- FAIL: TestSameOriginIsEchoed         cors_test.go:69: Access-Control-Allow-Origin = "", want "http://127.0.0.1:43210"
--- FAIL: TestPreflightFailsClosed       cors_test.go:85: status = 405, want 204 ...
--- FAIL: TestAPIResponseIsNotSniffed    cors_test.go:119: X-Content-Type-Options = "", want nosniff
```

Result: a cross-origin `GET /api/status?t=<token>` returned **200 with no
`Access-Control-Allow-Origin`**, so the browser handed the page an opaque response.
**A cross-origin read was not possible** — but only by accident of omission, not by policy.

## Findings

| # | Finding | Evidence | Severity |
|---|---|---|---|
| 1 | No CORS policy at all: no `Access-Control-Allow-Origin`, no `Vary: Origin`, no preflight handling. Cross-origin reads are blocked only because no header is ever emitted; any future one-line wildcard would silently open the whole API. | `server.go:224` (old) chain was `hostCheck(mux)` only; PoC above | minor (hardening) |
| 2 | Preflight answered `405`, so a legitimate second origin can never be supported and the failure mode is an opaque error rather than a CORS denial. | PoC `TestPreflightFailsClosed` | minor |
| 3 | No `X-Content-Type-Options: nosniff`. The API returns `application/json`; without nosniff a browser may run a JSON response loaded via `<script src="http://127.0.0.1:<port>/api/...?t=...">`, which is a cross-origin read that needs no CORS header. Not exploitable with today's bodies (all are objects, not valid JS), but it removes the last barrier. | PoC `TestAPIResponseIsNotSniffed` | minor |
| 4 | The guard accepts `?t=` on **every** endpoint, so a foreign page can send simple (preflight-free) requests. CORS is therefore the only read barrier, and the per-launch token is the only write barrier. | `server.go:250-253` | informational, for a02/a04 |
| 5 | LAN mode passes the key on the command line (`internal/serve/config.go:121`) and llama.cpp's own server sends permissive CORS headers. Any web page can then read the LAN-exposed model endpoint. Not runnable here (no model may be loaded); code-read only. | `internal/serve/config.go:94,121` | open question for a05/a06 |

## Fix

`internal/web/server.go`:

- `Serve` now builds `s.handler()` (`server.go:91`), so the route table plus wrappers is
  testable without a listener. `return s.hostCheck(s.cors(mux))` (`server.go:230`).
- New `cors` middleware (`server.go:258`): always `Vary: Origin`; echoes
  `Access-Control-Allow-Origin` only for the server's own origin
  (`http://<addr>`); answers a CORS preflight with `204` and no allow headers, so a
  foreign preflight fails closed. No wildcard, no `Allow-Credentials`.
- `hostCheck` adds `X-Content-Type-Options: nosniff` (`server.go:248`).

Behaviour change is additive: same-origin UI traffic is unchanged (it never sends a
cross-origin `Origin`), and a foreign origin still gets a readable-only-if-allowed 200.

## After

```
PASS TestCrossOriginReadIsDenied   (no ACAO for https://evil.example, Vary: Origin)
PASS TestSameOriginIsEchoed
PASS TestPreflightFailsClosed      (204, no allow headers)
PASS TestNoWildcardOnSimpleRequest
PASS TestAPIResponseIsNotSniffed
```

`go build ./...`, `go vet ./internal/web/`, `go test -p 2 ./internal/web/ ./internal/keys/ ./internal/serve/` pass.

## Residual risk

- CORS is browser-enforced. A non-browser local process is not stopped by any of this;
  the token is the only control there (a04, a09).
- The policy allows the exact origin `http://127.0.0.1:<port>`. If the UI is ever served
  from `localhost` or a different port, the echo stops matching; it is same-origin, so
  nothing breaks, but the echo is then dead code.
- Finding 5 needs a llama-server run (a05/a06) to confirm.

## Open questions

1. Should the API be reachable at all from a foreign origin, or should `hostCheck` also
   reject a non-empty `Origin` that is not our own? Rejecting would make CORS defence in
   depth rather than the only barrier; it would also break any future desktop client that
   sends `Origin: null`. Left out to keep the change small.
2. Does the pinned llama.cpp send `Access-Control-Allow-Origin: *` in LAN mode? If yes,
   that is a cross-origin read of completions for any page on the LAN user's browser.
