# T09/a06: multi-part split GGUF downloads

## Fault

Split GGUFs (for example `...-00001-of-00003.gguf`) are downloaded one part at a
time by `engine.DownloadModel`. The slice covers four situations: every part
present, a part missing, a part whose bytes on disk are the wrong size, and parts
that arrived out of order.

## Reproduction

`internal/engine/download_split_test.go` uses a small `httptest` server that
serves three parts with `http.ServeContent` (so it honours `Range`, sends
`206`/`416` and `Content-Range` the way a CDN does). Before the fix:

```
--- FAIL: TestDownloadSplitPartWrongSize (0.00s)
    --- FAIL: TestDownloadSplitPartWrongSize/want=false (0.00s)
        part kept the stale bytes: 450 bytes, want 400
    --- FAIL: TestDownloadSplitPartWrongSize/want=true (0.00s)
        stale oversized part: the downloaded file came out the wrong size
        (450 bytes instead of 400), so EZLM deleted it. Try again, ...
```

The other three cases already behaved correctly and are now regression tests:
all parts present makes no request, a missing part is the only one fetched,
parts fetched in the order 3, 1, 2 end up correct, and a part cut short
resumes from where it stopped.

### What goes wrong

A `.part` file left by an older, larger revision of a part is bigger than the
part the server has now. `Download` resumes from that offset; the server answers
`416 Requested Range Not Satisfiable`, which `fetch` treated as "already
complete" (`download.go:148` before the fix). The stale bytes were then renamed
into place. With a known size (`want > 0`) the file was instead deleted and an
error returned, so a 60 GB model failed its first retry for no reason.

## Fix (internal/engine/download.go only)

- `rangeTotal` reads the whole-file size out of `Content-Range`
  (`bytes */400` or `bytes 0-99/400`).
- On `416`, when the part on disk is larger than the server's file, `fetch`
  returns `errRestart`; `Download` truncates the part and fetches it from the
  start. Restarts are capped at two per `Download` call so a server that keeps
  changing its mind can't loop forever.
- With `want > 0`, an oversized completed part now gets one clean restart
  instead of an immediate "wrong size, deleted it" error. A short or otherwise
  mismatched part still produces the existing plain error and removes the file.

Before/after for the failing case: before, `Download(..., want=0)` kept 450
bytes; after, it re-fetches and leaves exactly 400. With `want=400` before, it
returned the wrong-size error; after, it succeeds.

## Findings outside download.go (read only)

- **major**: `internal/engine/engine.go:466` — "Per-file sizes aren't in the
  catalog, so only the single-file case is size-checked". `DownloadModel` passes
  `want = 0` for every part of a split quant, so a truncated part (shorter than
  the server's file, not longer) is never checked after it is renamed into
  place. `Download`'s early return on an existing `dst` (`download.go:45`)
  accepts any size when `want == 0`.
- **major**: `internal/app/app.go:1118` `complete()` returns true for a split
  quant as soon as every file in `SplitFiles(first)` exists
  (`len(q.Files) > 1 || ...`), regardless of size. A truncated or zero-byte part
  therefore looks installed and `DownloadModel` is never called again, so the
  engine loads a corrupt model.
- Fixing either needs per-part sizes. The catalog (`catalog.Quant`) only has
  the total `bytes`; the per-file sizes would have to come from the Hugging
  Face tree API and be stored per file, or `Download` would have to probe the
  server (HEAD/`Range: bytes=0-0`) for each part it finds on disk. Both changes
  live in `engine.go`/`app.go`, which this slice does not own (T06 owns
  `engine.go`; `internal/app` is read-only for T09).

## Residual risk

- The 416 fix relies on the server sending `Content-Range` with the total. If it
  doesn't, a resume from past the end is still accepted as complete; that is the
  old behaviour and only matters for an oversized stale part.
- Restarting a stale part throws away its bytes. For a multi-GB part that means
  a full re-download, which is correct but slow. A cheaper option would be to
  resume only if the server's total is larger than the bytes on disk, which the
  same `Content-Range` already gives us; not needed for correctness.
- A part that is too short at the final name is not detected with `want == 0`
  (see the findings above). The tests cover the `.part` resume case, not a
  truncated final part.

## Open questions

- Should split parts be downloaded in parallel? They are independent files, so
  a 60 GB three-part model could use three connections. Not attempted here.
- Is the second restart in `Download` worth it, or should a size mismatch after
  a restart be permanent at once? Kept two restarts to tolerate one flaky
  server response.

## Files

- `internal/engine/download_split_test.go` (new)
- `internal/engine/download.go` (fix)
- `agents/T09/a06/REPORT.md`
