# T08 a04: digests

**Fault class:** release assets installed without a usable SHA-256 digest.
**Status:** reproduced, fixed, tests pass. Branch `agent/T08-a04`.

## What the updater trusts today

`Check` copies GitHub's `assets[].digest` into `Release.Digest`
(`internal/update/update.go:104`). `Download` hands it to
`engine.VerifyDigest` (`internal/update/update.go:120`), which returns `nil`
whenever the string does not start with `sha256:`:

```go
want, ok := strings.CutPrefix(digest, "sha256:")
if !ok {
    return nil // GitHub did not publish one; size was already checked
}
```

That "size was already checked" comment is only true when `Release.Size` is
non-zero; a release whose asset has `size: 0` (or an empty digest) is
installed with no integrity check at all.

## Findings

1. **Missing digest installs silently. blocker.**
   `internal/engine/engine.go:320-324` returns `nil` for `""`, and
   `internal/update/update.go:120` treats that as success. A release whose
   metadata has no `digest` (or a proxy that strips it) installs whatever
   bytes arrived. Evidence: `TestDownloadRefusesMissingDigest` failed before
   the fix with "Download installed an asset with digest \"\"".
2. **Wrong algorithm installs silently. blocker.**
   `sha512:<hex>`, `md5:<hex>`, `sha1:<hex>` and a bare `sha256` all miss
   the `sha256:` prefix and are skipped. Evidence:
   `TestDownloadRefusesWrongAlgorithm` failed before the fix.
3. **Wrong-length digest is refused, but by accident. minor.**
   `sha256:abc` is compared as a string against the 64-hex sum, so it always
   mismatches. It is safe but produces the generic checksum-mismatch error
   with no hint that the metadata is malformed. Now rejected up front with a
   specific message (`TestDownloadRefusesWrongLengthDigest`).
4. **Digest mismatch is refused, but the message is not actionable. minor.**
   `engine.VerifyDigest` says "checksum mismatch for EZLM.dmg"; the app
   shows that verbatim in the "update" job. Now the updater wraps it with a
   sentence naming the next action.

## Fix

`internal/update/update.go`:

- new `digestSHA256` validates the digest before any bytes are fetched:
  algorithm must be `sha256` (case-insensitive), exactly 64 hex characters.
  A missing digest, another algorithm or the wrong length is an error.
- `Download` calls it first, then still runs `engine.VerifyDigest` on the
  normalized `sha256:<lower-hex>` after the download, and deletes the file
  on a mismatch (unchanged behaviour).
- mismatch errors now read: "the downloaded file does not match its
  published checksum, so EZLM deleted it; download the update again".

No change to `internal/engine` (not owned by T08).

## Tests

`internal/update/digest_test.go`, `httptest` asset server, `TMPDIR` set to
`t.TempDir()` so nothing touches the real temp or home directory:

- matching digest installs and the bytes are right (control);
- mismatch refuses and reports failure;
- empty and whitespace digests refuse;
- `sha512:`, `md5:`, `sha1:` and a colon-less digest refuse;
- empty, short, long and non-hex digests refuse;
- a refused download leaves no file at the final path.

Before: the missing-digest and wrong-algorithm subtests failed (see above).
After: `go test -p 2 ./internal/update/` passes; `gofmt -l .` empty;
`go build ./...` and `go vet ./...` clean.

## Residual risk and open questions

- **Old releases with no digest become uninstallable.** GitHub only started
  populating `assets[].digest` for new releases. If an older release without
  a digest is ever the newest, the app will offer it and then fail the
  download with "this release has no SHA-256 digest". If that matters,
  backfill digests on the release, or let `Check` skip releases whose asset
  has no usable digest. Not done here to keep the change inside `Download`
  and avoid changing which release `Check` picks (a13's flow test asserts
  that).
- `Check` still reports `Available` for a release whose digest is bad; the
  failure surfaces at download time. Same trade-off as above.
- `engine.VerifyDigest` is still lenient for its other callers (model
  downloads, T09). This fix only covers the updater path.
- Digest comparison is a plain string compare; not a timing concern for a
  public checksum.
- Release `Size` of 0 disables the size check inside `engine.Download`, but
  the digest check now always runs, so that is no longer a hole here.
