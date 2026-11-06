# T14. Competitive teardown — consolidated report

16 agents (a01–a16) worked this task in parallel: one per product (LM Studio, Ollama, Jan, GPT4All,
Msty, AnythingLLM, llamafile, LocalAI), plus the comparison table, the prioritised recommendations,
platform reality, the OpenAI-compatible server and keys, memory-fit logic, update mechanisms,
complaint mining, and report support. Consolidated onto `agent/T14-competitors` by the Lead.

Goal: learn from the eight products and turn it into concrete, prioritised recommendations for EZLM.

## Result

- **`agents/T14/products/a01..a08.md`** — one teardown per product, in the brief's shape (first-run,
  detection, model picking, memory fit, server and keys, coding-agent integrations, updates,
  complaints with links and counts, Windows-on-Arm / AMD / Intel issues, recommendations).
- **`agents/T14/comparison.md`** — 8 products + EZLM across the nine dimensions, one table per
  dimension.
- **`agents/T14/recommendations.md`** — 31 ranked recommendations, each with evidence, effort (S/M/L)
  and the EZLM area it touches, in three tiers.
- **`agents/T14/a15/complaints.md`**, **`a11/platforms.md`**, **`a12/servers.md`**,
  **`a13/fit.md`**, **`a14/updates.md`** — the cross-cutting analyses.
- Every claim carries a URL and an access date (all 2026-11-06). Reddit's JSON endpoint returned 403
  from this network, so complaint counts are GitHub-only and labelled as lower bounds.

## What the field does, and where EZLM is ahead

1. **No competitor plans the fit.** LM Studio, Jan, GPT4All, Msty, LocalAI and llamafile make the
   user pick a model and guess whether it fits; Ollama only offloads layers and tiers context by VRAM
   (4K below 24 GiB, while its own docs say agents need 64K). **EZLM's `internal/fit` planner is the
   differentiator** — the recommendations say to surface it, not replace it.
2. **EZLM is the only one of the eight that requires an API key from first run.** Seven ship an
   unauthenticated localhost API; Ollama and LocalAI let users expose it on 0.0.0.0 with no key.
   Keep auth-on-by-default and make it the headline.
3. **EZLM is the only one that writes the coding-agent configs** (OpenCode, Hermes, DeepSeek Harness)
   instead of documenting a manual step.
4. **Model breadth is EZLM's biggest gap**: LM Studio pulls any HF GGUF; EZLM ships a curated
   catalog (T02's work is the answer to that).
5. **The field's biggest complaint class is memory fit** (Ollama 73 "out of memory" titles,
   LM Studio 36 "failed to load", GPT4All 51 open bugs), and the second is downloads (Jan #7728,
   32 comments) — where EZLM already resumes and verifies.

## The recommendations that matter most

- **R01 — unload the model when idle, and say when it will unload.** LM Studio documents a 60-minute
  TTL and auto-evict; EZLM pins `-c`/`-np 1` from login and never releases the model.
- **R02 — the updater picks installers by filename prefix only** (`internal/update/update.go:43-51`),
  so Windows on Arm can be handed the x64 installer (effort S).
- **R03 — sign the Windows installer and notarize the macOS build.** Every product without it pays the
  SmartScreen/Defender tax; llamafile alone has 14 Defender-related issues.
- **R04 — API keys are stored in plaintext in `config.json`** (T07 already tightened the file mode;
  this is the next step).
- **R05 — add an Anthropic Messages surface** (`/v1/messages`, `count_tokens`, `x-api-key`):
  llamafile ships it, and EZLM does not, so Anthropic-native agents cannot use an EZLM deployment.
- **R06 — add a Host allowlist when LAN is on.** Jan shipped exactly this hole and fixed it with
  GHSA-x6p8-7cp8-c3p6.
- **R07 — show the fit verdict** (model, quant, context, X of Y GB, tok/s) on the plan card before
  the download.
- **R08 — copy `ollama launch`-style auto-wiring** for Claude Code / Codex / OpenCode.

## Platform reality

- **Windows on Arm:** EZLM falls back to the CPU build because `vendorOf()` does not know Qualcomm
  (`internal/scan/gpu_discrete.go:231`), while llama.cpp b12177 ships `win-vulkan-arm64` and
  `win-opencl-adreno-arm64` that no GUI product uses. AnythingLLM's two most-commented issues are
  exactly the Snapdragon NPU not working.
- **AMD/ROCm:** EZLM declares a ROCm backend but maps it to the Vulkan asset
  (`internal/engine/engine.go:125`); ROCm on Windows is now real but narrow.
- **Intel Arc:** the top-reacted LM Studio issues are Intel Mac (508 reactions) and Intel GPU
  backends; Vulkan is the reliable path.
- **Unified memory is macOS-only in EZLM**, so any Windows iGPU is treated as discrete and its free
  VRAM summed.

## Cross-cutting findings for other tasks

- **`internal/update/update.go:128-158` compares prerelease suffixes as strings**, so
  `0.1.11-beta.10` sorts below `beta.2` (reproduced) — the same class T08 fixed, and T11 a06/a12
  independently found.
- **The update feed includes pre-releases for everyone**, and there is no stable channel, no pin and
  no rollback.
- **The picker already shows** smarts/speed bars, words/s, context, quant and GPU/CPU placement
  (`windows/EZLM/Panes.cs:67-92`), and keys already have new/rotate/revoke (`Panes.cs:689-717`) — do
  not recommend those as missing.
- **`fit.go` still has the 8K ladder floor and the unified-memory gap** that T04 documented; the
  teardown independently flags both as costing 6–8 GB machines a plan.

## Limits of this study

Desk study only: no competitor product was installed or run. Reddit counts are missing (403). GitHub
search counts open issues only and are lower bounds. LM Studio's and Jan's internal fit logic are
undocumented, so those rows are partly inferred. Msty is unfetchable from this network for some
pages, so its section is partly qualitative.

## Files

- `agents/T14/products/a01..a08.md` — the teardowns.
- `agents/T14/comparison.md`, `recommendations.md`, `a15/complaints.md`, `a11/platforms.md`,
  `a12/servers.md`, `a13/fit.md`, `a14/updates.md`, `a16/REPORT-support.md`.
- `agents/T14/a01..a16/REPORT.md` — one report per agent, with sources and access dates.

## Next steps

1. Turn R01, R02 and R04 into issues first (the cheapest high-value ones).
2. Reconcile `recommendations.md` with the eight product sections now that they are merged (a10 wrote
   its list before they landed).
3. Decide the model-breadth question (curated catalog vs any HF GGUF) — it is the one place the
   field is clearly ahead.
