# T12 a06 report: C# dialogs, errors, tray/taskbar, self-test

Slice: user-facing C# in `windows/EZLM/` that lives in dialogs, error paths,
tray/taskbar and self-test. Report only, no code changed.

## What I scanned

| File | What is user-facing there | Rows |
|---|---|---|
| `Engine.cs` | every message thrown from `StartAsync` and `Read<T>`, shown on the full-screen failure page | 4 |
| `Shell.cs` | `ShowFailed`, the engine-died message, the per-page crash banner, close prompts, `Dialog()`/`ShowDialog` helpers | 2 |
| `Install.cs` | the "Make room for this model?" dialog, the deploy error panel and its hint, cancel/switch toasts | 3 |
| `ModelMode.cs` | the whole model-mode dialog: intro, per-app question, force-close warning, summary | 3 |
| `ReportLaunch.cs` (frozen) | the app log dialog, the report-a-problem form, its toasts | 4 |
| `ChatStore.cs` | `ChatError`, the text shown under a failed chat answer | 2 |
| `Chat.cs` | the memory dialog text and the chat error/speech errors | 2 |
| `SettingsPane.cs` | the storage dialog, folder picker and update errors | 3 |
| `SelfTest.cs` (frozen) | check names and the pass/fail summary | 2 |
| `Taskbar.cs` | none: no string literals at all | 0 |
| `Program.cs`, `App.xaml.cs` | none user-facing (`--selftest` path writes to a file) | 0 |

25 rows in `agents/T12/rows/a06.csv`.

`owner_task` is `T15` for files no task owns tonight (T15 is the task that reads
`windows/EZLM/*.cs` and reports C# findings) and `frozen` for `ReportLaunch.cs`
and `SelfTest.cs`, which are in the README frozen list.

## Worst problems

1. **Errors that name no next action.** The largest group (13 of 25 rows).
   `Engine.cs:129` shows `Request failed (404)` on the full-screen failure page
   when the engine's JSON body has no `error` field, and `Engine.cs:76` blames
   the antivirus but never says what to do. `Install.cs:225` falls back to
   `Setup failed.` and `SettingsPane.cs:311` to `Couldn't start the installer: ...`
   with no way forward. Each has a proposed "do X" sentence in the CSV.
2. **Two-sentence errors joined by a semicolon.** `Shell.cs:185` is the worst:
   `... Your model server keeps running; Try again reconnects to it.` A capital
   letter after a semicolon reads as a typo, and the button label is folded into
   the sentence. Same pattern at `Engine.cs:75`, `Install.cs:161`,
   `ModelMode.cs:64` and `ChatStore.cs:254`.
3. **The report-a-problem form.** `ReportLaunch.cs:181` says
   `... Thanks for helping.` and never tells the user to write what happened or
   that the text is posted publicly. `ReportLaunch.cs:199` breaks the voice
   rules with `Sent. Thank you! (report #n)`, and `:206` loses the typed report
   on failure with no next action. This is the path a beta tester takes when
   everything else has failed, so it should be the clearest text in the app.

## Notes for the Lead

- **Tray/taskbar has no strings.** `Taskbar.cs` only draws progress and flashes
  the button; `Snap.cs` and `Program.cs` add no user text. There is no tray icon
  at all in the Windows app, so there is no tray menu to audit.
- **Raw engine text reaches the UI.** `note = ex.Message` (for example
  `Install.cs:118`, `Panes.cs:214`, `Shell.cs:495`) and
  `ChatStore.cs:255` show the engine's own wording verbatim. Rewriting the C#
  side cannot fix that; it needs the Go error text fixed (a01-a04) or a mapping
  layer. I did not add a row per call site, only for the two places with their
  own wording.
- **Self-test names are developer-facing but ship.** `SelfTest.cs:206/208` use
  `no exceptions` and `apps: no exceptions`, which read as fragments in the
  PASS/FAIL file. a14 owns the self-test output; expect a duplicate there.
- **Overlap with a05/a14.** `Chat.cs`, `SettingsPane.cs` and `SelfTest.cs` are
  also in a05's and a14's areas. Rows are deduped by (file, line, current_text),
  so duplicates collapse; the problem/proposed text may differ slightly and the
  first file to be read wins.

## Open questions

- Should `owner_task` for non-frozen C# be `T15` or empty? TASKS.md gives no
  task ownership of `windows/EZLM/*.cs`; I used `T15` because it reads those
  files and reports C# findings, but the Lead may prefer `T15` to mean only
  `*_windows.go`.
- The Windows and Mac report-a-problem dialogs are close but not identical
  (the Mac one has its own sign-off). I left parity rows to a09.
- Several of these messages end in an ellipsis or are followed by a toast that
  disappears in 4.2 seconds (`Shell.cs:102`). A long proposed sentence may not
  be readable in time; the Lead may want to shorten toasts further at apply time.

## Sanity checks

- `git status` clean before the change; only `agents/T12/` touched.
- `python3 agents/T12/merge_rows.py --strict --rows agents/T12/rows --out /tmp/a06-merged.csv`
  (from a11's copy) parses all 25 rows with no skips.
