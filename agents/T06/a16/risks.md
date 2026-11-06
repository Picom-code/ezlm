# T06 a16: risk matrix and report support

Status: first pass, written at 2026-11-06 07:05 UTC (a16). Sibling findings from
a01 to a15 were not committed yet when this was written; section 5 is the slot
where they land at assembly time.

## 1. What this is grounded on

- Read: `internal/engine/engine.go` (all), `internal/engine/pinned.go`,
  `internal/serve/config.go`, `internal/serve/props.go`, `internal/serve/run.go`,
  `internal/serve/client.go` (endpoint use), `internal/scan/scan.go`,
  `internal/scan/gpu_discrete.go`, `internal/prereq/prereq.go`.
- Ran: a throwaway probe test (deleted, not committed) calling
  `engine.PickAssets` for every platform/backend pair at `b11960` (from
  `pinnedRelease`) and `b12110` (from the release JSON cached by a02 at
  `agents/T06/a02/latest-build.json`, copied here as `latest-build.json`).
  Raw output: `agents/T06/a16/picks.txt`, one line per case, format
  `<tag> <goos> <arch> <backend> c13=<bool> -> [asset, cudart?]`.
- Read-only GETs against raw.githubusercontent.com at both tags for
  `common/arg.cpp`, `common/chat.cpp`, `tools/server/server.cpp`,
  `tools/server/server-context.cpp` (not committed; sizes below).
- Release facts: `releases/latest` returns `v0.9.0` (2026-11-04) whose only
  asset is `nightly-tag.txt` containing `b12110`. `b12110` was published
  2026-11-04T12:52Z with 38 assets. Newer nightlies exist (`b12144`
  2026-11-06T03:45Z, `b12167` 2026-11-06T09:45Z); the asset-name set of both
  is identical to `b12110` once the tag is normalised (checked against the
  100-release JSON a07 cached).

## 2. Severity legend

- **blocker**: the upgrade cannot ship until this is fixed.
- **major**: likely user-visible breakage or a wrong artifact, with a workaround.
- **minor**: real but narrow; fix when convenient.
- **info**: no action, recorded so nobody re-investigates.

No blocker was found. The upgrade is asset-safe and flag-safe as far as this
slice can see; the risk is concentrated in regenerating `pinned.go` and in
choosing the right target tag.

## 3. Findings, ranked

### R1 (major) - `pinned.go` is a hand-shaped b11960 snapshot, so the pin move needs a full regeneration

`internal/engine/pinned.go:9-43` is 34 hardcoded assets with size, URL and
`sha256:` digest, generated from the `b11960` release. The b12110 release has
38 assets. Moving the pin means every URL, size and digest in this file must be
regenerated from
`https://api.github.com/repos/ggml-org/llama.cpp/releases/tags/b12110`;
a partially edited file (tag substituted, digests left behind) fails closed at
`verifyDigest` (`engine.go:320-338`) after the user has downloaded up to 600 MB.
Evidence: `grep -c 'Name: "' internal/engine/pinned.go` = 34;
`agents/T06/a16/latest-build.json` has 38 assets, tag `b12110`.
Next step: a14's plan should make regeneration a scripted step, not an edit,
and the review should diff asset names and sizes between the two tags.

### R2 (major) - "latest release" means b12110, not the newest build

`engine.LatestRelease` (`engine.go:69-96`) follows `releases/latest`; that
release (`v0.9.0`) has no binaries, only `nightly-tag.txt` = `b12110`. The
newest builds by date are `b12144` and `b12167` (published 2026-11-06), which
are nightlies the stable release does not pin. An upgrade plan that says
"newest build" would pin an untested nightly. Evidence: a02's `latest.json`
(v0.9.0, 1 asset), `nightly-tag.txt` (`b12110`), a07's `releases-100.json`
tag order and `published_at`.
Next step: the plan should name `b12110` as the target and note `b12144` /
`b12167` as "newer, not release-pinned" so the pin move is deliberate.

### R3 (minor) - an unsupported arch silently gets another arch's binary

