# T09 / a04: stalls and slow drips

**Fault:** a download whose server goes silent mid-transfer must be cut and resumed;
a connection that is slow but still delivering must not be cut.

**Worktree:** `~/ezlm-agents/work/subs/T09/a04`, branch `agent/T09-a04`.

## What the code does today

`Download` (`internal/engine/download.go:44`) retries `fetch` in a loop and resumes
from `<dst>.part` on each attempt. `fetch` (`download.go:100`) starts a watchdog
goroutine (`download.go:108-123`) that cancels a per-request context when no body
byte has arrived for `stallTimeout` (45 s, `download.go:21`). A cancelled request
surfaces as `errStalled` (`download.go:34`), which `Download` retries; if five
attempts in a row make no progress, `plainNetErr` reports
"the download keeps stalling ..." (`download.go:222-223`).

## Reproduction

I extended `internal/engine/download_stall_test.go` (the existing two tests are
kept unchanged) with four `httptest` cases:

| Test | Shape | Asserts |
|---|---|---|
| `TestDownloadResumesAfterStall` (existing) | half the body, then silence, HTTP/1.1 | exactly one resume, correct bytes |
| `TestDownloadResumesAfterStallHTTP2` (existing) | same over HTTP/2 | cut and resume, no hang |
| `TestDownloadSlowDripDoesNotStall` | 30 pieces, 120 ms apart (3.6 s total, 9x the 400 ms timeout) | completes, correct bytes, one request |
| `TestDownloadDripWithGapsJustUnderTimeout` | 12 pieces, gaps 0.73x the timeout | completes, correct bytes |
| `TestDownloadStallOnChunkedBodyFires` | headers, half a chunked body (no `Content-Length`), then silence | cut and resumed, correct bytes |
| `TestDownloadStallBeforeBodyReportsStalling` | headers only, never a body byte | returns "keeps stalling" instead of hanging; the attempt is retried |

**Result at baseline: every one of them passes.** The watchdog fires on a silent
body (with and without `Content-Length`, HTTP/1.1 and HTTP/2) and does not fire
on a drip whose gaps stay under the threshold, because it measures the time since
the last body byte, not the length of the transfer. I could not reproduce a
watchdog bug, so there is **no change to `download.go`**; the fix column for this
fault is "already correct, now covered".

Before/after is therefore identical: the new tests fail if the watchdog is
removed (`fetch` would block on `Read` until the 20 s dial / 60 s header timeouts
or forever on a body), which I confirmed by running the tests against a copy with
the goroutine disabled, but the shipped behaviour is unchanged.

## Findings (read-only, no fix)

1. **minor, `download.go:21` vs `download.go:29`:** the stall watchdog cuts a
   request after 45 s without body bytes, which is before
   `ResponseHeaderTimeout` (60 s) can ever fire. A server that is alive but takes
   45-60 s to send response headers (a loaded CDN on a cold range request) is cut
   and retried, and a server that always takes that long fails after five
   attempts even though a single request would have finished. The header timeout
   is effectively dead code.
2. **minor, `download.go:109`:** the watchdog checks every `stallTimeout/9`
   (5 s in production), so worst-case detection is ~50 s, and the retry backoff
   (2, 4, 6, 8 s) adds to the time before a stall is reported. Fine for a
   multi-GB download, worth knowing for the progress path (a13).
3. **minor, `download.go:171`:** the progress callback runs on the read path. A
   caller whose `Progress` blocks for longer than `stallTimeout` makes a healthy
   transfer look stalled. `internal/app`'s `emit` is non-blocking
   (`internal/app/app.go:108-125`), so this is latent, not live.
4. **open question:** there is no throughput floor, by design. A server that
   sends one byte every 44 s keeps a 20 GB download "alive" for years. a14 owns
   whether that should be a separate low-speed timeout; the brief for this slice
   says a slow-but-alive connection must not fire, so I left it alone.
5. **note:** `stallTimeout` and `retryStep` are mutable package globals. The
   tests mutate them in sequence (`withStallTimeout`, `fastRetries`), so they are
   safe today, but a future `t.Parallel` test would race on them.

## Residual risk

- The tests use a real local TCP connection, so they exercise the transport but
  not a real CDN. A half-open connection (Wi-Fi drop with no FIN/RST) is the
  same code path as the silent-body tests.
- `stallTimeout` is shortened to 150-400 ms in tests; the production 45 s value
  is never exercised by the suite (by design, to keep the tests fast).

## Checks

`gofmt -l .` empty, `go vet ./internal/engine/`, `go test -p 2 -count=1 ./internal/engine/`
all pass in the worktree.
