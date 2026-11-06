# 32 GB tier: models for November 2026

Tier definition: 32 GB of usable GPU or unified memory. The planner's budget is
about 30.7 GB on Linux and NVIDIA (4% margin), 29.4 GB on Windows, and 28.1 GB
on Windows with Vulkan. On top of the weights it needs llama.cpp compute buffers
(roughly 0.32 GB plus 4 KB per token of context plus 1% of the weights) and the
KV cache, so a 32 GB machine realistically runs 22 to 27 GB of weights at 64K to
256K context, depending on how many layers carry a KV cache.

All sizes below are exact bytes from the Hugging Face tree API
(`/api/models/<repo>/tree/main?recursive=true`, fetched 2026-11-06). Every entry
was checked against llama.cpp b11960 with
`python3 scripts/verify_catalog.py --catalog agents/T02/tiers/tier-32.json`:
12 of 12 quant file sets pass with no warnings. Layer counts, KV heads, head
size, native context and tensor types come from the GGUF headers over range
requests.

The catalog already has strong 32 GB models, so five of the seven picks are
catalog entries with a 32 GB quant subset. The other two are new models that are
not in the catalog and were confirmed on Hugging Face today. Rank order is the
recommendation for a 32 GB machine, not the raw score: context that fits
matters as much as the benchmark number.

## Ranked picks

