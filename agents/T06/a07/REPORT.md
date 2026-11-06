# T06 / a07: CPU-only builds for every OS, and runtime DLL/so sets

## Scope

Every CPU-only release asset from the pinned build b11960 to the current
release, for darwin, Windows and Linux. For each: which archive `engine.go`
picks, whether it still exists, and what runtime DLL/so files the archive must
contain for `llama-server` to start.

"Latest release" here means what `engine.LatestRelease` actually resolves:
`releases/latest` is v0.9.0 (2026-11-04), which carries only
`nightly-tag.txt`, and that file names **b12110**. The newest nightly by date
at the time of writing is **b12167**; its CPU assets are identical, so the
results below hold for both.

## Method

- `GET /releases/latest`, `/releases?per_page=100`, `/releases/tags/{b11960,
  b12110,b12144,b12167}` (read only). JSON cached in this folder.
- `GET` `nightly-tag.txt` from v0.9.0 (7 bytes) to resolve the release target.
- Release workflow at both tags (`.github/workflows/release.yml`) for the build
  flags and the packaging steps, and the CMake files at b12110 for the shared
  library and backend-module targets.
- Windows archive contents: HTTP range `GET` of the last 128 KiB of each
  `win-cpu-*.zip`, parsed from the zip central directory. No archive was
  downloaded whole.
- Linux archive contents: HTTP range `GET` of the first 6 MiB of
  `llama-b12110-bin-ubuntu-x64.tar.gz`, stream-decompressed and tar-listed.
  Only the first 14 entries are covered; the rest is derived from the same
  CMake targets and the Windows listing.

## Findings

### 1. No CPU asset was renamed or removed (info)

| OS / arch | b11960 | b12110 and b12167 |
|---|---|---|
| darwin arm64 | `llama-b11960-bin-macos-arm64.tar.gz` | `llama-b12110-bin-macos-arm64.tar.gz` |
| darwin x64 | `llama-b11960-bin-macos-x64.tar.gz` | `llama-b12110-bin-macos-x64.tar.gz` |
| windows x64 | `llama-b11960-bin-win-cpu-x64.zip` | `llama-b12110-bin-win-cpu-x64.zip` |
| windows arm64 | `llama-b11960-bin-win-cpu-arm64.zip` | `llama-b12110-bin-win-cpu-arm64.zip` |
| linux x64 | `llama-b11960-bin-ubuntu-x64.tar.gz` | `llama-b12110-bin-ubuntu-x64.tar.gz` |
| linux arm64 | `llama-b11960-bin-ubuntu-arm64.tar.gz` | `llama-b12110-bin-ubuntu-arm64.tar.gz` |
| linux s390x | `llama-b11960-bin-ubuntu-s390x.tar.gz` | `llama-b12110-bin-ubuntu-s390x.tar.gz` |

Formats are unchanged too: `.zip` on Windows, `.tar.gz` everywhere else.
b12110 added two s390x CPU variants, `-bin-ubuntu-26.04-s390x.tar.gz` and
`-bin-ubuntu-zdnn-s390x.tar.gz`; neither is an EZLM target, and the plain
`-ubuntu-s390x` archive is still published.

The selection logic also does the right thing for every one of these except
s390x, which is finding 2. In particular it does not mistake
`-bin-ubuntu-openvino-*`, `-bin-ubuntu-sycl-*`, `-bin-win-openvino-*` or the
CUDA/Vulkan archives for a CPU build, because the arch part of the pattern
(`-bin-ubuntu-x64`, `-bin-win-cpu-x64`, ...) is adjacent in the asset name.

### 2. linux/s390x picks the arm64 archive on b11960 (major, fixed)

`assetPatterns` maps `runtime.GOARCH` through
`map[string]string{"amd64": "x64", "arm64": "arm64"}` (engine.go:111). For any
other architecture the lookup yields `""`, so the pattern degrades to
`-bin-ubuntu-`, which matches the first ubuntu asset in release order. On
b11960 that is `llama-b11960-bin-ubuntu-arm64.tar.gz`, so an s390x machine
would install an arm64 `llama-server` and fail to start it. On b12110 it
happens to pick `llama-b12110-bin-ubuntu-26.04-s390x.tar.gz` (right arch, but
the 26.04 build rather than the plain one).

Reproduced by `TestPickAssetsCPU` before the fix:

```
b11960 linux/s390x: got llama-b11960-bin-ubuntu-arm64.tar.gz, want llama-b11960-bin-ubuntu-s390x.tar.gz
b12110 linux/s390x: got llama-b12110-bin-ubuntu-arm64.tar.gz, want llama-b12110-bin-ubuntu-s390x.tar.gz
```

Fix (one line, engine.go:111): add `"s390x": "s390x"` to the map. The pattern
then becomes `-bin-ubuntu-s390x`, which matches only the plain s390x archive
and not `26.04-s390x`, `zdnn-s390x` or `arm64`. No other platform changes.

### 3. Windows CPU runtime DLL set is identical at b11960 and b12110 (info)

Exact central-directory listing, both tags: 52 entries. The runtime set
`llama-server.exe` needs:

