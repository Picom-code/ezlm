# T16/a05: launchd plist handling in the Mac app

## What I reviewed

- `internal/serve/service_darwin.go` (the LaunchAgent writer and loader, owned).
- How the Mac app reaches it: `ezlm engine` (`cmd/ezlm/main.go:225`), the
  `start`/`stop` API (`internal/app/app.go:1379`), and `AppModel.boot()`
  (`macos/Sources/EZLM/AppModel.swift:238`).
- `internal/serve/config.go` (`Config.Args`, `StoppedFlag`) and
  `internal/serve/procs.go` (`OwnPIDs`), to check what the job starts.
- No Swift file in `macos/` mentions launchd, `launchctl`, `LaunchAgents` or
  `SMAppService`: the Mac app has no login-item UI of its own. The plist is
  written by the Go engine on deploy, so the plist is the Mac app's login
  item.

## Findings

### 1. The login agent ignored "Stop server" (major, fixed)

`internal/serve/service_darwin.go:50-69` wrote a job whose `ProgramArguments`
were `[llama-server, flags...]` with `RunAtLoad`. Pressing Stop in the app (or
`ezlm stop`) writes `~/.ezlm/stopped` (`internal/app/app.go:1379-1381`) and
boots the job out, but the plist stays in `~/Library/LaunchAgents`, so the next
login started the server again. The Windows login entry runs
`EZLM.exe --autostart` and `ezlm start --login` checks the flag
(`cmd/ezlm/main.go:164-168`); macOS had no equivalent check.

Evidence: real launchd run on this Mac, a fake server and a hand-built plist
with the old `ProgramArguments` (see Verification). It started once, and after
`bootout` + `touch ~/.ezlm/stopped` + `bootstrap` it started again.

Fix: the job now runs `/bin/sh -c "test -f <stopped flag> || exec <server> <args>"`.
`exec` keeps llama-server as the job's process, so `KeepAlive` still restarts a
crash. A stopped server makes the shell exit 0, which `KeepAlive
SuccessfulExit=false` does not restart. `OwnPIDs` still sees the process (it
matches the executable path, not the parent), so status and stop keep working.
Every argument is single-quoted for the shell and XML-escaped for the plist, so
model paths with spaces or apostrophes survive.

### 2. Stale plists were never repaired (minor, fixed)

`EnsureAutostart` was a no-op on darwin (`service_darwin.go:116`), while
Windows uses it to repair its login entry at every app start
(`internal/serve/service_windows.go:105`). A plist written by an older EZLM
(old llama-server path, old flags, or no stop guard), or one removed by a
cleaner, stayed that way until the next deploy or an explicit Start. The app
only posts `start` when the server is unhealthy (`AppModel.swift:247`), so a
working old server kept the old job indefinitely.

Fix: `EnsureAutostart` now loads the config and rewrites the plist when it is
missing or differs from what today's EZLM would write. It deliberately does not
`bootout`/`bootstrap`: reloading would stop a running server. The file takes
effect at the next login, which is documented in the comment. It skips the
write when `Config.Server` does not exist, so a login never runs a job that
fails and restarts every 10 seconds.

### 3. A failed bootstrap leaves the plist behind (minor, not fixed)

`Install` writes the file and then bootstraps (`service_darwin.go:56-58`). If
`launchctl bootstrap` fails (error 5 after a slow bootout is the common case),
the file stays and launchd loads it at the next login. Harmless now that
finding 2 repairs the file, and the retry loop in `bootstrap` covers the
common race, so I left it.

### 4. Log truncation can cut a dying server's last lines (minor, not fixed)

`Install` calls `os.Truncate(LogPath(), 0)` right after `bootout`. The old
llama-server may still be writing, and the lines a problem report wants (the
crash) are the ones just before the truncate. `StartedAt` also uses the log
mtime as a fallback, so the uptime readout resets. Low impact; a fix belongs
with the log tail code.

### 5. Notes, not bugs

- The app's engine (`ezlm engine`) serves its API on a random loopback port and
  exits when the app quits (`cmd/ezlm/main.go:225-240`); the model server is
  the launchd job and outlives the app. That is the intended design, and it is
  why quitting EZLM does not stop the model.
- Once a model is deployed there is no way in the app to turn the login item
  off; the plist is only removed by `ezlm uninstall`
  (`internal/app/app.go:1590`). Windows behaves the same. Open question for
  product: a "start at login" switch, or a line in Settings.
- `label()` is `com.ezlm.server` plus a suffix derived from `EZLM_HOME`
  (`service_name.go`), so a test instance can never bootout the real install.
  Correct.

## Verification

- Real launchd on this Mac, with a fake server and a temp `HOME`, unique label
  `com.ezlm.a05check.<pid>`: `plutil -lint` OK, job started once; after
  `bootout`, `touch <stopped>`, `bootstrap`, the fake server did not start
  again. This is the old-plist reproduction for finding 1 and the proof that the
  new `/bin/sh` guard works under launchd.
- `go test ./internal/serve -run 'Plist|EnsureAutostart'`: new
  `internal/serve/service_darwin_test.go` covers shell quoting (spaces,
  apostrophes), XML escaping, the stop guard, `KeepAlive`/`RunAtLoad`, and
  `EnsureAutostart` repairing a stale plist, writing a missing one, leaving a
  current one alone, and skipping a missing server binary. It failed before the
  fix (`TestPlistHonoursStop`, `TestPlistQuotesEveryArgument`) and passes after.
  It writes only under `t.TempDir()` with a temp `HOME` and `EZLM_HOME`.
- `gofmt -l .` empty, `go build ./...`, `go vet ./...`,
  `GOOS=linux go build ./...`, `GOOS=windows GOARCH=amd64 go build ./...`,
  `go test -p 2 ./...` all pass. `cd macos && swift build && swift test` pass.

## What needs a real Mac and a reboot

- The whole path: deploy, Stop, log out and back in, confirm the server stays
  stopped; then Start, log out and back in, confirm it starts.
- A plist left by an older EZLM (old `~/.ezlm/bin` layout) healed by opening
  the app once, then logging in.
- App translocation is a03's area; the plist stores absolute paths, so a
  translocated app still points at `~/.ezlm`, which is fine, but `ezlm engine`
  rewriting the plist from the translocated copy is worth a look.

## Files changed

- `internal/serve/service_darwin.go`: `plistXML` and `shQuote` helpers, the
  shell guard, `writePlist`, and a real `EnsureAutostart`.
- `internal/serve/service_darwin_test.go`: new.
