# T15 / a13 — Go process invocation on Windows

Slice: `internal/proc` on Windows, job objects, `CREATE_NO_WINDOW`, console
windows, process tree kill. No Windows machine here: every runtime test below
was cross-compiled (`GOOS=windows GOARCH=amd64|arm64 go test -c`) and
type-checked with `go vet`, but not executed. Code-read findings say so.

## Findings

### 1. major (fixed) — a job that was never joined made a timeout hang, and leftovers survive

`internal/proc/proc_windows.go:24-37` (before) created the job and assigned
the process in `track()`, but ignored both the `SetInformationJobObject`
error and the `AssignProcessToJobObject` error. When either failed the tree
still looked valid (`job != 0`), so:

- `kill()` called `TerminateJobObject` on a job with no processes in it and
  returned; `runTree` then waited on `<-done` for the real child, so
  `Output` with a deadline hung until the child finished on its own.
- `close()` closed a job that owned nothing, so a finished check left the
  client it started talking to the model — the exact failure the package
  exists to prevent.

Evidence: code-read; `AssignProcessToJobObject` returns
`ERROR_ACCESS_DENIED` when the process already belongs to a job that does not
allow nesting (Windows 7, or a launcher that sets no breakaway), and
`SetInformationJobObject` can fail on a job with a quota the caller's job
already owns.

Fix: `tree` now also holds the root's process handle. The job is kept only
if creating it, setting `KILL_ON_JOB_CLOSE` and assigning all succeeded;
otherwise `kill()` and `close()` fall back to `TerminateProcess` on that
handle. The handle is opened once in `track()` and closed in `close()` /
`release()`, so a recycled PID cannot be killed by mistake.

Test: `TestKillWithoutJob` (clears `tree.job` to simulate the failure, then
kills and asserts `Wait` returns inside 5s).

### 2. major (fixed) — the job is assigned after the process starts, so a fast grandchild escapes

`internal/proc/proc.go:62-65` calls `cmd.Start()` and only then `track()`.
`AssignProcessToJobObject` puts the named process in the job; children it
created *before* that call are not retroactively added. `cmd.exe /c npm …`
can spawn its child in that window, and a child that escapes the job is not
killed on timeout or on EZLM exit.

Evidence: code-read against the Win32 contract. `internal/integrate/pty_windows.go:105-121`
already does it correctly (`CREATE_SUSPENDED`, assign, `ResumeThread`) and
carries the comment "The job is set up before the program runs a single
instruction" — `internal/proc` does not.

Not fixed here: `exec.Cmd` gives no thread handle, so the fix needs either
`CREATE_SUSPENDED` plus a thread enumeration (`CreateToolhelp32Snapshot` +
`OpenThread` + `ResumeThread`), or moving `internal/proc` onto `CreateProcess`
directly. Both are too large for a one-line fix and cannot be verified
without Windows. Proposed fix for the follow-up: add `CREATE_SUSPENDED` in
`prepare()`, resume in `track()` after the assign, and terminate the process
if the resume fails.

### 3. minor (fixed) — `pids()` read the wrong offset on 32-bit Windows

`internal/proc/proc_windows.go:72-81` (before) read the list as
`[]uintptr` from index 1. `JOBOBJECT_BASIC_PROCESS_ID_LIST` is two DWORDs
then `ULONG_PTR ProcessIdList[]`: the header is 8 bytes on both pointer
sizes, so on 64-bit index 1 is right by accident, and on 32-bit index 1 is
still `NumberOfProcessIdsInList`, so the count was returned as a PID and the
first real PID was dropped.

Fix: the buffer is bytes now, parsed by `parseJobPids` in
`internal/proc/jobpids.go` with the ids at a fixed offset 8 and 4 or 8 bytes
per id. Because the parser takes the pointer size as an argument it is
tested on this Mac for both layouts:
`TestParseJobPids32BitFirstID`, `TestParseJobPids`,
`TestParseJobPidsTruncated`, `TestParseJobPidsLarge`.

Severity is minor only because the shipped builds are amd64 and arm64.

### 4. minor (fixed) — a job with more than 256 processes reported no leftovers

