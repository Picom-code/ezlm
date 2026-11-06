# T07/a15: the apps' engine clients and the per-launch token

Scope: read-only review of the Windows (C#, `windows/EZLM`) and Mac (Swift,
`macos/Sources/EZLM`) engine clients for how they carry the per-launch engine
token (`X-EZLM-Token`) and the API key: URL vs header, logs, and crash or
problem reports. No app files were edited; every fix below is a proposal.

All line numbers are from the T07 snapshot (branch `agent/T07-a15`).

## How the token actually travels (verified)

1. The engine prints one JSON line on stdout, `{"url","token"}`:
   `cmd/ezlm/main.go:236`. The URL has no token in it.
2. Windows parses that line and puts the token on the shared `HttpClient` as a
   default header: `windows/EZLM/Engine.cs:77-80`. Every later engine call
   (GET `Engine.cs:111`, POST `Engine.cs:118`, SSE `Engine.cs:145`) inherits it.
3. Mac does the same per request: `macos/Sources/EZLM/Engine.swift:88-91`
   (parse) and `Engine.swift:113` (all requests), `Engine.swift:159` (SSE).
4. Neither app ever builds a URL containing the token. Grep for `?t=`, `token=`
   and `?key=` in both app trees returns nothing; the only `X-EZLM-Token`
   strings are the two files above.
