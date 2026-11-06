# T10 a02: launchd restart behaviour

Slice: what launchd does when the server exits, how often it is respawned, and
what happens when the server program moves or is replaced (an update) while the
job is loaded. Everything below is tested against the generated plist and the
`Install` path with a stand-in `launchctl`; nothing was installed on this
machine and no real launchd was touched.

## What I reviewed

- `internal/serve/service_darwin.go` (whole file), `service_name.go`,
  `internal/serve/run.go` (entry point), `internal/engine/engine.go` (where
  the server path comes from), `internal/app/app.go` deploy/restore flows.
- The generated plist's restart keys: `RunAtLoad`, `KeepAlive`
  (`SuccessfulExit` false), `ThrottleInterval` 10, log paths, and the
  `bootout` / `bootstrap` sequencing in `Install`, `Start`, `Stop`.

## Tests added

`internal/serve/restart_darwin_test.go` (darwin only). It puts a fake
`launchctl` on `PATH` that records every call and keeps the job's loaded state
in a marker file, sets a temp `HOME` and `EZLM_HOME`, and drives the real
`Install`. So the test sees the same sequence a real launchd would: `print`
says whether the job is loaded, `bootout` unloads, `bootstrap` loads.

- `TestLaunchdRestartContract` - the plist starts at login (`RunAtLoad`),
  restarts only on an unsuccessful exit (`KeepAlive` dict,
  `SuccessfulExit` false), and never respawns faster than launchd's 10 second
  floor (`ThrottleInterval` 10). Also checks the program arguments, the log
  paths and that one job is loaded.
- `TestInstallReloadsJobWhenTheServerMoves` - an engine update installs a new
  folder and the app re-registers it: the old job is booted out before the new
  plist is bootstrapped, and the plist names the new program. Then an in-place
  replacement (same path, new file) must still reload the job, because launchd
  execs the program only when it loads the job.
- `TestInstallKeepsTheJobWhenTheServerIsMissing` - the fix below: `Install`
  with a program path that does not exist returns an error naming the path,
  leaves the working plist and job alone (no extra `bootout`, no extra
  `bootstrap`).
- `TestBootstrapRetriesWhenLaunchdIsBusy` - launchd answers error 5 for a
  moment after a bootout; `bootstrap` tries three times before reporting.

## Findings

### F1 (major, fixed) - Install loads a job whose program is missing

`service_darwin.go:49` wrote the plist for `c.Server` without checking it
exists, and it called `Uninstall()` first, so a config that still names a
removed engine folder tore down a working job and loaded one that cannot exec.
launchd then fails the exec and retries every `ThrottleInterval` (10 s)
forever, logging an error each time, and the user's server is silently down
until they open the app (which repairs the path in `restoreServer`). Evidence:
`TestInstallKeepsTheJobWhenTheServerIsMissing` fails before the fix with
`Install accepted a server program that isn't there`; it passes after.

Fix: `Install` stats `c.Server` before touching anything and returns
"EZLM's model server is missing at %s: set up the model again to reinstall it".
The working job and plist are left in place.

### F2 (minor, code-read) - a loaded job keeps running the old binary after an engine update

The engine installs into a per-tag folder and leaves the old one behind
(`internal/engine/engine.go:243-267`). The plist is only rewritten when the
app calls `serve.Install` (deploy, Start, restore). Between an engine update
and the next `Install`, the loaded job still runs the previous binary. That is
the intended "don't break a running server mid-update" behaviour, but it means
"the server is up to date" cannot be inferred from the plist. No change.

### F3 (minor, open question) - a clean exit (status 0) is not restarted

`KeepAlive` with `SuccessfulExit` false restarts only after a non-zero exit or
a signal (launchd `launchd.plist(5)`). A loaded job that exits 0 stays down
until the next login. That is deliberate for logout (launchd sends SIGTERM at
logout, and `KeepAlive` true would respawn during logout), and EZLM's own
stops go through `bootout`, which unloads the job. The risk is a server that
exits 0 on a fatal condition: recent llama.cpp builds shut down gracefully and
return 0 on SIGTERM. If that turns out to happen outside a stop, the fix is
`KeepAlive` true; I did not change it without a reproduction, because a
respawn loop at logout is the worse failure. Open question for the Lead.

### F4 (minor, code-read) - `Start` cannot repair a stale plist when the config is unreadable

`Start` (`service_darwin.go:98`) only rewrites the job when `Load()` succeeds;
otherwise it bootstraps whatever plist is on disk. A stale plist that names a
moved program is then loaded as-is. `Install`'s new check does not cover this
path because there is no config to check against. No change.

## Not finished / next steps

- Real launchd behaviour (throttle timing, error 5, respawn after a signal)
  is modelled, not observed: the brief forbids touching launchd here. A manual
  check on a scratch Mac would confirm F3.
- `Install` still tears the job down before writing the new plist, so a failed
  `os.WriteFile` (disk full, read-only LaunchAgents) leaves the service
  unloaded and the plist gone. Writing to a temp file and renaming before
  `bootout` would fix it; it overlaps a03's unload/reload slice, so I left it.
- Linux has the same missing-program hole in `service_linux.go` (`Install`),
  which a13 owns.
