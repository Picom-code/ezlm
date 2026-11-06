# EZLM local API threat model

Task T07, agent a11. Scope: EZLM's engine API (`internal/web`), the per-launch
token, the API keys (`internal/keys`, `internal/serve` auth parts) and the LAN
exposure of llama-server. Line numbers are from the baseline snapshot
(`5213370`). This document states the current control for each threat, so the
morning merge can tell "already held" from "still open".

## Headline

EZLM puts a control plane that can uninstall the app, kill a process and move
the model folder behind a 128-bit per-launch token on 127.0.0.1. The token is
carried in the URL query (and in `~/.ezlm/ui.url` and on stdout), so the whole
model rests on that token not leaking. The two leaks that matter:

1. **The API key is on the llama-server command line** (`--api-key`,
   `internal/serve/config.go:121`), so any other local user can read it with
   `ps` and use the model, including over the LAN when LAN mode is on.
2. **The engine token is in the page URL** (`internal/web/server.go:82`), so it
   lands in browser history, in `ui.url` and in terminal scrollback, and stays
   valid for the life of the process. A leaked token plus a simple form POST
   (`?t=` in the query, no preflight) can call destructive routes such as
   `/api/uninstall`.

The Host check, constant-time compare, CORS silence, `X-Frame-Options: DENY`,
0600 config file and 238-bit keys are already in the baseline and hold.

## 1. What the system is

Two HTTP servers, one process tree, one user account:

- **Engine API** (`internal/web/server.go`): the control plane the setup page
  and the native apps drive. Binds `127.0.0.1:0` (server.go:75), random port,
  one 16-byte hex token per `Listen` (server.go:79-82). Serves the static UI
  without a token (`GET /`, server.go:88); every `/api/` route requires the
  token (server.go:248-260, wrapped by `api()` at server.go:262).
- **llama-server**: the data plane. Binds `127.0.0.1:<port>` normally, or
  `0.0.0.0` in LAN mode (`Config.Host`, config.go:92-97). Requires a bearer
  key (`--api-key`, config.go:121); keys are `ezlm-` + 40 base62 chars
  (~238 bits, keys.go:11-22) and live in `~/.ezlm/config.json` (0600,
  config.go:86).

The engine never proxies inference. A browser page and the native apps talk to
the engine for control and to llama-server for chat, with different secrets.

```
 browser page (token in URL) ──┐
 native app (token from stdout) ─┼─> 127.0.0.1:engine-port  (token, Host check)
 LAN peer ───────────────────────┘        │
                                          ├─> ~/.ezlm (config.json, logs, ui.url)
                                          └─> launchd/systemd/Task: llama-server
 browser page / native app ───────────────> 127.0.0.1:model-port (API key)
 LAN peer ─────────────────────────────────> 0.0.0.0:model-port  (API key, LAN mode)
```

## 2. Assets

| # | Asset | Where it lives | Why it matters |
|---|---|---|---|
| A1 | Inference access to the model | llama-server, `127.0.0.1:port` or `0.0.0.0:port` | Costs the owner's GPU/CPU time; can be abused as a free LLM; prompts may be private. |
| A2 | Engine control plane | engine API on loopback | `uninstall`, `kill`, `storage`, `launch`, `close`, `stop/start`, key add/rotate/revoke. Effectively the user's account, minus file read. |
| A3 | API keys | `~/.ezlm/config.json` (0600), process argv, launchd plist / systemd unit / Windows task, connected apps' configs | A key is the only thing between a LAN peer and A1. |
| A4 | Engine token | engine URL query, `~/.ezlm/ui.url` (0600), stdout, browser history/sessionStorage | A token is the only thing between a page and A2. |
| A5 | Model files and settings | `~/.ezlm/models`, `~/.ezlm/config.json` | Deleting or moving them is destructive and slow to undo (re-download). |
| A6 | User prompts and replies | llama-server memory, server log, report body | Private content; the report path can publish them to GitHub. |
| A7 | The user's other processes | `/api/kill` -> `scan.Kill` | A token holder can end any process the user can end. |

## 3. Entry points

