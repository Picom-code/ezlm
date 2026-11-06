# T02 retirement study: catalog models outclassed as of 2026-11-06

Agent a12. Scope: the 32 models in `internal/catalog/catalog.json` at the
2026-11-06 source snapshot. I did not edit the catalog.

The question is narrower than "what is new". A model is *outclassed* here only
when a model that runs on EZLM today (a real GGUF, an architecture in llama.cpp
v0.9.0, a footprint in the same memory tier) beats it on quality, and the
replacement does not give up a capability the catalog was using it for. Models
that are still the best fit for a niche (EU origin, audio input, 1-bit
compression, 128GB fit) are marked keep even when a larger or foreign model
scores higher.

Only two open-weight chat models were released in November 2026 before this
snapshot: Kimi K3.1 Pro (2026-11-02) and Ripple-65B Preview (2026-11-04).
Neither is catalog-ready on 2026-11-06 (no usable GGUF for Kimi, no
`ricursive` architecture in llama.cpp v0.9.0 for Ripple). The retirement
cases below are therefore driven by September and October 2026 releases.
That is what "outclassed by November 2026" means in practice: the models a
user can actually install in November, not the ones announced that month.

Scores are the catalog's `score` field, which is Artificial Analysis
Intelligence Index v5.0 where AA publishes one and a hand-mapped estimate
otherwise. The score source is called out per row. Full-precision benchmark
scores and low-bit quant quality are not the same thing; the footprint column
is the smallest quant in the catalog, so a replacement that only fits at IQ1
or IQ2 gives up some of the headline benchmark margin. That caveat applies to
every "replace" row below.

## Summary table

Footprint is the smallest catalog quant for the current model, and the smallest
published GGUF quant for a new replacement. "AA" means an official Artificial
Analysis v5.0 index; "est." means the catalog's own estimate; "vendor" means a
model-card comparison run by the model's author.