`assetPatterns` maps only `amd64` and `arm64` (`engine.go:111`); any other
`GOARCH` becomes `""`, so the pattern degenerates to `-bin-ubuntu-` etc. and
`PickAssets` returns the first matching asset. Probe result at `b11960`:
`linux/s390x cpu -> llama-b11960-bin-ubuntu-arm64.tar.gz` (an arm64 binary on
an s390x host). At `b12110` it happens to pick
`llama-b12110-bin-ubuntu-26.04-s390x.tar.gz`, but only because that name sorts
before `ubuntu-arm64` in GitHub's asset order. Evidence: `picks.txt` lines for
`linux s390x cpu` at both tags; `engine.go:143-153` (`matches`), `engine.go:167-190`.
Impact on the upgrade: none for shipped platforms (EZLM builds amd64/arm64
only), but the selection is order-dependent for anything else, and the new
`ubuntu-26.04-s390x` / `zdnn-s390x` assets make the s390x family larger.
Next step: a08/a13 should decide whether unknown arches should error instead of
falling back; that is a one-line guard, not an upgrade requirement.

### R4 (minor, pre-existing) - ROCm backend is mapped to Vulkan; the ROCm assets are never selected

`assetPatterns` groups `scan.ROCm` with `scan.Vulkan` (`engine.go:125-126`,
`engine.go:134-135`), so a ROCm machine installs the Vulkan archive even though
both releases ship `llama-<tag>-bin-ubuntu-rocm-10.0-x64.tar.gz` and
`llama-<tag>-bin-win-rocm-10.0-x64.zip`. The scan code never sets
`h.Backend = scan.ROCm` anyway (`internal/scan/gpu_discrete.go:171` sets
Vulkan for AMD), so this is consistent, not a regression. Evidence:
`picks.txt` `rocm` rows at both tags pick `vulkan`; `pinned.go:25,39` list the
ROCm assets. Impact on the upgrade: none, the same mapping exists at both tags.
Next step: a06 should state whether EZLM intends to use ROCm builds; if not,
`scan.ROCm` is dead vocabulary and the pinned ROCm entries are ~516 MB of
unused metadata.

### R5 (minor) - new non-server assets since b11960, currently harmless

Between `b11960` and the first release in a07's list (`b11988`) four assets
appear that the pin does not have: `llama-<tag>-ui.tar.gz`,
`llama-<tag>-xcframework.zip`, `llama-<tag>-bin-ubuntu-26.04-s390x.tar.gz`,
`llama-<tag>-bin-ubuntu-zdnn-s390x.tar.gz`. All four are still present at
`b12110`, `b12144` and `b12167`. They do not affect selection today because
`PickAssets` requires the `llama-` prefix and a `-bin-` pattern, and
`matches` requires `.zip`/`.tar.gz` (`engine.go:143-153,167-172`); `ui.tar.gz`
and `xcframework.zip` contain no `-bin-`, so they cannot match. The risk is
future: a matcher that drops the `-bin-` requirement could pick the 75 MB
xcframework archive on macOS. Evidence: `latest-build.json` asset names,
`pinned.go` name set, a07's 100-release list.
Next step: keep the `-bin-` requirement in any rewritten matcher; note the new
names in a14's plan so nobody mistakes them for renamed builds.

### R6 (minor) - the nightly-tag follow path trusts the response

`LatestRelease` reads `nightly-tag.txt` from the asset URL without checking the
HTTP status (`engine.go:81-88`), truncates at 64 bytes, and uses whatever body
came back as a tag. A rate-limited or redirected response yields a garbage tag,
then a confusing "could not reach GitHub" error from the following
`/tags/<tag>` call. Also, `http.DefaultClient` has no timeout here (the
download path sets its own, `download.go`). This path is only reached with
`EZLM_LLAMA=latest`; the pinned path never touches the API. Evidence:
`engine.go:77-95`. Impact: no upgrade blocker, but worth a status check before
the beta starts pointing at the new pin.

### R7 (minor) - an upgrade leaves the previous engine folder on disk

`Install` names the folder `llama-<tag>-<flavor>` (`engine.go:226-231`) and
only removes the folder it is about to replace (`engine.go:263-266`). Changing
the pin therefore leaves the old build behind; a CUDA flavor is 160-290 MB per
archive plus the cudart archive (up to 594 MB). `existing()` (`engine.go:285-296`)
still finds the newest folder, so the app keeps working, but disk usage grows by
one full engine per upgrade. Evidence: `pinned.go` sizes; `engine.go:226-266`.
Next step: a14 should note a cleanup step (delete `llama-<oldtag>-*` after a
successful upgrade) or accept the growth for the beta.

