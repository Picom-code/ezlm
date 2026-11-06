# T06 / a13: asset test matrix

Owner: agent a13. Branch `agent/T06-a13`. Worktree `~/ezlm-agents/work/subs/T06/a13`.

## What I did

- Read `internal/engine/pinned.go` and `internal/engine/engine.go` (selection:
  `assetPatterns`, `matches`, `cudaFlavor`, `PickAssets`).
- Pulled the real asset lists (GET only) for the pinned build **b11960** and for
  **releases/latest**. `releases/latest` is **v0.9.0** (2026-11-04), which carries no
  binaries: its `nightly-tag.txt` says **b12110**, so that is the release the upgrade
  would actually install. Full name/size/digest snapshot in
  `agents/T06/a13/release-b12110.json`.
- Added `internal/engine/assets_matrix_test.go`: one table (`assetMatrix`, 21 rows)
  covering every OS/arch/backend combination `PickAssets` can express, run against
  both tags. It is the index for the platform agents: if a per-platform file already
  covers a case, this row is the canonical expectation to point at.
- Added three tests around that table:
  - `TestAssetMatrix`: exact picked names (server archive, plus matching cudart for
    CUDA), for b11960 and b12110.
  - `TestAssetMatrixCoversEveryReleaseAsset`: every published archive is either
    selected by a row or listed in `unselectedFlavors` with a reason, so a renamed or
    newly published asset fails here first.
  - `TestAssetMatrixCUDAHasRuntime`: every CUDA row gets exactly one cudart archive
    whose `-cuda-<ver>-<arch>` flavor matches the build.
- No change to `engine.go` or `pinned.go`; the current selection already returns the
  right archive for every row in both releases.

## Findings

### F1 (info) No renamed or removed assets between b11960 and b12110

Evidence: asset-name diff of the two releases (34 vs 38 assets). Additions only:

| added in b12110 | selected by EZLM? |
|---|---|
| `llama-b12110-bin-ubuntu-26.04-s390x.tar.gz` | no, no s390x target |
| `llama-b12110-bin-ubuntu-zdnn-s390x.tar.gz` | no, no s390x target |
| `llama-b12110-ui.tar.gz` | no, web UI bundle |
| `llama-b12110-xcframework.zip` | no, Apple framework bundle |

No archive-format change either: Windows still `.zip`, macOS/Linux still `.tar.gz`,
cudart naming unchanged (`cudart-llama-<tag>-bin-...` on Linux,
`cudart-llama-bin-...` on Windows). This is the "no surprises" result for the
upgrade.

### F2 (minor, pre-existing) ROCm archives are published but never selected

`internal/engine/engine.go:125` (linux) and `:134` (windows) send `scan.ROCm` down
the Vulkan path, so `llama-<tag>-bin-ubuntu-rocm-10.0-x64.tar.gz` and
`llama-<tag>-bin-win-rocm-10.0-x64.zip` are never picked, even though they exist in
both b11960 and b12110. The matrix rows `linux-x64-rocm` and `windows-x64-rocm`
assert the current Vulkan result and `unselectedFlavors` records why. `scan` never
assigns ROCm on its own (`internal/scan/scan.go:138` picks Vulkan for AMD), so ROCm
is only reachable through a hardware override (`scan.go:130`). Whether to prefer the
ROCm build is a06's call; if it changes, only those two rows and the two
`unselectedFlavors` entries change.

### F3 (info) arm64 CUDA 12 request falls back to CUDA 13

There is no CUDA 12 arm64 build in either release, so `PickAssets(..., cuda13=false)`
on arm64 returns the CUDA 13 archive. Not reachable in practice: `Install` sets
`cuda13` true for arm64 (`engine.go:218`). Rows `linux-arm64-cuda12` and
`windows-arm64-cuda12` pin the fallback so a future CUDA 12 arm64 build shows up as a
test change, not a silent switch.

### F4 (note for a14) The upgrade target resolves to b12110, not b12144

`LatestRelease` follows v0.9.0 -> `nightly-tag.txt` -> b12110. The brief mentions
b12144 as the newest build by date; I could not confirm it because the GitHub API
started returning 403 rate-limit responses at 12:47Z (shared address). The plan should
name b12110 as the target tag and note that `nightly-tag.txt` can move before the pin
is updated. All 38 b12110 assets carry a `sha256:` digest, so `verifyDigest` keeps
working after the upgrade.

## Test matrix

