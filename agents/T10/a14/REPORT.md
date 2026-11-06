# T10 a14: the process registry

## What this slice is

How EZLM knows what it started: `internal/serve/procs.go` (`OwnPIDs`,
`isOwnExe`, `OwnRSS`), the birth-time helpers (`birth_darwin.go`,
`birth_other.go`) behind `StartedAt`, concurrent starts of the same server,
and the single-instance guarantee.

The registry is a scan, not a record: EZLM writes no PID or birth file. A
process counts as EZLM's server when its name starts with `llama-server` and
its executable lives inside `paths.Home()` (`~/.ezlm`, or `EZLM_HOME`). Every
status check, stop and memory readout goes through it
(`cmd/ezlm/main.go:595`, `internal/app/app.go:433,478,1393,1428,1480,1591`,
`internal/app/live.go:59`, `internal/app/settings.go:144`,
`internal/serve/client.go:89,221`, `internal/serve/config.go:208`,
`internal/serve/service_windows.go:149,160`).

## Tests added

`internal/serve/registry_test.go` (all hermetic, temp `EZLM_HOME`):

- `TestOwnPIDsFindsOnlyOurServer`: builds a real fake `llama-server` binary
  in the temp home and one outside it, starts both, and proves only the one
  inside the home is in `OwnPIDs`. Kills both by PID.
- `TestOwnPIDsThroughLinkedHome`: `EZLM_HOME` is a symlink to the real home;
  the server's kernel-reported executable is the real path. This failed before
  the fix (8 s of polling, then "never saw").
- `TestOwnPIDsConcurrentReads`: eight simultaneous registry reads agree on the
  one running server.
- `TestOwnRSS`: a live fake server has RSS > 0; a missing PID contributes 0.
- `TestIsOwnExePaths`: look-alike folders (`.ezlm-old`, `.ezlmx`), a trailing
  separator in the home, and a server outside the home.
- `TestIsOwnExeResolvesLinks`: `isOwnExe` matches whether the caller holds
  the linked or the resolved form of either path.

## Findings

### 1. The registry lost EZLM's own server when the home was reached through a link (major, fixed)

`internal/serve/procs.go:35` compared `filepath.Clean`ed strings: the home
exactly as configured against the executable path the kernel reports, which is
always resolved. Any symlink in either path made them unequal, so
`OwnPIDs` returned nothing for a server EZLM had started. This is not exotic
on macOS: `t.TempDir()` (and `/tmp`, and `/var`) is a link to `/private/var`,
so the plain fake-process test failed before the fix. A moved `~/.ezlm` on
another volume, a home directory on a linked volume, or `EZLM_HOME` pointing
through a link all hit it. The code already knew the symptom: `client.go:86`
says "a profile reached through a link can hide our processes from OwnPIDs".

Consequences with a live server: `WaitHealthy` counts it as gone after
`stoppedGrace` and reports "the model server stopped" (`client.go:91`);
`StopAndWait` waits 10 s and returns `ErrStillRunning`, telling the user to
restart the computer (`config.go:196,208`); `App.Start` thinks nothing runs
and starts a second server on the same port (`app.go:1393`), which on Windows
can share the port (see the `SO_REUSEADDR` note at `config.go:140-148`), and
the two processes then make `StartSync`'s "crashed and was restarted" check
(`app.go:1480-1490`, order of `pids[0]`) unstable.

Fix: `realPath` resolves symlinks, including the deepest existing parent for
a path that is gone, and `isOwnExe` uses it for both sides. Paths are made
absolute first. No behavior change for ordinary installs.

Evidence: `TestOwnPIDsThroughLinkedHome` and `TestIsOwnExeResolvesLinks` fail
on the pre-fix code (the first also fails for `TestOwnPIDsFindsOnlyOurServer`
on macOS because of `/var`) and pass after.

### 2. Concurrent starts are not serialised across processes (major, code-read on Windows, not fixed)

`internal/serve/service_windows.go:159-168` does check-then-act: if
`OwnPIDs` is empty, `startLogged` launches `llama-server.exe`. Two EZLM
processes can both pass the check (the Run key starts the app with
`--autostart` while the user opens it, or `ezlm start` runs at the same
time), and both start a server. On Windows the second can bind the same port
because llama-server sets `SO_REUSEADDR`, and the two then share it
(`config.go:140-148`). Nothing in `internal/serve` takes a lock across
processes, and no PID/birth file records the start; `App.Start`'s `warming`
flag (`app.go:1393-1405`) only serialises goroutines in one process.

macOS has the same shape without the duplicate: `Start` calls
`Install` → `Uninstall` (bootout) → write plist → `bootstrap`
(`service_darwin.go:49-105`). Two concurrent starts interleave bootout and
bootstrap; the loser's `bootstrap` fails with launchd error 5 after three
tries (`service_darwin.go:35-45`), so the user gets "couldn't start" while a
server is in fact running.

Suggested fix (needs launchd/schtasks to prove, so left out of this
time-boxed slice): take an exclusive lock file in `paths.Home()` around
`Install`/`Start`, or after a bootstrap error check `OwnPIDs`/`Healthy` and
treat a running server as success. A PID+birth file written by EZLM before
launch would also give `StartedAt` and stop a precise answer, but launchd
starts llama-server directly, so EZLM would have to read the PID back from
`launchctl print`.

### 3. The name test is a heuristic (minor, documented)

`procs.go:22` accepts any process whose name starts with `llama-server`
and whose executable is under the home. A `llama-server-custom` build or a
server the user started by hand from `~/.ezlm/bin` counts as EZLM's and is
stopped by `Stop`. That is the intended trade-off ("inside EZLM's install
directory"), but worth stating: the home is the identity, not the start.

### 4. PIDs are returned without a birth time (minor, a08's fix)

`OwnPIDs` returns bare PIDs; `killOwn` (`service_windows.go:147-154`) and
`StartedAt` (`client.go:221`) act on them later. A PID reused between the
scan and the kill would be the wrong process. Recorded here; the stop-by-PID
slice (a08) owns the fix (match `CreateTime` before killing).

### 5. `StartedAt` fallback is a last-write time (minor)

`client.go:217-237` uses the newest own process's create time when one is
running, which is correct. With no own process it falls back to the log's
birth time on macOS (`birth_darwin.go`) and to its `ModTime` elsewhere
(`birth_other.go`), which is the last line written, not the start. Only used
for the uptime display (`app/live.go:70`), so it is cosmetic.

## What I could not finish

- A real cross-process single-instance lock (finding 2). It needs the service
  managers to prove, and installing launchd jobs on this machine is out of
  scope for the slice.
- A birth/PID file registry. It would make tracking exact, but it changes the
  start path on all three platforms and was too large for the time-box.

## Verification

`gofmt -l .`, `go build ./...`, `go vet ./...`,
`go test -p 2 ./internal/serve/ ./internal/proc/`, and
`GOOS=windows GOARCH=amd64 go build ./...` pass (see the final commit).
