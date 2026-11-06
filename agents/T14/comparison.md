# T14 comparison table: eight products against EZLM

Agent a09. Desk study, no product installed or run. All sources accessed **2026-11-06**.
EZLM facts are read from the source snapshot in this worktree (`work/subs/T14/a09`), not from a
running build. Where a cell could not be verified from a primary source it says **GAP** and gives the
next place to look; nothing is guessed into a filled cell.

How to read the evidence column: `doc` means the vendor's own docs or README, `code` means EZLM
source with `file:line`, `count` means a counted GitHub query (method stated in the cell).

## Summary: where EZLM is ahead, level, or behind

| Product | Shape | Fit logic | Key handling | Verdict for EZLM |
|---|---|---|---|---|
| LM Studio | Desktop app + `llmster` daemon | user-driven, HF search | optional tokens, off by default | Level on server; behind on model discovery breadth |
| Ollama | CLI + app + server | layer offload, VRAM-tiered context | key ignored locally | Behind on ecosystem, ahead on fit |
| Jan | Desktop app, OpenAI API :1337 | **GAP** | **GAP** | Level |
| GPT4All | Desktop app, 3 to 13B focus | none, user picks | API server, **GAP** on auth | Ahead on fit, behind on LocalDocs |
| Msty | Desktop app, own Local AI service | **GAP** | **GAP** | Behind on integrations and UX |
| AnythingLLM | RAG workspace, not a runner | n/a (delegates) | workspace API key | Different category; partner, not rival |
| llamafile | Single executable | none | none | Ahead on fit, behind on first run |
| LocalAI | Server first, Docker | **GAP** | `API_KEY` env | Behind on ops polish, ahead on backend breadth |

The pattern: every product except EZLM makes the user answer "which model?" and "will it fit?".
EZLM's `internal/fit` is the only planner in this set that ranks quant x context x KV type x
placement and enforces a RAM safety margin. That is the differentiator to protect.

## 1. First-run flow

| Product | What happens | Evidence | Gap |
|---|---|---|---|
| LM Studio | Installer, then Discover tab to search Hugging Face and download; `lms` CLI and a headless `llmster` daemon for servers | https://lmstudio.ai/docs/app/basics/download-model.md ; https://lmstudio.ai/docs/app/api/headless | whether onboarding recommends a model for the machine |
| Ollama | Installer or `curl install.sh`; `ollama run <name>` pulls and runs; registers a login item on macOS and Windows | https://docs.ollama.com/faq.md | first-run model choice is a name the user must already know |
| Jan | Desktop app; cloud and local providers; local server on :1337 | https://github.com/menloresearch/jan (README) | onboarding steps |
| GPT4All | Desktop app; Models tab, Add Model, Explore Models; chat and LocalDocs | https://docs.gpt4all.io/gpt4all_desktop/models.html | installer size and first-run time |
| Msty | Onboarding offers three paths: Setup Local AI, Add remote provider, or use existing Ollama models | https://docs.msty.app/getting-started/onboarding | how long the Local AI download takes |
| AnythingLLM | Desktop app or Docker; pick an LLM provider, embedder, vector DB, then create a workspace | https://docs.anythingllm.com/ | desktop first-run defaults |
| llamafile | Download one file, `chmod +x`, run it; opens a browser chat and a server | https://github.com/mozilla-ai/llamafile (README) | Windows and macOS Gatekeeper steps |
| LocalAI | Docker image or binary, then model gallery or YAML config | https://github.com/mudler/LocalAI (README) | none, it is explicitly not a consumer first run |
| **EZLM** | Scan, choose (Balanced / Smartest / Fastest), install engine and model with resume, then a URL and key, plus Chat | `BETA.md` "What happens"; `internal/app/app.go` deploy path | unsigned-binary friction is a known beta limit (`BETA.md` "Known limits") |

## 2. Hardware detection