- `llama-server.exe`, `llama-server-impl.dll`
- `llama.dll`, `llama-common.dll`
- `ggml.dll`, `ggml-base.dll`
- `ggml-cpu-x64.dll`, `ggml-cpu-sse42.dll`, `ggml-cpu-sandybridge.dll`,
  `ggml-cpu-ivybridge.dll`, `ggml-cpu-piledriver.dll`, `ggml-cpu-haswell.dll`,
  `ggml-cpu-skylakex.dll`, `ggml-cpu-cannonlake.dll`,
  `ggml-cpu-cascadelake.dll`, `ggml-cpu-icelake.dll`,
  `ggml-cpu-cooperlake.dll`, `ggml-cpu-zen4.dll`,
  `ggml-cpu-alderlake.dll`, `ggml-cpu-sapphirerapids.dll`
- `ggml-rpc.dll`, `mtmd.dll`, `libomp.dll`
- `LICENSE-LLVM-OpenMP`

The other entries are tool executables and their `-impl.dll` files
(`llama-cli`, `llama-bench`, `llama-quantize`, ...), which EZLM does not run.

The win-cpu-arm64 zip has 38 entries and only one CPU module,
`ggml-cpu.dll`, because the workflow passes `GGML_CPU_ALL_VARIANTS=OFF` for
arm64 (`GGML_CPU_ALL_VARIANTS=${{ matrix.arch == 'x64' && 'ON' || 'OFF' }}`).
Everything else in the runtime set is the same.

`GGML_BACKEND_DL=ON` is set in both CPU jobs, so the `ggml-cpu-*` files are
loaded at runtime rather than linked in. They must all be extracted next to
`llama-server.exe`. `extract()` copies the whole archive, so this works today.

### 4. Linux runtime set: versioned .so plus unversioned backend modules (info)

The archive unpacks into a single `llama-<tag>/` folder (the workflow tars
`build/bin` with `--transform "s,^\.,llama-<tag>,"`). `llama-server` needs:

- `libllama.so.0` -> `libllama.so.0.9.0`
- `libllama-common.so.0`, `libggml.so.0`, `libggml-base.so.0`,
  `libmtmd.so.0` (same versioned-file + symlink shape)
- one `libggml-cpu-<variant>.so` per CPU variant: 15 for x64, 8 for arm64
  (`armv8.0_1` ... `armv9.2_2`)
- `libggml-rpc.so`
- the `libllama-*-impl.so` files for tools EZLM does not run

The shared libraries carry `VERSION`/`SOVERSION` and ship as a real file plus a
`lib<name>.so.0` symlink; the backend modules are CMake `MODULE` targets and
are unversioned `.so` files. RPATH is `$ORIGIN`, so all of them must stay in
the same folder as `llama-server`. `untar` recreates symlinks, and
`TestCPUArchiveRuntimeSet` covers that.

### 5. macOS runtime set: dylibs, Metal only on arm64 (info)

Same top-level `llama-<tag>/` folder, RPATH `@loader_path`, so the dylibs must
stay beside the binary: `libllama.dylib`, `libllama-common.dylib`,
`libggml.dylib`, `libggml-base.dylib`, `libggml-cpu.dylib`,
`libggml-rpc.dylib`, `libmtmd.dylib`. The arm64 build also carries
`libggml-metal.dylib`; the x64 job passes `-DGGML_METAL=OFF`, so it does not.
The macOS jobs do not set `GGML_BACKEND_DL`, so there is no per-variant module
set and `libggml-cpu.dylib` is the only CPU backend file.

## Changes

- `internal/engine/assets_cpu_test.go` (new): table test for CPU selection on
  b11960 and b12110 across darwin arm64/x64, windows x64/arm64, linux
  x64/arm64/s390x, using the real asset list and ordering plus the GPU
  archives as decoys; a test that CPU and GPU builds get distinct install
  folders; and an extraction test that unpacks a synthetic archive shaped like
  each CPU release and checks the runtime files, symlinks and exec bits.
- `internal/engine/engine.go:111`: added `"s390x": "s390x"` to the arch map.
  No other selection logic changed.

## What I could not finish

- The full Linux and macOS archive listings. The brief forbids downloading
  release archives, so Linux is a 6 MiB partial stream (14 entries) and macOS
  is derived from the CMake targets. The Windows lists are exact.
- Did not check the b12144/b12167 nightly zips by range; their asset names and
  sizes match b12110 to within a few hundred bytes, and the workflow did not
  change.

## Next steps

- a13's matrix should add linux/s390x with the fixed expectation, and can
  cross-reference this file for the CPU rows.
- If EZLM ever prunes archives instead of extracting them whole, it needs a
  keep-list: the runtime sets above are the minimum. A manifest check would
  catch a future llama.cpp change from module backends to linked-in backends
  (which would remove the `ggml-cpu-*` files).
- The upgrade plan (a14) should pin to b12110, the tag v0.9.0 points at, not to
  b12167.

## Evidence

- `release-b11960.json`, `release-b12110.json`, `release-b12167.json`:
  GitHub release metadata used above.
- `release-latest.json`, `nightly-tag.txt`: the v0.9.0 -> b12110 resolution.
- `win-cpu-x64-b11960.files.txt`, `win-cpu-x64-b12110.files.txt`: exact zip
  central-directory listings.
- `ubuntu-x64-b12110.partial-files.txt`: first entries of the Linux tarball.
- `release-workflow-b11960.yml`, `release-workflow-b12110.yml`: CPU jobs,
  build flags and pack steps.
- `src-b12110-*.txt`, `ggml-CMakeLists-b12110.txt`: library, backend-module
  and install rules.