`<tag>` is elided from archive names below; every name above is prefixed with
`llama-<tag>-bin-` (and `cudart:` means `cudart-llama-<tag>-bin-` on Linux,
`cudart-llama-bin-` on Windows). Both the b11960 and b12110 columns are asserted;
they are identical.

| row | goos/arch | backend | cuda13 | picked (both tags) | status |
|---|---|---|---|---|---|
| macos-arm64-metal | darwin/arm64 | Metal | false | macos-arm64.tar.gz | ok |
| macos-x64-metal | darwin/amd64 | Metal | false | macos-x64.tar.gz | ok |
| macos-x64-cpu | darwin/amd64 | CPU | false | macos-x64.tar.gz | ok |
| linux-x64-cuda12 | linux/amd64 | CUDA | false | ubuntu-cuda-12.8-x64.tar.gz + cudart: ubuntu-cuda-12.8-x64.tar.gz | ok |
| linux-x64-cuda13 | linux/amd64 | CUDA | true | ubuntu-cuda-13.4-x64.tar.gz + cudart: ubuntu-cuda-13.4-x64.tar.gz | ok |
| linux-x64-vulkan | linux/amd64 | Vulkan | false | ubuntu-vulkan-x64.tar.gz | ok |
| linux-x64-rocm | linux/amd64 | ROCm | false | ubuntu-vulkan-x64.tar.gz | ok (F2) |
| linux-x64-cpu | linux/amd64 | CPU | false | ubuntu-x64.tar.gz | ok |
| linux-arm64-cuda13 | linux/arm64 | CUDA | true | ubuntu-cuda-13.4-arm64.tar.gz + cudart: ubuntu-cuda-13.4-arm64.tar.gz | ok |
| linux-arm64-cuda12 | linux/arm64 | CUDA | false | ubuntu-cuda-13.4-arm64.tar.gz + cudart: ubuntu-cuda-13.4-arm64.tar.gz | ok (F3) |
| linux-arm64-vulkan | linux/arm64 | Vulkan | false | ubuntu-vulkan-arm64.tar.gz | ok |
| linux-arm64-cpu | linux/arm64 | CPU | false | ubuntu-arm64.tar.gz | ok |
| windows-x64-cuda12 | windows/amd64 | CUDA | false | win-cuda-12.4-x64.zip + cudart: win-cuda-12.4-x64.zip | ok |
| windows-x64-cuda13 | windows/amd64 | CUDA | true | win-cuda-13.4-x64.zip + cudart: win-cuda-13.4-x64.zip | ok |
| windows-x64-vulkan | windows/amd64 | Vulkan | false | win-vulkan-x64.zip | ok |
| windows-x64-rocm | windows/amd64 | ROCm | false | win-vulkan-x64.zip | ok (F2) |
| windows-x64-cpu | windows/amd64 | CPU | false | win-cpu-x64.zip | ok |
| windows-arm64-cuda13 | windows/arm64 | CUDA | true | win-cuda-13.4-arm64.zip + cudart: win-cuda-13.4-arm64.zip | ok |
| windows-arm64-cuda12 | windows/arm64 | CUDA | false | win-cuda-13.4-arm64.zip + cudart: win-cuda-13.4-arm64.zip | ok (F3) |
| windows-arm64-vulkan | windows/arm64 | Vulkan | false | win-vulkan-arm64.zip | ok |
| windows-arm64-cpu | windows/arm64 | CPU | false | win-cpu-arm64.zip | ok |

Archives published but deliberately not selectable (`unselectedFlavors`): android
arm64 and snapdragon, linux arm64 snapdragon, ubuntu s390x / 26.04-s390x /
zdnn-s390x, openvino (linux and windows), sycl (linux fp16/fp32 and windows), win
opencl adreno, rocm (linux and windows), `ui.tar.gz`, `xcframework.zip`.

## Verification

- `gofmt -l .` empty, `go build ./...`, `go vet ./internal/engine/`,
  `go test -p 2 ./internal/engine/` pass in the worktree.
- Network: two GitHub API GETs plus one small `nightly-tag.txt` GET. No archive
  downloaded, no model loaded, no server started.

## Open questions / next steps

- a06: decide whether `scan.ROCm` should pick the ROCm archives (F2). If yes, change
  `assetPatterns` and the two ROCm rows here.
- a14: target b12110 (F4), and re-check `nightly-tag.txt` on the day the pin moves.
- a15: reuse `assetMatrix` rows (or the same table) so the script and the test cannot
  disagree.
- a16: the matrix table above is the platform x tag x asset x status table it needs.
- b12144 could not be inspected because of the API rate limit; if the lead needs it,
  retry after the hourly reset.
