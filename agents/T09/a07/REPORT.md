# T09 / a07: checksum failures

**Fault:** wrong SHA, missing SHA, a SHA of the wrong length.

**Reproduced:** yes. **Fixed:** yes (in `internal/engine/download.go`).

## What the code did before

`Download` (`internal/engine/download.go:44`) has no checksum at all. Two
consequences, both reproduced:

1. **A same-size file on disk is trusted.** The early return at
   `download.go:45` returns success for any file whose size matches `want`,
   without looking at its bytes. A corrupt file left by an earlier run (or a
   truncated/mangled transfer that happened to match the expected size) is
   never fetched again. Pre-fix evidence, run against the unmodified code:

   ```
   --- PASS: TestBeforeChecksumTrustsSameSizeFile
   ```
   (temp test: write 64 corrupt bytes to `dst`, serve 64 good bytes, call
   `Download(..., want=64)`: returns nil and leaves the corrupt bytes. No HTTP
   request is made. The temp test was removed after recording this.)

2. **There is no SHA to check.** Callers verify afterwards and delete on
   mismatch (`internal/engine/engine.go:250`, `internal/update/update.go:120`),
   but `DownloadModel` (`engine.go:475`) and `DownloadDraft` (`engine.go:493`)
   have no digest to verify: `catalog.Quant` and `catalog.Draft` carry no SHA
   (`internal/catalog/catalog.go:25`, `:43`). Model files are size-checked
   only.

The verification that does exist, `verifyDigest` (`engine.go:320`), skips any
digest without the `sha256:` prefix (`:321-324`) and reports a wrong-length
digest as `checksum mismatch for <file>` (`:334-335`), which blames the file
for what is a metadata problem. Neither is reachable from `download.go`.

## Fix

Added to `internal/engine/download.go` (additive; no existing signature or
caller changed):

- `DownloadSHA(ctx, url, dst, want, sha, prog)` — `Download`, then verify
  SHA-256. Empty `sha` means "the server published none" and keeps today's
  size-only behaviour. A corrupt file already on disk is deleted and fetched
  once more rather than trusted; a file that is still wrong after the retry is
  deleted and reported in plain words.
- `parseSHA` — validates before any bytes move. Accepts bare hex and
  `sha256:<hex>` (case-insensitive). A digest of the wrong length or with
  non-hex characters is a metadata error and fails without touching the
  network, so a 20 GB model is not downloaded only to fail at the end.
- `fileMatchesSHA` — streaming SHA-256 of the file, no size limit.

Messages are plain and name the next action, matching `download.go`'s existing
errors, for example: "the downloaded file is corrupt (its checksum doesn't
match), so EZLM deleted it. Try again, and if it happens again, report this
problem".

## Tests (`internal/engine/download_checksum_test.go`)

All fail before the fix (the function is undefined, `undefined: DownloadSHA`)
and pass after:

| Test | Covers |
|---|---|
| `TestDownloadSHARejectsCorruptFile` | right size, wrong bytes: error names the checksum, `dst` and `.part` are gone, at most one retry (2 requests), no loop |
| `TestDownloadSHARedownloadsCorruptFileOnDisk` | corrupt file already at `dst` with the right size is replaced in one call (1 request) |
| `TestDownloadSHAMissingAndPrefixed` | missing SHA still downloads; size check still applies with no SHA; `sha256:`-prefixed digest accepted |
| `TestDownloadSHAMalformed` | short, SHA-1-length, 65-char and non-hex digests: clear checksum error, zero network requests, no file written |

Run: `go test -p 2 ./internal/engine/ -run TestDownloadSHA -count=1` — all pass.

## Verification

- `gofmt -l .` — empty
- `go build ./...`, `go vet ./...` — clean
- `go test -p 2 ./internal/engine/` — ok
- `GOOS=windows GOARCH=amd64|arm64 go build ./...`, `GOOS=linux go build ./...` — clean

## Findings outside my slice (for the parent / T06)

- **major** `engine.go:475`, `engine.go:493`: model and draft downloads are
  size-checked only, because the catalog has no digest
  (`catalog.go:25`, `:43`). A same-size corrupt GGUF is accepted. Adopting
  `DownloadSHA` here needs a digest in the catalog (or the Hugging Face
  `ETag`/`x-linked-etag`, which is the SHA-256 for LFS files). This is the
  one place the a07 fault still reaches users.
- **minor** `engine.go:321-324`: `verifyDigest` returns nil for a digest
  without the `sha256:` prefix. A bare 64-hex digest (what a caller would
  naturally pass) is silently unchecked.
- **minor** `engine.go:334-335`: a wrong-length digest reports "checksum
  mismatch for <file>", blaming the download for bad metadata.
- `DownloadSHA` has no caller yet; `engine.go` belongs to T06. Swapping
  `Download` for `DownloadSHA` at `engine.go:242` (asset digests are in
  `pinned.go`) and at `update.go:117` (then dropping the separate
  `VerifyDigest` call) is a two-line adoption that also removes the
  download-then-verify gap.

## Residual risk

- The corrupt-on-disk path re-downloads once. On a server that keeps sending
  bad bytes this costs one extra full download before failing; acceptable for
  a file that would otherwise be silently wrong.
- Hashing a 20 GB file adds one full read at the end. That is cheap next to
  the download, but it is a second pass, not streaming; a future change could
  hash while writing.
- `parseSHA` treats `sha256:` with nothing after it as "missing", not as a
  malformed digest. Deliberate, so an empty field from a release API does not
  fail a download.

## Open questions

- Should a checksum mismatch retry more than once? One retry catches a mangled
  transfer; more would hide a bad CDN edge at the cost of bandwidth.
- Is the catalog's lack of model digests a T06 (release metadata) decision or a
  T09 follow-up? `DownloadSHA` is ready for it either way.