5. The engine still accepts the token in the query string, for EventSource:
   `internal/web/server.go:249-254`, and `web.Listen` returns a URL that
   contains it: `internal/web/server.go:82`. That is the *browser* UI's path,
   not the apps'. Both apps use the header even for SSE, so this slice does not
   widen the `?t=` exposure (that is a04's).
6. The API key (not the launch token) is sent to llama-server as
   `Authorization: Bearer <key>`: `windows/EZLM/Chat.cs:745`,
   `macos/Sources/EZLM/ChatModel.swift:246`. Header, not URL.
7. The token is never written to disk by either app: it lives in
   `Engine.token` / `Engine._token` only. `Engine.cs:107`,
   `Engine.swift:10,88`.

So the headline is good: header everywhere, URL nowhere, no persistence. The
findings below are about the edges of that.

## Findings

| # | Finding | Evidence | Severity |
|---|---------|----------|----------|
| F1 | Token is sent to whatever host the engine's hello line names; neither app checks it is loopback | `Engine.cs:78-80`, `Engine.swift:90-91` | minor |
| F2 | After the engine dies the SSE retry loop keeps sending the stale token to the stale port until the app is restarted; a local process that grabs the port receives it and can feed the app events | `Engine.cs:139-161`, `Engine.swift:154-175` | minor |
| F3 | Engine stderr is logged verbatim into the support bundle, but report redaction does not recognise the 64-hex launch token | `Engine.cs:84-96`, `Engine.swift:48-51`, `internal/integrate/log.go:34-43`, `server.go:81` | minor (latent) |
| F4 | Both apps copy the API key to the system clipboard automatically when opening llama.cpp's chat | `Panes.cs:246-252`, `ReportLaunch.swift:9-16` | minor |
| F5 | Support pastes hide only `ezlm-…` shaped keys; the engine-side report redaction also covers `sk-…` and `Bearer …` | `Shell.cs:214-215`, `AppModel.swift:1040-1050`, `internal/integrate/log.go:34-43` | info |
| F6 | Engine client follows redirects and uses the system proxy; a redirect or proxy would receive the token header | `Engine.cs:14`, `Engine.swift:141,161` | info |
| F7 | Crash notes/reports: no token path found; verified clean today | `App.xaml.cs:12-17`, `Shell.cs:189-198`, `ReportLaunch.cs:192-198`, `ReportLaunch.swift:366-412` | none (checked) |

### F1: the hello URL is trusted without a loopback check (minor)

`Engine.cs:78` assigns `baseUrl = hello.Url` and line 80 attaches the token to
every request to it. `Engine.swift:90` accepts any `URL(string:)` and line 91
stores it. Today the engine always prints `http://127.0.0.1:<port>`
(`cmd/ezlm/main.go:236`, `web.Listen`), so this is not exploitable by itself.
It matters as defence in depth: a replaced `Engine/ezlm.exe`, an `EZLM_ENGINE`
override (`Engine.cs:44-49`, `Engine.swift:27-30`) pointing at a wrapper, or a
future engine that prints a non-loopback URL would ship the token off-box over
plain HTTP.

Proposed fix (app side, small):
- Windows, after `Deserialize<Hello>`: reject unless
  `Uri.TryCreate(hello.Url, UriKind.Absolute, out var u) && u.Scheme == "http"
  && IPAddress.IsLoopback(u.HostIdnHostAddress)` (or `u.Host is "127.0.0.1" or
  "[::1]"`). Same check in `StartAsync` before storing the token.
- Mac, after `URL(string: hello.url)`: `guard url.scheme == "http",
  url.host == "127.0.0.1" || url.host == "::1"` before `_token = hello.token`.

### F2: stale token to a stale port after the engine exits (minor)

`ListenAsync` is started once (`Shell.cs:154`, `AppModel.swift:240`) and loops
forever, reading `baseUrl`/`token` fresh each pass. When the engine dies the
app shows the "engine stopped" screen but the loop keeps retrying
`{baseUrl}/api/events` with the old `X-EZLM-Token` every 500 ms
(`Engine.cs:160`, `Engine.swift:170`) until the user retries. On Windows the
token is a default header, so it goes out on that request by construction
(`Engine.cs:79-80`).

On loopback, any process of the same user can bind the freed port. It then
receives a token that is already useless (the engine that owned it is gone),
but it also becomes the app's event source: `onEvent` drives deploy progress,
launch state and toasts (`Engine.cs:156`, `Engine.swift:164-166`), so the
impostor can paint a fake "install failed" or "opened" state. Impact is
confusing the user, not key theft.

Proposed fix: on `Exited`/`onExit`, cancel the listen loop and restart it only
after `StartAsync` has a fresh hello (both apps already have a `quit`/
cancellation token and an `engineGone` latch). Optionally have the engine
write its pid into the hello line and verify the peer is that pid, which is
more work than the risk warrants.

### F3: engine stderr in the support bundle is redacted for `ezlm-` keys only (minor, latent)

Windows pumps engine stderr into `%LOCALAPPDATA%\EZLM\engine.log`
(`Engine.cs:84-96`); Mac appends it to the in-memory `AppLog`
(`Engine.swift:48-51`, `AppModel.swift:1071-1093`). Both bundles are shipped
with problem reports: Windows sends the tail of `app.log`
(`ReportLaunch.cs:192-198`) and "Copy details for support" reads `engine.log`
and `app.log` (`Shell.cs:209-213`); Mac sends `AppLog.shared.tail(6000)` plus
the crash note (`ReportLaunch.swift:236`) and `copyDiagnostics` includes the
server log and the app log (`AppModel.swift:1024-1035`).

Redaction happens in the engine, not in the apps:
`integrate.Redact` strips the literal key, then `ezlm-[A-Za-z0-9]{6,}`,
`sk-…` and `Bearer …` (`internal/integrate/log.go:34-43`); the report handler
applies it with the configured keys (`internal/app/launch.go:230-237`).
The launch token is `hex.EncodeToString(32 random bytes)` with no prefix
(`internal/web/server.go:81`), so none of those patterns match it.

Today nothing writes the token or the `?t=` URL to engine stderr: `engine()`
prints the hello line to stdout only (`cmd/ezlm/main.go:236`) and stderr is
used for fatal `ezlm: <err>` lines. But `web.Listen`'s URL with `?t=` is one
`log.Println` away from being in every beta report, and no app-side filter
would catch it. That is a trap for the next person, not a live leak.

Proposed fix (smallest, cross-task): make the launch token recognisable, e.g.
`"ezlm-t-" + hex` in `server.go:81`, and add that prefix to
`integrate.Redact`. `guard` already takes the header or `?t=`, so the format
change is internal. App side, optionally drop engine stderr from `app_log`
when `details` is off.

### F4: the key is copied to the clipboard automatically (minor)

Opening llama.cpp's built-in chat copies the API key first, because the page
asks for it: `Panes.cs:246-252` and `ReportLaunch.swift:9-16`, with a toast
saying so. That is deliberate and documented in the code comment, but the
clipboard is readable by every process of the user, survives the app, lands in
Windows clipboard history (Win+V) and can sync to the cloud, and on macOS
Universal Clipboard can carry it to other devices.

Proposed fix (product call, not a bug): copy only on an explicit "Copy key"
click and show the key in the page, or clear the pasteboard after a short
delay. Keep the existing "Manage keys"/"Copy key" buttons (`Panes.cs:704`,
`ReportLaunch.swift:10`) as the explicit path.

### F5: app-side redaction is narrower than engine-side (info)

`Shell.cs:215` and `AppModel.swift:1040-1050` use `ezlm-[A-Za-z0-9]{6,}`
only; the engine also hides `sk-…` and `Bearer …`. Today the only key the
status payload carries is an `ezlm-` key (`internal/keys/keys.go:11-22`), so
the two agree in practice. If a key of another shape ever reaches the status
JSON, the app-side paste would leak it.

Proposed fix: mirror the three patterns in both `HideKeys`/`hideKeys`
helpers, or have the engine return keys already masked in `status`.

### F6: proxy and redirects (info)

`new HttpClient()` (`Engine.cs:14`) uses the system proxy and follows
redirects by default; `URLSession.shared` (`Engine.swift:141,161`) also uses
the system proxy. The engine sends no redirects today (grep for
`http.Redirect` in `internal/` and `cmd/` is empty), so neither path is live.
A corporate proxy configured without a loopback bypass would see the token
header in clear text, and a future redirect would forward the header.

Proposed fix: build the client with
`new HttpClientHandler { UseProxy = false, AllowAutoRedirect = false }` and,
for Mac, a `URLSessionConfiguration.ephemeral` with
`connectionProxyDictionary = [:]`. Also keeps the token out of any URL cache.

### F7: crash paths (checked, clean)

- Windows logs `ex.ToString()` from the three global handlers to `app.log`
  (`App.xaml.cs:12-17`, `Shell.cs:189-198`), and the report sends the tail of
  that file (`ReportLaunch.cs:192-198`). The engine's exception text is the
  server's error body (`Engine.cs:124-133`); engine URLs carry no token, so
  the message cannot contain one.
- Mac's `CrashNote` summarises the `.ips` (exception, termination, up to 12
  symbols) and strips `/Users/<name>` (`ReportLaunch.swift:366-412`); it does
  not include heap strings, so the in-memory token is not in it.
