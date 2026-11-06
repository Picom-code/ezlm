# T12 / a12 report: messages with no next action

**Scope:** user-visible errors that say what went wrong but not what to do. Read-only audit.
Rows: `agents/T12/rows/a12.csv` (32 rows: 14 major, 18 minor).

## What I scanned

- All Go error constructors (`fmt.Errorf`, `errors.New`) in `internal/serve`, `internal/engine`,
  `internal/web`, `internal/keys`, `internal/prereq`, `internal/version`, `internal/report`,
  `internal/fit`, `internal/scan`, `internal/update`, `internal/proc`, `internal/awake`,
  `internal/catalog`, `internal/app` and `cmd/ezlm` (111 constructors, tests excluded).
- Every C# string in `windows/EZLM/*.cs` that is shown through `Toast`, `ShowFailed`, a dialog,
  or a `reply.Error` assignment.
- Every Swift error string in `macos/Sources/EZLM/*.swift` that reaches an alert, toast or chat
  bubble.

Of those, 32 messages name a failure with no next action and are in the CSV. Messages that already
name a next action (for example `internal/engine/download.go:227` and `:229`, `Chat.cs:671`,
`ChatStore.cs:254`) were left out, as the brief asks.

## Worst problems (blocker/major)

1. `internal/engine/engine.go:95`, `:189` (T06) - "llama.cpp release %s has no downloadable
   builds" / "no llama.cpp build for %s/%s (%s) in release %s". The setup cannot continue and the
   user is told nothing. Every other engine failure (`download.go:223-247`) already ends with an
   action, so these are the outliers.
2. `internal/serve/config.go:69` (T07) - a damaged settings file is reported with no recovery.
   The user cannot start EZLM again until they find and delete the file themselves.
3. `internal/serve/client.go:105` (T07) - a model load timeout dumps log lines but never suggests
   a smaller model, which is the fix in almost every case.
4. `internal/serve/service_windows.go:200` (T10) - "couldn't start the server: %v" is the last
   line the Windows user sees when the service fails; no log pointer, no retry.
5. `internal/app/app.go:1511` (frozen) - "port %d is in use by another program" leaves the user in
   the port dialog with no instruction; the fix is one sentence.
6. `windows/EZLM/Engine.cs:68` (T15) - "Couldn't start EZLM's engine." is a dead end at launch.
7. `windows/EZLM/ReportLaunch.cs:206` (frozen) - a failed report upload gives no retry path, so the
   user loses the report text they just wrote.
8. `macos/Sources/EZLM/Engine.swift:77` (T16) - "EZLM's engine stopped while starting." leaves the
   Mac user with a dead window.
9. `macos/Sources/EZLM/Updater.swift:25`, `:28` (T16) - a bad update download says what is wrong
   with the bundle but not that downloading again fixes it.
10. `macos/Sources/EZLM/ChatModel.swift:702` (T16) - the fallback chat error prints the server
    error verbatim with no suggestion.

## Pattern

- Go: `internal/engine/download.go` is the model to copy. Nine of its eleven branches end with a
  concrete action; the two that do not (`:34`, `:245`) are in the CSV.
- C#: `Toast(...)` call sites catch an exception and show `ex.Message` alone (`Chat.cs:1052`,
  `ReportLaunch.cs:129`, `:206`, `SettingsPane.cs:311`). One shared "Try again, or report a
  problem" suffix would fix the class.
- Swift: the Mac app has the same pattern in `Updater.swift` and the chat fallbacks; the wording
  should match the Windows one so the two apps read alike (a09 owns parity, these rows feed it).

## Proposed wording rules used

One extra sentence, imperative, naming the action: "Delete the file, then start EZLM again.",
"Pick a smaller model in Models.", "Try again in a few minutes: EZLM resumes where it stopped."
No em dashes, no exclamation marks, no marketing words. Format verbs (`%s`, `%v`, `{name}`) kept
in place so the rows apply mechanically.

## Owner-task note

- Go packages follow the Owns lists in `TASKS.md`. `internal/app/**` and `cmd/**` are in the
  README frozen list, so those rows say `frozen`.
- C# outside the frozen list has no patching owner; findings go in T15's report, so those rows say
  `T15`. Swift outside the frozen list says `T16`. Frozen C#/Swift files say `frozen`.

## Open questions

- `internal/serve/service_darwin.go:44` and `service_linux.go:20` are raw tool output. Is the
  service layer's error shown verbatim in the apps, or wrapped before display? I assumed verbatim
  (the Windows path is). If wrapped, these two rows are cosmetic only.
- `internal/engine/engine.go:344` ("unsafe path in archive") may be unreachable for normal users;
  kept as minor.
- The `frozen` rows will be dropped by the morning merge unless the owning task re-applies them;
  they are still worth keeping in `strings.csv` for the next pass.

## Verification

No code or docs changed. `gofmt -l .` is empty and `go build ./...` passes (sanity only).
