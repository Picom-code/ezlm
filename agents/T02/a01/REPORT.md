# T02 / a01 report: 4 GB tier

Agent: a01. Tier: 4 GB of usable GPU or unified memory. Branch: `agent/T02-a01`.
Deliverables: `agents/T02/tiers/tier-4.json` (3 models, 9 quant sets) and
`agents/T02/tiers/tier-4.md`.

## What I did

1. Read the T02 brief, the catalog schema (`internal/catalog/catalog.go`,
   `internal/catalog/catalog.json`) and the memory planner (`internal/fit/fit.go`)
   so the tier file is sized the way EZLM actually plans: 3.6 GiB on a 4 GB
   card under Linux/macOS, 3.0 GiB on Windows, 3.25 GiB on 4 GB unified.
2. Surveyed the November 2026 model landscape: Hugging Face org listings
   (Qwen, Google, Meta, Microsoft, IBM, LiquidAI, NVIDIA, Mistral, OpenBMB,
   InclusionAI, Nanbeige, IFM), the Artificial Analysis leaderboard, and the
   LMArena text leaderboard dataset.
3. Pulled the full AA model dataset out of the public model pages (726 model
   objects with `intelligenceIndex` and the v5.0.1 component scores). 108
   models have an official index and 98 have the full 9-component vector.
   Fitted a linear model of the index on those components (RMSE 1.6 points,
   matching the "RMSE 1.7" method the catalog notes cite) and a kNN variant
   over the 44 models that share the components K2 Horizon and MiniCPM5 have.
   Used those to bound the estimates for models AA has scored but not indexed.
4. Picked three models that fit with a usable context, verified their GGUF
   sources and metadata with HTTP range requests (no weights downloaded), and
   ran `scripts/verify_catalog.py --catalog agents/T02/tiers/tier-4.json`
   against llama.cpp b11960: 9/9 quant sets ok, 0 errors.
5. Wrote the ranked table, per-pick reasoning, and the drop list in
   `tier-4.md`.

## Picks

| Rank | id | Why | Best 4 GB quant |
|---|---|---|---|
| 1 | `nanbeige4.2-3b` | Best reported 3-4B reasoning/coding; 16K context at Q4_K_M on Linux | Q4_K_M, 2.68 GB |
| 2 | `k2-horizon-3.7b` | Best agentic/terminal with independent AA component scores; 512K native | IQ3_XXS, 2.32 GB |
| 3 | `minicpm5-2b` | Smallest footprint; 64K context fits in 2.77 GiB | Q4_K_M, 1.56 GB |

All three are dense, Apache-2.0, tool-calling, text-only, and validated
against b11960 (`nanbeige` and `k2-horizon` architectures are both known).

## Findings

- **AA coverage of small open models is thin.** As of 2026-11-06 only Granite
  4.2 3B (4.72) and Granite 4.2 8B (5.10) have official v5.0.1 indexes below
  15B. K2 Horizon 3.7B and MiniCPM5 2B have component scores but no index;
  Nanbeige4.2 3B is not in AA at all. That is why all three scores are
  ESTIMATEs with the method written into `score_source`, as the brief allows.
- **The 4 GB tier is KV-bound, not weight-bound.** K2 Horizon's 36 layers x
  8 KV heads x 128 cost 144 KiB/token at f16. The difference between a
  workable plan and an OOM is the KV type and context, not the quant: IQ3_XXS
  at 8K/q4_0 is 2.84 GiB, the same file at 32K/f16 is 7.12 GiB.
- **Schema gap: per-layer embedding tables.** NVIDIA's Nemotron 3.5 Bolt E4B
  4GB is marketed for 4 GB because Gemma 4 E4B's PLE tables can stay in system
  RAM, but the catalog's `Quant.Bytes` counts the whole file (3.53 GB at the
  smallest quant) and `internal/fit` treats it as resident. A `cpu_resident`
  byte count on `Quant` would let the planner offer this class of model.
  Recorded in `tier-4.md` as an open question; I did not change Go code (T02
  owns only `agents/T02/`).
- **No MoE fits 4 GB.** The smallest credible MoE, Ling 3.0 Tiny (7.9B total,
  1.3B active), starts at 3.00 GB for Q2_K, which is below EZLM's 0.94
  quant-quality floor for sub-20B models; its Q3 quants are 3.6-3.9 GiB.
- **LMArena could not confirm the picks.** The text leaderboard dataset
  (accessed 2026-11-06) has Granite 4.2 3B/8B and Gemma 4 but none of the
  three picks, so AA components and vendor cards are the only evidence.

## Sources (all accessed 2026-11-06)

- https://artificialanalysis.ai/leaderboards/models (v5.0.1 leaderboard)
- https://artificialanalysis.ai/models/k2-horizon-3-7b (component scores)
- https://artificialanalysis.ai/models/minicpm5-2b (component scores)
- https://huggingface.co/IFM/K2-Horizon-3.7B (card, released 2026-09-03)
- https://huggingface.co/Nanbeige/Nanbeige4.2-3B (card, released 2026-07-21)
- https://huggingface.co/openbmb/MiniCPM5-2B (card, released 2026-09-07)
- https://huggingface.co/nvidia/NVIDIA-Nemotron-3.5-Bolt-E4B-4GB (card, released 2026-10-23)
- https://huggingface.co/bartowski/K2-Horizon-3.7B-GGUF
- https://huggingface.co/bartowski/Nanbeige_Nanbeige4.2-3B-GGUF
- https://huggingface.co/openbmb/MiniCPM5-2B-GGUF
- https://datasets-server.huggingface.co (LMArena `text/latest` rows)

## Could not verify

- The three `score` values. No official AA index exists for any of them; the
  estimates come from AA component scores (K2, MiniCPM5) or the vendor table
  (Nanbeige4.2). Ranges are stated in each `score_source`.
- Nanbeige4.2 3B's vendor numbers. They are self-reported, in thinking mode,
  with the vendor's own scaffolds for SWE-bench/Terminal-Bench. If AA publishes
  a lower index, Nanbeige should drop below K2 Horizon.
- Real throughput on 4 GB hardware. No model was loaded (hard rule), so the
  tier file carries no speed claims; `internal/fit` estimates speed from
  bandwidth.

## Next steps

1. Re-check AA for official indexes for K2 Horizon 3.7B and MiniCPM5 2B and
   replace the estimates.
2. If another agent (a13/a14) finds an independent small-model leaderboard
   covering Nanbeige4.2, cross-check it.
3. Consider a `cpu_resident_bytes` (or PLE-aware) field in `catalog.Quant` so
   Nemotron 3.5 Bolt E4B 4GB and other Gemma 4 E4B derivatives can be offered
   on 4 GB machines.
4. If the 4 GB tier is later defined as 4 GiB of free memory after the OS
   (not a 4 GB card), K2 Horizon can move up to IQ3_M and Nanbeige to Q4_K_M
   at 16K f16; the tier file already lists those quants.