| # | Surface | Address | Auth | Reachable by |
|---|---|---|---|---|
| E1 | Engine API `/api/*` | `127.0.0.1:<random>` | `X-EZLM-Token` header or `?t=` query (server.go:248-260) | browser page (if token known), native apps, local processes |
| E2 | Engine static UI `GET /` | same | none (server.go:88) | anyone on loopback; cross-origin reads blocked by absent CORS |
| E3 | Engine SSE `/api/events` | same | `?t=` (EventSource cannot set a header, app.js:232) | page, apps |
| E4 | llama-server OpenAI API | `127.0.0.1:<port>` / `0.0.0.0:<port>` | `Authorization: Bearer ezlm-…` | local processes; LAN peers in LAN mode |
| E5 | llama-server `/metrics` | same | API key (config.go:125 comment) | key holders |
| E6 | `~/.ezlm` files | filesystem | OS permissions | local users (see I4) |
| E7 | Process command line | `ps` / Task Manager | OS | any local user (see I3) |
| E8 | Report upload | outbound HTTPS to `api.github.com` | build-time token (report.go:23-37) | the app itself |

## 4. Attacker capabilities

**A1, a malicious web page in the user's browser.** Can make cross-origin
requests, including *simple* requests that skip preflight (GET, `POST` with
`text/plain` or `application/x-www-form-urlencoded`), but cannot read the
response without CORS. Can use DNS rebinding so its own hostname resolves to
127.0.0.1. Can open windows and navigate, so it can read `Referer` on the
request it causes. Can measure coarse timing. Cannot set `Host` or arbitrary
request headers, cannot read `sessionStorage` of another origin, cannot read
`~/.ezlm`.

**A2, another local user on the same machine.** Can read world-readable files,
run `ps`/`lsof`, connect to loopback ports, and race. Cannot read 0600 files
owned by the victim, cannot ptrace the victim (without privileges), cannot read
the victim's browser history.

**A3, a peer on the same LAN.** Only when LAN mode is on. Can reach
`0.0.0.0:<model-port>`, probe ports, send requests. Cannot reach the engine API
(it stays on 127.0.0.1). Needs the API key for anything but error responses.

**Not modelled (out of scope, by design).** A process running as the same user
(can read `config.json`, `ui.url` and ptrace); a malicious model file or
llama.cpp itself; a compromised update; physical access; a compromised
`~/.ezlm` backup; the OS keychain (EZLM does not use one).

## 5. Threats

Severity: **blocker** = unauthenticated takeover or destruction; **major** =
another user or a leaked secret gets inference or control; **minor** = narrow
information leak or nuisance. "Status" is the state of the *control*, not of
the bug: `held` = the control exists and is believed effective, `partial` =
control exists but leaves a gap, `open` = no control.

### Spoofing

| # | Threat | Current control | Status | Severity |
|---|---|---|---|---|
| S1 | A rebinding page reaches the engine API by making `evil.example` resolve to 127.0.0.1 | Exact Host match against `127.0.0.1:<port>` before routing (`hostCheck`, server.go:232-237); runs before the token check and before `onActivity` | held | — |
| S2 | Guessing the engine token | 16 random bytes hex (server.go:79-81); constant-time compare (server.go:254); per launch, so guesses do not accumulate | held | — |
| S3 | Guessing an API key on the LAN | 40 base62 chars, ~238 bits (keys.go:11-22); llama-server compares | held | — |
| S4 | Framing or clickjacking the setup page to make the user act | `X-Frame-Options: DENY` (server.go:242); no CORS headers anywhere | held | — |

Note on S1: the check compares the whole `host:port`, so `localhost:<port>`
and `[::1]:<port>` are rejected too. That is a usability wrinkle, not a hole:
the printed URL always uses `127.0.0.1` (server.go:82).

### Tampering

