# T06 a06: HIP / ROCm assets, b11960 to the latest release

Slice: HIP / ROCm (Linux, with the Windows HIP asset noted), from the pinned
build `b11960` to the latest release. Test file:
`internal/engine/assets_rocm_test.go`. Owner files: `internal/engine/engine.go`
and `internal/engine/pinned.go` (read only here, one comment added to
`engine.go`).

## What "latest release" means here

`GET /repos/ggml-org/llama.cpp/releases/latest` returns **v0.9.0**
(2026-11-04T13:08:06Z) with a single asset, `nightly-tag.txt`, which names
**b12110**. `LatestRelease()` in `engine.go:77-100` follows that pointer, so
the upgrade target for asset selection is **b12110**. The newest build by date
at capture time was **b12167** (2026-11-06T09:45:44Z); **b12144** is the build
the brief names. I tested b11960 and b12110, and kept b12167 as a third row so
the newest build is covered too.

## Asset inventory

Across the 117 builds from b11960 to b12167 the HIP/ROCm assets are:

| OS | Asset | Format | Arch |
|---|---|---|---|
| Linux | `llama-<tag>-bin-ubuntu-rocm-10.0-x64.tar.gz` | `.tar.gz` | x64 only |
| Windows | `llama-<tag>-bin-win-rocm-10.0-x64.zip` | `.zip` | x64 only |

- **No renames, no removals, no additions** in the range. The ROCm version
  stays `10.0` in the name, the archive formats stay `.tar.gz` on Linux and
  `.zip` on Windows, and no arm64 ROCm build exists.
- **No asset has `hip` in its name.** The HIP backend ships under the `rocm`
  label. The Windows ROCm zip is the Windows HIP asset.
- **No ROCm runtime archive** next to the build. CUDA gets `cudart-...`
  companions (`engine.go:169-186` appends them); ROCm has none, and
  `PickAssets` only looks for `cudart-`, so it would not append one if it
  appeared.
- **b12050** published only 30 assets at capture time and was missing
  `llama-b12050-bin-win-rocm-10.0-x64.zip`; the next build has it again. This
  looks like a mid-upload snapshot, not a dropped asset. Evidence file line for
  b12050 shows the gap.
- **Windows ROCm is not self-contained.** `.github/workflows/release.yml`
  `windows-rocm` packs only `ggml-hip.dll`, `amdhip64_7.dll`,
  `rocm_kpack.dll` and `amd_comgr.dll`; the release-publish merge injects
  `llama-server.exe` and the CPU backend from the windows-cpu zip. The job
  comment says rocBLAS/hipBLASLt kernels "resolve fine via PATH and are not
  copied", so the machine still needs a ROCm 10.0 install for the BLAS
  kernels. The Linux `ubuntu-24-rocm` job tars `./build/bin` only, with
  `CMAKE_INSTALL_RPATH=$ORIGIN`, so it needs the system ROCm 10.0 libraries
  too.
- `pinned.go:25` and `pinned.go:39` already carry the b11960 Linux and Windows
  ROCm assets (with sizes and digests), but `PickAssets` can never return
  them, see the finding below.

## Findings

### F1 (major, behavioural, not a break): EZLM never selects the ROCm build

`assetPatterns` maps `scan.ROCm` to the Vulkan patterns on both platforms:

- `internal/engine/engine.go:126-127` (Linux): `{vulkan-x64}`, then plain
  `ubuntu-x64`.
- `internal/engine/engine.go:136-137` (Windows): `{win-vulkan-x64}`, then
  `win-cpu-x64`.

So `EZLM_BACKEND=rocm` installs the Vulkan build, and the ROCm archives in
`pinned.go:25,39` are dead weight. Evidence: `TestPickAssetsROCm` passes today
with `llama-<tag>-bin-ubuntu-vulkan-x64.tar.gz` / `...-win-vulkan-x64.zip`
for b11960, b12110 and b12167, and `TestPickAssetsNeverPicksROCm` proves no
combination returns a `-rocm-` asset.

This looks deliberate, not a bug: `internal/scan/scan.go:138` comments that
Vulkan is "the most reliable path on consumer AMD", `BETA.md:44` expects AMD
machines to report the Vulkan engine, and the ROCm archives need a system
ROCm 10.0 runtime (see above) that EZLM does not check. `EZLM_BACKEND` is
documented for `cpu/vulkan/cuda/metal` only (`scan.go:127-128`). The risk is
that a future plan reads the ROCm assets in `pinned.go` as supported and
switches the pattern without adding a ROCm runtime check. The tests pin the
current choice so that switch has to be deliberate.

Severity: major as a documentation/expectation gap, not a blocker for the
upgrade. **No selection change made.**

