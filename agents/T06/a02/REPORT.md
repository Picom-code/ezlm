# T06 a02: Windows CUDA 12 x64 assets

Slice: every Windows x64 CUDA 12 release asset from the pinned build to the latest
release, and the asset-selection behaviour that maps to them.

## Result

No rename, no removal, no archive-format change. Windows x64 CUDA 12 is
`llama-<tag>-bin-win-cuda-12.4-x64.zip` plus the untagged runtime archive
`cudart-llama-bin-win-cuda-12.4-x64.zip` at every build checked. Selection is
correct at both tags. One latent fallback bug was found and fixed (below).

## Tags used

| Role | Tag | Source |
|---|---|---|
| Pinned | `b11960` (2026-10-28) | `internal/engine/pinned.go`, re-checked against the API |
| Latest release | `v0.9.0` (2026-11-04) | `releases/latest`; it has no binaries, only `nightly-tag.txt` |
| Build the latest release points at | `b12110` (2026-11-04) | `nightly-tag.txt` of v0.9.0 |
| Newest build by date | `b12144` | noted in the brief; exists (HEAD 200), but v0.9.0 does not point at it |

So "the latest release" in EZLM terms (`LatestRelease` follows `nightly-tag.txt`)
is **b12110**. `b12144` is newer than the release but is not what the release
serves; a pin bump to "latest" today lands on b12110.

## Asset map

| Asset | b11960 | b12110 | Change |
|---|---|---|---|
| CUDA 12 x64 build | `llama-b11960-bin-win-cuda-12.4-x64.zip` (279,763,224 B) | `llama-b12110-bin-win-cuda-12.4-x64.zip` (287,014,657 B) | tag only; +2.6% size |
| CUDA 12 x64 runtime | `cudart-llama-bin-win-cuda-12.4-x64.zip` (391,443,627 B) | same name, same size | none |
| Archive format | `.zip` | `.zip` | none |

The runtime archive deliberately carries no build number on Windows (Linux ones
do: `cudart-llama-b11960-bin-ubuntu-...`), so `PickAssets` matches it by suffix
`-cuda-12.4-x64`. That still works: the suffix is unchanged.

Other Windows assets present at b12110 and absent at b11960 (not this slice,
for a03/a05/a07/a08): `llama-b12110-bin-win-opencl-adreno-arm64.zip`,
`...-win-rocm-10.0-x64.zip`, `...-win-sycl-x64.zip`,
`...-win-openvino-2026.4.1-x64.zip`. Windows arm64 CUDA 12 still does not
exist; arm64 is CUDA 13 only, as `Install` already assumes.

## Finding: CUDA 12 machine can be given a CUDA 13 build

**Severity: major (latent).** `internal/engine/engine.go:133`, Windows CUDA
chain before the fix:

```go
return [][]string{{"-bin-win-cuda-12", a}, {"-bin-win-cuda-", a}, {"-bin-win-vulkan-" + a}, {"-bin-win-cpu-" + a}}
```

`cuda13` is false exactly when the driver is older than 13.0
(`cudaOK(hw.CUDAMax, "13.0")` in `Install`). If a release has no CUDA 12 build,
the second pattern silently selects the CUDA 13 archive, which needs a 13.0
driver and will not start. Vulkan is the correct fallback and the third pattern
never gets a chance.

Reproduction (`TestPickAssetsWindowsCUDA12FallbackIsVulkan`, a release with
only cuda-13.4 and vulkan x64):

```
got [llama-b99999-bin-win-cuda-13.4-x64.zip cudart-llama-bin-win-cuda-13.4-x64.zip], want the Vulkan archive
```

Fix: dropped the generic `-bin-win-cuda-` pattern from the non-CUDA-13 Windows
chain, with a one-line comment. The CUDA 13 chain is chosen earlier by
`cuda13`, so the pattern was redundant there.

Not triggered by b11960 or b12110 (both publish CUDA 12.4 x64), so this is a
guard against the next upstream drop, not a break in the upgrade itself.

Same shape exists for Linux at `internal/engine/engine.go:124`
(`{"-bin-ubuntu-cuda-", a}`). Left alone: Linux CUDA is a04's slice. It has the
same failure mode and the same one-line fix.

## Tests

`internal/engine/assets_wincuda_test.go`:

| Test | Covers |
|---|---|
| `TestPickAssetsWindowsCUDA12X64/b11960` | exact pair for the pinned release, names exist, runtime is the Windows archive, size/URL present |
| `TestPickAssetsWindowsCUDA12X64/b12110 (latest release v0.9.0)` | same for the real b12110 Windows asset list |
| `TestPickAssetsWindowsCUDA12FallbackIsVulkan` | no CUDA 12 published: Vulkan, never CUDA 13 (failed before the fix) |

The b12110 list is the published Windows asset set from the tag API, so a
selection that invents a name fails the "is a real asset" check.

## Evidence (`agents/T06/a02/`)

- `latest.json`: `releases/latest` (v0.9.0, assets: `nightly-tag.txt` only).
- `nightly-tag.txt`: contents `b12110`.
- `latest-build.json`: full asset list for b12110.
- `b11960.json`: full asset list for the pinned tag.
- `releases-100.json`: rate-limit response, kept as evidence that the list
  endpoint was exhausted (60/hour shared by all 16 agents).

## Method and limits

- GET/HEAD only; no archive downloaded.
- Intermediate tags sampled by HEAD (cheap, not rate-limited):
  b11960, b12000, b12020, b12040, b12060, b12080, b12100, b12110, b12144 all
  return 200 for both Windows CUDA 12 x64 archives. b11975 and b11990 return
  404 for every asset (CPU too), so they are skipped build numbers, not
  renames.
- The full release list (`releases?per_page=100`) could not be fetched: the
  shared IP hit GitHub's 60/hour API limit after three calls. Renames between
  sampled tags are therefore checked by HEAD probe, not by reading every
  release's asset list.

## Checks

`gofmt -l .` empty, `go build ./...`, `go vet ./...`,
`go test -p 2 ./internal/engine/` pass in the worktree.

## Next steps

- a04: apply the same fallback fix to the Linux CUDA chain.
- a13: index this case in the matrix (`windows/amd64/CUDA/cuda13=false`).
- a14: when pinning, use the tag from `nightly-tag.txt` (b12110 today), not
  `b12144`; asset names need no change.
