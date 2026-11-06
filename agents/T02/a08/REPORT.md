# T02 / a08 report: 48 GB tier

Agent: a08. Tier: 48 GB usable GPU or unified memory.
Worktree: `~/ezlm-agents/work/subs/T02/a08`, branch `agent/T02-a08`.
Deliverables: `agents/T02/tiers/tier-48.json`, `agents/T02/tiers/tier-48.md`,
this report.

## What I did

1. Read `internal/catalog/catalog.json` (32 models, version 8) for the schema and
   the incumbent picks, and `internal/fit/fit.go` for the budget rules
   (`ComputeBudgets`, 10% margin, 64K minimum context for agent apps).
2. Pulled the Artificial Analysis leaderboard (2026-11-06). The legacy
   `/api/v2/data/llms/models` endpoint returns 410 Gone ("retired on Wed, 04 Nov
   2026"), so I decoded the RSC payload embedded in
   `https://artificialanalysis.ai/leaderboards/models` (51 `self.__next_f.push`
   chunks, 282 model rows, 183 open-weight). This gives the official v5 index
   plus per-component scores for every row.
3. Surveyed Hugging Face for models released since 2026-08-01 (`/api/models`
   search and sort by downloads and creation date), then fetched parameter
   counts from the safetensors totals to filter to what can fit 48 GB.
4. Read real GGUF metadata over HTTP range requests with the reader in
   `scripts/verify_catalog.py` (`header()`), and real byte sizes from
   `https://huggingface.co/api/models/<repo>/tree/main?recursive=true`.
5. Checked architecture and tensor-type support against llama.cpp b11960 by
   fetching `src/llama-arch.cpp` and `ggml/include/ggml.h`.
6. Wrote five catalog-ready entries and validated them:

```
$ python3 scripts/verify_catalog.py --catalog agents/T02/tiers/tier-48.json --jobs 4
llama.cpp b11960: 166 architectures, 44 tensor types; checking 19 files sets
... all 19 lines "ok" ...
0 file set(s) with errors
```

## The proposals

| id | params | active | best quant at 48 GB | score | licence |
|---|---|---|---|---|---|
| `flint-40b-a2b` | 39.79B | 2.5B | UD-Q6_K_XL 36.32 GB (full 262K) | 12.0 ESTIMATE | Apache-2.0 |
| `k2-horizon-mova-36b-a4b` | 37.44B | 4.0B | Q6_K 30.77 GB at 64K | 13.0 ESTIMATE | Apache-2.0 |
| `hypernova-60b-2605` | 58.66B | 4.8B | Q5_K_M 44.34 GB at 64K | 7.0 ESTIMATE | Apache-2.0 |
| `exaone-4.5-33b` | 34.35B | 34.35B | Q8_0 35.14 GB at 64K | 4.2 ESTIMATE | EXAONE 1.2-NC |
| `kimi-linear-48b-a3b` | 49.12B | 3.0B | Q4_K_M 30.06 GB (full 1M) | 2.0 ESTIMATE | MIT |

All five ids are new: none collides with an id in `internal/catalog/catalog.json`.

## Findings

1. **No 200B+ MoE fits 48 GB, even at 1 bit.** (major, for the whole task)
   Measured smallest quants: GLM-5.3-Flash UD-IQ1_S 93 GB
   (`unsloth/GLM-5.3-Flash-GGUF`), Ling 3.1 Flash IQ1_S 131 GB
   (`bartowski/Ling-3.1-flash-1019-GGUF`), MiniMax-M3.1-Flash UD-IQ1_M 129 GB
   (`unsloth/MiniMax-M3.1-Flash-GGUF`), Beam 1.0 UD-IQ1_S 103 GB
   (`unsloth/Beam-1.0-501B-A23B-GGUF`). The tier is therefore a 30B to 60B
   tier in November 2026.

2. **llama.cpp b11960 is missing two architectures that new 48 GB-class GGUFs
   use.** (blocker for two otherwise-good picks)
   `src/llama-arch.cpp` at b11960 lists 166 architectures but not
   `longcat-flash-ngram` (LongCat-Flash-Lite, 68.5B A3B, tau2 79.5%) or `g9v3`
   (G9v3-39A5B, 39B A5B, GPQA 80.5%). Their GGUFs exist and validate
   structurally, but `verify_catalog.py` flags them as unknown architectures,
   so they cannot ship with the pinned build. Recorded in tier-48.md.

3. **Artificial Analysis retired the v2 data API on 2026-11-04.** (minor, but it
   changes how every agent should read AA)
   `GET /api/v2/data/llms/models` returns HTTP 410 with "migrate to the
   documented V2 API". The full table (282 rows, official v5 index, component
   scores) is still embedded in the leaderboard page's React payload. I decoded
   it by joining the `self.__next_f.push([1,"..."])` chunks and
   `json.raw_decode`-ing the `initialModels` array.

