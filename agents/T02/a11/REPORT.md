# T02 agent a11 report: 128 GB and above tier

Branch `agent/T02-a11`, worktree `~/ezlm-agents/work/subs/T02/a11`, 2026-11-06.

## What I did

- Read `README.md`, the T02 brief and the T02 section of `TASKS.md`, then the catalog
  schema (`internal/catalog/catalog.go`), the current catalog, and `internal/fit/fit.go`
  so the proposals match how plans are scored and budgeted.
- Surveyed the November 2026 open-weight field: the Artificial Analysis leaderboard page
  (fetched 2026-11-06), Hugging Face trending and per-organisation listings, and the
  model cards of every candidate.
- For each pick, read the real file list and byte sizes from the Hugging Face tree API and
  the real GGUF metadata (layers, KV heads, head size, context, sliding window, split
  count, tensor types) from the first file of each quant with HTTP range requests, using
  the same reader as `scripts/verify_catalog.py`.
- Computed planned memory (weights + KV at 64K + compute buffers) with the formulas in
  `internal/fit/fit.go`, and effective scores with its quant factors, to rank the picks by
  the machine size they actually serve.
- Wrote `agents/T02/tiers/tier-128plus.json` (6 models, 35 quant sets) and
  `agents/T02/tiers/tier-128plus.md`.
- Validation: `python3 scripts/verify_catalog.py --catalog agents/T02/tiers/tier-128plus.json`
  prints `0 file set(s) with errors`; all 35 sets are `ok`, with no warnings.
- Helper scripts, kept for review: `agents/T02/a11/quants.py` (quant directory sizes from
  the tree API), `agents/T02/a11/gguf_meta.py` (GGUF metadata over range requests),
  `agents/T02/a11/build_tier.py` (regenerates the tier JSON from the API).

## Findings

### 1. The catalog entry for Qwen3.8-Flash-Next stops at the wrong quant (major)

`internal/catalog/catalog.json`, entry `qwen3.8-flash-next`: its `quants` list ends at
UD-IQ3_XXS (75.8 GB). The unsloth repo now also has UD-IQ4_XS (93.67 GB), UD-Q4_K_XL
(111.33 GB), UD-Q5_K_XL (158.29 GB), UD-Q6_K_XL (169.17 GB) and Q8_0 (188.23 GB).

Each of those files ends with a `per_layer_token_embd.weight` tensor of shape
[160, 320001536] stored as IQ4_NL, which is 28.80 GB that can stay on disk with
`--lazy-mode`. UD-Q4_K_XL is therefore 82.53 GB resident and 85.6 GB planned at 64K, so
it fits a 128 GB machine comfortably, and its effective score (27.0 x 0.985 = 26.6) is
higher than GLM-5.3-Flash at IQ3_XXS (26.3) at 38 GB more planned memory. It is the best
128 GB plan found in this study. The entry should be extended with IQ4_XS and Q4_K_XL at
least, and with Q5/Q6/Q8 for 160 to 256 GB machines.

Evidence: tree API for `unsloth/Qwen3.8-Flash-Next-GGUF`; header of
`UD-Q4_K_XL/Qwen3.8-Flash-Next-UD-Q4_K_XL-00004-of-00004.gguf` (one tensor, type 20,
IQ4_NL). The catalog's own `lazy_bytes` values for this model (for example 22.58 GB on
GSQ-RCO-Q2_0) use the same mechanism.

### 2. DeepSeek V4.1 Flash 1017 needs `lazy_bytes` or it looks twice as large (major)

`deepseek-ai/DeepSeek-V4.1-Flash-1017` is 763.2B parameters, but 196.6B of them are one
`per_layer_token_embd.weight` table (768,022,850 rows of 256). It is shipped as its own
shard in every quant: IQ4_NL (110.6 GB) in UD-IQ1_S and UD-IQ1_M, Q5_K (135.2 GB) in
UD-IQ2_XXS and UD-Q2_K_XL, Q6_K (161.3 GB) in UD-IQ3_XXS. Without `lazy_bytes` the
smallest file reads as 233.0 GB and the model looks like a 320 GB+ proposition; with it
the resident body is 122.4 GB at IQ1_S, 165.3 GB at IQ2_XXS and 199.6 GB at Q2_K_XL.
The tensor name is identical to the n-gram table the catalog already treats as lazy for
Qwen3.8-Flash-Next, so the same `--lazy-mode` path should apply. Confirm on a real
machine before trusting a 192 GB plan.

Evidence: header of the last shard of each quant in
`unsloth/DeepSeek-V4.1-Flash-1017-GGUF`; `params_b` from the HF safetensors count.

### 3. The strongest open model has no GGUF yet (minor)

