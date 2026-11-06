# T02 a09: 64 GB tier

Agent a09, tier 64 GB (usable GPU or unified memory). Branch `agent/T02-a09`.

## What I did

1. Read `internal/catalog/catalog.json` (version 8, 32 models) and used its entries as the
   schema and as the incumbent baseline for this tier.
2. Pulled the Artificial Analysis v5.0 leaderboard data from the model pages (the legacy
   `/api/v2/data/*` endpoint returns 410 Gone since 2026-11-04; the page embeds the full model
   list with `intelligenceIndex` and component scores, which is what I parsed).
3. Enumerated the open-weight models released up to 2026-11-06 and measured, for every
   candidate, the smallest GGUF quant in `ggml-org`, `unsloth`, `bartowski`, `Qwen`, `IFM` or
   `stepfun-ai` repos from the Hugging Face tree API. Models whose smallest quant exceeds
   ~60 GB were dropped for this tier.
4. For the five picks, read the GGUF header of the first file of each quant with HTTP range
   requests (same reader as `scripts/verify_catalog.py`) to get `block_count`,
   `attention.head_count_kv`, `attention.key_length`, `context_length`, sliding-window and
   MLA/linear-attention keys, and to confirm a chat template and an `mmproj` file exist.
5. Wrote `agents/T02/tiers/tier-64.json` (5 entries, 12 quant file sets, all byte sizes from
   the tree API) and `agents/T02/tiers/tier-64.md` (ranked table with incumbents, picks,
   drops, planner notes).

## Findings

**F1 (major, scouting): the 64 GB tier has no new winner in November 2026.** Every open model
that scores above Nanbeige4.3-33B (20.4) and Qwen3.8-27B (20.1) is 180B or larger, and the
smallest quant of the 180B Qwen3.8-Flash-Next is UD-IQ1_S at 66,423,878,624 bytes
(`unsloth/Qwen3.8-Flash-Next-GGUF`), 2.4 GB over the tier budget before any KV cache. GLM-5.3-Flash
(93.1 GB), MiniMax-M3.1-Flash (129.4 GB) and DeepSeek V4.1 Flash 1017 (233.0 GB) are further out.
So a 64 GB proposal is an *addition* set, not a replacement set: the catalog's dense 27-33B picks
remain the best value at this tier. Evidence: tree API on the repos above, 2026-11-06.

**F2 (minor, catalog gap): the catalog has no coding-specialist entry that fits 64 GB.**
`qwen3.6-35b-a3b` is the nearest but scores 9.5. `qwen3-coder-next` (80B/3B, Apache-2.0,
256k context, non-thinking) fits at Q5_K_M (56.7 GB) and is proposed here; its general index is
low (estimated 5.3, dragged down by AA-Briefcase 404 Elo), so it is a coding-only pick.

**F3 (minor, data): Artificial Analysis retired its legacy data API on 2026-11-04.** Anything
that read `/api/v2/data/llms/models` now gets HTTP 410 and must move to
`/api/v2/language/models/free` (needs a key) or scrape the model pages. The catalog's
`score_source` strings cite "leaderboard JSON, fetched 2026-11-02", i.e. the retired endpoint.

**F4 (minor, verifier): hybrid attention models trip the verifier's KV undercount check.**
`scripts/verify_catalog.py:195` warns "kv_layers 12 < 48 attention layers" for
`qwen3-coder-next` because the GGUF carries `ssm.*` keys but no `full_attention_interval`.
12 is correct (12 blocks of 4 layers, one full-attention layer each). The same warning would
appear for any Qwen3-Next-family GGUF; the check only treats it as a warning, so validation
still exits 0.

**F5 (minor, licence): Mistral Medium 3.5 is "Modified MIT", not plain MIT.** The LICENSE file
in `mistralai/Mistral-Medium-3.5-128B` adds a USD 20M/month consolidated-revenue cap. The
catalog's `license` field should say so if this model is added; `mistral-small-4-119b` in
catalog v8 already carries a plain "Apache-2.0" for a different model, so no conflict.

**F6 (minor, quant hygiene): Step 3.7 Flash cannot be taken from the official repo at 64 GB.**
`stepfun-ai/Step-3.7-Flash-GGUF` starts at IQ3_XXS (75.8 GB). The tier entry uses
`unsloth/Step-3.7-Flash-GGUF` (UD-IQ1_M 56.8 GB, UD-IQ2_XXS 61.7 GB), which is a third-party
quant of an Apache-2.0 model. Worth a catalog note.

## Validation

```
$ python3 scripts/verify_catalog.py --catalog agents/T02/tiers/tier-64.json --jobs 4
llama.cpp b11960: 166 architectures, 44 tensor types; checking 12 files sets
...
qwen3-coder-next  Q4_K_M  WARN kv_layers 12 < 48 attention layers
qwen3-coder-next  Q5_K_M  WARN kv_layers 12 < 48 attention layers
0 file set(s) with errors
```

All 12 file sets: file exists, byte total matches, split parts named for llama.cpp, tensor
types known to b11960, architecture known, and the KV numbers agree with the file (the two
warnings are F4). IDs do not collide with any catalog v8 id; quants are sorted by size.

## What I could not verify

- **No independent benchmark exists for three of the five picks.** Step 3.7 Flash, K2 Horizon
  MoVA 36B A4B, Kimi Linear 48B A3B and Qwen3 Coder Next have no official AA v5 index, and
  Kimi Linear has no AA component scores at all. Their `score_source` strings state the method
  (a linear fit on AA components, or a hand placement) and the error bars; they are estimates,
  not measurements.
- **The fitted estimates are only as good as the fit.** The v5 four-component fit (AutomationBench,
  Terminal-Bench 4.0, GDP.pdf, AA-Briefcase) has RMSE 3.0 on 107 models; the legacy
  SciCode/CritPt fit has RMSE 5.5 on 98 models. Step 3.7 Flash's 11.4 should be read as 11 +/- 5.
- **No decode-speed measurement.** I did not load any model (README rule). Throughput claims in
  the notes are derived from active-parameter counts and card claims, not measured.
- **Quant quality at 2.3 bits (Step 3.7 Flash IQ1_M) is unmeasured.** It fits; whether it is
  worth it versus Nanbeige4.3-33B at Q8 needs a real eval on a 64 GB machine.
- **Mistral Medium 3.5's "GGUFs built before the config fix are degraded" note** comes from the
  model card; I checked that the unsloth files post-date the fix but did not evaluate quality.

## Next steps

1. Run a 64 GB machine test: Nanbeige4.3-33B Q8_0 versus Step 3.7 Flash UD-IQ1_M versus
   Nemotron 3.5 Super VL UD-IQ3_XXS on the same 10-prompt coding and tool-calling set.
2. Re-score Step 3.7 Flash, K2 Horizon MoVA and Qwen3 Coder Next when AA publishes a v5 index.
3. If a 96 GB tier agent proposes Qwen3.8-Flash-Next (66.4 GB) and K2 Horizon 375B A23B
   (85.0 GB), do not also propose them here; the boundary between 64 and 96 is exactly those two.
4. Ask a16 to dedupe by id across tiers; `k2-horizon-mova-36b-a4b` and `kimi-linear-48b-a3b`
   may also be proposed by the 48 GB and 96 GB agents.