| # | Current id | Score | Replacement | Repl. score | Footprint | Evidence | Action |
|---|---|---|---|---|---|---|---|
| 1 | ternary-bonsai-2-27b | 18.0 est. | — | — | 7.63GB | 18.0 at 7.6GB is still the best ternary 27B; no successor in the Bonsai 2 line | keep |
| 2 | ternary-bonsai-2-9b | 4.5 est. | — | — | 2.56GB | Still the only ternary 9B; no newer PrismML release | keep |
| 3 | ternary-bonsai-2-4b | 2.0 est. | — | — | 1.21GB | Best quality at 1.2GB (2.0 vs 1.0 for the 0.55GB 2B); no successor | keep |
| 4 | ternary-bonsai-2-2b | 1.0 est. | — | — | 0.55GB | Only sub-0.6GB chat model in the catalog | keep |
| 5 | bonsai-8b-1bit | 2.0 est. | ternary-bonsai-2-4b | 2.0 est. | 1.16GB → 1.21GB | [PrismML cards](https://huggingface.co/prism-ml/Ternary-Bonsai-2-4B-gguf), 2026-09-23; old card 2026-03-18 | retire |
| 6 | qwen3.8-27b | 20.1 AA | — | — | 7.27GB | [AA 20.1](https://artificialanalysis.ai/models/qwen3-8-27b), 2026-11-06 | keep |
| 7 | qwen3.8-flash-next | 27.0 est. | — | — | 66.42GB | Best model that fits 128GB; 256GB machines can use DeepSeek V4.1 Flash 1017 (below) | keep |
| 8 | qwen3.6-35b-a3b | 9.5 est. | flint | vendor: SWE 74.2, TB2.1 54.9 | 10.76GB → 11.86GB | [flint card](https://huggingface.co/RecursiveSI/flint), 2026-10-21 | replace |
| 9 | qwen3.6-27b | 12.0 est. | qwen3.8-27b | 20.1 AA | 9.57GB → 7.27GB | [AA 20.1](https://artificialanalysis.ai/models/qwen3-8-27b), 2026-11-06 | replace |
| 10 | qwen3.5-122b-a10b | 7.5 est. | nemotron-3.5-super-vl-120b-a12b | 13.6 AA | 40.28GB → 46.14GB | [AA 13.6](https://artificialanalysis.ai/models/nemotron-3-5-super-vl), 2026-11-06 | replace |
| 11 | qwen3.5-9b | 5.0 est. | MiMo-V2.6-Distill-Qwen-9B | vendor: SWE 61.1, SWE Pro 44.6 | 3.19GB → 3.54GB | [MiMo card](https://huggingface.co/XiaomiMiMo/MiMo-V2.6-Distill-Qwen-9B), 2026-09-21 | replace |
| 12 | qwen3.5-4b | 2.0 est. | — | — | 1.52GB | Still the small multimodal Qwen; K2-Horizon-3.7B is text-only | keep |
| 13 | qwen3.5-2b | 1.0 est. | — | — | 0.77GB | Small multimodal; no same-size vision replacement | keep |
| 14 | gemma-4-31b | 8.0 est. | qwen3.8-27b | 20.1 AA | 8.53GB → 7.27GB | [AA qwen3.8 20.1](https://artificialanalysis.ai/models/qwen3-8-27b); AA lists Gemma 4 31B at 4.4 | replace |
| 15 | gemma-4-26b-a4b | 5.5 est. | qwen3.8-27b | 20.1 AA | 9.92GB → 7.27GB | [AA qwen3.8 20.1](https://artificialanalysis.ai/models/qwen3-8-27b), 2026-11-06 | replace |
| 16 | gemma-4-12b | 3.5 est. | — | — | 4.21GB | Only any-to-any 12B (image/audio/text); keep for that niche | keep |
| 17 | gemma-4-e4b | 2.0 est. | — | — | 3.55GB | Audio+image at the edge; no replacement with audio | keep |
| 18 | gemma-4-e2b | 1.5 est. | — | — | 2.29GB | Audio+image at the edge; no replacement with audio | keep |
| 19 | gpt-oss-120b | 7.0 est. | nemotron-3.5-super-vl-120b-a12b | 13.6 AA | 63.02GB → 46.14GB | [AA 13.6](https://artificialanalysis.ai/models/nemotron-3-5-super-vl), 2026-11-06 | replace |
| 20 | gpt-oss-20b | 5.0 est. | qwen3.8-27b | 20.1 AA | 11.87GB → 7.27GB | [AA 20.1](https://artificialanalysis.ai/models/qwen3-8-27b), 2026-11-06 | replace |
| 21 | muse-glimmer-30b | 6.3 AA | — | — | 10.75GB | Only US 30B in the 12GB tier; qwen3.8-27b scores higher but is CN | keep |
| 22 | mistral-small-4-119b | 4.3 AA | — | — | 32.31GB | Only EU Apache 120B-class option; keep for origin, not score | keep |
| 23 | devstral-small-2-24b | 4.0 est. | flint | vendor: SWE 74.2, TB2.1 54.9 | 9.41GB → 11.86GB | [flint card](https://huggingface.co/RecursiveSI/flint), 2026-10-21 | replace |
| 24 | nemotron-3.5-super-vl-120b-a12b | 13.6 AA | — | — | 46.14GB | Newest US 120B-class vision model; still the 48GB pick | keep |
| 25 | nemotron-3.5-bolt-74b-a9b | 11.5 est. | — | — | 20.31GB | Best US model at 24-32GB; keep | keep |
| 26 | nemotron-3.5-lightning-30b-a3b | 5.5 AA | nemotron-3.5-bolt-74b-a9b | 11.5 est. | 18.90GB → 20.31GB | Same NVIDIA family; [Bolt card](https://huggingface.co/nvidia/NVIDIA-Nemotron-3.5-Bolt-74B-A9B-BF16), 2026-10-09 | replace |
| 27 | granite-4.2-30b | 5.2 est. | — | — | 10.86GB | IBM enterprise/US option; outscored by qwen3.8-27b but kept for vendor support | keep |
| 28 | granite-4.2-8b | 5.1 AA | K2-Horizon-7B | vendor: SWE 72.4, HLE 19.5 | 4.35GB → 5.59GB | [K2-7B card](https://huggingface.co/IFM/K2-Horizon-7B), 2026-09-01 | replace |
| 29 | lfm3-1.7b | 1.5 est. | — | — | 1.05GB | Newest (2026-10-23) CPU/phone model; no successor | keep |
| 30 | nanbeige4.3-33b | 20.4 AA | — | — | 10.07GB | Highest dense score in the catalog; keep | keep |
| 31 | bonsai-27b-1bit | 9.5 est. | — | — | 3.80GB | Only 27B-class model under 4GB; keep | keep |
| 32 | granite-4.2-3b | 4.7 AA | K2-Horizon-3.7B | vendor: SWE 65.6, HLE 14.0 | 1.46GB → 3.16GB | [K2-3.7B card](https://huggingface.co/IFM/K2-Horizon-3.7B), 2026-09-01 | replace |

Result: 12 replace, 1 retire, 19 keep.

## The replacement cases in detail

### qwen3.6-27b -> qwen3.8-27b

Same 27.8B dense class and the same Apache-2.0 licence. Qwen3.8-27B is the
August 2026 refresh: AA v5.0 gives it 20.1 at Xhigh effort, versus the
catalog's 12.0 estimate for Qwen3.6-27B, and the smallest quant is smaller
(7.27GB UD-IQ2_XXS versus 9.57GB). The catalog note already says
"superseded ... keep only as fallback", so this is the least controversial
retirement in the set.

Evidence: [AA Qwen3.8 27B](https://artificialanalysis.ai/models/qwen3-8-27b)
(accessed 2026-11-06, page says "Released August 2026").

### qwen3.6-35b-a3b and devstral-small-2-24b -> flint

Flint (Recursive, 2026-10-21) is a 39.8B MoE with about 2.5B active per
token, Apache-2.0, 262K context, multi-turn tool use, and a llama.cpp
architecture (`flint`) that is present in v0.9.0. Its card reports 74.2 on
SWE-bench Verified, 89.7 on SWE-bench Pro v2 and 54.9 on Terminal-Bench 2.1.
The GGUF from unsloth starts at 11.86GB (UD-IQ2_XXS) and 25.05GB
(UD-Q4_K_XL), so it lands in the same 16-32GB tier as Qwen3.6-35B-A3B
(10.76GB) and Devstral Small 2 (9.41GB) while being a much stronger coding
agent. The 2.5B active count also preserves the speed reason the catalog kept
Qwen3.6-35B-A3B for.

These are vendor-reported numbers, not AA. The comparison is still useful
because both old models are weak on the same kind of agentic benchmarks: the
catalog note for Qwen3.6-35B-A3B says "SWE-Pro well below" Qwen3.8-27B, and
Devstral is a December 2025 model.

Evidence: [flint card](https://huggingface.co/RecursiveSI/flint) and
[unsloth GGUF](https://huggingface.co/unsloth/flint-GGUF) (accessed
2026-11-06). llama.cpp support checked in
`src/llama-arch.cpp` at tag v0.9.0.

### qwen3.5-122b-a10b and gpt-oss-120b -> nemotron-3.5-super-vl-120b-a12b

Both current models are in the 48-64GB tier and both are now clearly behind
NVIDIA's October 2026 hybrid. Nemotron 3.5 Super VL 120B A12B scores 13.6 on
AA v5.0, runs from 46.14GB (UD-IQ1_M), has vision and 1M context, and is
US-origin under OpenMDW-1.1. Qwen3.5-122B-A10B is a February 2026 model at
7.5 (estimate, 40.28GB); gpt-oss-120b is from August 2025 at 7.0 (estimate,
63.02GB). The replacement is smaller than gpt-oss-120b, about the same size
as the Qwen, and faster on the memory plan because it is a Mamba2/MoE hybrid.

Evidence: [AA Nemotron 3.5 Super VL](https://artificialanalysis.ai/models/nemotron-3-5-super-vl)
(13.6, accessed 2026-11-06).

### qwen3.5-9b -> MiMo-V2.6-Distill-Qwen-9B

The MiMo model is a supervised fine-tune of the exact same Qwen3.5-9B base,
MIT-licensed, and it keeps the multimodal projection (the GGUF repo ships an
mmproj). On the vendor's own table it wins every agentic row: SWE-bench
Verified 61.1 vs 60.0, SWE-bench Pro 44.6 vs 32.0, AutomationBench 30.3 vs
5.0, Terminal-Bench 2.1 37.1 vs 27.0, Toolathlon 35.2 vs 25.9. Quant sizes
are the same class (bartowski IQ2_M 3.54GB, Q4_K_M 5.84GB), so this is a
drop-in tier replacement that does not lose vision.

Evidence: [MiMo-V2.6-Distill-Qwen-9B](https://huggingface.co/XiaomiMiMo/MiMo-V2.6-Distill-Qwen-9B)
(2026-09-21) and [bartowski GGUFs](https://huggingface.co/bartowski/MiMo-V2.6-Distill-Qwen-9B-GGUF)
(accessed 2026-11-06).

### gemma-4-31b and gemma-4-26b-a4b -> qwen3.8-27b

Both Gemma 4 mid-size entries are vision models with tool calling, and both
are outscored by Qwen3.8-27B at a smaller footprint. Qwen3.8-27B is 20.1 on
AA v5.0 at 7.27GB minimum. Gemma 4 31B is 8.0 in the catalog (AA's own v5
entry for the reasoning variant is 4.4) at 8.53GB, and Gemma 4 26B A4B is
5.5 at 9.92GB. The catalog keeps Gemma 4 12B, E4B and E2B for the
any-to-any and audio niches, so retiring the two mid-size Gemmas does not
remove Google from the catalog.

Evidence: [AA Qwen3.8 27B](https://artificialanalysis.ai/models/qwen3-8-27b)
and [AA Gemma 4 31B](https://artificialanalysis.ai/models/gemma-4-31b)
(accessed 2026-11-06).

### gpt-oss-20b -> qwen3.8-27b

August 2025 model at a 5.0 estimate and an 11.87GB minimum. Qwen3.8-27B is
20.1 at 7.27GB with vision. The only reason to keep gpt-oss-20b was US
origin and MXFP4 speed; the speed case is weaker now that Nemotron 3.5 Bolt
74B A9B (US, 11.5, 20.31GB) exists, so the note in the table offers Bolt as
the US alternative.

Evidence: [AA Qwen3.8 27B](https://artificialanalysis.ai/models/qwen3-8-27b)
(accessed 2026-11-06); [gpt-oss-20b card](https://huggingface.co/openai/gpt-oss-20b)
(2025-08-04).

### nemotron-3.5-lightning-30b-a3b -> nemotron-3.5-bolt-74b-a9b

Same NVIDIA OpenMDW-1.1 family. Bolt is the newer Star-Elastic compression of
Super VL and scores 11.5 versus 5.5 for Lightning, at 20.31GB versus 18.90GB
minimum. Lightning's low-bit quants barely shrink because the Mamba and shared
tensors stay high precision, so the two models are effectively the same size on
disk. Bolt wins the 24-32GB slot.

Evidence: [Bolt card](https://huggingface.co/nvidia/NVIDIA-Nemotron-3.5-Bolt-74B-A9B-BF16)
(2026-10-09); catalog scores and quant sizes.

### granite-4.2-8b -> K2-Horizon-7B

K2-Horizon-7B (IFM, 2026-09-01) is Apache-2.0, 512K context,
text-only, with tool calling. The vendor's same-harness table puts it at
SWE-bench Verified 72.4 vs 47.7 for Granite 4.2 8B, HLE 19.5 vs 9.7, and
Terminal-Bench 2.1 44.6 vs 39.7 for Qwen3.5-9B. It ships Q4_K_M at 5.59GB
and Q5_K_M at 6.47GB, so it fits the same 8-12GB tier as Granite 4.2 8B
(4.35GB Q3_K_M). Granite 4.2 30B is kept for the IBM enterprise niche; the
8B is not. IFM is MBZUAI-led (Abu Dhabi, Silicon Valley, Paris), so this
trades IBM/US enterprise origin for quality; keep Granite if that origin
matters more than the benchmark gap.

Evidence: [K2-Horizon-7B](https://huggingface.co/IFM/K2-Horizon-7B) and
[IFM GGUF](https://huggingface.co/IFM/K2-Horizon-7B-GGUF) (accessed
2026-11-06).

### granite-4.2-3b -> K2-Horizon-3.7B

Same argument at the small end. K2-Horizon-3.7B is Apache-2.0,
512K context, and the vendor table has SWE-bench Verified 65.6 vs 32.2 for
Granite 4.2 3B, HLE 14.0 vs 6.6, GPQA Diamond 68.9 vs 55.9, Terminal-Bench
2.1 39.7 vs 13.9. Q4_K_M is 3.16GB, versus 1.46GB for Granite 4.2 3B Q2_K;
the K2 model is bigger, but it is still a 4-6GB tier model and the quality gap
is large. Qwen3.5-4B stays as the multimodal option at 1.52GB.

Evidence: [K2-Horizon-3.7B](https://huggingface.co/IFM/K2-Horizon-3.7B) and
[IFM GGUF](https://huggingface.co/IFM/K2-Horizon-3.7B-GGUF) (accessed
2026-11-06).

### bonsai-8b-1bit -> ternary-bonsai-2-4b

The March 2026 1-bit 8B is superseded by the September 2026 Ternary Bonsai 2
line at the same size: 1.21GB versus 1.16GB, same 2.0 estimate, and the
ternary model adds vision. The old 8B was kept for raw speed (368 tok/s on an
RTX 4090); the 4.7B ternary model is smaller and should be at least as fast,
but I did not benchmark it, so this is the weakest retirement case in the set.

Evidence: [Bonsai-8B card](https://huggingface.co/prism-ml/Bonsai-8B-gguf)
(2026-03-18) and [Ternary Bonsai 2 4B](https://huggingface.co/prism-ml/Ternary-Bonsai-2-4B-gguf)
(2026-09-23).

## Keeps that are outclassed on score

These stay because the replacement would lose something the catalog needs.

- **muse-glimmer-30b** (6.3) is far below qwen3.8-27b (20.1) but is the only
  US-origin 30B-class model at the 12GB tier. Nemotron Bolt is US but needs
  20.31GB.
- **mistral-small-4-119b** (4.3) is the only EU-origin Apache 120B-class
  model. Its score is weak, but replacing it with a US or Chinese model
  removes the EU option.
- **granite-4.2-30b** (5.2) is kept for the IBM enterprise/US niche. The 8B
  and 3B are replaced because K2-Horizon matches their licence (Apache-2.0),
  though not their IBM/US enterprise origin.
- **qwen3.5-4b** and **qwen3.5-2b** are natively multimodal. K2-Horizon and
  flint are text-only, so replacing them would remove vision at the 2-4GB
  tier.
- **gemma-4-12b** is the only any-to-any (image/audio/text) model in the
  catalog. **gemma-4-e4b** and **gemma-4-e2b** are the only audio-input
  models at the edge.
- **qwen3.8-flash-next** (27.0) is the best model that fits 128GB. The
  higher-scoring October/November models need 256GB or more; see the watch
  list.
- **bonsai-27b-1bit** (9.5) is the only 27B-class model under 4GB.
- **ternary-bonsai-2-\*** are the only ternary/QAT compression line and have
  no successor.
- **nanbeige4.3-33b** (20.4) is the highest-scoring dense model in the
  catalog.
- **nemotron-3.5-super-vl** and **nemotron-3.5-bolt** are the newest US
  models in their tiers.
- **lfm3-1.7b** is the newest CPU/phone model (2026-10-23).

## Watch list: outclassed but not catalog-ready

These are the October and November 2026 headline models. They are not in the retirement
table because they do not fit a current EZLM tier, do not run on llama.cpp
v0.9.0, or do not beat the current pick by enough to justify the download.

| Model | Released | Score | Why not yet |
|---|---|---|---|
| Kimi K3.1 Pro | 2026-11-02 | 46.5 AA | 2800B total / 104B active, 1.56TB FP8; no GGUF on HF as of 2026-11-06; does not fit 128GB even at 1-bit. Quality ceiling, not a local pick. |
| DeepSeek V4.1 Flash 1017 | 2026-10-17 | 32.6 AA | 552B / 16B active; smallest GGUF is 233.0GB (UD-IQ1_S). Fits 256GB machines, not 128GB. Would replace qwen3.8-flash-next on a 256GB+ tier. |
| GLM-5.3-Flash | 2026-08-26 | 27.8 AA | 93.1GB at UD-IQ1_S, 328GB FP8 (641.6GB BF16 GGUF). Only +0.8 over qwen3.8-flash-next and needs IQ1, so the quality-per-byte case is weak. |
| MiniMax-M3.1-Flash | 2026-10-19 | 29.1 AA | 428B / 23B active; 129.4GB at UD-IQ1_M. Needs 256GB. |
| Ripple-65B Preview | 2026-11-04 | vendor: TB2.1 80.5, SWE 76.5 | 65B dense, Apache-2.0, Q3_K_L 33.56GB / Q4_K_M 38.80GB. llama.cpp `ricursive` architecture is not in v0.9.0 (checked `src/llama-arch.cpp`), so EZLM cannot deploy it yet. Re-check after the next llama.cpp upgrade. |
| SenseNova-Loop-9B-Preview | 2026-10-30 | not on AA | 9B looped multimodal; llama.cpp `sensenova_loop` is not in v0.9.0. |
| Clover-35B-A3B-Preview | 2026-10-24 | not on AA | 35B-A3B diffusion LM; llama.cpp `clover_moe` is supported, but block-diffusion decoding is not a drop-in chat path. |
| Granite Switch 4.1 (8B/30B) | 2026-10-14 | not on AA | Modular base+adapter composition on the April 2026 Granite 4.1 base, not a quality upgrade over Granite 4.2. |
| AesCode-32B/8B | 2026-09-29 | not on AA | HTML/CSS visual-artifact specialist, not a general chat replacement. |

## What I could not verify

- **Independent scores for the new small models.** K2-Horizon-7B,
  K2-Horizon-3.7B, flint and MiMo-V2.6-Distill-Qwen-9B are not on the AA
  v5.0 leaderboard as of 2026-11-06. Their numbers are vendor-reported, run
  on the vendor's own harness. The comparisons are still same-harness within
  each card, but they are not independent.
- **Quant degradation.** All the headline benchmark numbers are full
  precision. The smallest GGUF quants (IQ1/IQ2) lose several points. The
  footprint comparison is correct; the score comparison is optimistic for
  both sides.
- **K2-Horizon-7B vision.** The card is text-only; I did not find a vision
  projector. It is a text replacement for Granite 4.2 8B, not for Qwen3.5-9B
  (which is replaced by the multimodal MiMo distill instead).
- **Ripple-65B llama.cpp support.** The GGUF exists from a community
  converter, but the `ricursive` architecture is absent from v0.9.0. It may
  be a one-line addition in a later build; I did not test it.
- **Bonsai 8B speed claim.** I did not benchmark ternary-bonsai-2-4b against
  the old 1-bit 8B. The retirement is based on release date, size and vision,
  not measured throughput.
- **AA scores for catalog estimates.** Several current models (qwen3.6-27b,
  qwen3.6-35b-a3b, qwen3.5-122b-a10b, qwen3.5-9b, gemma-4-\*, gpt-oss-\*,
  devstral, nemotron bolt) have no official v5.0 index; the catalog score is
  a hand-mapped estimate. The direction of every replacement is clear, but
  the exact delta is not.

## Sources (all accessed 2026-11-06)

- Artificial Analysis Intelligence Index v5.0 leaderboard and model pages:
  https://artificialanalysis.ai/leaderboards/models and
  https://artificialanalysis.ai/models/<slug> (for example
  https://artificialanalysis.ai/models/qwen3-8-27b,
  https://artificialanalysis.ai/models/nemotron-3-5-super-vl,
  https://artificialanalysis.ai/models/gemma-4-31b).
- Artificial Analysis release dates: https://artificialanalysis.ai/models/releases
  (releases array embedded in the page; parsed locally).
- flint: https://huggingface.co/RecursiveSI/flint (2026-10-21);
  GGUFs: https://huggingface.co/unsloth/flint-GGUF.
- K2-Horizon-7B: https://huggingface.co/IFM/K2-Horizon-7B (2026-09-01);
  GGUFs: https://huggingface.co/IFM/K2-Horizon-7B-GGUF.
- K2-Horizon-3.7B: https://huggingface.co/IFM/K2-Horizon-3.7B (2026-09-01);
  GGUFs: https://huggingface.co/IFM/K2-Horizon-3.7B-GGUF.
- MiMo-V2.6-Distill-Qwen-9B:
  https://huggingface.co/XiaomiMiMo/MiMo-V2.6-Distill-Qwen-9B (2026-09-21);
  GGUFs: https://huggingface.co/bartowski/MiMo-V2.6-Distill-Qwen-9B-GGUF.
- Ripple-65B Preview: https://huggingface.co/ricursiveAI/Ripple-65B-Preview
  (2026-11-04); GGUFs:
  https://huggingface.co/prithivMLmods/Ripple-65B-Preview-GGUF.
- DeepSeek V4.1 Flash 1017:
  https://huggingface.co/deepseek-ai/DeepSeek-V4.1-Flash-1017 (2026-10-17);
  GGUFs: https://huggingface.co/unsloth/DeepSeek-V4.1-Flash-1017-GGUF.
- Kimi K3.1 Pro: https://huggingface.co/moonshotai/Kimi-K3.1-Pro (2026-11-02).
- GLM-5.3-Flash: https://huggingface.co/zai-org/GLM-5.3-Flash (2026-08-25);
  GGUFs: https://huggingface.co/unsloth/GLM-5.3-Flash-GGUF.
- MiniMax-M3.1-Flash:
  https://huggingface.co/MiniMaxAI/MiniMax-M3.1-Flash (2026-10-17);
  GGUFs: https://huggingface.co/unsloth/MiniMax-M3.1-Flash-GGUF.
- llama.cpp architecture list: `src/llama-arch.cpp` at tag v0.9.0,
  https://raw.githubusercontent.com/ggml-org/llama.cpp/v0.9.0/src/llama-arch.cpp.
- Catalog scores, quants and repos: `internal/catalog/catalog.json`.
