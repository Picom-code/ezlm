# 48 GB tier, November 2026

"48 GB usable" means 48 GB of VRAM or unified memory after the planner's safety
margin. At 48 GB the model has to live entirely in that pool, so the working
budget is about 44 GB for weights plus KV cache, leaving room for compute
buffers. Every entry below was checked against llama.cpp b11960 with
`scripts/verify_catalog.py` (19 quant sets, 0 errors) and against the real GGUF
headers over HTTP range requests.

**Headline:** the strongest new models of late 2026 (Kimi K3.1 Pro, DeepSeek
V4.1 Flash, GLM-5.3, Mistral Large 4, Ling 3.1, MiMo-V2.6, MiniMax-M3.1,
Muse Spark) are 200B to 2.8T MoEs. Even their 1-bit quants are 93 GB to 130 GB,
so none of them fits 48 GB. This tier is decided by 30B to 60B models, and the
best of those are dense 27B to 33B models at Q8 or small MoEs at Q6. The catalog
already holds the two strongest full-fit models here (`qwen3.8-27b` at 20.1 and
`nanbeige4.3-33b` at 20.4, both in `internal/catalog/catalog.json`), so the five
proposals below add capability classes rather than displace them.

## Ranked picks

| # | Model | Why it wins | Benchmark evidence (fetched 2026-11-06) | Best quant for 48 GB | Licence |
|---|---|---|---|---|---|
| 1 | **Flint 40B A2.5B** | Best coding agent that fits: 40B total, 2.5B active, 262K context, and a KV cache of only 1.9 GB at 64K because 35 of 49 layers are sliding-window | Vendor: Terminal-Bench 2.1 54.9%, SWE-bench Verified 74.2%, SWE-bench Pro v2 89.7% ([model card](https://huggingface.co/RecursiveSI/flint), 2026-11-06). No AA row yet | `UD-Q6_K_XL` 36.32 GB, full 262K context (36.3 + 7.5 GB KV) | Apache-2.0 |
| 2 | **K2 Horizon MoVA 36B A4B** | Best new general/agentic MoE: 4B active, 524K context, AA-Briefcase 1165 and TB2.1 56.6% beat every other sub-40B new model | AA components 2026-11-06: AA-Briefcase 1164.8, GDPval-AA 1196.7, AutomationBench 26.1%, HLE-Diamond 23.4%, AA-LCR 71.7%, TB2.1 56.6%, SciCode 40.0%, GPQA 82.2%, tau3 32.0%. Vendor card: TB2.1 58.6, HLE 25.2, GPQA 80.8 | `Q6_K` 30.77 GB at 64K (30.8 + 12.9 GB KV) | Apache-2.0 |
| 3 | **HyperNova 60B 2605** | Largest general model that fits fully: 60B total, 4.8B active, native OpenAI-style tool calling, strong IFBench and tau2 | AA 2026-11-06: IFBench 66.5%, tau2 63.2%, GPQA 73.3%, HLE 16.6%, AA-LCR 36.3%, GDPval-AA 448.6, AA-Briefcase 232 | `Q5_K_M` 44.34 GB at 64K (44.3 + 2.2 GB KV); `Q4_K_M` 40.69 GB keeps the full 131K | Apache-2.0 |
| 4 | **EXAONE 4.5 33B** | Only vision model in this tier with strong tool calling and six-language coverage; 3:1 sliding-window attention keeps KV small | AA 2026-11-06: tau2 78.1%, GPQA 79.4%, HLE 12.9%, AA-LCR 54.7%, IFBench 58.0%, TB-hard 20.5% | `Q8_0` 35.14 GB at 64K (35.1 + 5.1 GB KV); `Q4_K_M` 20.05 GB keeps the full 262K | EXAONE 1.2-NC (non-commercial) |
| 5 | **Kimi Linear 48B A3B Instruct** | The only way to get a real 1M-token window at 48 GB: linear attention, 7 full-attention layers, 3B active | AA 2026-11-06: GPQA 41.2%, HLE 2.5%, AA-LCR 28.0%, LCB 37.8%, AIME-2025 36.3% | `Q4_K_M` 30.06 GB gives the full 1M (30.1 + 16.9 GB KV); `Q6_K` 40.46 GB at 256K | MIT |

## Why each pick

### 1. Flint 40B A2.5B (Recursive, 2026-10-21)

The one model here built specifically for coding agents. 49 layers: 2 dense then
47 MoE, 256 routed experts top-6 plus a shared expert, 2.5B active. The
35 sliding-window layers (window 513) leave only 14 full-attention layers, so
the KV cache is 1.9 GB at 64K, 7.5 GB at 262K. That is what makes the full
native context affordable on 48 GB: `UD-Q6_K_XL` (36.32 GB) plus 7.5 GB is
43.8 GB.

The benchmark numbers are vendor-reported, not independent: Terminal-Bench 2.1
54.9%, SWE-bench Verified 74.2%, SWE-bench Pro v2 89.7% (model card,
2026-11-06). Artificial Analysis has no Flint row yet. Terminal-Bench 2.1 is the
one number that can be compared directly: K2 Horizon MoVA scores 56.6% on AA's
run, so the two are close on agentic terminal work, and Flint's SWE-bench claim
is the strongest in this tier. Treat the estimate (12.0) as unverified until AA
lists it.

### 2. K2 Horizon MoVA 36B A4B (IFM, 2026-09-01)

The best-evidenced new model at this size. It has an independent AA profile:
AA-Briefcase 1164.8 and GDPval-AA 1196.7 are close to `qwen3.8-27b`'s numbers,
and AutomationBench 26.1%, HLE-Diamond 23.4%, AA-LCR 71.7%, TB2.1 56.6%,
SciCode 40.0% and GPQA 82.2% are all well above the other sub-40B candidates.
The linear fit on 38 models with an official v5 index gives 14.1 (RMSE 2.9);
the legacy fit gives 12.7; the entry uses 13.0.

The catch is KV: all 48 layers are full attention (8 KV heads x 128), so 64K
f16 costs 12.9 GB. `Q6_K` (30.77 GB) plus that is 43.7 GB and fits. `Q8_0`
(39.83 GB) only fits with 32K context (46.3 GB). Context beyond 128K needs
`Q4_K_M` (22.37 GB). It is Apache-2.0 and fully open (data, recipe, code).

### 3. HyperNova 60B 2605 (Multiverse Computing, 2026-04-30)

A CompactifAI compression of `gpt-oss-120b` down to 58.7B total and 4.8B active
(80 experts top-4). Half the layers are sliding-window (128), so KV is 2.2 GB at
64K and 4.3 GB at the full 131K. `Q5_K_M` (44.34 GB) plus 2.2 GB is 46.5 GB;
`Q4_K_M` (40.69 GB) leaves room for the full context.

It is the strongest tool-calling model in the tier after the two above: IFBench
66.5%, tau2 63.2%, GPQA 73.3%. The weak spots are real: AA-Briefcase 232 and
GDP.pdf 0.1% are far below the 2026 leaders, which is why the estimate is 7.0
(v5 fit 5.6, legacy fit 8.5). It is the largest fully-resident general model
here and Apache-2.0.

### 4. EXAONE 4.5 33B (LG AI Research, 2026-04-04)

The vision pick. 33B dense plus a 1.2B vision encoder, 262K context, 3:1
sliding-window (4096) to full attention across 65 blocks. tau2 78.1% is the
best tool-calling number in this tier, GPQA 79.4% is solid, and the six
languages (en, ko, es, de, ja, vi) plus document understanding are not covered
by anything else that fits 48 GB. `Q8_0` (35.14 GB) plus 5.1 GB of KV is
40.3 GB; `Q4_K_M` (20.05 GB) plus 18.0 GB keeps the full 262K.

The blocker is the licence: EXAONE 1.2-NC is research and non-commercial only
(the LICENSE file says "solely for research and educational purposes"). For a
paid public beta that means either a commercial agreement or keeping it as an
optional download. Vision also needs the separate `mmproj` file, which the
catalog has no field for today.

### 5. Kimi Linear 48B A3B Instruct (Moonshot AI, 2025-10-30)

Included for one capability, not for its score. Kimi Delta Attention keeps only
7 full-attention layers out of 27, so the cache is 1.0 GB at 64K, 4.1 GB at
256K and 16.9 GB at 1M (MLA, one KV head). `Q4_K_M` (30.06 GB) plus 16.9 GB
fits 48 GB and gives a real 1M-token window; no other full-fit model here does
that.

The trade-off is quality: GPQA 41.2%, HLE 2.5%, AA-LCR 28.0%, LCB 37.8%,
AIME-2025 36.3% (AA, 2026-11-06). A 2025 model is dated against the 2026 field,
so rank it last and use it only for long-document work.

## What the catalog already covers at 48 GB

These are in `internal/catalog/catalog.json` and still beat the new picks on
score, so the tier is not empty without this file:

- `qwen3.8-27b` (20.1): Q8_0 29.0 GB, 16 KV layers, 262K context. The best
  general dense model that fits.
- `nanbeige4.3-33b` (20.4): Q8_0 35.2 GB, full attention on 52 layers, 524K
  context. Best score at this tier, but KV-heavy.
- `nemotron-3.5-super-vl-120b-a12b` (13.6): fits at IQ2 quants, vision.
- `nemotron-3.5-bolt-74b-a9b` (11.5): Q3_K_XL 39.0 GB, 8 attention layers.
- `qwen3.8-flash-next` (about 27): Q2_0 66.4 GB with 22.6 GB of lazy embedding
  bytes, so about 43.8 GB resident. The highest-scoring model that fits 48 GB,
  but it depends on the planner's lazy-load path.

The five new entries should be merged alongside these, not instead of them.

## Rejected, and why

- **LongCat-Flash-Lite 68.5B A3B** (MIT, tau2 79.5%): would have been a strong
  tool-calling pick at Q4_K_M 40.2 GB, but its GGUF architecture is
  `longcat-flash-ngram`, which is not in llama.cpp b11960's 166-architecture
  list. It cannot be deployed with the pinned build.
- **G9v3-39A5B** (Apache-2.0, GPQA 80.5%): same problem, architecture `g9v3`
  is not in b11960.
- **Qwen3-Coder-Next 79.7B A3B**: Q4_K_M is 48.5 GB, already over budget
  before KV, and at Q3_K_XL (36.3 GB) Flint beats it on coding evidence
  (TB2.1 38.2% vs 54.9%).
- **Olmo 3.1 32B Think**: fully open and good at math (AIME 77.3,
  LiveCodeBench 69.5) but no tool calling in the chat template and only 65K
  context, which rules it out for agent apps.
- **Step-3.5-Flash 199B A11B**: Q2 is about 60 GB, over 48 GB.
- **Sarvam 105B, Solar Open 100B**: fit at Q2 (about 40 GB) but score below the
  picks (GPQA 73.8% / 65.7%, weak agentic components).
- **Apertus 70B / Apertus v1.5 70B**: GPQA 27.2% is far below the field.
- **granite-switch-4.1-30b** (2026-10-14, Apache-2.0): the newest 30B dense
  model, but AA-Briefcase 395, AutomationBench 1.5% and TB4 0% put it around
  3, below the picks.
- **Cloudflare Clef 27B** (2026-09-30): a decision model that emits option
  probabilities, not free-form text, so it does not fit the catalog's chat use.
- **The 200B+ MoEs**: Kimi K3.1 Pro (2.8T), DeepSeek V4.1 Flash (763B),
  GLM-5.3 (753B) / GLM-5.3-Flash (321B), Mistral Large 4 (1T), Ling 3.1
  (572B), MiMo-V2.6 (310B) / Pro (1T), MiniMax-M3.1-Flash (220B), Muse Spark
  (816B), Beam 1.0 (501B), Motif-3 (315B), Hy3 (299B), Trinity Large (399B).
  Smallest available quants: GLM-5.3-Flash UD-IQ1_S 93 GB, Ling 3.1 IQ1_S
  131 GB, MiniMax-M3.1 IQ1_M 129 GB. They need 96 GB or expert offload.

## Open questions

- Flint has no independent evaluation yet. If AA adds a row, replace the 12.0
  estimate with the real index.
- The 48 GB tier is where `qwen3.8-flash-next`'s lazy-embedding path matters
  most. If that path is unreliable, the tier's top score drops from about 27 to
  20.4.
- EXAONE 1.2-NC blocks commercial use; decide whether EZLM ships NC models.
- LongCat-Flash-Lite and G9v3-39A5B become good 48 GB picks the moment
  llama.cpp adds their architectures.
