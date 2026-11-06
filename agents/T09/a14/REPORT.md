# T09 / a14: stall watchdog review

Slice: the stall watchdog in `internal/engine/download.go`. Thresholds, what
counts as progress, false positives on slow links, and its interaction with the
retry policy. Tests in `internal/engine/download_watchdog_test.go`.

## What the watchdog does today

- `stallTimeout = 45s` (`download.go:21`). A goroutine per attempt ticks every
  `stallTimeout/9` (`download.go:109`) and cancels that attempt when
  `time.Since(last) > stallTimeout` (`download.go:116`).
- `last` is stored once before the request (`download.go:106`) and then only
  when a body read returns bytes (`download.go:175`). Header arrival was not
  activity.
- A cut attempt returns `errStalled`, which `Download` retries. The retry
  budget (`download.go:71-80`) is "five failures in a row without the .part
  file growing". Any growth resets it. After five, the error is
  `plainNetErr(errStalled)`, "the download keeps stalling...".
- Caller cancellation is not a stall: `Download` checks `ctx.Err()` before
  retrying and returns it.

## Findings

### 1. False stall on a slow link: header wait and first byte share one clock (major, fixed)

`last` was set only at request start, so the time waiting for response headers
and the time waiting for the first body byte were charged to the same
`stallTimeout`. A link where each wait is 80% of the threshold is alive the
whole time but was cut as a stall after one threshold.

Evidence: `TestWatchdogNoFalseStallBetweenHeadersAndFirstByte` (headers after
0.8s, first byte 0.8s later, threshold 1s). Before the fix it failed after
burning all five retries (25s):

```
download_watchdog_test.go:158: slow handoff was taken for a stall: the
download keeps stalling (no data arrives for a long time)
```

After the fix it passes with one request. The same test is the regression test.

Fix (`download.go:139-143`): store the watchdog clock again when the response
headers arrive, so the body gets a full `stallTimeout` of its own. Three lines
including the comment, no behaviour change for a genuinely silent server.

### 2. `ResponseHeaderTimeout` is unreachable (minor, documented)

`download.go:29` sets `ResponseHeaderTimeout: 60s`, but the watchdog already
cuts a request after 45s from request start, so the transport's 60s header
timeout can never fire for a download. It is dead configuration, not a bug in
behaviour. I left it: after fix 1 the watchdog is still the earlier of the
two, and making them agree is a design choice (see open questions).

### 3. Detection is up to one tick late (minor, documented)

With a 45s threshold and a 5s tick, a stall is detected between 45s and 50s
after the last byte. The comment on `stallTimeout` says "this long", so the
worst case is about 11% over. Not worth a timer reset on every read; a
`time.AfterFunc` per byte would allocate far more than it saves. Tests assert
"a gap of 2x the threshold is a stall", not a tight bound, so they stay
stable on a loaded machine.

### 4. Progress is "the file grew", so a stalling link that drips never gives up (minor, by design, documented)

`Download` resets the five-failure budget whenever `.part` grew
(`download.go:75`). A server that sends a few bytes and then stalls forever
counts as progress on every attempt, so the download never reaches the
"keeps stalling" error: it just runs at one byte per 45s. This is deliberate
for dropping connections (the comment at `download.go:69`), and for stalls it
is the difference between "slow but alive" and "alive but useless". A 4 GB
model at 1 byte per 45s would take centuries, but the user does see progress,
so it is not a hang.

Evidence: `TestWatchdogStallProgressResetsRetryBudget` needs eight stalled
attempts and finishes, proving more than five are allowed.

Residual risk: no cap on total stalls and no minimum-rate check. A CDN that
stalls every few seconds is retried forever with a 2s backoff, which is a
polite load but a slow download. Suggested fix if wanted: after N stall
retries, require the rate over the last attempts to be above some floor
(for example 1 KB/s) or fail with "the connection is too slow". Not done here
because it changes the retry policy that other T09 agents are testing.

### 5. Watchdog covers dial, TLS and redirects (minor, documented)

The clock starts before the request, so DNS, dial, TLS and any 302 hops count
as silence. Dial is capped at 20s and TLS at 20s (`download.go:27-28`), both
under 45s, so this is only a real cut when a redirect chain itself is slow.
Fix 1 gives the body a fresh budget after the final headers, which covers the
common case (a slow CDN after a Hugging Face 302). Left as is.

## Tests added (`internal/engine/download_watchdog_test.go`)

| Test | Boundary | Result before fix |
|---|---|---|
| `TestWatchdogSlowButAliveLink` | total transfer 3x the threshold, every gap 1/3 of it: one request, no cut | pass |
| `TestWatchdogGapOverThresholdResumes` | one gap of 2x the threshold: cut, resumed from the right offset | pass |
| `TestWatchdogNoFalseStallBetweenHeadersAndFirstByte` | 0.8 threshold of header wait plus 0.8 threshold to the first byte | **fail** (finding 1) |
| `TestWatchdogSilentStallsGiveUp` | headers then nothing, five times: "keeps stalling", `.part` kept for a resume | pass |
| `TestWatchdogStallProgressResetsRetryBudget` | eight stalled attempts that each deliver bytes | pass |
| `TestWatchdogCallerCancelIsNotARetry` | context deadline during a silent body: the context error, one request | pass |

The helper `watchdogSrv` serves Range requests and lets each test control the
header delay and the body timing, including "headers now, then silence" (a
handler that returns early would look like a short body, not a stall).

## Changed

- `internal/engine/download.go`: refresh the stall clock when the response
  headers arrive (finding 1). Everything else unchanged.

## Verified

`gofmt -l .` empty, `go build ./...`, `go vet ./...`,
`go test -p 2 ./internal/engine/`, `GOOS=windows GOARCH=amd64|arm64 go build
./...`, `GOOS=linux go build ./...` all pass.

## Open questions

1. Should the header phase have its own budget, or should the total time to
   first byte be capped? Today: 45s to headers plus 45s to the first body
   byte, 90s worst case. I chose that over a shared clock because the failure
   mode it removes (a false stall on a slow link) is the one users hit.
2. Should `ResponseHeaderTimeout` be removed or set to `stallTimeout`? It is
   currently dead. A single knob would be easier to reason about, but
   `dlClient` is shared with other callers, so I did not touch it.
3. Should a stall with progress have a separate, larger budget than a stall
   with none (finding 4)? The current policy lets a byte-per-stall server run
   forever.
4. Out of my slice, for a02/a03: a 416 on a resume with `want == 0` returns
   `nil` (`download.go:148-149`), so an oversized or stale `.part` is renamed
   to the final name with no size check. With `want > 0` the size check
   catches it.
