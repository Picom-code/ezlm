# T12 / a04: Go user-facing errors in web, keys, proc, awake, prereq, version, report

## Scope and method

Scanned every non-test Go file in `internal/web/`, `internal/keys/`, `internal/proc/`,
`internal/awake/`, `internal/prereq/`, `internal/version/` and `internal/report/` (17 non-test files,
~980 lines). Read-only: no code or docs changed. For each string literal, checked whether it can
reach a person (HTTP response body, `prereq.Check.Detail`/`Action` in the status JSON or a deploy
error) and whether it meets the README voice rules.

Also traced where the text surfaces:

- `internal/web/server.go`: the three `http.Error` bodies are returned to the browser. The JSON API
  errors (`err.Error()` at line 268) come from the app layer, so they belong to a01/a13, not here.
- `internal/prereq/prereq.go`: `Check.Detail`/`Action` go into `app.Status.Prereqs` and are
  currently not rendered by either app or the web UI (no `prereq`/`Prereqs` references in
  `windows/EZLM/*.cs`, `macos/Sources/EZLM/*.swift` or `internal/web/static/app.js`). The one path
  that reaches a user today is `app.deployPlan` (`internal/app/app.go:656`), which turns a
  `prereq.Fail` into the deploy error `c.Name + ": " + c.Action`.

## Counts

- Files scanned: 17 non-test files in 7 packages.
- User-visible strings found: 18. `web` 3, `prereq` 15 (including status labels), all other
  packages 0.
- Rows written: 10 (3 web, 7 prereq). Severity: 1 major, 9 minor.

## Findings

| # | File:line | Current | Problem | Severity |
|---|---|---|---|---|
| 1 | `internal/web/server.go:235` | `forbidden` | Raw lowercase word, no next action; visible if someone opens an API URL in the browser. | minor |
| 2 | `internal/web/server.go:255` | `this page is out of date: reopen EZLM` | Lowercase, colon instead of a full stop, and diverges from the same message in `internal/web/static/app.js` ("This page is out of date. Close it and open EZLM again."). | minor |
| 3 | `internal/web/server.go:278` | `streaming unsupported` | Jargon, no next action. | minor |
| 4 | `internal/prereq/prereq.go:136` | `This Linux is too old for the engine (needs glibc …, e.g. Ubuntu 24.04).` | Fail `Action` states the problem instead of naming what to do, and this exact text is the deploy error a Linux user is blocked on. | **major** |
| 5 | `internal/prereq/prereq.go:98,101` | `EZLM will install/update it from Microsoft …` | Reads as if Microsoft does the installing. | minor |
| 6 | `internal/prereq/prereq.go:129` | `Couldn't read the glibc version.` | Warn with no next action. | minor |
| 7 | `internal/prereq/prereq.go:152,154,156` | NVIDIA driver warns | Two sentences at 152; 154 and 156 state a requirement without telling the user to update the driver. | minor |

Proposed wording for each is in `rows/a04.csv`.

## What is already correct

- `internal/prereq/prereq.go:115` (macOS fail action) names the exact path in System Settings.
- `internal/prereq/prereq.go:150` explains what EZLM will do instead, which is the right next
  action for a warning the user cannot fix.
- `internal/report/report.go` only appends `…(trimmed)` to a long issue body; that is fine and is
  not an em dash.
- No em dashes, emoji or exclamation marks in any of the seven packages.

## No user-facing text in these packages

- `internal/keys/keys.go`: one `panic(err)` on a crypto/rand failure. Not a user message; it is a
  crash path. (Robustness note for T07, not a wording row.)
- `internal/proc/*`: returns raw `exec` errors; the wording is built by callers (a01/T10).
- `internal/awake/*`: `why` is supplied by callers; on Linux it appears in
  `systemd-inhibit --list` as `--why=…`, so its wording is owned by the caller, not this package.
- `internal/version/version.go`: no text.
- `internal/report/report.go`: no error prose shown to a person; failures silently fall back to the
  pre-filled GitHub page.

## Owner mapping

`internal/web/server.go` is owned by **T07**. `internal/prereq/` is owned by **T11** for test files
only, so no task in `TASKS.md` owns `prereq.go` itself; the Lead should apply those rewrites
directly (or route them through T11) rather than assume a task patch will carry them. All rows are
marked `T11` for grouping.

## Open questions

- Should the Go 401 body be deleted in favour of the `app.js` text, or kept as the single source?
  The two are duplicated today and `internal/web/static/**` is frozen.
- `prereq.Check.Detail`/`Action` are currently unrendered. If the apps start showing them, the
  `Fix` rows need the same next-action review as the `Fail` rows.
- Line 136 interpolates `need` (2.34 or 2.38 depending on backend/arch); the proposed text keeps
  that interpolation so the number stays accurate.

## Verification

No code changed. `gofmt -l .` empty and `go build ./...` passes on this branch.
