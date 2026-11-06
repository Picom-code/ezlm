# T09 / a11 — resume after the app restarts

**Fault:** a resume after the app restarts. The state is persisted, the resume is
correct, and a stale state file does not corrupt the download.

Owned paths: `internal/engine/download.go`, `internal/engine/download_resume_test.go`.
Tests: `internal/engine/download_resume_test.go` (all `httptest`, no network, temp dirs).

## What the code does today

`Download` (`internal/engine/download.go:47`) treats `dst+".part"` as its whole
persisted state: on a fresh call it takes the file size, sends
`Range: bytes=<size>-`, and appends the rest. Nothing on disk records *which
version* of the file those bytes belong to, and the 416 path
(`download.go:148`, old) assumed "range not satisfiable" always means "already
complete".

## Reproduction (before the fix)

`go test -run TestDownloadStale ./internal/engine/` on the baseline:

```
--- FAIL: TestDownloadStalePartLongerThanFile/size_unknown_(split_part)
    stale partial corrupted the file: 1152 bytes, want 576
--- FAIL: TestDownloadStalePartLongerThanFile/size_known
    the downloaded file came out the wrong size (1152 bytes instead of 576), so EZLM deleted it
--- FAIL: TestDownloadStaleStateRestartsWhenFileChanged
    resume appended to a stale partial: "old-revision-…-new-revision-…"
```

`TestDownloadResumeAfterRestart` and `TestDownloadCompletePartAfterRestart` passed
before and after: the plain restart case was already correct.

## Findings

1. **major — a stale `.part` longer than the server's file is silently kept.**
   `fetch` returned `nil` on 416 (`download.go:149`, old), so `Download` renamed
   the oversized partial to the model path. With `want == 0` — which is how every
   split GGUF part is downloaded (`engine.go:470`) — nothing checks the size, so a
   model file of the wrong length lands on disk. Evidence:
   `TestDownloadStalePartLongerThanFile/size_unknown_(split_part)` (1152 bytes
   written where the file is 576).
2. **major — the same stale partial with a known size loses the download.** The
   size check then failed and deleted the partial
   (`download.go:93-94`), so the next attempt starts from zero. No corruption,
   but the user pays for the whole multi-GB file again. Evidence:
   `TestDownloadStalePartLongerThanFile/size_known`.
3. **major — a partial from another revision of the same file is appended to.**
   If the app is closed, the model is re-uploaded upstream (same name, same
   length), and the user resumes, the new bytes are appended to the old ones.
   The result has exactly the right size, so the `want` check passes and the
   corrupt file is renamed into place. Evidence:
   `TestDownloadStaleStateRestartsWhenFileChanged` (before: half old revision +
   half new revision, correct length).

## Fix (minimal, all in `download.go`)

- The partial now has a sidecar, `dst+".part.state"`, holding the response's
  ETag, or `Last-Modified` when there is no ETag (`statePath`, `readState`,
  `writeState`, `validator`). It is written when a response starts writing the
  part and removed when the partial is renamed, deleted or restarted.
- A resume with a saved revision sends `If-Range: <revision>` with the `Range`
  header (`download.go:151-157`). A server whose file changed answers with the
  whole body (200), which the existing `StatusOK` branch already truncates.
  If the server ignores `If-Range` but returns 206 with a different validator,
  `fetch` returns `errStalePart`.
- 416 is no longer "done" by default: `fetch` reads `Content-Range: bytes */N`
  (`contentRangeTotal`). Complete only when `N == have`; otherwise the partial
  is stale.
- `Download` drops a partial longer than `want` before the first request, and on
  `errStalePart` deletes the partial and its state and retries once from zero
  (`download.go:60-84`). A second stale answer falls into the normal retry path,
  so a server that always says 416 cannot loop.
- `plainNetErr` explains a persistent revision mismatch in plain words.

An unreadable, empty or garbage state file is ignored (`readState` rejects
control characters and long values), so a crash while writing it can only cost a
restart, never a wrong file.

## Before / after

| Test | before | after |
|---|---|---|
| `TestDownloadResumeAfterRestart` | pass | pass |
| `TestDownloadStalePartLongerThanFile/size_unknown` | corrupt 1152-byte file | correct |
| `TestDownloadStalePartLongerThanFile/size_known` | error, partial deleted | correct |
| `TestDownloadStaleStateRestartsWhenFileChanged` | mixed old/new bytes | correct, one full request |
| `TestDownloadUnreadableStateFileIgnored` | pass | pass |
| `TestDownloadCompletePartAfterRestart` | pass | pass |

Checks run in the worktree: `gofmt -l .` (empty), `go build ./...`, `go vet ./...`,
`go test -p 2 ./internal/engine/`, `GOOS=windows GOARCH=amd64|arm64 go build ./...`,
`GOOS=linux go build ./...`.

## Residual risk

- A server that sends neither `ETag` nor `Last-Modified` cannot be verified
  across a restart; the size checks still catch a wrong length but not a
  same-length different revision.
- A partial corrupted in place (bit rot, not a new revision) needs a checksum;
  that is a07's fault, not this one.
- `If-Range` with a weak ETag (`W/"..."`) is valid per RFC 9110, but a server
  that mishandles it will just send the full body, which is correct but slower.
- Changing the file mid-transfer inside one `Download` call (a new ETag on a
  retry) is a03's fault. The persisted state here is what a03's in-memory
  validator could reuse.

## Open questions

- Should the state file also keep the expected total size, so a partial can be
  checked before any request is made? The 416/`Content-Range` path covers it
  today with one extra request.
- a03 and a11 both touch ETag handling in `download.go`. If a03 adds an
  in-memory validator, it should read/write the same sidecar instead of a second
  mechanism.
- `internal/app/app.go:409` counts `name` and `name+".part"` when showing how
  much is left. The new `.part.state` file is tiny and correctly ignored, but
  the app could show "resuming" from it in a later task.