`internal/proc/proc_windows.go:73` (before) passed a fixed 256-entry buffer
and treated every error as "no leftovers". Windows answers
`ERROR_MORE_DATA` and the needed size, which was dropped. `pids()` now
retries once with the reported size (`TestParseJobPidsLarge` covers the
parse; the retry is code-read).

### 5. minor (fixed) — the package's only Windows-relevant test skipped Windows

`internal/proc/proc_test.go:12-14` skips `TestTimeoutKillsTree` on Windows
("covered on Windows by the e2e run"), and `TestOutput` (:25) ran `sh -c` on
every platform, so `go test ./internal/proc/` failed on Windows before it
reached anything. The tree kill, the whole point of the package, had no
Windows test at all. Added `internal/proc/proc_windows_test.go` (build tag
`windows`): hidden-console flags, timeout kills the tree, kill without a job,
`pids()` sees a grandchild, `InstallLeft` reports a leftover. `TestOutput`
now skips when `sh` is missing.

### 6. minor (open) — `hide()`/`hidden()` throw away a caller's `SysProcAttr`

`internal/prereq/hide_windows.go:9`, `internal/scan/exec_windows.go:11`,
`internal/serve/service_windows.go:44-47` assign a fresh
`&syscall.SysProcAttr{...}`. Any flag the caller set first is lost, and the
`0x08000000` literal should be `windows.CREATE_NO_WINDOW`. No caller sets
flags today, so nothing is broken yet; `prepare()` in `internal/proc`
already merges (`|=`) and is the pattern to copy.

### 7. minor (open, frozen file) — `startLogged` never reaps the server

`internal/serve/service_windows.go:188-198` starts the server and drops
`cmd`, so the process handle is never closed and the child is never waited
for: one leaked handle per start, and no exit status when the start is
"successful" but the server dies immediately. The flags are fine:
`DETACHED_PROCESS` wins over `CREATE_NO_WINDOW` (the docs say the latter is
ignored with the former), so there is no console flash, and
`CREATE_BREAKAWAY_FROM_JOB` with a documented fallback is the right shape.

### 8. note — `CREATE_NO_WINDOW` vs `HideWindow`

`HideWindow` alone still creates a console (it only sets `STARTF_USESHOWWINDOW`);
`CREATE_NO_WINDOW` is what removes it. `prepare()` sets both, which is
correct and redundant, and every other helper launcher in the tree does the
same. No change needed.

## What I could not verify

- Any of the new Windows tests actually passing: there is no Windows machine
  here. They cross-compile for amd64 and arm64 (`go test -c`), and
  `GOOS=windows go vet` is clean.
- Finding 2 needs a real Windows run to show the escape (spawn a grandchild
  immediately in `cmd.exe` and check it survives).
- Whether `AssignProcessToJobObject` ever fails in practice on the beta
  machines; the fallback is safe either way.

## Files changed

- `internal/proc/proc_windows.go` — process-handle fallback, job validity
  checks, byte-buffer `pids()` with `ERROR_MORE_DATA` retry.
- `internal/proc/jobpids.go` (new) — portable `JOBOBJECT_BASIC_PROCESS_ID_LIST`
  parser, so the layout is testable off Windows.
- `internal/proc/jobpids_test.go` (new) — parser tests (32- and 64-bit).
- `internal/proc/proc_windows_test.go` (new) — the Windows runtime tests.
- `internal/proc/proc_test.go` — skip `TestOutput` when `sh` is missing.

## Open questions

- Should `internal/proc` move to `CreateProcess` directly (as
  `internal/integrate/pty_windows.go` already does) to close the assign race
  in finding 2? It would also give `CREATE_SUSPENDED` for free.
- Should `Output` treat a job it could not join as an error instead of
  silently degrading? Today it degrades, and the fallback only kills the
  root, not grandchildren.

Verification run in the worktree: `gofmt -l .` empty, `go build ./...`,
`go vet ./...`, `go test -p 2 ./internal/proc/`, `GOOS=windows
GOARCH=amd64|arm64 go build ./...`, `GOOS=linux go build ./...`,
`GOOS=windows go test -c` for both architectures.
