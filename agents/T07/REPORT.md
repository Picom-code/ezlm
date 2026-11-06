# T07. Local API and key security review — consolidated report

16 agents (a01–a16) worked this task in parallel: one per threat, plus a fuzz test, the threat model,
key storage, the shared auth harness, a handler review, the apps' clients, and the findings table.
Consolidated onto `agent/T07-api-security` by the Lead.

Threat model: a stranger's PC running EZLM, with (1) a malicious web page in their browser and (2)
another user on their LAN.

## Result

- **15 threats assessed**: 11 fixed, 2 mitigated by code-read, 2 open with proposals.
- **Gate:** `gofmt -l .` empty, `go build ./...`, `go vet ./...`, `go test -p 2 ./...` pass, plus
  `GOOS=windows` amd64/arm64 and `GOOS=linux` builds.

## Fixed

| Threat | Before | After |
|---|---|---|
| **DNS rebinding** (a01) | The baseline had an exact-match Host check, which also 403'd `localhost:PORT` and `[::1]`. | `hostOK` accepts the listener address plus `localhost` and loopback literals with the right port; a hostname that merely resolves to 127.0.0.1 is still rejected. |
| **LAN peer with a spoofed Host** (a05) | A peer that reached the port with a valid token and `Host: 127.0.0.1:port` got 200. | Loopback `RemoteAddr` is required as well. |
| **CSRF / simple-request POSTs** (a02) | `POST /api/deploy?t=<token>` with `text/plain` from `evil.example` returned 200 and deployed. | Query token only for `GET /api/events` (EventSource); every other route needs the header, and a non-GET `/api` request must be `application/json` (415 otherwise). |
| **Token in the URL** (a04) | `?t=` was a full credential on every route, including state-changing POSTs. | Same restriction; `Referrer-Policy: no-referrer` on every response; no-store already present. |
| **API key on the llama-server command line** (a06) | `--api-key <key>` visible in `ps` to any local user. | Keys are written to `~/.ezlm/api-keys.txt` (0600, temp+rename, rewritten on rotation) and passed with `--api-key-file`; inline `--api-key` is only a fallback. |
| **`~/.ezlm` permissions** (a07) | Directory 0755, `server.log` 0644, `settings.json` kept 0644, `config.json.tmp` 0644. | 0700 directory (repaired on start), 0600 log, 0600 settings and config, tmp chmod'd before rename. Windows: code-read only, no DACL is set at all. |
| **Log redaction** (a08) | `TailLog`, `/api/logs`, `/api/integrate/log` passed key/Bearer/`sk-` lines through. | `integrate.Redact` applied at the log tail and the web boundary. |
| **Empty-token acceptance** (a09) | `subtle.ConstantTimeCompare("","")==1`, so a `Server` with an empty token accepted a tokenless request (no production path, latent). | `keys.Equal` (constant-time, empty-safe) used by the guard. |
| **Static handler 500** (a10) | An invalid-UTF-8 path returned 500. | 404. |
| **Unbounded bodies / silent zero-value actions** (a14) | A 9 MiB body was accepted; malformed JSON ran `RevokeKey("")`; error text echoed `/Users/example/.ezlm/...`. | 8 MiB `MaxBytesReader`, 400 on bad JSON, 413 over cap, home paths redacted. |
| **Empty/corrupt key in config** (a12) | A blank key passed `--api-key ""` to llama-server, which accepts `Bearer `, so LAN had no real auth; `Config.Key` could disagree with `Keys[0]`. | Blank keys replaced, mirror kept in step, legacy/corrupt config tightened to 0600. |
| **CORS policy** (a03) | Accidental: no ACAO at all, no `Vary`, preflight 405. | Explicit same-origin-only middleware (echo only `http://<addr>`, preflight 204, no wildcard/credentials) plus `X-Content-Type-Options: nosniff`. |

## Open (with proposals in the reports)

1. **No Windows ACL for `~/.ezlm`** (a07). Go's 0600 only sets the read-only attribute on Windows, so
   the key is protected only by inherited profile ACLs. Needs `SetNamedSecurityInfo` (x/sys/windows).
2. **A corrupt config silently rotates the key** in the frozen `internal/app` (a12), so an old
   llama-server can be orphaned.
3. **The token still lands in browser history** (a04): `static/app.js` never calls
   `history.replaceState`; the file is frozen tonight.
4. **LAN policy gaps** (a05): no LAN-only key, and `lanIP` can pick a VPN or tunnel interface.
5. **`/api/kill` takes any PID** (a11), and the engine has no read/idle timeouts.
6. **The apps' clients** (a15) are mostly good: token in the `X-EZLM-Token` header on every call
   including SSE, never in a URL or on disk. Residual: hello URL trusted without a loopback check,
   stale-token retry loop, and the 64-hex launch token matches no redaction pattern.

## Merge notes

Nine branches changed `internal/web/server.go` and three changed `internal/serve/config.go`; a16
warned about exactly this. The Lead merged by taking a14's `decodeJSON`/`apiError` refactor as the
base, appending the other agents' helpers in `internal/web/merged_helpers.go`, and then re-adding the
pieces the 3-way merge dropped (the loopback host check, the query-token scope, `Referrer-Policy`,
CORS wiring). Two behaviour changes had to be reconciled with older tests:

- `--api-key-file` (a06) replaced `--api-key`, so a05's LAN test and a12's storage tests now read the
  key file.
- a12's blank-key repair generated keys without the `ezlm-` prefix and `internal/serve` could not
  import `internal/keys` without a test import cycle, so `serve` carries a local `newKey()` with the
  same format.

`internal/web/authtest/` (a13) is the shared harness: `authtest.New(t)` starts the real API on a
loopback port with a stub backend. It must be used from `package web_test` (import cycle otherwise).

## Files

- `internal/web/`: `host_rebinding_test.go`, `csrf_test.go`, `cors_test.go`, `token_leak_test.go`,
  `lan_test.go`, `redaction_test.go`, `fuzz_api_test.go`, `handler_review_test.go`, `server.go`,
  `merged_helpers.go`, `authtest/`.
- `internal/keys/`: `timing_test.go`, `permissions_test.go`, `storage_test.go`, `keys.go`.
- `internal/serve/`: `config.go`, `key_cmdline_test.go`.
- `agents/T07/`: `a11/threat-model.md`, `a16/findings.md`, `a15/clients.md`, and one report per agent.

## Next steps

1. Windows ACL for `~/.ezlm` (the only major open item).
2. Apply the frozen-file proposals: `history.replaceState`, the report title redaction, the engine
   token prefix, the config-error handling in `app.go`.
3. Decide the LAN key policy (separate LAN key, interface selection).
