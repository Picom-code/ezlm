# 64 GB tier: what to run when the machine has 64 GB of usable GPU or unified memory

64 GB is the tier where the November 2026 wave of 200B-550B MoE models stops fitting. At two
bits per weight a 200B model is ~50 GB, so the weights budget here is about 50-58 GB and the
rest goes to KV cache, the vision encoder and the OS. The useful picks split into two groups:

- dense 27-33B models at Q8, which still have the highest intelligence per byte at this size;
- 37-200B MoE models at 2-4 bits, which buy knowledge and long context at the cost of bit depth.

`agents/T02/tiers/tier-64.json` holds five new entries. The current catalog (v8) already
covers the two dense picks and two of the MoE picks, so the ranked table below shows both and
marks which are new.

## Ranked table

Scores are the Artificial Analysis Intelligence Index v5.0 (0-100 scale) unless marked
`est.`. "Fit" is weights + f16 KV cache at 32k context; the KV cache roughly halves with
q8_0 and quarters with q4_0.

| # | model | params / active | score | why it wins here | best quant for 64 GB | fit | licence | status |
|---|---|---|---|---|---|---|---|---|
| 1 | Nanbeige4.3-33B | 33.1B dense | 20.4 | Highest-scoring model that fits with room to spare; Q8 keeps it near lossless and 524k context is affordable | Q8_0 35.2 GB | ~42 GB | Apache-2.0 | in catalog v8 |
| 2 | Qwen3.8-27B | 27.8B dense | 20.1 | Best tool calling and vision in the tier, 262k context, Q8 fits in 29 GB | Q8_0 29.0 GB | ~31 GB | Apache-2.0 | in catalog v8 |
| 3 | Nemotron 3.5 Super VL 120B A12B | 124B / 12B | 13.6 | Biggest model that still fits (hybrid Mamba2, only 8 attention layers so KV is tiny); vision, 1M context | UD-IQ3_XXS 59.8 GB | ~60 GB | OpenMDW-1.1 | in catalog v8 |
| 4 | Step 3.7 Flash | 201B / 11B | 11.4 est. | Only 200B-class model that fits 64 GB; 3 of 4 layers are sliding-window so KV is 1.5 GB at 32k; vision | UD-IQ1_M 56.8 GB | ~59 GB | Apache-2.0 | **new** |
| 5 | Nemotron 3.5 Bolt 74B A9B | 74B / 9B | 11.5 | 74B MoE at Q4 with vision and 262k context; fast decode at 9B active | UD-Q4_K_XL 50.2 GB | ~51 GB | OpenMDW-1.1 | in catalog v8 |
| 6 | K2 Horizon MoVA 36B A4B | 37.4B / 4B | 10.6 est. | Fits at Q8 in 40 GB, leaving 20 GB for about 100k tokens of context (192 KiB/token); fully open weights, data and code | Q8_0 39.8 GB | ~46 GB | Apache-2.0 | **new** |
| 7 | Kimi Linear 48B A3B | 49.1B / 3B | 5.5 est. | 1M-token context is practical here: 7 of 27 layers are MLA attention, so KV is 0.5 GB at 32k | Q8_0 52.2 GB | ~53 GB | MIT | **new** |
| 8 | Qwen3 Coder Next | 79.7B / 3B | 5.3 est. | Coding-agent specialist with a 3B active footprint; 256k context and a tiny KV cache | Q5_K_M 56.7 GB | ~58 GB | Apache-2.0 | **new** |
| 9 | Mistral Medium 3.5 128B | 127.7B dense | 4.6 | Only dense 128B that fits; unified instruct/reasoning/coding with per-request effort, vision, 256k context | IQ2_M 44.1 GB (32k) or Q3_K_S 54.4 GB (16k) | ~61 GB at 32k | Modified MIT (USD 20M/month cap) | **new** |

Evidence, all accessed 2026-11-06:

- Nanbeige4.3-33B: <https://artificialanalysis.ai/models/nanbeige4-3-33b> (index 20.41),
  <https://huggingface.co/Nanbeige/Nanbeige4.3-33B>.
