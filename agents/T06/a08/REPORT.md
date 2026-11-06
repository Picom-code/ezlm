# T06 a08: SYCL, OpenCL, Mac x64, naming conventions, checksums

Slice: the release assets that are not the main per-platform slices. Worktree
`work/subs/T06/a08`, branch `agent/T06-a08`. Pinned tag unchanged (`b11960`).

## What I did

1. Cached the GitHub release payloads for `b11960` (pinned), `b12110` (what
   `releases/latest` resolves to through `nightly-tag.txt` in v0.9.0) and
   `b12144` (newest build by date) under this folder. GET only; the API rate
   limit hit after my first fetches, so the payloads were copied from a
   sibling agent's identical GETs.
2. Added `internal/engine/assets_other_test.go`: table tests for the Intel
   Mac, SYCL, OpenCL and OpenVINO assets, the tagless Windows CUDA runtime
   archives, the asset naming convention over every published name, the s390x
   additions, checksum naming, and the `hasBinaries` release-shape cases.
3. Two minimal fixes in `internal/engine/engine.go` (below), each with a test
   that failed before and passes after.

Evidence: `release-b11960.json`, `release-b12110.json`, `release-b12144.json`,
`release-latest.json`, `nightly-tag.txt`, `asset-diff.txt` (generated from
them).

## Findings

### F1 (major, fixed) An unmapped architecture matches any backend's archive

`assetPatterns` did `a := map[string]string{"amd64": "x64", "arm64":
"arm64"}[arch]`, so any other arch produced `a == ""` and a pattern such as
`-bin-ubuntu-` or `-bin-macos-`, which matches every archive for that OS.
`PickAssets(r, "linux", "s390x", CPU)` returned
`llama-b12110-bin-ubuntu-26.04-s390x.tar.gz` by luck of alphabetical order;
`PickAssets(r, "linux", "ppc64le", CPU)` returned the same s390x archive and
`PickAssets(r, "darwin", "386", Metal)` returned the arm64 build. Relevant
now because llama.cpp does publish s390x assets and just added two more
variants (F3). Fixed: return `nil` when the arch is not mapped, so
`PickAssets` reports "no llama.cpp build for linux/ppc64le (cpu)" instead of
installing the wrong binary. Test: `TestPickAssetsUnknownArch`.

### F2 (minor, fixed) `hasBinaries` counted the CUDA runtime archives

`hasBinaries` looked for `-bin-` anywhere, and `cudart-llama-bin-win-cuda-*`
contains it. A stable release carrying only runtime archives would have been
treated as having a downloadable server, and `PickAssets` would then fail with
a confusing error. Now requires the `llama-` prefix. Test: `TestHasBinaries`.

### F3 (minor) s390x gained qualified variants; nothing else was renamed

Between `b11960` (36 assets) and `b12110` (38) two assets were added and none
removed:

- `llama-b12110-bin-ubuntu-26.04-s390x.tar.gz` (Ubuntu release qualifier in
  the OS slot)
- `llama-b12110-bin-ubuntu-zdnn-s390x.tar.gz`

The plain `-bin-ubuntu-s390x` archive is still published. `b12144` has the
same 38 names as `b12110`; there is no other add, remove or rename in the
range. Test: `TestRenamedS390xAssets`.

### F4 (minor, open) The Ubuntu release qualifier is not understood

If the qualifier reaches x64 or arm64 (`llama-b<tag>-bin-ubuntu-26.04-x64`),
the Linux patterns `-bin-ubuntu-x64` and `-bin-ubuntu-arm64` stop matching and
Linux installs fail. `b11960` to `b12144` keep the plain names, so this is a
watch item, not a break today. Test `TestUbuntuReleaseQualifierIsUnsupported`
pins the current behaviour so the upgrade plan sees it. A fix would need
patterns that accept an optional `<release>.` component without matching
`cuda-`, `vulkan-`, `rocm-`, `sycl-` or `openvino-`; larger than this slice
and it overlaps a04/a07, so left open.

### F5 (minor) SYCL, OpenCL and OpenVINO are published but never selectable