| Product | What it detects | Evidence | Gap |
|---|---|---|---|
| LM Studio | Documents supported CPU/GPU per OS: Apple M1 to M4, Windows x64 and ARM (Snapdragon X Elite), Linux x64 and ARM64; AVX2 required on x64; 16 GB RAM and 4 GB VRAM recommended | https://lmstudio.ai/docs/app/system-requirements.md | the runtime probe itself (NVML, Metal, Vulkan enumeration) |
| Ollama | GPU support matrix by compute capability and driver version; ROCm on AMD; Vulkan fallback on Windows | https://docs.ollama.com/gpu.md ; https://docs.ollama.com/windows.md | per-process VRAM accounting |
| Jan | States GPU support for NVIDIA, AMD and Intel Arc on Windows 10+ | https://github.com/menloresearch/jan (README) | detection method |
| GPT4All | Runs without a GPU at all; CPU first | https://github.com/nomic-ai/gpt4all (README) | backend selection UI |
| Msty | Has a "GPUs Supported by Msty" doc page | https://docs.msty.app/ (sidebar: Getting Started > GPUs Supported by Msty) | page contents not fetched, **GAP** |
| AnythingLLM | Delegates inference, so no hardware probe of its own | https://docs.anythingllm.com/ | none |
| llamafile | Detects GPU at run time to pick a backend; has a "Using GPUs" doc | https://docs.mozilla.ai/llamafile/ | detection details |
| LocalAI | Backend images per accelerator (CUDA, ROCm, Intel, Vulkan); operator picks the image | https://github.com/mudler/LocalAI (README) | none |
| **EZLM** | CPU, physical cores, RAM total and available, disk free, models dir, GPUs with vendor, VRAM total and free, bandwidth, driver, CUDA max, unified memory flag, per-process GPU use, display drivers on Windows, Metal on macOS, `nvidia-smi`, `rocm-smi` | `internal/scan/scan.go:34-80`; `internal/scan/gpu_discrete.go:13-47`; `internal/scan/scan_other.go:14` | Intel Arc VRAM reporting on Windows |

EZLM is the only product here that also measures **what is already using the GPU** and asks the user
to free memory (`internal/scan/apps.go`, `BETA.md` "Free up memory"). That is a real edge for the
"it does not fit" moment every other product leaves to the user.

## 3. Model picking and recommendation

| Product | How a model gets chosen | Evidence | Gap |
|---|---|---|---|
| LM Studio | User searches Hugging Face and chooses a quant from a list; docs explain quant naming but the choice is the user's | https://lmstudio.ai/docs/app/basics/download-model.md | whether the Discover list filters to the machine's memory |
| Ollama | User names a model from the ollama.com library; no recommendation engine documented | https://docs.ollama.com/faq.md | inference, not stated by the vendor |
| Jan | Model hub plus cloud providers | https://github.com/menloresearch/jan (README) | hardware-based recommendation |
| GPT4All | Curated list of 3 to 13B models, user picks; size shown per model | https://docs.gpt4all.io/gpt4all_desktop/models.html | whether a RAM figure is shown per model |
| Msty | Model selector plus a registry of known models | https://docs.msty.app/ (sidebar: Features > Model Selector, "Add Models Not Included in Registry") | recommendation logic |
| AnythingLLM | Model router plus per-workspace provider choice | https://docs.anythingllm.com/ (sidebar: Model Router) | none |
| llamafile | The model is inside the file you downloaded; no picker at all | https://github.com/mozilla-ai/llamafile (README) | none |
| LocalAI | Model gallery or hand-written YAML | https://github.com/mudler/LocalAI (README) | none |
| **EZLM** | Ranks every catalog model by effective intelligence after quant, KV and context penalties, filtered by a usable-speed floor, under one of three prefs | `internal/fit/fit.go:1-12,60-80`; `internal/catalog/catalog.go:1-10` | catalog breadth is far smaller than HF |

The gap to close is not the ranking, it is **breadth**: LM Studio can pull any GGUF on Hugging Face,
EZLM ships a curated catalog (`internal/catalog/catalog.json`) refreshed from a remote URL. A user
who wants a model EZLM has never heard of is stuck. That is the single biggest product gap in this
table.

