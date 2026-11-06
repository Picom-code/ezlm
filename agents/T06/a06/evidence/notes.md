# a06 evidence notes

All network calls were GET to `api.github.com` or `raw.githubusercontent.com`
on 2026-11-06. No release archive was downloaded.

## Upgrade target

```
GET https://api.github.com/repos/ggml-org/llama.cpp/releases/latest
  -> v0.9.0, 2026-11-04T13:08:06Z, assets: [nightly-tag.txt]
GET https://github.com/ggml-org/llama.cpp/releases/download/v0.9.0/nightly-tag.txt
  -> b12110
```

`engine.go:77-100` (`LatestRelease`) follows the same pointer. Newest build by
date in the listing: b12167 (2026-11-06T09:45:44Z); b12144 is the one the
brief names.

## Asset listing

```
GET https://api.github.com/repos/ggml-org/llama.cpp/releases?per_page=100      (page 1)
GET https://api.github.com/repos/ggml-org/llama.cpp/releases?per_page=100&page=2
GET https://api.github.com/repos/ggml-org/llama.cpp/releases/tags/b11960
GET https://api.github.com/repos/ggml-org/llama.cpp/releases/tags/b12110
```

117 builds from b11960 to b12167. Every one has exactly:

```
llama-<tag>-bin-ubuntu-rocm-10.0-x64.tar.gz
llama-<tag>-bin-win-rocm-10.0-x64.zip
```

except b12050 (30 assets, missing the Windows ROCm zip; the next build has it
again). No `hip`-named asset, no arm64 ROCm, no ROCm runtime companion
archive.

## Why the ROCm archives are not self-contained

llama.cpp source at b12110:

`.github/workflows/release.yml` (line numbers at that tag)

- `ubuntu-24-rocm` job, `Pack artifacts` at line 1618:
  `tar -czvf ... -C ./build/bin .` with
  `-DCMAKE_INSTALL_RPATH='$ORIGIN'` (line 1605). Only `build/bin` is packed;
  the ROCm libraries from `ROCM_PATH/lib` are not.
- `windows-rocm` job, `Bundle HIP runtime DLLs` at line 1056 copies
  `amdhip64_7.dll`, `rocm_kpack.dll`, `amd_comgr.dll` into `build/bin/Release`
  and the `Pack artifacts` step at line 1085 packs those plus `ggml-hip.dll`
  only. The comment in that step says rocBLAS/hipBLASLt kernels
  resolve via PATH and are not copied.
- `.github/workflows/release-publish.yml`, `Merge artifacts` at line 77
  injects the windows-cpu zip (llama-server.exe, ggml-cpu) into every Windows
  backend zip, so the final `llama-<tag>-bin-win-rocm-10.0-x64.zip` does run
  llama-server, but still needs a ROCm 10.0 install for rocBLAS.

That matches `internal/scan/scan.go:138` ("Vulkan builds are the most reliable
path on consumer AMD") and `BETA.md:44` (AMD machines should report the Vulkan
engine), which is why `assetPatterns` maps ROCm to Vulkan.

## Reproduce the test

```
cd ~/ezlm-agents/work/subs/T06/a06
go test -p 2 ./internal/engine/ -run 'ROCm|PickAssets' -v
```
