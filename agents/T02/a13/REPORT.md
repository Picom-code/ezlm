# T02 / a13 report: schema and field audit

Branch `agent/T02-a13`, worktree `~/ezlm-agents/work/subs/T02/a13`.
Deliverables: `agents/T02/SCHEMA.md`, `agents/T02/validate_proposal.py`, this report.

## What I did

1. Read `internal/catalog/catalog.go` (the `Model`, `Quant`, `Draft`, `Catalog`
   structs and the planner helpers) and all 32 entries of
   `internal/catalog/catalog.json` (`version: 8`), plus `scripts/verify_catalog.py`
   so the offline rules line up with the online one.
2. Wrote `agents/T02/SCHEMA.md`: every model, quant, draft and sliding-window
   field with its JSON type, Go type and line, meaning, requiredness and exact
   range/format. It also documents the tier-file and assembled-file shapes, the
   URL rules (`FileURL` at `catalog.go:124`), the cross-field rules, what the
   offline checker cannot prove, and the known gaps in the shipped catalog.
3. Finished `agents/T02/validate_proposal.py` (a previous attempt left it
   incomplete in the worktree). The checker is standalone Python 3, no third-party
   imports. It validates, per entry:
   - presence of the core Go fields plus the five brief-only fields
     (`vendor`, `country`, `vision`, `native_1bit`, `notes`) in strict mode;
   - every field's type and range (numbers finite, integers not bools, string
     lengths and whitespace, ISO alpha-2 country, ids, repos, file paths);
   - id uniqueness within a file and against `internal/catalog/catalog.json`;
   - quant `bytes` ascending, quant names unique, duplicate files, split-file
     naming (`-00001-of-0000N.gguf`), `lazy_bytes < bytes`, `quality` in (0,1];
   - URL rules: bare `org/name` repos (no scheme, `huggingface.co/`,
     `/resolve/`, `.git`, trailing slash), repo-relative `.gguf` files (no
     absolute paths, backslashes, `..`, empty components, query/hash, whitespace);
   - draft type in `draft-mtp|dflash|dspark|eagle3`, and draft repo/file/bytes;
   - sliding-window fields set together and consistent with `layers`/`kv_layers`/`ctx_max`;
   - tier label, file-name/tier agreement, unknown fields, and the assembled
     catalog's `version` (must exceed the embedded 8 for `catalog.Load`).
   Output is one `[PASS]`/`[FAIL]` line per entry with each error/warning
   indented under it, per-file and overall totals, and `--json` for a
   machine-readable report. Exit codes: 0 pass, 1 any failure, 2 unreadable or
   invalid JSON / bad arguments. `--lenient` audits the shipped catalog (only
   fields Go decodes are required); `--allow-extra`, `--existing PATH` and
   `--no-existing` are available.

## Test evidence

### The four tier files that existed at 03:35

```
$ python3 agents/T02/validate_proposal.py \
    ~/ezlm-agents/work/subs/T02/a01/agents/T02/tiers/tier-4.json \
    ~/ezlm-agents/work/subs/T02/a02/agents/T02/tiers/tier-6.json \
    ~/ezlm-agents/work/subs/T02/a04/agents/T02/tiers/tier-12.json \
    ~/ezlm-agents/work/subs/T02/a09/agents/T02/tiers/tier-64.json
existing catalog: .../a13/internal/catalog/catalog.json (32 ids)

.../tier-4.json  (tier 4)
  [PASS] nanbeige4.2-3b
  [PASS] k2-horizon-3.7b
  [PASS] minicpm5-2b
  3 entries: 3 passed, 0 failed, 0 warnings

.../tier-6.json  (tier 6)
  [PASS] k2-horizon-7b
  [PASS] k2-horizon-3.7b
  [PASS] ministral-3-8b-reasoning
  [PASS] lfm2.5-8b-a1b
         warn: lfm2.5-8b-a1b.license is marked UNVERIFIED
  4 entries: 4 passed, 0 failed, 1 warning

.../tier-12.json  (tier 12)
  [FAIL] nanbeige4.3-33b
         error: nanbeige4.3-33b: missing required field 'native_1bit'
         error: nanbeige4.3-33b: id 'nanbeige4.3-33b' already exists in internal/catalog/catalog.json
  [FAIL] qwen3.8-27b
         error: qwen3.8-27b: missing required field 'native_1bit'
         error: qwen3.8-27b: id 'qwen3.8-27b' already exists in internal/catalog/catalog.json
  [FAIL] ternary-bonsai-2-27b
         error: ternary-bonsai-2-27b: id 'ternary-bonsai-2-27b' already exists in internal/catalog/catalog.json
  [FAIL] qwen3.6-35b-a3b
         error: qwen3.6-35b-a3b: missing required field 'native_1bit'
         error: qwen3.6-35b-a3b: id 'qwen3.6-35b-a3b' already exists in internal/catalog/catalog.json
  [PASS] k2-horizon-7b
  [FAIL] gemma-4-12b
         error: gemma-4-12b: missing required field 'native_1bit'
         error: gemma-4-12b: id 'gemma-4-12b' already exists in internal/catalog/catalog.json
  6 entries: 1 passed, 5 failed, 0 warnings

.../tier-64.json  (tier 64)
  [PASS] step-3.7-flash
  [PASS] k2-horizon-mova-36b-a4b
  [PASS] kimi-linear-48b-a3b
  [PASS] qwen3-coder-next
  [PASS] mistral-medium-3.5-128b
  5 entries: 5 passed, 0 failed, 0 warnings

4 files, 18 entries: 13 passed, 5 failed, 0 file errors
FAILED  (exit 1)
```

