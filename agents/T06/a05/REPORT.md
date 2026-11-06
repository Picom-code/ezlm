# T06 / a05: Vulkan assets (Windows and Linux)

Slice: every llama.cpp release asset for the Vulkan backend, from the pinned
`b11960` to the latest release, mapped against what `internal/engine/engine.go`
picks.

## What I did

- Read `internal/engine/pinned.go` (b11960 asset snapshot) and the asset
  selection in `internal/engine/engine.go` (`assetPatterns`, `matches`,
  `PickAssets`).
- Listed the assets of every llama.cpp release from `b11838` to `b12167` with
  the GitHub releases API (GET only, two pages of `?per_page=100`).
- Resolved what `LatestRelease` actually returns today: `releases/latest` is
  `v0.9.0` (2026-11-04), which carries no binaries and only `nightly-tag.txt`;
  that file contains `b12110`.
- Added `internal/engine/assets_vulkan_test.go`: table tests that assert the
  exact archive name `PickAssets` returns for Vulkan on Linux and Windows, both
  architectures, at `b11960`, `b12110` (what "latest release" resolves to) and
  `b12167` (newest build by date).
- No change to `engine.go` or `pinned.go` was needed: the existing patterns
  already express every Vulkan case in the range.

## Findings

### 1. Vulkan asset names are stable across the whole range (info, no action)

From `b11838` to `b12167` every release that ships binaries has exactly these
four Vulkan archives, with unchanged names and archive formats:

| OS | Arch | Asset |
|---|---|---|
| Linux | x64 | `llama-bNNNNN-bin-ubuntu-vulkan-x64.tar.gz` |
| Linux | arm64 | `llama-bNNNNN-bin-ubuntu-vulkan-arm64.tar.gz` |
| Windows | x64 | `llama-bNNNNN-bin-win-vulkan-x64.zip` |
| Windows | arm64 | `llama-bNNNNN-bin-win-vulkan-arm64.zip` |

No renames, no removals (other than finding 2), no new Vulkan variants, no
archive-format change, and no `cudart-` companion archive (Vulkan does not need
one; `PickAssets` returns a single asset, which the tests assert).
Evidence: `vulkan-assets.json` (per-tag Vulkan asset names for 200 releases),
`b12110-assets.txt`, `b12167-assets.txt`.

Archive sizes grew only slightly between the pin and `b12167` (for example
`win-vulkan-x64` 37.3 MB to 37.7 MB), so the archives did not gain a large
bundled runtime.

### 2. `b12050` shipped no Windows Vulkan archive (minor)

`llama.cpp` release `b12050` (2026-11-02) published the Linux Vulkan archives
but not `llama-b12050-bin-win-vulkan-{x64,arm64}.zip`. It is the only release
in the range with that gap.

Effect today: `assetPatterns` for Windows Vulkan
(`internal/engine/engine.go:134-135`) falls back to `-bin-win-cpu-<arch>`, so
`PickAssets` returns `llama-b12050-bin-win-cpu-x64.zip` and the install
succeeds, but `Install` still records `Installed.Backend = hw.Backend`
(`internal/engine/engine.go:267`), which is `Vulkan`. A Windows Vulkan user on
that build silently gets a CPU build while EZLM reports Vulkan.

Severity: minor. The gap is transient and only reachable with
`EZLM_LLAMA=latest` on that exact build. Covered by
`TestVulkanMissingWindowsAssetFallsBackToCPU`, which pins the current
behaviour; it is not a regression introduced by the upgrade.

### 3. "Latest release" is `b12110`, not the newest build (major for the upgrade plan)

`releases/latest` is `v0.9.0`; it has one asset, `nightly-tag.txt`, whose
content is `b12110\n`. `LatestRelease` follows that pointer
(`internal/engine/engine.go:69-96`), so the build EZLM would install today is
`b12110`. The newest `bNNNNN` release by date is `b12167`
(2026-11-06T09:45Z), 57 builds later.

The upgrade plan (a14) must decide which one to pin: pinning "the latest
release" means `b12110`; pinning "the newest build" means `b12167`. Both were
tested here and both have the four Vulkan archives. Note that a stable release
carrying only `nightly-tag.txt` means the pin can point at a build that is
already a few days old.

Severity: major for the plan, not a code bug. No EZLM code change proposed.

## Test matrix (Vulkan)

| Platform | Arch | b11960 | b12110 (latest release) | b12167 (newest) | Status |
|---|---|---|---|---|---|
| Linux | amd64 | `llama-b11960-bin-ubuntu-vulkan-x64.tar.gz` | `llama-b12110-bin-ubuntu-vulkan-x64.tar.gz` | `llama-b12167-bin-ubuntu-vulkan-x64.tar.gz` | pass |
| Linux | arm64 | `llama-b11960-bin-ubuntu-vulkan-arm64.tar.gz` | `llama-b12110-bin-ubuntu-vulkan-arm64.tar.gz` | `llama-b12167-bin-ubuntu-vulkan-arm64.tar.gz` | pass |
| Windows | amd64 | `llama-b11960-bin-win-vulkan-x64.zip` | `llama-b12110-bin-win-vulkan-x64.zip` | `llama-b12167-bin-win-vulkan-x64.zip` | pass |
| Windows | arm64 | `llama-b11960-bin-win-vulkan-arm64.zip` | `llama-b12110-bin-win-vulkan-arm64.zip` | `llama-b12167-bin-win-vulkan-arm64.zip` | pass |

Also covered: `cuda13=true` passed with a Vulkan backend (the path `Install`
takes when a CUDA 13 machine falls back to Vulkan) returns the same Vulkan
archive; archive suffix is `.tar.gz` on Linux and `.zip` on Windows; the pinned
release contains all four Vulkan archives so offline installs work.

## What I could not finish

- I did not open the archives to check their contents (runtime DLL/so sets).
  That is a07's slice; sizes alone show no large new dependency.
- No live `LatestRelease` test was added (it would need `httptest` and the
  `ReleasesAPI` global); the `v0.9.0 -> nightly-tag.txt -> b12110` chain is
  recorded from the API instead.

## Next steps

1. a14: choose the pin target (`b12110` vs `b12167`) and record it in the plan.
2. Consider surfacing the CPU fallback to the user when a Vulkan install lands
   on a CPU archive (small `Install`/report change, outside this task's slice).
3. a15: the asset-check script should treat a missing Vulkan archive as a
   warning, since selection still succeeds through the CPU fallback.
