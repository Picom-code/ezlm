# T08 / a03 — truncated downloads

**Fault:** a download whose body is shorter than it claims. Three shapes: a
`Content-Length` larger than the body, a connection that closes early, and a body
that ends exactly on a chunk boundary. It must be detected and retried, or fail
cleanly. It must never be renamed into place and run.

**Reproduced:** yes (one shape installed the file). **Fixed:** yes, in
`internal/update/update.go`. Branch `agent/T08-a03`, commit `13eb0fa`.

## What the updater does today

`update.Download` (`internal/update/update.go:111`) delegates to
`engine.Download` (`internal/engine/download.go:44`), which resumes from
`<asset>.part` with a `Range` request and retries up to five times without
progress. The transport's own framing does the truncation detection:
`net/http` turns a short `Content-Length` body into `io.ErrUnexpectedEOF`, and a
chunked body that stops at a chunk boundary without the terminating chunk into
the same error. Both arrive in `fetch` as a non-`io.EOF` read error
(`internal/engine/download.go:188-193`), become `dropped`, and are retried.

That leaves one gap. A body that ends **with a clean terminating chunk** is a
complete HTTP message, so the transport reports `io.EOF` and `fetch` returns
nil. The only thing that can catch it is the size check
(`internal/engine/download.go:87-96`, only when `want > 0`) or the digest
(`internal/engine/engine.go:320-324`, which returns nil for an empty digest).

## Findings

### F1 — a short body that ends on a clean chunk boundary is installed (fixed)

- **Where:** `internal/update/update.go:111-125` before the fix, with
  `internal/engine/download.go:44-50` and `:87-96`; the digest guard at
  `internal/engine/engine.go:320-324`.
- **Reproduction:** `TestDownloadRejectsCleanShortBody` in
  `internal/update/truncate_test.go`. An `httptest` server answers with a
  well-formed chunked body: one 400-byte chunk, then `0\r\n\r\n`. The release
  lists `size: 0` and no digest, as a mirror or a stripped-down release
  metadata document can.
- **Before:** `Download` returned a path and the 400-byte file was renamed to
  `EZLM.dmg`. The test failed with `short body with no size or digest was
  accepted: ".../EZLM-update/EZLM.dmg"`. The assertion that the server saw
  exactly one request proves the body was a clean HTTP response, not a
  transport error.
- **After:** `Download` removes the file and returns
  `EZLM couldn't check that the update downloaded completely, because the
  release lists no size or checksum. ...`.
- **Severity:** minor. GitHub always publishes `size` (and now `digest`) for a
  release asset, so the normal path is already caught by the size check, which
  the same test asserts. The hole needs metadata with neither, and then a
  half-written installer is handed to the Mac `Updater` or the Windows setup
  exe.

### F2 — the size check is the only completeness check when the digest is absent (context)

- **Where:** `internal/engine/download.go:87-96`, `internal/engine/engine.go:320-324`.
- **Evidence:** with `want == 0` and `digest == ""`, `engine.Download` returns
  nil for any clean EOF, whatever the length. This is the mechanism behind F1.
  Not changed here: the engine belongs to other tasks, and the updater-side
  guard in F1 covers the updater.

### F3 — a stale `.part` (or complete file) from an older release is reused (found, not fixed)

- **Where:** `internal/update/update.go:116` names the file
  `filepath.Base(r.Asset)`; the asset name is constant across releases
  (`macos/scripts/make-dmg.sh:5-9`: `build/EZLM.dmg (the name releases use)`).
  `internal/engine/download.go:45-55` reuses a `.part` and an existing `dst` by
  size alone, with no release identity.
- **Reproduction (probe, not committed):** release A served to
  `update.Download`, then release B with a different body of the same size and
  no digest. The second call returned success and the file on disk was
  1020 of 1024 bytes from release A.
- **Why it matters here:** a cancelled download of release A leaves
  `EZLM.dmg.part`; the next release resumes it at that offset, and the finished
  file is the right size but half of the previous installer. That is a
  truncated-and-repaired download that the size check cannot see, and the digest
  only catches it when GitHub published one.
- **Severity:** major when the release has no digest, minor otherwise. Left
  alone because the fix touches the `.part`/`dst` lifecycle that a05 (replayed
  old asset) and a09 (disk full) also work in; it needs either a
  version-scoped part name or dropping the part when no digest is published.

## Already correct (asserted, no change)

- `Content-Length` larger than the body: `TestDownloadRetriesShortContentLength`.
  The first response promises 1024 bytes and delivers 512, the connection is
  cut; the updater retries with `Range: bytes=512-`, gets a 206, and the
  installed file matches the asset byte for byte.
- Connection closes early: `TestDownloadRetriesEarlyClose`. One chunk announces
  300 bytes and the connection dies after 200; the retry resumes and completes.
- Body ends exactly on a chunk boundary (terminating chunk missing):
  `TestDownloadRetriesChunkBoundaryClose`. A whole 256-byte chunk, then close;
  the retry resumes at 256.
- A server that never finishes the body gives up without installing anything:
  `TestDownloadNeverInstallsIncompleteBody`.
- A complete body is still accepted, with size+digest and with digest only:
  `TestDownloadAcceptsCompleteBody`.

## What changed

`internal/update/update.go:120-137`: after `engine.Download` returns, the
updater now (a) refuses a release that lists neither a size nor a `sha256:`
digest, because nothing can prove the body is whole, and (b) re-checks the
installed file's size against the release size and deletes it on a mismatch, so
the updater's own boundary enforces "never install a truncated file" even if the
engine's check regresses. Both paths remove the file and return plain text. The
digest check is unchanged and still runs last.

No changes outside `internal/update/**`.

## Verification

In the worktree, after the change:

```
gofmt -l .                        (empty)
go build ./...                    ok
go vet ./...                      ok
go test -p 2 ./internal/update/   ok, 10.5s
go test -p 2 ./...                ok
GOOS=windows GOARCH=amd64 go build ./...   ok
GOOS=windows GOARCH=arm64 go build ./...   ok
GOOS=linux go build ./...                  ok
```

Tests are hermetic: `TMPDIR`, `HOME` and `EZLM_HOME` point at `t.TempDir()`,
because `update.Download` writes under `os.TempDir()/EZLM-update`
(`internal/update/update.go:112`). No real network.

## Residual risk and next steps

- F3 above is the one that can still install a wrong-but-right-sized installer.
  Suggested fix: name the part file after the release (`EZLM-<version>.dmg.part`)
  or delete an unverifiable part when the release has no digest.
- `update.Download` writes into the shared `os.TempDir()/EZLM-update` with no
  lock, so two update downloads at once share one part file. Not exercised here.
- The retry pacing (`internal/engine/download.go:37`, one second per step) makes
  a full five-failure exhaustion take about 20 seconds; the tests bound it with a
  context deadline instead of waiting it out.
- Severity for F1 assumes GitHub keeps publishing `size`. If the project ever
  points `EZLM_UPDATE_URL` at a mirror that drops it, F1 becomes major.
