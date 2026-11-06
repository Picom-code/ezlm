# T12 / a11 report: row merge tooling

**Slice:** tooling. One deliverable of code: `agents/T12/merge_rows.py`. Report only, no
EZLM source touched.

## What it does

`merge_rows.py` reads every `agents/T12/rows/*.csv`, merges them, and writes
`agents/T12/strings.csv` with exactly the contract columns:

```
file,line,current_text,problem,proposed_text,owner_task,severity
```

Run it from anywhere; paths default to the directory the script lives in:

```sh
python3 agents/T12/merge_rows.py
```

Options: `--rows DIR`, `--out FILE`, `--strict` (exit 2 if anything was skipped or any
duplicate conflicts), `--show-conflicts N` (default 10).

Output on stdout, one line per owner task, then per severity:

```
merged 214 rows from 15 files -> agents/T12/strings.csv
dropped 6 duplicate rows, skipped 0 malformed
per owner task:
  T01    42
  ...
  frozen  8
per severity: major 96, minor 88, blocker 30
```

Exit codes: `0` ok, `1` no rows directory or no CSVs, `2` `--strict` found problems.

## Decisions

- **Dedupe key** is `(file, line, current_text)` as the brief says, with whitespace
  collapsed and case folded, so `"Failed to start,  check logs"` and `"Failed to
  start, check logs"` are one row. Paths are normalised (`.\` to `/`, leading `./`
  dropped).
- **First file in name order wins.** `rows/` is read sorted, so a01 beats a10. Empty
  fields on the winner are filled from later copies; a later copy that disagrees on
  `problem`, `proposed_text`, `owner_task` or `severity` is reported as a CONFLICT and
  the first copy is kept. That way one agent's stray severity does not silently
  overwrite another's.
- **Sort** is by file (case-insensitive), then the first number in `line`, then the
  text. `line` may be `42`, `42-45`, `42,44` or empty; empty sorts first, ranges keep
  their own order.
- **Tolerant input**, because 16 people are writing CSVs by hand: UTF-8 BOM, CRLF,
  missing header (assumed documented column order), header aliases (`path`, `text`,
  `proposed`, `owner`, ...), stray quotes around cells, short and blank rows. Bad rows
  are skipped, counted and named on stderr, never fatal.
- **Deterministic**: same inputs give a byte-identical `strings.csv`, so the Lead can
  re-run it safely and diff the result.
- Unknown severities are kept and counted (`urgent` is not one of the three values but
  dropping the row would lose the finding).

## Validation

Ran against fixture row sets in a temp directory, not committed:

- two agents reporting the same string at the same line: one row out, duplicate counted;
- one agent's `""` empty fields versus another's filled fields: filled, no conflict;
- real disagreements on `proposed_text` and `severity`: CONFLICT lines, first kept;
- BOM + CRLF file, headerless file, 3-column short row, blank row: all handled;
- missing `rows/` -> rc 1; empty `rows/` -> header-only file, rc 1;
- `--strict` with a conflict -> rc 2, clean input -> rc 0;
- two runs into different files -> byte-identical;
- default paths work when invoked from `/` with an absolute script path.

## Open questions

- Should `strings.csv` be committed by the Lead after the merge, or stay a generated
  file? It is not on my branch, since at my commit time `rows/` was still empty and a
  header-only file would be misleading.
- Do we want the Lead's run to fail hard on any conflict (`--strict`) or just print
  them? Default is print and exit 0, so one bad row does not block consolidation.
- Owner tasks must match the names in `TASKS.md` Owns lists. The script does not
  validate them; the per-owner counts are the check.

## Next steps

- Lead: run `python3 agents/T12/merge_rows.py` after the 16 branches are checked out,
  then apply rewrites grouped by file (a16's report groups them).
