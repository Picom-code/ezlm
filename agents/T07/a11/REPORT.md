# T07 / a11 report: threat model

## What I did

Wrote `agents/T07/a11/threat-model.md`: assets, entry points, attacker
capabilities, and a STRIDE-shaped threat table with the current control and its
file:line for each. Read only; no code changes, so nothing to collide with the
other agents.

## Findings that matter to the merge

These are the ones the document calls out as open or partial. All are other
agents' fixes; I did not edit their files.

| # | Finding | Evidence | Severity | Owner |
|---|---|---|---|---|
| 1 | API key on the llama-server command line, readable by other local users with `ps` | `internal/serve/config.go:121` (`"--api-key", c.keyList()`), args built at 115-138 | major | a06 |
| 2 | Engine token in the URL query, so it reaches browser history, `~/.ezlm/ui.url`, stdout and the SSE URL; no `Referrer-Policy` | `internal/web/server.go:82`, `:252`, `cmd/ezlm/main.go:277-302`, `internal/web/static/app.js:232` | major (chains with #3) | a04 |
| 3 | A leaked token is enough for a blind CSRF POST to destructive routes (`?t=` + `text/plain` form, no preflight) | `internal/web/server.go:248-260`, `:219` (`/api/uninstall`), `:104` (`/api/kill`) | major | a02 |
| 4 | `/api/kill` accepts any PID, not just PIDs EZLM found | `internal/web/server.go:104-110` -> `internal/scan/scan.go:259-277` | minor alone, major if the token leaks | T07 / T10 |
| 5 | `~/.ezlm` dirs are 0755 and logs 0644 while `config.json` is 0600 | `internal/paths/paths.go:41`, `internal/serve/run.go:18`, `internal/serve/config.go:86` | minor | a07 |
| 6 | No read/write/idle timeouts and no `MaxBytesReader` on the engine API | `internal/web/server.go:224`, handlers 90-221 | minor | a14 |
| 7 | Engine logs no requests, so no record of `/api/uninstall` or `/api/kill` | `internal/web/server.go` has no logging | minor | a16 (document) |

## What holds today (baseline)

Already present in `5213370`, contrary to parts of the brief:

- Host check: `server.go:232-237` rejects any `Host` that is not
  `127.0.0.1:<port>` (a01's threat is mitigated; a01 should still add the test).
- Constant-time token compare: `server.go:254` (a09's fix is in; add the test).
- Token required on every `/api/` route: `server.go:248-260`, `api()` at 262.
- No CORS headers, `X-Frame-Options: DENY`, `Cache-Control: no-store`:
  `server.go:241-242`.
- 0600 `config.json` and service files; 238-bit keys; per-launch 128-bit token.

So a01 and a09 are "prove and keep", not "fix". The document says so.

## Not done

- No PoC tests; that is a01 to a16's job. The document is the map for them.
- Two items are unverified and marked as such: whether llama.cpp `/metrics`
  and `/v1/models` reject a keyless request (config.go:125 comment), and the
  Windows ACL EZLM actually gets (code-read only).
- Did not read the whole of `internal/app`; only the handlers named above.

## Next steps

1. a06 should use `--api-key-file` (0600 file) and keep `--api-key` only as a
   fallback, then the LAN story is sound.
2. a04 should add `Referrer-Policy: no-referrer` and accept the token in a
   header everywhere except SSE.
3. a02 should require `Content-Type: application/json` on POSTs, which turns
   a blind form POST into a preflight failure.
4. a16 should fold the table above into `findings.md` with statuses.
