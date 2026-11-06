# T13 a14 report: unclear messages

**Deliverable:** `agents/T13/a14/unclear-messages.md`, the list of messages that are too vague to
document, for T12. 32 numbered entries (25 tier 1, 7 tier 2), plus 6 checked-and-clear examples,
5 cross-cutting patterns, and the method.

## What I did

1. Read `README.md`, `run/T13-brief.md`, the T13 section of `TASKS.md`, the T12 section (to avoid
   duplicating the string audit), and the root `BETA.md` for the voice the guide uses.
2. Surveyed every message source: `errors.New` and `fmt.Errorf` under `internal/` and `cmd/ezlm`
   (224 sites, read-only), message fields that reach the API (`Msg`, `StartError`, `Detail`, `Why`,
   `update.Info.Error`, `prereq.Check.Detail`, `bench.Result.Detail`), the C# strings in
   `windows/EZLM`, and the Swift strings in `macos/Sources/EZLM`.
3. For each candidate, traced the string to the place a tester sees it (toast, error screen, server
   panel, self-test row, Settings line, app tile) and cited both the build site and the display site.
4. Applied the test "can a09 to a12 fill in what it means and what to do from the message alone?"
   Kept only the ones where the answer is no. Recorded near misses in the checked-and-clear section
   so the next reader does not re-litigate them.

No code was changed. Docs only, one file plus this report.

## What I found

- The largest cluster is **HTTP status codes appended to sentences** (9 sites), followed by **raw Go
  errors after a colon** (4+ sites). Both are mechanical to fix once, and both leave the tester
  without a next action.
- **Three update failures** share one Settings line and none of them says what to do
  (`internal/update/update.go:68`, `:73`, `:90`). This is the most likely first-run complaint after
  a network hiccup.
- **The self-test detail column can show a raw Go network error** (`internal/bench/bench.go:105` and
  the other four tests). That string includes a URL and a socket address.
- **Two "no answer" states are worded so similarly** (`internal/serve/client.go:202`, `:208`) that a
  tester cannot tell them apart, though the causes differ.
- **Windows and Mac drift on the same failure:** "Couldn't open EZLM." against "Couldn't open it."
  (`ReportLaunch.cs:92`, `ReportLaunch.swift:46`).
- The apps' own messages are mostly good. Only the fallbacks that fire when the engine sends an
  empty reason are vague (#2 to #5).

## What I could not verify

- I did not build or run either app, so I could not watch the screens. Every entry cites the display
  site, but a string that is built and never shown could still be listed. The parent or a16 should
  spot-check the app-tile entries (#2, #3) on a real screen.
- I did not audit chat, memory, installer or uninstall strings; they are other agents' slices.
- I did not check the `.iss` installer text (not present in this worktree).
- Line numbers are from `agent/T13-a14` at the `main` snapshot. Other agents' branches will shift
  them; the quoted text is the stable key.

## Open questions

1. `internal/app`, `internal/bench` and `internal/integrate` are owned by no T01 to T16 task tonight
   (T07 and T09 read `internal/app`). Entries #9 to #13 and #20 to #25 therefore have no owner for
   the rewrite. Should T12 own them, or should they wait for the morning?
2. Should the CLI messages (#31, #32) be in scope for a guide aimed at non-technical testers? I
   included them because the brief says "every message a tester can see", but they are lower value.
3. T12's stretch goal overlaps this list (engine errors with no plain explanation). The Lead may want
   to hand T12 this file directly instead of merging two lists.

## Next steps

- Lead: route the unowned-file entries (question 1) before assembly, then pass the file to T12.
- a09 to a12: use the entries that fall in your packages; the "proposed wording" column is ready to
  copy, and the "checked and clear" list says which short messages are safe to document as they are.
- a16: check the Windows/Mac drift items (#3, pattern 4) against the final sections for
  contradictions.

## Commit

`T13: list the messages too vague to document (feeds T12)`