4. **AA publishes v5 component scores for models without an official index.**
   (useful for the estimate fields)
   For example `k2-horizon-mova-36b-a4b` has `intelligenceIndex: null` but
   AA-Briefcase 1164.79, GDPval-AA 1196.67, AutomationBench 26.1%,
   HLE-Diamond 23.4%, AA-LCR 71.7%, TB2.1 56.6%, SciCode 40.0%, GPQA 82.2%.
   That is enough to fit a v5-equivalent estimate. I used two fits:
   a 5-component v5 fit (89 models, RMSE 2.9) and the legacy v4 fit
   (87 models, RMSE 4.9, matching the method already used in catalog.json).

5. **`params_b` should follow the counted total, not the marketing name.**
   (minor, consistency)
   K2 Horizon's name says 36B but its safetensors total is 37,444,792,020, and
   the Q8_0 GGUF is 39.83 GB (about 8.5 bits/param), which matches the counted
   total. EXAONE 4.5's 33B name excludes its 1.2B vision encoder (34.35B
   counted). I used the counted totals, as `qwen3.8-27b` does (27.78B).

6. **EXAONE 4.5 is non-commercial.** (major if EZLM ships it to paying users)
   `LGAI-EXAONE/EXAONE-4.5-33B/LICENSE` is "EXAONE AI Model License Agreement
   1.2 - NC": use "solely for research and educational purposes". It is still
   the only vision model in this tier with strong tool calling (tau2 78.1%), so
   it is listed with the licence called out.

7. **The catalog already owns the top of this tier.** (context, not a bug)
   `qwen3.8-27b` (20.1), `nanbeige4.3-33b` (20.4) and `qwen3.8-flash-next`
   (about 27 with 43.8 GB resident after lazy embedding bytes) all fit 48 GB and
   outscore every new model in this class. The new entries add coding-agent
   (Flint), agentic MoE (K2), 60B general (HyperNova), vision/multilingual
   (EXAONE) and 1M context (Kimi Linear) rather than a higher score.

## What I could not verify

- **Flint's benchmark numbers are vendor-reported** (Terminal-Bench 2.1 54.9%,
  SWE-bench Verified 74.2%, SWE-bench Pro v2 89.7%). Artificial Analysis has no
  Flint row as of 2026-11-06, so `score: 12.0` is an estimate anchored on
  K2 Horizon's independent TB2.1 56.6%.
- **Kimi Linear has no GDPval or CritPt component** on AA, so the legacy fit
  cannot be applied; 2.0 is a hand estimate from GPQA 41.2%, HLE 2.5%,
  AA-LCR 28.0%, LCB 37.8% and AIME-2025 36.3%.
- **KV sizes in the .md are computed, not measured.** They use
  `2 x kv_layers x ctx x kv_heads x head_dim x 2 bytes` (f16) plus the
  sliding-window share. No model was loaded, per the rules.
- **Qwen3.8-Flash-Next's `lazy_bytes` path** is taken from the catalog entry; I
  did not re-derive which tensors are lazy from the GGUF.
- The EXAONE mmproj vision file (2.57 GB) is not listed in `quants`, because
  the catalog schema has no mmproj field; other vision entries in the catalog
  have the same gap.

## Next steps

1. Re-run `verify_catalog.py` for `longcat-flash-lite` and `g9v3-39a5b` once
   llama.cpp adds `longcat-flash-ngram` and `g9v3`; both are better 48 GB picks
   than Kimi Linear and EXAONE.
2. Replace Flint's estimate with the official AA v5 index when it appears.
3. Decide the policy for non-commercial licences (EXAONE 1.2-NC) and for vision
   mmproj files before the catalog grows more multimodal entries.
4. For the assembly agent: the five ids are new and do not collide with
   `catalog.json`; `tier-48.json` has a top-level `models` key and sorts every
   quant list by bytes.

## Sources (all fetched 2026-11-06)

- https://artificialanalysis.ai/leaderboards/models (v5 index and components)
- https://huggingface.co/RecursiveSI/flint (card, benchmarks)
- https://huggingface.co/IFM/K2-Horizon-MoVA-36B-A4B (card, benchmarks)
- https://huggingface.co/MultiverseComputingCAI/Hypernova-60B-2605 (card)
- https://huggingface.co/LGAI-EXAONE/EXAONE-4.5-33B (card, LICENSE)
- https://huggingface.co/moonshotai/Kimi-Linear-48B-A3B-Instruct (card)
- https://raw.githubusercontent.com/ggml-org/llama.cpp/b11960/src/llama-arch.cpp
- HF tree API for each GGUF repo named above.
