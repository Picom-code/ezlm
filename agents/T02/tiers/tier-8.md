# 8 GB tier: the best open-weight models that fit 8 GB of usable GPU or unified memory

Scouted 2026-11-06. Six proposals. Every byte size below was read from the Hugging Face
tree API, and every layer/KV/context number from the GGUF header with range requests
(`scripts/verify_catalog.py` reports 0 errors on all 21 quant sets).

## How "fits" is measured

EZLM's planner (`internal/fit/fit.go`) budgets 8 GiB of usable memory as:

- discrete GPU (Linux): `8 - max(4%, 0.4) = 7.68 GiB`
- unified memory or Windows: `8 - max(10%, 1.0) = 7.0 GiB`

and then requires `weights + KV cache + overhead(weights, ctx) <= budget`, where
`overhead = 320 MiB + 4 KiB * ctx + 1% of weights`. The KV cache is
`2 * kv_layers * kv_heads * head_dim * bytes * ctx` plus the sliding-window cache for
Gemma-architecture models. A "best quant" here means the largest quant that leaves a
usable context (8K minimum, 64K where the cache allows) inside 7.0 GiB, so the entry
works on an 8 GB Mac as well as an 8 GB card. Sizes are GiB (2^30); exact bytes are in
the JSON.

## Ranked table

