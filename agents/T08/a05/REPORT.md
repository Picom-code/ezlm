# T08 a05: downgrades

Fault class: the updater can offer or install a release that is older than the
running version, or a new release whose asset is an old build (a replay).

## Findings

1. **Major: an older release is returned as `Info.Latest`.**
   `internal/update/update.go:93-104` (before the fix) picked the highest
   release in the list without comparing it to `current`. The result carried
   `Latest: 0.1.10-beta` while `Available` was false. Every caller that
   downloads `Latest` instead of gating on `Available` (the Windows and macOS
   panes do gate today, see `windows/EZLM/Panes.cs:125`,
   `macos/Sources/EZLM/Models.swift:348`) would downgrade the install. The
   package contract said "Latest is the newer version" and that was false.

2. **Major: a replayed old asset is offered and installed as a new version.**
   If the newest release's `browser_download_url` or `digest` is an older
   release's asset, the old code set `Available: true` and downloaded it. The
   digest check (`internal/engine/engine.go:318`) passes because the metadata
   digest is the old asset's digest, so an old build installs while the app
   reports the new version. That is a silent downgrade below the running
   version.

3. **Minor / already correct: `1.10` vs `1.9`.**
   `Newer` parses each numeric component with `strconv.Atoi`
   (`internal/update/update.go:147-158`), so `1.10` sorts above `1.9` even
   though it sorts below as text. No bug; the tests are a regression guard.
   Textual ordering of pre-release suffixes (`beta.10` vs `beta.9`) is still
   compared as text; that is a separate, minor issue (see open questions).

## Reproduction

`internal/update/downgrade_test.go` stands up an `httptest` server that serves
a GitHub release list through `EZLM_UPDATE_URL`, one test per case. Before the
fix:

```
--- FAIL: TestDowngradeOlderReleaseNotOffered
    Check returned older release 0.1.10-beta as Latest; a caller that downloads Latest would downgrade
--- FAIL: TestDowngradeNumericVersionOrder
    Check(1.10.0) offered the older 1.9.0
--- FAIL: TestDowngradeReplayedOldAsset/same_asset_URL
    Check offered a replayed old asset as 0.1.12-beta
--- FAIL: TestDowngradeReplayedOldAsset/same_asset_digest
    Check offered a replayed old asset as 0.1.12-beta
```

After the fix all four pass, plus `TestDowngradeDistinctAssetOffered`, which
proves the replay check does not block a normal update.

## Fix

`internal/update/update.go` only:

- A release is a candidate only when `Newer(v, current)` is true, so `Latest`
  is always newer than the running version. `Available` is then simply
  `Latest != nil`.
- After a candidate is chosen, the whole release list is checked for another
  release (different version, same platform asset) with the same non-empty
  `browser_download_url` or the same non-empty digest. If one exists the
  candidate is dropped and `Error` says so, so the refusal is visible in the
  UI and the app retries the check instead of caching it (the app re-checks
  whenever `Error` is set, `internal/app/settings.go:37`).

No signature changes, so the Windows and macOS call sites are untouched.

## Residual risk

- A replay where the old release has fallen out of the 10-item window
  (`ReleasesURL` asks for `per_page=10`) and the metadata digest was replayed
  with it cannot be detected from the list alone. The digest check still
  catches a new digest over old bytes.
- A release that reuses the previous version's asset legitimately (no code
  change, republished) is refused too. The user sees the error and can
  download from the releases page.
- `Download` still trusts its caller: it has no running version to compare
  against. Every shipped caller goes through `Check` first.

## Open questions

- Should pre-release suffixes compare numerically (`beta.10` vs `beta.9`) or
  is plain text ordering intended? `Newer` compares them as text today.
- Should a refused replay fall back to the next-newest release instead of
  reporting no update? Reporting no update is the safer choice for tonight.

## Checks

`gofmt -l internal/update` (empty), `go build ./...`, `go vet ./internal/update/`,
`go test -p 2 ./internal/update/` all pass.
