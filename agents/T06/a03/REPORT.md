# T06 / a03: Windows CUDA 12 arm64 and CUDA 13 variants

Scope: every llama.cpp release asset for **Windows CUDA** (arm64 and CUDA 13) from the pinned
`b11960` to the latest release, and the asset selection EZLM does for those machines.

## Result in one line

Nothing to rename or remove. The Windows CUDA asset names are identical at every release from
`b11960` to `b12167`, there is **no CUDA 12 build for Windows arm64 anywhere**, and EZLM's existing
selection already picks the right archive and the matching `cudart` for each case.

## What I did

1. Read `internal/engine/engine.go` (`assetPatterns`, `PickAssets`, `flavor`) and `pinned.go`.
2. Listed the Windows CUDA assets of `b11960` (pin), `b12110` (the build `v0.9.0` points at),
   `b12144` (newest build when the brief was written) and `b12167` (newest at fetch time):
   `agents/T06/a03/assets-windows-cuda.tsv`.
3. Scanned **every** release in the window, 119 releases from `b11960` (2026-10-28) to `b12167`
   (2026-11-06), for renamed, added or removed Windows CUDA assets:
   `agents/T06/a03/range-scan.txt`.
4. Added `internal/engine/assets_winarm_test.go`: table tests for the pinned tag and both later
   builds, plus the "no CUDA 12 arm64" gap and the Vulkan fallback.
5. `go test -p 2 ./internal/engine/`, `gofmt -l .`, `go build ./...`, `go vet ./...` all pass.
   No change to `engine.go`, `pinned.go` or the pinned tag was needed.

## Asset inventory (unchanged across all 119 releases)

| Asset | Format | Arch | CUDA | b11960 | b12110 | b12167 |
|---|---|---|---|---|---|---|
| `llama-<tag>-bin-win-cuda-12.4-x64.zip` | zip | x64 | 12.4 | 279.8 MB | 287.0 MB | 288.7 MB |
| `llama-<tag>-bin-win-cuda-13.4-arm64.zip` | zip | arm64 | 13.4 | 152.7 MB | 156.8 MB | 158.0 MB |
| `llama-<tag>-bin-win-cuda-13.4-x64.zip` | zip | x64 | 13.4 | 165.0 MB | 169.2 MB | 170.4 MB |
| `cudart-llama-bin-win-cuda-12.4-x64.zip` | zip | x64 | 12.4 | 391.4 MB | same | same |
| `cudart-llama-bin-win-cuda-13.4-arm64.zip` | zip | arm64 | 13.4 | 153.3 MB | same | same |
| `cudart-llama-bin-win-cuda-13.4-x64.zip` | zip | x64 | 13.4 | 423.5 MB | same | same |

- `cudart` archives carry **no tag** in their name and are byte-identical (same size and sha256)
  in every release that publishes them. The engine already matches them by suffix, not by tag.
- Every Windows CUDA archive, `llama` and `cudart`, is `.zip`. No format change in the window.
- The latest stable release `v0.9.0` (2026-11-04) has **no binaries**, only `nightly-tag.txt`,
  which names `b12110`. So "the latest release" for asset purposes is `b12110`, not `b12144`/`b12167`.

## Findings

### 1. Windows arm64 has no CUDA 12 build (informational, no action)

`llama-b11960-bin-win-cuda-12.4-x64.zip` exists but there is no `...-cuda-12.4-arm64.zip`, at any
tag. Windows arm64 CUDA is CUDA 13.4 only. Evidence: `range-scan.txt` ("matches: none") and
`assets-windows-cuda.tsv`.

EZLM already copes: `internal/engine/engine.go:218` sets `cuda13 = prereq.NeedsCUDA13(hw) ||
runtime.GOARCH == "arm64"`, so arm64 asks for the CUDA 13 pattern first. Covered by
`TestPickAssetsWindowsCUDA/arm64_cuda13`.

Note for the upgrade plan: even if that line lost the `arm64` clause, selection would still land on
the CUDA 13.4 arm64 archive through the generic `-bin-win-cuda-` fallback
(`internal/engine/engine.go:133`). The clause only matters for the driver check at
`internal/engine/engine.go:219`: without it, an arm64 machine whose driver reports less than CUDA 13
would get a CUDA 13 archive instead of falling back to Vulkan.

### 2. `cuda13=false` can silently return a CUDA 13 archive (minor, latent, cross-slice)

`assetPatterns` for Windows CUDA with `cuda13=false` is
`{"-bin-win-cuda-12", a}, {"-bin-win-cuda-", a}, {"-bin-win-vulkan-" + a}, {"-bin-win-cpu-" + a}`
(`internal/engine/engine.go:133`). The second pattern matches **any** CUDA version, including
`cuda-13.4`. Today `cuda-12.4-x64` exists at every tag, so on x64 the first pattern always wins and
the bug cannot fire. It would fire the day a release drops CUDA 12 builds: a machine whose driver
caps at CUDA 12 would be given a CUDA 13 archive that cannot start, instead of the Vulkan archive
that is next in the list.

