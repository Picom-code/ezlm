# T08 a07: malformed version tags

**Fault class:** a release tag that is not a version (`v`, `v1`, `1.2.3.4`, unicode,
a huge string, an empty tag) must not panic the updater and must not be installed.

**Status:** reproduced yes, fixed yes. Severity: major (a malformed tag can install a
release, and an overflowed tag sorts above every real release).

## What the updater did before

`internal/update/update.go` (baseline) read a tag with `split`:

```go
v = strings.TrimPrefix(strings.TrimSpace(v), "v")
if i := strings.IndexAny(v, "-+"); i >= 0 { v, pre = v[:i], v[i+1:] }
for i, p := range strings.SplitN(v, ".", 3) { n[i], _ = strconv.Atoi(p) }
```

Missing parts and parse errors were silently treated as `0`, so any string became
a version, and `Check` accepted it.

## Findings

1. **major, `update.go:94-105` (baseline 93-104): a tag that is not a version is
   installed.** `v1` parses as `1.0.0`, `v9` as `9.0.0` and `1.2.3.4` as `1.2.0`
   (`SplitN` drops the fourth part), all newer than the running `0.1.11-beta`, so
   `Check` returned `Available: true` with that version. The app then offers and
   downloads that installer.
2. **major, `update.go:154-155` (baseline): a huge numeric tag overflows into the
   newest release.** `strconv.Atoi` returns the clamped `MaxInt` *together with* a
   range error; the error was discarded, so `9999...9` (3000 digits) parsed as
   `9223372036854775807.0.0` and beat every real release. Reproduced:
   `tag "999...9" was offered as an update`.
3. **minor, `update.go:93-104` (baseline): a malformed tag still becomes
   `Info.Latest`.** With tag `""` or `"v"`, `Latest.Version` was `""` even though
   `Available` was false, so the UI cache holds a release with a blank version.
4. **minor, `update.go:150` (baseline): build metadata changes the order.**
   `v1.2.3-beta+build` compared as pre-release `beta+build` and sorted above
   `1.2.3-beta`, and `1.2.3+build` sorted above `1.2.3`; semver ignores build
   metadata when ordering, so a re-published build looked like a new release.
5. **minor, `update.go:154` (baseline): unicode tags are accepted.**
   `v1.2.3-β` parsed as `1.2.3` with pre-release `β` and was offered as an update.
6. **no panic found.** `SplitN(..., 3)` bounds the index to 2 and `Atoi` errors are
   ignored, so none of the tags above panicked on the baseline code. The tests keep
   that property under guard (recover in the table test and the huge-tag check).

## Fix

`internal/update/update.go`, `internal/update/tag_test.go`:

- New `parseVersion` (`update.go:177`) accepts only `[v]major.minor.patch`,
  optional `-pre` and `+build`, digits only, each number at most 9 digits and the
  whole tag at most 128 bytes. Build metadata is validated and dropped.
- `version` (`update.go:141`) holds the parsed numbers and pre-release, with
  `String` (`update.go:148`) producing the canonical form and `newerThan`
  (`update.go:156`) keeping the existing ordering rules (numbers, then release
  beats its own pre-release, then pre-release text).
- `Newer` (`update.go:135`) returns false if either side does not parse, so an
  unreadable tag can never be "newer" in either direction.
- `Check` (`update.go:94`) skips a release whose tag does not parse before it is
  considered or stored, and stores `v.String()`, so `Latest.Version` is always a
  clean version without `v` or build metadata.
- `versionPart` (`update.go:220`) validates pre-release and build identifiers.

Policy chosen: tags must be exactly three numeric parts. A two-part tag (`0.2`) is
now skipped rather than guessed at. `V1.2.3` (capital V) is rejected; EZLM's own
tags use a lowercase `v`.

## Evidence

`internal/update/tag_test.go` (new, 8 tests). Before the fix, with the
`parseVersion`-level assertions removed so the file built against the old code:

```
--- FAIL: TestMalformedTagRejected (0.00s)
    ... 25 malformed tags compared as versions
--- FAIL: TestBuildMetadataDoesNotAffectOrder (0.00s)
    Newer ordered "1.2.3-beta+build" and "1.2.3-beta" as different versions
--- FAIL: TestCheckRejectsMalformedTags (0.00s)
    tag "v1" was offered as an update (version "1")
    tag "1.2.3.4" was offered as an update (version "1.2.3.4")
    tag "v9" was offered as an update (version "9")
    tag "v1.2.3-β" was offered as an update (version "1.2.3-β")
    tag "999...9" was offered as an update (version "9223372036854775807.0.0")
    tag "" was kept as the latest release (version "")
--- FAIL: TestCheckMalformedTagDoesNotBeatValidRelease (0.00s)
    latest = "9", want the valid release 0.2.0
--- FAIL: TestCheckHugeTagDoesNotPanic (0.00s)
    a huge tag was offered as an update
```

After the fix: `go test -p 2 ./internal/update/` passes, including the existing
`TestNewer` and `TestAssetFor`. `gofmt -l .` empty, `go build ./...`,
`go vet ./...`, `GOOS=windows GOARCH=amd64`, `GOOS=windows GOARCH=arm64` and
`GOOS=linux` builds all pass.

The tests use `httptest` for the release list, `t.Setenv("EZLM_UPDATE_URL", ...)`,
no network and no files. They skip on platforms with no installer asset (linux),
because `assetFor` only matches darwin and windows.

## Interpretation and open questions

- The brief lists `v1.2.3-beta+build` under malformed tags. It is valid semver, so
  the fix accepts it and canonicalizes it to `1.2.3-beta`, but build metadata no
  longer changes the order (`Newer("1.2.3-beta+build", "1.2.3-beta")` is false).
  If the Lead wants all build-metadata tags rejected, that is a one-line change in
  `parseVersion`; say so and I will flip it.
- `Newer` is now false in both directions when either side is malformed, so it is
  not a total order over malformed input. a12's fuzz test should assert ordering
  only for tags that `parseVersion` accepts, or use `parseVersion` directly.
- Prerelease ordering is still plain string comparison (`beta.10` < `beta.9`), as
  before. That is a05/a12 territory; I did not change it.
- `Check` still accepts a release with no digest or a mismatched digest; that is
  a04's slice, untouched here.

## Residual risk

- A real release tagged with two parts (`0.2`) or a capital `V` would now be
  skipped, so the updater would report "up to date" until a well-formed tag
  ships. This is a safe failure (no install), and EZLM tags three parts.
- Version numbers above 999,999,999 are rejected. Not a realistic EZLM version.
- `Download` does not re-check the version; it trusts `Check`. Unchanged.
