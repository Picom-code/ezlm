# T09 a12: shared fake Hugging Face server

**Deliverable:** `internal/engine/fakehuggingface_test.go` (package `engine`, test only).
**Status:** done. `go test -race ./internal/engine/` passes.

## What it is

A configurable `httptest` stand-in for `huggingface.co` plus its CDN, so the
fault agents (a01 to a11) don't each grow their own inline handler. Two servers
start together: a primary that 302s to the CDN by default (like HF) and the CDN
that serves the bytes. `newFakeHF(t, data)` closes both with `t.Cleanup`.

```go
data := bytes.Repeat([]byte("ezlm-model-"), 4096)
hf := newFakeHF(t, data)
hf.Redirect = true                       // 302 to the CDN, like HF
hf.ResetAt = []int64{int64(len(data) / 2)} // cut the first response mid-body
err := Download(ctx, hf.URL("model.gguf"), dst, uint64(len(data)), nil)
reqs := hf.Requests()                    // path, Range, resume offset, status, ETag, target
```

`hf.URL(name)` is the caller's URL; `hf.CDNURL(name)` skips the redirect.
Fields are read per request, so they can be set before the first request; use
`SetETag`/`SetData` to change them while a download runs.

## Knobs, by fault

| Fault | Knob |
|---|---|
| a01 redirects, changed target on resume | `Redirect`, `RedirectTo(r, have, hit)` (hit = 1-based primary request) |
| a02 server ignores Range | `IgnoreRange` (200 + whole body) |
| a03 ETag change mid-download | `ETag`, `SetETag`, `SetData`; the fake honours `If-Range` and answers 200 when it no longer matches |
| a04 stalls, slow drips | `StallAfter` (bytes per response), `StallFor` (0 = until the client gives up), `Drip` (pause per 1 KiB write) |
| a05 resets at offsets | `ResetBefore` (before headers, N times), `ResetAt` (absolute offsets, each fires once, 0 and len-1 work) |
| a06 split GGUF parts | `Files` (path to body), `Missing` (404) |
| a07 checksums | serves arbitrary bytes; checksum logic is client-side |
| a08 to a11 | `Hits()`, `Requests()` for evidence; otherwise server-neutral |

Range handling matches a CDN: 206 with `Content-Range`, 200 for the first
request, 416 past the end, `Accept-Ranges`, `Content-Length`, ETag derived from
the body when `ETag` is empty.

## Self-tests (all in the same file, prove the fake works)

`TestFakeHFServesRangeThroughRedirect` (302 then CDN 200, bytes intact),
`TestFakeHFResumeThroughRedirect` (cut at half, second primary hit sees the
resume offset), `TestFakeHFIgnoreRange` (resume answered 200, download
restarts), `TestFakeHFStallIsResumed` (watchdog cuts at 250 ms, completes in
under 5 s), `TestFakeHFDripIsNotAStall` (100 ms between 1 KiB writes, one
request only), `TestFakeHFResetsAtOffsets` (pre-header, offset 0, mid-body,
last byte: 5 requests, file intact), `TestFakeHFSplitParts` (two parts plus a
404), `TestFakeHFSetETag` (header changes between requests).

## Finding: `stallTimeout` is read by the watchdog after `Download` returns

**Severity: minor** (test-visible data race, no user-visible misbehaviour).
**File:** `internal/engine/download.go:109` and `:116` (before the fix).

The watchdog goroutine read the package-level `stallTimeout` itself. A test
that shortens `stallTimeout` (as `download_stall_test.go` and a04/a12 do) and
restores it in a defer races with a watchdog goroutine that has not been
scheduled yet. Reproduced with `go test -race -run TestFakeHF ./internal/engine/`:

```
WARNING: DATA RACE
Write at 0x000100de0798 by goroutine 71:
  ...TestFakeHFStallIsResumed() fakehuggingface_test.go:440
Previous read at 0x000100de0798 by goroutine 49:
  ...fetch.func1() download.go:109
```

**Fix:** `fetch` reads `stall := stallTimeout` once before starting the
goroutine and the goroutine uses the local (5 added, 3 changed lines). After:
`go test -race ./internal/engine/` passes, including the existing stall tests.
The same fix means a mid-download change of the timeout can no longer give the
ticker and the threshold different values.

## Handed to a03 (not fixed here)

`download.go` sends no `If-Range` and never compares ETag: a resume after the
file changed appends the new file's bytes onto the old prefix
(`download.go:129-131` sets only `User-Agent` and `Range`). If the changed file
has the same size, the size check at `download.go:87-96` passes and the result
is silently corrupt. The fake already honours `If-Range`, so a03's fix can use
it directly.

## Residual risk and open questions

- HTTP/1.1 only: resets use `http.Hijacker`, which HTTP/2 does not support. A
  stall-over-HTTP/2 test still needs its own `httptest` server (as
  `TestDownloadResumesAfterStallHTTP2` does).
- The fake serves bytes, not the HF API (`/api/models/...`); no test needs it
  because `Download` takes a direct URL.
- Fault agents that already wrote inline handlers do not have to switch; the
  fake is additive and has no non-test dependencies.
- No `t.Parallel` support for tests that change the package globals
  (`stallTimeout`, `retryStep`); same constraint as the existing tests.
