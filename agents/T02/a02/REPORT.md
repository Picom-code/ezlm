# T02 a02 report: 6 GB tier

## What I did

- Read `README.md`, `run/T02-brief.md`, `TASKS.md` (T02) and `internal/catalog/catalog.json`, then researched the current small-model landscape for a 6 GB budget on 2026-11-06.
- Wrote four catalog-ready entries in `agents/T02/tiers/tier-6.json`: K2 Horizon 7B, K2 Horizon 3.7B, Ministral 3 8B Reasoning, LFM2.5 8B A1B.
- Read every GGUF header with HTTP range requests and every file size from the Hugging Face tree API. `scripts/verify_catalog.py --catalog agents/T02/tiers/tier-6.json` passes: 11 file sets, 0 errors.
- Wrote `agents/T02/tiers/tier-6.md` with the ranked table, the fit math and every drop.

## Findings

1. **The catalog has no 6 GB-class entry from the September 2026 release wave.** The smallest current entries are Gemma 4 12B (3.5), Qwen3.5 9B (5.0) and Granite 4.2 8B (5.1); the K2 Horizon family (released 2026-09-03, AA) is absent. Severity: major opportunity for the 6 GB tier, not a bug.
2. **K2 Horizon 7B is the strongest agentic model that fits 6 GB.** AA measured Terminal-Bench 2.1 44.2, tau3-Banking 27.6 and HLE 18.2, above Qwen3.5 9B (29.2 / 7.0 / 14.9) and Gemma 4 12B (27.3 / n/a / 15.7). Severity: major, it changes the recommended pick for this tier.
3. **The official K2 Horizon 7B GGUF starts at Q4_K_M (5.59 GB) and does not fit 6 GB with KV cache.** At 16K context and q4_0 KV the Q4_K_M file needs about 6.3 GiB resident, over the 5.6 GiB budget. The bartowski IQ3_XXS (4.07 GB) or Q3_K_M (4.42 GB) files are required. Severity: major for the entry, and a reason to list community quants.
4. **K2 Horizon 3.7B GGUF files are named `K2-Horizon-4B-*`.** Anyone matching files to model size by name will overstate the parameters. Severity: minor, handled by the entry (`params_b` 3.7, repo `IFM/K2-Horizon-3.7B-GGUF`).
5. **Gemma 4 12B already fits 32K in 6 GB** thanks to 8 global layers with 1 KV head x 512 and 40 sliding-window layers (catalog `gemma-4-12b`, `kv_heads` 1, `kv_layers` 8, `swa_*` set). This confirms the tier math and means the 6 GB tier is not limited to 4B models. Severity: informational.
6. **EZLM's fit planner starts its context ladder at 64K (`internal/fit/fit.go:110`), and a 6 GB card cannot hold 64K KV for any 7B-class model here.** The planner will fall to 32K, 16K or 8K; the entries list quants that fit at 16K or 32K with q4_0 KV so a plan exists. Severity: informational, no code change proposed (T04 owns the planner).

## Sources (all accessed 2026-11-06)

- Artificial Analysis leaderboard, https://artificialanalysis.ai/leaderboards/models
- AA model pages: https://artificialanalysis.ai/models/k2-horizon-7b, /k2-horizon-3-7b, /ministral-3-8b, /lfm2-5-8b-a1b
- K2 Horizon 7B card, https://huggingface.co/IFM/K2-Horizon-7B
- K2 Horizon 3.7B card, https://huggingface.co/IFM/K2-Horizon-3.7B
- Ministral 3 8B Reasoning card, https://huggingface.co/mistralai/Ministral-3-8B-Reasoning-2512
- LFM2.5 8B A1B card, https://huggingface.co/LiquidAI/LFM2.5-8B-A1B
- GGUF trees: bartowski/K2-Horizon-7B-GGUF, IFM/K2-Horizon-3.7B-GGUF, unsloth/Ministral-3-8B-Reasoning-2512-GGUF, LiquidAI/LFM2.5-8B-A1B-GGUF
- llama.cpp b11960 `src/llama-arch.cpp`, for architecture support (`k2-horizon`, `mistral3`, `lfm2moe` all present)

## What I could not verify

- AA has no v5 Intelligence Index for any of the four models, so all four scores are estimates mapped from AA per-benchmark runs and the nearest catalog scores. Each `score_source` documents the mapping and says UNVERIFIED.
- Ministral 3 8B vendor reasoning numbers (AIME25 78.7, GPQA 66.8, LCB 61.6) are self-reported; AA's independent run is for the non-thinking instruct mode and is much lower.
- K2 Horizon vendor numbers (SWE-bench 72.4 for the 7B, AIME25 89.2 for the 3.7B) are self-reported; AA has no SWE-bench or AIME run for these models.
- Vision for Ministral 3 8B is not usable in EZLM today (`internal/integrate/types.go:21` says an image projector is never loaded).

## Next steps

- a16 should merge this file with the other tiers; ids are unique (`k2-horizon-7b`, `k2-horizon-3.7b`, `ministral-3-8b-reasoning`, `lfm2.5-8b-a1b`).
- a12 should consider whether Gemma 4 12B (3.5) is outclassed by K2 Horizon 7B (8.5) in the same 6 to 8 GB window.
- a15 can cross-check the four GGUF headers; `scripts/verify_catalog.py` already passes.
- If EZLM later ships mmproj loading, add `mmproj-F16.gguf` (858 MB) for Ministral 3 8B and set a vision-aware memory estimate.
