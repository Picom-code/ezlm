# Tier 4 GB: new model picks for November 2026

Scope: machines with 4 GB of usable GPU memory or 4 GB of unified memory. All
three picks are dense models that fit with a usable context once EZLM applies
its safety margins. Sizes below are decimal GB from the Hugging Face tree API
(1 GB = 1,000,000,000 bytes); budget arithmetic uses GiB (1 GiB = 2^30 bytes),
which is what `internal/fit` works in.

Budget used for the fits: on a 4 GB card EZLM plans against **3.6 GiB** on
Linux and macOS (4% or 0.4 GiB margin), **3.0 GiB** on Windows (8% or 1 GiB),
and a 4 GB unified machine gets **3.25 GiB**. Resident memory is weights + KV
cache + llama.cpp overhead (320 MiB + 4 KiB per token of context + 1% of the
weights).

## Ranked table

| # | Model | Why it wins | Benchmark evidence (source, date) | Best quant for 4 GB | Licence |
|---|---|---|---|---|---|
| 1 | **Nanbeige4.2 3B** | Best reported reasoning and coding at 3-4B: it beats Qwen3.5-9B and Gemma 4 12B across the vendor suite, and its 22-layer/8-KV layout leaves room for 16K context at Q4_K_M on Linux. | Vendor card, 2026-07-21: GPQA-Diamond 87.4, HLE 17.8, LiveCodeBench-V6 72.5, SWE-bench Verified 63.6, Terminal-Bench 2.0 44.1, IF-Bench 54.6. [Nanbeige4.2-3B card](https://huggingface.co/Nanbeige/Nanbeige4.2-3B). No AA index yet. | Q4_K_M, 2.68 GB (2.50 GiB); 16K with q4_0 KV totals 3.29 GiB on Linux, 8K q4_0 totals 3.06 GiB. | Apache-2.0 |
| 2 | **K2 Horizon 3.7B** | Best agentic/terminal model in this class and the only pick with independent AA component scores; native 512K context and an anti-doom-loop post-training stage aimed at long agent runs. | [AA model page](https://artificialanalysis.ai/models/k2-horizon-3-7b) (fetched 2026-11-06): HLE 13.9, GPQA 69.2, SciCode 22.0, AA-LCR 62.3, Terminal-Bench 2.1 28.1, tau2-Banking 19.6. Vendor card, 2026-09-03: SWE-bench Verified 65.6, Terminal-Bench 2.1 39.7, BFCL v4 64.6. [IFM card](https://huggingface.co/IFM/K2-Horizon-3.7B). | IQ3_XXS, 2.32 GB (2.16 GiB); 8K q4_0 totals 2.84 GiB (fits Windows too), 16K q4_0 totals 3.19 GiB on Linux. | Apache-2.0 |
| 3 | **MiniCPM5 2B** | The small-footprint option: 64K context fits in 2.77 GiB with q4_0 KV, and it has the strongest long-context and tool-use numbers per parameter in this tier. | [AA model page](https://artificialanalysis.ai/models/minicpm5-2b) (fetched 2026-11-06): GPQA 70.2, HLE 8.9, AA-LCR 59.3, tau2-Banking 20.8, SciCode 26.3, Terminal-Bench 2.1 8.6. Vendor card, 2026-09-07: 2B-class SOTA on its comparison set. [OpenBMB card](https://huggingface.co/openbmb/MiniCPM5-2B). | Q4_K_M, 1.56 GB (1.45 GiB); 64K with q4_0 KV totals 2.77 GiB, 32K with f16 KV totals 3.22 GiB. | Apache-2.0 |

All three validate against llama.cpp b11960 with
`python3 scripts/verify_catalog.py --catalog agents/T02/tiers/tier-4.json`
(9/9 quant sets ok, architectures `nanbeige` and `k2-horizon` both known).

## Why these three

**Nanbeige4.2 3B (rank 1).** The 2026-07-21 refresh of the Nanbeige 3B line is
the only 3-4B model in this tier whose vendor table claims wins over both
Qwen3.5-9B and Gemma 4 12B, and the margin is not small: GPQA 87.4 vs 81.7,
SWE-bench Verified 63.6 vs 53.1, Terminal-Bench 2.0 44.1 vs 29.2. Those are
vendor-run numbers in thinking mode (`preserve_thinking=true`), so treat them
as an upper bound, but even discounted they put it ahead of the catalog's
Granite 4.2 3B (AA 4.72, HLE 6.6, GPQA 55.9). The architecture is friendly to
4 GB: 22 layers with 8 KV heads and head_dim 128 cost 88 KiB per token at f16,
so Q4_K_M plus 16K context with q4_0 KV is 3.29 GiB, inside the Linux budget.
On Windows (3.0 GiB) step down to Q3_K_M or IQ4_XS at 8-16K.

**K2 Horizon 3.7B (rank 2).** This is the pick I trust most, because
Artificial Analysis has actually run its v5.0.1 suite on it and the numbers are
public even though the index itself is not assigned yet. HLE 13.9 and GPQA
69.2 are more than double Granite 4.2 3B's, and the terminal/tool rows
(Terminal-Bench 2.1 28.1, tau2-Banking 19.6) are what matter for a local
coding agent. The vendor card adds SWE-bench Verified 65.6 and BFCL v4 64.6.
The cost is KV memory: 36 layers x 8 KV heads x 128 is 144 KiB per token at
f16, so 4 GB means q4_0 KV and 8-16K context. IQ3_XXS (2.16 GiB) is the
largest quant that leaves a working plan, and it clears EZLM's 0.94 quality
floor (IQ3_XXS is scored 0.945).

**MiniCPM5 2B (rank 3).** A 2.52B model with only 2 KV heads is the right
answer when the 4 GB budget is shared with the OS or the user wants a long
context rather than peak intelligence. AA's components (GPQA 70.2, AA-LCR
59.3, tau2-Banking 20.8) are close to the two 3-4B picks on knowledge and tool
use, and the 64K context at q4_0 KV fits in 2.77 GiB. Terminal-Bench 2.1 at
8.6 is the weak spot: do not pick it as a coding agent. It is also the only
one of the three that fits a 3.0 GiB Windows budget at 32K context without
dropping to Q3.

## What I rejected and why

- **NVIDIA Nemotron 3.5 Bolt E4B 4GB (2026-10-23).** The marquee "4 GB"
  model of the month, post-trained from Gemma 4 E4B, with excellent
  instruction-following and tool rows on its card. It does not fit EZLM's 4 GB
  budget: the smallest llama.cpp file is the unsloth UD-IQ2_M at 3.53 GB
  (3.29 GiB), and after overhead and any KV cache the plan is 3.9-4.3 GiB.
  NVIDIA's 4 GB claim relies on the Gemma per-layer embedding tables living in
  system RAM, which the catalog schema and planner do not model. It should be a
  6 GB pick, or the schema needs a `cpu_resident_bytes` field.
- **K2 Horizon 3.7B at Q4_K_M (3.25 GB, 3.03 GiB).** With 8K context and
  q4_0 KV the plan is 4.05 GiB, so it misses even the Linux budget. The 4 GB
  entry lists IQ3_XXS/Q3_K_M/IQ3_M only.
- **Nanbeige4.2 3B at Q4_K_M on Windows.** 8K q4_0 is 3.06 GiB against a
  3.0 GiB Windows budget, a hair over. IQ4_XS (2.83 GiB at 8K q4_0) or Q3_K_M
  (2.87 GiB at 16K q4_0) are the Windows-safe quants.
- **Any MoE.** The smallest credible MoE candidate, Ling 3.0 Tiny (7.9B total,
  1.3B active, MIT), starts at 3.00 GB for Q2_K and only reaches Q3 at
  3.6-3.9 GiB. Q2_K scores 0.88 on EZLM's quant-quality scale, below the 0.94
  floor for models under 20B, so the planner would skip it. There is no MoE
  that fits 4 GB with a usable context today.
- **Ministral 3 3B (2025-12-02, Apache-2.0, 256K).** Solid Mistral small
  model, but older than the picks and no independent evaluation turned up in
  AA or LMArena; its reported numbers do not beat Nanbeige4.2 or K2 Horizon.
  Keep as a fallback when a Mistral-family licence or tokenizer is required.
- **Models already in the catalog.** Granite 4.2 3B (AA 4.72), Qwen3.5 4B,
  Gemma 4 E2B, Ternary Bonsai 2 4B and Bonsai 8B already have catalog ids, so
  they cannot be proposed here. Granite 4.2 3B remains the best
  independently-verified 4 GB option and is the anchor for the estimates
  above; none of the new picks is clearly worse, and two are clearly better on
  the published numbers.
- **K2 Horizon 7B and gpt-oss-20b.** 7B at Q4_K_M is about 4.3 GB and
  gpt-oss-20b is 11.9 GB, both well outside the tier.

## Evidence notes

- Scores are Artificial Analysis Intelligence Index v5.0.1 values where AA has
  assigned one; none of the three has an index yet, so all three scores are
  ESTIMATEs and the `score_source` field says exactly how each was derived.
  The catalog's own Granite 4.2 3B entry (4.72, AA, fetched 2026-11-02) is the
  calibration point for the small-model floor.
- AA component scores were read from the public model pages on 2026-11-06.
  LMArena's text leaderboard (accessed 2026-11-06) lists Granite 4.2 3B/8B
  and Gemma 4 but not K2 Horizon, Nanbeige4.2 or MiniCPM5, so it could not be
  used as a second independent check for the picks.
- GGUF metadata (layers, KV heads, head dim, native context, chat template)
  was read with HTTP range requests from the files themselves, the same way
  `scripts/verify_catalog.py` does; sizes come from the Hugging Face tree API.
- Quant-quality overrides: none. IQ3_XXS (0.945) is the lowest quant listed
  and is above the 0.94 floor for sub-20B models.

## Open questions

1. AA may publish official indexes for K2 Horizon 3.7B and MiniCPM5 2B soon.
   Replace the estimates when it does.
2. Nanbeige4.2 3B has no independent evaluation at all. If a second source
   contradicts the vendor table, demote it below K2 Horizon.
3. Gemma 4 E4B derivatives (Nemotron Bolt) need a planner field for
   CPU-resident embedding tables before they can be offered on 4 GB machines.
