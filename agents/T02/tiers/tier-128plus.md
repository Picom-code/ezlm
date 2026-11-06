# 128 GB and above: model proposals (T02, agent a11)

Researched 2026-11-06. Sizes are decimal GB (10^9 bytes), the same unit the catalog uses.
Every byte size comes from the Hugging Face tree API, and every layer, KV-head, head-size
and context number comes from the GGUF header of the first file of each quant, read with
HTTP range requests. `scripts/verify_catalog.py --catalog agents/T02/tiers/tier-128plus.json`
passes on all 35 quant sets with no errors and no warnings.

## What the tier can hold

`internal/fit` keeps a 10% safety margin, so the weights plus the KV cache plus the
compute buffers must fit in about 90% of the machine's memory. A 128 GB machine therefore
has about 115 GB, and the table below uses that budget with a 64K context and an f16 KV
cache. Planned sizes are weights plus cache plus buffers, computed with the formulas in
`internal/fit/fit.go`, and effective scores apply the quant factors from the same file.

| Machine | Budget | Best plan at this size | Planned size | Effective score |
|---|---|---|---|---|
| 128 GB | 115 GB | Qwen3.8-Flash-Next UD-Q4_K_XL (already cataloged, see below) | 85.6 GB | 26.6 |
| 128 GB | 115 GB | Ornith 1.5 122B UD-Q5_K_XL | 97.8 GB | 25.8 (estimate) |
| 128 GB | 115 GB | GLM-5.3-Flash UD-Q2_K_XL | 112.0 GB | 25.6 |
| 160 GB | 144 GB | Qwen3.8-Flash-Next UD-Q5_K_XL | 133.0 GB | 26.8 |
| 160 GB | 144 GB | GLM-5.3-Flash UD-IQ3_XXS | 123.8 GB | 26.3 |
| 160 GB | 144 GB | Ornith 1.5 122B UD-Q6_K_XL | 117.6 GB | 25.9 (estimate) |
| 192 GB | 173 GB | DeepSeek V4.1 Flash 1017 UD-IQ2_XXS | 172.9 GB | 28.7 |
| 192 GB | 173 GB | MiniMax-M3.1-Flash UD-IQ3_XXS | 170.3 GB | 27.5 |
| 256 GB | 230 GB | DeepSeek V4.1 Flash 1017 UD-Q2_K_XL | 207.6 GB | 30.0 |
| 256 GB | 230 GB | MiniMax-M3.1-Flash UD-Q3_K_XL | 204.5 GB | 28.0 |
| 320 GB | 288 GB | DeepSeek V4.1 Flash 1017 UD-IQ3_XXS | 241.7 GB | 30.8 |
| 320 GB | 288 GB | GLM-5.3 UD-IQ2_M | 253.3 GB | 28.5 |
| 384 GB | 346 GB | GLM-5.3 UD-IQ3_XXS | 296.9 GB | 29.9 |
| 448 GB | 403 GB | GLM-5.3 UD-IQ4_XS | 381.3 GB | 31.0 |

## Ranked proposals

Ranked by the effective score each model reaches inside the tier. The ladder above maps
them to machine sizes, so a 128 GB machine should read the first three rows of that table.

