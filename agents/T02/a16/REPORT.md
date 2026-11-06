# T02 a16: assembly script

Agent a16, cross-cutting job: merge the tier proposals into one catalog-shaped
file. Deliverables are `agents/T02/assemble.py`, `agents/T02/assemble-notes.md`
and this report. No Go file was touched.

## What I did

- `agents/T02/assemble.py` reads every `agents/T02/tiers/tier-*.json`, keeps
  the first entry for each id, sorts by score, writes
  `agents/T02/catalog-proposed.json` as `{"version": 1, "models": [...]}` in the
  layout of `internal/catalog/catalog.json`, and prints a summary table plus a
  duplicate table and a warning list.
- The merge is deterministic: tier files are read in tier order (4 to
  128plus), ties in score break on id, and the output is written atomically.
  Unknown fields are kept, known fields are put back in catalog order.
- The script has to run on whatever exists. Missing directory, zero files,
  empty `models`, broken JSON, a non-object top level, entries without an id,
  missing quants or a quant list out of size order are all handled without a
  traceback. Warnings do not fail the run; `--strict` makes them exit 1.
- `agents/T02/assemble-notes.md` documents the behaviour, the flags, the exit
  codes, the limits and the test evidence, and gives the Lead the two commands
  to run at consolidation.
- `agents/T02/catalog-proposed.json` is committed as the empty merge from the
  last local run, because no tier file existed yet. The Lead's run replaces
  it. `agents/T02/tiers/.gitkeep` keeps the directory in the merged tree.

## Findings

1. **The shipped catalog does not keep `quants` sorted by size.** 13 of the 32
   models in `internal/catalog/catalog.json` have a quant list whose `bytes`
   are not ascending, for example `qwen3.8-27b` (GSQ-RCO-IQ2_XS 8.42 GB,
   GSQ-RCO-IQ2_XS-mtp 8.77 GB, ..., UD-IQ2_XXS 7.27 GB at index 9) and
   `qwen3.6-35b-a3b` (MTP variants appended after larger files). The T02 brief
   requires proposals to be sorted by size, so a validator written to that rule
   (a13's `validate_proposal.py`) would reject the shipped catalog itself if it
   were pointed at it. Severity: **minor**. Order is cosmetic for planning:
   `internal/fit/fit.go:303` iterates and ranks every quant, it does not assume
   the list is sorted. `assemble.py` sorts an unsorted list and says so in a
   warning; the Lead can decide whether to normalise `catalog.json` later.
2. **An empty proposal file cannot be loaded by the app.** `parse` in
   `internal/catalog/catalog.go:190` rejects a catalog with no models
   (`catalog.go:196`). So `EZLM_CATALOG=agents/T02/catalog-proposed.json` only
   works after a real merge, not on the committed empty file. Severity:
   **minor**, informational for the Lead.
3. **`scripts/verify_catalog.py` checks files, not fields.** It reads
   `cat["models"]` (`verify_catalog.py:220`) and validates every quant against
   the real GGUF headers, but it does not check duplicate ids, required fields
   or score sources. That is why `assemble.py` reports duplicates and missing
   fields itself, and why a13's validator is still needed on the merged file.
   Severity: informational.

## Evidence

- Synthetic run with two valid tiers, one duplicate id, one existing catalog
  id, one broken JSON file and one id-less entry: 3 models merged, 1 duplicate
  dropped (kept from the smaller tier), 4 warnings, exit 0.
- Two consecutive runs on the same input produced identical output bytes and
  identical stdout, so the merge is deterministic.
- `--dry-run` wrote nothing, `--strict` exited 1 with warnings, an unwritable
  `--out` exited 1 with a one-line error.
- Real entry: `qwen3.8-27b` copied into a temporary tier file, merged, then
  `python3 scripts/verify_catalog.py --catalog <merged> --only qwen3.8-27b`
  reported all 19 quant sets `ok` and `0 file set(s) with errors` (llama.cpp
  tag b11960, run 2026-11-06).
- Full commands and outputs are in `agents/T02/assemble-notes.md`.

## What I changed and why

Only new files under `agents/T02/`: `assemble.py`, `assemble-notes.md`,
`catalog-proposed.json` (empty merge), `tiers/.gitkeep`, and this report. No
existing file was edited, so the morning merge cannot conflict with T02 tier
agents on anything but the generated catalog file, which only this branch
writes.

## What I could not finish or verify

- There were no tier files in any worktree when I ran, so the committed
  `catalog-proposed.json` is empty. The real merge happens when the Lead runs
  the script at consolidation. This is expected, not a blocker.
- a13's `validate_proposal.py` does not exist on `main` yet, so I could not
  run the merged file through it. `assemble.py` and the validator should agree
  on field rules; if they do not, a13's file wins because it is the documented
  rule set.
- I did not verify any model data. That is the tier agents' job and
  `verify_catalog.py`'s job.

## Next steps

1. Lead: `python3 agents/T02/assemble.py`, then
   `python3 scripts/verify_catalog.py --catalog agents/T02/catalog-proposed.json`,
   then read the duplicate and warning sections before merging anything into
   `internal/catalog/catalog.json`.
2. Re-run the script after any tier file is fixed. It is idempotent.
3. Optional: if the Lead wants id collisions with the shipped catalog to be
   fatal, add `--strict` to the consolidation command; that also makes missing
   fields fail the run.
4. Optional follow-up for T02 or a later task: normalise the `quants` order in
   `internal/catalog/catalog.json` so the shipped catalog follows the same rule
   the proposals are held to (finding 1).