| # | Threat | Current control | Status | Severity |
|---|---|---|---|---|
| T1 | A page makes the user's browser POST to `/api/uninstall` (CSRF). Simple `text/plain` POST needs no preflight | Token required on every `/api/` route (`guard`, server.go:248-260; `api()` server.go:262). Without the token the request is 401 and the action does not run | held while the token stays secret; **partial** overall | major (chains with I1) |
| T2 | Method confusion (e.g. `GET /api/uninstall`) | Method-scoped mux patterns (`GET /api/status`, `POST /api/...`, server.go:88-222); Go answers 405 for the wrong method | held | — |
| T3 | Path traversal through the static file server | `http.FileServerFS` over an embedded `fs.FS` (server.go:88, 23-24); Go rejects `..` outside the FS | held (a14 fuzzes) | — |
| T4 | A token holder kills an arbitrary process via `POST /api/kill {"pid":N}` | None. `scan.Kill` only checks the PID is killable by this user (scan.go:259-277); it does not check the PID belongs to EZLM | open | major if the token leaks, else minor |
| T5 | A token holder moves the model folder to an arbitrary writable path via `POST /api/storage` | `checkFolder` requires a writable folder (settings.go:108-115); no allowlist | open (by design) | minor |
| T6 | A token holder uninstalls: stops the server, deletes models and settings | Token only (server.go:219; app.go:1586-1601). No confirmation parameter, no undo | open (by design) | major if the token leaks |

T1 detail: because `guard` accepts `?t=` on every route, a page that learns the
token can submit `<form action="http://127.0.0.1:port/api/uninstall?t=…"
method="post">` with no preflight and no ability to read the reply. The
destructive routes are exactly the ones where a blind POST is enough. That is
why the token leak (I1) is rated with T1, not on its own.

### Repudiation

| # | Threat | Current control | Status | Severity |
|---|---|---|---|---|
| R1 | No record of who called the control plane (`/api/uninstall`, `/api/kill`) | None: the engine logs no requests; the server log is truncated on each start (service_darwin.go:85) | open | minor |
| R2 | No per-key attribution in llama-server usage | llama-server accepts any configured key (config.go:161-171); it does not report which key was used | open (upstream) | minor |

### Information disclosure

| # | Threat | Current control | Status | Severity |
|---|---|---|---|---|
| I1 | The engine token leaks: `?t=` in the page URL (server.go:82), in `ui.url` (main.go:277-299), on stdout (main.go:302), in browser history, in the SSE URL (app.js:232) | Per-launch token; `sessionStorage` not `localStorage` (app.js:8-9); `ui.url` written 0600 (main.go:298) and removed on exit (main.go:299). No `Referrer-Policy` header (server.go:241-242) | **partial** | major |
| I2 | The token is printed in the terminal and in the engine's stdout JSON for native apps | Same-user only; accepted for the CLI. Native apps read it from the child's stdout (main.go:236), not from a file | accepted | minor |
| I3 | The API key is readable by other local users from the process command line: `--api-key <key>` (config.go:121, Args at 115-138) | None. The key is also in the launchd plist (service_darwin.go:49-84), systemd unit (service_linux.go:49-71) and the Windows task (service_windows.go:167) | **open** | major |
| I4 | Key and token at rest: `~/.ezlm` dirs are 0755 (paths.go:41), logs 0644 (run.go:18; service_windows.go:181); `config.json` is 0600 (config.go:86) | 0600 on the files that hold secrets; dirs and logs world-readable | **partial** | minor |
| I5 | A key or token appears in a report or a log | `integrate.Redact` removes the literal config keys, `ezlm-…`, `sk-…` and `Bearer …` patterns (app/launch.go:236-244; integrate/log.go:29-50); logs are opt-in (`details`) | held for the known formats; **partial** for a rotated key that is no longer in the config at report time | minor |
| I6 | Error strings leak absolute paths (`/Users/name/.ezlm/config.json`) | `Load` includes the path (config.go:69); the API returns `err.Error()` verbatim (server.go:268) | open, token-holder only | minor |
| I7 | Prompts and replies cross loopback in cleartext | Plain HTTP on both servers (server.go:75, config.go:100) | accepted | minor |
| I8 | `/metrics` exposed with the model API | Comment says it is protected by the API key (config.go:125); needs a llama.cpp behaviour check | **partial** (unverified) | minor |
| I9 | LAN peers learn the model name and alias without a key | Depends on llama.cpp: `/v1/models` requires the key; error bodies may name the model | **partial** (unverified) | minor |

I1 detail: modern browsers default to `strict-origin-when-cross-origin`, so a
link from the setup page to `http://127.0.0.1:<model-port>/` (app.js:841) or to
GitHub (app.js:989) sends only the origin, not the query. That is luck, not a
control: one `Referrer-Policy: no-referrer` header, or moving the token out of
the URL except for SSE, closes it. The history entry stays valid until the
process exits.