| # | Model | Why it wins | Benchmark evidence | Best quant here | Planned size | Licence |
|---|-------|-------------|--------------------|-----------------|--------------|---------|
| 1 | DeepSeek V4.1 Flash 1017 | AA 32.59, the strongest open model with a working GGUF. A 196.6B lookup table can stay on disk, so the resident body is 165.3 GB at IQ2_XXS and 199.6 GB at Q2_K_XL. Wins from 192 GB up. | Artificial Analysis Intelligence Index v5.0, 32.59, fetched 2026-11-06; card: HLE without tools 42.6 | UD-IQ3_XXS | 241.7 GB | MIT |
| 2 | GLM-5.3 | AA 31.65, the best model that needs no lazy loading. 753B total, 78 full-attention layers with a 576-wide MLA cache. Wins at 448 GB where its IQ4_XS reaches 31.0. | Artificial Analysis v5.0, 31.65; card: Terminal-Bench 2.1 88.2, DeepSWE 66.9, NL2Repo 58.0 | UD-IQ4_XS | 381.3 GB | glm-5.3 (custom) |
| 3 | MiniMax-M3.1-Flash | AA 29.06. 428B total, 23B active, native multimodal, 1M context. The best plan at 192 GB and a strong second at 256 GB. | Artificial Analysis v5.0, 29.06; card: DeepSWE v1.1 73.7, OSWorld V2 61.7, CyberGym 82.2 | UD-IQ3_XXS | 170.3 GB | minimax-community |
| 4 | GLM-5.3-Flash | AA 27.8, the only new model measured above the current 180B flagship that fits a 128 GB machine. 320B total, 18B active, hybrid linear attention keeps the cache at 24 KB/token. | Artificial Analysis v5.0, 27.8, fetched 2026-11-06 | UD-Q2_K_XL (128 GB), UD-IQ3_XXS (160 GB) | 112.0 / 123.8 GB | MIT |
| 5 | Ornith 1.5 122B | Coding and agentic specialist that fits 128 GB at Q5 and 160 GB at Q6. Beats the current 180B flagship on SWE-bench Pro and Verified in the developer's evals. | Card 2026-10-16: SWE-bench Pro 76.9, SWE-bench Verified 84.5, Terminal-Bench 2.1 81.2 | UD-Q5_K_XL | 97.8 GB | MIT (link is a 404) |
| 6 | Flint | Coding agent specialist: 39.8B total, 2.5B active, so it is fast with a 256K context. It does not need the tier's capacity; it is the speed pick. | Card 2026-10-21: Terminal-Bench 2.1 54.9, SWE-bench Pro v2 89.7, SWE-bench Verified 74.2 | Q8_0 | 45.4 GB | Apache-2.0 |

## The best 128 GB option is already in the catalog, but its quants are cut short

`qwen3.8-flash-next` is cataloged with quants up to UD-IQ3_XXS (75.8 GB). The unsloth
repo now has larger files, and the two that matter for this tier are:

- UD-IQ4_XS, 93.67 GB, with a 28.80 GB `per_layer_token_embd` table (IQ4_NL, 4.5 bits)
  that can stay on disk: 64.87 GB resident, 26.4 effective.
- UD-Q4_K_XL, 111.33 GB, same 28.80 GB table: 82.53 GB resident, 26.6 effective at
  85.6 GB planned.

Q4_K_XL scores slightly better than GLM-5.3-Flash at IQ3_XXS (26.6 vs 26.3) and uses
about 38 GB less planned memory, so it is the best 128 GB plan in this study. The catalog
entry should be extended with IQ4_XS and Q4_K_XL, and with Q5_K_XL (158.3 GB),
Q6_K_XL (169.2 GB) and Q8_0 (188.2 GB) for 160 to 256 GB machines, all with the same
28.80 GB lazy table. That is a `catalog.json` edit, so it is reported here and in the
report rather than made in this branch.

## Why each pick

### DeepSeek V4.1 Flash 1017 (rank 1)

Released 2026-10-17, 763B total, MIT, multimodal, 1M context. AA 32.59, the best score of
any open model that has a working GGUF today. 40 layers, top-6 of 384 experts, about 13B
active (estimated from the config).

The file layout is unusual and it is the reason this entry has `lazy_bytes`: 196.6B of the
763B parameters are a single `per_layer_token_embd` table (768M rows of 256, an engram)
stored as a separate shard of every quant. It is read a few rows per token, so it can stay
on disk with `--lazy-mode`, exactly like the n-gram table in the cataloged
`qwen3.8-flash-next`. The tensor is IQ4_NL in IQ1_S and IQ1_M (110.6 GB), Q5_K in
IQ2_XXS and Q2_K_XL (135.2 GB) and Q6_K in IQ3_XXS (161.3 GB).

