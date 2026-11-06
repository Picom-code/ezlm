# 96 GB tier: usable GPU or unified memory

Scope: one 96 GB card (RTX PRO 6000 Blackwell, RTX 6000 Ada in 2x48 or 1x96 form, L40S 48GB pairs) or a 128 GB unified-memory Mac with a 96 GB wired limit. After `internal/fit` margins the weight budget is roughly 90-92 GB, and the rest has to hold KV cache, compute buffers and any MTP draft.

Rank is by expected effective score (AA v5 score x quant quality), not by raw score alone. Every file below was checked against llama.cpp b11960 by `scripts/verify_catalog.py`, which reads the real GGUF headers over range requests. All nine file sets pass with zero errors.

## Ranked picks

| # | Model | Why it wins | Benchmark evidence | Best quant for 96 GB | Licence |
|---|---|---|---|---|---|
| 1 | **Qwen3.8 Flash Next** (MoE 180B, 6B active) | Highest score that fits with usable context. Only 12 of 48 layers keep KV, so 256K context is about 6.3 GB at F16. | AA v5 component fit 27.0 ([model page](https://artificialanalysis.ai/models/qwen3-8-flash-next), fetched 2026-11-06; AA has components but no official index). | GSQ-RCO-IQ3_S, 83.62 GB (IQ3_XXS, 75.84 GB, for more headroom) | qwen-community-1.0 (UNVERIFIED) |
| 2 | **Nemotron 3.5 Super VL 120B A12B** (MoE 124B, 12B active) | Best US-origin model in the tier, vision, 1M context, tiny KV (8 attention layers). | AA v5.0 13.6 ([model page](https://artificialanalysis.ai/models/nemotron-3-5-super-vl), fetched 2026-11-06). | UD-Q4_K_XL, 86.91 GB (UD-Q3_K_XL, 65.76 GB) | OpenMDW-1.1 |
| 3 | **Step 3.7 Flash** (MoE 201B, 11B active) | Biggest model that fits at 3-bit, vision, 262K context; 33 of 45 layers use a 512-token window so KV is cheap. | AA legacy-component fit 11.4 ([model page](https://artificialanalysis.ai/models/step-3-7-flash), fetched 2026-11-06); no official v5 index. | UD-IQ3_S, 80.03 GB (UD-Q3_K_M, 89.26 GB) | Apache-2.0 |
| 4 | **Qwen3 Coder Next** (MoE 80B, 3B active) | Coding specialist that runs at Q8_0 with 11 GB to spare; no quant loss on a 96 GB box. | AA v5 component fit 5.3 general, legacy fit 6.9; coding-agent use is its point ([model page](https://artificialanalysis.ai/models/qwen3-coder-next), fetched 2026-11-06). | Q8_0, 84.81 GB | Apache-2.0 |
| 5 | **Mistral Medium 3.5 128B** (dense 127.7B) | The only dense option: steadier decode on unified memory, vision, 256K native, EU origin. | AA v5.0 4.65 ([model page](https://artificialanalysis.ai/models/mistral-medium-3-5-128b), fetched 2026-11-06). | UD-Q4_K_XL, 75.69 GB at ~32K context (Q5_K_M, 88.32 GB, at ~8K) | Modified MIT (revenue cap) |

## Why each pick

### 1. Qwen3.8 Flash Next

The tier's top pick by a wide margin: 27.0 estimated v5, against 13.6 for the next model. It is a 180B MoE with 6B active, a hybrid Gated DeltaNet stack with Qwen Sparse Attention, and a 51B n-gram embedding table that llama.cpp can keep mmapped, so the effective VRAM cost is lower than the file size suggests. The 12 KV layers are the important number here: 2 KV heads x 256 head dim x 12 layers is about 24 KB/token at F16, so the full 262K context costs 6.3 GB and still fits beside the 83.6 GB IQ3_S file. IQ3_XXS at 75.8 GB is the safer choice if the machine also has to hold a draft model or a large batch.

Caveat: AA has not published a v5 index for this model (no public API), so 27.0 is a linear-fit estimate from AA's own component scores, RMSE 1.7 points. The licence string is also unverified.

### 2. Nemotron 3.5 Super VL 120B A12B

The US-origin pick and the best value per byte for long context: 40 Mamba + 40 MoE + 8 attention blocks means the KV cache is negligible even at 1M tokens. Q4_K_XL is 86.9 GB, leaving about 5 GB for context and compute, which is enough here because only 8 layers cache. Q3_K_XL at 65.8 GB is the comfortable option and still scores the same. Vision input and an MTP draft from ggml-org are both published.

### 3. Step 3.7 Flash

201B total, 11B active, Apache-2.0, vision. It is the largest model in the tier and the only Apache-2.0 200B-class option. UD-IQ3_S at 80.0 GB fits with room for 256K context (12 full-attention layers, 8 KV heads, 128 dim, about 49 KB/token at F16); UD-Q3_K_M at 89.3 GB is a quality step up but leaves only about 6 GB, which is tight for 64K-plus context. Score 11.4 is an estimate with a wide error bar (+/- 5).

### 4. Qwen3 Coder Next

Coding-agent specialist, 3B active, 256K context. The reason to list it at 96 GB is Q8_0: 84.8 GB of essentially lossless weights with 11 GB left for context, which no other model in this tier can do. For general chat it is a step down from the three above; for repository-scale coding it is the strongest small-active model available.

### 5. Mistral Medium 3.5 128B

The dense pick. On unified memory, dense decode avoids the expert-routing stalls that MoE models show when the working set spills, and Mistral Medium is the only 128B dense model with vision and 256K context. The cost is KV: 88 full-attention layers at 8 KV heads x 128 dim is about 352 KB/token, so 64K context needs 23 GB. That is why the entry pairs UD-Q4_K_XL (75.7 GB, ~32K) with Q5_K_M (88.3 GB, ~8K). Licence has a USD 20M/month revenue cap.

## Drops and why

- **GLM-5.3-Flash** (AA 28, 320B): the highest-scoring model that nearly fits, but the smallest published quant is UD-IQ1_S at 93.09 GB, leaving under 3 GB for KV and compute on a 96 GB budget. It is a 128 GB pick, not a 96 GB one.
- **GLM-5.3** (753B, AA 32): smallest quant UD-IQ1_S is 216.7 GB.
- **Kimi K3 / K3.1 Pro** (2.8T): smallest quant UD-Q1_0 is 466 GB.
- **DeepSeek V4.1 Flash 1017** (552B): smallest quant UD-IQ1_S is 233 GB.
- **Qwen3.5 122B A10B** (7.5): dominated by Nemotron Super VL at the same size class (13.6, 1M context, US origin) and by Flash Next at a smaller footprint.
- **Mistral Small 4 119B** (4.3): same vendor and size as Mistral Medium but a lower score; the 1M context is its only edge and Nemotron already covers that with a much smaller KV.
- **Ornith 1.5 397B**: only Q4_K_M and up are published (244 GB minimum).
- **Qwen3.8 27B, Gemma 4 31B, Nanbeige 33B**: all fit easily at Q8 but are 20-30 point models; a 96 GB machine should spend the memory on a 120-200B MoE instead. They remain the 12-24 GB picks.
- **Ternary Bonsai 2 27B**: same argument, and its 1-bit format is wasted on a machine this large.

## Notes and open questions

- The Mistral Medium KV estimate assumes F16 KV and no cache quantisation; with q8_0 KV the 64K context drops to about 11.5 GB and UD-Q4_K_XL fits at 64K.
- Flash Next and Step 3.7 Flash scores are estimates, not official AA indices. If AA publishes v5 for either, re-rank; the ordering between them (27.0 vs 11.4) is unlikely to flip but the absolute numbers may move.
- All sizes are the sum of the real split files from the Hugging Face tree API, and all nine file sets were verified against llama.cpp b11960 by `scripts/verify_catalog.py` on 2026-11-06.