### R8 (info) - server flags and `/props` schema EZLM depends on are unchanged from b11960 to b12110

Checked because a16 assembles the risk picture; a09 to a12 own the detail.

- Every flag `Config.Args` passes (`internal/serve/config.go:116-137`) still
  exists in `common/arg.cpp` at b12110 with the same definition: `-m`,
  `--alias`, `--host`, `--port`, `--api-key`, `-c`, `-ngl`, `--jinja`,
  `--metrics`, `-np`, `--n-cpu-moe`, `-md`, `--device`.
- `--jinja`, `--kv-unified`, `--cache-ram`, `--reasoning-format`,
  `--reasoning-budget` option text and defaults are byte-identical in
  `arg.cpp` at both tags.
- `tools/server/server.cpp` is byte-identical at both tags (32346 bytes), so
  `/health`, `/props`, `/slots`, `/metrics`, `/v1/messages` routes are
  unchanged.
- `common/chat.cpp` is byte-identical (83283 bytes), so
  `chat_template_caps`, `reasoning_effort_levels`, `supports_thinking`,
  `enable_thinking` keep their shape.
- `tools/server/server-context.cpp` (304170 -> 306078 bytes) still emits
  `total_slots`, `chat_template_caps`, `build_info`, `modalities`, and
  `default_generation_settings.n_ctx`; `internal/serve/props.go:41-58` reads
  exactly those.
Evidence: raw files fetched at both tags; counts and diffs recorded here.
Caveat: this is a text-level check, not a running server (no model may be
loaded), so runtime behaviour changes inside unchanged code are out of scope.

### R9 (info) - CUDA version guard still matches the shipped archives

`cudaOK(hw.CUDAMax, "12.4")` (`engine.go:215`) is the floor for CUDA 12, and
the releases still ship `win-cuda-12.4` and `ubuntu-cuda-12.8` at both tags.
CUDA 12.x minor-version compatibility covers a 12.4 driver running a 12.8
build, so the floor is deliberately conservative. The risk would be a future
release that drops 12.4 or moves to 13-only for x64; the selection then falls
through `-bin-*-cuda-` to the newest CUDA or to Vulkan
(`engine.go:124,133`). Evidence: `latest-build.json` asset list; `picks.txt`.

## 4. Test matrix: platform x tag x expected asset x status

Probe: `PickAssets(release, goos, arch, backend, cuda13)`, output in
`agents/T06/a16/picks.txt`. Status meaning: **PASS** the expected asset (and
cudart, for CUDA) is chosen; **WARN** a documented fallback; **FAIL** wrong or
order-dependent choice. `c13` is `cuda13` as `Install` computes it
(`engine.go:218`).

