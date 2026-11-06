# T10 a16 REPORT: severity table

## What this slice is

a16 owns no product code. Its deliverable is `agents/T10/a16/findings.md`: one table of every
T10 finding with status (open/fixed/mitigated), evidence (file:line), severity, fix, and the
Windows-only items marked as code-read. This REPORT records what was reviewed, what was run, and
what the Lead should do with the table.

## What was reviewed

Read in full at baseline `5213370` in this worktree:

- `internal/proc/**`: `proc.go`, `proc_unix.go`, `proc_windows.go`, `pdeath_*.go`, tests.
- `internal/awake/**`: `awake.go`, `awake_darwin.go`, `awake_linux.go`, `awake_windows.go`,
  `awake_other.go`, tests.
- `internal/serve/**` service/lifecycle parts: `service_darwin.go`, `service_linux.go`,
  `service_windows.go`, `service_name.go`, `procs.go`, `birth_*.go`, `hide_*.go`, `run.go`,
  plus the lifecycle paths in `client.go` (`Healthy`, `Ours`, `WaitHealthy`, `StartedAt`) and
  `config.go` (`PortFree`, `FreePort`, `StopAndWait`, `StoppedFlag`).
- Callers that decide when a server is started/stopped: `internal/app/app.go` (port choice,
  `StopAndWait`, `WaitHealthy`, `awake.Hold`), `internal/app/settings.go`,
  `internal/engine/engine.go` (install layout, so the `libPath` walk could be bounded).
- Existing tests: `internal/serve/procs_test.go`, `port_test.go`, `stopped_test.go`,
  `startlog_windows_test.go`, `internal/proc/*_test.go`, `internal/awake/awake_test.go`.

## What was run

- `go test -p 2 -count=1 ./internal/proc/ ./internal/awake/ ./internal/serve/` -> all pass
  (0.834 s, 0.486 s, 5.328 s).
- `go build ./...` -> pass.
- `GOOS=windows GOARCH=amd64 go build ./...` -> pass.
- `GOOS=linux go build ./...` -> pass.
- No `gofmt` change: only Markdown was added, so `gofmt -l .` is unchanged from baseline.

No model was loaded and no llama-server, launchd, systemd or schtasks command was run. No process
was started or killed by this slice.

## Findings

Full table with evidence and fixes: [`agents/T10/a16/findings.md`](findings.md). Counts:

| Severity | Count | Open | Mitigated | Fixed |
|---|---|---|---|---|
| blocker | 0 | 0 | 0 | 0 |
| major | 10 | 10 | 0 | 0 |
| minor | 32 | 25 | 7 | 0 |
| already-fixed guards (section E) | 7 | - | - | 7 |

The ten majors, in one line each:

1. **P1** Unix kills by process-group id without checking the group still belongs to the started
   process; a reused pgid can kill an unrelated group (`internal/proc/proc_unix.go:25-27`).
2. **P2** Windows assigns the job object after the child starts and ignores assignment errors;
   grandchildren can escape and a failed assign makes kill a no-op (`proc_windows.go:24-37`).
3. **D1** macOS `Install` removes the old launch agent before the new plist is known-good; a
   failed write/bootstrap leaves no autostart and no rollback (`service_darwin.go:80-86`).
4. **L1** The systemd unit quoting does not escape `$`, which systemd expands in `ExecStart`
   (`service_linux.go:25-27`).
5. **L2** `Environment=LD_LIBRARY_PATH=` and the log paths are unquoted/unescaped, so a home
   with a space or `%` breaks the unit (`service_linux.go:58-68`).
6. **W1** Windows `Start` treats any existing `llama-server` as healthy, so a wedged leftover
   blocks recovery forever (`service_windows.go:159-162`).
7. **W2** Windows Start is check-then-act with no lock; two starters can spawn two servers and
   truncate each other's log (`service_windows.go:159-168`).
8. **O1** `OwnPIDs` compares a resolved exe path with an unresolved home, so a symlinked
   `~/.ezlm` hides EZLM's own server (`procs.go:25,35-41`).

Plus **R1** (`FreePort` returns a busy port when all 100 are taken) and **R2** (check-then-bind
race) from the port slice; both are majors for a06/a07 to fix, listed here so the table is
complete.

## Windows-only items (code-read on this Mac)

W1-W9 in `findings.md`, and the Windows half of P2/P3 (process job object), O2 (elevated server
invisible to `OwnPIDs`), F6/F7. The Windows build compiles; nothing was executed on Windows. The
existing Windows test `TestStartLoggedUnicodeFolder` is skipped here for the same reason.

## What was changed

Only `agents/T10/a16/findings.md` and this `REPORT.md`. No product code, no test file, no shared
file. That keeps the a16 branch a pure documentation patch and avoids colliding with a01-a15.

## What could not be finished

- Sibling findings: a01, a02, a03, a05, a07, a10, a11, a12, a14 and a15 committed while this
  table was being written. Section F of `findings.md` maps each commit to the rows it changes.
  a10 and a14 both fixed O1 in `procs.go` and will conflict; the merge should keep one
  implementation and both tests. No sibling has fixed P1, P2, D1, L1, L2, W1 or W2 yet.
- No reproduction tests for P1/P2/O1: a reused pgid and a symlinked home need real processes; the
  slice's time-box was spent on the table. a08/a12/a14 own the fake-process harness and should
  add those tests. The evidence is the exact call path, not a guess.
- L1/L2/L3 are code-read because `service_linux.go` is not compiled on this Mac; the `GOOS=linux`
  build passes, and each finding cites the systemd syntax rule that makes it fail.

## Next steps for the Lead

1. Merge the table as the T10 severity index; ask a01-a15 to change statuses in their own reports
   rather than editing a16's table (one owner avoids merge conflicts).
2. Order fixes by the table: P2, D1, W2, O1 first (orphans and lost autostart), then L1/L2 and
   W1, then the minor list.
3. The table's section E is the regression list: F1/F2 (test instance must never stop the real
   service) and F3/F5 (never adopt a foreign listener) must survive the merge.