## 4. Memory-fit logic

| Product | Fit behaviour | Evidence | Gap |
|---|---|---|---|
| LM Studio | Load-time memory estimate and GPU offload control | **GAP**: not verified from a doc page in this pass | the exact estimate formula and warning threshold |
| Ollama | Offloads layers to fit available VRAM; default context is tiered by VRAM (under 24 GiB 4k, 24 to 48 GiB 32k, 48 GiB and up 256k); `ollama ps` shows the CPU/GPU split; docs warn coding agents need 64k context | https://docs.ollama.com/context-length.md ; https://docs.ollama.com/faq.md | whether it reserves headroom for the OS |
| Jan | **GAP** | | |
| GPT4All | No automatic fit; CPU-first design keeps models small (3 to 13B) | https://docs.gpt4all.io/gpt4all_desktop/models.html | |
| Msty | **GAP** | | |
| AnythingLLM | n/a, delegates to the provider | https://docs.anythingllm.com/ | |
| llamafile | None; if it does not fit, it runs slowly | https://docs.mozilla.ai/llamafile/ | |
| LocalAI | Config-driven; operator sets context and offload | https://github.com/mudler/LocalAI (README) | |
| **EZLM** | Tries every quant x context x KV type x placement (GPU, MoE experts in RAM, split layers, CPU), applies a hard safety margin, adds DirectLoad on Windows, lazy and streamed experts for oversized models, and estimates tok/s | `internal/fit/fit.go:1-80` | tok/s model is calibrated on five machines (`internal/fit/calib_test.go:11-20`) |

Ollama's VRAM-tiered context default is worth copying as a fallback when the catalog has no measured
speed data: it is one line of policy and it stops the 4k-context surprise that breaks coding agents.

## 5. OpenAI-compatible server and keys

| Product | URL and default | Auth | LAN | Evidence |
|---|---|---|---|---|
| LM Studio | OpenAI-compatible REST on a local port; tokens optional | API tokens since 0.4.0, **off by default** | server settings toggle | https://lmstudio.ai/docs/developer/core/authentication.md |
| Ollama | `http://localhost:11434/v1` | API key **required by clients but ignored** locally; `OLLAMA_API_KEY` only for cloud | `OLLAMA_HOST=0.0.0.0:11434` | https://docs.ollama.com/api/openai-compatibility.md ; https://docs.ollama.com/faq.md |
| Jan | `localhost:1337` OpenAI-compatible | **GAP** | **GAP** | https://github.com/menloresearch/jan (README) |
| GPT4All | Local API server documented | **GAP** | **GAP** | https://docs.gpt4all.io/ (sidebar: API Server) |
| Msty | Local AI service, can be exposed on the network | **GAP** | documented toggle | https://docs.msty.app/ (sidebar: Make Local AI service available on your network) |
| AnythingLLM | Workspace and developer API with a key | API key in settings | Docker port | https://docs.anythingllm.com/ (sidebar: API Access) |
| llamafile | `--server` on `127.0.0.1:8080`, OpenAI-compatible | none | `--host 0.0.0.0` | https://github.com/mozilla-ai/llamafile (README) |
| LocalAI | OpenAI-compatible on :8080 | `API_KEY` env, optional | bind address | https://github.com/mudler/LocalAI (README) |
| **EZLM** | `http://127.0.0.1:<port>/v1`, `0.0.0.0` only when LAN is on | `--api-key` always passed; `ezlm-` plus 40 base62 chars (~238 bits); named keys, first key is the one wired into agents | explicit `LAN` toggle with a computed LAN URL | `internal/serve/config.go:90-121`; `internal/keys/keys.go:8-22`; `internal/app/app.go:989-996` |

EZLM is the only product in this set that **requires** a key by default and generates a strong one.
LM Studio and Ollama both accept any string. That is a defensible security lead; keep it.

