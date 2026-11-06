# T06/a15: asset check script

Agent a15, cross-cutting. Branch `agent/T06-a15`.

## What I did

Added `agents/T06/check_assets.py`, a standard-library-only Python 3 script that
mirrors EZLM's release asset selection and proves the picks exist:

- For one or more tags (or `latest`), it fetches the release and prints, for
  every OS/arch/backend EZLM can produce, the exact archive
  `engine.PickAssets` would choose and the matching `cudart-` runtime archive.
- It mirrors `engine.assetPatterns` (`internal/engine/engine.go:110`),
  `matches` (`:143`), `cudaFlavor` (`:156`), `PickAssets` (`:167`) and
  `LatestRelease` (`:69`) line for line, including the CUDA 12 over CUDA 13
  preference, the CUDA 13 path for Blackwell and arm64, the Vulkan fallback
  rows for ROCm machines, and the cudart companion lookup by flavor and OS.
- It verifies every pick is present in the release listing, and with `--head`
  that the download URL answers. `--json` gives machine-readable output for
  a13/a16, `--cache DIR` caches API JSON, `--pinned` also checks the picks
  against `internal/engine/pinned.go`.
- It reads releases from the REST API, and falls back to GitHub's
  `releases/expanded_assets/<tag>` HTML when the API is rate limited (it was,
  see F1). GET and HEAD only; it never downloads an archive.
- `latest` is resolved the way `engine.LatestRelease` does it: follow
  `releases/latest` to the stable v0.9.0 tag, read its `nightly-tag.txt`, then
  read that build's assets. The HTML fallback does the same over the release
  page.

The 18 platform rows cover darwin arm64/amd64, linux amd64/arm64, windows
amd64/arm64, and every backend `internal/scan` assigns: Metal, CUDA 12,
CUDA 13, ROCm (which EZLM installs as Vulkan), Vulkan and CPU. CUDA 13 rows
exist only where `Install` can set `cuda13`: Blackwell x64 and all arm64.

## Proof

Full output is in `agents/T06/a15/samples.txt`.

| tag | role | rows | ok | no match | without cudart |
|---|---|---|---|---|---|
| b11960 | pinned fallback | 18 | 18 | 0 | 0 |
| b12110 | latest stable release (`releases/latest` = v0.9.0 -> nightly-tag.txt) | 18 | 18 | 0 | 0 |
| b12144 | newest build by date | 18 | 18 | 0 | 0 |

- b11960: every pick is also present in `pinned.go` (34 assets), so the
  offline fallback agrees with the API listing for every platform EZLM serves.
- `--head b12144`: 22 distinct URLs checked, all present (HTTP 200 after
  GitHub's redirect). No archive was downloaded.
- Selection is stable across the three tags: only the tag inside each name
  changes.

## Findings

**F1 (minor, operational). GitHub's REST API is exhausted by the parallel
agents.** `GET /repos/ggml-org/llama.cpp/releases/tags/b11960` returned
`403 API rate limit exceeded for 104.184.2.59` during the run, and the same
for `/releases/latest`. The script recovers by reading
`https://github.com/ggml-org/llama.cpp/releases/expanded_assets/<tag>`, which
serves the same names and is not API limited (`source=html` in the samples).
EZLM itself makes at most two API calls per install (`engine.go:69`,
`engine.go:76`), so this does not affect users; it does mean a14/a16 cannot
assume the API is available while sixteen agents are running. The script's
`--cache` flag and HTML fallback are the workaround.

**F2 (major for the upgrade plan). "Latest release" is b12110, not
b12144.** `releases/latest` is v0.9.0 (2026-11-04); it ships no binaries, only
`nightly-tag.txt`, whose contents are `b12110`. The newest build tag by date,
`b12144`, exists and has the same 38 asset names (modulo tag), but the stable
release does not point at it yet. Moving the pin to "the latest release" means
**b12110** unless a14 deliberately chooses the newest build. Both are proven
clean here.

**F3 (minor). No asset renames or removals that affect EZLM between b11960 and
b12110/b12144.** Normalised name diff b11960 -> b12110 adds only
`llama-<tag>-bin-ubuntu-26.04-s390x.tar.gz` and
`llama-<tag>-bin-ubuntu-zdnn-s390x.tar.gz`; nothing is removed. b12144 adds
and removes nothing relative to b12110. All macos, win-cpu, win-cuda,
ubuntu, ubuntu-cuda, ubuntu-vulkan and cudart names EZLM selects still exist.
In particular macOS x64 is still published, Windows arm64 still has only a
CUDA 13 build (which `Install` forces on arm64), and every CUDA build still
has its matching cudart.

**F4 (minor, latent). A CUDA pick with no cudart companion is installed
silently.** `PickAssets` appends the cudart archive only if it finds one and
still returns the primary archive when it does not (`engine.go:167-196`), and
`Install` only requires `llama-server` in the unpacked tree. If a future
release publishes a CUDA build without its runtime archive, EZLM installs a
build that cannot start on Windows instead of falling back to Vulkan. No
current tag triggers it (0 rows without cudart at all three tags); the script
reports the case as `ok, no cudart` so a future run will catch it.

**F5 (minor, latent). CUDA version preference is listing order, not version
order.** The CUDA 12 pattern is the prefix `-bin-ubuntu-cuda-12` plus the arch
(`engine.go:122`), so if a release ever shipped both `cuda-12.4` and
`cuda-12.8` builds for one arch, the first in the API's upload order would
win, not the newer one. Only one CUDA 12 build per arch exists at all three
tags, so this is latent. Same caveat for the HTML fallback, whose order can
differ from the API's; no current row is ambiguous.

## What I did not do

- Did not edit `internal/engine/engine.go`, `pinned.go` or any test file: the
  selection logic expresses every platform correctly at all three tags, so
  there was nothing to fix and no failing test to write.
- Did not verify archive contents, digests or sizes; the script checks names
  and URLs only. a07 owns runtime DLL/so sets.
- Did not download an archive (HEAD only), per the brief.

## Open questions for the Lead

1. Upgrade target: b12110 (what `releases/latest` resolves to today) or
   b12144 (newest build)? F2; a14 needs a decision either way.
2. Should `PickAssets` error, or fall back, when a CUDA build has no cudart
   companion? F4; current behaviour is a silent partial install. Not fixed
   here to keep the a15 patch to the script.
