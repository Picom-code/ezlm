# T16 / a11: networking, timeouts, retries, the engine client

Mac with Swift 6.2, arm64, macOS 14 SDK. `cd macos && swift build && swift test` pass
(31 tests, 9 of them new). Reviewed `Engine.swift`, `ChatModel.swift` (the chat HTTP
client), `Updater.swift` (local only, no network) and the call sites in `AppModel.swift`,
`ReportLaunch.swift`, `ModelMode.swift`. The engine side (`internal/web/server.go`) was
read to check what the client is actually talking to: loopback only, `X-EZLM-Token`
header, JSON errors as `{"error": "..."}`, and `/api/events` as SSE with a `: ping`
comment every 15 s.

## What I changed (`macos/Sources/EZLM/Engine.swift`)

1. **Reconnect wait was a fixed 500 ms forever** (was line 170). An engine that had
   exited, or that crashed on every start, was asked twice a second for as long as the
   app was open, with no ceiling and no diagnostics. Now `Backoff` doubles from 0.5 s to
   8 s, and resets when a stream stayed up for more than 5 s. The wait is a small struct
   so the policy is testable without a server.
2. **Transport errors reached the person as URLSession's own words.** `sendRaw` let
   `URLError` escape, and about a dozen call sites in `AppModel.swift` do
   `note = error.localizedDescription`. Measured on this Mac:
   `URLError(.cannotConnectToHost).localizedDescription` is
   `The operation couldn't be completed. (NSURLErrorDomain error -1004.)`, and
   `.timedOut` gives `... error -1001.` That is what a user saw when the engine was
   down. `Engine.friendly(_:)` now maps connection, timeout and lost-connection errors
   to one plain sentence with a next action, and leaves `.cancelled` alone.
3. **GET had no retry.** `get`/`getRaw` now retry once after 250 ms on
   `cannotConnectToHost`, `cannotFindHost` and `networkConnectionLost`. POST is
   deliberately not retried: `scan`, `deploy`, `integrate`, `report` and `update` all
   have side effects, and running one twice is worse than showing the error.
4. **The event stream's idle timeout was 3600 s** while the engine pings every 15 s. A
   socket that died during sleep was not noticed for up to an hour. It is now 90 s, six
   missed pings.
5. **Address and token were read under two separate locks.** A restart between the two
   reads produced a request with the old port and the new token, which the engine
   answers with 401 `this page is out of date: reopen EZLM`, advice that is wrong there.
   Both now come from one lock as a pair.
6. `Engine` takes an injectable `URLSession` (default `.shared`) and a `use(base:token:)`
   seam, so the request path can be tested without a process. The app's own call sites
   are unchanged.

Tests are in `macos/Tests/EZLMTests/NetworkTests.swift`: backoff progression, ceiling,
reset; the three friendly messages; cancellation left alone; a 400 carrying the engine's
`error` string through to the person; a connection failure; the path, query and token
that reach the engine (served by a `URLProtocol` stub, no port, no process).

## Findings not fixed here

- **minor, `AppModel.swift:246`**: boot does one `engine.get("status")` with no retry.
  Any single failure goes straight to the failure screen. The new GET retry in the
  client covers the common case; a second attempt in `boot` would cover the rest.
  (`AppModel.swift` is frozen, so this is report-only.)
- **minor, `Engine.swift:71-79`**: when the 30 s hello line never arrives, `start()`
  throws but leaves the process it spawned running. It exits with the app (stdin), and
  a retry calls `stop()` first, so this leaks only for someone who never retries.
- **minor, `ChatModel.swift:243`**: the chat URL is force-unwrapped. A `localUrl` from
  the engine that `URL(string:)` cannot parse crashes the app instead of showing the
  usual "Couldn't reach your model" error. Engine-controlled, so unlikely.
- **minor, `ChatModel.swift:78`**: 900 s idle timeout for a chat request. Fine with
  Stop available, but a model that hangs shows a spinning signal for 15 minutes.
- **observation, `ChatModel.swift:303-307`**: chat already treats
  `cannotConnectToHost`/`networkConnectionLost`/`timedOut` with its own words; that part
  is in good shape. No retry is attempted, which is right for a user-visible answer.
- **overlap with a02**: the 90 s stream idle timeout and the reconnect backoff both
  touch SSE recovery after sleep/wake. My change is small and in `Engine.events()`
  only; a02 may also touch this file.

## Verified on this Mac

`swift build` and `swift test` pass; the new tests fail to compile against the old
`Engine.swift` (no `Backoff`, no `friendly`) and pass now. Not verified: real sleep/wake
behaviour and a real engine restart, which need a device run.

## Open questions

- Should a POST that fails with `cannotConnectToHost` be retried when the engine is
  known to be restarting? Today the failure screen and "Try again" are the path.
- Is 90 s the right idle limit, or should the client ask the engine for a ping interval?
