# T14 / a11 report: GPU and platform reality

Agent a11. Topic: Windows on Arm, AMD/ROCm, Intel Arc/Vulkan across LM Studio, Ollama, Jan, GPT4All,
Msty, AnythingLLM, llamafile and LocalAI, and what EZLM should expect. Deliverable:
`agents/T14/a11/platforms.md`.

## What I did

- Read the llama.cpp release assets for the current build (`b12177`, 2026-11-06) to establish the
  real platform matrix underneath every product: Windows x64 and Arm64 CPU, CUDA 12/13, Vulkan x64
  and Arm64, OpenCL Adreno, ROCm 10 x64, SYCL x64, OpenVINO x64, Linux Arm and Snapdragon builds.
- Read the primary platform docs for Ollama (GPU and Windows pages), LM Studio (system requirements),
  LocalAI (GPU acceleration), llamafile (supported systems, GPU support), and checked release assets
  for Ollama, Jan and GPT4All.
- Mined the LM Studio and Jan bug trackers with GitHub search for counts and representative issues
  (rocm 109, vulkan 282, intel arc 22 in lmstudio-bug-tracker; arm64+gpu 27, vulkan 265, rocm 40 in
  janhq/jan).
- Read EZLM's scanner and engine downloader to compare: `internal/scan/scan.go`,
  `internal/scan/gpu_discrete.go`, `internal/scan/scan_darwin.go`,
  `internal/scan/display_windows.go`, `internal/engine/engine.go`.

## Findings

1. **Windows on Arm is CPU-only across the whole set** (blocker for the beta's "it uses your GPU"
   promise on Copilot+ PCs). LM Studio supports the OS but reports 0 GPUs and 0 GB VRAM on a
   Snapdragon X2 Elite Extreme (#2577, 2026-11-05). Ollama ships an Arm64 Windows zip with no GPU
   payload. Jan ships an Arm64 installer. GPT4All ships an Arm64 installer (last release 2025-02-25).
   llamafile does not support Windows Arm at all. EZLM inherits this: `vendorOf` does not know
   Qualcomm, so a Snapdragon machine gets `win-cpu-arm64`.
2. **The Arm builds exist upstream and nobody uses them.** `win-vulkan-arm64` and
   `win-opencl-adreno-arm64` are in every current llama.cpp release. No GUI product in this set
   selects either. This is a cheap differentiator for EZLM.
3. **ROCm on Windows is real but narrow** (RX 7000 and PRO W7000 only, per Ollama; RDNA2 often
   cannot get ROCm 7 at all). Vulkan is the documented fallback and Ollama recommends it there.
   EZLM declares a `ROCm` backend but never selects it, and maps it to the Vulkan asset.
4. **Vulkan has documented failure modes that will look like EZLM bugs**: DeviceLost crashes on RX
   9070 XT in newer llama.cpp runtimes (LM Studio #2524), performance degradation after another
   Vulkan workload (#2426), and free-VRAM reporting that needs `cap_perfmon` or root (Ollama docs).
5. **Intel is the least-served vendor**: only LocalAI ships SYCL; everyone else is Vulkan. llama.cpp
   publishes `win-sycl-x64`, so this is a product choice, not a technical limit.
6. **CUDA 13 drops Pascal** and Jan 0.8.8 broke on every Pascal card because of it (#9501). EZLM's
   CUDA-12-first rule already avoids this and should be kept.
7. **EZLM's unified-memory flag is macOS-only**, so any iGPU it does detect on Windows is treated as
   discrete and its free VRAM is summed. On Windows Arm that makes the planner wrong.

## What I changed

Docs only. Added `agents/T14/a11/platforms.md` and this report. No code, no other paths.

## Sources (all accessed 2026-11-06)

- <https://api.github.com/repos/ggml-org/llama.cpp/releases?per_page=5> (b12177 asset list)
- <https://docs.ollama.com/gpu>, <https://docs.ollama.com/windows>
- <https://lmstudio.ai/docs/app/system-requirements>
- <https://github.com/lmstudio-ai/lmstudio-bug-tracker/issues/2577>, `/255`, `/2524`, `/2426`,
  `/2358`; searches `?q=rocm` (109), `?q=vulkan` (282), `?q=intel+arc` (22)
- <https://api.github.com/repos/ollama/ollama/releases/latest> (v0.40.15)
- <https://api.github.com/repos/janhq/jan/releases/latest> (v0.8.8); issues #8529, #9501, #8266;
  searches arm64+gpu (27), vulkan (265), rocm (40)
- <https://api.github.com/repos/nomic-ai/gpt4all/releases/latest> (v3.10.0)
- <https://docs.mozilla.ai/llamafile/reference/support>; llamafile 0.10.6 release notes
- <https://localai.io/docs/features/gpu-acceleration/>
- <https://docs.msty.app/>, <https://docs.anythingllm.com/> (wrapper behaviour only)

## Could not verify

- Whether `win-vulkan-arm64` works on current Adreno drivers in practice; no hardware, and no product
  ships it.
- LM Studio's exact ROCm card list on Windows: their docs no longer have a per-vendor GPU page, and
  the bug tracker is the only signal.
- Jan/GPT4All GPU behaviour on Windows Arm: inferred from asset names and docs, not tested.
- Msty and AnythingLLM platform behaviour: they wrap Ollama or LM Studio, so it is inherited; marked
  inferred.

## Open questions for the lead

- Should EZLM ship a HIP build for the RX 7000 allowlist in the beta, or stay Vulkan-only and revisit?
- Is "CPU only" an acceptable answer on Snapdragon for the first beta, provided the UI says so?
- Do we want an `EZLM_BACKEND` escape hatch documented for users whose Vulkan driver is broken? The
  variable already exists in the code.
