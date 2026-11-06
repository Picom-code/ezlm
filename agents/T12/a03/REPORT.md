# T12 a03: Go user-facing text in `internal/scan`, `internal/fit`, `internal/catalog`, `internal/update`

Report-only. No code or docs changed. Rows: `agents/T12/rows/a03.csv` (16 rows).

## What I scanned

All non-test Go files in the four packages (about 30 user-facing strings):

- `internal/update/update.go` (158 lines): the three `Info.Error` texts, the raw
  `os.MkdirAll` error in `Download`, and the pass-through errors from
  `engine.Download` / `engine.VerifyDigest`.
- `internal/scan/scan.go`, `quick.go`, `apps.go`, `scan_darwin.go`, `scan_other.go`,
  `gpu_discrete.go`, `display_windows.go`, `apps_unix.go`, `apps_windows.go`:
  the two hardware `Notes`, the ten app notes in `describe`, friendly names, and
  the two errors returned by `Kill`.
- `internal/fit/fit.go` (581 lines): `Plan.Recipe`, the only user-facing text.
- `internal/catalog/catalog.go`: `parse` errors and the embedded-catalog panic.

## Where these strings reach users

- `update.Info.Error` is rendered in Settings as `"EZLM <ver>. <error>."`
  (`windows/EZLM/SettingsPane.cs:66`, which appends a period unconditionally) and
  on macOS as `"EZLM <ver>. <error>."` after capitalising the first letter and
  adding a period only if missing (`macos/Sources/EZLM/Settings.swift:293-296`).
  A lowercase Go string therefore reads badly on Windows only, for example
  `EZLM 0.1.1. couldn't reach GitHub to check for updates.` That is the parity
  defect behind rows 68, 73 and 90.
- `update.Download` errors go to the Settings update note
  (`SettingsPane.cs:260`, `Settings.swift:25`) and to the download job's `fail`
  event (`internal/app/settings.go:52-55`).
- `scan.Kill` errors reach the web UI as `alert(e.message)`
  (`internal/web/static/app.js:483`) and the Windows app as a dialog note
  (`windows/EZLM/Shell.cs:495`). `os.ErrPermission` is shown verbatim as
  "operation not permitted".
- Hardware `Notes` are rendered by the web UI (`internal/web/static/app.js:451`,
  frozen) and the CLI (`cmd/ezlm/main.go:375`). The desktop apps parse
  `hw.Notes` but I found no place that displays it.
- App notes (`describe` in `apps.go`) are shown in the Windows Model mode list
  (`windows/EZLM/ModelMode.cs:114`). The Mac app's app model has no note field,
  so these notes are Windows and web only (web `app.js:448-508`).
- `Plan.Recipe` is stored in the launch config and printed in the problem report
  (`internal/app/launch.go:205`).

## Worst problems

1. **Update errors are lower-case fragments on Windows** (rows 68, 73, 90,
   `internal/update/update.go`). They are written for the Mac app's
   capitalise-first-letter wrapper, so the Windows Settings screen shows
   `EZLM 0.1.1. couldn't reach GitHub...`. The most common failure (offline
   machine) is the worst-looking one. Fixing the Go strings to start with a
   capital and end without a period fixes both apps; the C# file is frozen.
2. **Raw OS and library errors are shown verbatim** (rows 114, 262, 265).
   `scan.Kill` returns `os.ErrPermission` ("operation not permitted") and the
   gopsutil process error ("process 4242 does not exist"); `update.Download`
   returns the `os.MkdirAll` error. None of them say what to do next.
3. **The app note for runtime processes can produce nested brackets**
   (`internal/scan/apps.go:377`). The note already wraps `a.Name` in brackets,
   and `friendlyName` can return `Hermes (Python)`, so the user sees
   `A program running in the background (Hermes (Python)).` The proposed text
   drops the bracket because the name is shown next to the note.

## Counts by owner task

| owner_task | rows |
|---|---|
| T08 (`internal/update`) | 4 |
| T05 (`internal/scan`) | 8 |
| T04 (`internal/fit`) | 2 |
| T01 (`internal/catalog`) | 2 |
| total | 16 |

Severity: 3 major (update 68 and 73, scan 265), 13 minor, 0 blocker.

## Cross-package findings (no rows; outside this slice)

- `internal/engine/engine.go:335` returns `checksum mismatch for EZLM-Setup.exe`
  and `update.Download` passes it to the Settings update note unchanged. The user
  sees no explanation and no next action. This is an a13 (engine errors) item;
  the owner of `engine.go` is T06.
- `internal/update/update.go:64` ignores the error from
  `http.NewRequestWithContext`; a bad `EZLM_UPDATE_URL` leaves `req == nil` and
  `req.Header.Set` panics. Not a text bug, reported for T08.
- `internal/catalog/catalog.go:163` panics on a damaged embedded catalog. The
  panic text is user-visible only through a crash report, hence minor.
- The Windows Settings line appends `"."` after `Info.Error` and the Mac line
  adds one only when missing (`SettingsPane.cs:66`, `Settings.swift:295`). Until
  one of those is unified, Go strings in `Info.Error` should not end with a
  period. All proposed texts for rows 68, 73 and 90 follow that rule.

## Open questions

- `internal/catalog/catalog.go` is not in any task's Owns list (T01 owns
  `catalog.json`, `scripts/verify_catalog.py` and `internal/catalog/*_test.go`;
  T11 owns new `*_test.go` only). I attributed its two rows to T01 as the
  catalog task. The Lead may want to confirm.
- `windows/EZLM/ModelMode.cs` and `internal/web/static/app.js` are frozen
  (ModelMode.cs is not in the frozen list, but app.js is); the note rewrites are
  Go-side only and will show in both places once applied.
- Should the HTTP status stay in the update error? I kept `(%s)` in row 73
  because it helps problem reports; dropping it would read more plainly.

## Verification

`gofmt -l .` is empty and `go build ./...` passes (nothing was changed).
