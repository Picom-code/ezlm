# T08 / a13: the check flow end to end

Slice: `internal/update/checkflow_test.go`. One integration test file: a fake GitHub
releases API plus the release assets, asserting which release and which file `Check`
picks for this platform, and carrying that decision into `Download`.

## What the file does

`checkflowGitHub` serves `GET /releases` (a JSON list shaped like GitHub's, with
`tag_name`, `draft`, `prerelease`, `html_url`, `body` and `assets[]`) and
`GET /assets/<name>`. It fills each asset's `browser_download_url`, `size` and
`digest` from the payload, unless the test set them first, so a test can publish a
lie (a wrong digest) while the bytes on the wire stay honest. `hits` counts API
requests, so "does not spin" is assertable.

Tests:

| Test | What it pins |
|---|---|
| `TestCheckFlowPicksThePlatformAsset` | A newer beta with `.dmg`, `.exe`, a notes file and a checksum file. Asserts `Available`, `Version`, `Name`, `Notes`, `Page`, and that `Asset`/`URL`/`Size`/`Digest` are the platform's installer, not the other platform's and not the sidecar. Asserts exactly one API request. |
| `TestCheckFlowChannelDecisions` | 12 cases: newer stable, newer beta, stable outranks its own beta, beta of the running stable, draft skipped, draft only, up to date, older release, newest release with no file for this platform (falls back to the older one), no installer at all, list out of order, and `prerelease: true` with a stable tag. |
| `TestCheckFlowDownloadInstallsTheCheckedFile` | Check then `Download` with `TMPDIR` in `t.TempDir()`: the file lands at `<tmp>/EZLM-update/<asset>` and its bytes are the asset the check picked. This is the join between the decision and the digest. |
| `TestCheckFlowRefusesAForeignAsset` | The release publishes a digest that does not match its own bytes: the check still offers it, `Download` refuses it with a checksum error and removes the file, so nothing wrong is left to run. |
| `TestCheckFlowReportsAFailedCheck` | 500, 403 with `X-RateLimit-Remaining: 0`, a JSON object instead of a list, and an empty list. Error set (or not), no update offered, exactly one request. |

The fixture is what the fault agents can plug into: build a `checkflowRelease` list,
set an asset's `Digest`/`Size`/`URL` to the lie you need, and assert the decision.
Names are prefixed `checkflow` so they cannot collide with another agent's helpers
when the branches merge. a15's shared fake can replace this one; it is deliberately
self-contained and small.

Tests set `EZLM_UPDATE_URL` (the documented hook, `update.go:59`) and `TMPDIR`, both
through `t.Setenv`; no test touches the real home or the network. Both platform
tests skip on Linux, where `assetFor` has no installer and the updater can never
report an update.

## Findings

No production code changed. The flow's decisions are right; the findings are traps
for the next reader or the next call site.

1. **`Info.Latest` is set even when the release is not newer** (`internal/update/update.go:93-105`).
   Any non-draft release with a matching asset is recorded; `Available` is computed
   afterwards at line 105. A consumer that shows `Latest` without checking
   `Available` offers a downgrade or an "update" to the running version. Both apps
   gate correctly (`macos/Sources/EZLM/Settings.swift:16`,
   `windows/EZLM/Panes.cs:125`, `windows/EZLM/SettingsPane.cs:54`). Severity: minor.
   Pinned by the `seen` column in the channel table; no fix, because changing it
   would break the "latest" meaning for the two apps.

2. **The first matching asset wins, not the best one** (`internal/update/update.go:98-102`).
   If a release ever ships two files that match (`EZLM.dmg` and `EZLM-0.1.12.dmg`,
   or `EZLM-Setup.exe` and `EZLM-Setup-0.1.12.exe`), the choice is whatever order
   GitHub lists them in. Today every release has exactly one per platform, so this
   is latent. Severity: minor. A cheap future rule: prefer the exact name, then
   the first match.

3. **The `prerelease` flag is ignored; the tag suffix is the channel**
   (`internal/update/update.go:76-88` decodes no `prerelease` field; the header
   comment at lines 19-22 says beta builds are included on purpose). A release
   marked prerelease with a stable tag is treated as stable, and a stable release
   with a `-beta` tag is only offered when it beats the running version. That is
   the documented policy, and the real releases confirm it (`v0.1.11-beta` is
   `prerelease: false` on GitHub). Severity: none, but it is the thing a06 should
   not change without updating this test. Pinned by "prerelease flag follows the
   tag".

4. **The release list body has no size limit** (`internal/update/update.go:89`).
   The 10s timeout (line 62) bounds time, not bytes; a broken or hostile API could
   return a very large JSON document. Ten releases are a few KB. Severity: minor.
   `io.LimitReader` on the decoder would close it; left for a08/a11 to weigh
   against the rate-limit changes in the same function.

5. **The asset URL's scheme and host are not checked** (`internal/update/update.go:100`).
   Whatever URL the API names is handed to `Download`. A rewritten response or a
   redirect can send the download to plain `http`. a02 and a11 own this; noted
   because the check flow is where the URL enters the system.

6. **A failed check is re-run on every settings call** (`internal/app/settings.go:36`:
   `updCache.Error != ""` forces a fresh `Check`). With a rate-limited or offline
   API, each refresh hits GitHub again. a01 owns the backoff; this test pins that
   one `Check` is one request.

## What could not be finished

Nothing in the slice is unfinished. The check flow has no injectable platform, so
the tests run the real `runtime.GOOS` and cover the other platform's asset
filtering by putting both files in the same release.

## Open questions

- Should the check flow prefer an exact asset name over a pattern match (finding 2)?
- Should `Latest` be cleared when it is not newer (finding 1), or is "newest seen"
  worth keeping for the UI?
- Does a15's shared fixture replace `checkflowGitHub`, or should both stay? Two
  fixtures in one package is harmless but the second is dead weight after the merge.

## Verification

`gofmt -l .` empty, `go vet ./...`, `go build ./...`, `go test -p 2 ./internal/update/`,
and the three cross-compiles, all as the brief requires.
