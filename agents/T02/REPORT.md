# T02. New model scouting (November 2026) — consolidated report

16 agents (a01–a16) worked this task in parallel: one per memory tier (4, 6, 8, 12, 16, 24, 32, 48,
64, 96, 128+ GB) plus five cross-cutting jobs (retirement study, schema and validator, benchmark
sources, GGUF helper, assembly). Consolidated onto `agent/T02-model-scouting` by the Lead.

## Result

- **11 tier proposals**, one file per tier under `agents/T02/tiers/`, all verified against real GGUF
  headers and the Hugging Face tree API (no downloads).
- **`agents/T02/catalog-proposed.json`**: 37 entries, `version: 1`, catalog schema.
  - **26 new ids** ready to add.
  - **11 incumbent ids** re-proposed with the quant subset that fits that tier (deliberate: the
    tiers are about which quant to run, not only which new model to add).
- **Validator:** `agents/T02/validate_proposal.py` — 37/37 entries pass, 0 failed, 2 warnings
  (one `UNVERIFIED` licence note).
- **Retirement study:** 13 of the current 32 catalog models proposed for retire/replace, 19 keep.
- **Every quant file set verified** by `scripts/verify_catalog.py --catalog <tier file>` against the
  pinned llama.cpp **b11960**: 0 errors across all 11 tier files.

## Tier summary

| Tier | Models | Strongest picks |
|---|---|---|
| 4 GB | 3 | Nanbeige4.2 3B (Q4_K_M 2.68 GB), K2 Horizon 3.7B, MiniCPM5 2B. Dense only; the tier is KV-bound, so 4 GB means q4_0 KV and 8–16K context. |
| 6 GB | 4 | K2 Horizon 7B (top), K2 Horizon 3.7B, Ministral 3 8B reasoning (vision), LFM2.5 8B A1B (MoE). |
| 8 GB | 6 | Ornith 1.5 9B (new, MIT), Granite 4.2 8B, Qwen3.5 9B, Gemma 4 12B, Nemotron Bolt E4B, LFM2.5 8B A1B. |
| 12 GB | 7 | Qwen3.8 27B, Ternary Bonsai 2 27B, Nanbeige4.3 33B, Qwen3.6 35B A3B, K2 Horizon 7B (new), Gemma 4 12B/31B (vision). |
| 16 GB | 5 | Qwen3.8 27B, Nanbeige4.3 33B, Ternary Bonsai 2 27B, Qwen3.6 35B A3B, Gemma 4 31B. |
| 24 GB | 4 | Ornith 1.5 35B A3B (new, MIT), Groot 19B A3B, Flint 40B A2.5B, LFM2.5 24B A2B. |
| 32 GB | 7 | Qwen3.8 27B, Nanbeige4.3 33B, Ternary Bonsai 2 27B, Ornith 1.5 35B A3B, K2 Horizon MoVA 36B A4B, Nemotron Bolt 74B A9B, Gemma 4 31B. |
| 48 GB | 5 | Flint 40B A2B, K2 Horizon MoVA 36B A4B, Hypernova 60B, EXAONE 4.5 33B, Kimi Linear 48B A3B. |
| 64 GB | 5 | Step 3.7 Flash, K2 Horizon MoVA 36B A4B, Kimi Linear 48B A3B, Qwen3 Coder Next, Mistral Medium 3.5 128B. |
| 96 GB | 5 | Qwen3.8 Flash Next, Nemotron 3.5 Super VL 120B A12B, Step 3.7 Flash, Qwen3 Coder Next, Mistral Medium 3.5 128B. |
| 128+ GB | 6 | DeepSeek V4.1 Flash 1017 (32.59), GLM-5.3 (31.65), MiniMax M3.1 Flash (29.06), GLM-5.3 Flash (27.8), Ornith 1.5 122B, Flint. |

An alternate 24 GB lineup of six incumbent entries is preserved at
`agents/T02/a06/tier-24-incumbents.json` (not assembled; the brief wants new ids).

## Cross-cutting deliverables

- **`agents/T02/SCHEMA.md`** — every field, type and range a proposal must satisfy, including the
  rules the validator enforces.
- **`agents/T02/validate_proposal.py`** — strict offline checker: per-entry PASS/FAIL, id uniqueness
  against `catalog.json`, quant bytes ascending, split-file naming, repo/file URL rules, draft and
  SWA cross-checks, `--json` / `--lenient` / `--no-existing`.
- **`agents/T02/BENCHMARKS.md` + `benchmarks.json`** — 23 evaluation sources with URL, coverage,
  caveats, cadence and access date. Key caveats: Open LLM Leaderboard is frozen (2025-03-15), the
  Aider board is stale (2025-10-03), Artificial Analysis drifted from v5.0 to v5.0.1, LMArena 403s
  scripts, and **no source measures quantised GGUFs or local tok/s**.
- **`agents/T02/gguf_info.py`** — repo + file → catalog fields via range requests, stdlib only,
  `--json/--parts/--revision`. Validated against all 32 catalog models (every layer/KV/head/ctx
  matched), 1.3–2.1 s and ~16.8 MB per header.
- **`agents/T02/retire-proposed.md` / `.json`** — 13 of 32 catalog models proposed for retire or
  replace, 19 keep.
- **`agents/T02/assemble.py`** — the merger used to build `catalog-proposed.json` (dedupe by id,
  keep the smallest tier, sort by score, warnings for catalog-id collisions).

## Decisions the morning merge needs

1. **Incumbent ids in proposals.** Eleven entries reuse catalog ids with tier-specific quant subsets.
   If the catalog should stay new-only, filter those out; the new ids are the other 26.
2. **Scores are mostly estimates.** Artificial Analysis has no v5 index for most new models; every
   such `score_source` says `ESTIMATE`, names the method and gives a range. Treat them as
   order-of-magnitude until AA covers the model.
3. **llama.cpp architecture support gates the catalogue.** Ripple-65B (arch `ricursive`, needs
   b12167+), LongCat-Flash-Lite (`longcat-flash-ngram`) and G9v3-39A5B (`g9v3`) are absent from the
   pinned b11960 and were excluded. Kimi K3.1 Pro (AA 46.45) still has no GGUF.
4. **Two catalog entries should be extended** (a11): `qwen3.8-flash-next` stops at IQ3_XXS but the
   repo has IQ4_XS (93.7 GB) and Q4_K_XL (111.3 GB, 82.5 GB resident with the 28.8 GB lazy table);
   `deepseek-v4.1-flash-1017` needs `lazy_bytes` for its 196.6B embedding table or it looks twice
   its resident size.

## Files

- `agents/T02/tiers/tier-<label>.json` and `.md` — 11 tiers (4, 6, 8, 12, 16, 24, 32, 48, 64, 96, 128plus).
- `agents/T02/catalog-proposed.json` — the assembled proposal (37 entries).
- `agents/T02/SCHEMA.md`, `validate_proposal.py`, `assemble.py`, `gguf_info.py`, `BENCHMARKS.md`, `benchmarks.json`, `retire-proposed.*`.
- `agents/T02/a01..a16/REPORT.md` — one report per agent.

## Next steps

1. Decide new-only vs replacement entries, then bump the catalog version and add the accepted ids.
2. Fill the AA gap: when AA publishes v5 indexes for the new models, replace the ESTIMATE scores.
3. Extend the two catalog entries named above, and add `lazy_bytes` to DeepSeek V4.1 Flash.
4. Re-check the excluded architectures when llama.cpp moves past b11960 (T06 covers the upgrade plan).
