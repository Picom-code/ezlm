# T02 tier 6 GB: proposed catalog entries

Tier meaning: 6 GB of usable GPU memory, or 6 GB of usable unified memory (for example an 8 GB Apple machine with a 6 GB wired limit). Everything below fits that budget after EZLM's safety margins: 5.6 GiB on a discrete card (0.4 GiB margin) or 5.0 GiB on unified memory (1 GiB OS floor). Resident size = weights + KV cache + llama.cpp overhead (0.32 GiB + 4 KiB per token of context + 1% of weights).

All entries are new ids; none collides with `internal/catalog/catalog.json`. Sizes and GGUF metadata were read from the Hugging Face tree API and with HTTP range requests on 2026-11-06, and `python3 scripts/verify_catalog.py --catalog agents/T02/tiers/tier-6.json` passes for all 11 file sets.

## Ranked picks

| Rank | Model | Why it wins | Benchmark evidence | Best quant for 6 GB | Licence |
|---|---|---|---|---|---|
| 1 | K2 Horizon 7B | Strongest agentic and coding model that fits. AA Terminal-Bench 2.1 44.2 beats every small model here; 512K native context. | AA 2026-11-06 ([page](https://artificialanalysis.ai/models/k2-horizon-7b)): GPQA 75.3, HLE 18.2, TB2.1 44.2, tau3-Banking 27.6, SciCode 27.5, AA-LCR 69.7. Vendor card ([IFM/K2-Horizon-7B](https://huggingface.co/IFM/K2-Horizon-7B)): SWE-bench Verified 72.4, BFCL v4 67.0. | IQ3_XXS, 4.07 GB, 16K context at q4_0 KV is ~4.8 GiB (32K ~5.5 GiB) | Apache-2.0 |
| 2 | K2 Horizon 3.7B | Same architecture and tool use as the 7B at half the weights, so it keeps 32K context inside 6 GB. Best capability per byte. | AA 2026-11-06 ([page](https://artificialanalysis.ai/models/k2-horizon-3-7b)): GPQA 69.2, HLE 13.9, TB2.1 28.1, tau3-Banking 19.6, SciCode 22.0, AA-LCR 62.3. Vendor card ([IFM/K2-Horizon-3.7B](https://huggingface.co/IFM/K2-Horizon-3.7B)): AIME25 89.2, SWE-bench 65.6, BFCL v4 64.6. | Q4_K_M, 3.16 GB, 32K context at q4_0 KV is ~4.7 GiB; Q6_K 4.16 GB at 16K is ~4.9 GiB | Apache-2.0 |
| 3 | Ministral 3 8B Reasoning | Only pick with vision, 256K context and native function calling in this tier. Reasoning variant is much stronger than the instruct run AA measured. | Vendor card ([Mistral](https://huggingface.co/mistralai/Ministral-3-8B-Reasoning-2512)): AIME25 78.7, GPQA 66.8, LiveCodeBench 61.6. AA 2026-11-06 ([instruct mode](https://artificialanalysis.ai/models/ministral-3-8b)): GPQA 47.1, HLE 4.3, LCB 30.3, tau2 26.6. | UD-Q3_K_XL, 4.43 GB, 16K context at q4_0 KV is ~5.1 GiB | Apache-2.0 |
| 4 | LFM2.5 8B A1B | Fastest pick: 1.5B active parameters and only 6 attention layers, so the KV cache is tiny. Good instruction following and tool calls on weak hardware. | AA 2026-11-06 ([page](https://artificialanalysis.ai/models/lfm2-5-8b-a1b)): GPQA 51.3, HLE 6.9, IFBench 55.6, tau2 16.1. | Q4_0, 4.84 GB, 32K context at q4_0 KV is ~5.2 GiB (64K ~5.5 GiB) | LFM Open License v1.0 |

## Why each pick

### 1. K2 Horizon 7B

The 6 GB tier cannot run a 12B or a 20B at a usable quant, so the question is which 7B class model is smartest. K2 Horizon 7B is the only small model with independent agentic numbers this high: AA measured Terminal-Bench 2.1 at 44.2, close to Qwen3.6-35B-A3B (44.9) and far above Qwen3.5 9B (29.2) and Gemma 4 12B (27.3), while using 7B of weights. Its reasoning numbers are weaker than the bigger models (GPQA 75.3 versus 84.1 for Qwen3.6-35B-A3B), so the estimated score of 8.5 sits just under the catalog's Qwen3.6-35B-A3B (9.5). At 6 GB it fits at 16K context with q4_0 KV, and 32K fits on a 5.6 GiB discrete budget.

The official IFM GGUF starts at Q4_K_M (5.59 GB), which leaves too little for KV at 6 GB. Use bartowski's IQ3_XXS (4.07 GB) or Q3_K_M (4.42 GB). IQ3_XXS is the better trade: 4.8 GiB resident at 16K versus 5.2 GiB for Q3_K_M, and the planner's context factor rewards the extra context.

### 2. K2 Horizon 3.7B

Same 36-layer, 8 KV-head, 128-dim shape as the 7B, so KV costs the same, but the weights are 2.5 GB smaller. That buys 32K context at Q4_K_M (4.7 GiB) or a better quant at 16K (Q6_K, 4.9 GiB). AA numbers are roughly Nemotron 3.5 Lightning level (GPQA 69.2, HLE 13.9, TB2.1 28.1) and the vendor reports strong math and SWE-bench scores. It is the right pick for a 6 GB machine that needs a 32K agent context more than the last point of reasoning.

The GGUF files are named `K2-Horizon-4B-*` even though the model is 3.7B; the entry uses the real 3.7B parameter count and the repo `IFM/K2-Horizon-3.7B-GGUF`.

### 3. Ministral 3 8B Reasoning

The only 6 GB pick with an image encoder (0.4B, 858 MB F16 mmproj) and a 256K context window. It is also the only one with vendor-published reasoning numbers near the K2 pair (AIME25 78.7, GPQA 66.8, LCB 61.6). AA's independent run of the same family is much lower (GPQA 47.1, HLE 4.3), because it evaluates the non-thinking instruct mode; the entry score of 5.0 is the midpoint of the two, and the notes say so.

Fit: UD-Q3_K_XL (4.43 GB) at 16K is 5.1 GiB; IQ4_XS (4.71 GB) fits at 8K (5.1 GiB). EZLM does not load mmproj today, so vision is declared for future use, not as a shipped feature.

### 4. LFM2.5 8B A1B

The MoE option. 8.3B total but 1.5B active, and only 6 of 24 blocks have attention, so KV is about 7 KB per token at q4_0. That means 32K context costs almost nothing: Q4_0 at 32K is 5.2 GiB and 64K is 5.5 GiB. It is the fastest thing in the tier on CPU and small GPUs, and its instruction following is good (AA IFBench 55.6). Knowledge and hard coding are clearly below the K2 models, so it ranks last despite being the most pleasant to use on slow hardware.

## Drops and why

- **Gemma 4 12B (already in catalog, score 3.5).** At UD-IQ3_XXS (4.64 GB) it fits 32K at q4_0 KV (~5.5 GiB) thanks to its small global KV (1 KV head x 512, 8 global layers). Its AA numbers (GPQA 75.3, HLE 15.7, TB2.1 27.3, IFBench 73.5) are respectable, but K2 Horizon 7B is better on agentic work and this id already exists, so it is not proposed again.
- **Qwen3.5 9B (already in catalog, score 5.0).** AA GPQA 80.6 but TB2.1 29.2 and tau3-Banking 7.0; the K2 pair is the better agentic pick and this id already exists.
- **Gemma 4 E4B / E2B (already in catalog).** Fine for 4 GB, not competitive at 6 GB.
- **K2 Horizon 7B Q4_K_M (5.59 GB) and IQ4_XS (5.24 GB).** Q4_K_M plus KV exceeds 5.6 GiB even at 8K. IQ4_XS only fits at 8K; its quality gain (0.978 versus 0.945) does not make up for the context factor (0.60 versus 0.85), so IQ3_XXS at 16K wins.
- **K2 Horizon 3.7B Q8_0 (5.39 GB).** Fits only at 8K and Q6_K at 16K is both smaller and better placed for agent use.
- **Ministral 3 14B.** At 6 GB it needs IQ2 (about 4.6 to 5.0 GB), and the quality loss plus the KV cost at 34 layers x 8 heads leaves little context. The 8B is the right size for this tier.
- **LFM2.5 8B A1B Q5_K_M (6.03 GB).** Over the weight budget before any KV cache.
- **LFM2.5 24B A2B.** Too large at every quant that fits 6 GB.
- **Qwen3.8 27B IQ2 (7.27 GB), Nanbeige4.3 33B IQ2 (10.07 GB), gpt-oss-20b MXFP4 (12.11 GB), Qwen3.6-35B-A3B IQ2 (10.76 GB).** All exceed 6 GB in weights alone.
- **Ministral 3 3B.** AA GPQA 35.8, AIME25 22.0, LCB 24.7; the K2 3.7B is stronger at a similar quantised size.

## Caveats

- Artificial Analysis has no v5 Intelligence Index for any of these four models. The scores are estimates mapped from AA's per-benchmark runs and the nearest catalog scores, and each `score_source` says exactly how. Treat them as unverified.
- Vendor benchmark tables (K2 cards, Mistral card) are self-reported. They are used only where AA has no equivalent, and are labelled as vendor claims in the table above.
- AA's Ministral 3 8B page measures the non-thinking mode. The thinking-mode score is an estimate between the vendor numbers and AA's run.
- The 6 GB budget assumes the whole model is resident on the GPU. A discrete card with 6 GB free after desktop use gives 5.6 GiB after EZLM's margin; an 8 GB unified machine with a 6 GB wired limit gives 5.0 GiB after the OS floor. The recommended quants are chosen to fit the tighter 5.0 GiB case at 8 to 16K context where possible.
- Vision for Ministral 3 8B needs `mmproj-F16.gguf` (858 MB), which EZLM does not load today.
