# T06/a14: upgrade plan (report)

Agent a14, task T06 (llama.cpp upgrade study). Cross-cutting slice: the
ordered plan for moving the pin, not the move itself. The pinned tag is
unchanged: `internal/engine/pinned.go` still says `b11960`.

Deliverable: `agents/T06/a14/upgrade-plan.md`.

## What I did

1. Read `README.md`, the T06 brief and the T06 section of `TASKS.md`.
2. Fetched the real release data (GET only) for `releases/latest`, the
   `nightly-tag.txt` asset at v0.9.0, and the b12144 tag; read a02's cached
   b12110 JSON (a02 is the Windows CUDA agent).
3. Diffed the asset-name set of the pinned b11960 against b12110 and b12144.
4. Ran the real selection code (`PickAssets`) against the real b12110 JSON
   for all 13 platform/backend rows EZLM can produce, with a throwaway test
   that was deleted afterwards. No source file was changed.
5. Wrote the plan: ordered steps, the exact `pinned.go` change, verification,
   rollback and risks, with gaps marked.

## Findings

**F1. "Latest release" resolves to b12110, not to the newest build (major,
expectation).**
`releases/latest` is `v0.9.0` (2026-11-04T13:08:06Z) and carries one asset,
`nightly-tag.txt`, whose content is `b12110`. `LatestRelease()`
(`internal/engine/engine.go:69-96`) follows exactly that indirection. The
newest build by date is `b12144` (2026-11-06T03:45:50Z). So pinning "the
latest release" means pinning `b12110`; pinning b12144 would make the default
ahead of what the app calls latest. Recommendation in the plan: pin b12110.

**F2. No asset was renamed or removed between b11960 and b12110/b12144
(minor, good news).**
34 assets at b11960, 38 at b12110 and at b12144. Four names added, none
removed, none renamed (compared with the tag replaced by `bNNNNN`):
`llama-<tag>-bin-ubuntu-26.04-s390x.tar.gz`,
`llama-<tag>-bin-ubuntu-zdnn-s390x.tar.gz`, `llama-<tag>-ui.tar.gz`,
`llama-<tag>-xcframework.zip`. None of them can be picked by
`assetPatterns` (`engine.go:110-141`): the first two are s390x, the last two
have no `-bin-` and are excluded by `PickAssets` (`engine.go:170`). The
`cudart-` names and the CUDA version suffixes are unchanged, so the pairing
in `engine.go:174-185` still works.

**F3. `pinned.go` is a hand-maintained literal with no generator (major for
the upgrade process).**
34 rows of `{Name, Size, URL, Digest}`, and its header says "Generated from
https://api.github.com/..." (`pinned.go:1-8`), but nothing in the repo
generates it (`grep -rn pinned --include=*.go --include=*.py` finds only
`pinned.go` and the tests). A single wrong size fails every install of that
asset with "the downloaded file came out the wrong size"
(`download.go:87-95`); a wrong digest deletes the archive
(`engine.go:250-253`). The plan makes generation and size verification an
explicit step.

**F4. `internal/app/demo.go:208` hardcodes `"Engine ready (b11960)"`
(minor, cosmetic).**
It will be stale the moment the pin moves. `internal/app` is a do-not-touch
area for T06, so this is a note for the app owners, not a patch here.

**F5. An empty digest silently disables integrity checking (minor, latent).**
`verifyDigest` returns nil when the digest is not `sha256:...`
(`engine.go:320-324`). Every b12110 asset does publish a digest, so this only
bites if a future pin drops one; the plan requires copying the digest for
every row.

**F6. GitHub's API rate limit is a real operational constraint (minor).**
60 requests an hour per address, shared by every agent on this Mac. I hit it
after four calls and got b12110's digests only from a02's cache. The plan
caches the JSON and does one fetch per tag.

**F7. s390x selection is ambiguous (minor, not blocking).**
The arch map only knows `amd64` and `arm64` (`engine.go:111`), so for s390x
`a` is empty and the Linux default pattern `-bin-ubuntu-`
(`engine.go:128`) matches `-bin-ubuntu-26.04-s390x`, `-bin-ubuntu-s390x`,
`-bin-ubuntu-zdnn-s390x` and the CUDA builds; the winner depends on API
order. EZLM does not support s390x today, so this is a note for whoever adds
it.

**F8. Selection output is unchanged by the bump (no severity).**
Verified against the real b12110 JSON: linux amd64/arm64 CPU, Linux CUDA 12
and 13, Linux Vulkan, macOS arm64/x64, Windows CPU x64/arm64, Windows CUDA 12
and 13 x64, Windows CUDA 13 arm64 and Windows Vulkan all return the expected
name, and the CUDA rows return the matching `cudart-` archive. The full table
is in `upgrade-plan.md` section 2. This is why the plan changes data only and
no logic.

## What changed in the repo

Only `agents/T06/a14/upgrade-plan.md` and this report. No change to
`internal/engine/**`, no change to the pinned tag, no test file (a13 owns the
matrix, a15 the script).

## Verification run

- `go test -count=1 -p 2 ./internal/engine/` passes (used for the probe and
  for the baseline).
- The probe test was temporary (`zz_probe_a14_test.go`) and removed; no
  build output or cached JSON was committed.

## Gaps and next steps

- **G1**: the server-flag findings (a09-a12) were not in this worktree, so
  risk R4 (`--jinja`, `-np 1`, `--metrics` at `internal/serve/config.go:124-126`,
  `/props` at `internal/serve/props.go:13`) is unquantified. The pin should
  not move until they report.
- **G2**: a01-a08 and a13's per-platform tables were not available; section 2
  comes from the real JSON and the current code, not from their reports.
- **G3**: b12144 digests were not fetched (rate limit). Needed only if the
  team pins b12144.
- **G4**: no real install was performed, so the "archive actually runs" step
  is untested.
- Next: fold a09-a12 into the plan's risk table, add a15's
  `check_assets.py` as the generation step, and have the lead choose between
  b12110 (recommended) and b12144.