- Qwen3.8-27B: <https://artificialanalysis.ai/models/qwen3-8-27b> (xhigh 20.11, medium 17.48,
  low 15.69, non-reasoning 10.03), <https://huggingface.co/Qwen/Qwen3.8-27B>.
- Nemotron 3.5 Super VL 120B A12B: <https://artificialanalysis.ai/models/nemotron-3-5-super-vl>
  (13.62), <https://huggingface.co/nvidia/NVIDIA-Nemotron-3.5-Super-VL-120B-A12B-BF16>.
- Step 3.7 Flash: <https://artificialanalysis.ai/models/step-3-7-flash> (no official index;
  AA components Terminal-Bench Hard 35.6%, tau2 98.5%, IFBench 67.3%, GPQA 80.9%, SciCode 43.9%),
  <https://huggingface.co/stepfun-ai/Step-3.7-Flash>.
- Nemotron 3.5 Bolt 74B A9B: <https://artificialanalysis.ai/models/nemotron-3-5-bolt-74b-a9b>
  (no official index; catalog v8 score 11.5 is an estimate), <https://huggingface.co/nvidia/NVIDIA-Nemotron-3.5-Bolt-74B-A9B-BF16>.
- K2 Horizon MoVA 36B A4B: <https://artificialanalysis.ai/models/k2-horizon-mova-36b-a4b>
  (no official index; AA components AutomationBench 26%, GDP.pdf 4.2%, AA-Briefcase 1165 Elo,
  Terminal-Bench 4.0 0%), <https://huggingface.co/IFM/K2-Horizon-MoVA-36B-A4B>.
- Kimi Linear 48B A3B: <https://huggingface.co/moonshotai/Kimi-Linear-48B-A3B-Instruct>
  (vendor MMLU-Pro 51.0, RULER-128k 84.3; no AA data), <https://artificialanalysis.ai/models/kimi-linear-48b-a3b-instruct>.
- Qwen3 Coder Next: <https://huggingface.co/Qwen/Qwen3-Coder-Next> (256k context, 3B active,
  non-thinking), <https://artificialanalysis.ai/models/qwen3-coder-next> (components only).
- Mistral Medium 3.5 128B: <https://artificialanalysis.ai/models/mistral-medium-3-5> (4.65),
  <https://huggingface.co/mistralai/Mistral-Medium-3.5-128B>.

## How the five new entries were chosen

**Step 3.7 Flash (rank 4).** The official StepFun GGUF starts at IQ3_XXS (75.8 GB), which is
over budget; unsloth's IQ1_M is 56.8 GB and IQ2_XXS is 61.7 GB. Vision adds a 4.0 GB mmproj,
so with images IQ1_M totals about 62.7 GB at 32k and IQ2_XXS does not leave room for vision. The architecture is what makes
this work: 45 layers, one full-attention layer in four (12 layers), the rest on a 512-token
sliding window, so the KV cache is 48 KiB/token, about 1.5 GB at 32k. It is a vision model
(mmproj in the same repo) with selectable reasoning levels. The risk is the bit depth: 2.3 bits
per weight. Its AA component scores are strong for a model this compressed, so it is the
best value if you want a 200B-class model on a 64 GB box, and the first thing to drop if a
benchmark shows IQ1_M has collapsed.

**K2 Horizon MoVA 36B A4B (rank 6).** The safest new pick. At Q8_0 it is 39.8 GB, so 64 GB
buys the near-lossless quant and a 128k context (6 GB of KV at 32k) at once. 524k native
context, 4B active, Apache-2.0, and the training data and code are public, which no other
2026 release at this size offers. Score is an estimate, so it should be validated before it
outranks the incumbents anywhere.

**Kimi Linear 48B A3B (rank 7).** Bought for one property: 1M-token context that fits. Seven
of 27 layers use MLA attention, so KV is 15.8 KiB/token (0.5 GB at 32k, 16 GB at 1M). MIT
licence, 3B active. The score is a hand placement, not a measurement, so treat it as a
long-context specialist rather than a general pick.

