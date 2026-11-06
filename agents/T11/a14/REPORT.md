# T11 / a14: flakiness review of `internal/serve`

Branch `agent/T11-a14`, worktree `~/ezlm-agents/work/subs/T11/a14`.
Go 1.27.3 darwin/arm64.

## Method

Ran the existing `internal/serve` tests repeatedly with the race detector, then widened the
search where a flake could hide (shuffle order, one CPU, several copies of the test binary at
once). No new tests were added and no non-test file was touched.

| Run | Command (in the worktree) | Result |
|---|---|---|
| required | `go test -p 2 -race -count=20 ./internal/serve/` | ok, 103.3s, no flakes |
| stress | `-race -count=100 -run 'TestPortFreeSeesListener\|TestTestInstanceServiceName\|TestIsOwnExe\|TestOursChecksKeyAndModel\|TestFetchProps'` | ok, 1.6s |
| order | `-race -shuffle=on -count=10 ./internal/serve/` | ok, 52.6s |
| scheduling | `-race -cpu=1 -count=5 ./internal/serve/` | ok, 26.7s |
| concurrency | `go test -c -race`, then four copies of the binary at once, each `-test.count=5` | ok, 4/4 pass |

The two `WaitHealthy` tests sleep in real time (about 2s and 4s per iteration), so the
required run costs about 100s. That is the package's own wall clock, not a flake.

## Findings

### 1. `TestFetchProps` read the handler's variables without synchronisation (minor, fixed)

`internal/serve/props_test.go:16` (`auth`) and `:19` (`body`) were written by the
`httptest` handler goroutine and read or rewritten by the test goroutine. The only ordering
between them was the HTTP response itself, which the race detector does not model as a
synchronisation edge (the two sides share no mutex or channel), so `-race` stays quiet while
the Go memory model still calls it a data race. It never failed in 100+ runs, but it is the
kind of race that only shows up when the scheduler or `httptest` changes.

Fixed in the test only: a `sync.Mutex` guards `auth` and `body`; the handler copies `body`
under the lock before writing it. `go test -race -count=20 ./internal/serve/` passes after
the change (103.3s).

### 2. `stoppedGrace` / `stoppedChecks` are package globals mutated by two tests (minor, not fixed)

`internal/serve/stopped_test.go:28-29` and `:51-52` save, set and restore the package-level
knobs in `client.go:126`. Correct only while no test in the package calls `t.Parallel()`.
Adding one `t.Parallel()` anywhere in the package turns this into a `-race` failure and makes
the timeout of one test depend on the other's order. Shuffle order is clean today
(`-shuffle=on -count=10`), so this is a trap for a future edit, not a present flake.

### 3. `TestWaitHealthyNoticesAStoppedServer` can wait out the full timeout if its port is taken (minor, not fixed)

`internal/serve/stopped_test.go:22-36` reserves a port by listening on `127.0.0.1:0` and
closing it, then relies on nothing answering on that port. If another process binds it in the
gap, `listening()` (`client.go:112`) returns true, `gone` resets every second, and the test
falls through to the one-minute `WaitHealthy` timeout, failing the `15*time.Second` bound and
reporting "the model did not finish loading" instead of `ErrStopped`. The window is tiny; it
did not fire in the four-concurrent-binaries run. A test-only fix would retry the whole body
when `PortFree` says the port is busy, but that hides the same race, so it was left alone.

### 4. `TestPortFreeSeesListener` asserts a specific ephemeral port stays free (minor, not fixed)

`internal/serve/port_test.go:20-30`: after `l.Close()` the test requires `PortFree(port)` to
be true. A concurrent binder on the same host makes it false. Not observed in 100 runs or in
the four-binary run.

### 5. `TestStartLoggedUnicodeFolder` is Windows-only and cannot be run here (open question)

`internal/serve/startlog_windows_test.go:26` returns as soon as the log holds both lines,
without waiting for `cmd.exe` to exit. `startLogged` (`service_windows.go:177`) closes only
its own handle, so the child can still hold the log open when `t.TempDir()` cleanup runs;
on Windows that can fail with "TempDir RemoveAll cleanup". The test also `t.Fatal`s when
`ComSpec` is unset, which some CI runners do. Unverifiable on darwin (no wine here), so it is
reported, not changed.

## What I changed

- `internal/serve/props_test.go`: `sync.Mutex` around `auth` and `body` (finding 1).

`gofmt -l .` is empty, `go vet ./internal/serve/` and
`go test -p 2 -race -count=20 ./internal/serve/` pass.

## Open questions

- `internal/serve` is not in T11's owned paths (`internal/catalog`, `keys`, `paths`,
  `prereq`, `report`, `version`, `web`). The morning merge takes `git diff main...branch`,
  so the `props_test.go` fix may be dropped; the owner of `internal/serve` should take it.
- `internal/serve/props.go` is a do-not-touch file tonight; findings above stay in tests.