Resident sizes: 122.4 GB at IQ1_S (233.0 GB on disk), 139.8 GB at IQ1_M, 165.3 GB at
IQ2_XXS (300.5 GB), 199.6 GB at Q2_K_XL (334.8 GB), 233.5 GB at IQ3_XXS (394.7 GB).
The 0.85 quant floor for 150B+ models skips IQ1_S and IQ1_M, so the first usable quant is
IQ2_XXS: a 192 GB machine at q8 KV or 32K context, comfortable at 256 GB. IQ3_XXS is the
320 GB pick and the best plan in the tier at that size (30.8).

Sources: https://huggingface.co/deepseek-ai/DeepSeek-V4.1-Flash-1017 (card, 2026-10-17);
https://artificialanalysis.ai/leaderboards/models (fetched 2026-11-06); GGUF metadata and
the engram tensor from https://huggingface.co/unsloth/DeepSeek-V4.1-Flash-1017-GGUF.

### GLM-5.3 (rank 2)

Released 2026-08-25, 753B total with top-8 of 256 experts. AA 31.65. MLA attention with
a 576-wide compressed cache (512 latent plus 64 rope), 176 KB/token f16 and 11.5 GB at
64K. No lazy table, so the file size is the resident size, which makes it the pick when
lazy loading is not wanted.

The card reports Terminal-Bench 2.1 88.2, Terminal-Bench 3.0 28.3, DeepSWE 66.9,
NL2Repo 58.0 and Toolathlon Verified 73.0, all developer-run. The 0.85 quant floor skips
IQ1_S and IQ1_M, so the smallest usable file is UD-IQ2_M at 238.6 GB (253.3 GB planned,
a 320 GB machine). At 448 GB, UD-IQ4_XS (365.3 GB, 381.3 GB planned) reaches 31.0, the
highest effective score in the tier short of DeepSeek at the same size. The licence is the
custom `glm-5.3` licence, not MIT.

Sources: https://huggingface.co/zai-org/GLM-5.3 (card, 2026-08-25);
https://artificialanalysis.ai/leaderboards/models (fetched 2026-11-06); GGUF metadata
from https://huggingface.co/unsloth/GLM-5.3-GGUF.

### MiniMax-M3.1-Flash (rank 3)

Released 2026-10-17, 428B total and 23B active, native multimodal with a 1M context. AA
29.06, and the card claims DeepSWE v1.1 73.7, OSWorld V2 61.7 and CyberGym 82.2. Full
attention on all 60 layers with 4 KV heads of 128: 120 KB/token f16, 7.9 GB at 64K, which
is the largest cache of the proposals here.

The smallest file is UD-IQ1_M at 129.4 GB (139.3 GB planned), but the 0.85 quant floor
skips it. The first usable quant is IQ2_XXS (144.9 GB planned): a 160 GB machine at 32K
context, comfortable at 192 GB. At 192 GB, UD-IQ3_XXS (160.1 GB, 170.3 GB planned) is the
best plan in the tier at that size (27.5), and at 256 GB UD-Q3_K_XL (194.0 GB, 204.5 GB
planned) reaches 28.0. The reference checkpoint is 252 GB, so the GGUF is not far from
the original format.

Sources: https://huggingface.co/MiniMaxAI/MiniMax-M3.1-Flash (card, 2026-10-17);
https://artificialanalysis.ai/leaderboards/models (fetched 2026-11-06); GGUF metadata
from https://huggingface.co/unsloth/MiniMax-M3.1-Flash-GGUF.

### GLM-5.3-Flash (rank 4)

