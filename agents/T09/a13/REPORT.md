# T09 / a13: the progress and observability path

Scope: what a caller sees during `engine.Download` (bytes, speed, ETA, errors)
and whether a failure can be told apart from a stall. Read-only for
`internal/app`; code changes only in `internal/engine/download.go` plus one new
test file.

## Answer to the question

- **At the end**: yes, a stall is distinguishable from a failure. The final
  error text differs: "the download keeps stalling (no data arrives for a long
  time)" (`internal/engine/download.go:237`) vs "the connection to the download
  server keeps dropping" (download.go:245) vs the permanent 404/403 texts
  (download.go:251-264).
- **While it happens**: no. Before this change a stalled or dropped transfer
  produced **zero** progress calls between the last byte of one attempt and the
  resumed byte of the next. The UI kept the last speed and ETA on screen for
  the whole stall (45 s by default, 245 s before the download gives up), so a
  stalled download was pixel-identical to a hung one.
- **Fix**: `Download` now emits one progress tick with `bps == 0` and the bytes
  actually on disk at every retry boundary, i.e. as soon as the watchdog cuts a
  silent connection or a body read fails (download.go:78-83). Both apps already
  hide speed and ETA when `bps <= 0` (windows/EZLM/Install.cs:258-261,
  macos/Sources/EZLM/AppModel.swift:549-551, 689-692), so the frozen "12 min
  left" disappears and the step shows bytes so far instead.

## Findings

| # | Severity | Finding | Evidence |
|---|---|---|---|
| 1 | major | A stall is invisible to the caller: no progress tick from the watchdog (`download.go:112-128`) or from the retry loop (`download.go:69-90`). With the default 45 s stall timeout and 5 attempts, the UI can sit unchanged for ~4 minutes before any error appears (`fails >= 5`, download.go:84; backoff 2+4+6+8 s, download.go:90). | `TestProgressReportsStallBeforeRetry` failed before the fix: "3 ticks ... none with speed 0"; passes after. |
| 2 | major | A dropped connection is equally invisible. `dropped{}` (download.go:218) is only used to pick the final message. | `TestProgressReportsDropBeforeRetry` failed before, passes after. |
| 3 | minor | Progress can move backwards on a restart. When a server answers 200 to a Range request, `have` is reset to 0 and the part file truncated (`download.go:152-154`); the first tick after the restart is the first read, so `done` drops to a few KB. Windows keeps its bar (`Math.Max`, Install.cs:255) but the text regresses; the Mac app assigns the fraction directly (`AppModel.swift:545`), so its bar visibly jumps back. | `TestProgressRestartIsFromZero` (asserts at most one backward step, then monotonic to the whole file). |
| 4 | minor | `bps` could be `+Inf` when a body completed with no measurable elapsed time (`float64(sinceStart)/0` in the old download.go:179/184). `internal/app` marshals `Event` to JSON (`internal/web/server.go:271`), and `encoding/json` rejects `+Inf`, which would drop the event. Not reproduced (the monotonic clock has ns resolution); guarded anyway. | `rate()` in download.go:176-181. |
| 5 | minor (app) | The apps compute ETA from a single instantaneous sample, `(total-done)/bps` (Install.cs:259-260, AppModel.swift:689-692), and the sample restarts at each attempt (`start, sinceStart` are per `fetch`, download.go:169-170). For a few hundred ms after a resume the ETA is derived from one read, which is optimistic by however long the gap was. | code reading; not a correctness bug. |
| 6 | minor (app) | Progress ticks are never stored in history (`internal/app/app.go:110-114`) and are dropped for a full subscriber channel (app.go:112-114). A UI that reconnects mid-download shows nothing until the next tick; on a stalled connection that is up to 45 s of empty bar. | code reading. |
| 7 | minor (app) | `DownloadModel` passes `q.Bytes` as the total for split GGUFs while each part is fetched with `want == 0` (`internal/engine/engine.go:471-484`). If the catalog size is off, the bar stops short of 100% on a successful download. | code reading. |
| 8 | note | `Download` treats an existing `dst` as complete when `want == 0` (download.go:45). For split GGUFs and update assets that is the only size check, so a short file from an earlier run is reported as 100% complete. Cross-cutting with a02/a07/a11. | code reading. |

