# T12 support report (a16)

Report-only. This file is the consolidation aid for the Lead: how many rows to expect per owner
task, which strings are worst, and the rewrites that can be applied in one pass after the merge.
It changes no code and no docs.

## Method and counts

Snapshot at 10:41, 13 of 15 row agents had committed: **288 rows, 242 unique strings**
(`file`, `line`, `current_text`). a10 (voice sweep) and a11 (merge) had not landed yet, so the
final `strings.csv` will be larger. Counts below are the real rows, not estimates.

| owner_task | rows | share |
|---|---:|---:|
| frozen | 94 | 33% |
| T15 | 51 | 18% |
| T16 | 39 | 14% |
| T07 | 24 | 8% |
| T06 | 20 | 7% |
| T09 | 16 | 6% |
| T10 | 11 | 4% |
| T13 | 10 | 3% |
| T05 | 8 | 3% |
| T11 | 7 | 2% |
| T08 | 4 | 1% |
| T04 | 2 | 1% |
| T01 | 2 | 1% |

Severity: 179 minor, 109 major, 0 blocker.

Rows per agent: a01 39, a12 32, a05 31, a02 29, a13 26, a06 25, a08 24, a07 24, a03 16,
a15 13, a14 11, a04 10, a09 8.

Files with the most rows: `internal/app/app.go` 32, `internal/engine/engine.go` 20,
`internal/serve/client.go` 17, `internal/engine/download.go` 16, `windows/EZLM/Chat.cs` 14,
`windows/EZLM/SelfTest.cs` 11, `windows/EZLM/SettingsPane.cs` 10, `windows/EZLM/Shell.cs` 10,
`cmd/ezlm/main.go` 10, `BETA.md` 10.

The wide sweep I ran first (2613 candidate literals, 1780 in frozen files) shows the rows are a
curated subset. Frozen files carry a third of the rows, so the Lead applies a third of the patch
directly.

### Duplicates and conflicts (blocker for a clean merge)

46 of the 288 rows repeat a `(file, line, current_text)` that another agent already reported
(36 duplicate groups). **Every one of the 36 groups has two or three different proposed texts.**
Examples: `internal/serve/client.go:185` has three proposals, `internal/engine/engine.go:189` has
three, and `windows/EZLM/Chat.cs:673` has two. a11's dedupe will silently keep one of them.

Recommended merge rule: dedupe by `(file, line, current_text)`, keep the proposal with the highest
severity, and write the rejected variants to `agents/T12/conflicts.md` so the Lead can choose. The
one-pass table below is my tie-break for the lines it covers; it keeps every format argument and
uses one sentence per action.

Open question for the Lead: C# and Swift files outside the frozen list are not named in any
`Owns:` line. Rows use T15 (Windows) and T16 (Mac), which is what I assumed; installer rows use
T14 and `BETA.md` uses T13. Normalise owner labels once, in `strings.csv`, before patching.

## Worst offenders

Ordered by user impact. Line numbers are from the snapshot in this worktree.

1. **Raw exception text reaches the UI** (major, T15 and T16). `windows/EZLM/Chat.cs:673` shows
   `"Something went wrong: " + ex.Message`; `macos/Sources/EZLM/ChatModel.swift:309` shows
   `"Something went wrong: \(error.localizedDescription)"`. Users get `Object reference not set...`
   or a Swift type name. The same pattern appears in toasts: `Chat.cs:1052`,
   `SettingsPane.cs:216`, `ReportLaunch.cs:129`, `ReportLaunch.cs:206`,
   `macos/Sources/EZLM/ReportLaunch.swift:252`.
2. **Bare state words** (major, frozen). `windows/EZLM/Panes.cs:677` falls back to `"Failed"`;
   `macos/Sources/EZLM/AppModel.swift:948` falls back to `"Failed"`. No next action, and the two
   apps agree on the bad wording.
3. **Shell commands as the error** (major, T10). `internal/serve/service_darwin.go:44`
   returns `launchctl bootstrap: %s`; `internal/serve/service_linux.go:20` returns
   `systemctl %s: %s`. These reach the app's error area with no plain explanation and no next step.