Released 2026-08-25, 320B total and 18B active. The first GLM with hybrid sparse and
linear attention: only 12 of the 45 layers keep a KV cache, so the cache is 24 KB/token
f16, about 1.6 GB at 64K. The GGUF carries an mmproj, so vision works. The header gives
46 blocks including one MTP layer, 1 KV head of 512 (MLA latent 512, no rope), and a 1M
context. Artificial Analysis puts it at 27.8, above Qwen3.8-Flash-Next (27.0 estimate in
the catalog) and well above the dense 27B class.

At 128 GB the only plan that fits is UD-Q2_K_XL (108.7 GB, 112.0 GB planned, 25.6) or
UD-IQ2_XXS (101.8 GB, 105.1 GB planned, 24.5). On a 160 GB machine UD-IQ3_XXS (120.4 GB,
123.8 GB planned) reaches 26.3, and on a 192 GB machine UD-Q3_K_XL (147.5 GB, 151.2 GB
planned) reaches 26.7. There is no lazy table, so the file size is the resident size.

Sources: https://huggingface.co/zai-org/GLM-5.3-Flash (card, 2026-08-25);
https://z.ai/blog/glm-5.3-flash; https://artificialanalysis.ai/leaderboards/models
(fetched 2026-11-06); GGUF metadata from
https://huggingface.co/unsloth/GLM-5.3-Flash-GGUF.

### Ornith 1.5 122B (rank 5)

Released 2026-10-16, 125B total and about 6B active, built on Qwen3.5 and Gemma4 bases
with a self-improvement loop. It fits 128 GB from Q3_K_XL (59.8 GB) up to Q5_K_XL
(94.6 GB, 97.8 GB planned), so the machine can run it at near lossless quality. Full
attention on 12 of 48 layers, 2 KV heads of 256, 262K context, mmproj included.

The evidence is developer-run, not independent: SWE-bench Pro 76.9 against 43.9 for
Qwen3.8-Flash-Next, SWE-bench Verified 84.5 against 82.8, HLE with tools 48.0 against
38.0, Terminal-Bench 2.1 81.2 against 83.4. The card documents the harness (Claude Code
2.1.126, OpenHands, 256K context, anti-hacking instructions). It is not on Artificial
Analysis, so the score of 26.0 is an estimate placed between Qwen3.8-Flash-Next (27.0)
and Nanbeige4.3-33B (20.4). The card's MIT link points at a missing LICENSE file, so the
licence is unverified, and the vendor country is not stated.

Sources: https://huggingface.co/ornith-ai/Ornith-1.5-122B (card, 2026-10-16);
https://ornith.ai/ornith_1_5.html; GGUF metadata from
https://huggingface.co/unsloth/Ornith-1.5-122B-GGUF.

### Flint (rank 6, the coding speed pick)

Released 2026-10-21 by Recursive, 39.8B total and 2.5B active, built for coding agents.
49 layers, 35 sliding-window layers of 513 tokens and 14 full-attention layers, 4 KV heads
of 128: 28 KB/token f16 plus a fixed window cache. 262K context, Apache-2.0, tool calling
documented on the card.

The card reports Terminal-Bench 2.1 54.9 (Terminus 2), SWE-bench Pro v2 89.7 and
SWE-bench Verified 74.2, developer-run. It is not on Artificial Analysis, so the score of
19.0 is an estimate and should not be compared with the general picks. At 128 GB it runs at
Q8_0 (42.3 GB, 45.4 GB planned) with a long context and a lot of memory left over, which
makes it the model to pick when speed matters more than the general index. It also fits
machines well below this tier, so it will likely appear in the smaller tiers too.

Sources: https://huggingface.co/RecursiveSI/flint (card, 2026-10-21);
https://www.recursive.com/updates/introducing-recursive-flint; GGUF metadata from
https://huggingface.co/unsloth/flint-GGUF.

## Rejected, and why

