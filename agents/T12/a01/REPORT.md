# T12 a01 report: Go user-facing errors in `internal/app/` and `cmd/ezlm/`

Report-only. No code or docs were changed. All 39 rows are in
`agents/T12/rows/a01.csv`.

## What I scanned

- `internal/app/`: `app.go`, `connect.go`, `demo.go`, `demo_apps.go`, `launch.go`,
  `live.go`, `settings.go` (non-test files, about 3,800 lines).
- `cmd/ezlm/`: `main.go`, `debugstacks.go` (about 735 lines).

I read every `errors.New`, `fmt.Errorf`, `Event{State: "fail"}` and
`step(..., "fail", ...)` value, plus the CLI output that a user sees when a
command fails (usage lines, `status`, `models`, `test`, `update`, `scan`).
I also followed the path into `internal/web/server.go`, which returns
`{"error": err.Error()}` to the page, so every returned error here is shown
verbatim in the Windows, Mac and web UI. About 120 user-visible strings were
read; 39 were not already correct enough to leave alone.

Rows by severity: **11 major, 28 minor, 0 blocker**. Rows by file:
`internal/app/app.go` 29, `internal/app/demo.go` 1, `cmd/ezlm/main.go` 9.

## Owner task

Every file in this slice is in the README frozen list (`internal/app/**`,
`cmd/**`), so `owner_task` is `frozen` on every row. These rewrites are for
the T12 consolidation pass; they cannot be patched by a task owner tonight
without colliding with the rewrites landing in those trees.

## Worst problems

1. **Dead ends with a raw id and no next action.** `internal/app/app.go:635`
   (`model %q doesn't fit this machine`), `:1136` (`unknown model %q`),
   `:1168` (`%s has no file %q`) and `cmd/ezlm/main.go:415`
   (`no model fits this machine right now`) tell a terminal user that
   something is wrong but never point at `ezlm models` or at closing apps.
   These are the failures a beta tester is most likely to hit first.
2. **`port %d is in use by another program` (`internal/app/app.go:1511`).**
   The server has already been stopped when this returns, so the user is left
   with a stopped server and no instruction. Proposed text names the next
   port action.
3. **One state, two names.** `nothing is deployed yet` (app.go 993, 1019,
   1266, 1300; cmd 268, 521, 591) and `nothing is set up yet` (app.go 1390,
   1499) describe the same condition, and neither says what to do. This is
   the consistency problem the task exists to catch; proposed text is
   `Set up a model first.` everywhere, matching `connect.go:44` and
   `launch.go:31`.
4. **`EZLM is already working on something` (`app.go:574`).** This is what a
   user sees when they press a second action. "Something" gives them no idea
   whether to wait, cancel, or where progress is shown.
5. **Disk-full error starts lowercase and runs three clauses**
   (`app.go:744`). It is the message a user sees at the worst moment, after a
   long download, and it reads like a log line.
6. **Voice slips.** `Heads up:` (`app.go:735`) is the only casual opener in
   the slice; `app.go:785` and `:1541` join sentences with semicolons and
   leave raw `%v` errors in brackets.

## Method and conventions

- `current_text` is the exact message text (without Go quotes) and every
  `%s` / `%q` / `%.0f` verb is preserved so a rewrite can be applied without
  changing the arguments. A script checked all 39 (file, line, text) triples
  against the source; all matched.
- `line` is the line of the string literal in the worktree at
  `agent/T12-a01`.
- Proposed text follows the README voice rules: one sentence, neutral, no em
  dash, no emoji, no exclamation mark, and a named next action. Where a
  message has to carry two facts I kept one sentence and used a semicolon
  rather than adding a second sentence.
- I did not rewrite strings that are already plain and actionable, for
  example `that's the model you're running; switch to another one first`
  (`app.go:1178`), `pick a port between 1024 and 65535` (`app.go:1502`) and
  the `serverReason()` causes (`app.go:1556-1560`).

## Open questions

- Are CLI usage lines in scope for "user-facing errors"? I included them
  where a command fails and prints one, but a reviewer may prefer to drop the
  five `usage:` rows if a10/a12 own that voice pass.
- The frozen marker means these rows may never be applied. If the Lead wants
  them applied, they need an owner for `internal/app/**` and `cmd/**` after
  the merge; the README currently says patches there are thrown away.
- `internal/app/demo.go` and `demo_apps.go` are demo-only text. I included
  only `demo.go:195` (`no model fits`), because the demo is what reviewers
  see, and left the rest of the demo details alone since they are already
  plain and actionable.
- Error text produced by other packages (`prereq.Check.Action`,
  `integrate.Target.Detail`, `serve` errors) is passed through by
  `internal/app` and is not judged here; those belong to a02/a04.
- Several proposed messages mention `ezlm models` / `ezlm key list`. These
  are the real command names in `usage()`; confirm they stay stable before
  applying.