| # | platform | backend | c13 | b11960 picks | b12110 picks | status |
|---|---|---|---|---|---|---|
| 1 | darwin/arm64 | metal | no | `llama-b11960-bin-macos-arm64.tar.gz` | `llama-b12110-bin-macos-arm64.tar.gz` | PASS |
| 2 | darwin/amd64 | metal | no | `llama-b11960-bin-macos-x64.tar.gz` | `llama-b12110-bin-macos-x64.tar.gz` | PASS |
| 3 | linux/amd64 | cuda 12 | no | `...-ubuntu-cuda-12.8-x64.tar.gz` + cudart | same, tag b12110 | PASS |
| 4 | linux/amd64 | cuda 13 | yes | `...-ubuntu-cuda-13.4-x64.tar.gz` + cudart | same, tag b12110 | PASS |
| 5 | linux/arm64 | cuda 13 | yes | `...-ubuntu-cuda-13.4-arm64.tar.gz` + cudart | same, tag b12110 | PASS |
| 6 | linux/amd64 | vulkan | no | `...-ubuntu-vulkan-x64.tar.gz` | same, tag b12110 | PASS |
| 7 | linux/arm64 | vulkan | no | `...-ubuntu-vulkan-arm64.tar.gz` | same, tag b12110 | PASS |
| 8 | linux/amd64 | rocm | no | `...-ubuntu-vulkan-x64.tar.gz` (rocm unused) | same | WARN (R4) |
| 9 | linux/amd64 | cpu | no | `...-ubuntu-x64.tar.gz` | same, tag b12110 | PASS |
| 10 | linux/arm64 | cpu | no | `...-ubuntu-arm64.tar.gz` | same, tag b12110 | PASS |
| 11 | windows/amd64 | cuda 12 | no | `...-win-cuda-12.4-x64.zip` + cudart | same, tag b12110 | PASS |
| 12 | windows/amd64 | cuda 13 | yes | `...-win-cuda-13.4-x64.zip` + cudart | same, tag b12110 | PASS |
| 13 | windows/arm64 | cuda 13 | yes | `...-win-cuda-13.4-arm64.zip` + cudart | same, tag b12110 | PASS |
| 14 | windows/amd64 | vulkan | no | `...-win-vulkan-x64.zip` | same, tag b12110 | PASS |
| 15 | windows/arm64 | vulkan | no | `...-win-vulkan-arm64.zip` | same, tag b12110 | PASS |
| 16 | windows/amd64 | rocm | no | `...-win-vulkan-x64.zip` (rocm unused) | same | WARN (R4) |
| 17 | windows/amd64 | cpu | no | `...-win-cpu-x64.zip` | same, tag b12110 | PASS |
| 18 | windows/arm64 | cpu | no | `...-win-cpu-arm64.zip` | same, tag b12110 | PASS |
| 19 | linux/s390x | cpu | no | `...-ubuntu-arm64.tar.gz` (wrong arch) | `...-ubuntu-26.04-s390x.tar.gz` (by name order) | FAIL (R3) |

Notes on the table:

- The cudart asset for every CUDA row is the matching
  `cudart-...-cuda-<same version>-<same arch>`; Windows cudart names carry no
  tag (`cudart-llama-bin-win-cuda-12.4-x64.zip`), Linux ones do. Pairing is by
  suffix in `engine.go:174-185` and was verified in the probe output.
- `b12144` and `b12167` have the same 38 asset names as `b12110` after
  normalising the tag (checked from a07's `releases-100.json`), so every row
  above holds for them with the tag substituted. Only `b12110` is the release
  target (R2).
- Row 19 is out of scope for the beta build matrix but is included because it
  is the only failing case and it shows the fallback is order-dependent.

## 5. Sibling findings to merge at assembly time

Not available when this file was written (siblings had not committed). Fill in
from `agents/T06/a0*/`, `a09`-`a12` and `a14`/`a15`, then re-rank:

| source | expected finding | severity if confirmed | status |
|---|---|---|---|
| a01 | macOS Metal asset renames / xcframework | ? | pending |
| a02-a04 | CUDA 12/13 asset names, cudart pairing | ? | pending |
| a05 | Vulkan assets | ? | pending |
| a06 | HIP/ROCm assets, Windows HIP | ? | pending |
| a07 | CPU archives, runtime DLL/so sets | ? | pending |
| a08 | SYCL/OpenCL, Mac x64, checksum naming | ? | pending |
| a09 | `--jinja`, `-np`, `--cache-ram`, `--kv-unified` behaviour | ? | pending |
| a10 | reasoning flags/defaults | ? | pending |
| a11 | `/props`, `/slots`, `/v1/messages` schema | ? | pending |
| a12 | metrics/health endpoints | ? | pending |
| a13 | matrix test in `assets_matrix_test.go` | ? | pending |
| a14 | upgrade plan steps and rollback | ? | pending |
| a15 | `check_assets.py` | ? | pending |

## 6. Assembly checklist for the Lead

1. Decide the target: `b12110` (release-pinned) vs `b12144`/`b12167`
   (newest nightlies). Recommend `b12110` (R2).
2. Regenerate `pinned.go` from the b12110 release JSON; do not hand-edit
   digests (R1). Keep or drop the unused asset families deliberately (R4, R8).
3. Run the matrix test (a13) for every row in section 4, plus the s390x guard
   if a08 adds one (R3).
4. Re-read the a09-a12 flag/schema findings and append them to section 3
   before the report ships; section 3 currently has no blocker, and R8 says
   the flag surface is unchanged, so any blocker from a09-a12 would contradict
   R8 and should be double-checked.
5. Add the cleanup step for the old engine folder (R7) or accept the disk
   growth for the beta.