4. **Engine jargon with no action** (major, frozen). `internal/serve/props.go:38` returns
   `"/props answered %s"`. A user cannot know what `/props` is or what to do.
5. **Developer wording in a user path** (major, frozen). `internal/app/app.go:1136`
   `"unknown model %q"` and `app.go:1168` `"%s has no file %q"`. `%q` prints quotes, and "file"
   means a quantisation artefact.
6. **Wrapped raw errors** (major, T07/frozen). `internal/serve/config.go:69`
   `"EZLM's settings file (%s) is damaged: %v"`, `internal/app/settings.go:264`
   `"can't use %s: %v"` and `:268` `"EZLM can't save files in %s: %v"` append the Go error with
   no recovery step.
7. **Test-request failures are unnamed** (minor, T07/frozen). `internal/serve/client.go:185`
   `"test request failed: %s"`, `:202` `"test request returned no answer"`, `:208`
   `"the model answered with nothing"`.
8. **Duplicate rows with conflicting proposals** (blocker for consolidation). See the section
   above: 46 duplicate rows, 36 groups, every group split on wording. This is the one thing that
   must be resolved before a one-pass patch, or the Lead will apply inconsistent text.

Good news for parity: `Chat.cs:671` and `ChatModel.swift:307` already use the same sentence
("Couldn't reach your model. It may have stopped: check Overview."), and the voice sweep found no
em dashes, emoji or exclamation marks in user-facing strings (the only `—` is a comment in
`Models.swift:112`; the only `!` is test data in `SelfTest.cs:78`).

## One-pass rewrites, grouped by file

Apply in this order. This table is the tie-break where rows disagree; every entry keeps the
existing format arguments unless noted, so each patch is a literal replacement. "frozen" means the
Lead applies it directly, not a task branch.

### `internal/web/server.go` (T07)

| line | current | proposed |
|---|---|---|
| 235 | `forbidden` | `EZLM only answers requests from this computer. Open EZLM from the address it shows.` |
| 255 | `this page is out of date: reopen EZLM` | `This page is out of date. Close it and open EZLM again.` |
| 278 | `streaming unsupported` | `Live updates are not available in this browser. Reload the page.` |

### `internal/prereq/prereq.go` (T11)

| line | current | proposed |
|---|---|---|
| 98 | `EZLM will install it from Microsoft (Windows asks for permission once).` | `EZLM will download it from Microsoft and install it. Windows asks for permission once.` |

### `internal/serve/props.go` (frozen)

| line | current | proposed |
|---|---|---|
| 38 | `"/props answered %s"` | `"The model server did not answer the status check (%s). Press Start server on Overview to try again."` |

### `internal/serve/client.go` (T07)

| line | current | proposed |
|---|---|---|
| 185 | `"test request failed: %s"` | `"The test message failed (%s). Press Start server on Overview to try again."` |
| 202 | `"test request returned no answer"` | `"The model server did not answer the test message. Press Start server on Overview to try again."` |
| 208 | `"the model answered with nothing"` | `"The model replied with an empty message. Try again."` |

### `internal/serve/config.go` (T07)

| line | current | proposed |
|---|---|---|
| 69 | `"EZLM's settings file (%s) is damaged: %v"` | `"EZLM's settings file (%s) is damaged (%v). Rename it, then reopen EZLM."` |

### `internal/serve/service_windows.go`, `service_darwin.go`, `service_linux.go` (T10)

| file:line | current | proposed |
|---|---|---|
| `service_windows.go:183` | `"couldn't open the server log: %v"` | `"EZLM could not open the server log (%v); check that the models folder is writable, then press Start server."` |
| `service_windows.go:200` | `"couldn't start the server: %v"` | `"EZLM could not start the model server (%v); press Start server on Overview to try again."` |
| `service_darwin.go:44` | `"launchctl bootstrap: %s"` | `"EZLM could not start the background service (%s); restart your Mac, then press Start server."` |
| `service_linux.go:20` | `"systemctl %s: %s"` | `"EZLM could not start the background service (%s); restart this computer, then press Start server."` |

