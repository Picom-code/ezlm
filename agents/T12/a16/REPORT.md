# T12 a16 report (report support)

Report-only. No code, no docs, no rows CSV: a16's deliverable is the consolidation support file
`agents/T12/a16/REPORT-support.md`.

## What I did

- Read `README.md`, the T12 brief and the T12 section of `TASKS.md`.
- Swept every non-test Go, C# and Swift file under `internal/`, `cmd/`, `windows/EZLM/` and
  `macos/Sources/EZLM/` for user-facing string literals: 2613 candidates, 1780 of them in frozen
  files. This is the coverage estimate, not the audit itself.
- Collected the row CSVs from the other agents as they landed (13 of 15 at the end) and aggregated
  them: 288 rows, 242 unique strings, 179 minor, 109 major, 0 blocker.
- Built the per-owner count table, the worst-offender list, and the one-pass rewrite list grouped
  by file in `REPORT-support.md`.
- Checked CSV validity and duplicate behaviour across agents.

## Worst problems found

1. **46 duplicate rows in 36 groups, and every group has conflicting proposals.** a11's merge
   dedupes silently, so the Lead can apply the wrong wording. Needs a conflict log and a severity
   tie-break rule.
2. **Raw exception text reaches the UI** in `Chat.cs:673`, `ChatModel.swift:309` and the report
   toasts (`ReportLaunch.cs:129`, `ReportLaunch.cs:206`, `ReportLaunch.swift:252`).
3. **Bare `"Failed"` fallbacks** in `Panes.cs:677` and `AppModel.swift:948`.
4. **Shell commands as errors**: `launchctl bootstrap: %s` and `systemctl %s: %s` in
   `internal/serve/service_*.go`.
5. **Engine jargon**: `"/props answered %s"` (`props.go:38`), `"unknown model %q"`
   (`app.go:1136`), `"%s has no file %q"` (`app.go:1168`).

The voice sweep was otherwise clean: no em dashes, emoji or exclamation marks in user-facing
strings. Windows/Mac parity is already good on the chat error path
(`Chat.cs:671` and `ChatModel.swift:307` are identical).

## What I could not finish

- a10 and a11 had not committed when I aggregated, so the counts exclude the voice sweep and the
  final merge. The table will need a refresh from `agents/T12/strings.csv`.
- Owner labels for non-frozen C#/Swift are an assumption (T15/T16); no `Owns:` line names them.
- `internal/app/demo.go` (83 strings) and `internal/bench` have no owning task. Recorded as open
  questions in `REPORT-support.md`.

## Next steps

1. a11 merges with `csv.DictReader`, validates width 7, and writes the 36 conflicts out.
2. Lead picks a proposal per conflict (the a16 table is a tie-break for the lines it covers).
3. Apply the frozen-file rewrites first, then the T07/T10 rows, then the app rows.

## Commit

`T12: report support counts, worst offenders and one-pass rewrites` on `agent/T12-a16`.