| # | Model | Why it wins | Benchmark evidence | Best quant for 8 GB | Licence |
|---|---|---|---|---|---|
| 1 | **Ornith 1.5 9B** (new) | Best model at this tier by a wide margin. 9.65B dense, MIT, 262K context, vision mmproj. Beats Gemma-4-31B (a 31B model, v5 8.0) on coding, GPQA and agentic suites at a quarter of the weights. | [model card, 2026-08-19](https://huggingface.co/ornith-ai/Ornith-1.5-9B): Terminal-Bench 2.1 46.2, SWE-bench Verified 70.6, SWE-bench Pro 47.5, GPQA-Diamond 86.4, HLE 20.2, ClawEval 66.5 (mean of 5 runs; vendor-run) | Q5_K_M **6.19 GiB** at 8-16K; Q4_K_M 5.38 GiB for a 7.0 GiB budget | MIT |
| 2 | **Granite 4.2 8B** (existing) | Only model here with an official AA v5 score (5.1). Apache-2.0, US origin, 128K, reasoning-augmented tool calls. Expensive KV cache is its weakness. | [AA Intelligence Index v5.0, fetched 2026-11-02](https://artificialanalysis.ai/) (catalog value) | Q5_0 **5.68 GiB** with q8_0 KV at 8K; Q4_K_M 4.98 GiB | Apache-2.0 |
| 3 | **Qwen3.5 9B** (existing) | 32 KiB/token KV cache, the cheapest of the 9B class, so it keeps 32K context at Q5. Native vision. Now outclassed by Ornith 1.5 on the same architecture. | [Ornith 1.5 card comparison, 2026-08-19](https://huggingface.co/ornith-ai/Ornith-1.5-9B): Qwen3.5-9B TB2.1 21.3, SWE-V 53.2. Catalog v5 5.0 (estimate) | UD-Q5_K_XL **6.28 GiB** at 8-16K | Apache-2.0 |
| 4 | **Gemma 4 12B** (existing) | The any-to-any pick: text, image and audio input. 262K context. Its 40 sliding-window layers cost a fixed ~0.94 GiB cache, which is why only Q3-class quants fit. | Catalog v5 3.5 (estimate from AA legacy components); [Gemma 4 release, 2026-05-23](https://huggingface.co/google/gemma-4-12B-it) | UD-Q3_K_XL **5.61 GiB** at 8K; UD-IQ3_XXS 4.32 GiB is the safe one | Apache-2.0 |
| 5 | **Nemotron 3.5 Bolt E4B** (new) | The long-context pick. 18 trailing layers share a KV cache, so 64K costs only ~1.1 GiB; runs the full 64K at Q5. Built for low-TTFT voice/device agents; 8.0B total, 4B effective. | [NVIDIA card, 2026-10-05](https://huggingface.co/nvidia/NVIDIA-Nemotron-3.5-Bolt-E4B-4GB): MultiChallenge 53.1 vs 42.1 (Gemma-4-E4B), BFCL-v3 68.5, ToolSandbox 68.8, Tau2 55.1, IFBench 63.8 | UD-Q5_K_XL **5.43 GiB** at 64K | OpenMDW-1.1 |
| 6 | **LFM2.5 8B A1B** (new) | The speed/agent pick: 1B active, 12 KiB/token KV (2.7x cheaper than the 9Bs), full 128K at Q5, 18.5K output tok/s on an H100. Weak knowledge. | [Liquid card, 2026-05-28](https://huggingface.co/LiquidAI/LFM2.5-8B-A1B): IFEval 91.84, IFBench 56.47, MATH500 88.76, AIME25 42.53, BFCLv3 64.79, AA-Omniscience -24.70 | UD-Q5_K_XL **5.95 GiB** at 128K | LFM Open License v1.0 |

Scores are the catalog's v5-equivalent scale. Only Granite 4.2 8B has an official AA v5
number; the other five carry a `score_source` that states the basis and says ESTIMATE
where the number is anchored to catalog models rather than measured.

## Reasoning per pick

**Ornith 1.5 9B (rank 1).** Released 2026-08-19, MIT, dense 9.65B on the Qwen3.5-9B
architecture (`qwen35` GGUF: 32 layers, 4 KV heads, head_dim 256, full attention every
4th layer, 262144 context, one NextN head inside the file). The card's numbers are the
strongest I found at this size: Terminal-Bench 2.1 46.2 against 21.3 for Qwen3.5-9B and
42.1 for Gemma-4-31B, SWE-bench Verified 70.6 against 53.2 and 52.0. It does not beat
Qwen3.6-35B-A3B (v5 9.5) on agentic suites, so I anchored it at 9.0 rather than higher.
The risk is that all of these numbers are vendor-run and averaged over five runs, with no
AA v5 index yet; the 2026-08-19 date means three months of independent evaluation is
missing. Q5_K_M is the right quant: 6.19 GiB leaves ~0.8 GiB for a 16K cache and
buffers on a 7.68 GiB budget, and 6.83 GiB total still fits. On a 7.0 GiB unified
machine use Q4_K_M (5.38 GiB, ~6.0 GiB total).

**Granite 4.2 8B (rank 2).** The catalog's only official AA v5 number in this tier
(5.1), Apache-2.0, US origin, 128K native, and the 4.2 generation replaced the hybrid
Mamba design with a plain dense GQA transformer. The catch is the KV cache: 40 layers x
8 KV heads x 128 = 160 KiB/token at f16, so 8K of context alone is 1.2 GiB. With q8_0
KV that drops to 0.61 GiB, and Q5_0 (5.68 GiB) plus overhead fits at ~6.7 GiB. Long
context is not its strength on this tier; correctness and licence are.

**Qwen3.5 9B (rank 3).** Same architecture as Ornith 1.5, so the header numbers are
identical, but the 2026-02 model is now behind on every coding and agentic number Ornith
published. It stays because the 32 KiB/token cache buys 32K context at Q5 where Granite
gets 8K, and because vision and llama.cpp support are mature. I trimmed the catalog
quant list to what fits: Q6_K (6.95 GiB) plus 0.25 GiB of 8K KV plus 0.39 GiB of
buffers is 7.59 GiB, which only fits the 7.68 GiB discrete budget and not 7.0 GiB.

**Gemma 4 12B (rank 4).** The only any-to-any model in the tier: image and audio input,
262K context. Its memory shape is unusual: 8 global layers with 1 KV head x 512 and 40
sliding-window layers with 8 KV heads x 256 and a 1024-token window. The window cache is
`2*40*8*256*2 * 3072 = 0.94 GiB` no matter how short the context, so IQ4_XS (5.94 GiB)
totals ~7.4 GiB and misses the 7.0 GiB budget. Q3_K_XL (5.61 GiB) totals ~7.05 GiB and
fits 7.68; IQ3_XXS (4.32 GiB) is the safe choice. It is the weakest general model of the
six, so it is here for modality coverage, not score.

**Nemotron 3.5 Bolt E4B (rank 5).** Released 2026-10-05, an NVIDIA post-train of
Gemma-4-E4B for voice and device assistants. 42 blocks with `shared_kv_layers=18`, so
only 4 global + 20 sliding-window caches are allocated: 16 KiB/token global plus a fixed
0.1 GiB window cache. 64K therefore costs ~1.1 GiB, and UD-Q5_K_XL (5.43 GiB) totals
~6.9 GiB: the full 64K at Q5 on an 8 GB machine. It beats Gemma-4-E4B and mostly beats
Qwen3.5-4B on instruction following and tool benchmarks, but loses Tau2 to Qwen3.5-4B and
is not competitive with the 9B class on hard reasoning or code. Its value is long
context, speed and a 0.37 GiB DFlash drafter, not raw score.

**LFM2.5 8B A1B (rank 6).** 8.47B total, 1B active, 24 blocks of which only 6 keep a KV
cache (8 heads x 64), so 12 KiB/token, the cheapest cache in the tier and 128K at Q5.
IFEval 91.84 and BFCLv3 64.79 are strong for 1B active; AA-Omniscience -24.70 and 8.67%
accuracy show the knowledge gap. This is the pick for a fast tool-calling agent, not for
questions about the world. The LFM Open License has commercial conditions; check them
before shipping.

## Rejected, and why

- **Ternary Bonsai 2 27B** (catalog 18.0, Q2_0 7.63 GB = 7.10 GiB). The highest-scoring
  model that almost fits. 7.10 GiB weights + 0.50 GiB KV at 8K (16 KV layers x 4 heads
  x 256) + 0.40 GiB overhead = 8.0 GiB, over both budgets. No smaller quant exists, and
  the 8K ladder rung is the minimum context, so it cannot be made to fit.
- **LFM2.5 24B A2B** (UD-IQ2_XXS 7.47 GB = 6.96 GiB). Tempting: 24B MoE at 1B active,
  card claims it beats Qwen3.6-35B-A3B on agentic tasks (LiveClaw 48.09 vs 38.19). But
  6.96 GiB leaves only ~0.7 GiB for a 40 KiB/token KV cache and buffers; it fits 7.68 GiB
  at 8K only, and misses 7.0 GiB entirely. Recorded here as the first stretch pick if the
  tier is ever redefined as 8 GB of discrete VRAM only.
- **Gemma 4 26B A4B** (UD-IQ2_XXS 9.92 GB = 9.24 GiB), **Qwen3.6 35B A3B** (UD-IQ2_XXS
  10.76 GB = 10.0 GiB), **Muse Glimmer 30B** (UD-IQ2_XXS 10.75 GB), **Devstral Small 2
  24B** (UD-IQ3_XXS 9.41 GB): weights alone exceed the budget.
- **Qwen3.6 27B** (UD-IQ2_XXS 9.57 GB = 8.91 GiB), **Nanbeige 4.3 33B** (IQ2_XXS 10.07
  GB), **Gemma 4 31B** (UD-IQ2_XXS 8.53 GB = 7.94 GiB plus a 1-2 GiB sliding-window
  cache): each needs a quant below Q2 or a cache that does not fit.
- **Ornith 1.5 35B A3B**: its smallest GGUF in the official repo is Q4_K_M at 21.7 GB.
  Unsloth may publish smaller quants later; none existed on 2026-11-06.
- **North Mini Code 1.0**: 9.4 GB at IQ1_M, so the coding model misses the tier by 1.7 GB.
- **Qwen3.8 9B**: no base model exists. Qwen3.8 ships at 27B, 180B (Flash Next) and
  2.4T; the "9B" repos are community distills, not a first-party release.

## Caveats

- Ornith 1.5 and Nemotron Bolt scores are estimates anchored to catalog models, not
  independent v5 measurements. If AA publishes v5 numbers before the beta, replace them.
- The 8 GB tier has two different budgets (7.68 GiB discrete, 7.0 GiB unified/Windows).
  The table names the quant that works for both; the JSON also lists the larger quants so
  the planner can use them on a discrete card.
- Vision needs the separate mmproj file (0.56 GiB for Bolt, 0.92 GiB for Ornith, 0.18
  GiB for Gemma 4 12B). The planner does not count it; on a 7.0 GiB budget, drop vision
  first.
