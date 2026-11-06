# T10 / a13 — Linux systemd user unit (`service_linux.go`)

Branch `agent/T10-a13`. Stretch slice: review the generated systemd user unit and the
install/uninstall path, fix what is real, and test the unit content on any OS and the
install/uninstall flow on Linux.

## What I reviewed

- `internal/serve/service_linux.go` (the whole file): `unitPath`, `systemctl`, `quote`,
  `libPath`, `Install`, `Stop`, `Start`, `Uninstall`, `EnsureAutostart`.
- Its neighbours for context: `service_name.go` (`testSuffix`, unit name), `client.go`
  (`LogPath`), `paths.go` (`paths.Home`/`Logs`), `config.go` (`Config.Args`), and the
  launchd and Windows service files to compare restart and uninstall behaviour.
- `internal/app/app.go` call sites of `Install`/`Start`/`Stop` (read only).

## Findings

1. **major — `libPath` walked the whole file system for a flat engine install.**
   Old `service_linux.go:31`: `root` started at `filepath.Dir(server)` and the loop
   advanced until *the parent* was named `bin`. For the flat layout
   (`~/.ezlm/bin/llama-server`, an older install) `dir(server)` is already `bin`, so the
   loop climbed past it to `/` and `filepath.WalkDir("/")` ran on every install/start.
   Evidence: a standalone copy of the old function, given a temp `tmp/bin/llama-server`,
   did not return within 40 s (it was walking `/`). Also returns every `.so` directory on
   the disk as `LD_LIBRARY_PATH`.
   Fixed in `service_unit.go:32` (stop at a directory named `bin`, fall back to the
   server's own directory when there is no `bin` ancestor). Regression test:
   `TestLibPathStaysInTheEngineInstall/flat_install`, plus `no bin directory`.

2. **major — `Environment=LD_LIBRARY_PATH=...` was not quoted.**
   Old `service_linux.go:65`. `Environment=` is a whitespace-separated list of
   assignments, so an install path with a space (`/home/Ann Lee/.ezlm/...`) was split:
   `LD_LIBRARY_PATH` became the first word and the CUDA runtime beside llama-server was
   not found, pushing the model to the processor. Fixed: the whole assignment is quoted
   and specifier-escaped (`service_unit.go:62`). Test: `TestUnitFile` asserts the
   `Environment=` line is one quoted assignment containing the space.

3. **minor — `%` in a path was read as a systemd specifier.**
   Old `service_linux.go:65-68`: `Environment=` and `StandardOutput=`/`StandardError=`
   interpolated `libPath()` and `LogPath()` raw. systemd expands `%` in unit settings
   (`%h`, `%n`, ...), so `~/.ezlm` under a profile such as `/home/100% real` produced a
   bad `LD_LIBRARY_PATH` and possibly a different log path, or a unit that fails to
   load. Fixed with `unitEscape` (`service_unit.go:28`); `quote` already doubled `%`.
   Test: `TestUnitFile` uses an `EZLM_HOME` containing `100% real`.

4. **minor — `Uninstall` swallowed every error.**
   Old `service_linux.go:96-101` ignored the result of `os.Remove(unitPath())` and
   always returned nil, so a unit file that could not be removed (permissions, a
   read-only config dir) still reported success and stayed enabled. Now the remove error
   is returned (a missing file is still fine), `systemctl` stays best effort
   (`service_linux.go:66`). Test: `TestUninstallRemovesTheUnit` (Linux).

5. **minor — the log directory was never created.**
   `StandardOutput=append:<~/.ezlm/logs/server.log>` is opened by systemd, but nothing
   in `Install` created `~/.ezlm/logs`; on a setup where `paths.Ensure` has not run
   yet, the unit can fail to start with a log-open error. Now `Install` creates the unit
   directory (previously ignored error) and the log directory before writing
   (`service_linux.go:31`).

6. **minor — `unitPath` ignored `os.UserConfigDir` errors and a relative
   `XDG_CONFIG_HOME`.** systemd ignores a relative `XDG_CONFIG_HOME`, so the unit was
   written somewhere systemd never reads. Fallback to `~/.config`
   (`service_linux.go:11`).

7. **minor — `After=network-online.target` without `Wants=` is ineffective.**
   Ordering alone does not pull the target in. Added `Wants=network-online.target`
   (`service_unit.go:62`).

### Code-read, not changed

- `Restart=on-failure` with `RestartSec=10` and systemd's default
  `StartLimitBurst=5`/`StartLimitIntervalSec=10s`: restarts are 10 s apart, so the
  rate limit is never reached and a crash loop restarts forever. That matches the goal
  ("survives a crash"); a hard limit would leave the user with no server.
- `loginctl enable-linger` is best effort (may need polkit). Without linger the unit
  starts at login, not at boot. Deliberate, unchanged.
- The unit is written `0600`; `systemd --user` reads it as the user, so fine.
- `StandardOutput=append:<path>` is left unquoted for a path with spaces. Current
  systemd takes everything after `append:` as the path; quoting it would make older
  systemd versions keep the quotes literally in the file name.
- `Stop()` returns an error when the unit is not loaded (fresh machine); callers ignore
  it and `StopAndWait` then waits out its loop. A `systemctl is-active` guard would be
  cleaner, but it changes behaviour outside this slice.

## What changed

- New `internal/serve/service_unit.go` (shared, no OS tag): the unit builder
  `unitFile`, `quote`, `unitEscape`, `libPath` (fixed), `unitName`, `unitPathIn`.
  `service_linux.go` is now only the Linux side: `unitPath` (fixed fallback),
  `systemctl`, and `Install`/`Stop`/`Start`/`Uninstall`.
  Moving the builder out of a `_linux` file is what makes the unit content testable on
  macOS and Windows, matching the launchd-plist approach.
- No behaviour change on darwin/windows: the moved helpers are new names there and the
  platform files still provide `Install`/`Stop`/`Start`/`Uninstall`.

## Tests

- `internal/serve/service_unit_test.go` (runs on every OS):
  `TestQuote`, `TestUnitFile` (quoting, `%` escaping, quoted `Environment=`, restart
  and install sections, log lines), `TestUnitName` (real vs. `-test-<hash>` unit,
  stable and distinct per `EZLM_HOME`), `TestUnitPathIn`,
  `TestLibPathStaysInTheEngineInstall` (versioned, flat, and no-`bin` layouts).
- `internal/serve/service_linux_test.go` (Linux only, filename suffix): fake
  `systemctl`/`loginctl` scripts first on `PATH` that record their arguments, temp
  `HOME`/`XDG_CONFIG_HOME`/`EZLM_HOME`. `TestInstallWritesUnitAndEnablesIt` checks
  the file equals the builder, the log is truncated, and the calls are
  `--user daemon-reload`, `--user enable`, `--user restart`; `TestUninstallRemovesTheUnit`
  checks the unit file is gone and the calls are `disable --now` then `daemon-reload`
  (and a second uninstall succeeds); `TestStartWithoutConfigStartsTheUnit` checks the
  no-config path calls `--user start`.

Proof on this Mac (macOS host): the cross-built Linux test binary was run in a
container, with no unit installed anywhere:

```sh
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -c -o /tmp/a13-serve.test ./internal/serve
docker run --rm -v /tmp/a13-serve.test:/serve.test:ro postgres:16-alpine \
  /serve.test -test.run 'TestInstallWritesUnitAndEnablesIt|TestUninstallRemovesTheUnit|TestStartWithoutConfigStartsTheUnit' -test.v
# PASS (3/3)
```

Quality bar in the worktree: `gofmt -l .` empty, `go build ./...`, `go vet ./...`,
`go test -p 2 ./...`, `GOOS=windows GOARCH=amd64 go build ./...`,
`GOOS=windows GOARCH=arm64 go build ./...`, `GOOS=linux go build ./...` all pass.

## Open questions / not finished

- `systemd-analyze verify` was not run on the generated unit (no Linux machine with
  systemd here, and installing a unit was forbidden). The unit is only compared to the
  documented syntax.
- Whether `StandardOutput=append:` accepts a space in the path on older systemd versions
  is untested; left unquoted as discussed above.
- A real logout/login and reboot test (linger, `default.target`) is still unproven on
  hardware.
