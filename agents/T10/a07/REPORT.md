# T10 / a07: a different program already on the port

Slice: a port conflict with a *different* program already listening. EZLM must
not adopt it and must not kill it. Test file: `internal/serve/portforeign_test.go`
(one file for this slice). Fix: `internal/serve/client.go` (`Ours`, 3 lines).

## What I did

- Wrote tests that use a **real TCP listener** (a real `http.Server` on a real
  port, not a mock) for the "other program" cases, and a **real process** (the
  test binary copied to `llama-server` and re-exec'd, listening on a port) for
  the "must not kill it" case.
- Found and fixed one adoption bug in `Ours`; a second bug (`StopAndWait`
  refusing to stop) is a consequence of the same one and is proved by a test.
- Ran the tests before the fix to prove they fail, and after to prove they pass.

## Findings

### 1. Major: another program on the port is adopted as EZLM's server

`internal/serve/client.go:58-60` (before the fix): when `/v1/models` answers
200 with our bearer key but the body is not JSON, or is JSON with no `data`
entries, `Ours` returned `true` ("accepted our key; nothing to compare").
Any program that ignores the `Authorization` header and answers 200 (a dev
server, a proxy, a stub, a JSON API on 8080) was therefore taken for EZLM's
own server. `Healthy` (client.go:26-36) inherits it.

Consequences:
- `StartSync` (`internal/app/app.go:1431-1433`) sees `Healthy` and returns nil
  without starting anything: EZLM never starts its server and reports the
  other program as up. `Status` (`internal/app/app.go:478-480`) then sets
  `Unmanaged`.
- `StopAndWait` (`internal/serve/config.go:201-213`) uses `Ours` to decide
  "our server is still answering", so it cannot tell a stopped EZLM server
  from a foreign program on the port (finding 2).

Evidence: `TestForeignListenerIsNotOurs` (cases `empty model list`, `json but
no models`, `not json`) failed before the fix with
`Ours() adopted the program on port N` / `Healthy() adopted the program on
port N`, and passes after. `TestOwnServerOnListenerIsOurs` proves a real
server with our key and alias is still recognised.

Fix: a body that lists no model proves nothing; require our model alias in
the list. Only a config with no `ModelID` (an install from before model ids)
still goes on the key alone, which keeps that case working.

### 2. Major: a foreign program on the port makes "stop" fail

`internal/serve/config.go:208`: `PortFree(c.Port) || !Ours(ctx, c)`. With
finding 1, a foreign program that answers 200 keeps `Ours` true, so
`StopAndWait` waits its full 10 seconds and returns `ErrStillRunning` ("...
restart your computer, or end \"llama-server\" in Task Manager ...") even
though EZLM's own server is gone. That blocks:

- Windows `Install` -> `StopAndWait` (`internal/serve/service_windows.go:58`),
  so a user with a dev server on the port cannot deploy at all;
- `SetPort` (`internal/app/app.go:1509-1513`) and the model-swap path
  (`internal/app/app.go:774`).

Evidence: `TestStopAndWaitLeavesForeignServerAlone` failed before the fix with
`StopAndWait reported "the running model server didn't stop..."` after
11.69 s; after the fix it returns nil at once and the other program is still
listening and answering `/health` (also proves EZLM did not kill it).

### 3. Minor / open: `isOwnExe` compares paths textually

`internal/serve/procs.go:35-41` uses `strings.HasPrefix(exe, home+sep)`.
Found while writing the process test: `t.TempDir()` is under `/var/folders/...`
but the process's own exe path (macOS `proc_pidpath`) is
`/private/var/folders/...`, so EZLM's own server was invisible to `OwnPIDs`
until the test resolved the symlink. A real install under `~/.ezlm` is not
affected, but a profile reached through a link is (the same risk is already
noted in `internal/serve/client.go:87-89`). Not changed here (it belongs with
the PID/registry slices); the test resolves the temp path and says why.

### 4. Code-read (Windows): a foreign llama-server is never killed

`killOwn` (`internal/serve/service_windows.go:147-154`) only kills PIDs from
`OwnPIDs`, which requires the process name to start with `llama-server` **and**
the exe to live under `paths.Home()` (procs.go:14-30). Another app's
llama-server, or the user's own, is never in that list. On macOS/Linux stop
goes through launchd/systemd labels, which are per EZLM home, so they cannot
touch a foreign listener either. `TestOwnPIDsIgnoresForeignLlamaServer`
proves the gate on macOS with a real process: a copy of `llama-server`
outside `EZLM_HOME` is not in `OwnPIDs`; the same copy inside it is.

## What changed

- `internal/serve/client.go`: `Ours` no longer accepts a 200 with an
  unreadable or empty model list; requires the configured model alias, except
  when the config has no `ModelID`.
- `internal/serve/portforeign_test.go`: the tests above. Real listeners, real
  process, temp `EZLM_HOME` for every test, nothing from the real home.

## Checks

`gofmt -l .` empty, `go build ./...`, `go vet ./...`,
`go test -p 2 ./internal/serve/`, `GOOS=windows GOARCH=amd64 go build ./...`
(see the commit message / final message for the exact run).

## Open questions

- Should `Healthy` also require an own PID? An admin-started server is
  invisible to `OwnPIDs` (comment at `internal/app/app.go:1432`), so HTTP
  identity is the only signal there. Left as is.
- Another EZLM install (different `EZLM_HOME`) with the same key and model on
  the port is indistinguishable over HTTP and would still be adopted. Low risk;
  the key is per install.