- Residual risk: a full WER minidump or a macOS crash dump contains process
  memory, so a token could be in a dump the user attaches by hand. Per-launch
  and loopback-only, so low value.

## What the apps get right (worth keeping)

- Header transport, including for SSE, so the token is not in URLs, `Referer`
  headers, history or server access logs.
- Token in memory only; a new token per engine launch; hello parsed from the
  child's stdout, not from a file or an argument.
- The API key to llama-server is a `Bearer` header (`Chat.cs:745`,
  `ChatModel.swift:246`), and the UI shows the key masked
  (`Install.cs:346`, `Panes.cs:702`) with an explicit copy button.
- `copyDiagnostics` / "Copy details for support" hide `ezlm-` keys on both
  platforms, and the Mac report page (`ReportLaunch.swift:262-269`) applies
  `hideKeys` and the home-folder replacement before opening GitHub.

## Open questions

1. Should the hello line also carry the engine pid, so the apps can tell "my
   engine" from "something on the same port" (F2)? Cheap on the engine side,
   more code in both apps.
2. Is the automatic key copy (F4) load-bearing for the llama.cpp chat flow? If
   the page can pre-fill from the URL fragment (`#key=`), the clipboard step
   can go.
3. Does anything else in the engine write to stderr on a normal run? If a
   debug flag is added later, F3 becomes live immediately; the prefixed token
   fix is the cheap insurance.
