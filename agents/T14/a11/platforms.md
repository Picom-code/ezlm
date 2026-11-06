# GPU and platform reality: Windows on Arm, AMD/ROCm, Intel Arc/Vulkan

Agent a11, task T14. Desk study, no product installed. All URLs accessed **2026-11-06** unless noted.
Where a claim is inferred rather than stated by the source, it is marked **(inferred)**.

Scope: what the eight products (LM Studio, Ollama, Jan, GPT4All, Msty, AnythingLLM, llamafile,
LocalAI) actually do on Windows on Arm, on AMD GPUs, and on Intel Arc/Vulkan, what they document as
broken, and what that means for EZLM's scanner, planner and engine downloader.

## 0. The ground truth underneath all of them: what llama.cpp ships

Every product here is a llama.cpp distribution with a UI, so the real platform matrix is the set of
llama.cpp release assets. Checked against the newest release at time of writing, `b12177`
(2026-11-06T17:09Z), via the GitHub API:
<https://api.github.com/repos/ggml-org/llama.cpp/releases?per_page=5> (accessed 2026-11-06).

| Asset family (Windows unless noted) | Present in b12177 | Notes |
|---|---|---|
| `win-cpu-x64`, `win-cpu-arm64` | yes | ARM64 CPU is a first-class build |
| `win-cuda-12.4-x64`, `win-cuda-13.5-x64`, `win-cuda-13.5-arm64` | yes | CUDA on Windows Arm exists (NVIDIA N1X/GB10-class Arm SoCs) |
| `win-vulkan-x64`, `win-vulkan-arm64` | yes | Vulkan is the only cross-vendor path that covers Arm |
| `win-opencl-adreno-arm64` | yes | Snapdragon Adreno iGPU via OpenCL |
| `win-rocm-10.0-x64` | yes | HIP/ROCm on Windows, x64 only |
| `win-sycl-x64` | yes | Intel oneAPI/SYCL |
| `win-openvino-2026.4.1-x64` | yes | Intel OpenVINO |
| `ubuntu-vulkan-arm64`, `linux-arm64-snapdragon`, `android-arm64-snapdragon` | yes | Linux Arm has Vulkan; Snapdragon gets its own build |

So the platform story in late 2026 is not "llama.cpp cannot do Arm or AMD". It is: llama.cpp can do
all of it, and the product layer (detection, runtime choice, defaults) is where support is won or
lost. That is exactly the layer EZLM owns.

## 1. Windows on Arm

