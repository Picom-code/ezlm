# T06 / a01: macOS Metal assets (arm64 and x64)

Slice: every llama.cpp release asset for macOS Metal from the pinned `b11960` to the
latest release, mapped to what EZLM's asset selection returns.

## What I did

1. Resolved "latest release" the way EZLM does: `releases/latest` is `v0.9.0`
   (2026-11-04), which carries only `nightly-tag.txt` (7 bytes, `b12110`), so the
   build EZLM would install is **b12110**. The newest build by date is **b12167**;
   its macOS names are recorded too.
2. Listed the macOS assets at b11960, b12110 and b12167 (GET only, no archive
   downloaded). Names and evidence are in `macos-metal-assets.md`.
3. Added `internal/engine/assets_metal_test.go`: table tests over both tags and both
   architectures, plus the install-folder flavor and the missing-asset error path.
4. Found and fixed one selection bug (finding 1 below).

## Findings

### 1. `darwin` + CUDA + cuda13 matched an Android build (minor, fixed)

`internal/engine/engine.go:112` built the CUDA-13 preference from
`map[string]string{"linux": ..., "windows": ...}[goos]`. For any other `goos` the
lookup yields `""`, so the pattern became `{"", "arm64"}`, which matches every
`arm64` `.tar.gz` in the release. For `darwin/arm64` with `scan.CUDA` and
`cuda13=true`, `PickAssets` returned
`llama-b12110-bin-android-arm64-snapdragon.tar.gz` instead of
`llama-b12110-bin-macos-arm64.tar.gz`. Evidence, before the fix:

```
--- FAIL: TestPickAssetsMacOSMetal (0.00s)
    assets_metal_test.go:132: b12110 darwin/arm64 cuda:
        got "llama-b12110-bin-android-arm64-snapdragon.tar.gz",
        want "llama-b12110-bin-macos-arm64.tar.gz"
```

Severity: minor. It is not reachable through `Install` today, because on a Mac
`CUDAMax` is empty and `Install` rewrites CUDA to Vulkan at
`internal/engine/engine.go:215`. It is reachable through the exported
`PickAssets`, through `EZLM_BACKEND=cuda` (`internal/scan/scan.go:130`) on a machine
that also reports a CUDA version, and by any future caller that passes `darwin` with
CUDA. Unknown `goos` values had the same defect.

Fix (2 lines, `internal/engine/engine.go:110-121`): the CUDA-13 rule now applies
only when the OS has a CUDA prefix, otherwise selection falls through to the
per-OS rules. Behaviour for Linux, Windows and every non-CUDA case is unchanged.

### 2. macOS asset names did not change between the two tags (info)

`llama-<tag>-bin-macos-arm64.tar.gz` and `llama-<tag>-bin-macos-x64.tar.gz` exist
at b11960, b12110 and b12167. Same archive format (`.tar.gz`), no rename, no
removal, no new macOS-specific asset. An upgrade needs no `pinned.go` change for
macOS beyond the tag itself. Sizes grew about 1.5% (arm64 16465074 to 16650691,
x64 12205764 to 12388601); both are still small.

### 3. Two new assets since b11960 could collide, but do not (info)

b12110 adds `llama-b12110-ui.tar.gz` and `llama-b12110-xcframework.zip`. Neither
starts with `llama-<tag>-bin-`, and the darwin pattern is `-bin-macos-<arch>`, so
neither can be selected. `hasBinaries` (`engine.go:98`) still sees the `-bin-`
macOS assets, so the `v0.9.0` to `b12110` resolution keeps working.

### 4. Every arm64 Mac computes `cuda13=true` and a Vulkan backend (minor, not fixed)

`Install` sets `cuda13 := prereq.NeedsCUDA13(hw) || runtime.GOARCH == "arm64"`
(`engine.go:218`) and then, because `CUDAMax` is empty on macOS, rewrites
`backend` to `scan.Vulkan` (`engine.go:219-221`). macOS selection only still works
because `assetPatterns` ignores the backend for `darwin`. That is correct today and
now pinned by a test (the `scan.Vulkan` rows in `TestPickAssetsMacOSMetal`), but it
is accidental: anyone making the darwin rule backend-sensitive would install a
Vulkan-named build or fail. Worth a comment or an early darwin return in `Install`;
left alone because it is outside the selection-only scope of this slice.

### 5. Unknown architecture falls back to any macOS archive (minor, not fixed)

`a := map[string]string{"amd64": "x64", "arm64": "arm64"}[arch]` (`engine.go:111`)
is `""` for any other architecture, so `darwin` with, say, `riscv64` would match
`llama-<tag>-bin-macos-arm64.tar.gz`. No such macOS target exists today. Open
question: return an error for an unsupported arch instead of an empty substring.

## What I changed

- `internal/engine/engine.go`: the CUDA-13 branch is guarded on a non-empty OS
  prefix (finding 1). No other selection change; the pinned tag is untouched.
- `internal/engine/assets_metal_test.go` (new): `TestPickAssetsMacOSMetal`,
  `TestMacOSAssetsExistAtBothTags`, `TestMacOSFlavorPerTagAndArch`,
  `TestPickAssetsMacOSMissing`. The fixtures are the real asset lists for b11960 and
  b12110, so a rename upstream shows up as a test failure once the fixture is
  refreshed.

## Checks

`gofmt -l .` empty, `go build ./...`, `go vet ./...`,
`go test -p 2 ./internal/engine/` pass on this worktree (darwin/arm64).

## Not finished / next steps

- b12167 sizes and digests were not read: `api.github.com` was rate-limited
  (60/hour shared, exhausted 12:47 UTC 2026-11-06). Only names were recovered, from
  the release page HTML. Re-read later if a14 needs them.
- Archive *contents* were not inspected (no downloads). The extractor's
  `llama-server` lookup and dylib handling are T09's area; the macOS archive has
  kept the same layout by name only, unverified.
- a13's matrix test should carry the two darwin rows from
  `TestPickAssetsMacOSMetal` and reference this file rather than duplicate the
  fixtures.
- a14's upgrade plan needs no macOS asset work: changing the tag is enough. Note
  that "latest release" resolves to b12110, not to the newest build b12167.
