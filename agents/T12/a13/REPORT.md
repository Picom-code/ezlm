# T12 a13 report: engine errors with no plain explanation

## What I scanned

The engine layer and the places its errors become screen text:

- `internal/engine/` (engine.go, download.go, pinned.go): every error the
  install and download paths can return.
- `internal/serve/client.go`, `service_*.go`, `config.go`, `props.go`: the
  llama-server lifecycle errors.
- The display path in `internal/app/app.go`, `launch.go`, `connect.go`
  (read only, to prove each string reaches a screen) and the two apps'
  renderers (`windows/EZLM/Panes.cs`, `macos/Sources/EZLM/AppModel.swift`,
  `MainView.swift`) to confirm nothing rewrites `event.msg`.

Count: 26 strings, all in `agents/T12/rows/a13.csv`. The long form, with
evidence and a proposed wording for each, is
[`engine-errors.md`](engine-errors.md).

## Worst problems

1. **A raw llama.cpp log line is the entire error** (`internal/serve/client.go:81`,
   from `fatalInLog`). This is the most likely beta failure (a model that
   does not fit) and the user gets `llama_model_load: ... failed to
   allocate ...` with no meaning and no next action. It reaches the screen at
   `internal/app/app.go:842` and `windows/EZLM/Panes.cs:677`.
2. **Two mappers disagree.** `internal/app/app.go:1552` `serverReason`
   explains out-of-memory, a missing model file and a taken port in plain
   words, but only on the Start path. The same failures during Setup
   (`app.go:842`) show the raw line. One shared mapper would fix both.
3. **`engine.Install` errors have no mapper at all** (`app.go:664`). The
   model download path rewrites a full disk (`app.go:743`); the engine
   install path does not, so the first setup step can fail with `no space
   left on device`.
4. **Raw OS and exec errors** (`permission denied`, `Access is denied`,
   `launchctl ...`, `systemctl ...`) are shown with internal paths and no
   next action.

## Owner split of the rows

- T06 (`internal/engine/engine.go`): 8
- T09 (`internal/engine/download.go`): 4
- T07 (`internal/serve/client.go`, `config.go`): 8
- T10 (`internal/serve/service_*.go`): 4
- frozen (`internal/app/app.go:1570`): 1

Severity: 15 major, 11 minor, no blocker. No code or docs changed.

## What I could not finish

- Did not prove the ANSI-escape claim in `TailLog` output with a real
  Windows log; it is an open question, not a row.
- Did not audit the CLI (`cmd/ezlm`) rendering of the same errors; that is
  a01's area. Any rewrite changes terminal output too.

## Open questions

- The brief lists `internal/serve/client.go` under T07, but `WaitHealthy` is
  lifecycle code that T10 may also touch. Rows use T07.
- `internal/serve/props.go:38` has the same shape but its error is
  swallowed by `refreshProps`, so it is not a row yet.
