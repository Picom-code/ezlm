# T10 / a10: orphans after a crash

Slice: the engine dies unexpectedly, EZLM restarts, and the old llama-server
must be found and handled (or deliberately left), never duplicated silently.
Fake processes only; no model or service was started on this machine.

## What I reviewed

- `internal/serve/procs.go` — `OwnPIDs`, `OwnRSS`, `isOwnExe`: the only way
  EZLM recognizes a server it started, and the gate before every stop.
- `internal/serve/config.go` `StopAndWait` and `client.go` `Ours`,
  `Healthy`, `StartedAt` — the callers that turn "is it ours?" into adopt,
  stop, or start another.
- `internal/serve/service_{darwin,linux,windows}.go` — `Start`/`Stop`/
  `Install` on each platform, read for the restart-after-crash path.
- `internal/app/app.go` start and status paths (read only): `Start`,
  `StartSync`, `Status`, `Uninstall`.

## Tests added

`internal/serve/orphan_test.go` plus a helper program
`internal/serve/testdata/fakeserver/main.go`, built into a temp EZLM home by
the test and named `llama-server` so it looks exactly like the real server.
Each test starts it, leaves it running the way a crash would (the test still
stops it by PID at cleanup), and asks who owns it.

- `TestOwnPIDsFindsServerLeftByACrash`: a live server started from EZLM's
  home is found after the "engine" is gone.
- `TestOwnPIDsFindsServerThroughASymlinkedHome`: the same when EZLM's home
  is reached through a symlink.
- `TestOwnPIDsClaimsOnlyEZLMsLlamaServer`: another app's `llama-server` and
  a differently named program inside EZLM's home are not claimed (the safety
  side of the fix: it must not start killing neighbours).

All three are hermetic (`t.TempDir`, temp `EZLM_HOME`, no network, no real
home). The first two fail on the baseline code; see below.

## Findings

### 1. A crashed engine's server is invisible when EZLM's home has a symlink (major, fixed)

`internal/serve/procs.go:25` (baseline): `OwnPIDs` compared
`p.ExeWithContext(ctx)` against `paths.Home()` with a plain string prefix.
The kernel reports a process's *real* path, with links resolved. On macOS the
same home comes back different: `t.TempDir()` gives `/var/folders/...` while
the running process reports `/private/var/folders/...` (`/var` is a link to
`/private/var`). The same happens when `~/.ezlm` is moved to another drive
and linked, or when `EZLM_HOME` is a path under `/tmp`.

Evidence (baseline, live process, PID printed by the test):

```
a server running from /var/folders/.../bin/llama-server (pid 60865) is EZLM's own,
but OwnPIDs = []; a restart would start a second one
```

Consequences after an engine crash and restart, all reachable from the one
invisible PID:

- `internal/app/app.go:1428` (`StartSync`) sees no own PID. If the config
  changed (new model or key), `Ours` is false, so EZLM decides "another
  program holds your port", moves to a new port and starts a second server
  while the first still holds the model in memory. That is the silent
  duplicate this slice is about: two servers, double memory, one of them
  unmanaged.
- `Status` reports `Unmanaged = true` (`app.go:480`): the user is told
  another program started their server.
- `serve.Stop()`/`StopAndWait` never sees it, so a stop waits 10 s and then
  fails with `ErrStillRunning` ("restart your computer").
- `App.Uninstall` deletes `~/.ezlm` under a running server.

Fix (`internal/serve/procs.go:14-60`): `OwnPIDs` now compares against
EZLM's home *and* the path its links resolve to (`ownHomes`, using
`filepath.EvalSymlinks`), and resolves the executable's own path before the
prefix test (`ownExe`). `isOwnExe` keeps its signature and its test, so the
"inside home, never a look-alike folder" rule is unchanged. This only makes
EZLM find more of its *own* processes; the name and directory checks are the
same, and the third test proves an outside server is still left alone.

### 2. A stale server is adopted without checking it matches the config (minor, not fixed: app code)

`internal/app/app.go:1428`: `StartSync` returns success as soon as any own
PID exists. After a crash in the middle of a deploy the surviving process can
serve the old model on the old port while `config.json` already names the new
one. The app then shows "starting" indefinitely (`Status`: `Healthy` false,
`Starting` true) and never reconciles. The fix belongs in `internal/app`
(read-only for this task): compare the running server's `/v1/models` and port
with the config, and stop or adopt it deliberately.

### 3. darwin and linux `Start()` do not check for a running server (minor, code-read, not fixed)

`internal/serve/service_windows.go:159` returns early when `OwnPIDs` is
non-empty; `service_darwin.go:98` and `service_linux.go:90` go straight to
`Install`, which boots the service out and starts it again. No duplicate
results (launchd/systemd stop the old job), but a server that was healthy is
reloaded, and the three platforms do not share one contract. Callers currently
guard with `OwnPIDs`, so this is latent. Left alone because changing it
changes service restart semantics owned by a01/a03/a13.

### 4. A server started outside the service manager cannot be stopped (minor, code-read, not fixed)

`internal/serve/service_darwin.go:90`: `Stop` is only `bootout()`.
`serve.RunForeground` (`run.go:11`) starts llama-server as a plain child, so
a crashed `ezlm run-server` leaves a process that launchd does not know.
`OwnPIDs` finds it, `Start` adopts it, but `Stop` never kills it and
`StopAndWait` ends in `ErrStillRunning`. Windows handles this (`killOwn`,
`service_windows.go:149`); linux has the same hole as darwin. Adding a
PID kill to darwin `Stop` is small, but if `bootout` ever fails to unload the
job, launchd would restart the killed server; that needs its own test with a
fake `launchctl`, so it is left as a next step rather than done blind.

## Checks

`gofmt -l .` empty, `go build ./...`, `go vet ./...`,
`go test -p 2 ./...` (all packages ok), and the Windows amd64/arm64 and
linux cross builds pass. Windows-only paths are code-read.

## Open questions

- Should `StopAndWait` (T07's `config.go`) own the "stop anything of ours that
  the service manager does not know about" fallback, so darwin, linux and
  windows behave the same?
- Should a restart with a mismatched running server stop it and start fresh,
  or adopt it and report the difference? Finding 2 needs a product answer.