### `internal/app/app.go` (frozen)

| line | current | proposed |
|---|---|---|
| 1136 | `"unknown model %q"` | `"EZLM does not know the model %q. Pick a model from the Models list."` |
| 1168 | `"%s has no file %q"` | `"%s is missing its %s model file. Press Set it up again to download it."` |

`app.go:1168` changes `%q` to `%s`; the argument order (`m.Name`, `quant`) is unchanged.

### `internal/app/settings.go` (frozen)

| line | current | proposed |
|---|---|---|
| 264 | `"can't use %s: %v"` | `"EZLM can't use %s (%v). Pick another folder."` |
| 268 | `"EZLM can't save files in %s: %v"` | `"EZLM can't save files in %s (%v). Pick another folder."` |

### `windows/EZLM/Chat.cs` (T15) and `macos/Sources/EZLM/ChatModel.swift` (T16)

| file:line | current | proposed |
|---|---|---|
| `Chat.cs:673` | `"Something went wrong: " + ex.Message` | `"Something went wrong. Try again, or press Report a problem to send the details."` |
| `ChatModel.swift:309` | `"Something went wrong: \(error.localizedDescription)"` | `"Something went wrong. Try again, or press Report a problem to send the details."` |

The log keeps the exception; only the visible sentence changes.

### `windows/EZLM/Panes.cs` (frozen) and `macos/Sources/EZLM/AppModel.swift` (frozen)

| file:line | current | proposed |
|---|---|---|
| `Panes.cs:677` | `e.Msg ?? "Failed"` | `e.Msg ?? "Setup failed. Try again."` |
| `AppModel.swift:948` | `e.msg ?? "Failed"` | `e.msg ?? "Setup failed. Try again."` |

### `windows/EZLM/ReportLaunch.cs` (frozen) and `macos/Sources/EZLM/ReportLaunch.swift` (frozen)

| file:line | current | proposed |
|---|---|---|
| `ReportLaunch.cs:129` | `"Couldn't read the log: " + ex.Message` | `"Couldn't read the log. Try again, or copy the details for support."` |
| `ReportLaunch.cs:206` | `"Couldn't send the report: " + ex.Message` | `"Couldn't send the report. Check your connection and try again."` |
| `ReportLaunch.swift:252` | `"Couldn't send the report: \(error.localizedDescription)"` | `"Couldn't send the report. Check your connection and try again."` |
| `ReportLaunch.swift:46` | `"Cancelled."` | `"Cancelled. Choose the app again when you are ready."` |

## After the merge

1. Run a11's merge, then check the row count and the malformed rows:

   ```sh
   python3 agents/T12/merge_rows.py
   python3 - <<'PY'
   import csv
   rows = list(csv.DictReader(open("agents/T12/strings.csv", newline="")))
   print(len(rows), "rows")
   from collections import Counter
   print(Counter(r["owner_task"] for r in rows))
   print(Counter(r["severity"] for r in rows))
   PY
   ```

2. Resolve the 36 conflict groups before applying anything. `a04.csv` was malformed when I first
   sampled it (10 rows, widths 7 and 9, unescaped inner quotes at `server.go:255`) and a04 fixed it
   within two minutes; check widths again after the merge in case another file regressed.
3. Apply the rewrites above file by file, in the order listed. Frozen files first, because they do
   not wait on a branch merge.
4. Re-run the a10 voice sweep after patching: the replacements above contain no em dash, emoji or
   exclamation mark.

## Open questions

- Should `owner_task` for non-frozen C#/Swift be `T15`/`T16` (my choice), `unowned`, or the
  Lead's own name? The a04 rows already use `T11` for `internal/prereq/prereq.go`, which T11 owns
  only for tests, so some inconsistency is already in the set.
- `internal/bench/` and `internal/catalog/catalog.go` have no task that owns their source; I
  labelled catalog as T01 and left bench out of the tables. Confirm before applying.
- `demo.go` (83 strings) is demo-mode copy that a real user can see. It is frozen and no
  assignment covers it. It needs an owner before the one-pass patch.
