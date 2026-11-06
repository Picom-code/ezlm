# T10/a03: launchd unload/reload and leftover jobs

Slice: installing twice, uninstalling, and a stale plist that still names the
old server path. macOS only; the Windows and Linux service code was not
touched.

## What I reviewed

- `internal/serve/service_darwin.go` (the whole file): `label`, `plistPath`,
  `bootout`, `bootstrap`, `Install`, `Start`, `Stop`, `Uninstall`.
- `internal/serve/service_name.go` (`testSuffix`): the label is
  `com.ezlm.server` plus a hash of `EZLM_HOME` for test instances.
- The call sites in `internal/app/app.go` (read only): `Install` after a
  deploy, `Install(old)` to restore, `Start`/`Stop`, and `Uninstall`.

## Tests added

`internal/serve/service_darwin_test.go` (build tag `darwin`). The tests put a
small fake `launchctl` shell script first on `PATH`. The fake models the parts
of launchd the code uses: a job is loaded or not, `bootstrap` refuses an
already loaded label with error 5, `bootout` only succeeds for a loaded job,
and `print` tells whether the job is loaded. The real launchd is never
touched. `HOME` and `EZLM_HOME` are temp dirs.

- `TestInstallTwiceLeavesOneJob`: install with server path A, then with B.
  Exactly one job is loaded, the plist names B and not A, and every
  `bootstrap` is preceded by a `bootout`.
- `TestUninstallRemovesJobAndPlist`: the job is unloaded, the plist is gone,
  and a second `Uninstall` does not fail.
- `TestInstallReplacesStalePlistWithOldPath`: a plist written by an older
  install (old server path) is already loaded; `Install` boots it out, replaces
  the file, and loads the new one.
- `TestStartDropsStalePlistWhoseProgramIsGone`: no settings, a stale plist
  whose program path does not exist. `Start` must fail with the missing path,
  remove the plist, and not bootstrap it. Failed before the fix.
- `TestStartReloadsAPlistWhoseProgramExists`: the normal no-settings Start
  still reloads a plist whose program is there.

## Findings

1. **Major, fixed. A stale plist with a missing program is loaded forever.**
   `Start` (`internal/serve/service_darwin.go:98-105` before the fix) with no
   `config.json` called `bootout`, truncated the log, and bootstrapped whatever
   plist was on disk, returning nil. If the plist named a program that had
   moved or been deleted (the folder was cleaned up, or an update moved the
   app), launchd loaded the job and `KeepAlive` retried the missing program
   every `ThrottleInterval` (10 s) until logout, while `Start` reported
   success. Evidence: `TestStartDropsStalePlistWhoseProgramIsGone` fails on
   the baseline ("Start loaded a plist whose program is missing") and passes
   after the fix. Fix: read the program out of the plist (`plistProgram`), and
   when it is missing, `Uninstall` (unload and remove) and return an error
   that names the path and tells the user to set the model up again.

2. **Minor, not fixed. `bootout` gives up silently after 10 s.**
   `bootout` (`service_darwin.go:25-33`) polls `launchctl print` 50 times
   (200 ms) and returns with no error either way. `Uninstall` then removes the
   plist even if the job is still loaded, so the server keeps running with no
   plist, and a later `Install` bootstraps a label that is already loaded and
   fails with launchd error 5. With the current code the failure surfaces as a
   confusing "launchctl bootstrap: Bootstrap failed: 5: Input/output error"
   from `Install`. Next step: make `bootout` return an error when the job is
   still there, propagate it from `Uninstall`, and let `Install` report it
   before writing the new plist. This needs the wait count/duration as package
   variables so the test does not take 10 s.

3. **Minor, not fixed. Leftover plists for other labels are never removed.**
   `Uninstall` removes only `plistPath()` for the current label
   (`service_darwin.go:108-114`). A plist left by an install that used a
   different `EZLM_HOME` (each one gets its own `-test-<hash>` suffix, see
   `service_name.go:13-19`) stays in `~/Library/LaunchAgents` and launchd
   keeps starting that server at login. Next step: on a real uninstall
   (`testSuffix() == ""`) also boot out and remove
   `com.ezlm.server-test-*.plist`. A test instance must not do this, or it
   would remove the real install's job.

4. **Minor, not fixed. The plist is not written atomically.** `Install`
   removes the old plist, then `os.WriteFile`s the new one
   (`service_darwin.go:80-84`). A crash or a full disk between the two leaves
   the user with no login service. Next step: write a temp file in the same
   folder and `os.Rename` it over the plist.

5. **Info. `Install` ignores the error from `Uninstall`** (`service_darwin.go:80`).
   If the unload fails, the install continues and fails later at `bootstrap`;
   the message is about launchd error 5 rather than about the unload. Related
   to finding 2.

## What the tests show works

Installing twice replaces the job rather than adding a second one; the plist
always names the latest server path; the old job is booted out before the new
one is bootstrapped; uninstalling unloads and removes; uninstall is
idempotent; a stale plist from an older install is replaced.

## Open questions

- Is there a launchd label older than `com.ezlm.server` in the wild (the module
  is `github.com/picom-code/ezlm`)? If so, `bootout`/`Uninstall` can never
  remove it, and a sweep of `com.ezlm.server*` would not match it either.
- Should a real uninstall remove test-instance plists (finding 3), or is that
  too surprising while a developer's test server is running?

## Verification

`gofmt -l .` empty; `go build ./...`; `go vet ./...`; `go test -p 2
./internal/serve/ ./internal/proc/ ./internal/awake/`; `GOOS=windows
GOARCH=amd64 go build ./...` and `GOOS=windows GOARCH=arm64 go build ./...`
all pass (see the commit for the exact run).
