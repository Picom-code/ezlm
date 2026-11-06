# Upgrade plan: move the llama.cpp pin from b11960 to the latest release

Status: draft, based on data collected 2026-11-06. The pinned tag in this
branch is unchanged (`internal/engine/pinned.go` still says `b11960`). This is
the plan, not the upgrade.

Owner: T06/a14. Consumes findings from a01 to a13 and a15, a16. Where a
finding was not available in this worktree it is marked **GAP**.

## 1. What "latest" resolves to

| Thing | Value | Evidence |
|---|---|---|
| `releases/latest` | `v0.9.0`, published 2026-11-04T13:08:06Z, **1 asset**: `nightly-tag.txt` (7 bytes) | GitHub API, 2026-11-06 |
| `nightly-tag.txt` at v0.9.0 | `b12110` | downloaded, content is `b12110` |
| Newest build by date | `b12144`, 2026-11-06T03:45:50Z, 38 assets | release page |
| Assets at `b12110` | 38 | cached JSON, `agents/T06/a02/latest-build.json` (a02's cache) |
| Assets at `b11960` (pin) | 34 | `internal/engine/pinned.go` |

`LatestRelease()` (`internal/engine/engine.go:69`) follows the same path a user
takes with `EZLM_LLAMA=latest`: v0.9.0 has no `-bin-` asset, so it reads
`nightly-tag.txt` and fetches `releases/tags/b12110`.

**Recommendation: pin `b12110`.** It is the build the latest release names, so
the pin and the opt-in `latest` path install the same binaries. Pinning
`b12144` would put the default ahead of what the app calls "latest" and would
leave two behaviours in the field. `b12144` is only 22 builds newer; take it
in the following bump.

If the team wants `b12144`, everything below still applies; only the tag, the
asset list and the digests change. Its digests were **not** fetched (API rate
limit), see gap G3.

## 2. Asset inventory change, b11960 to b12110 (and b12144)

Compared by name with the tag replaced by `bNNNNN`:

| Change | Asset | Effect on EZLM |
|---|---|---|
| added | `llama-bNNNNN-bin-ubuntu-26.04-s390x.tar.gz` | none: s390x is not a supported arch |
| added | `llama-bNNNNN-bin-ubuntu-zdnn-s390x.tar.gz` | none, same |
| added | `llama-bNNNNN-ui.tar.gz` | none: no `-bin-`, never matches `assetPatterns` |
| added | `llama-bNNNNN-xcframework.zip` | none, same |
| removed | none | |
| renamed | none | |

No archive format changed (still `.tar.gz` for macOS/Linux, `.zip` for
Windows). CUDA version suffixes are unchanged: Linux `cuda-12.8` and
`cuda-13.4`, Windows `cuda-12.4` and `cuda-13.4`, Windows arm64 CUDA 13 only.
The `cudart-` runtime archives keep the same names modulo tag, so
`cudaFlavor`/cudart pairing (`internal/engine/engine.go:155-190`) still works.

Verified by running the real selection code against the real b12110 JSON (a
throwaway test, removed afterwards). Every platform/backend EZLM can produce
returns the expected asset, with the expected `cudart-` companion:

```
linux/amd64  cpu      -> llama-b12110-bin-ubuntu-x64.tar.gz
linux/arm64  cpu      -> llama-b12110-bin-ubuntu-arm64.tar.gz
linux/amd64  cuda     -> llama-b12110-bin-ubuntu-cuda-12.8-x64.tar.gz + cudart-...
linux/amd64  cuda13   -> llama-b12110-bin-ubuntu-cuda-13.4-x64.tar.gz + cudart-...
linux/amd64  vulkan   -> llama-b12110-bin-ubuntu-vulkan-x64.tar.gz
darwin/arm64 metal    -> llama-b12110-bin-macos-arm64.tar.gz
darwin/amd64 metal    -> llama-b12110-bin-macos-x64.tar.gz
windows/amd64 cpu      -> llama-b12110-bin-win-cpu-x64.zip
windows/arm64 cpu      -> llama-b12110-bin-win-cpu-arm64.zip
windows/amd64 cuda     -> llama-b12110-bin-win-cuda-12.4-x64.zip + cudart-...
windows/amd64 cuda13   -> llama-b12110-bin-win-cuda-13.4-x64.zip + cudart-...
windows/arm64 cuda13   -> llama-b12110-bin-win-cuda-13.4-arm64.zip + cudart-...
windows/amd64 vulkan   -> llama-b12110-bin-win-vulkan-x64.zip
```

**Consequence: no selection-logic change is required for this bump.** The only
change is the `pinnedRelease` data.

## 3. Ordered steps

Do these in order. Steps 1 to 3 are one commit; 4 to 6 a second; 7 a third.

### Step 1: capture the release JSON for the target tag

```
curl -sSL -H 'Accept: application/vnd.github+json' -H 'User-Agent: ezlm' \
  https://api.github.com/repos/ggml-org/llama.cpp/releases/tags/b12110 \
  -o agents/T06/a14/b12110.json
```

Keep the file under 5 MB (this one is ~60 KB) or leave it out of the commit.
Re-fetch `releases/latest` first and re-read `nightly-tag.txt`: the tag can
move between now and the upgrade.

### Step 2: regenerate the `pinnedRelease` literal

Replace the whole literal in `internal/engine/pinned.go` from the JSON. The
file is 34 hand-written `{Name, Size, URL, Digest}` tuples and there is no
generator in the repo (see finding F3), so either:

- run a15's `agents/T06/check_assets.py` (stretch) or a one-off Python pass to
  emit the block, or
- generate with `jq -r` and paste, then diff carefully.

Keep, in this order:

1. `var pinnedRelease = Release{Tag: "b12110", ...}`
2. every asset, sorted by name as today, with `Size` exactly as the API
   reports it and `Digest` as the API's `sha256:<hex>`
3. the comment above it, with the tag URL updated.

Do not filter the list down to "assets EZLM can pick": the full list is what
the file holds today and it is what makes cudart pairing and future patterns
work.

### Step 3: keep the pinned-tag comment truthful

`internal/engine/pinned.go:1-7` explains the fallback. Only the URL and tag
inside it change. No code change.

### Step 4: update the test fixtures

`internal/engine/engine_test.go` hardcodes b11960 asset names in
`release` (line 16) and in `TestPickAssets` (line 36), and
`TestPinnedReleaseCoversPlatforms` (line 68) runs the pinned release. The
fixture is a separate, smaller list; it must keep the same shape at the new
tag. a13's `assets_matrix_test.go` is the index that should cover both tags.

### Step 5: run the matrix at both tags

For each of the 13 platform/backend rows in section 2, assert the picked name
at b11960 **and** at b12110 (and b12144 if that is chosen). a13 owns the
single table; a15's script prints and checks the names against the real
release. A row that changes name between the two tags is a selection bug, not
an upgrade task.

### Step 6: verify sizes and digests without downloading archives

- Sizes: `curl -sSI -L <browser_download_url>` and compare
  `content-length` with the `Size` written into `pinned.go`. A wrong size is
  fatal: `Download` requires an exact match when `want > 0`
  (`internal/engine/download.go:87-95`), and a wrong digest deletes the
  archive after download (`engine.go:250-253`).
- Digests: every b12110 asset carries a `sha256:` digest in the API response
  (verified). `verifyDigest` returns nil for an empty digest
  (`engine.go:320-324`), so a missing digest in the literal silently drops
  integrity checking.
- Spot-check two archives by hashing them only if a real install is being
  tested; do not download all 38.

### Step 7: smoke test each OS/backend once

Install on one machine per OS/backend family and check
`llama-server --version` (or `/props` `build_info`) reports `b12110`. This is
the only step that proves the archives actually run, and it is the step most
likely to be skipped. On this shared Mac, do not load a model; starting
`llama-server` alone is enough and only on a machine that is not serving.

## 4. What to change in `pinned.go`, precisely

| Line | Today | After |
|---|---|---|
| 8 | `var pinnedRelease = Release{Tag: "b11960", ...` | `Tag: "b12110"` |
| 9-41 | 34 asset tuples | 38 asset tuples from the b12110 JSON |
| 1-7 | comment citing the b11960 API URL | same comment, b12110 URL |

Nothing else in the file. No function, no type, no behaviour.

If a later tag renames a family (e.g. `ubuntu-cuda-12.8` to `ubuntu-cuda-12.9`),
that is an `assetPatterns` change in `engine.go` and needs its own failing test
first. Nothing in this range requires it.

## 5. What to verify before merging

Required, per the task brief:

- `gofmt -l .` empty
- `go build ./...`, `go vet ./...`
- `go test -p 2 ./internal/engine/`
- `GOOS=windows GOARCH=amd64 go build ./...`, `GOOS=windows GOARCH=arm64 go build ./...`,
  `GOOS=linux go build ./...`

Specific to the bump:

1. selection matrix green at b11960 and b12110 (section 2 table)
2. every asset EZLM can pick exists at the new tag (a15 script)
3. size and digest fields match the API (step 6)
4. `pinnedRelease.Tag` equals the tag `LatestRelease()` resolves today; if it
   does not, say why in the commit message
5. one install per OS/backend reports the new tag (step 7)
6. an existing b11960 install still starts (the old folder is untouched; see
   risk R3)
7. no `internal/app/demo.go` string says `b11960` (finding F4)

## 6. Rollback

The pin is a single self-contained file, and installs live in per-tag folders
(`~/.ezlm/bin/llama-<tag>-<flavor>`, `engine.go:228`). So:

1. `git revert <pin commit>` (or restore `pinned.go` from the previous
   commit) and ship. The next launch resolves the old tag again.
2. If the old b11960 folder is still on disk, `Install` reuses it without
   downloading (`engine.go:229-231`), so the rollback is instant and offline.
3. If a bad archive was unpacked, delete that tag's folder
   (`~/.ezlm/bin/llama-b12110-*`) and relaunch; nothing else references it.
4. Nothing in the upgrade deletes the previous tag's folder, so rollback is
   always available until someone cleans up. If a later change adds cleanup,
   it must keep at least the previous tag.

The upgrade is reversible without touching user data: models live under
`~/.ezlm/models`, keys and settings are separate.

## 7. Risks

| # | Risk | Severity | Evidence | Mitigation |
|---|---|---|---|---|
| R1 | "latest" is not the newest build: v0.9.0 names b12110, while b12144 exists | major (expectation) | nightly-tag.txt = b12110; b12144 published 2026-11-06 | pin b12110; record b12144 as the next bump |
| R2 | Hand-typed size/digest in a 38-row literal; one wrong byte fails every install of that asset | major | `pinned.go` has 34 literal rows, no generator; `download.go:87-95` exact-size check | generate the block from JSON; verify content-length (step 6) |
| R3 | Old installs are not removed: disk grows by one build per upgrade (CPU ~25 MB, CUDA 12 x64 Windows ~290 MB plus 390 MB cudart) | minor | `engine.go:228` names folders by tag; no cleanup code | accept for beta; document; keep previous tag for rollback |
| R4 | Server flags and `/props` schema may have changed between b11960 and b12110 (`--jinja`, `-np 1`, `--metrics` at `internal/serve/config.go:124-126`; `Props` parsing at `internal/serve/props.go:13`) | major, unquantified | **GAP G1**: a09-a12 findings not in this worktree | do not ship the pin until a09-a12 report; test `/props` against a real b12110 server |
| R5 | GitHub API rate limit (60/h per address) blocks regenerating the pin | minor, operational | hit during this study on this Mac | cache the release JSON; one fetch per tag; a15 script |
| R6 | s390x asset family is now ambiguous (`-bin-ubuntu-26.04-s390x`, `-bin-ubuntu-s390x`, `-bin-ubuntu-zdnn-s390x`), and the arch map only knows amd64/arm64 so `a` is empty and `-bin-ubuntu-` matches several | minor | `engine.go:111`, `engine.go:128`; b12110 asset list | no s390x support today; if added, add an explicit arch mapping and a test |
| R7 | Pinning b12144 instead of b12110 makes the pin disagree with `EZLM_LLAMA=latest`, so two builds are in the field | minor | `LatestRelease` follows nightly-tag.txt | pin b12110 |
| R8 | `internal/app/demo.go:208` says "Engine ready (b11960)" | minor, cosmetic | hardcoded string | app owners update it in the same release; T06 must not edit `internal/app` |

## 8. Gaps in this plan

- **G1**: server-flag and `/props` findings (a09-a12) were not available in
  this worktree. R4 stays open until they land.
- **G2**: the platform-by-platform asset reports (a01-a08) and a13's matrix
  are not in this worktree; section 2 is verified from the real JSON with the
  current selection code, but not yet from their per-agent tables.
- **G3**: b12144 digests and sizes were not fetched (API rate limit). If
  b12144 is chosen, re-run step 1 for it.
- **G4**: no real install was performed; step 7 is untested.
- **G5**: a15's `check_assets.py` did not exist in this worktree at the time
  of writing; steps 2 and 6 assume it or a one-off script.