### Is a failure distinguishable from a stall?

During the transfer: only by the new zero-speed tick, which both faults
produce, so **no**. At the end: **yes**, the error strings differ and a
permanent failure returns immediately without retries (download.go:71-74). If
the app needs to say "retrying" rather than "stalled", it needs a new signal,
which means a change to `Progress` or `app.Event` (both outside this slice).

## What I changed

`internal/engine/download.go`:

1. In `Download`'s retry path, after refreshing the on-disk size and before
   the backoff sleep, emit `prog(have, want, 0)` when `prog != nil`
   (download.go:78-83). `have` is the real file size, so a caller never sees
   more bytes than are on disk. `want` is the size the caller asked for; when
   it is 0 the caller's wrapper supplies the total (`engine.go:242-244`,
   `engine.go:475-478`).
2. Replaced the two inline speed expressions with `rate()`, which returns 0
   when no time has elapsed (download.go:176-181, 193, 198).

No signature changes, so `internal/app`, `internal/update` and
`internal/integrate` are untouched.

## Tests

`internal/engine/download_progress_test.go` (new, 6 tests, hermetic,
`httptest` only, no real network):

- `TestProgressTicksAreUsable`: known total, done never exceeds it, a positive
  speed while bytes arrive, final tick equals the file size.
- `TestProgressWhenFileIsAlreadyComplete`: exactly one tick at 100% with no
  speed and no request.
- `TestProgressReportsStallBeforeRetry`: silent server, watchdog fires; a
  zero-speed tick at the bytes on disk must appear before the resume. Failed
  before the fix, passes after.
- `TestProgressReportsDropBeforeRetry`: connection cut mid-body; same check.
  Failed before the fix, passes after.
- `TestProgressWithUnknownTotal`: chunked body and `want == 0`; total stays 0
  (the apps' "unknown" case), done still reaches the end.
- `TestProgressRestartIsFromZero`: server ignores Range; progress may step
  back once (the restart) and then grows to the whole file.

## Verification

- `go test -p 2 -count=1 ./internal/engine/`: ok.
- `gofmt -l .`: empty; `go build ./...`: ok; `go vet ./internal/engine/`: ok.
- `GOOS=windows GOARCH=amd64 go build ./...`, `GOOS=linux go build ./...`: ok.
- Commit: `738a819` (test + fix). This report is the second commit.

## Could not finish / next steps

- The UI still cannot say *why* it went quiet. Next step (app-owned): give
  `app.Event` a retry/stall state, e.g. a `state: "retrying"` progress event
  emitted from the same place as the new tick, and render "waiting for data"
  with the attempt count. `Progress` would need a variant that carries the
  attempt number or the error class, which is a change in `engine.go` and
  `app.go`.
- ETA smoothing (a moving average over the last few ticks) belongs in the
  apps; the engine should keep reporting the current attempt's speed.
- `Download` reports nothing while the HTTP request is in flight and before
  the first body byte (connect + headers can take 60 s under
  `ResponseHeaderTimeout`, download.go:29). A caller showing a spinner for
  `deploy` is fine today because `app` emits the "Getting the model" step
  first, but a zero-progress tick at request start would make that explicit.
- Open question: should a permanent failure (404) emit a final tick so the bar
  freezes at the bytes on disk instead of the last 200 ms sample? Today the
  job's `fail` event replaces the step text, so it matters little.

## Open questions

- Is a 4-minute silent retry window acceptable for the beta? The alternative
  is a shorter give-up budget (5 attempts is arbitrary) or an app-visible
  retry event. The stall watchdog's thresholds belong to a14.
- Split GGUFs have no per-part size in the catalog (`engine.go:466-468`), so
  per-part totals and a meaningful overall ETA are impossible without a
  catalog change.
