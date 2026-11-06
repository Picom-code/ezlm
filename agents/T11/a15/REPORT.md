# T11 a15: coverage for `internal/awake` and `internal/proc`

Branch `agent/T11-a15`, worktree `~/ezlm-agents/work/subs/T11/a15`.
Test files only, no logic changes:

- `internal/awake/awake_a15_test.go`
- `internal/proc/proc_a15_test.go`

## Coverage (macOS arm64, `go test -cover`)

| package | before | after |
|---|---|---|
| `internal/awake` | 92.9% | 92.9% |
| `internal/proc` | 84.9% | 96.2% |

`internal/awake` cannot move on macOS: the only uncovered statement is the
`cmd.Start() != nil` branch in `awake_darwin.go:13` (caffeinate is started by
absolute path, so the test can't make it fail without changing logic). The new
tests pin the `Hold` contract instead: non-nil release, idempotent and
concurrent release, release returns promptly, repeated holds. `TestHoldLinux`
(fake `systemd-inhibit` on `PATH`, then an empty `PATH`) exercises the Linux
`hold` on Linux CI; it is skipped on macOS.

Remaining uncovered in `internal/proc`: `pids()` error branches
(`proc_unix.go:36` when `process.Pids()` fails, `proc_unix.go:41` when
`Getpgid` loses a process that exited in between). Both are environment
failures that need a real process table race to hit; not worth a flaky test.

## What the new tests cover

`internal/proc` (state machine around `runTree`):

- combined stdout+stderr and exit-status pass-through from `Output`;
- start failure for `Output`, `InstallTo`, `InstallLeft` (error, no output);
- `InstallTo` streams to the writer; `InstallLeft` reports no PIDs when the
  command leaves nothing behind;
- `Install` keeps a background process alive, and `Output` kills it: both
  use a fake `sh` installer that backgrounds `sleep 30` and echoes its PID,
  then check that PID with `Signal(0)` (fake process, by PID, cleaned up by
  PID).

`internal/awake`:

- `Hold` returns a usable release; release is idempotent under 8 concurrent
  callers (run with `-race`); release returns within 10s; holds can be
  repeated.

## Bugs found

None. The two packages' logic is small and the new tests found no divergence
from the documented behavior, so there are no `t.Skip("bug: ...")` tests.

Minor observation, not filed as a bug: `runTree` starts `cmd` even when the
caller's `ctx` is already done (it checks `ctx` only in the `select` after
`Start`), unlike `exec.CommandContext`, which refuses to start. The process is
killed immediately, so the effect is a wasted fork/exec for an already
cancelled install. A one-line `if err := ctx.Err(); err != nil { return err }`
before `prepare(cmd)` would remove it. Left alone: no logic changes in T11,
and the window is too small to make a deterministic failing test.

## Verification

`gofmt -l internal/awake internal/proc` empty; `go vet` for darwin, linux and
windows passes on both packages; `go test -p 2 -race` passes; `go build ./...`
passes.