| Product | Arm64 app | GPU acceleration on WoA | Evidence |
|---|---|---|---|
| LM Studio | yes, x64 and ARM (Snapdragon X Elite) | No. GPU runner missing; hardware monitor reports 0 GPUs, 0 GB VRAM; models run on CPU | <https://lmstudio.ai/docs/app/system-requirements>; bug tracker #2577 (2026-11-05, 0 comments) <https://github.com/lmstudio-ai/lmstudio-bug-tracker/issues/2577>; #255 "Windows arm64 - gpu runner" open since 2024-12-16 <https://github.com/lmstudio-ai/lmstudio-bug-tracker/issues/255> |
| Ollama | yes, `ollama-windows-arm64.zip` | CPU only: the Arm zip is 217 MB against 1.6 GB for the x64 zip, and there is no Arm CUDA/ROCm payload (the ROCm payload ships only as `ollama-windows-amd64-rocm.zip`) | <https://api.github.com/repos/ollama/ollama/releases/latest> (v0.40.15, 2026-11-03) |
| Jan | yes, `Jan_0.8.8_arm64-setup.exe` | CPU only on Windows Arm **(inferred** from the absence of an Arm GPU runtime in the release assets and from Jan's llama.cpp backend selection) | <https://api.github.com/repos/janhq/jan/releases/latest> (v0.8.8, 2026-10-28) |
| GPT4All | yes, `gpt4all-installer-win64-arm-v3.10.0.exe` | CPU only; the project has shipped no release since 2025-02-25 | <https://api.github.com/repos/nomic-ai/gpt4all/releases/latest> |
| llamafile | **no**: docs say "Windows 10+ (AMD64 only)" | none on Windows Arm; ARM64 CPUs are supported on Linux and macOS only | <https://docs.mozilla.ai/llamafile/reference/support> |
| LocalAI | yes, `nvidia-l4t-arm64` images for Jetson/DGX Spark | GPU only through NVIDIA Arm64 containers, not through Snapdragon iGPU | <https://localai.io/docs/features/gpu-acceleration/> |
| Msty | x64 Electron app | inherits whichever engine it points at (Ollama or LM Studio), so CPU only on WoA **(inferred)** | <https://docs.msty.app/> |
| AnythingLLM | x64 desktop app | same, inherits the engine **(inferred)** | <https://docs.anythingllm.com/> |

What is documented as broken: LM Studio's own hardware panel showing "0 GPUs detected and VRAM:
0 GB" on a Snapdragon X2 Elite Extreme with 47.62 GB unified memory, even with the QNN, OpenVINO and
DirectX 12 SDKs installed (#2577). The reporter's ask is the important part: "VRAM should map
dynamically to a portion of the ... unified RAM pool". No product in this set models Arm unified
memory the way Apple Silicon is modelled.

Reading: Windows on Arm in this market is "it installs and runs, slowly". The gap is not the app
build, it is detection plus a runtime choice. `win-vulkan-arm64` and `win-opencl-adreno-arm64` exist
and are unused by every GUI product here.

## 2. AMD: ROCm versus Vulkan

| Product | Windows AMD | Linux AMD | Evidence |
|---|---|---|---|
| Ollama | ROCm 7 for RX 7900 XTX/XT/GRE, 7800 XT, 7700 XT, 7600 XT/7600, PRO W7900/7800/7700/7600/7500; Vulkan by default as the recommended fallback, notably for RX 6000/RDNA2 where ROCm 7 is not exposed | ROCm 7, much wider list (RX 9000, 7000, 6000, Ryzen AI Max, Instinct) | <https://docs.ollama.com/gpu>, <https://docs.ollama.com/windows> |
| LM Studio | ships a ROCm llama.cpp runtime, but it is a per-runtime choice and card support is narrower than Ollama's list; 109 open/closed issues match "rocm" in the bug tracker, e.g. #2358 "ROCm llama.cpp with 9070xt on Windows 11" (open, 2026-09-02) | ROCm runtimes available | <https://github.com/lmstudio-ai/lmstudio-bug-tracker/issues?q=rocm> (109 results), <https://github.com/lmstudio-ai/lmstudio-bug-tracker/issues/2358> |
| Jan | no ROCm/HIP on Windows: feature request #8529 "ROCm/HIP backend support on Windows for AMD GPUs, parity with Linux and LM Studio" (open, 2026-07-25) | ROCm works, with Flatpak detection problems (#8266, open) | <https://github.com/janhq/jan/issues/8529>, <https://github.com/janhq/jan/issues/8266> |
| llamafile | AMD HIP/rocBLAS "Linux, Windows", but the library is downloaded separately, needs the ROCm runtime, and "multi-GPU may be broken on Radeon Any"; the 0.10 series AMD path is "best-effort" | same | <https://docs.mozilla.ai/llamafile/reference/support> |
| LocalAI | ROCm container images; Strix Halo (gfx1151) needs ROCm 7.11+, kernel parameters (`iommu=pt amdgpu.gttsize=126976 ttm.pages_limit=32505856`) and four environment variables (`HSA_OVERRIDE_GFX_VERSION=11.5.1`, `ROCBLAS_USE_HIPBLASLT=1`, `HSA_XNACK=1`, `HSA_ENABLE_SDMA=0`) | same | <https://localai.io/docs/features/gpu-acceleration/> |
| GPT4All, Msty, AnythingLLM | no first-party ROCm; GPT4All relies on Vulkan, the other two on their engine | same | <https://docs.gpt4all.io/> |

What is documented as broken on AMD:
- Vulkan crashes on RDNA4: LM Studio #2524 "llama.cpp v2.59-2.62 crash with ErrorDeviceLost on RX
  9070 XT (last working v2.58.0)" (open, 2026-10-21) <https://github.com/lmstudio-ai/lmstudio-bug-tracker/issues/2524>.
- Vulkan performance degrades after another Vulkan workload has run: LM Studio #2426 (open,
  2026-09-21) <https://github.com/lmstudio-ai/lmstudio-bug-tracker/issues/2426>.
- Vulkan does not report free VRAM without extra rights: Ollama needs `cap_perfmon` (or root) to
  expose available VRAM, otherwise it falls back to approximate sizing
  <https://docs.ollama.com/gpu>. This matters for any planner: on Vulkan, free VRAM is a hint, not a
  measurement.
- Mixed iGPU/dGPU systems pick the unstable iGPU unless pinned: Ollama tells users to set
  `GGML_VK_VISIBLE_DEVICES` <https://docs.ollama.com/windows>.
- RDNA2 on Windows often cannot get ROCm 7 at all, so Vulkan is the only option there
  <https://docs.ollama.com/windows>.

## 3. Intel Arc and Vulkan

| Product | Intel path | Evidence |
|---|---|---|
| Ollama | Vulkan only (no SYCL/oneAPI runtime in the release assets); Intel Linux needs the Intel Vulkan driver from dgpu-docs | <https://docs.ollama.com/gpu>, <https://api.github.com/repos/ollama/ollama/releases/latest> |
| LM Studio | Vulkan runtime; 282 issues match "vulkan", a mix of AMD and Intel | <https://github.com/lmstudio-ai/lmstudio-bug-tracker/issues?q=vulkan> |
| LocalAI | SYCL/oneAPI is a first-class backend (`intel` capability, `gpu-intel` images); backends carry their own Intel driver so no host driver install is needed; `ZE_ENABLE_ALT_DRIVERS` for newer cards | <https://localai.io/docs/features/gpu-acceleration/> |
| llamafile | no oneAPI/SYCL backend at all; "Vulkan covers a lot of the hardware that CUDA and ROCm do not" | <https://docs.mozilla.ai/llamafile/reference/support> |
| Jan, GPT4All | Vulkan **(inferred** from their llama.cpp runtimes; neither documents an Intel-specific backend) | <https://jan.ai/docs>, <https://docs.gpt4all.io/> |

Reading: Intel is the vendor with the least first-party support in this set. Only LocalAI ships SYCL.
llama.cpp ships `win-sycl-x64` and `win-openvino-2026.4.1-x64` for Windows, so a product that wanted
to be fastest on Arc could be; none of the GUI products here is. Vulkan on Arc works and is the safe
answer, at some performance cost versus SYCL **(inferred**: llama.cpp's own SYCL docs describe SYCL
as the Intel-optimised path, <https://github.com/ggml-org/llama.cpp/blob/master/docs/backend/SYCL.md>).

## 4. CUDA 13 drops old cards: a shared trap

llama.cpp now ships CUDA 12.4 x64 and CUDA 13.5 x64/Arm64. CUDA 13 no longer contains sm_61 (Pascal)
code, and this bites products that moved wholesale to a CUDA 13 engine: Jan 0.8.8 crashes on every
model load on Pascal GPUs (#9501 "Pascal GPUs crash on every model load in v0.8.8 (CUDA 13 engine has
no sm_61 code)", open, 2026-10-30)
<https://github.com/janhq/jan/issues/9501>. The safe pattern is what Ollama documents as a driver
floor (compute capability 5.0+, driver 550+, 570+ for 5.0-6.2,
<https://docs.ollama.com/gpu>) and what llama.cpp does by keeping both CUDA 12 and 13 assets.

## 5. What EZLM does today (code reading)

Facts from the worktree at `agent/T14-a11`:

- Backend choice: NVIDIA wins CUDA, AMD and Intel win Vulkan, everything else is CPU
  (`internal/scan/scan.go:135-141`). `ROCm` is a declared `Backend` (`internal/scan/scan.go:26`) but
  nothing ever selects it, and `assetPatterns` maps `ROCm` to the **Vulkan** asset on both Linux and
  Windows (`internal/engine/engine.go:125`, `:134`). So EZLM never ships a HIP build even though
  `win-rocm-10.0-x64` and `ubuntu-...-rocm` exist upstream.
- AMD detection on Linux is `rocm-smi --showmeminfo vram --showproductname --json`
  (`internal/scan/gpu_discrete.go:147`). On Windows there is no `rocm-smi`, so AMD and Intel both go
  through the registry display-adapter scan (`internal/scan/display_windows.go:23`), which sets
  Vulkan. That is a defensible default, but it is the only path EZLM has.
- `vendorOf` knows nvidia, amd, intel only (`internal/scan/gpu_discrete.go:231`). A Qualcomm Adreno
  or Snapdragon adapter therefore matches nothing, `usableAdapters` drops it
  (`internal/scan/gpu_discrete.go:203-215`), and EZLM downloads `win-cpu-arm64` on a Snapdragon
  machine even though `win-vulkan-arm64` and `win-opencl-adreno-arm64` exist. It also has no
  `win-sycl-x64` or `win-opencl-*` pattern at all (`internal/engine/engine.go:110-140`).
- `Unified` is set only on macOS (`internal/scan/scan_darwin.go:34`). On Windows Arm the Adreno iGPU
  shares system RAM, but EZLM will treat any GPU it does see as discrete, so `GPUUsable` is the sum
  of `MemFree` and the planner's headroom math is wrong on that class of machine.
- Built-in AMD/Intel graphics count only when they have 8 GB or more set aside
  (`internal/scan/gpu_discrete.go:211`), which is deliberate and matches llama.cpp's "iGPU only when
  there is no dGPU" rule. Arc 140V-class laptops with a 4-6 GB share are CPU in EZLM. That is
  defensible, but it should be a visible note rather than silent.
- CUDA version handling is better than most: CUDA 12 is preferred, CUDA 13 is used only when
  Blackwell needs it, and the Arm pattern `-bin-win-cuda-` + `arm64` does match
  `win-cuda-13.5-arm64` (`internal/engine/engine.go:110-140`). Pascal machines keep working.

## 6. What EZLM should expect

1. **Windows on Arm will be a support ticket generator** unless detection names the GPU. Snapdragon X
   machines are sold as "Copilot+ PCs" with an iGPU and an NPU; every product here currently runs
   them on CPU, and LM Studio's own panel reports zero GPUs (#2577). Users will compare against
   "it has a GPU" marketing, not against our benchmark.
2. **NPU is not a runtime.** No product in this set runs a GGUF on the Hexagon NPU; LM Studio users
   are asking for it (#2577). EZLM should not promise NPU acceleration, and should say "CPU" plainly
   when that is what it picked.
3. **Vulkan is the correct default on AMD and Intel, and it is not free.** It is the only path that
   covers RDNA2-on-Windows, RDNA4, Arc, and Arm in one build. It is also the path with the
   documented crashes (#2524) and the degraded-VRAM reporting (Ollama's `cap_perfmon` note). Expect
   a small tail of "it crashed after a game" reports that are not EZLM's bug.
4. **ROCm on Windows is now real but narrow.** RX 7000 and PRO W7000 only, per Ollama's own list.
   Shipping a HIP build for those cards is a speed win; shipping it broadly is a support burden.
5. **Intel users get Vulkan from everyone.** That is table stakes, not differentiation. SYCL is the
   only way to be visibly faster on Arc, and only LocalAI does it.
6. **Old NVIDIA cards are a live regression risk.** Jan's CUDA 13-only engine breaks Pascal
   (#9501). EZLM's CUDA 12-first rule already avoids this; keep it.

## 7. Recommendations for EZLM

| # | Recommendation | Evidence | Effort | Priority |
|---|---|---|---|---|
| P1 | Teach the scanner about Qualcomm: match `qualcomm`/`adreno` in `vendorOf`, treat it as unified memory (`Unified=true`, usable = a fraction of RAM), and select `win-vulkan-arm64` (fallback `win-opencl-adreno-arm64`) on Windows Arm. Emit a note when it falls back to CPU. | #2577 shows LM Studio reporting 0 GPUs; `win-vulkan-arm64` and `win-opencl-adreno-arm64` exist in b12177; EZLM code `internal/scan/gpu_discrete.go:231`, `internal/engine/engine.go:110` | M | High |
| P2 | Add a `rocm` asset pattern for Windows/Linux x64 (`-bin-win-rocm-`, `-bin-ubuntu-...-rocm` or the HIP builds llama.cpp publishes), selected only for cards on a small allowlist (gfx1100/1101/1102/1200/1201/1151), Vulkan otherwise. | Ollama's ROCm list and RDNA2 caveat <https://docs.ollama.com/gpu>; `win-rocm-10.0-x64` in b12177; EZLM never selects ROCm (`internal/engine/engine.go:125`) | M | Medium |
| P3 | On Windows Arm, stop treating a detected GPU as discrete: set `Unified` when the adapter is an iGPU with shared memory, and cap `GPUUsable` at a share of RAM rather than summing `MemFree`. | `internal/scan/scan_darwin.go:34` is the only `Unified=true`; #2577 asks for exactly this mapping | S | High |
| P4 | Say what was chosen and why on the hardware screen: backend, build flavour, and one line of reason ("AMD RX 7800 XT, Vulkan build: ROCm is not available on Windows for this card"). Silent CPU fallback is the most common complaint shape in this set. | Ollama docs recommend Vulkan explicitly for RX 6000; EZLM notes only "No supported GPU found" (`internal/scan/scan_other.go`) | S | High |
| P5 | Consider a SYCL build for Arc as an opt-in experiment, not a default. Keep Vulkan as the default. | Only LocalAI ships SYCL; llama.cpp ships `win-sycl-x64`; SYCL is Intel's optimised path | M | Low |

## 8. Could not verify / open questions

- Whether `win-vulkan-arm64` is usable on current Adreno drivers in practice. No hardware here, and
  no product ships it, so there is no user report either way.
- Whether LM Studio's ROCm runtime on Windows covers RX 9000. Their docs no longer have a per-vendor
  GPU page; the bug tracker is the only signal, and it is noisy.
- Jan's and GPT4All's Arm64 Windows builds were checked by asset name only; neither documents GPU
  support on Arm.
- Msty and AnythingLLM are wrappers; their platform behaviour was not tested and is marked
  **(inferred)** throughout.
