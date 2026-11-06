# T06 a04: Linux CUDA 12 x64 and CUDA 13 assets

Slice: every llama.cpp release asset for Linux CUDA from b11960 (the pinned
tag) to the latest release, mapped to what `internal/engine` selects.

## What I did

- Read the Linux CUDA assets for all 117 builds from b11960 to b12167 from
  the GitHub releases API (GET only, `releases?per_page=100`, pages 1 and 2).
  The per-tag asset names, sizes and digests are in
  `agents/T06/a04/linux-cuda-assets.json`.
- Resolved what "latest release" means for asset selection:
  `releases/latest` is v0.9.0 (2026-11-04), which carries only
  `nightly-tag.txt`; its content is `b12110`. `LatestRelease()` follows that
  file, so the upgrade target for assets is b12110.
- Added `internal/engine/assets_linuxcuda_test.go` with table tests over the
  pinned build, b12110 (latest release target) and b12167 (newest nightly at
  the time of writing), plus fallback and cudart-pairing cases.
- No change to `engine.go` or `pinned.go` was needed: selection already
  returns the right asset for every case in this slice.

## Findings

### 1. No renamed, removed or reformatted Linux CUDA assets (no action)

From b11960 (2026-10-28) to b12167 (2026-11-06) every release publishes
exactly six Linux CUDA assets, with the same names apart from the build tag,
the same CUDA versions (12.8 and 13.4) and the same `.tar.gz` format:

```
llama-bN-bin-ubuntu-cuda-12.8-x64.tar.gz
llama-bN-bin-ubuntu-cuda-13.4-x64.tar.gz
llama-bN-bin-ubuntu-cuda-13.4-arm64.tar.gz
cudart-llama-bN-bin-ubuntu-cuda-12.8-x64.tar.gz
cudart-llama-bN-bin-ubuntu-cuda-13.4-x64.tar.gz
cudart-llama-bN-bin-ubuntu-cuda-13.4-arm64.tar.gz
```

Evidence: `agents/T06/a04/linux-cuda-assets.json` (117 tags, 6 assets each,
no shape changes). Test: `TestLinuxCUDAAssetShapeUnchanged`.

`engine.go:114-116` (CUDA 12 preference), `engine.go:129-130` (CUDA 13 for
Blackwell) and `engine.go:174-185` (cudart pairing) all still match these
names, so the upgrade needs no asset-side change for Linux CUDA.

### 2. The upgrade target is b12110, not b12144 or b12167 (major, for a14)

`releases/latest` returns v0.9.0, which has a single asset `nightly-tag.txt`
(7 bytes) whose content is `b12110`. `LatestRelease()` in `engine.go:80-104`
reads that file and returns the b12110 release. The brief's "newest build by
date is b12144" was already stale when I looked: b12167 (2026-11-06T09:45Z)
is newer, and b12144 is in between.

So the upgrade plan should pin **b12110** if it follows what EZLM resolves
today, or b12167 if it deliberately wants the newest nightly. Whichever is
chosen, the Linux CUDA asset names are the same, only the tag changes.

### 3. New Linux assets at b12110 that selection must ignore (minor)

b12110 has 38 assets against b11960's 36. The additions relevant to Linux
are `llama-b12110-bin-ubuntu-26.04-s390x.tar.gz`,
`llama-b12110-bin-ubuntu-zdnn-s390x.tar.gz`, `llama-b12110-ui.tar.gz` and
`llama-b12110-xcframework.zip`. None of them carries `-x64` or `-arm64`
alongside `-bin-ubuntu-`, so the x64/arm64 patterns cannot match them; the
fixture releases in the test file include all four to prove it. The s390x
split is a07/a08 material.

### 4. Linux CUDA install grows about 5 MB (info)

The CUDA runtime archives are unchanged in size (594 MB, 552 MB, 440 MB,
varying by less than 1 KB). The build archives grow:

| asset | b11960 | b12110 | b12167 |
|---|---|---|---|
| ubuntu-cuda-12.8-x64 | 182,524,002 | 187,314,621 (+2.6%) | 188,169,294 (+3.1%) |
| ubuntu-cuda-13.4-x64 | 163,957,335 | 168,275,993 (+2.6%) | 169,468,279 (+3.4%) |
| ubuntu-cuda-13.4-arm64 | 159,059,972 | 163,315,223 (+2.7%) | 164,518,837 (+3.4%) |

A Linux CUDA 12 x64 install goes from 776,895,480 bytes to 781,687,098
bytes (build plus cudart). Nothing in EZLM depends on a size ceiling.

### 5. The pinned Linux CUDA entries match the published release (info)

`pinned.go:9-11` and `pinned.go:14-16` were checked against the b11960 API
listing: name, size and sha256 digest all match. Test:
`TestPinnedReleaseHasLinuxCUDA` also asserts the offline fallback resolves a
build plus cudart for the three Linux CUDA cases.

## What I changed

- `internal/engine/assets_linuxcuda_test.go` (new): table tests for
  b11960, b12110 and b12167 on linux/amd64 CUDA (12.8 and 13.4) and
  linux/arm64 CUDA 13.4, plus the shape, format, cudart-pairing, fallback
  and pinned-release cases. `engine.go` and `pinned.go` are untouched.

## What I could not finish

- Nothing blocking. I did not test the extracted archive contents (which
  `.so` files each tarball must carry); that is a07's slice, and it would
  need a download, which the brief forbids.

## Open questions

- a14: pin b12110 (what `LatestRelease()` resolves today) or b12167 (newest
  nightly)? The asset mapping is identical either way.
- Should `LatestRelease()` also fall back when `nightly-tag.txt` names a tag
  that no longer exists? Not exercised here; b12110 exists.

## Verification

```
gofmt -l .                     # empty
go build ./...
go vet ./...
go test -p 2 ./internal/engine/
```
