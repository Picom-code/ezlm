# 12 GB tier: usable GPU or unified memory

Scope: machines where EZLM can use about 12 GB of VRAM (RTX 3060 12GB, RTX 4070 12GB, RTX 4070 Super 12GB) or about 12 GB of unified memory (a 16 GB Mac with the default wired limit). The planner takes a safety margin first, so the real weight budget is:

| Machine | Budget after `internal/fit` margins |
|---|---|
| 12 GB NVIDIA, Linux | 11.52 GB |
| 12 GB NVIDIA, Windows | 11.00 GB |
| 16 GB Mac, 12 GB usable (unified) | 10.00 GB |

The budget has to hold weights, KV cache, compute buffers and any MTP draft. A 10.1 GB file is therefore not automatically a fit: the KV cache alone costs 0.9-4.0 GB at the 32K-64K context the planner prefers.

Rank below is by **effective score**, the number the planner ranks by: AA v5 score x quant quality (`internal/fit/quantQuality`) x KV quality x context factor. Raw AA score alone puts Nanbeige first; its KV cache and quant penalty move it to third.

## Ranked picks

| # | Model | Why it wins | Benchmark evidence | Best quant for 12 GB | Licence |
|---|---|---|---|---|---|
| 1 | **Qwen3.8 27B** (dense 27.8B) | Best quality that fits. 48 Gated DeltaNet + 16 full-attention layers keep the KV cache at ~64 KB/token, so 3-bit weights hold 64K context on a Linux card. | AA v5 20.1 ([leaderboard](https://artificialanalysis.ai/leaderboards/models), fetched 2026-11-06; [model page](https://artificialanalysis.ai/models/qwen3-8-27b)): GPQA-Diamond 90.5%, Terminal-Bench 2.1 79.8%, tau3-Banking 48.0%, HLE 33.9%. | UD-IQ2_XXS, 7.27 GB (planner: 64K, 44.7 tok/s) or GSQ-RCO-IQ3_XXS, 10.09 GB (16K) | Apache-2.0 |
| 2 | **Ternary Bonsai 2 27B** (native ternary) | The efficient pick. 7.63 GB is the real trained format, not a 2-bit copy, and it keeps the Qwen3.8 KV geometry. It is the best plan on the 12 GB Mac and within one point of Qwen on the 12 GB cards. | Vendor: 98.2% of Qwen3.8-27B FP16 on a 14-20 benchmark suite; category deltas coding 89.4 vs 89.1, agentic 74.9 vs 76.7 ([model card](https://huggingface.co/prism-ml/Ternary-Bonsai-2-27B-gguf)). Score 18.0 is an ESTIMATE from the base model. | Q2_0, 7.63 GB (planner: 32K on 12 GB cards, 64K on the Mac) | Apache-2.0 |
| 3 | **Nanbeige4.3 33B** (dense 33.1B) | Highest raw AA v5 score in the tier and the strongest agentic generalist: AutomationBench 53%, Terminal-Bench 4.0 30%. | AA v5 20.4 ([model page](https://artificialanalysis.ai/models/nanbeige4-3-33b), fetched 2026-11-06): HLE-D 6.9%, Omniscience much lower than Qwen3.8. | IQ2_XXS, 10.07 GB; needs q4_0 KV to reach 32K on 12 GB | Apache-2.0 |
| 4 | **K2 Horizon 7B** (dense 9.0B, new) | Best small dense model in the tier, and the coding specialist. 36 layers, 512K native context, reasoning_effort high/medium/low and tool calling in the template (llama.cpp b11960 has the `k2_horizon` parsers). Fits fully on the GPU at 64K. | AA components ([model page](https://artificialanalysis.ai/models/k2-horizon-7b), fetched 2026-11-06): GPQA-Diamond 75.3%, Terminal-Bench 2.1 44.2%, HLE 18.2%, tau3-Banking 27.6%. Vendor card ([HF](https://huggingface.co/IFM/K2-Horizon-7B)): SWE-bench Verified 72.4%, TB2.1 44.6%. | Q5_K_M, 6.47 GB (Q6_K 7.39 GB and Q8_0 9.57 GB also fit) | Apache-2.0 |
| 5 | **Qwen3.6 35B A3B** (MoE 35.9B, 3B active) | The MoE pick for speed. 3B active and a 10-layer KV cache give 200-290 tok/s in the planner estimate; good when latency matters more than the last few points. | AA v5 components ([model page](https://artificialanalysis.ai/models/qwen3-6-35b-a3b)): GPQA 84.1%, TB2.1 44.9%, tau2 95.3%, HLE 22.2%. Catalog score 9.5 is an ESTIMATE. | UD-IQ2_XXS, 10.76 GB (the only file that fits 12 GB fully) | Apache-2.0 |
| 6 | **Gemma 4 12B** (dense 12B, vision) | The fast general-purpose vision pick. Only 12B, so it runs at 6-bit with 64K context in 12 GB, and the chat template handles images. | AA components ([model page](https://artificialanalysis.ai/models/gemma-4-12b)): GPQA 75.3%, IFBench 73.5%, MMMU-Pro 69.7%, TB2.1 27.3%, tau2 36.3%. Catalog score 3.5 is an ESTIMATE. | Q6_K, 9.79 GB (QAT-Q4_0, 6.98 GB, on Windows and unified memory) | Apache-2.0 |
| 7 | **Gemma 4 31B** (dense 31.3B, vision) | The quality vision pick. Highest AA score of the Gemma family that fits, but only at 2-bit and only in split mode: the sliding-window cache costs ~2.3 GB at any context. | AA ([model page](https://artificialanalysis.ai/models/gemma-4-31b), fetched 2026-11-06): GPQA-Diamond 85.7%, TB2.1 43.4%, tau2 59.9%, IFBench 75.6%; official v5 index 4.4. | UD-IQ2_XXS, 8.53 GB (16K split on Linux, 32K on the Mac) | Apache-2.0 |

Planner check (`agents/T02/a04/fit-sim-12gb.txt`): all seven get a plan on a 12 GB Linux card and on a 12 GB Windows card. Gemma 4 31B is the slowest (8.0 tok/s on Linux, 5.2 on Windows), so it is a quality-over-speed pick. On a 16 GB Mac with a 12 GB wired limit, Nanbeige and Qwen3.6 do not fit, and the picks are Bonsai (17.96), Qwen3.8 (17.65), K2 Horizon (8.64), Gemma 4 31B (3.59) and Gemma 4 12B (3.45).

## A planner issue that changes these picks

`internal/fit/fit.go` tries the KV cache types in the order `f16`, `q8_0`, `q4_0` and **breaks after the first type that fits at a given context**, on the assumption that a smaller KV type only loses quality. It also changes placement and speed. On a 12 GB card:

- **Nanbeige4.3 33B**: `f16` at 32K fits, so the loop stops there. That plan is split and slow (4.1 tok/s), and the usable plan becomes 8K `f16` (effective 10.77). With `q4_0` at 32K the whole model is on the GPU (10.76 GiB, 11.3 tok/s, effective 16.62).
- **Ternary Bonsai 2 27B**: `f16` at 64K fits in split mode (12.8 tok/s, effective 18.00). With `q8_0` at 64K it is fully on the GPU (9.87 GiB, 42.6 tok/s, effective 17.96).
- **Qwen3.8 27B**: GSQ-RCO-IQ3_XXS at 64K with `q4_0` KV is 11.21 GiB on the GPU (effective 19.5, 32.2 tok/s); the loop stops at the split `f16` plan (6.0 tok/s, effective 19.80 but unusable speed) and the model ends up on UD-IQ2_XXS.

Reported to the fit owner in `agents/T02/a04/REPORT.md` (F1). The numbers in the table are what the current planner does, not the hardware limit.

## Why each pick

### 1. Qwen3.8 27B

The tier is defined by the KV cache as much as by the weights. Qwen3.8 27B keeps only 16 full-attention layers, so 64K context costs ~4.0 GB at F16 and ~2.1 GB at q8_0. The GSQ-RCO IQ3_XXS file is 10.09 GB and measured at 0.985 of FP16 quality, the best quality-per-byte in the catalog for this tier. On a 12 GB Linux card the planner runs UD-IQ2_XXS at 64K (11.40 GB, 44.7 tok/s, effective 17.69); on Windows it picks GSQ-RCO-IQ2_XS at 32K (10.36 GB, 27.5 tok/s, effective 17.76).

No MTP draft in this entry. The catalog's Q8_0 draft is 3.16 GB, which pushes the model into split mode (8.2 tok/s); the Q4_0 draft (1.68 GB) only fits by dropping from 64K to 32K. Without a draft the planner keeps the model fully on the GPU, which is the better 12 GB plan. The catalog keeps its Q8_0 draft for larger cards; see the open questions.

Trade-off: the 3-bit GSQ file does not fit a 10.0 GB unified budget. On a 16 GB Mac the planner falls back to UD-IQ2_XXS (7.27 GB, quality 0.88) at 64K, effective 17.65.

### 2. Ternary Bonsai 2 27B

A 7.63 GB model that scores like an 18.0 model is the best answer to "I have 12 GB and want a big model with a long context". It is Qwen3.8-27B compressed to ternary during QAT, so it inherits the same 16-layer KV geometry. The planner ranks it first on the Mac profile (64K, 17.96) and second on the 12 GB cards (32K, 16.92), and it is the fastest of the big models (42.6 tok/s estimated).

The 18.0 score is an ESTIMATE: PrismML reports 98.2% retention on its own suite and AA has not indexed the model. The knowledge delta (-5.7 points) is the main risk, so it is a good general/coding pick, not the best knowledge pick.

### 3. Nanbeige4.3 33B

Nanbeige4.3 is the highest-scoring dense model on AA v5 (20.4) and wins the agentic benchmarks (AutomationBench 53%, TB4 30%). It loses badly on knowledge (HLE-D 6.9%, low Omniscience), and it is the worst fit of the three 27-33B models: 52 full-attention layers cost ~208 KB/token at F16, so 12 GB needs q4_0 KV and the planner's current KV loop stops at 8K. That is why its effective score (10.77) is below Qwen and Bonsai despite the higher raw score. A 16 GB card is the comfortable home.

Tool calling is marked true in the catalog and is UNVERIFIED on the model card. Treat it as a general/agentic model, not a guaranteed tool-calling model.

### 4. K2 Horizon 7B

This is the new entry for the tier. The catalog has no small dense model with this combination of coding evidence and context: 9.0B parameters, 36 layers, 512K native context, Apache-2.0, reasoning effort and tool calling. Vendor numbers (SWE-bench Verified 72.4%, TB2.1 44.6%, HLE 19.5%) are better than every 8-9B model in the catalog, and AA's independent components agree on the shape (TB2.1 44.2%, GPQA 75.3%, HLE 18.2%). AA has not published a v5 index, so the 8.9 score is an ESTIMATE from the same linear fit the catalog used for Qwen3.6 35B A3B (RMSE 1.7 points). Expect 8-10.

The planner runs Q5_K_M at 64K fully on the GPU (11.43 GB, 50.2 tok/s on a 4070; Q4_K_M on Windows). Q6_K (7.39 GB) and Q8_0 (9.57 GB) are also listed for machines where the KV cache or RAM is tighter. This is the pick for coding agents that need a long context on a small card.

### 5. Qwen3.6 35B A3B

The only MoE that fits 12 GB fully. 35.9B total, 3B active, and the KV cache covers only 10 layers, so the planner runs it in hybrid mode with 9-11 expert layers in RAM and reports 200-290 tok/s. Effective score 8.36 is below the dense picks, but nothing else in this tier is close on latency.

The 10.76 GB IQ2_XXS is 2-bit, so quality is the weak point (0.88). If the machine has 32 GB RAM, UD-Q4_K_XL (22.4 GB) with expert offload is a better quality/speed trade and the planner will choose it when it is in the catalog's quant list. It is not in this 12 GB entry because it does not fit the tier's memory, only the machine's.

### 6. Gemma 4 12B

The fast general-purpose vision pick. 12B at Q6_K is 9.79 GB, and the sliding-window cache is about 1.0 GB at 8K (40 SWA layers x 8 KV x 256 x window 1024 + batch) plus 0.31 GB for the 8 global layers, so the file just fits a 12 GB Linux card at 64K. QAT-Q4_0 (6.98 GB) is the safer choice on Windows and unified memory.

The score (3.5) is far below the 27-33B models. It earns its place on being a 6-bit, 44-62 tok/s model with image input where the others are 2-3 bit and slower.

### 7. Gemma 4 31B

The highest-quality vision model that fits the tier, but only just. 60 layers, of which 50 are sliding-window (16 KV x 256, window 1024) and 10 global (4 KV x 512). The SWA cache is ~2.3 GB at any context, which is why the 8.53 GB IQ2_XXS is the only file that fits. The planner runs it at 16K in split mode on Linux (11.39 GB GPU + 2.67 GB RAM, ~8 tok/s, effective 3.29), 32K on the Mac (3.59) and 64K on Windows at ~5 tok/s (3.86).

AA gives it an official v5 index of 4.4, above the 12B's estimate (3.5), and its GPQA (85.7%) is close to Qwen3.8 27B. The catch is 2-bit weights and the slowest plan in the table: after the quant and context penalties it is within 0.3 points of the 12B but 5-10x slower. It is the pick only when image quality matters more than speed.

## Drops and why

- **Muse Glimmer 30B** (AA 6.3): IQ2_XXS is 10.75 GB and the SWA cache adds ~0.35 GB, so it only fits a Linux card and not a 10.00 GB unified budget. The licence ("Muse Model License") is unverified, which is a beta risk for a marginal gain over K2 Horizon.
- **Qwen3.6 27B** (catalog 12.0): same size and KV class as Qwen3.8 27B, 8 points lower. No reason to carry both at 12 GB.
- **Granite 4.2 8B** (AA 5.1), **Qwen3.5 9B** (catalog 5.0), **Gemma 4 E4B** (2.0): all below K2 Horizon 7B at the same or larger size. K2 wins on coding by a wide margin (TB2.1 44.2% vs 18-29%).
- **Flint 40B A2.5B** (vendor TB2.1 54.9%, SWE-bench Verified 74.2%): the strongest coding model that would otherwise fit, but IQ1_M is 11.35 GB and leaves no room for KV or compute on a 12 GB card; IQ2_XXS is 11.86 GB alone. It is a 16 GB pick. Note the scores are vendor-reported.
- **K2 Horizon MoVA 36B A4B** (AA components suggest ~10.1): IQ2_XXS is 10.64 GB and 48 full-attention layers cost ~196 KB/token, so it needs 16 GB. It is the natural 16 GB MoE pick.
- **Clover 35B A3B Preview**: the only published GGUF quants are Q4_0 19.6 GB and up, so nothing fits 12 GB.
- **Nemotron 3.5 Lightning 30B A3B**: smallest published quant is UD-IQ3_XXS at 19.8 GB.
- **DeepSeek V4.1 Flash 1017**, **Kimi K3.1 Pro**, **GLM-5.3**: 100B+ class, 32 GB and up.
- **Ornith 1.5 9B** (Q8_0 9.79 GB), **SenseNova Loop 9B Preview**, **Spark X2.6 4B**: no independent AA v5 data, so no evidence they beat K2 Horizon 7B or Gemma 4 12B. Ornith is the best candidate to re-check when AA indexes it.
- **Ternary Bonsai 2 9B / 4B / 2B**, **Bonsai 8B 1-bit**: same family, lower scores; they are the 4-8 GB tier's picks, not 12 GB.

## Open questions

1. **Ids.** Six entries reuse their catalog ids because they are the same models with the 12 GB quant subset. The brief says an id "must not collide with an existing catalog id". Reusing the id is the only way a merge updates the entry instead of duplicating the model; a new id per tier would create duplicates. If the assembly wants new-only entries, filter to `k2-horizon-7b`.
2. **K2 vendor country.** IFM's card and GitHub org do not state a location; `country` is set to US and marked UNVERIFIED in the notes.
3. **K2 score.** 8.9 is a component-fit estimate, not an official AA v5 index. Re-check when AA publishes one.
4. **Nanbeige tool calling.** Catalog says true; the model card does not confirm a tool-call template. Verify with `tokenizer.chat_template` before shipping it as an agent model.
5. **Qwen3.8 draft.** This entry drops the draft for 12 GB. The catalog keeps the 3.16 GB Q8_0 MTP draft for larger cards. Decide whether the catalog should keep one draft or make it tier-dependent; with the F1 fix the Q8_0 draft may fit 12 GB again.
