# T10 / a15 — hide/show window handling

Slice: `hide_*.go` in `internal/serve` (macOS/Linux no-op, Windows console hiding) plus the
window show/hide code that consumes it. Branch `agent/T10-a15`, worktree
`~/ezlm-agents/work/subs/T10/a15`.

## What I reviewed

- `internal/serve/hide_other.go` (non-Windows no-op) and `internal/serve/hide_windows.go`
  (delegates to `hidden`).
- `internal/serve/service_windows.go`: `hidden` (line 44), `startLogged` (line 177) and every
  `schtasks` call that hides its console.
- `internal/serve/run.go:25`, the only caller of `hideChild`.
- The other copies of the same helper, for drift: `internal/proc/proc_windows.go:16`,
  `internal/prereq/hide_windows.go:8`, `internal/scan/exec_windows.go:11`.
- The show side, read only: `windows/EZLM/Program.cs` (single instance, `ShowWindow`),
  `windows/EZLM/Shell.cs:45` (window title), `windows/EZLM/Engine.cs:56` and
  `windows/EZLM/Program.cs:35` (child processes started with `CreateNoWindow`), and on macOS
  `macos/Sources/EZLM/EZLMApp.swift:32` (`setActivationPolicy`/`activate`).

## What is correct

- The macOS/Linux no-op (`hide_other.go:7`) is right: a child of the `.app` has no console of
  its own, and when `ezlm run-server` is run from a terminal the child should share that terminal.
- `startLogged` (`service_windows.go:188-194`) hides the server console on both the
  `CREATE_BREAKAWAY_FROM_JOB` attempt and the fallback, and keeps `HideWindow: true` on both.
- The Windows app starts its engine children with `CreateNoWindow` (`Engine.cs:62`,
  `Program.cs:35`), so no console flashes from the app side either.
- `hideChild` is called after `Stdout`/`Stderr` are wired in `run.go:23-25`, and the Windows
  implementation only adds process attributes, so the log handle is unaffected.

## Findings

### F1 — `hidden` replaced the caller's process attributes (minor, latent, FIXED)

`internal/serve/service_windows.go:44-47` (before this change) assigned a brand new
`&syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}`, discarding whatever the
caller had already set: `CreationFlags` (a process group, a breakaway job), `CmdLine`, `Token`,
inherited handles. `hide_windows.go:5` routes `hideChild` straight into it, and `run.go:25` is
the only caller today. That caller passes a nil `SysProcAttr`, so there is no live bug, but the
next caller that sets attributes before hiding would lose them silently. `internal/proc` already
does it the safe way (`proc_windows.go:16-22` merges into the existing struct).

Fixed by merging, and by keeping the caller's pointer:

```go
if cmd.SysProcAttr == nil { cmd.SysProcAttr = &syscall.SysProcAttr{} }
cmd.SysProcAttr.HideWindow = true
cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
```

Evidence (platform-neutral replay of old vs new logic):

```
old hidden: caller attrs kept=false flags=0x8000000
new hidden: caller attrs kept=true  flags=0x8000200
```

Test `TestHideChildKeepsCallerAttributes` (`internal/serve/hide_test.go`) asserts pointer
identity and that `CREATE_NEW_PROCESS_GROUP` survives while `CREATE_NO_WINDOW` is added. On
Windows it fails against the old code and passes against the new one. Windows-only: executed
nowhere (no Windows host, no wine here), compiled with
`GOOS=windows GOARCH=amd64 go test -c ./internal/serve`.

### F2 — three more copies of the helper still clobber (minor, code-read, NOT fixed)

The same five lines exist four times with two behaviours:

| copy | behaviour |
|---|---|
| `internal/serve/service_windows.go:44` | merges (fixed here) |
| `internal/proc/proc_windows.go:16` | merges |
| `internal/prereq/hide_windows.go:8` | replaces |
| `internal/scan/exec_windows.go:11` | replaces |

The `prereq` and `scan` copies drop caller attributes the same way F1 did. Not fixed because
those packages are outside T10's owned paths (`internal/proc`, `internal/awake`, service parts
of `internal/serve`); the clean fix is one exported helper in `internal/proc` used by all four.

### F3 — second launch can show nothing (minor, code-read, read-only file)

`windows/EZLM/Program.cs:44-49`: a second EZLM finds the first window by title
(`FindWindow(null, "EZLM")`, title set at `Shell.cs:45`) and restores it. If the first
instance is still starting and has not created its window yet, `FindWindow` returns zero and
the second launch returns silently: the user double-clicks the icon and nothing happens. A
title match is also fragile (another app can own the title "EZLM"; a localized or renamed
title breaks it). Suggested fix: have the first instance signal readiness (named event or
pipe) and retry for a few seconds before giving up, or match the window class instead of the
title. Severity minor (UX, no data loss).

### F4 — `--autostart` keeps a hidden process for up to ten minutes (minor, code-read)

`windows/EZLM/Program.cs:30-39` starts `ezlm start --login` with `CreateNoWindow` and then
`WaitForExit(10 * 60 * 1000)`. If the engine never finishes starting, a windowless `EZLM.exe`
stays in Task Manager for ten minutes. Not a window bug, but it is the other half of the
"invisible process" story and worth a timeout that matches the engine's own start timeout.

### F5 — macOS activation is unconditional (info, read-only file)

`macos/Sources/EZLM/EZLMApp.swift:32-35` always sets `.regular` and calls
`activate(ignoringOtherApps: true)`. There is no login item or `LSUIElement` in `macos/`
today, so nothing launches the app hidden; if one is added later, this will steal focus at
login. No change made.

## Tests added

`internal/serve/hide_test.go` (this agent's file; no other test file touched):

- `TestHideChildKeepsCallerAttributes` — caller's `SysProcAttr` pointer and
  `CREATE_NEW_PROCESS_GROUP` survive, `CREATE_NO_WINDOW` is added.
- `TestHideChildNilAttributes` — nil attributes are safe; off Windows they stay nil, on
  Windows `HideWindow` and `CREATE_NO_WINDOW` are set (fields read via reflection so the file
  compiles on every OS).

Windows-only behaviour is code-read plus cross-compiled; it was not executed on Windows.

## Could not finish / next steps

- Run the two hide tests on a real Windows machine (or wine) to execute the Windows branch.
- F2: move the helper to one place in `internal/proc` and use it from `serve`, `prereq` and
  `scan`; needs a T10-owned change to `prereq`/`scan` or a follow-up task.
- F3/F4: owner of `windows/EZLM` should make the single-instance handshake event-based.