Kimi K3.1 Pro scores 46.45 on the Artificial Analysis index, 14 points above anything
else open, but as of 2026-11-06 there is no GGUF: `unsloth/Kimi-K3.1-Pro` is a
safetensors mirror (96 shards) and `DevQuasar/moonshotai.Kimi-K3.1-Pro-GGUF` held only a
README. Kimi K3's smallest quant is UD-Q1_0 at 466.4 GB, so K3.1 Pro will need 512 to
768 GB. It is the first thing to add when a GGUF appears.

### 4. Ornith 1.5 122B licence is unverified (minor)

`ornith-ai/Ornith-1.5-122B` declares MIT, but its `license_link` points at
https://huggingface.co/ornith-ai/Ornith-1.5-122B/blob/main/LICENSE, which returns
"Entry not found". The vendor country is not stated anywhere on the card or the site. The
entry says MIT and flags both in `notes`.

### 5. MiniMax-M3.1-Flash parameter count differs between the card and the checkpoint (minor)

The card says 428B total and 23B active; the HF safetensors count is 220.5B because the
reference checkpoint stores experts as U8 and F8 at about 4.7 bits per parameter. The GGUF
sizes (UD-Q4_K_XL 249.4 GB) confirm the 428B figure, so `params_b` uses 428.0. Anyone
sizing from the safetensors count would be wrong by about 2x.

### 6. MiMo-V2.6-Flash has only one quant (minor)

`ggml-org/MiMo-V2.6-Flash-RL-GGUF` publishes only Q2_K (126.4 GB) and MXFP4 (167.4 GB).
Q2_K leaves about 1 GB of a 128 GB machine for the cache and MXFP4 needs 192 GB, so the
model cannot serve the 128 GB floor of this tier even though it scores 26.7.

### 7. `internal/fit/fit.go` has an unreachable quant case (minor)

`internal/fit/fit.go:130-135`: the switch returns 0.8 for `IQ1_M` at line 130, so the
`case strings.HasPrefix(n, "IQ1_M"): return 0.82` at lines 132-133 can never match. The
0.82 value is dead code. Not touched (T02 owns only `agents/T02/`); a one-line fix is to
drop the 0.82 case or move it above the 0.8 one.

Also worth knowing for the other tier agents: `minQuality` returns 0.85 for models of
150B and more, so IQ1_S (0.78) and IQ1_M (0.80) are skipped for GLM-5.3, GLM-5.3-Flash,
DeepSeek V4.1 Flash and MiniMax-M3.1-Flash. The smallest quant the engine will actually
use for those four is IQ2_XXS. The tier JSON keeps the IQ1 files because the catalog does
the same for other models, but the markdown ranks the IQ2 files.

### 8. Artificial Analysis retired its legacy API on 2026-11-04 (minor)

`https://artificialanalysis.ai/api/v2/data/llms/models` now returns 410 Gone with a
migration note. The leaderboard page still embeds the model JSON with the index values, so
the scores in this tier were read from the page on 2026-11-06, but per-model component
scores need the new `/data-api` endpoints.

## What I changed

- `agents/T02/tiers/tier-128plus.json`: 6 models, 35 quant sets, every size from the
  tree API, every architecture field from the GGUF header.
- `agents/T02/tiers/tier-128plus.md`: the ranked table, the machine-size ladder, the
  reasoning per pick, and the rejected list.
- `agents/T02/a11/quants.py`, `agents/T02/a11/gguf_meta.py`,
  `agents/T02/a11/build_tier.py`: the helpers used to gather and regenerate the data.
- No Go files and no `catalog.json` edits.

## What I could not finish

- Kimi K3.1 Pro: no GGUF, so no quants could be listed. Reported instead of guessed.
- Artificial Analysis component scores: the legacy API is gone and the new one was not
  documented on the page at the time of writing.
- The lazy-mode assumption for the DeepSeek engram table is not tested on hardware; no
  model was loaded, per the rules.
- Ornith 1.5 122B and Flint have no independent index; their scores are estimates and are
  marked `ESTIMATE` in `score_source`.

## Next steps

1. Extend the `qwen3.8-flash-next` catalog entry with IQ4_XS, Q4_K_XL and, for larger
   machines, Q5_K_XL, Q6_K_XL and Q8_0, with `lazy_bytes` 28.80 GB on each. This is the
   single highest-value change for the 128 GB tier.
2. Add DeepSeek UD-Q3_K_XL and UD-Q4_K_XL for 448 to 512 GB machines once the shard that
   holds the engram table is identified for each (Q4_K_XL keeps it in a middle shard, not
   the last one).
3. Re-check Kimi K3.1 Pro weekly; it would take rank 1 across the whole tier.
4. Confirm lazy-mode residency for DeepSeek on a real 192 GB machine with a 64K prompt.
5. If the Lead wants one number per model rather than an estimate, run Ornith 1.5 122B and
   Flint through the same harnesses the catalog uses before the morning merge.