### F2 (minor): no test covered ROCm at all

`internal/engine/engine_test.go` has no ROCm row in `TestPickAssets` and no
coverage of any tag newer than b11960, so a rename of the ROCm assets would
not have failed a test. Fixed by the new test file.

### F3 (minor): pinned.go ROCm entries are unreachable

`pinned.go` is generated from the b11960 release and keeps both ROCm assets
(251 MB and 265 MB) that nothing can select. Harmless (metadata only, the
archives are not committed), but it inflates the offline fallback list and
suggests support that does not exist. Left as is; the pin must not change in
this task.

## Changes

1. **New** `internal/engine/assets_rocm_test.go`, table-driven, no network:
   - `TestPickAssetsROCm`: b11960, b12110, b12167 x Linux/Windows x
     amd64/arm64, `scan.ROCm`, expects the Vulkan asset name.
   - `TestROCmAssetNames`: asserts exactly one ROCm asset per OS per tag, the
     expected name and archive format, and no arm64/HIP-named variant.
   - `TestPickAssetsNeverPicksROCm`: every OS/arch/backend combination in
     those releases never returns a `-rocm-` asset.
   - `TestPickAssetsROCmFallbackWithoutVulkan`: with Vulkan removed, ROCm
     falls back to the plain CPU build, not the ROCm archive.
2. **Comment only** in `internal/engine/engine.go:127-130`: says why the ROCm
   backend uses the Vulkan patterns. No behaviour change; `PickAssets` and
   `assetPatterns` are untouched otherwise.

## Test matrix

| Platform | Tag | Backend | Expected asset | Status |
|---|---|---|---|---|
| linux/amd64 | b11960 | rocm | `llama-b11960-bin-ubuntu-vulkan-x64.tar.gz` | pass |
| linux/amd64 | b12110 | rocm | `llama-b12110-bin-ubuntu-vulkan-x64.tar.gz` | pass |
| linux/amd64 | b12167 | rocm | `llama-b12167-bin-ubuntu-vulkan-x64.tar.gz` | pass |
| linux/arm64 | b11960 | rocm | `llama-b11960-bin-ubuntu-vulkan-arm64.tar.gz` | pass |
| linux/arm64 | b12110 | rocm | `llama-b12110-bin-ubuntu-vulkan-arm64.tar.gz` | pass |
| linux/arm64 | b12167 | rocm | `llama-b12167-bin-ubuntu-vulkan-arm64.tar.gz` | pass |
| windows/amd64 | b11960 | rocm | `llama-b11960-bin-win-vulkan-x64.zip` | pass |
| windows/amd64 | b12110 | rocm | `llama-b12110-bin-win-vulkan-x64.zip` | pass |
| windows/amd64 | b12167 | rocm | `llama-b12167-bin-win-vulkan-x64.zip` | pass |
| windows/arm64 | b11960 | rocm | `llama-b11960-bin-win-vulkan-arm64.zip` | pass |
| windows/arm64 | b12110 | rocm | `llama-b12110-bin-win-vulkan-arm64.zip` | pass |
| windows/arm64 | b12167 | rocm | `llama-b12167-bin-win-vulkan-arm64.zip` | pass |

The ROCm archives themselves (`...-ubuntu-rocm-10.0-x64.tar.gz`,
`...-win-rocm-10.0-x64.zip`) exist at every tag in the table but are never
picked; `TestROCmAssetNames` checks they are present and correctly named.

## Evidence

- `evidence/rocm-assets-by-release.txt`: every build b11960..b12167 with its
  published ROCm asset names and asset count. Captured from
  `releases?per_page=100` pages 1 and 2 on 2026-11-06.
- `evidence/rocm-assets-b11960-b12110.json`: the ROCm assets for the pinned
  build and the upgrade target with size, digest and download URL, straight
  from `releases/tags/<tag>`.
- `evidence/notes.md`: workflow lines, the nightly-tag pointer, and how to
  reproduce.

## Not finished / open questions

- The GitHub API rate limit (60/hour per address, shared) was hit after the
  tag calls, so I could not re-fetch b12167's per-asset digests; the listing
  data above was already captured. b12144 has the same two ROCm assets per
  the listing.
- Archive contents were not downloaded (task rule), so the "needs system
  ROCm 10.0" conclusion comes from the release workflows, not from unpacking
  the archives. If the upgrade plan wants ROCm as a selectable backend, check
  the archives and add a ROCm runtime check before changing the pattern.
- No Windows arm64 ROCm build exists at any tag in the range; a Windows arm64
  AMD machine always gets Vulkan, which is what the code does.
- `pinned.go` will need the b12110 (or b12167) asset list regenerated when the
  pin moves; that is T06 a14's plan, not this slice.
