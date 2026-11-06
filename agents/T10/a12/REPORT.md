# T10 / a12: the fake-process helper

**Slice:** a small shared test helper that other T10 agents use to build and
control fake processes, so start/stop/restart can be proved without touching
the real machine and without `sh`, `sleep` or `llama-server`.

**Status:** done. Helper + its own tests are committed. No changes outside
`internal/proc/fakeproc/**` and this report.

## What I added

`internal/proc/fakeproc` (package `fakeproc`), about 330 lines including its
tests. It re-execs the test binary (`os.Args[0] -test.run=^TestFakeProcHelper$ -- <mode> ...`),
so there is no `go build` step, no helper binary on disk and no dependency on
shell utilities.

### How another agent uses it

1. Add this one test to a `_test.go` file in your package (once per package):

   ```go
   func TestFakeProcHelper(t *testing.T) { fakeproc.Run() }
   ```

2. Start and control helpers:

   ```go
   p := fakeproc.Start(t, "sleep")        // runs until killed; cleanup stops it
   p.Ready()                               // waits for the READY line
   pid := p.PID()                          // a real PID you may put in a PID file
   p.Stop()                                // SIGKILLs the helper's process group, waits
   ```

### Modes

| mode | arguments | behaviour |
|---|---|---|
| `sleep` | `[duration]` | prints `READY`, sleeps (default: forever) |
| `stubborn` | - | prints `READY`, ignores SIGTERM/SIGINT, sleeps |
| `exit` | `<code>` | prints `READY`, exits with `code` |
| `tree` | `[n]` | starts `n` (default 1) sleeping children, prints `CHILD <pid>` per child, then `READY`, sleeps |
| `http` | `[key] [model]` | binds `127.0.0.1:0`, prints `READY port=<n>`, serves `/health` (200) and `/v1/models` (401 on a wrong `Bearer` key, else the model id) |

### API

`Start(t, mode, args...)` / `StartCmd(t, cmd)` / `Cmd(t, mode, args...)`
(the last returns an unstarted `*exec.Cmd`, so a test can set `Dir`, `Env`,
`Stdin` first, e.g. to run in a folder with spaces or a temp `EZLM_HOME`).
`Proc` has `PID`, `Ready`, `Port`, `Children`, `WaitFor`, `Wait`, `ExitCode`,
`Signal`, `Stop`, `Exited`, `Output`. Package helpers: `Alive(pid)`,
`WaitGone(t, pid, timeout)`.

### Why it cannot kill the wrong process

Each helper is started with `Setpgid` (Unix), so it and everything it starts
form their own process group. `Stop` sends `SIGKILL` to that group only and
then waits for the helper. Nothing is ever matched by name; `Alive` and
`WaitGone` only look at a PID and never signal. Windows (code-read only):
no group, `Stop` kills the one process it started.

## Tests (my slice's test file)

`internal/proc/fakeproc/fakeproc_test.go`:

- `TestStartAndStop`: helper is alive after READY, gone after `Stop`, double
  `Stop` is safe.
- `TestExitCode`: `exit 7` is reported as 7.
- `TestChildrenDieWithParent`: two children are alive, then gone after the
  parent is stopped (the invariant behind stop-by-PID-tree tests).
- `TestStubbornNeedsAKill`: survives SIGTERM, dies on `Stop`.
- `TestHTTPHelper`: real listener in another process, right key/model 200,
  wrong key 401, `/health` 200.
- `TestCmdCanBeCustomised`: `Cmd` + custom `Dir` + `StartCmd`.
- `TestAliveOfGonePID`.

All pass on macOS/arm64; no helper processes left behind (checked with `ps`).

## Findings (review, not fixed by me)

1. **minor - `internal/proc/proc_test.go:19`**: `TestTimeoutKillsTree` uses
   `sh -c "sleep 30 & wait"`. It skips on Windows and needs a real `sh` and
   `sleep`; on a minimal Linux image (busybox-less) it fails for the wrong
   reason. The new helper covers the same tree-kill case with a built-in
   process. Suggested: a08/a09 can use `fakeproc.Start(t, "tree")` instead.
2. **minor - `internal/proc/leftover_unix_test.go:25`**: cleanup kills leftover
   PIDs with `syscall.Kill(pid, SIGKILL)` after the fact. It started them, so
   this is allowed, but a PID that exited and was reused before cleanup would be
   signalled. `fakeproc.Stop` avoids the window by killing the group it owns
   while the leader is still known.
3. **info - `internal/serve/stopped_test.go:45`**: the "loading server" test
   uses an in-process `httptest` listener. That is right for `WaitHealthy`, but
   it is not a separate process, so it cannot prove `OwnPIDs`/port-conflict
   behaviour; `fakeproc.Start(t, "http", key, model)` gives a separate
   process for a06/a07/a14.

## Open questions

- Should `internal/proc/proc_test.go` be migrated to `fakeproc` (one test
  helper, no `sh`)? I left it alone because other a0x agents may be editing
  process tests; migration is a two-line change per test.
- Do we want a `fakeproc` mode that binds a *specific* port (a06/a07 need
  "the chosen port is taken")? Today a test can take `Port()` from the
  helper's random port, but it cannot ask for a fixed one. Trivial to add if
  the port-conflict agents need it.
