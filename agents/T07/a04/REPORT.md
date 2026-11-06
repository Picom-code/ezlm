# T07 a04: the per-launch token in the URL

**Threat:** a stranger's PC runs EZLM and opens the setup page at
`http://127.0.0.1:PORT/?t=<token>`. The token is the only thing that stops a
web page or another local user from driving the engine API (`/api/deploy`,
`/api/keys/add`, `/api/uninstall`). Because it travels in the page URL, it is
copied into places the token was never meant to be: the browser's address bar
and history, `Referer` headers, `~/.ezlm/ui.url`, the terminal, and the
process table. This slice asks where it leaks, proves each leak, and reduces
the blast radius.

## What the code does today

- `internal/web/server.go:82`: `Listen` returns `http://<addr>/?t=<token>`
  with a fresh 128-bit token per launch.
- `internal/web/server.go:248-260`: `guard` accepts the token from the
  `X-EZLM-Token` header **or from the `t` query string, on every endpoint**.
- `internal/web/server.go:224`: the only middleware is `hostCheck`, which sets
  `Cache-Control: no-store` and `X-Frame-Options: DENY` but no
  `Referrer-Policy`.
- `internal/web/static/app.js:5-10`: the page reads `?t=`, keeps it in
  `sessionStorage`, and never calls `history.replaceState`; `app.js:232` uses
  `?t=` again for `EventSource("/api/events?t=...")`.
- `cmd/ezlm/main.go:299-301`: `ui()` prints the token URL to stdout and writes
  it to `~/.ezlm/ui.url` (mode `0600`). `cmd/ezlm/main.go:314` passes the
  token URL as argv to `open` / `rundll32` / `xdg-open`.
- `cmd/ezlm/main.go:236`: `ezlm engine` prints `{"url","token"}` on stdout.

## Leaks proved, and what changed

| # | Leak | Evidence | Before | After |
|---|------|----------|--------|-------|
| 1 | The query token is a full API credential, so any copy of the URL (history, Referer, log, screenshot) can drive the API, including state-changing POSTs | `TestTokenInQueryOnlyOpensTheEventStream` (server_test file) | `GET /api/status?t=TOKEN` = 200, `POST /api/deploy?t=TOKEN` = 200 | 401; only `GET /api/events?t=` still takes the query, because EventSource cannot set a header |
| 2 | No `Referrer-Policy`, so the browser chooses; the legacy default (`no-referrer-when-downgrade`) sends the whole URL, `?t=` included, to every external link the page opens | `TestResponsesForbidReferer` | header absent on document, API and error pages | `Referrer-Policy: no-referrer` on every response |
| 3 | A leaked token must not be usable through the query while a header is present | `TestHeaderWinsOverQuery` | header wins (already) | unchanged, now pinned |
| 4 | Error pages must not echo the token | `TestErrorPagesDoNotEchoTheToken` | clean (holds) | pinned |
| 5 | Caching must not keep the token-bearing page | `TestTokenBearingRepliesAreNotCached` | `no-store` present (holds) | pinned |
| 6 | The token stays in the address bar and history; the document does not redirect it away | `TestDocumentKeepsTheTokenInTheURL`, `app.js:5-10` | stays | stays; proposed one-line fix below (file is frozen tonight) |

Severity of #1: **major** before the fix, because the URL is the thing users
copy and paste, and a stale token in the address bar of an open tab is a live
credential for the whole launch. After the fix: **minor**, because the token
still reaches history and Referer but now buys only the read-only event
stream.

Severity of #2: **minor**. Current Chrome, Edge, Firefox and Safari default
to `strict-origin-when-cross-origin`, which sends only the origin cross-site,
so the token does not leak to external sites in today's browsers. It does leak
on the legacy default and to same-origin requests. The header costs one line,
so it is worth pinning rather than relying on a browser default.

Severity of #6: **minor**. The token is per launch, so a history entry dies
when EZLM restarts. It is live while the same instance is open, which for a
deploy-and-chat session can be hours.

## Fix (in my owned file, `internal/web/server.go`)

1. `guard` reads the query only for `GET /api/events`; every other call needs
   `X-EZLM-Token`. A copied URL can at most watch events.
2. `hostCheck` sets `Referrer-Policy: no-referrer` on every response.

Nothing else changed. `no-store` and `X-Frame-Options` stay as they were.

## Findings outside my owned files (report only, not patched)

- **`internal/web/static/app.js:5-10`, frozen.** The page never strips the
  token from the address bar. Proposed patch, one line after the token is
  read: `history.replaceState(null, "", "/")` (keep `sessionStorage`, which
  already holds the token). That removes the URL from history and from the
  address bar, and makes the Referer harmless. It does not fix the copy of
  the URL the user already pasted elsewhere. `app.js:232` must keep using
  `?t=` for the EventSource, which is why the query form cannot be removed
  outright.
- **`cmd/ezlm/main.go:299-300`, frozen.** The token URL is printed to stdout
  ("Setup page: ..."), so it lands in terminal scrollback, `script` logs and
  any wrapper that captures output. `main.go:301` also writes it to
  `~/.ezlm/ui.url` (`0600`, so another user cannot read it; it is removed on
  exit and by the idle timer).
- **`cmd/ezlm/main.go:314-320`, frozen.** The URL is passed as argv to `open`
  (macOS), `rundll32 url.dll,FileProtocolHandler` (Windows) or `xdg-open`
  (Linux). While the launcher runs, `ps` shows the token to every local
  user. Short window, but real on a shared machine.
- **`cmd/ezlm/main.go:326-333`, frozen.** `reachable()` does a plain
  `GET` of the token URL. If EZLM died and another local process took the
  same ephemeral port, the next launch sends that token to it. The token is
  dead by then, so this is informational.
- **`cmd/ezlm/main.go:236`, frozen.** `ezlm engine` writes
  `{"url","token"}` to stdout. The native apps read it; a manual run leaves
  the token on the terminal.
- **Windows `windows/EZLM/Engine.cs:77-80` and macOS
  `macos/Sources/EZLM/Engine.swift:113,158-159`, read only.** Both clients
  carry the token in the `X-EZLM-Token` header, and the Mac client sends the
  header on the event stream too. Neither needs the query form, so fix 1 is
  safe for them. The query form exists only for the browser page.

## Residual risk

- A token copied from the address bar or history still opens the event
  stream while that launch is alive. Events carry job progress and status
  text, not keys (`internal/app/app.go:34-43`).
- The token is still in the URL on first load and in the page script's
  `sessionStorage`, so any XSS in the page (or an extension with page access)
  can read it. That is inherent to a browser UI on a localhost origin.
- No access log exists in `internal/web`, so there is nothing to redact
  server-side today; a future request logger must redact `t=`.

## Open questions

1. Should the URL token be one-time, exchanged for an `HttpOnly` cookie on the
   first document load? That would let `app.js` stop needing `?t=` and would
   make history entries useless. It needs a change in the frozen page script
   and adds cookie-based CSRF exposure, which a02 is closing off, so it is a
   cross-slice decision.
2. Is a TTL on the query token workable at all? The event stream reconnects
   with `?t=` for as long as the page is open, so a short TTL would break
   long sessions unless the page refreshes the token. I did not add one;
   scoping the query to `/api/events` gives most of the benefit without that
   breakage.

## Tests

`internal/web/token_leak_test.go`, with a `fakeBackend` so the tests never
touch the real app, disk or network. Before the fix: leak 1 and leak 2 tests
fail (`GET /api/status?t=` 200, `Referrer-Policy` empty). After: all pass.
`go vet ./internal/web/` clean, `gofmt -l` empty.
