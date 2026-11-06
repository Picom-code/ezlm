# Tier 16 GB: new model picks for November 2026

Scope: machines with 16 GB of usable GPU memory or 16 GB of unified memory.
This is the first tier where a 27-33B dense model runs fully resident with a
64K working context, so the picks are the models that keep the most quality
per byte of weights and per byte of KV cache, not the models that merely load.

Sizes below are decimal GB from the Hugging Face tree API
(`/api/models/<repo>/tree/main?recursive=true`, fetched 2026-11-06); budget
arithmetic uses GiB (1 GiB = 2^30 bytes), which is what `internal/fit` works
in.

Budget used for the fits: a 16 GB card gets **15.36 GiB** on Linux and macOS
(4% or 0.4 GiB margin), **14.72 GiB** on Windows (8% or 1 GiB margin), and a
16 GB unified machine gets **14.4 GiB** (10% margin). Resident memory is
weights + KV cache + sliding-window cache + llama.cpp overhead
(320 MiB + 4 KiB per token of context + 1% of the weights).

## Ranked table

| # | Model | Why it wins | Benchmark evidence (source, date) | Best quant for 16 GB | Licence |
|---|---|---|---|---|---|
| 1 | **Qwen3.8 27B** | The best fit at this tier: AA's top dense model under 70B, 48 Gated DeltaNet + 16 full-attention layers give a 64 KiB/token cache (6x cheaper than a full-attention 27B), so UD-IQ4_XS still holds 64K context. | [AA Intelligence Index v5.0](https://artificialanalysis.ai/leaderboards/models) leaderboard JSON, fetched 2026-11-02: 20.1 at Xhigh effort (Medium 17.5, Low 15.7, non-reasoning 10.0). Card: [Qwen/Qwen3.8-27B](https://huggingface.co/Qwen/Qwen3.8-27B). | UD-IQ4_XS, 14.25 GB (13.27 GiB); 64K with q4_0 KV totals 15.09 GiB (Linux), 64K q8_0 on Windows. UD-Q3_K_XL (13.15 GB) holds 64K at q8_0. | Apache-2.0 |
| 2 | **Nanbeige4.3 33B** | Highest dense score on AA v5.0, level with Qwen3.8-27B but stronger on agentic rows (AutomationBench 53% vs 48%, TB4 30% vs 6%). Its price is KV: full attention on all 52 layers costs 208 KiB/token at f16, so 16 GB means IQ2 weights at 64K. | [AA Intelligence Index v5.0](https://artificialanalysis.ai/leaderboards/models) leaderboard JSON, fetched 2026-11-02: 20.4 (highest dense model on v5). Card: [Nanbeige/Nanbeige4.3-33B](https://huggingface.co/Nanbeige/Nanbeige4.3-33B). | IQ2_S, 11.15 GB (10.38 GiB); 64K with q4_0 KV totals 14.71 GiB (fits Windows too). IQ3_XXS (14.49 GB) only reaches 16K. | Apache-2.0 |
| 3 | **Ternary Bonsai 2 27B** | The long-context and low-memory option: native ternary {-1,0,+1} Q2_0 of Qwen3.8-27B, 7.10 GiB of weights, so 128K context fits with q8_0 KV and 262K with q4_0 KV. 98.2% of the Qwen3.8-27B FP16 suite, within 0.4 pts of its UD-Q4_K_XL. | PrismML model card, released 2026-09-17: 84.78 vs 86.32 on a 14-20 benchmark suite; coding 89.4/89.1, agentic 74.9/76.7, knowledge 79.9/85.6 vs FP16. [prism-ml/Ternary-Bonsai-2-27B-gguf](https://huggingface.co/prism-ml/Ternary-Bonsai-2-27B-gguf). Score 18.0 is a hand-mapped estimate from those retention numbers (UNVERIFIED). | Q2_0 (the only file), 7.63 GB (7.10 GiB); 64K f16 totals 11.74 GiB, 128K q8_0 totals 12.24 GiB, 262K q4_0 totals 12.99 GiB. | Apache-2.0 |
| 4 | **Qwen3.6 35B A3B** | The fast MoE: 3B active of 35B, 20 KiB/token KV, so 64K context fits at UD-IQ3_S with f16 KV and 128K at q8_0. It is the pick when interaction speed matters more than peak quality. | [AA](https://artificialanalysis.ai/leaderboards/models) v5 components, linear fit (RMSE 1.7): predicted 9.5 (legacy fit 7.9); AA API reports 133 t/s. Card: [Qwen/Qwen3.6-35B-A3B](https://huggingface.co/Qwen/Qwen3.6-35B-A3B). | UD-IQ3_S, 13.68 GB (12.74 GiB); 64K f16 KV totals 14.68 GiB, 128K q8_0 totals 15.01 GiB (Linux). UD-IQ3_XXS (13.21 GB) is the Windows-safe one at 64K. | Apache-2.0 |
| 5 | **Gemma 4 31B** | The generalist and vision pick: strong tool calling and document QA, 50 sliding-window + 10 global layers, and the only pick with a mature QAT line. Weakest coder of the five. | [AA](https://artificialanalysis.ai/leaderboards/models) v4-era components fitted to v5 (RMSE 4.9): 8.0; partial v5 components (AutomationBench 5%, GDP.pdf 6%, Briefcase 369) agree. Card: [google/gemma-4-31B-it](https://huggingface.co/google/gemma-4-31B-it). | UD-IQ3_XXS, 11.84 GB (11.02 GiB); 64K with q4_0 KV totals 13.76 GiB. UD-IQ2_XXS (8.53 GB) reaches 128K at q4_0 (12.31 GiB). | Apache-2.0 |

All 13 quant sets validate against llama.cpp b11960 with
`python3 scripts/verify_catalog.py --catalog agents/T02/tiers/tier-16.json`
(13/13 ok, no warnings; architectures `qwen35` for the three Qwen picks,
`qwen35moe`, `llama` for Nanbeige4.3-33B and `gemma4`, all known to b11960).
Every file's byte size was checked
against the Hugging Face tree API and matched the catalog exactly, and every
header was read from the GGUF itself.

## Why these five

**Qwen3.8 27B (rank 1).** 20.1 on AA v5 at Xhigh is the highest score that
keeps a 64K context at a near-lossless quant on 16 GB. The hybrid layout is
the reason: only 16 of 64 layers carry a KV cache (4 heads x 256), 64 KiB per
token at f16, so UD-IQ4_XS (13.27 GiB) plus 64K q4_0 KV plus overhead is
15.09 GiB, inside the Linux budget. On Windows (14.72 GiB) use UD-Q3_K_XL at
64K q8_0 (15.05 GiB) or UD-IQ3_S at 64K q8_0 (14.01 GiB). The catalog also
lists MTP variants; they add ~0.3 GB for speculative decoding and are left
out because a 16 GB budget needs the headroom more than the 1.4-2x speedup.

**Nanbeige4.3 33B (rank 2).** The top dense score on v5.0 (20.4), and it
wins the agentic rows that matter for a local coding agent: AutomationBench
53% vs 48% and Terminal-Bench 4.0 30% vs 6% for Qwen3.8-27B, though it loses
HLE-Diamond 7% vs 15% and Omniscience by a wide margin. Full attention on 52
layers costs 208 KiB/token at f16, so 16 GB buys IQ2_S at 64K q4_0 (14.71
GiB) and no more; IQ3_XXS only fits 16K. Its tool calling is marked
UNVERIFIED in the catalog, which is the one reason it is not rank 1.

**Ternary Bonsai 2 27B (rank 3).** A native ternary quantization of
Qwen3.8-27B that is not a lossy 2-bit hack: 98.2% of the FP16 suite, within
0.4 pts of the 17.6 GB UD-Q4_K_XL, in 7.10 GiB. That leaves 7-8 GiB for
context, so it is the only pick here that reaches 128K (q8_0) or 262K (q4_0)
with room to spare. It keeps Qwen3.8's hybrid KV layout (64 KiB/token at
f16) and its vision tower. Use it for long-document work, or when the 16 GB
is shared with the desktop. The 18.0 score is an estimate, so treat the
ranking as approximate.

**Qwen3.6 35B A3B (rank 4).** The catalog is blunt about it: 9.5, below
Qwen3.8-27B on LCBv6 (74.6 vs 90.3) and SWE-Pro. It earns a slot because
it is the only MoE that fits 16 GB at a 64K context with a good quant, and
3B active parameters make it 2-3x faster than the dense picks. The cheap KV
(20 KiB/token at f16) also means UD-IQ3_S holds 64K at f16 KV (14.68 GiB)
and 128K at q8_0 (15.01 GiB on Linux).

**Gemma 4 31B (rank 5).** 8.0 is the weakest score of the five, but it is
the best tool-calling and document-QA model here after Qwen3.8, and its 50
sliding-window layers keep the cache affordable: 80 KiB/token f16 but only
1024+2048 tokens of window cache per SWA layer. UD-IQ3_XXS at 64K q4_0 is
13.76 GiB. Weaker coding than the Qwen picks; choose it for vision, PDF and
tool-use workloads.

## What I rejected and why

- **gpt-oss-20b (MXFP4, 12.06 GB).** Fits comfortably (13.28 GiB at 64K f16)
  and is the best-known tool-caller in this size class, but its catalog score
  is 5.0 against 9.5 for Qwen3.6-35B-A3B, so it is the second MoE, not a
  pick. It is the fallback if Qwen3.6's tool calling disappoints.
- **Qwen3.6 27B (score 12.0).** Same family and layout as Qwen3.8-27B but
  clearly behind it (12.0 vs 20.1); UD-Q3_K_XL only reaches 32K. Redundant.
- **Qwen3.8 27B UD-Q4_K_XL (17.56 GB, 16.35 GiB).** Over the 15.36 GiB
  Linux budget before any KV cache. The largest Qwen3.8 quant that fits is
  UD-IQ4_XS.
- **Nanbeige4.3 33B IQ3_M / IQ4_XS (17.80 / 18.64 GB).** Both exceed the
  budget with zero context; IQ3_XXS (14.49 GB) fits only at 16K, so IQ2_S is
  the practical ceiling.
- **Muse Glimmer 30B (6.3), Devstral Small 2 24B (4.0), Granite 4.2 30B
  (5.2).** All fit, none beats the five above on score, and Devstral's
  160 KiB/token cache costs 5 GiB at 32K. Devstral remains the specialist
  choice for a pure coding agent if the parent wants a sixth entry.
- **Nemotron 3.5 Bolt 74B A9B (11.5).** The smallest file is 20.3 GB, over
  budget; it is a 24-32 GB pick.
- **Newer-than-catalog candidates.** None added. The newest-GGUF feed on
  Hugging Face for 2026-11-06 was dominated by re-quants and merges of
  models already in the catalog, and adding a model here needs independent
  benchmarks plus header verification, which is the part that ran out of time.
  The catalog's 32 entries already include the November flagships.

## Open questions

- Nanbeige4.3-33B's tool calling is UNVERIFIED; if it tests badly, move
  Qwen3.6-35B-A3B up to rank 2 and add gpt-oss-20b as the MoE pick.
- Ternary Bonsai 2 27B's 18.0 is a hand-mapped estimate. If AA publishes an
  index for it, re-rank 1-3.
- The tier is described as "16 GB of usable GPU or unified memory". On a
  16 GB unified machine the budget is 14.4 GiB, which drops UD-IQ4_XS (15.09
  GiB at 64K) in favour of UD-Q3_K_XL (15.05 GiB, still over) or UD-IQ3_S
  (14.01 GiB at 64K q8_0). The parent may want a `unified` variant of this
  tier, as the 4 GB tier did for Windows.