On arm64 this behaviour is harmless and is in fact what makes arm64 work (finding 1), so I did not
change the shared pattern list. Owner: a02 (Windows CUDA 12 x64) / a14 (upgrade plan). My test
`arm64 without cuda13 flag` pins the current arm64 behaviour so a fix for x64 does not break arm64.

### 3. `cuda13=true` has no CPU fallback (minor, latent)

`internal/engine/engine.go:116` returns only `{cuda-13 pattern}, {vulkan pattern}` for the CUDA 13
path, while the CUDA 12 path (`engine.go:133`) ends with a `-bin-win-cpu-` fallback. If a release
ever shipped neither a CUDA 13 nor a Vulkan archive for the machine, install would fail with
"no llama.cpp build" instead of using the CPU build. All 117 releases in the window ship both, so
this is not a live break. Owner: a07 (CPU builds) / a14.

### 4. `cudart` selection is correct for arm64 and does not cross architectures (informational)

`PickAssets` pairs the runtime by taking `cudaFlavor` from the `llama` archive and matching a
`cudart-` asset with the same OS part and the same suffix (`internal/engine/engine.go:174-184`).
For `cuda-13.4-arm64` that yields exactly `cudart-llama-bin-win-cuda-13.4-arm64.zip` and never the
x64 one. `TestPickAssetsWindowsCUDA` asserts the exact pair, including the architecture suffix, for
all three tags. This matters because the arm64 `cudart` is 153 MB against 423 MB for x64: picking
the wrong one would ship x64 DLLs to an arm64 install.

### 5. The download cache name for `cudart` is shared across tags (informational)

Because `cudart` names carry no tag, `internal/engine/engine.go:241` writes
`paths.Bin()/cudart-llama-bin-win-cuda-13.4-arm64.zip` for every tag. That is safe today (the bytes
are identical) and the digest is still verified (`engine.go:250`). If a future release ever
republishes a changed `cudart` under the same name, a stale copy cannot slip through the digest
check. No action.

## What changed

- Added `internal/engine/assets_winarm_test.go` (tests only).
- No production code change. `internal/engine/pinned.go` untouched; the pin stays `b11960`.

## Test matrix

| Platform | Tag | cuda13 | Expected asset | cudart | Status |
|---|---|---|---|---|---|
| windows/arm64 | b11960 | true | `llama-b11960-bin-win-cuda-13.4-arm64.zip` | `cudart-llama-bin-win-cuda-13.4-arm64.zip` | pass |
| windows/arm64 | b11960 | false | `llama-b11960-bin-win-cuda-13.4-arm64.zip` (fallback) | arm64 | pass |
| windows/arm64 | b12110 | true | `llama-b12110-bin-win-cuda-13.4-arm64.zip` | arm64 | pass |
| windows/arm64 | b12110 | false | `llama-b12110-bin-win-cuda-13.4-arm64.zip` (fallback) | arm64 | pass |
| windows/arm64 | b12167 | true | `llama-b12167-bin-win-cuda-13.4-arm64.zip` | arm64 | pass |
| windows/arm64 | b12167 | false | `llama-b12167-bin-win-cuda-13.4-arm64.zip` (fallback) | arm64 | pass |
| windows/amd64 | b11960 | true | `llama-b11960-bin-win-cuda-13.4-x64.zip` | x64 | pass |
| windows/amd64 | b12110 | true | `llama-b12110-bin-win-cuda-13.4-x64.zip` | x64 | pass |
| windows/amd64 | b12167 | true | `llama-b12167-bin-win-cuda-13.4-x64.zip` | x64 | pass |
| windows/amd64 | b11960 | false | `llama-b11960-bin-win-cuda-12.4-x64.zip` | x64 | pass |
| windows/amd64 | b12110 | false | `llama-b12110-bin-win-cuda-12.4-x64.zip` | x64 | pass |
| windows/amd64 | b12167 | false | `llama-b12167-bin-win-cuda-12.4-x64.zip` | x64 | pass |
| windows/arm64 | b12110 | true, no CUDA 13 arm64 asset | `llama-b12110-bin-win-vulkan-arm64.zip` | none | pass |

Commands run in the worktree:

```sh
gofmt -l .                                  # empty
go build ./...
go vet ./...
go test -p 2 ./internal/engine/
```

## Open questions

- Does EZLM support Windows arm64 at all today, or is the arm64 CUDA path only reachable from a
  future Windows-on-ARM build? The selection code handles it, but the Windows app's packaging
  (`windows/EZLM`) was out of scope here.
- `prereq.NeedsCUDA13` matches GPU names by substring (`rtx 50`, `spark`, `gb10`, ...). A future
  GB10-class name that does not contain one of those strings would be sent to the CUDA 12 archive on
  x64, which does not carry Blackwell kernels. Worth a check in the upgrade plan (a14), not here.

## Next steps

- a14: record that "latest release" resolves through `nightly-tag.txt` to `b12110`, and that
  Windows arm64 CUDA is 13.4-only, so an arm64 machine with a CUDA 12 driver gets Vulkan.
- a02/a14: decide whether the generic `-bin-win-cuda-` fallback should exclude CUDA 13 when
  `cuda13=false`, or whether the Vulkan fallback should come first.