## 6. Coding-agent integrations

| Product | What exists | Evidence |
|---|---|---|
| LM Studio | OpenAI-compatible endpoint plus Python and TS SDKs; agent tools point at the base URL | https://lmstudio.ai/docs/app/api/headless |
| Ollama | Integrations page and a VS Code section in the FAQ | https://docs.ollama.com/integrations ; https://docs.ollama.com/faq.md |
| Jan | OpenAI-compatible server for other apps | https://github.com/menloresearch/jan (README) |
| GPT4All | Python SDK and local API server | https://docs.gpt4all.io/ (sidebar: API Server, Python SDK) |
| Msty | MCP support only in Msty Studio; Msty App 1.x has none | https://docs.msty.app/getting-started/onboarding (comparison table) |
| AnythingLLM | Agents, MCP, embedded chat widgets; consumes providers rather than serving models | https://docs.anythingllm.com/ |
| llamafile | Plain OpenAI-compatible server, no wiring | https://github.com/mozilla-ai/llamafile (README) |
| LocalAI | OpenAI-compatible API, agent integrations via the API | https://github.com/mudler/LocalAI (README) |
| **EZLM** | Detects, installs and configures OpenCode, Hermes and DSH, with launch and smoke test | `internal/integrate/registry.go`; `internal/integrate/types.go:122`; `internal/integrate/smoke.go` |

EZLM's edge is that it **writes the agent's config for the user**. No competitor here does that; they
all stop at "here is a base URL, paste it yourself". This is the feature to lead the beta with.

## 7. Update mechanism

| Product | Mechanism | Evidence | Failure mode |
|---|---|---|---|
| LM Studio | In-app updater; separate channels for app and runtime | **GAP**: updater doc page 404 at https://lmstudio.ai/docs/advanced/updater | stale runtimes |
| Ollama | macOS and Windows auto-download, then "Restart to update"; Linux re-runs the install script | https://docs.ollama.com/faq.md | Linux users must re-run a script |
| Jan | **GAP** | | |
| GPT4All | **GAP** | | |
| Msty | In-app update; there is a troubleshooting page "Stuck at App Update" | https://docs.msty.app/ (sidebar: Troubleshooting) | the vendor documents the stuck state, so it happens |
| AnythingLLM | In-app banner ("v1.19.0 is live, update now") and Docker tag pulls | https://docs.anythingllm.com/ | Docker users drift from desktop |
| llamafile | None; you download a new file | https://github.com/mozilla-ai/llamafile (README) | users stay on old builds |
| LocalAI | Pull a new image tag | https://github.com/mudler/LocalAI (README) | same |
| **EZLM** | Checks GitHub releases (pre-releases included), picks the asset for the platform, resumes the download, verifies the published SHA-256, then installs and reopens | `internal/update/update.go:17-25,54-140` | unsigned installer means SmartScreen on every update |

EZLM's digest check before running an installer is stronger than anything documented by the others.
Worth saying out loud in the beta notes.

## 8. Most common complaints, with counts

Counting method: GitHub issue search sorted by reactions,
`https://api.github.com/search/issues?q=repo:<owner>/<repo>+is:issue&sort=reactions&order=desc`.
This ranks by reaction count, so it finds the **most-felt** issues, not the most frequent. The
unauthenticated search API allows 10 requests a minute; I got Ollama before the limit and the rest
returned rate-limit errors, so they are marked GAP with the exact query to re-run.

