# T09 a05: connection resets at different offsets

**Fault:** a proxy or CDN cuts the TCP connection at an arbitrary point: before the
response, in the middle of the headers, in the middle of the body, or right after the
last byte. The download must retry, resume from the bytes already on disk, and never
throw away a finished file.

**Test file:** `internal/engine/download_reset_test.go` (all tests use `httptest` and a
real TCP reset: hijack the connection, `SetLinger(0)`, close).

## What was reproduced

| Case | Test | Before the fix | After |
|---|---|---|---|
| reset before the status line, twice, then the file | `TestDownloadResetBeforeHeaders` | pass | pass |
| reset in the middle of the response headers | `TestDownloadResetMidHeader` | pass | pass |
| reset every 8 KB through the body | `TestDownloadResetMidBody` | pass | pass |
| reset after the last byte of a chunked body (no closing chunk) | `TestDownloadResetAtLastByte` | pass | pass |
| reset after the last byte, then every resume request also resets | `TestDownloadResetAtLastByteKeepsFile` | **fail** | pass |
| server resets every request, file never starts | `TestDownloadResetEveryRequestFailsCleanly` | pass | pass |

The four offsets themselves were already handled by the existing retry loop: a reset is
not `permanent`, the retry sends `Range: bytes=<part size>-`, and progress resets the
failure counter. What was broken is the case where the reset arrives at the very end.

### Finding: a complete `.part` is thrown away when the resume keeps resetting (major)

`Download` only checked the destination file, then retried. If the last body byte was
written to `.part` and the connection reset before the client saw the end of the body,
the next request asked for `Range: bytes=<want>-`. If that request (and the next four)
reset before the headers, `Download` gave up after five failures with "the connection to
the download server keeps dropping" and never renamed the finished `.part`. The user lost
a multi-GB download that was complete on disk. Evidence: the failing test above, which
takes one attempt plus five pointless retries and then errors with the file already
complete in `model.gguf.part`.

Fix, `internal/engine/download.go:57-63` (6 lines): at the top of the retry loop, when
`want > 0` and the bytes on disk already equal `want`, stop retrying and finish. The
final size check and rename still run, so a corrupt or oversized file is still rejected
by the existing code. With the fix the dead server is asked exactly once and the file is
kept.

## Residual risk

- **Reset in the middle of the headers on a kept-alive connection.** Go's transport
  retries an idempotent request once when a reused connection dies before any response
  byte; the test does not pin that behaviour, only that the download still finishes.
- **HTTP/2 `RST_STREAM`.** Not covered here. A stream reset surfaces as
  `http2.StreamError`, which `plainNetErr` does not classify, so the final message would
  be the raw Go error instead of the plain "connection keeps dropping" text. Retry and
  resume still work. Candidate follow-up for a13 (error presentation).
- **The 30 ms pause before the reset** in the fake server is needed on macOS: an
  immediate `SetLinger(0)` close races the request and the client only notices via a
  30 s TCP keepalive. It is test scaffolding, not product code.
- A server that resets every request while `want == 0` (unknown size) still fails after
  five attempts. That is intended: without a known size there is no way to tell whether
  the bytes on disk are the whole file.

## Open questions

- Should `Download` also treat a `.part` that is complete but was left behind by a crash
  as success when `want == 0`? Today only the caller's size or checksum can decide.
- The retry budget is five consecutive failures without progress. On a link that resets
  at offset 0 for more than five attempts in a row but would recover, the download gives
  up. Is a longer budget (or a total-time budget) wanted for multi-GB files? Left to
  a14 (retry policy review).

## Checks run

`gofmt -l .` empty, `go build ./...`, `go vet ./...`,
`go test -p 2 -count=1 ./internal/engine/`, and the Windows amd64/arm64 and Linux
cross-builds all pass.