- **Kimi K3.1 Pro** (AA 46.45, the strongest open model) has no GGUF as of 2026-11-06.
  `unsloth/Kimi-K3.1-Pro` is a safetensors mirror and
  `DevQuasar/moonshotai.Kimi-K3.1-Pro-GGUF` held only a README when checked. Kimi K3, its
  predecessor, has a smallest quant of UD-Q1_0 at 466.4 GB, so K3.1 Pro will need 512 to
  768 GB. Revisit when GGUFs land; it would take rank 1 at the top of the range.
- **Qwen3.8-2.4T-A95B** is 2.4T parameters; nothing fits 128 to 512 GB at a usable quant.
- **Mistral Large 4 1T-A52B** (AA 26.12): smallest file 233.0 GB, and it is behind
  DeepSeek V4.1 Flash and GLM-5.3 at the same memory.
- **Nemotron 3 Ultra 550B A55B** (AA 11.76): smallest file 188.0 GB, far behind the
  models that need the same memory.
- **Muse Spark Open 816B A42B** (AA 24.88): smallest file 304.1 GB, behind DeepSeek
  V4.1 Flash at a similar size.
- **MiMo-V2.6-Pro** (AA 29.89): smallest file 418.8 GB, and GLM-5.3 at 381 GB planned
  scores better effectively (31.0 vs 29.9 before quant loss).
- **MiMo-V2.6-Flash** (AA 26.73): the only GGUF is Q2_K at 126.4 GB, which leaves about
  1 GB for the cache on a 128 GB machine, and there is no smaller file to fall back to.
  It needs 160 GB to be safe, and there its Q2_K_XL equivalent reaches 24.6 effective,
  behind GLM-5.3-Flash IQ3_XXS at 26.3 and 16 GB smaller.
- **Ling 3.1 Flash 1019** (AA 29.57): smallest file 131.0 GB, just over 128 GB, and at
  192 GB its IQ2_XXS (170.8 GB) gives 26.0, behind MiniMax-M3.1-Flash IQ3_XXS (27.5 at
  170.3 GB).
- **K2 Horizon 375B A23B** (AA 17.82): fits at IQ2_XXS 111.0 GB but is far behind the
  models that fit the same memory.
- **Beam 1.0 501B A23B** (AA 17.73): smallest quant is about 140 GB and the score is
  well below the 160 GB class.
- **Inkling Small** (AA 12.91): smallest file 74.8 GB, dominated by Ornith at 128 GB.
- **Dense models**: no new dense model above 40B appeared between September and November
  2026. The best dense picks remain Nanbeige4.3-33B (AA 20.4) and Qwen3.8-27B (AA 20.1),
  both already cataloged; at 128 GB they run at Q8 (35 to 36 GB) and do not use the tier.
  Every proposal above is a mixture of experts.
- **Qwen3.8-27B, Qwen3.8-Flash-Next, Nanbeige4.3-33B, Nemotron 3.5, Gemma 4, gpt-oss,
  Granite 4.2, Mistral Small 4**: already in the catalog, so they are not re-proposed.
  Qwen3.8-Flash-Next needs its quant list extended, as described above.

## Open questions

- The Artificial Analysis numbers were read from the leaderboard page on 2026-11-06. Its
  legacy data API was retired on 2026-11-04, so per-model component scores were not
  extracted; the index values are the ones the catalog already uses for its other entries.
- Ornith 1.5 122B and Flint have no independent index. Their scores are estimates and are
  marked as such in `score_source`.
- DeepSeek V4.1 Flash 1017 assumes the engram table is safe to keep on disk with
  `--lazy-mode`, the same mechanism the catalog already uses for Qwen3.8-Flash-Next. That
  should be confirmed on a real machine with a long prompt before the entry is trusted for
  a 192 GB plan.
- GLM-5.3's active parameter count (about 32B) is estimated from the config because the
  card does not state it. DeepSeek's (about 13B) is estimated the same way.
- The MiniMax card says 428B total, but the Hugging Face safetensors count is 220.5B
  because the reference checkpoint stores experts in U8 and F8 at about 4.7 bits. Size
  estimates must use the 428B figure, which the GGUF file sizes confirm.