### Synthetic fixtures (`/tmp/a13fx`, not committed)

Built from the valid `tier-4.json` by mutation; every mutation failed with the
expected message and exit 1:

| fixture | expected failure | observed |
|---|---|---|
| `ok.json` | none | 3/3 PASS, exit 0 |
| `bad-tier.json` | tier not in the allowed set | `tier '7' is not one of ...`, exit 1 |
| `dup-id.json` | duplicate id | `duplicate id (first seen at entry[0])`, exit 1 |
| `unsorted.json` | quants reversed | two `quants[N].bytes ... sorted by size` errors, exit 1 |
| `urls.json` | `USA`, URL repo, absolute file | country, repo URL, absolute-path errors, exit 1 |
| `types.json` | missing notes, `moe: 1`, `layers: 0`, `ctx_max: 512` | all four reported, exit 1 |
| `draft.json` | bad draft type | `draft.type is 'draft-nope'; allowed: ...`, exit 1 |
| `swa-partial.json` | one SWA field | missing 3 SWA fields + sum > layers, exit 1 |
| `split.json` | wrong part count in split name | `part 2 ... llama.cpp looks for ...`, exit 1 |
| `extra.json` | unknown field, quality 1.5 | both errors, exit 1 |
| `empty.json` | `models: []` | `'models' is empty`, exit 1 |
| `list.json` | top level is a list | warning + PASS, exit 0 |
| `notjson.json` | malformed JSON | `not valid JSON`, exit 2 |

`--json` was checked on `tier-4.json` (`ok: true`, 3 models, exit 0), and
`--no-existing` on the same file (exit 0).

### Shipped catalog audit

```
$ python3 agents/T02/validate_proposal.py --lenient internal/catalog/catalog.json
1 file, 32 entries: 19 passed, 13 failed, 0 file errors
```

All 13 failures are the same rule: those models list quants grouped by source
repo rather than ascending by `bytes`. That is the brief's sorting rule applied
to proposals; the shipped catalog predates it. With `--no-existing` the result
is identical, so the only failures are sorting, not ids.

## Findings (severity)

- **major, tier-12 (a04):** `agents/T02/tiers/tier-12.json` reuses five ids
  that are already in `internal/catalog/catalog.json` (`nanbeige4.3-33b`,
  `qwen3.8-27b`, `ternary-bonsai-2-27b`, `qwen3.6-35b-a3b`, `gemma-4-12b`).
  Per the brief a proposal id must not collide with an existing catalog id, so
  these entries will fail the merge check. If the intent is to *keep* an
  existing model in a tier, the assembler should treat it as a reference rather
  than a new entry, or the id needs a suffix. Four of the five also omit
  `native_1bit`, which the brief requires filled.
- **minor, tier-6 (a02):** `lfm2.5-8b-a1b` marks its licence `UNVERIFIED`;
  warning only, but the Lead should decide before merge.
- **minor, shipped catalog:** 13/32 models have quants not sorted ascending by
  bytes (see above). `native_1bit` is omitted on 26/32 and `notes` on
  `bonsai-27b-1bit`; `gemma-4-e4b`/`gemma-4-e2b` have `active_params_b` below
  `params_b` with `moe: false` (MatFormer, expected). These are catalog-quality
  observations for a12/a16, not proposal failures.
- **minor, schema ambiguity:** `kv_layers: 0` means "all layers" in
  `Model.KVBytesPerToken` (`catalog.go:90`) but strict mode requires a positive
  integer; proposals should always state the real number. `score` is the
  Artificial Analysis Intelligence Index v5.0 (current range 1.0-27.0), not the
  older v4.3 scale.
- **note:** `scripts/verify_catalog.py` accepts `--catalog` for any JSON with
  `models`, so it can check tier files directly, but it needs network and the
  pinned llama.cpp tag; the offline checker is the fast gate.

## What I could not verify

- No network use was needed for this slice; nothing was fetched. Every rule is
  derived from `catalog.go`, `catalog.json` and `verify_catalog.py`.
- The checker cannot confirm that a proposed file exists or that `bytes` is the
  real size; that remains `scripts/verify_catalog.py` (HEAD + GGUF headers).
- I only saw the four tier files that existed at 03:35. The other seven tier
  agents' files were not in this worktree, so the run above is a sample, not a
  full-assembly audit.

## Next steps

1. Re-run the validator over `agents/T02/tiers/tier-*.json` as the remaining
   tier agents land; fix the tier-12 id collisions and missing `native_1bit`.
2. Run it on `agents/T02/catalog-proposed.json` after a16 assembles, then
   `scripts/verify_catalog.py --catalog agents/T02/catalog-proposed.json` with
   network for the real file sizes and GGUF metadata.
3. Decide whether the shipped catalog's quant ordering should be normalized in
   a separate catalog change; the brief's rule is currently stricter than the
   data it ships with.

## Commit

See the commit on `agent/T02-a13` with message
`T02: document catalog schema and add strict proposal validator`.
