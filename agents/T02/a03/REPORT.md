# T02 a03 report: 8 GB tier

## What I did

Proposed six catalog-ready models for 8 GB of usable GPU or unified memory, in
`agents/T02/tiers/tier-8.json`, with the ranked rationale in
`agents/T02/tiers/tier-8.md`.

- Ornith 1.5 9B (new, MIT) at Q5_K_M 6.19 GiB / Q4_K_M 5.38 GiB
- Granite 4.2 8B (existing, official AA v5 5.1) at Q5_0 5.68 GiB
- Qwen3.5 9B (existing) at UD-Q5_K_XL 6.28 GiB
- Gemma 4 12B (existing) at UD-Q3_K_XL 5.61 GiB
- Nemotron 3.5 Bolt E4B (new, OpenMDW-1.1) at UD-Q5_K_XL 5.43 GiB, full 64K
- LFM2.5 8B A1B (new MoE, 1B active) at UD-Q5_K_XL 5.95 GiB, full 128K

## How it was checked

- `python3 scripts/verify_catalog.py --catalog agents/T02/tiers/tier-8.json`:
  21 quant file sets, **0 errors, no warnings** (llama.cpp b11960). The script read the
  real byte sizes from the HF tree API and the real GGUF headers by range request.
- Header numbers (layers, kv_heads, head_dim, kv_layers, ctx, sliding-window fields)
  come from the files themselves, not model cards. Notable finds: Ornith 1.5 9B is
  `qwen35` with 32 layers + 1 NextN head and full attention every 4th layer (8 KV
  layers); Nemotron Bolt has `shared_kv_layers=18`, so only 24 layers allocate a cache
  and 64K context costs ~1.1 GiB; Gemma 4 12B has a fixed 0.94 GiB sliding-window cache
  at any context; LFM2.5 8B A1B keeps a cache on only 6 of 24 blocks (12 KiB/token).
- Fit math follows `internal/fit/fit.go`: budget 7.68 GiB on a discrete Linux GPU,
  7.0 GiB unified/Windows, `overhead = 320 MiB + 4 KiB*ctx + 1% weights`, KV cache from
  the header numbers plus the sliding-window term. Every "best quant" in the table fits
  7.0 GiB at 8K or more.

## Sources

- https://huggingface.co/ornith-ai/Ornith-1.5-9B (card dated 2026-08-19; vendor
  benchmark table, five-run means)
- https://huggingface.co/nvidia/NVIDIA-Nemotron-3.5-Bolt-E4B-4GB (2026-10-05; benchmark
  table against Gemma-4-E4B and Qwen3.5-4B)
- https://huggingface.co/LiquidAI/LFM2.5-8B-A1B (2026-05-28; AA-Omniscience, IFEval,
  IFBench, MATH500, AIME25, BFCL, Tau2)
- https://artificialanalysis.ai/ Intelligence Index v5.0, fetched 2026-11-02 (Granite
  4.2 8B official score, already in the catalog)
- HF tree API and GGUF headers for every quant, read 2026-11-06

## What I could not verify

- Ornith 1.5 9B and Nemotron Bolt have no independent evaluation (AA, LMArena,
  LiveBench) that I could find; their scores are ESTIMATEs anchored to catalog models and
  the `score_source` says so.
- Ornith AI's country is recorded as US from the vendor site; I did not find a registry
  confirmation.
- LFM2.5 8B A1B is under the LFM Open License v1.0, which has commercial conditions;
  the exact terms were not reviewed.

## Rejected

Ternary Bonsai 2 27B (7.10 GiB weights + 0.5 GiB KV + 0.4 GiB overhead = 8.0 GiB, just
over), LFM2.5 24B A2B (6.96 GiB weights, fits only 7.68 GiB at 8K), Gemma 4 26B A4B,
Qwen3.6 35B A3B, Muse Glimmer 30B, Devstral Small 2 24B, Qwen3.6 27B, Nanbeige 4.3 33B,
Gemma 4 31B, Ornith 1.5 35B A3B, North Mini Code 1.0, and Qwen3.8 9B (does not exist).
Reasons are in the markdown.

## Next steps

- If AA publishes v5 numbers for Ornith 1.5 9B or Bolt E4B, replace the estimates.
- Revisit LFM2.5 24B A2B if the tier is ever split into "8 GB discrete VRAM" and "8 GB
  unified"; it is the best model that fits the former at 8K.
- Check whether unsloth publishes smaller Ornith 1.5 35B A3B quants; none existed on
  2026-11-06.
