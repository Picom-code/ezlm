# T12. User-facing text audit — consolidated report

16 agents (a01–a16) worked this task in parallel: nine source areas (Go, C#, Swift), a Windows/Mac
parity pass, a voice-rule sweep, messages with no next action, engine errors with no plain
explanation, installer and self-test strings, the docs, the merge tooling, and report support.
Consolidated onto `agent/T12-text-audit` by the Lead.

**This task is report-only** — no code or docs were changed, as the brief requires, because the
string owners are the other tasks tonight.

## Result

- **`agents/T12/strings.csv` — 280 unique strings** with file, line, current text, problem,
  proposed text, owner task and severity: **89 major, 191 minor, 0 blocker**.
- Grouped by owner: **frozen 91** (the README's frozen files), **T15 59**, **T16 52**, T09 15,
  T07 15, T13 11, T06 10, T05 8, T11 7, T08 4, T10 4, T01 2, T04 2.
- `agents/T12/conflicts.txt` — 160 cases where two agents proposed different wording for the same
  string (mostly C#/Swift overlapping scopes). The merge tool keeps the first and records the
  alternatives; the morning merge should pick one, not both.
- `agents/T12/a16/REPORT-support.md` — the one-pass rewrites grouped by file with exact
  current → proposed text.

## The findings that matter most

1. **Dead-end errors.** The largest class: an error that says what broke and offers no action.
   Windows and Mac both paste `ex.Message` or a raw engine line as the whole message. One shared
   "Try again, or report a problem" suffix plus a named control would fix the class.
2. **A raw llama.cpp log line as the user-facing error** (`internal/serve/client.go:81`, reaches the
   screen at `internal/app/app.go:842`, printed verbatim by both apps). `app.go:1552` already
   explains out-of-memory, missing model and port-in-use in plain words, but only on the Start path.
3. **A raw Go error appended to a plain sentence** in five download messages (`plainNetErr` uses
   `(%w)`), so users see `dial tcp … no such host` after an otherwise good sentence (T09).
4. **`internal/update` error strings start lowercase** and get appended after a period by the Windows
   Settings pane: "EZLM 0.1.1. couldn't reach GitHub…" (T08).
5. **The report-a-problem path — the last thing a beta tester touches — is the least clear text**:
   it never says to write what happened or that the post is public, and a failed send loses the
   typed report.
6. **`Chat.cs:774` leaks an identifier**: the `/help` toast shows the literal
   `ChatLogic.Commands:` because a `$` interpolation is missing (a real bug, not just wording).
7. **Voice rules**: 44 two-sentence errors/notices, 10 marketing words (including the
   "Smartest"/"Snappy replies first" quality presets in both apps and `BETA.md`), 2 exclamation
   marks, 2 emoji/glyph-in-prose, and **no em dashes** in user-facing text.
8. **Windows/Mac parity** is mostly good — most journey text is word-for-word identical. The
   divergences are small (button labels, "this Mac" vs "this computer", a missing Mac toast) plus
   three non-text gaps (the Mac wizard has no "Saves to"/Change row; the Windows update has no demo
   branch).
9. **Docs drift** (`BETA.md`): it says "or press Stop server", which wrongly implies autostart is
   off (the Run key survives), and it never mentions DeepSeek Harness, a shipped integration.

## Owner-label caveat

No task's **Owns** list names `windows/EZLM/*.cs`, the non-frozen Swift files, `internal/prereq`,
`internal/bench`, `BETA.md` or `internal/catalog/catalog.go`. The agents used **T15** for Windows
C#, **T16** for Swift, **T13** for `BETA.md` and T01 for `catalog.go` as the closest owners; the
morning merge should either confirm those or assign the rewrites directly.

## How to apply it

`agents/T12/strings.csv` is sorted by file then line, so each file's rewrites can be applied in one
pass. `agents/T12/a16/REPORT-support.md` groups them that way with the exact before/after text.
Two strings are load-bearing and must keep their phrasing because `internal/app` matches on them:
"the model server stopped while loading" and "did not finish loading".

## Files

- `agents/T12/strings.csv` — the deliverable (280 rows).
- `agents/T12/conflicts.txt` — the 160 wording conflicts.
- `agents/T12/rows/a01..a15.csv` — per-agent rows (the raw material).
- `agents/T12/merge_rows.py` — the merger (run with `--strict` to fail on conflicts).
- `agents/T12/a13/engine-errors.md`, `agents/T12/a16/REPORT-support.md`, and one report per agent.

## Next steps

1. Apply the 89 major rewrites, starting with the dead-end errors and the raw engine text.
2. Fix `Chat.cs:774` (missing interpolation) — that one is a bug.
3. Decide the quality-preset names; the Swift raw value is persisted, so rename the display title
   only.
4. Resolve the 160 conflicts with a single voice pass.