### Denial of service

| # | Threat | Current control | Status | Severity |
|---|---|---|---|---|
| D1 | Slowloris from a local user: connections held open | `ReadHeaderTimeout: 10s` only; no `ReadTimeout`, `WriteTimeout` or `IdleTimeout` (server.go:224) | **partial** | minor |
| D2 | Unbounded request bodies: `json.NewDecoder(r.Body)` with no `MaxBytesReader` on every POST handler (server.go:90-221) | Token required; loopback only | **partial** | minor |
| D3 | SSE connections pinned open | One per page; no cap, no timeout (server.go:275-303) | open, loopback only | minor |
| D4 | A LAN peer floods the single model slot (`-np 1`, config.go:126) and starves the owner | API key required; one slot is deliberate for memory | accepted (LAN shares the model) | minor |
| D5 | Long jobs (scan, deploy, update download) block idle shutdown | Idle check skips while `Busy` (main.go:311-313) | accepted | minor |

### Elevation of privilege

| # | Threat | Current control | Status | Severity |
|---|---|---|---|---|
| E1 | The control plane is user-level root: any token leak becomes uninstall, kill, storage move, app launch/close | Host check + token + no CORS + `X-Frame-Options` (S1, S2, S4, T1). The chain depends on I1 and I3 not leaking | **partial** | major |
| E2 | LAN mode binds llama-server to every interface, including public Wi-Fi, with no separate confirmation | Opt-in toggle (`SetLAN`, app.go:990-1010); key required; `LANURL` names the LAN address (config.go:102-111) | accepted; the UI should say "only on networks you trust" | minor |
| E3 | Connected apps (OpenCode, Hermes) store the key in their own config files, with permissions EZLM does not set | `integrate` writes the key into the app's config; see a15/T11 | open, outside T07's files | minor |

## 6. What holds the line today

Ordered by how much they carry:

1. The Host check (server.go:232-237) makes DNS rebinding a non-issue.
2. The token on every `/api/` route (server.go:248-260) makes CSRF a non-issue
   until the token leaks.
3. The token is per launch and 128-bit; the API keys are 238-bit.
4. Constant-time comparison (server.go:254).
5. No CORS headers, so a page cannot read an API response.
6. `config.json` is 0600 and the service files are 0600.
7. Report redaction for the known key formats.

## 7. Open questions

1. Does llama.cpp's `/metrics` and `/v1/models` really reject requests without
   the key? The comment in config.go:125 says so; nobody has checked.
2. On Windows, what ACL does `~/.ezlm` actually get on a standard user
   (inherited from `%USERPROFILE%`, or Everyone-readable when the profile is on
   a share)? Code-read only tonight (a07).
3. Is the token in browser history acceptable for a per-launch secret? The
   entry dies with the process, but the user may share a screenshot.
4. Should `/api/kill` accept only PIDs from the last scan? Today any PID works.
5. Does the native app ever pass the token on a command line or in a log, as
   opposed to the child's stdout? a15 covers the clients.

## 8. How to verify (map to T07 agents)

| Control | Agent / test |
|---|---|
| Host check | a01, `internal/web/host_rebinding_test.go` |
| CSRF and simple POST | a02 |
| CORS | a03 |
| Token in the URL | a04 |
| LAN exposure | a05, two `httptest` clients |
| `--api-key` on argv | a06, `internal/serve` |
| File modes | a07 |
| Redaction | a08 |
| Constant-time compare | a09 |
| Request parsing | a10 fuzz, a14 handlers |
| Key storage and rotation | a12 |
| Apps' clients | a15 |

## 9. Sources

Read for this document: `internal/web/server.go`, `internal/web/static/app.js`,
`internal/keys/keys.go`, `internal/serve/config.go`, `run.go`, `client.go`,
`service_darwin.go`, `service_linux.go`, `service_windows.go`,
`internal/paths/paths.go`, `internal/app/app.go` (Kill, SetLAN, keys,
Uninstall), `internal/app/settings.go` (SetStorage), `internal/app/launch.go`
(Report), `internal/integrate/log.go`, `cmd/ezlm/main.go`, `windows/EZLM/Engine.cs`,
`macos/Sources/EZLM/Engine.swift` (token transport only).
