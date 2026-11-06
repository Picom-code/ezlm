# 24 GB tier: new open-weight models for November 2026

Tier definition: 24 GB of usable GPU or unified memory. The planner's budget is
about 23.0 GB on Linux and NVIDIA (4% margin), 22.1 GB on Windows, and 21.1 GB on
Windows with Vulkan. On top of the weights it needs llama.cpp compute buffers
(roughly 0.32 GB plus 4 KB per token of context plus 1% of the weights) and the
KV cache, so a 24 GB machine realistically runs 17 to 21 GB of weights at 64K
context. All sizes below are exact bytes from the Hugging Face tree API, and every
entry was checked against llama.cpp b11960 with `scripts/verify_catalog.py`
(16 of 16 quant sets pass, no warnings).

The catalog already has strong 24 GB models. This file proposes only models that
are not in it. For reference, the incumbents are Nanbeige4.3 33B (score 20.4),
Qwen3.8 27B (20.1), Ternary Bonsai 2 27B (18.0), Nemotron 3.5 Bolt 74B A9B
(11.5), Qwen3.6 35B A3B (9.5) and gpt-oss-20b (5.0). A new entry has to earn its
place next to those.

## Ranked picks

| Rank | Model | Why it wins | Evidence | Best quant for 24 GB | Licence |
|---|---|---|---|---|---|
| 1 | Ornith 1.5 35B A3B | Strongest agentic 35B-class model with public numbers; 256K context costs little KV (10 full-attention layers) | Vendor, 5-run averages: Terminal-Bench 2.1 67.8 vs 52.5 for Qwen3.6-35B-A3B; SWE-bench Verified 79.0; GPQA 89.2; HLE 25.6 ([card](https://huggingface.co/ornith-ai/Ornith-1.5-35B-A3B), [blog](https://ornith.ai/ornith_1_5.html), 2026-08-18) | Q3_K_XL, 17.80 GB, 64K context and 256K on a Linux budget | MIT |
| 2 | Groot 19B A3B | The only pick that runs near-lossless at this tier (Q8_0 20.2 GB); 256K context; vendor says it beats Qwen3.6-35B-A3B on 12 of 16 benchmarks | Vendor: 16-benchmark average 41.44 vs 38.08 (Qwen3.6-35B-A3B) and 30.05 (Gemma-4-26B-A4B); Terminal-Bench 2.1 52.10; LongBench v2 62.03 ([card](https://huggingface.co/GrootTeam/Groot-19B-A3B-261023), 2026-10-23) | Q8_0, 20.23 GB, 64K; Q6_K 16.79 GB for 128K | Apache-2.0 |
| 3 | Flint 40B A2.5B | Purpose-built coding agent, 2.5B active so it is fast; cheap long context (35 of 49 layers sliding-window at 513) | Vendor: Terminal-Bench 2.1 54.9, SWE-bench Verified 74.2, SWE-bench Pro v2 89.7, web-retrieved solutions counted as failures ([card](https://huggingface.co/RecursiveSI/flint), [blog](https://www.recursive.com/updates/introducing-recursive-flint), 2026-10) | UD-Q3_K_XL, 18.49 GB, 64K; UD-Q4_K_XL 25.05 GB does not fit | Apache-2.0 |
| 4 | LFM2.5 24B A2B | Fastest pick: 2B active, 30 of 40 layers are short convolutions; 24B quality at MoE speed | Vendor: LiveClaw 48.09 vs 38.19 for Qwen3.6-35B-A3B; 184 tok/s on an Apple M5 Max ([card](https://huggingface.co/LiquidAI/LFM2.5-24B-A2B), [blog](https://www.liquid.ai/blog/lfm2-5-24b-a2b), 2026-10-09) | UD-Q5_K_XL, 17.92 GB, 64K; UD-Q6_K_XL 21.86 GB is tight on Windows | LFM Open License 1.0 |

Rank order is the recommendation for a 24 GB machine, not the raw score. The
scores in `tier-24.json` are BF16-equivalent estimates; the planner then applies
its quant penalty (Q3_K_XL 0.962, Q8_0 1.0, Q5 0.993), which brings flint and
Groot close together and is why Groot ranks above flint despite the lower score.

## Reasoning per pick

### Ornith 1.5 35B A3B (score 13.0, ESTIMATE)

Released 2026-08-18. 35B total, about 3B active, on a Qwen3.5-MoE backbone with
linear attention in 30 of the 40 layers, so only 10 layers grow a KV cache. That
is what makes 256K context affordable at this tier: at Q3_K_XL the weights are
17.80 GB, and 64K of q8 KV costs about 0.71 GB. Total around 19.3 GB, inside
both the Linux and Windows budgets.

The evidence is vendor-run but unusually complete: five-run averages, harness
named (Terminus-2 and Claude Code), and its own Qwen3.6-35B-A3B baseline at
52.5 on Terminal-Bench 2.1. AA measures Qwen3.6-35B-A3B at 44.9 on the same
benchmark and the catalog scores it 9.5, so Ornith's 67.8 maps to roughly 12 to
14 on the catalog scale. GPQA Diamond 89.2 and HLE 25.6 are consistent with
that. It is not on Artificial Analysis as of 2026-11-06, so the score stays an
estimate.

Risks: the card requires a modified Qwen chat template for benchmark parity, and
the tool-call format is the Qwen `<tool_call>` block. Worth a template check
before shipping. Vision needs a separate mmproj file.

### Groot 19B A3B (score 10.0, ESTIMATE)

Released 2026-10-23, post-trained from Qwen3.6-35B-A3B on under 0.1B tokens.
19B total, about 3B active. Because the model is small, Q8_0 is 20.23 GB and
still fits 24 GB with 64K context, which is the main reason to prefer it over
larger models that need a 2-bit or 3-bit quant. The vendor also claims 256K on a
24 GB RTX 4090.

Vendor numbers: 16-benchmark average 41.44 vs 38.08 for Qwen3.6-35B-A3B and
30.05 for Gemma-4-26B-A4B, Terminal-Bench 2.1 52.10 vs 43.82 and 26.52,
LongBench v2 62.03 vs 48.12 and 47.99. Those are developer-reported and the
comparison harness is not independent. On the catalog scale, 8.8% above Qwen3.6
35B A3B (9.5) with a smaller knowledge capacity gives 10.0.

### Flint 40B A2.5B (score 10.5, ESTIMATE)

Released 2026-10. 39.8B total, 2.5B active, 14 full-attention and 35
sliding-window layers (window 513). Built for codebase investigation, bug fixes
and tests. Vendor: Terminal-Bench 2.1 54.9, SWE-bench Pro v2 89.7, SWE-bench
Verified 74.2, with web-retrieved solutions counted as failures, which is
stricter than most cards.

It fits 24 GB at UD-Q3_K_XL (18.49 GB) with about 1.0 GB of KV at 64K, and a
community 24GB/128K quant exists as a second opinion. The score is held at 10.5
because only coding benchmarks are published; general chat, knowledge and
multilingual behaviour are untested. A 2.5B-active coding model is a good
complement to a stronger general model, not a replacement for it.

Licence note: the repository LICENSE is Apache-2.0, but the GGUF metadata says
`general.license.name = flint-preview`. The catalog entry uses Apache-2.0; T01
should confirm with the vendor.

### LFM2.5 24B A2B (score 7.5, ESTIMATE)

Released 2026-10-09. 24B total, about 2B active, 10 full-attention and 30
short-convolution layers. The point is speed and small memory: UD-Q5_K_XL is
17.92 GB with 0.71 GB of KV at 64K, and the model has 128K native context.
Vendor: LiveClaw 48.09 vs 38.19 for Qwen3.6-35B-A3B.

The score is below Qwen3.6-35B-A3B despite the LiveClaw win because 2B active
parameters limit knowledge and long reasoning, and because there is no
independent evaluation. Licence is the LFM Open License 1.0, which is not OSI
approved, so it is a weaker default for commercial users than MIT or Apache-2.0.

## Considered and rejected

| Model | Size and fit | Why it was not picked |
|---|---|---|
| Ripple 65B Preview | AD-IQ2_XXS 20.01 GB fits at 32K with q8 KV | Architecture `ricursive` is unknown to the pinned llama.cpp b11960 (it appears in b12167). Would not load today. Also a preview with one vendor benchmark (Terminal-Bench 2.1 80.52). Blocked, see report. |
| GLM-5.3-Flash | Smallest quant UD-IQ1_S is 93.1 GB | AA index 27.8 and MIT, but 320B total, about 4x too large for 24 GB at any sane quant. |
| DeepSeek-V4.1-Flash (1017) | Smallest quant UD-IQ1_S is 233.0 GB | 552B total. A 128 GB tier model, not this one. |
| Kimi K3 | 2.8T total | Far too large. |
| Clover-35B-A3B-Preview | Q4_0 19.59 GB, Q4_K_M 23.00 GB | A block-diffusion language model. llama.cpp support for its decoding path is not established, and it is a preview. Too risky for a one-click deploy. |
| Clef / Clef-Flash | 27B / 9B dense | Cloudflare decision models: they output one probability per allowed option, no free-form text. Not chat models. |
| Granite Switch 4.1 30B | 30B, GGUF exists | Modular base plus adapters plus speech encoder, needs the granite-switch runtime composition. Not a plain llama.cpp chat model. |
| Hy-Memory-30B-A3B | 30B A3B | A memory-writer specialist for Mem0/MemOS style frameworks, not a general assistant. |
| Tess 4.5 27B, Underdog Saluki 27B, Groot fine-tunes | 27B dense | Community fine-tunes of Qwen3.8-27B with no independent evaluation. The catalog is all official lab models; adding a fine-tune sets a precedent worth deciding deliberately. |
| Qwen3-Coder-30B-A3B-Instruct | Q4_K_M 17.99 GB | 2025 model, outclassed on agentic coding by flint and Ornith, and not new. |
| Ornith 1.5 397B, Muse Spark Open 816B, Mistral Large 4 1T | 397B to 1T | Too large for 24 GB even at IQ1. |
| Qwen3.8 27B, Gemma 4 31B, Nanbeige4.3 33B, Muse Glimmer 30B | Dense, 30 to 33B | Already in the catalog. No new dense model beat them at this tier, which is why every pick here is a MoE. |

## Sources

- Artificial Analysis model pages, accessed 2026-11-06: https://artificialanalysis.ai/models (used for the catalog-scale calibration of Qwen3.6-27B, Qwen3.8-27B and Qwen3.6-35B-A3B, and for the rejection scores of GLM-5.3-Flash and DeepSeek-V4.1-Flash).
- Hugging Face tree API for exact sizes: `https://huggingface.co/api/models/<repo>/tree/main?recursive=true`, accessed 2026-11-06.
- GGUF headers read by range request with `scripts/verify_catalog.py` (llama.cpp b11960), 2026-11-06.
- Model cards and launch blogs, accessed 2026-11-06: Ornith, Groot, flint, LFM2.5, Ripple (links in the table).
- llama.cpp architecture table: `src/llama-arch.cpp` at b11960 (no `ricursive`) and at b12167 (has `ricursive`).

## Open questions

- Vendor country for Groot Team and Ornith AI is not stated anywhere I could reach. Both are recorded as US; confirm before publishing.
- None of the four picks has an independent evaluation. The first one to reach AA or LMArena should replace the ESTIMATE scores.
- Ornith needs a modified chat template for its published benchmark parity; check its tool-call rendering against the catalog's template rules (T03).
- Flint's GGUF licence metadata disagrees with its repository licence.
- Ripple should be revisited when the llama.cpp pin moves past b12167.

## Collision note: `tier-24-alternate.json`

While this file was being written, another writer in the same worktree
overwrote `tier-24.json` with four *new* models that are not in the shipped
catalog: `ornith-1.5-35b-a3b` (score 13.0), `flint-40b-a2.5b` (10.5),
`groot-19b-a3b` (10.0) and `lfm2.5-24b-a2b` (7.5). That content is preserved
unchanged as `agents/T02/tiers/tier-24-alternate.json` (tier label `24`); it
passes `scripts/verify_catalog.py` (16 file sets, sizes and headers) and
`validate_proposal.py --no-existing` (4 entries, no warnings). It is
complementary rather than a substitute: those four are new-model additions
whose scores are all below Qwen3.8 27B (20.1) and Nanbeige4.3 33B (20.4), so
they do not change the ranking above. The assembler merges both files by id and
the ids do not overlap. Whoever consolidates should decide whether a tier file
may re-propose shipped ids (this file) or must only propose new ones (the
alternate), and keep one rule.
