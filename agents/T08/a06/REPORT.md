# T08 a06: prereleases and drafts

Branch: `agent/T08-a06`. Worktree: `~/ezlm-agents/work/subs/T08/a06`.

## Fault

The updater picks a release out of GitHub's `/releases` list. The documented channel
policy is on `ReleasesURL` (`internal/update/update.go:19`): the list endpoint is used
on purpose because it includes beta builds, which `/releases/latest` skips, and on
`Newer` (`internal/update/update.go:131`): a final release beats its own pre-release.
Drafts are unpublished, so they are not candidates at all.

I exercised the three cases in the brief against the current updater:

| Case | Expected | Before fix |
|---|---|---|
| `prerelease: true`, newer tag | offered | offered, correct |
| `draft: true` | not offered | not offered, correct |
| `draft: true` and `prerelease: true` | not offered | not offered, correct |
| draft newer than a published prerelease | published one offered | correct |
| final `v0.1.12` and `v0.1.12-beta` in either list order | final offered | correct |
| final release of the running beta version | offered | correct |

The selection logic itself follows the policy. The real gap is that `Check` throws
GitHub's `prerelease` flag away: the decoded field was missing from the release
struct and from the `Release` it returns. The apps can therefore not tell a beta
update from a final one, and a release flagged `prerelease: true` whose tag has no
`-beta` suffix is indistinguishable from a final release after the check.

This is **minor**: it mislabels an update, it does not install something it should not.

## Reproduction

`internal/update/channel_test.go` stands in for the GitHub API with `httptest` and
sets `EZLM_UPDATE_URL`; no request leaves the process and no files are written. On
`darwin`/`windows` the fake release carries the asset `Check` looks for; on other
platforms `assetFor` returns nothing, so those tests skip.

Before the fix, `TestChannelKeepsPrereleaseFlag` failed:

```
--- FAIL: TestChannelKeepsPrereleaseFlag
    channel_test.go:142: prerelease flag lost for 0.1.12
```

After the fix all of `TestChannel*` pass.

## Fix

`internal/update/update.go`:

- `Release` carries `Prerelease bool` (`json:"prerelease"`), set from the API field
  when a release is chosen (`update.go:34`, `update.go:83`, `update.go:104`).
- A comment on the selection loop states the channel policy (`update.go:95`).

No behaviour change in which release is chosen. The flag is metadata for the apps,
which can now label a beta update. `windows/EZLM/Models.cs:248` and
`macos/Sources/EZLM/Models.swift:338` do not decode it yet (both are frozen for
tonight), so this is a follow-up for whoever owns the update UI.

`internal/update/channel_test.go` locks in the policy: prereleases offered, drafts
and draft prereleases skipped, a draft does not hide a published release, the final
release wins over its own pre-release in either order, and the flag survives.

## Evidence from the real release list

Read-only `GET https://api.github.com/repos/picom-code/ezlm/releases?per_page=30`
(2026-11-06) shows the flag and the tag disagree in practice:

- `v0.1.11-beta` … `v0.1.1-beta`: `prerelease: false`, `draft: false`.
- `v0.1.0-beta`: `prerelease: true`.

So the tag suffix, not the flag, has to stay the ordering signal (`Newer`), and the
updater must not filter on `prerelease` or the whole beta series would disappear.
That matches the code: the flag is carried, not used to gate. One consequence is
that `Newer` treats `v0.1.12-beta` as a pre-release even when GitHub marks it final,
which is the right call for this repo.

## Residual risk and open questions

- **Cached draft (not fixed).** `internal/app/settings.go` caches `update.Info` for an
  hour and `DownloadUpdate` downloads from the cached `Release`. If a release is
  turned into a draft after the check, the updater still installs it from the cached
  metadata. `Download` never re-reads the release. Revalidating there overlaps the
  redirect/asset-change fault (a02) and the end-to-end flow (a13), so I left it.
- **No channel gate.** There is no notion of a stable channel: a user on a final
  version would be offered the next beta. That follows `ReleasesURL`'s "beta builds
  included" today (the shipped app is `0.1.11-beta`), but if a stable channel is
  added, `Check` needs the running version's channel as an input. Worth a product
  decision, not a silent change here.
- `Check` requests `per_page=10`. If authenticated drafts ever enter the list, they
  consume slots and can push a published release off the page. Unauthenticated
  requests do not return drafts, so this is latent.

## Verification

```
gofmt -l .                      # empty
go build ./...                  # ok
go vet ./...                    # ok
go test -p 2 ./internal/update/ # ok
GOOS=windows GOARCH=amd64 go build ./...
GOOS=windows GOARCH=arm64 go build ./...
GOOS=linux go build ./...
```

Commit: `e53f789` (fix and tests), plus this report.
