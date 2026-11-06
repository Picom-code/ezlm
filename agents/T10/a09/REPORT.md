# T10 / a09 — stop when the server already exited, stale or broken PID records

Worktree `~/ezlm-agents/work/subs/T10/a09`, branch `agent/T10-a09`.

## What I reviewed

- `internal/serve/service_darwin.go`, `service_linux.go`, `service_windows.go` (stop/start paths)
- `internal/serve/procs.go` (`OwnPIDs`, `isOwnExe`), `config.go` (`StopAndWait`), `client.go`
- `internal/proc/**` (process-tree stop) and `internal/serve/birth_*.go`

## The starting point: there was no PID file

EZLM did not keep a PID file for the model server. It found the server by
scanning processes (`OwnPIDs` in `internal/serve/procs.go:14`): name starts
with `llama-server` and the executable is under `paths.Home()`. So the
"stale / corrupt / missing PID file" cases could not happen at all, but the
same failure modes did: a scan that misses the server, and a recorded PID
that now names another program.

Where EZLM starts the server itself (Windows, `startLogged`,
`internal/serve/service_windows.go:185`) the PID was known and then thrown
away, so `Stop` could only re-find it by name. This slice adds the record and
makes every read tolerate a missing, corrupt or stale file.

## What I changed

1. **`internal/serve/pidfile.go`** (new): `recordServerPID`,
   `forgetServerPID`, `serverPID`. `serverPID` treats these as "no usable
   record" and removes the file: missing file, empty/garbled/negative/huge
   number, a PID that no longer runs, and a live PID whose process is not an
   own `llama-server` (the number was reused). Callers then fall back to the
   process scan; a wrong PID is never returned for killing.
2. **`internal/serve/service_windows.go`**: `startLogged` records the PID of
   the server it started (`:203`); `killOwn` stops that recorded server
   before the scan, then forgets the record, and ignores a stale/broken record
   (`:147`). Windows-only code, cross-built.
3. **`internal/serve/procs.go`**: `isOwnExe` now resolves symlinks on both
   the executable and EZLM's home before comparing (`:35`).

## Tests (`internal/serve/stop_pidfile_test.go`)

One test file for this slice. The fake server is this test binary, copied to
`$EZLM_HOME/bin/llama-server` and started by PID (`TestMain` doubles as the
fake); it is stopped by PID in cleanup, never by name, and never touches the
real home.

- `TestServerPIDRejectsMissingCorruptAndStaleRecords`: missing, empty,
  whitespace, text, negative, zero, overflow and "12 13" records read as no
  record and are removed.
- `TestServerPIDRejectsAReusedPID`: the test process's own live PID, which
  is not a `llama-server`, reads as no record.
- `TestServerPIDFindsTheFakeServerAndDropsItAfterExit`: a live fake server
  is returned by PID; after it exits (zombie reaped) the still-present file
  reads as stale and is removed.
- `TestIsOwnExeThroughASymlinkedHome`: a real executable under a linked home
  counts as EZLM's; a program outside it does not.
- The existing `TestIsOwnExe` (another agent's file) still passes.

## Findings

| # | Where | Severity | Status | Evidence |
|---|---|---|---|---|
| 1 | `internal/serve/procs.go:35` `isOwnExe` compared cleaned paths only. With `EZLM_HOME` reached through a symlink (home on another volume, a profile link), the process reports its real path and EZLM reports the link: the server is invisible, `Stop` does nothing and `Start` can start a second server. | major | fixed | `TestIsOwnExeThroughASymlinkedHome` fails on the old comparison (real path does not share the link prefix) and passes now. `client.go:91` already names this case as a known hazard. |
| 2 | No PID record existed (`internal/serve/procs.go:14`): stop was scan-only, so a server that exits between the scan and the kill, or a PID reused by another program, could be signalled. | minor | mitigated | `serverPID` re-checks name and executable before returning a PID; `TestServerPIDRejectsAReusedPID`. |
| 3 | `internal/serve/config.go:201` `StopAndWait`: when the server already exited, `OwnPIDs` is empty and a free port returns immediately (good), but a port still held by another program makes it poll until `!Ours` answers. Not changed; covered by existing tests. | minor | noted | Code read. |
| 4 | `internal/proc/proc_unix.go:25` `tree.kill` signals the process group by the leader's PID even when the leader already exited. If the group is empty and the PGID was reused, an unrelated group could be signalled. | minor | open | Code read; a deterministic PGID-reuse reproduction needs a hook the package does not have. Report for a08/a14. |

## Open questions

- Should `RunForeground` (`internal/serve/run.go:13`, the legacy Windows
  task entry point) also record its PID? It starts the server as a child, so
  the same record would work; left out to keep the change minimal.
- On macOS/Linux launchd/systemd own the process, so no PID record is
  written there by design. If a future registry (a14) writes one, `serverPID`
  is already tolerant of stale, corrupt and missing files.

## Verification

`gofmt -l .` empty, `go build ./...`, `go vet ./...`,
`go test -p 2 ./...`, and the Windows (amd64, arm64) and Linux cross builds
all pass in this worktree. Windows behaviour is code-read plus cross-build;
the PID-file helpers are tested on macOS.