Every tag ships `bin-ubuntu-sycl-fp16-x64`, `bin-ubuntu-sycl-fp32-x64`,
`bin-win-sycl-x64`, `bin-win-opencl-adreno-arm64` and the OpenVINO builds.
`scan.Backend` has only metal, cuda, rocm, vulkan and cpu
(`internal/scan/scan.go:27-31`), and `assetPatterns` has no SYCL, OpenCL or
OpenVINO pattern, so those machines get Vulkan (`internal/scan/scan.go:138`)
or the CPU archive. That is a reasonable decision, not a break; a SYCL or
OpenCL backend would be a feature. Tests: `TestPickAssetsOther` documents the
fallback picks. Note the `-bin-win-opencl-adreno-arm64` name is the only
OpenCL asset, and it is the only name that puts a vendor word before the
architecture.

### F6 (none) Intel Mac selection is fine at every tag

`darwin/amd64` picks `llama-b<tag>-bin-macos-x64.tar.gz` for `b11960`,
`b12110` and `b12144`; no CUDA runtime is paired and the backend argument
does not change the pick. `flavor` gives `macos-x64`, so an Intel install
does not collide with `macos-arm64` in `~/.ezlm/bin`. The
`llama-b<tag>-xcframework.zip` sidecar (69 to 76 MB) is never picked; EZLM's
Mac app downloads the server at install time and does not bundle the
framework. Tests: `TestPickAssetsOther`, `TestPickAssetsSidecars`,
`TestFlavorMacX64`.

### F7 (minor) There is no checksum file; the per-asset digest is the only checksum

No tag publishes `SHA256SUMS`, `checksums.txt`, `.sha256`, `.sig` or `.asc`.
Every asset in all three tags carries a `sha256:` digest in the API, which
`Asset.Digest` reads and `verifyDigest` enforces. Two consequences for the
upgrade: the new pin must take digests from the API (there is nothing to
fetch and verify against), and `verifyDigest` silently accepts an asset with
an empty digest on size alone (`internal/engine/engine.go:327-331`), so a
missing digest is not a failure. Test: `TestChecksumFileNaming`.

### F8 (minor) Windows CUDA runtime archives have no build tag in their name

`cudart-llama-bin-win-cuda-12.4-x64.zip` (and the two 13.4 variants) carry no
`bNNNNN`, and their digests are byte-identical across `b11960`, `b12110` and
`b12144` (the runtime is static). Any rename-by-tag script or digest-diff
check must not assume every asset changes per tag. `PickAssets` still pairs
them correctly after a bump because it matches on the `-cuda-<version>-<arch>`
suffix, not the tag. Test: `TestPickAssetsOther` (windows CUDA rows).

### F9 (minor, open) Selection depends on the order of the release's asset list

`PickAssets` takes the first asset matching a pattern. The Linux arm64 CPU
pattern `-bin-ubuntu-arm64` is a substring of
`-bin-ubuntu-vulkan-arm64` and `-bin-ubuntu-cuda-13.4-arm64`, so the CPU
pick is right only because the API returns assets sorted by name. If the
ordering ever changes, `linux/arm64` CPU would install Vulkan. A fix is to
reject names containing another backend word in the fallback patterns; it
touches a04/a05/a07 territory, so left as a finding.

### F10 (none) `pinned.go` matches the b11960 API exactly

34 of the 36 published assets, with every size and digest equal. The two
omitted assets are the sidecars `llama-b11960-ui.tar.gz` and
`llama-b11960-xcframework.zip`, which `PickAssets` never selects, so the
offline fallback is complete for selection. This confirms the copy in
`pinned.go` is not stale.

## Changes

- `internal/engine/engine.go`: `assetPatterns` returns `nil` for an unmapped
  architecture (F1); `hasBinaries` requires the `llama-` prefix (F2).
- `internal/engine/assets_other_test.go`: new table tests, listed above.

No change to the pinned tag, `pinned.go`, `download.go` or `internal/serve/`.

## Open questions

- Should EZLM add a SYCL backend for Intel Arc? Vulkan is the current answer
  (`internal/scan/scan.go:138`) and works on Arc, so this is a quality
  question, not a blocker.
- Who owns the s390x / Ubuntu-qualifier watch (F3, F4)? I did not add a
  pattern because the fix overlaps a04/a07; a13's matrix is the natural home
  for a "new asset name" tripwire.

## Verification

`gofmt -l .` empty, `go build ./...`, `go vet ./internal/engine/`,
`go test -p 2 ./internal/engine/` all pass. Both fixes were run as failing
tests first (`TestPickAssetsUnknownArch`, `TestHasBinaries`), then green.
