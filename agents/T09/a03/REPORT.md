# T09 / a03: an ETag change mid-download

**Fault:** the server swaps the file between the first response and a resume. The
resume must notice and start over instead of splicing the old head onto the new
tail.

**Verdict:** reproduced, fixed. Severity **blocker** when it happens (a corrupt
model that passes the size check and is renamed into place), but it needs a file
to change on the server mid-download, so it is **major** in frequency.

## Reproduction (before the fix)

`internal/engine/download_etag_test.go`, `TestDownloadRestartsWhenETagChanges`.
A fake server serves 512 bytes with `ETag: "v1"`, cuts the connection halfway,
then answers the resume from a *different* 512-byte file with `ETag: "v2"`.
Both subtests fail on the baseline:

```
--- FAIL: TestDownloadRestartsWhenETagChanges/server_honours_If-Range
    file mixes the old and the new copy: got "old-model-bytes-...NEW-MODEL-BYTES-..."
--- FAIL: TestDownloadRestartsWhenETagChanges/server_ignores_If-Range
    file mixes the old and the new copy: got "old-model-bytes-...NEW-MODEL-BYTES-..."
```

The result is 512 bytes, the expected size, so `Download` returns nil and renames
a file that is half of one model and half of another. On a real GGUF that is a
model that loads and produces garbage, or fails a checksum much later. The size
check in `Download` cannot catch it because the two copies are the same length.

Cause: `fetch` never looked at `ETag` and never sent `If-Range`, so a `206` from
the new file was appended to the old file's bytes (`download.go:143-148` on the
baseline).

## Fix (`internal/engine/download.go`, minimal)

1. `Download` keeps the `ETag` seen for the `.part` bytes and passes it into
   `fetch`; `fetch` now returns the tag it saw.
2. On a resume, `fetch` sends `If-Range: <etag>` (strong tags only). A server
   that supports it answers `200` with the whole new file, which the existing
   `200` path already handles by truncating and restarting.
3. For servers that ignore `If-Range`, a `206` whose `ETag` differs from the
   stored one returns `errChanged`. `Download` deletes the `.part`, clears the
   tag, and fetches from zero. This does not count against the five-failure
   retry budget, but it is capped at three restarts so a server that changes the
   file on every request cannot spin forever.
4. `sameTag` compares tags ignoring the weak marker (`W/"x"` == `"x"`).

Also fixed a race the new test exposed: the stall watchdog goroutine read the
package-level `stallTimeout` after `fetch` returned, so a test that restores the
knob raced with a watchdog that had not started yet. `fetch` now reads it once
into a local. `go test -race` on the download tests is clean.

## After the fix

```
--- PASS: TestDownloadRestartsWhenETagChanges/server_honours_If-Range
--- PASS: TestDownloadRestartsWhenETagChanges/server_ignores_If-Range
--- PASS: TestDownloadResumesWhenETagMatches
```

`TestDownloadResumesWhenETagMatches` guards against over-correction: with an
unchanged tag the resume must not refetch a byte (the test counts bytes served
and requires exactly the file size).

## Verification

- `gofmt -l .` empty, `go build ./...`, `go vet ./...`
- `go test -p 2 ./internal/engine/` passes
- `GOOS=windows GOARCH=amd64/arm64 go build ./...`, `GOOS=linux go build ./...`
- `go test -race -run 'ETag|Flaky|ResumesAfterStall' ./internal/engine/` passes

Commit: failing test first (`T09: failing test for an ETag change mid-download`),
then the fix (`T09: restart download when the ETag changes mid-resume`).

## Residual risk

- **No tag, no detection.** Servers that send no `ETag` (or only a weak one,
  where `If-Range` is not sent) still splice on a resume; the response tag is
  still compared when present. `Last-Modified` could be a second validator if
  this matters.
- **Tags reused.** A server that republishes different bytes under the same tag
  defeats this; nothing short of a checksum catches that (a07).
- **Across process restarts the tag is lost.** The `.part` from an earlier run
  has no remembered tag, so the first resume after a restart is blind; the tag
  is learned again from that response. Persisting it next to the `.part` is a
  small change that belongs with a11 (persisted resume state), and would need
  cleanup on success and on delete.
- **Restart cost.** A changed file restarts a multi-GB download from zero. That
  is the only correct outcome, but the progress callback jumps backwards; a13
  may want to surface it.
- `416` with a changed tag still counts as complete; it is only reached when the
  server believes the range is past the end, which already implies a size
  mismatch that the final size check catches when `want > 0`.

## Open questions

- Should `Download` treat a restart as progress for the retry budget? Currently
  it is neutral, so a flapping file plus a flaky connection can reach the
  restart cap first. The cap's message names the real cause, so this seems fine.
- Is the three-restart cap the right number for a nightly re-upload? One is the
  common case; three allows a change during the restart itself.
