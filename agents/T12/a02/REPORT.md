# T12 a02: Go user-facing errors in `internal/serve/` and `internal/engine/`

Report-only. No code or docs changed. Rows: `agents/T12/rows/a02.csv` (29 rows: 19 major, 10 minor).

## What I scanned

Every non-test Go file in the two packages:

- `internal/serve/`: `client.go`, `config.go`, `run.go`, `procs.go`, `props.go`, `service_darwin.go`, `service_linux.go`, `service_windows.go`, `service_name.go`, `birth_*.go`, `hide_*.go`.
- `internal/engine/`: `engine.go`, `download.go`, `pinned.go`.

That is 1,067 lines. I extracted every `fmt.Errorf`, `errors.New` and user-visible constant (29 candidates) and traced each one to where it reaches the UI: `internal/app` emits the raw `err.Error()` as a deploy step message (`internal/app/app.go:605`, `:664`, `:746`, `:842`, `:865`), and the Windows app shows that text verbatim (`windows/EZLM/Install.cs:225`, `:278`). The Mac app does the same through its engine event stream. So a Go error string is user-facing unless something upstream rewrites it.

Owner tasks come from the Owns lists in `TASKS.md`: `engine.go` and `pinned.go` are T06, `download.go` is T09, `serve/config.go`, `client.go` and `run.go` are T07, and the service files are T10. `props.go` is on the README frozen list.

## Worst problems

1. **Raw Go errors are appended to plain explanations.** `plainNetErr` ends five of its messages with `(%w)` (`internal/engine/download.go:223-231`). The user sees text like `this computer can't find the download server (huggingface.co), so it looks offline. Check the internet connection and try again: EZLM picks up where it stopped (Get "https://...": dial tcp: lookup huggingface.co: no such host)`. The wrap is useful for `errors.As`; the fix is an error type whose `Error()` is only the plain sentence and whose `Unwrap()` returns the cause. Same pattern at `engine.go:255`.
2. **A raw llama.cpp log line is the whole error.** `internal/serve/client.go:81` returns `fatalInLog()` unchanged, so a failed load shows `llama_model_load: error loading model: ...` with no explanation or next action. This is the most common hard failure during setup.
3. **Windows-only instruction on macOS.** `internal/serve/config.go:196` (`ErrStillRunning`) tells the user to end `llama-server` in Task Manager. The Mac app already says Activity Monitor for the same situation (`macos/Sources/EZLM/MainView.swift:238`), and the Windows app says Task Manager (`windows/EZLM/Panes.cs:141`). The shared Go string should name both, or the apps should own the wording.
4. **Command output shown as the error.** `internal/serve/service_darwin.go:44` and `service_linux.go:20` surface `launchctl bootstrap: ...` and `systemctl ...: ...` verbatim when the login service cannot be installed.
5. **No next action on the setup failures.** `client.go:97` and `:105` (server stopped or timed out while loading) and `engine.go:189` (no build for this machine) say what went wrong and stop there.
6. **Jargon in the engine install path.** `llama-server not found in ...`, `checksum mismatch for ...`, `unsafe path in archive: ...` and `unpacking ...: ...` are shown to end users. The app calls the same thing "the AI engine (llama.cpp)" (`internal/app/app.go:661`), so the engine strings should use that name.

## Consistency notes

- "report this problem" (`download.go:94`, `:241`, `:243`) versus the app's "send a report" (`windows/EZLM/Install.cs:166`, `internal/app/app.go:634`).
- "EZLM picks up where it stopped" (`download.go:211`) versus "EZLM resumes where it stopped" (`download.go:247`, `internal/app/app.go:744`).
- `couldn't`/`didn't` contractions in `serve` versus `could not`/`has no` in `engine.go:61`, `:95`.
- Error strings start lowercase; the deploy error panel shows the message as the first text in a dialog, where a capital would read better. I kept the Go convention in the proposed text and left this as an open question.

## Strings checked but not reported

- `internal/serve/props.go:38` (`/props answered %s`): `refreshProps` swallows the error (`internal/app/connect.go:89-95`), so it does not reach the UI today. If it is ever shown, it needs the same treatment.
- `internal/engine/engine.go:61` and `:95`: `Install` swallows a `LatestRelease` failure and falls back to `pinnedRelease` (`engine.go:200-206`), so these two only reach the UI through a future caller.
- `internal/engine/download.go:34` (`errStalled`): internal sentinel, always wrapped by `plainNetErr`.
- `internal/serve/procs.go`, `service_name.go`, `birth_*.go`, `hide_*.go`: no user-facing text.

## Test coupling to keep in mind when applying the rewrites

- `internal/app/fallback_test.go:38` and `connect_test.go:136` depend on `the model server stopped while loading`.
- `internal/app/app.go` `driverFailure` checks `strings.HasPrefix(l, serve.ErrStopped)` and the literal `did not finish loading`; both must stay or the app must change with them.
- `internal/engine/download_status_test.go:30` depends on `missing-Q4.gguf isn't on the download server`.
- `internal/engine/download_neterr_test.go` matches on `looks offline`, `keeps dropping` and `intercepting secure connections`; the proposed texts keep those phrases.

## Open questions

- Should the shared Go error name both Task Manager and Activity Monitor, or should each app own that sentence? The apps already do, so the Go string is the duplicate.
- `run.go` returns `Load`, `os.OpenFile` and `cmd.Start` errors unchanged. It is the Windows scheduled task entry point and a debugging aid, so the errors go to the log rather than a UI; I did not write rows for them. Confirm whether the Mac CLI shows them.
- Severity call: I marked every jargon or missing-next-action string that appears during setup as major, and wording and two-sentence issues as minor. The Lead may want to re-rank the `download.go` `%w` rows, since one fix removes five of them.

## Next steps

1. T09: introduce a plain error type for `plainNetErr` and `statusError` so the user text does not include the wrapped cause; one change clears 5 major rows.
2. T07: wrap `fatalInLog()` in a sentence with a next action, and add the platform wording to `ErrStillRunning`.
3. T06: replace `llama.cpp`/`llama-server` with "the AI engine" and add a next action to the install failures.
4. T10: wrap `launchctl`/`systemctl` output in a sentence with a next action.