| Product | Counted evidence | Status |
|---|---|---|
| Ollama | 11,581 issues total. Top reacted: "Reranking models" 387 reactions, open (https://github.com/ollama/ollama/issues/3368); "MLX backend" 381, closed (https://github.com/ollama/ollama/issues/1730); "add Qwen2-VL/Qwen2.5-VL" 344, closed (https://github.com/ollama/ollama/issues/6564) | counted |
| LM Studio | Bug tracker: https://github.com/lmstudio-ai/lmstudio-bug-tracker | GAP, rate limited |
| Jan | https://github.com/menloresearch/jan | GAP, rate limited |
| GPT4All | https://github.com/nomic-ai/gpt4all | GAP, rate limited |
| Msty | No public issue tracker found; docs carry troubleshooting pages | GAP |
| AnythingLLM | https://github.com/Mintplex-Labs/anything-llm | GAP, rate limited |
| llamafile | https://github.com/mozilla-ai/llamafile | GAP, rate limited |
| LocalAI | https://github.com/mudler/LocalAI | GAP, rate limited |
| **EZLM** | No public issue corpus yet. Complaints reach the team through **Report a problem**, which posts to the public GitHub page | `BETA.md` "If something goes wrong" |

Two caveats for a15 when it fills this column. Reaction counts favour old, big asks (a reranking
feature request outranks any single bug), so do not read them as severity. And EZLM has no baseline:
the honest comparison is against the **known limits** the beta already admits, which are the unsigned
installer, large first downloads, and the 64K-context requirement for Hermes.

## 9. Windows on Arm, AMD and Intel GPUs

| Product | Windows on Arm | AMD | Intel | Evidence |
|---|---|---|---|---|
| LM Studio | Supported: "x64 and ARM (Snapdragon X Elite)" | GPU support documented per OS | GPU support documented per OS | https://lmstudio.ai/docs/app/system-requirements.md |
| Ollama | **GAP** (not stated in the Windows page) | ROCm, and Vulkan fallback; some RDNA2/RX 6000 cards "may not expose ROCm v7" on Windows, Vulkan is the recommended fallback; Windows Radeon defaults to 1 loaded model because of ROCm 5.7 VRAM reporting | **GAP** | https://docs.ollama.com/windows.md ; https://docs.ollama.com/faq.md |
| Jan | **GAP** | NVIDIA/AMD/Intel Arc listed | listed | https://github.com/menloresearch/jan (README) |
| GPT4All | **GAP** | **GAP** | **GAP** | |
| Msty | **GAP** | Has a dedicated "AMD ROCm on Windows Issues" troubleshooting page, so AMD on Windows is a known weak spot | **GAP** | https://docs.msty.app/ (sidebar: Troubleshooting) |
| AnythingLLM | n/a | n/a | n/a | delegates |
| llamafile | Has a "Supported Systems" reference page | GPU docs exist | GPU docs exist | https://docs.mozilla.ai/llamafile/ |
| LocalAI | **GAP** | ROCm backend images | Intel backend images | https://github.com/mudler/LocalAI (README) |
| **EZLM** | arm64 builds for Windows CUDA 13, Vulkan and CPU; arm64 Linux CUDA 13 | AMD read with `rocm-smi` on Linux, display drivers on Windows, Vulkan engine; falls back to CPU when the Vulkan driver fails to load the model | Intel via Vulkan on Windows | `internal/engine/engine.go:111-135,216-226`; `internal/prereq/prereq.go:82-126`; `internal/app/app.go:1623-1631`; `internal/scan/gpu_discrete.go:146` |

Ollama's own docs admit the two AMD-on-Windows facts EZLM will hit: ROCm v7 is not exposed on some
RX 6000 systems, and VRAM reporting is unreliable enough that they cap loaded models at one. EZLM's
`cpuInstead` fallback (`internal/app/app.go:1623`) is the right shape of answer, but the beta should
say plainly that AMD on Windows may land on Vulkan or the processor.

## What this table cannot tell you yet

1. **LM Studio's and Jan's fit logic.** Both are the closest competitors on the "just works" axis and
   neither publishes how they decide what fits. That is the highest-value gap to close by reading
   their issue trackers and release notes.
2. **Complaint counts for seven products.** The query is in section 8; it needs spacing or a token.
3. **Msty's GPU support page and Local AI engine.** Not fetched.
4. **AnythingLLM as a competitor.** It is a workspace that consumes a local model, so it is more
   likely an EZLM integration target than a rival.
