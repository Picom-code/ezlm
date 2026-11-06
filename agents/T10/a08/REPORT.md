# T10 a08: stop by PID tree

Slice: stopping a process tree must reach children after the parent exits,
stop at the process-group boundary, and never signal a process that merely
reused the tree's PID. Fake processes only, stopped by PID.

## What I reviewed

- `internal/proc/proc.go` (`runTree`: start, wait, kill on cancel, close or
  release the leftovers).
- `internal/proc/proc_unix.go` (process group as the tree).
- `internal/proc/proc_windows.go` (job object as the tree), code-read.
- `internal/serve/run.go`, `service_windows.go` (`killOwn`), `procs.go`
  (`OwnPIDs`, `isOwnExe`) for the stop path around it.

## Findings

### 1. A reused PID could be killed after the tree exited (blocker, fixed)

`internal/proc/proc_unix.go:25` and `:27` signalled the whole process group
with `kill(-pid)` and never checked that the group still belonged to the
command. The group id is the leader's PID. Once the command and its children
are gone, the id is free, and the kernel can hand it to a new process. A new
process that becomes a group leader has `pgid == pid`, so the next
`kill(-pid)` (the deferred `close()` after every `Output`/`Install`, or
`kill()` on cancel) SIGKILLs a group EZLM never started. This is the
"never kill the wrong process" invariant, and it was unguarded.

Evidence: `TestStopDoesNotKillAReusedPID` starts a fake that is its own group
leader, builds a `tree` for that id with the start time of a *previous*
leader, and calls `close()` and `kill()`. With the guard disabled the fake
dies (`stopped 61190, which reused the group id`); with the guard it
survives. The other four tests in the file pass both ways and are regression
guards.

Fix (`internal/proc/proc_unix.go`): the `tree` records the leader's start
time (`CreateTime`, ms) when the command starts. Before signalling, `ours()`
re-reads the start time at that PID:

- no process there: the group, if it still exists, is ours (a new group
  cannot take an id while old members live), so signal;
- a process there with a different start time: the id was reused, do not
  signal;
- start time unknown at track time: old behaviour, signal.

`pids()` uses the same guard, so an installer's leftover list never names a
stranger's processes.

### 2. The tree walk stops at the process group (minor, regression guard)

`tree` is the process group, not the parent chain, so a child that outlives
its parent is still reached: `TestStopKillsAChildWhoseParentExitedFirst`
runs `sh -c 'sleep 30 & exit 0'` and asserts the child is dead after
`Output` returns. A process in another group is untouched:
`TestStopStopsAtTheProcessGroupBoundary` and
`TestLeftoversStopAtTheProcessGroupBoundary` keep a fake in its own group
alive while the command's child is stopped and is the only leftover listed.

### 3. Windows job object (code-read, minor)

`proc_windows.go` tracks a job object with `KILL_ON_JOB_CLOSE`, which holds
handles to its processes, so PID reuse cannot hit it. `pids()` parses
`JOBOBJECT_BASIC_PROCESS_ID_LIST` as an 8-byte header followed by
`uintptr` ids (`buf[1:]`). That is right on 64-bit Windows; on 32-bit the
header is still 8 bytes but `uintptr` is 4, so the first id would be read
from the count. Only amd64/arm64 are built, so this is latent.

### 4. Known boundary: a daemon that calls setsid escapes the tree (minor)

`prepare()` puts the command in a new group; a program that calls `setsid`
leaves it, and `close()`/`pids()` deliberately do not chase it. The code
comments call this best effort. A caller that must not leave such a process
behind has to track it by PID itself.

## Tests

`internal/proc/stoptree_test.go` (build tag `!windows`), five tests, about
1s, all fake processes stopped by PID:

- `TestStopKillsAChildWhoseParentExitedFirst`
- `TestStopStopsAtTheProcessGroupBoundary`
- `TestLeftoversStopAtTheProcessGroupBoundary`
- `TestStopDoesNotKillAReusedPID` (fails without the fix)
- `TestStopWithoutABirthTimeStillKills`

## Checks

`gofmt -l .` empty, `go build ./...`, `go vet ./...`, `go test -p 2` for
`internal/proc`, `internal/awake`, `internal/serve`, and the windows/amd64,
windows/arm64 and linux builds pass.

## Open questions

- Start time is millisecond granularity. A PID reused within the same
  millisecond as the old leader's start would still pass the guard. That is
  the usual token (`systemd`, `ps` use start time); a kernel-level pidfd
  would be exact on Linux, but not on macOS.
- Should the leftover list (`InstallLeft`) also verify each PID right before
  the caller acts on it? The list can still go stale between `after(tree)`
  and the caller's use; the caller owns that decision.