| Rank | Model | Why it wins | Evidence | Best quant for 32 GB | Licence |
|---|---|---|---|---|---|
| 1 | Qwen3.8 27B | Best all-round dense model that still gets near its 256K context here; cheap KV, vision, strong agentic numbers | AA v5 index 20.1 (Xhigh); GPQA-D 90.5%, Terminal-Bench 2.1 79.8%, tau3-Banking 48.0%, HLE 33.9 (catalog, AA fetched 2026-11-06) | UD-Q6_K 21.98 GB: 205K Linux, 172K Windows, 139K Vulkan | Apache-2.0 |
| 2 | Nanbeige4.3 33B | Highest measured score at this tier; strongest dense model on AA v5 | AA v5 index 20.4 (AA leaderboard JSON, fetched 2026-11-02); AutomationBench 53%, TB4 30% | IQ4_XS 18.64 GB: 96K Linux, 85K Windows; Q4_K_M 20.79 GB: 78K / 67K | Apache-2.0 |
| 3 | Ternary Bonsai 2 27B | Best quality per byte: 27B at 7.63 GB, full 256K context, vision, leaves half the tier free | Vendor: 98.2% of Qwen3.8-27B FP16 on a 14-20 suite; coding 89.4 vs 89.1, agentic 74.9 vs 76.7, knowledge 79.9 vs 85.6 (catalog ESTIMATE 18.0) | Q2_0 7.63 GB: full 256K on every budget (18.2 GB total) | Apache-2.0 |
| 4 | Ornith 1.5 35B A3B (new) | Fast agentic MoE: 3B active, 10 of 40 layers carry KV, so 256K is affordable | Vendor 5-run averages: Terminal-Bench 2.1 67.8 vs 52.5 for Qwen3.6-35B-A3B; SWE-bench Verified 79.0; GPQA 89.2 ([card](https://huggingface.co/ornith-ai/Ornith-1.5-35B-A3B), 2026-08-18) | Q5_K_M 25.49 GB: 256K Linux, 217K Windows, 132K Vulkan; Q4_K_M 21.86 GB: 256K everywhere | MIT |
| 5 | K2 Horizon MoVA 36B A4B (new) | Strong general MoE with open training data and recipe; 4B active | AA v5 components: TB2.1 56.6%, GPQA 82.2%, HLE-D 23.4%, AA-LCR 71.7% (fitted v5-equivalent 13.0) ([card](https://huggingface.co/IFM/K2-Horizon-MoVA-36B-A4B), 2026-09-01) | Q4_K_M 22.37 GB: 70K Linux, 58K Windows, 46K Vulkan | Apache-2.0 |
| 6 | Nemotron 3.5 Bolt 74B A9B | Biggest model that fits: 74B total, 9B active, 256K context, tiny KV | AA legacy-fit v5-equivalent 11.5; NVIDIA recipe 28 GB for 32 GB GPU at 65K ([card](https://huggingface.co/nvidia/NVIDIA-Nemotron-3.5-Bolt-74B-A9B-BF16), 2026-10-09) | UD-IQ2_M 24.61 GB: full 256K Linux, 488K headroom Windows | OpenMDW-1.1 |
| 7 | Gemma 4 31B | Strong tool calling and document QA; 128K context at a mid quant | AA legacy-fit v5-equivalent 8.0; RTX 4090 Q4_K_M 45 tok/s (catalog) | UD-Q5_K_XL 21.89 GB: 160K Linux, 133K Windows; Q6_K 25.20 GB: 92K / 65K | Apache-2.0 |

## Reasoning per pick

### Qwen3.8 27B (score 20.1, AA v5)

The default pick for this tier. It is one point off the top score, it is dense,
and its hybrid attention (48 Gated DeltaNet layers plus 16 full-attention
layers) makes context cheap: at UD-Q6_K (21.98 GB) it still reaches about 205K
tokens on a Linux budget, 172K on Windows and 139K on Vulkan. At UD-Q5_K_XL
(20.88 GB) the numbers are 233K, 200K and 168K. The 4% margin, compute buffers
and q8 KV cache are all included in those figures.

That combination is why it ranks first despite Nanbeige's slightly higher index:
a 32 GB machine can actually use the context window. Vision is native, tool
calling is in the chat template, and the licence is Apache-2.0. The GSQ-RCO
quants in the catalog were measured at 0.94 to 0.985 of FP16 quality; this tier
uses the unsloth UD quants because they are larger and cheaper to run.

### Nanbeige4.3 33B (score 20.4, AA v5)

The highest measured score available at 32 GB: AA v5 index 20.4, level with
Qwen3.8-27B Xhigh on the index but ahead on AutomationBench (53% vs 48%) and
TB4 (30% vs 6%), and behind on HLE-D (7% vs 15%) and Omniscience. It is a
Llama-architecture dense 33B with full attention on all 52 layers, so KV is
about six times heavier per token than Qwen3.8-27B. That is the whole trade:
IQ4_XS (18.64 GB) reaches about 96K context on Linux and 85K on Windows, while
the quality option Q4_K_M (20.79 GB) reaches 78K and 67K. 128K would need
14.0 GB of q8 KV on top of the weights, which does not fit.

Two caveats. Tool calling is unverified on the catalog entry, and the model is
brand new (2026-10-24), so its AA number should be confirmed with an independent
evaluation before it becomes a default.

### Ternary Bonsai 2 27B (score 18.0, ESTIMATE)

A ternary (-1, 0, +1, group 128) QAT compression of Qwen3.8-27B. At 7.63 GB it
is the best quality-per-byte model at this tier: the vendor reports 98.2% of the
FP16 base on a 14-20 benchmark suite, with coding 89.4 vs 89.1 and agentic 74.9
vs 76.7, and a 5.7-point knowledge loss. The full 256K native context fits on
every 32 GB budget at 18.2 GB total, and there is room left for a second
workload or a large batch.

The score is a hand-mapped estimate, not an AA measurement, so it ranks below
the two measured models. The practical case for it is a 32 GB machine that wants
long context, vision (separate mmproj file) and headroom at the same time. Only
one quant exists; there is no fallback if PQ2_0 turns out to be better on a
given machine.

### Ornith 1.5 35B A3B (score 13.0, ESTIMATE, new)

A 35B-total, 3B-active MoE on a Qwen3.5-MoE backbone with linear attention in 30
of 40 layers, so only 10 layers grow a KV cache. That is what makes it a 32 GB
pick rather than a 24 GB one: Q5_K_M (25.49 GB) still runs the full 256K
context on a Linux budget (30.0 GB total), 217K on Windows and 132K on Vulkan,
while Q4_K_M (21.86 GB) runs 256K on every budget.

The evidence is vendor-run but unusually complete: five-run averages, named
harness (Terminus-2 and Claude Code), its own Qwen3.6-35B-A3B baseline at 52.5
on Terminal-Bench 2.1, SWE-bench Verified 79.0, GPQA 89.2. Scaling through AA's
measured points for Qwen3.6-35B-A3B (44.9, catalog 9.5) and Qwen3.8-27B (79.8,
20.1) puts it at 12 to 14, hence 13.0. It is not on AA as of 2026-11-06.

Risks: the card asks for a modified Qwen chat template for benchmark parity, and
vision needs a separate mmproj file. MIT licence is the most permissive here.

### K2 Horizon MoVA 36B A4B (score 13.0, ESTIMATE, new)

Released 2026-09-01, fully open (training data, recipe and code). 37.4B counted,
4B active, Mixture-of-Values attention, 524K native context. The reason it is
fifth rather than higher is KV cost: all 48 layers are full attention, so 64K of
f16 KV is 12.9 GB. Q4_K_M (22.37 GB) reaches about 70K context on Linux, 58K on
Windows and 46K on Vulkan. Q5_K_M (26.44 GB) only reaches about 32K on Linux
and does not fit Windows with usable context, so it is not listed.

The score is fitted from AA's published v5 components (TB2.1 56.6%, GPQA 82.2%,
HLE-D 23.4%, AA-LCR 71.7%, AutomationBench 26.1%) through a linear fit with
RMSE 2.9, and cross-checked against a legacy fit at 12.7. The architecture
`k2-horizon` is known to the pinned llama.cpp b11960 (verify passes), which is
the hard requirement for shipping it.

### Nemotron 3.5 Bolt 74B A9B (score 11.5, ESTIMATE)

The largest model that fits 32 GB. It is a Star-Elastic compression of Super VL
(heterogeneous expert counts, top-k 22 to 9) with the same 8 attention layers,
so 74B total and 9B active with almost no KV cost: UD-IQ2_M (24.61 GB) reaches
the full 256K context at 27.4 GB total on Linux, with 488K of headroom on
Windows and 338K on Vulkan. UD-Q2_K_XL (28.75 GB) is a Linux-only option at
about 150K, useful when quality matters more than context.

NVIDIA's own recipe is 28 GB for a 32 GB GPU at 65K context, which agrees with
this. The score is a legacy-component fit (13.1) cross-checked against partial
v5 components (11.0), so 11.5. The 24 GB quant from NVIDIA also exists
(22.02 GB) but is dominated by UD-IQ2_M here.

### Gemma 4 31B (score 8.0, ESTIMATE)

Kept for tool calling and document QA, where it is stronger than its index
suggests. 50 sliding-window layers (16 KV heads x 256, window 1024) plus 10
global layers (4 KV heads x 512). The global layers make KV expensive: at
UD-Q5_K_XL (21.89 GB) it reaches about 160K context on Linux and 133K on
Windows, and 128K costs about 29.2 GB including the 0.45 GB sliding-window
cache, at the edge of the Windows budget. Q6_K (25.20 GB) reaches about 92K on
Linux, 65K on Windows and 38K on Vulkan.

It ranks last because it is a March 2026 model with a lower index and heavier KV
than the Qwen hybrids. The 0.51 GB MTP draft in the catalog entry is optional
and was left in place; it does not change the fit.

## Considered and rejected

| Model | Size and fit | Why it was not picked |
|---|---|---|
| Devstral Small 2 24B | Q8_0 25.06 GB: 32K Windows, 64K Linux; UD-Q5_K_XL 16.77 GB: 128K Linux | Coding specialist with a catalog score of 4.0. Qwen3.8 27B and Ornith beat it on agentic coding and cover general use too. Worth a look if the tier ever needs a second coding entry. |
| Qwen3.6 27B | UD-Q4_K_XL 17.59 GB | Same family as Qwen3.8 27B, lower score (12.0), no advantage at 32 GB. |
| Qwen3.6 35B A3B | UD-Q5_K_XL 26.64 GB, MXFP4 21.72 GB | Catalog score 9.5; outclassed by Ornith and K2 at the same size, and its KV is heavier than Ornith's. |
| Qwen3.5 122B A10B | Smallest quant 40.3 GB | Too large for 32 GB at any quant. |
| Mistral Small 4 119B | Smallest quant 32.3 GB | 32.3 GB leaves no room for KV or compute inside a 30.7 GB budget. |
| Nemotron 3.5 Super VL 120B A12B | Smallest quant 46.1 GB | Too large; the 74B Bolt is the 32 GB representative of that family. |
| gpt-oss-120b | MXFP4 63.4 GB | Too large. |
| Qwen3.8 Flash Next 180B A6B | Smallest quant 66.4 GB | Top score in the catalog (27.0) but 2x too large at this tier. |
| Muse Glimmer 30B | Q8_0 29.6 GB, UD-Q6_K_XL 26.3 GB | Score 6.3 and no capability that the picks do not cover; Q8_0 leaves no context on Windows. |
| Gemma 4 26B A4B | UD-Q4_K_XL 17.0 GB | Score 5.5; the 31B is better and fits. |
| Granite 4.2 30B | Q8_0 31.1 GB, Q6_K 24.0 GB | Score 5.2, 64 layers of full attention; dominated by the picks. |
| Nanbeige Q6_K_S / Q8_0 | 27.75 GB / 35.20 GB | Q6_K_S leaves 3 GB for KV, about 20K context; Q8_0 does not fit at all. |
| Nemotron Bolt UD-Q3_XXS | 31.56 GB | Over the Linux budget before any KV. |
| Ornith Q6_K | 30.53 GB | Leaves 0.2 GB on Linux and nothing on Windows. |

## Sources

- Artificial Analysis leaderboards, accessed 2026-11-06: https://artificialanalysis.ai/models (v5 index for Qwen3.8-27B and Nanbeige4.3-33B; v5 components for K2 Horizon; legacy-component fits for Nemotron Bolt and Gemma 4).
- Hugging Face tree API for exact sizes: `https://huggingface.co/api/models/<repo>/tree/main?recursive=true`, fetched 2026-11-06.
- GGUF headers over HTTP range requests, checked against llama.cpp b11960 by `scripts/verify_catalog.py`, 2026-11-06.
- Model cards and launch notes: Qwen3.8-27B, Nanbeige4.3-33B, Ternary Bonsai 2 27B, Ornith 1.5 35B A3B (2026-08-18), K2 Horizon MoVA 36B A4B (2026-09-01), Nemotron 3.5 Bolt 74B A9B (2026-10-09), Gemma 4 31B (2026-03-11).
- `internal/catalog/catalog.json` (version 8) for the verified catalog numbers, scores and notes.

## Open questions

- Nanbeige4.3 33B has no verified tool-calling result; its chat template needs a T03-style check before it is offered as an agent model.
- Ternary Bonsai 2's score is a hand-mapped estimate from vendor retention numbers, not an independent evaluation. If AA adds it, the score should be replaced.
- K2 Horizon and Ornith have no AA index. Both scores are estimates (13.0) and should be replaced when independent data appears.
- K2 Horizon's context at this tier (58K on Windows) is well below its 524K native window; a sliding-window or KV-quant option on that architecture would change the ranking.
- Gemma 4's 8.0 is a legacy-component fit; its tool-calling quality is the reason to keep it, and that is not captured by the index.
- The tier file reuses five catalog ids with smaller quant sets. If the assembler expects only new ids, these five should be dropped or renamed; the two new models stand on their own.
