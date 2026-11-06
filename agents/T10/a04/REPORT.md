# T10 a04: Windows login task (Run key) and legacy Task Scheduler cleanup

## Scope and what is actually there

The snapshot no longer generates a Task Scheduler XML file or a PowerShell
`-EncodedCommand`. `internal/serve/service_windows.go` now installs a per-user
`HKCU\Software\Microsoft\Windows\CurrentVersion\Run` entry (a command line,
not XML), and only *cleans up* the old scheduled tasks. `taskXMLPath` and
`activeTaskPath` are leftovers that are removed, never written.

So this slice reviewed: the Run key command line (quoting, working directory,
log path, restart behaviour), the legacy task cleanup, and the old task entry
point (`RunForeground`). There is no Windows machine here: everything that
needs Windows to execute is marked **code-read** below.

## Tests added

`internal/serve/windows_task_test.go` (runs on any OS, because the command
builder is now portable):

- `TestRunKeyCommand`: app path with spaces and unicode, engine fallback,
  unicode profile without spaces, `--autostart` as a separate argument.
- `TestEscapeArg`: empty, plain, spaces, unicode, trailing backslash before a
  closing quote, embedded quote.
- `TestRunKeyCommandSeparatesFlag`.

`internal/serve/runkey_command.go` holds `runKeyCommand` and `escapeArg`.
`escapeArg` is Go's `syscall.EscapeArg` algorithm (BSD licence) copied so the
exact Windows command line can be asserted on macOS. The Windows build still
uses the same rules, so this is not a behaviour change.

Evidence:

```
$ go test -p 2 ./internal/serve/ -run 'TestRunKey|TestEscapeArg' -v
--- PASS: TestRunKeyCommand
--- PASS: TestEscapeArg
--- PASS: TestRunKeyCommandSeparatesFlag
```

`GOOS=windows GOARCH=amd64 go build ./...` and `GOARCH=arm64` pass, as do
`go build ./...` and `go vet ./internal/serve/`.

## Findings

### 1. Major (code-read, not fixed): no restart after a crash on Windows

- `internal/serve/service_windows.go:83-100` registers only a Run key. A Run
  key runs once at sign-in. There is no `RestartOnFailure` equivalent.
- Compare `internal/serve/service_darwin.go:63-71` (`RunAtLoad`, `KeepAlive`
  with `SuccessfulExit=false`, `ThrottleInterval`) and
  `internal/serve/service_linux.go:61-62` (`Restart=on-failure`,
  `RestartSec=10`).
- The `--autostart` path is `windows/EZLM/Program.cs:30-40`: it starts
  `Engine\ezlm.exe start --login`, waits up to ten minutes, then exits. The
  engine's `StartSync` (`internal/app/app.go:1430`) returns once the server is
  healthy, so nothing is left watching the server.
- Effect: if llama-server exits while the user is logged in (driver reset, out
  of memory, a bad request path), the model server stays down until the next
  login. launchd and systemd restart it. This is a real regression from the
  old Task Scheduler task, which could carry restart-on-failure settings.
- Why not fixed here: the fix needs a long-lived supervisor process. The
  natural place is `ezlm start` keeping the child alive, but `cmd/**` is
  being rewritten tonight and is outside this task's owned paths. Suggested
  next step: a small `ezlm supervise` (or `start --keep-alive`) that waits on
  the child and restarts it with a backoff, registered by `setRunKey`; or a
  per-user Task Scheduler task with `<RestartOnFailure>` if normal users can
  create one (the comment at `service_windows.go:64-68` says they get Access
  denied).

### 2. Minor (fixed): `EnsureAutostart` disabled the only working login entry

`internal/serve/service_windows.go:105-125` wrote the Run key, ignored the
error, and then disabled the legacy scheduled tasks. If the registry write
failed, the user was left with neither the Run key nor the old task: no
autostart at all. It now returns before disabling anything when `setRunKey`
fails (`:116-120`). Windows-only, so code-read: the registry write is not
hermetic on this Mac.

### 3. Minor (code-read): stale Task Scheduler text and dead paths

- `internal/serve/run.go:10-13` said Windows uses `RunForeground` as the
  scheduled task entry point. No task XML is generated now; `ezlm run-server`
  is the only caller. Comment corrected.
- `internal/serve/service_windows.go:158` said `Start` runs "the task". There
  is no task; the Run key fires only at login, so `Start` is the direct start.
  Comment corrected.
- `taskXMLPath()` (`:55`) and `activeTaskPath()` (`:38`) are only ever
  removed, never read or written. They are harmless legacy cleanup, kept on
  purpose; no change.
- `NoLoginTask` (`:40-42`) is set in `Install` but never read anywhere in the
  Go tree (grep). A failed login entry is therefore invisible to the user.
  Kept as is; the app should read it or it should be removed.

### 4. Quoting (tested, no bug found)

`setRunKey` now calls `runKeyCommand` (`service_windows.go:94-95`). The old
code used `syscall.EscapeArg` directly; the copied `escapeArg` matches it.
Paths with spaces and non-ASCII letters are quoted correctly, and
`--autostart` is a separate argument. A path with no space is left unquoted,
which is correct for `CommandLineToArgvW`. Note that the Run key value is
launched by Explorer, not `cmd.exe`, so `&`, `^`, `%` and parentheses need no
quoting; none of them are treated as special here.

### 5. Working directory (code-read, no bug)

The Run key sets no working directory. The engine resolves everything through
`internal/paths` (`os.UserHomeDir` or `EZLM_HOME`), and `startLogged` sets
`cmd.Dir = filepath.Dir(server)` (`service_windows.go:190`), so the CUDA DLLs
beside `llama-server.exe` are found regardless of the inherited cwd. The C#
`--autostart` launcher passes an absolute engine path
(`windows/EZLM/Program.cs:34`).

### 6. Log path (code-read, covered by an existing Windows test)

`LogPath()` is `%USERPROFILE%\.ezlm\logs\server.log`
(`internal/serve/client.go:21`). `startLogged`
(`service_windows.go:179-203`) opens the file itself and hands the handle to
llama-server, so a profile such as `C:\Users\José Müller\` does not hit
llama.cpp's ANSI `fopen`. `internal/serve/startlog_windows_test.go` covers
that folder shape; it runs only on Windows.

### 7. Legacy task cleanup (code-read, no bug)

`taskNames` (`:18`) covers "EZLM Server" and "EZLM Server (<user>)".
`legacyTasks()` returns nothing when `EZLM_HOME` is set, so a test instance
never ends or deletes the real install's task. `Install` deletes with
`schtasks /Delete /F`, falls back to `/Change /DISABLE`; `Stop` ends with
`/End`; `Uninstall` deletes. Process stops are by PID (`killOwn`, `:147-154`),
never by name.

## Open questions

1. Is "no restart until next login" acceptable for the Windows beta? If not,
   which supervisor shape is preferred (engine loop vs per-user task)?
2. Can a normal (non-admin) user create a per-user Task Scheduler task with
   restart-on-failure on current Windows? The code comment says no; if that is
   true, only an engine-side supervisor fixes finding 1.
3. Should `NoLoginTask` be surfaced in the app's status, or removed?

## Commands run

```
gofmt -l internal/serve (empty)
go test -p 2 ./internal/serve/ -run 'TestRunKey|TestEscapeArg'
go build ./...
go vet ./internal/serve/
GOOS=windows GOARCH=amd64 go build ./...
GOOS=windows GOARCH=arm64 go build ./...
```
