# assemble.py: merging the T02 tier proposals

`agents/T02/assemble.py` merges every `agents/T02/tiers/tier-*.json` into
`agents/T02/catalog-proposed.json`. The Lead runs it at consolidation, once the
tier agents have landed their files.

## Run it

```sh
cd ~/ezlm-agents/work/subs/T02
python3 agents/T02/assemble.py
```

It writes `agents/T02/catalog-proposed.json` as `{"version": 1, "models": [...]}`,
prints a summary table and exits 0. It works from any directory and uses the
Python 3 standard library only.

| flag | default | use |
|---|---|---|
| `--tiers DIR` | `agents/T02/tiers` | read another tier directory |
| `--out FILE` | `agents/T02/catalog-proposed.json` | write somewhere else |
| `--catalog FILE` | `internal/catalog/catalog.json` | file used for the existing-id check |
| `--dry-run` | off | print the summary, write nothing |
| `--strict` | off | exit 1 when there were warnings |

The script reads nothing but the tier files, the catalog file above (when it
exists) and its own output path. It never touches the network.

## What it does

1. Finds `tier-*.json` in the tier directory and orders them by tier label:
   4, 6, 8, 12, 16, 24, 32, 48, 64, 96, 128plus, then any other label
   alphabetically. The order is fixed, so the result does not depend on the
   file system.
2. Loads each file as `{"tier": "<label>", "models": [...]}`. A file with an
   empty `models` list is legal and reported as a warning.
3. Keeps the first entry for each id and drops the rest. "First" means the
   smallest tier that proposed the id, so a 4 GB pick is never replaced by a
   later 24 GB proposal of the same id.
4. Sorts the kept models by score, highest first, and by id when scores tie.
5. Writes the merged catalog with one space of indentation and a trailing
   newline, the same layout as `internal/catalog/catalog.json`. The write is
   atomic: a temporary file is renamed over the target, so a failed run cannot
   leave a half-written catalog.

Field order follows `catalog.json` (`id`, `name`, ..., `quants`). Fields the
script does not know are kept, after the known ones, in their original order,
so a newer field is never dropped. A quant list that arrives unsorted is sorted
by `bytes` in the merged file, and a warning names the model. Order inside
`quants` is cosmetic for the app: `internal/fit/fit.go:303` ranks every quant
on its own, it does not assume the list is sorted.

## What it prints

The summary has four parts, in this order:

1. A one-line count of tier files, models, quants and duplicate ids.
2. One row per tier file: tier label, file name, models contributed to the
   output, their quant count and their score range. A tier whose models were
   all duplicates contributes 0.
3. The merged models, ranked by score, with params, active params for MoE,
   the tier that contributed the entry, the quant count and the smallest quant.
   Sizes are GiB (MiB or KiB for small test entries).
4. The duplicate table (`id`, `kept in`, `also in`) and the warning list.

Example from the synthetic test below:

```
tier   file          models  quants  score range
8 GB   tier-8.json        2       3  4.2 - 6.4
12 GB  tier-12.json       1       1  20.1 - 20.1

rank  id                       score  params  active  tier   quants  smallest quant
   1  qwen3.8-27b               20.1   27.4B       -  12 GB       1  Q4_K_M 15.8 GiB
   2  synthetic-qwen-test-8b     6.4    8.2B       -  8 GB        2  Q2_K 2.8 GiB

duplicates (first one kept, the rest dropped)
id                       kept in      also in
synthetic-gemma-test-4b  tier-8.json  tier-12.json
```

## Warnings and exit codes

Warnings never stop the merge. They cover:

- a tier file that is not valid JSON, is not an object, or has no `models` list
  (skipped, the rest still merge);
- a file with no `tier` label (the file name is used);
- an entry without an id (skipped, it cannot be deduplicated);
- an entry without a name, score source or quants, a quant without a name,
  file list or byte size, and a quant list that had to be sorted;
- an id that already exists in `internal/catalog/catalog.json`;
- no tier files at all.

Exit codes: `0` when the merged file was written, `1` when it could not be
written or when `--strict` was given and there were warnings. The output file
is always rewritten from scratch, so an empty tier directory produces an empty
`{"version": 1, "models": []}`, not a stale merge.

## Limits

- It does not check GGUF headers, file sizes or download URLs. That is
  `scripts/verify_catalog.py` (network, per quant) and a13's
  `validate_proposal.py` (field types and ranges).
- It does not judge scores, score sources or benchmark claims. It only flags
  the fields that are missing.
- It does not merge the per-tier `.md` files, and it does not touch
  `internal/catalog/catalog.json`.
- Deduplication is by id only. Two entries for the same repo under different
  ids both survive; a12 (retirement) and a13 (schema audit) cover that.
- An id that collides with the shipped catalog is a warning, not an error,
  because the Lead may rename it during consolidation.
- A broken tier file is skipped so a partially landed set still produces a
  usable file. Re-run once the file is fixed.

## Test evidence

All of this ran in the a16 worktree on 2026-11-06, with synthetic tier files
that were deleted afterwards. The only files left in `agents/T02/tiers` are
`.gitkeep`; the committed `catalog-proposed.json` is therefore the empty merge
from the last run and the Lead's run replaces it.

1. No tier files: exit 0, warning `no tier-*.json files in agents/T02/tiers`,
   output `{"version": 1, "models": []}`.
2. Two synthetic tiers, `tier-8.json` (two models, one with quants out of
   order) and `tier-12.json` (a duplicate id and an id that exists in the
   shipped catalog), plus `tier-16.json` with broken JSON and `tier-24.json`
   with an id-less entry. The run merged 3 models, 4 quants, dropped 1
   duplicate, reported the 4 expected warnings and exited 0. The duplicate was
   kept from `tier-8.json` (the smaller tier) even though `tier-12.json` gave
   it a score of 99.0.
3. The same input twice: identical output bytes and identical table, so the
   merge is deterministic.
4. `--dry-run`: the summary printed and the output file was not created.
   `--strict` with warnings: exit 1. Unwritable `--out`: exit 1 with
   `error: cannot write ...`, no traceback.
5. Real entry: one shipped catalog model (`qwen3.8-27b`) copied into a
   temporary `tier-32.json`, merged to a temporary output, then checked with
   the repo's own validator:

   ```sh
   python3 scripts/verify_catalog.py --catalog /tmp/.../catalog-proposed.json --only qwen3.8-27b
   # llama.cpp b11960: 166 architectures, 44 tensor types; checking 19 files sets
   # ... 19 x ok ...
   # 0 file set(s) with errors
   ```

   All 19 quant sets passed, so the merged shape and the copied entries are
   accepted by `verify_catalog.py`.

## For the Lead at consolidation

```sh
cd ~/ezlm-agents/work/subs/T02
python3 agents/T02/assemble.py
python3 scripts/verify_catalog.py --catalog agents/T02/catalog-proposed.json
```

Read the duplicate table: it shows which tier won each shared id. Read the
warnings before merging into `internal/catalog/catalog.json`. If a13's
`validate_proposal.py` exists by then, run it on the merged file as well.