**Qwen3 Coder Next (rank 8).** The coding pick. 80B total, 3B active, 256k context, 12 of 48
layers on full attention, so KV is 24 KiB/token. It is non-thinking only. Its general index
estimate (5.3) is dragged down by a weak AA-Briefcase score (404 Elo), but it is built for
agentic coding and its Q5_K_M is 56.7 GB.

**Mistral Medium 3.5 128B (rank 9).** The only dense 128B that fits, with vision and a unified
instruct/reasoning mode. The catch is the KV cache: 88 layers x 8 KV heads is 352 KiB/token,
11 GB at 32k. At Q3_K_S (54.4 GB) the KV cache pushes 32k to 66 GB, so Q3_K_S is a 16k-context
pick; the IQ2_M file (44.1 GB) is the one that keeps 32k. Vision adds a 5.4 GB
mmproj, so with images use IQ2_M. Its AA index (4.65) is the lowest in the table;
it is here for dense-model stability and multilingual coverage, not for score.

## Drops, and why

| model | params | score | smallest quant | why it does not fit or does not win |
|---|---|---|---|---|
| Qwen3.8-Flash-Next | 180B / 6B | 27.0 est. | UD-IQ1_S 66.4 GB | The best open model that almost fits: 2.4 GB over budget before any KV. On a 64 GB box the planner must reject it. First pick for the 96 GB tier. |
| GLM-5.3-Flash | 320B / 18B | 27.8 | UD-IQ1_S 93.1 GB | 29 GB over budget. |
| MiniMax-M3.1-Flash | 428B / 23B | 29.1 | UD-IQ1_M 129.4 GB | Needs 128 GB+. |
| DeepSeek V4.1 Flash 1017 | 552B / 16B | 32.6 | UD-IQ1_S 233.0 GB | 96 GB+ tier and above. |
| K2 Horizon 375B A23B | 375B / 23B | 17.8 | IQ1_S 85.0 GB | 21 GB over budget; the 96 GB tier should take it. |
| Inkling Small | 266B / 12B | 12.9 | UD-IQ1_S 74.8 GB | 11 GB over budget. |
| Command A+ | 218B / 25B | 6.2 | IQ1_S 49.3 GB | Fits only at 1 bit and scores below every pick above. |
| Solar Open 100B | 102B / 12B | no AA index | IQ4_XS 55.5 GB | Plausible but unmeasured, and the Upstage Solar licence is more restrictive than the picks. |
| Nemotron 3.5 Bolt 74B A9B at Q5 | 74B / 9B | 11.5 | UD-Q5_K_XL 58.9 GB | Fits, but Q4_K_XL (50.2 GB) leaves more room for context and the extra bit is not worth it here. |
| Kimi K3.1 Pro, Muse Spark Open, Nemotron 3 Ultra, Step 5 Preview, Ling 3.1 Flash | 550B-2.8T | 11.8-46.5 | 75-270 GB | All need 96 GB or more. |

## Notes for the planner

- Every entry has `layers`, `kv_heads`, `head_dim` and `kv_layers` read from the GGUF header
  with range requests, and `scripts/verify_catalog.py --catalog agents/T02/tiers/tier-64.json`
  reports 0 errors on all 12 file sets.
- Qwen3 Coder Next gets a verifier warning ("kv_layers 12 < 48 attention layers") because the
  GGUF does not carry `full_attention_interval`; the 48 layers are 12 Gated DeltaNet blocks of
  4 layers each, and only the last layer of each block has full attention. 12 is correct, and
  matches how the catalog treats Qwen3.8-Flash-Next.
- Kimi Linear is MLA: `kv_heads` is 1 and `head_dim` is 576 (kv_lora_rank 512 + rope 64),
  following the catalog's MLA convention.
- Vision flags are set only where the GGUF repo ships an `mmproj` file (Step 3.7 Flash,
  Mistral Medium 3.5). K2 Horizon, Kimi Linear and Qwen3 Coder Next have no vision tower in
  their GGUF repos.
- Mistral Medium 3.5 GGUFs built before the 2026-07-15 config fix have degraded long-context
  behaviour; the unsloth files checked here are from after the fix.
