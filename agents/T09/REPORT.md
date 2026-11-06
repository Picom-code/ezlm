# T09. Model download robustness — consolidated report

16 agents (a01–a16) worked this task in parallel: eleven fault classes, the shared fake Hugging
Face server, the progress path, the watchdog review, a 20 GB resource measurement, and the severity
table. Consolidated onto `agent/T09-downloads` by the Lead.

Goal: multi-GB model downloads succeed on bad networks and fail cleanly when they can't.

## Result

- **9 real defects fixed**, 2 faults proved already handled and pinned by tests.
- **Gate:** `gofmt -l .` empty, `go build ./...`, `go vet ./...`, `go test -p 2 ./...` pass, plus
  `GOOS=windows` amd64/arm64 and `GOOS=linux` builds.

## Fixed

| Fault | Before | After |
|---|---|---|
| **ETag change on resume** (a03, a11) | A file replaced under a resume was appended to, producing a right-sized but corrupt model; `Download` returned success. | The validator (ETag/Last-Modified) is persisted in `dst.part.state`, sent as `If-Range`, and a changed file restarts the download (bounded). A 416 is parsed via `Content-Range: bytes */N` and only counts as complete when it matches. |
| **206 with the whole body from byte 0** (a02) | Appended to the part file: silent corruption when the size was unknown, wasted progress when it was known. | `Content-Range` start is checked; a start of 0 restarts, any other mismatch is a clear permanent error. |
| **Oversized/stale part on 416** (a06, a11) | A 416 with `want == 0` (how every split GGUF part downloads) renamed the oversized part into place. | `rangeTotal`/`errRestart`: truncate and refetch, bounded. |
| **Concurrent downloads of one file** (a10) | Two callers transferred the file twice; the loser failed with a rename error and could delete the winner's part. | Per-destination in-process lock (refcounted, ctx-aware); the second caller returns success after the first finishes. |
| **Checksums for model files** (a07) | `Download` trusted any same-size file on disk; model and draft downloads had no digest at all. | New `DownloadSHA` (bare hex or `sha256:`), corrupt file re-fetched once then deleted with a plain message; wrong-length or non-hex rejected before any network. |
| **Disk full** (a08) | ENOSPC retried 5× for ~20 s, part left behind, raw Go error. | `diskErr` is terminal: part removed, plain "not enough free space" message, no retries. |
| **Missing destination folder** (a09) | The first download into a new model folder died with a raw `no such file or directory` (broken for any fresh model directory, non-ASCII names, 260+ char paths). | `MkdirAll` the destination folder; a local filesystem error is permanent, not retried (20.0 s → 0.00 s). |
| **Stall watchdog false positive** (a14) | Header wait and time-to-first-byte shared one 45 s budget, so a slow link was cut as a stall. | The clock resets when headers arrive. |
| **Progress on failure** (a13) | A stall emitted no progress between the last byte and the resume, so a stall looked like a hang for 45 s and up to ~4 min before any error text. | One `prog(have, want, 0)` tick at every retry boundary; `bps` guarded against +Inf. |
| **Reset after the last byte** (a05) | If the complete body reached the part file but the connection reset, the finished download was retried and eventually reported failed. | If the part already holds `want` bytes, finish. |

Proved already correct and pinned by tests: **HF 302 → CDN redirects, including a changed signature
on resume** (a01: Range is forwarded by Go's redirect copier and the target is never cached), and
**servers that ignore Range with a 200** (a02: the body is truncated and restarted).

## Measurements (a15)

A 20 GB download through an in-process httptest server at 756–1045 MB/s (19–27 s) used 26 % of one
core, +1.7–1.9 MiB peak RSS, 1.4 MiB heap before and after, and 102 monotonic progress callbacks.
The test is `-short`-guarded and needs ~20 GB of free disk.

## Found, not fixed

1. **Split GGUF parts are size-checked only when a size is known**, and `DownloadModel` passes
   `want = 0` for multi-part models; the catalog has totals, not per-part sizes (a06). A truncated
   final part can still slip through.
2. **`DownloadSHA` is not yet used at the call sites** (`engine.go:242`, `update.go:117`) — a two-line
   change that closes the download-then-verify gap (a07).
3. **A 206 with a correct `Content-Range` but a full body** is undetectable without a size or
   checksum (a02).
4. **`ResponseHeaderTimeout` (60 s) is unreachable** behind the 45 s watchdog (a04, a14).
5. **HTTP/2 RST_STREAM** is not classified by `plainNetErr`, so it surfaces as a raw Go error (a05).
6. **Cross-process locking** is not covered — two EZLM instances can still race (a10).

## Merge notes

Twelve branches changed `internal/engine/download.go`. The Lead merged a14 → a13 → a12 → a10 → a09 →
a08 → a07 → a06 → a05 → a03 → a02 → a11, then repaired the result by hand: restored the
`etag`/`restarts` declarations, the `openPart` seam (`partfile.go`), `DownloadSHA` and its helpers
(`download_sha.go`), the `diskErr` wrapping of write/open errors, and the "part already complete"
early finish. Two tests were already written by agents to prove their fix bites.

## Files

- `internal/engine/download.go` — the hardened downloader, plus `partfile.go`, `download_sha.go`.
- `internal/engine/download_*_test.go` — one file per fault, `fakehuggingface_test.go`, and the
  20 GB benchmark.
- `agents/T09/a16/findings.md` and one report per agent.

## Next steps

1. Adopt `DownloadSHA` at the two call sites, and add per-part sizes to the catalog.
2. Decide the `.part` retention policy on disk-full (currently discarded, losing resume bytes).
3. Re-check the 416/`Content-Range` handling against a real HF CDN.
