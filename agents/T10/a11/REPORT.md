# T10 / a11: sleep and wake

Slice: `internal/awake/**`. Branch `agent/T10-a11`, worktree `~/ezlm-agents/work/subs/T10/a11`.
Time-boxed: one sitting, about 25 minutes.

## What I reviewed

`internal/awake` is small: `Hold` starts one helper process that keeps the machine
from idle-sleeping (`caffeinate -i -w <ezlm pid>` on macOS, `systemd-inhibit ...
tail --pid=<ezlm pid>` on Linux, a `PowerCreateRequest` on Windows), and the
release kills it. The only caller is `internal/app/app.go:592`, which holds while
a setup job (scan, download, deploy) runs.

The hold path itself is sound. It already had the properties that matter:

- the helper ends by itself when EZLM dies (`-w <pid>`, `tail --pid`), so a crash
  never leaves the machine unable to sleep;
- `Hold` guards release with `sync.Once`, so a double or concurrent release is safe;
- a missing or failing helper degrades to a no-op release, not a panic.

## Findings

| # | Severity | Where | Finding | Status |
|---|---|---|---|---|
| F1 | major | `internal/awake` (whole package), caller `internal/app/app.go:592` | Nothing in EZLM notices that the machine slept. The package only *prevents* sleep; there is no state machine that detects a wake, so after a lid close or a `pmset sleepnow` the model server may be dead or stalled and the app still shows it as running. `grep -rn awake.` finds exactly one call site (`Hold`). | Fixed here: added `Watcher` (`internal/awake/wake.go`). Wiring into the app/serve supervisor is a next step (both files belong to other tasks). |
| F2 | minor | `internal/awake/awake_linux.go:17` | An empty reason was passed through as `--why=`, leaving a blank inhibitor entry. Now defaults to "EZLM is working". | Fixed, covered by `TestHoldEmptyReason`. |
| F3 | minor | `internal/awake/awake_unix.go:9` | The old `hold` returned `Kill(); <-done` directly. Correct today because `Hold` wraps it, but the invariant lived in the caller. Moved the `sync.Once` into `hold` too, so the release stays idempotent if `Hold` ever changes. | Fixed (defence in depth). |
| F4 | minor, code-read | `internal/awake/awake_windows.go:38` | Only the power-request handle is kept by the release closure; the UTF-16 reason buffer (`s`) and the `reasonContext` are not. If `PowerCreateRequest` keeps the pointer instead of copying the string, GC could free the reason. Cannot confirm without a Windows run. | Open question. |
| F5 | info, code-read | `internal/awake/awake_windows.go:18-23` | `reasonContext` is 32 bytes on 64-bit where the C struct is 24. The extra bytes are trailing padding and the kernel reads only the first 24, so this is harmless; it would also be harmless (4 extra bytes) on 386. Not a bug, noted so nobody "fixes" it. | No change. |
| F6 | info | `internal/awake/awake_darwin.go:13` | macOS has no place for `why`: `caffeinate` has no reason field, so the reason only appears in `pmset -g assertions` as the process name. Cosmetic. | No change. |
| F7 | info | `internal/awake/awake_unix.go:9` | A failed hold is silent: the caller gets a no-op release and cannot tell that sleep protection is off (e.g. `systemd-inhibit` present but no systemd session). | Documented only; an `Active()` accessor would be an API change. |
| F8 | minor, code-read | `internal/awake/awake_darwin.go:14` | `caffeinate -w <ezlm pid>` is tied to a PID. If EZLM is SIGKILLed and its PID is reused before caffeinate notices, caffeinate keeps holding until the new process exits. Very narrow. | No change. |

The existing `awake_test.go` (`TestHoldMac`, `TestHoldWindows`) is untouched and
still passes.

## Tests added

`internal/awake/sleepwake_test.go` (any OS):

- `TestWatcherIgnoresNormalTicks`: late ticks are not wakes.
- `TestWatcherReportsSleep`: a wall jump the monotonic clock did not see is
  reported, with roughly the slept duration.
- `TestWatcherIgnoresBackwardsWallStep`: an NTP step backwards is not a wake.
- `TestWatcherHonoursSlack`: a gap under the slack is a slow machine, not a sleep.
- `TestWatcherDefaults`: the zero-value settings are usable and quiet.
- `TestWatcherStopsWithContext`: a cancelled context returns at once, no wake.
- `TestHoldReleaseIsConcurrentSafe`: eight goroutines releasing one hold finish.
- `TestHoldEmptyReason`: empty reason, released twice.

`internal/awake/sleepwake_unix_test.go` (`darwin || linux`), using a fake child
built by re-executing the test binary, so no real caffeinate/systemd-inhibit is
needed and every child is one the test started:

- `TestHoldStartsAndStopsOneChild`: exactly one child, alive while held, gone
  after release.
- `TestHoldsAreIndependent`: two holds, two children; releasing one leaves the
  other running.
- `TestReleaseAfterChildExit`: the child dies first; release still returns.
- `TestHoldWithoutHelper`: no helper, release does nothing and returns.
- `TestHoldWhenStartFails`: a helper that cannot start does not hang or panic.

The tests need a seam to substitute the helper, so the per-OS `hold` bodies were
split: `holdCmd` (built in `awake_darwin.go` / `awake_linux.go`) builds the
command, and `hold` in the new `awake_unix.go` supervises it. Windows and other
OSes are unchanged.

## Checks run (in the worktree, before the last commit)

- `gofmt -l .`: empty
- `go build ./...`, `go vet ./...`: pass
- `go test -p 2 ./internal/awake/`: pass (5 new tests on macOS plus 5 fake-child
  tests, all others unaffected)
- `go test -p 2 ./...`: pass
- `GOOS=windows GOARCH=amd64 go build ./...`, `GOOS=windows GOARCH=arm64 go
  build ./...`, `GOOS=linux go build ./...`: pass
- `GOOS=linux go vet ./internal/awake/` and `GOOS=windows go vet
  ./internal/awake/`: pass, so the build-tagged test file compiles for Linux

## What I could not finish

- Wiring `Watcher` into the app. `internal/app/app.go` is read-only for this
  task and is being rewritten tonight; the wake hook belongs next to
  `app.go:592` (the `awake.Hold` call) or in a serve health loop.
- A real sleep/wake on this machine. I did not run `pmset sleepnow` (it would
  sleep the shared Mac), so the detector is proven against a fake clock and the
  real-clock default, not against a suspend.
- Windows: everything is code-read. `powercfg /requests` needs an administrator,
  so `TestHoldWindows` skips here.

## Next steps

1. In the app's job/serve supervisor, run `(&awake.Watcher{}).Run(ctx, func(slept
   time.Duration) { ... })` and on a wake probe the engine's `/health` with a
   short timeout; restart the server if it does not answer.
2. Confirm on Windows whether `PowerCreateRequest` copies the reason string
   (F4); if it does not, keep the UTF-16 buffer alive on the handle.
3. Consider an `Active() bool` on the hold for reporting, if the UI ever wants
   to say "your Mac may sleep during the download" (F7).
