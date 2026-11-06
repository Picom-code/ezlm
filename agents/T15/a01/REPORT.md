# T15/a01: long paths (>260, `\\?\`, UNC)

Agent a01 of T15 (Windows code review). Slice: long paths, the `\\?\` prefix, UNC
paths, and every place a path is built. Branch `agent/T15-a01`.

No Windows machine was available: every finding below is either proved by a test
that compiles under `GOOS=windows`, proved by reading the Go 1.27 runtime source
in `$(go env GOROOT)`, or marked **code-read** and left for the Windows checklist.

## What changed

1. **`internal/paths/long_windows.go`** (new): `Extended(p string) string`
   returns the extended-length form Windows needs for paths of 248 bytes or
   more: `\\?\C:\...` for a drive path, `\\?\UNC\server\share\...` for a
   network share. Short, relative, already-extended (`\\?\`, `\??\`) and device
   (`\\.\`) paths are returned unchanged. The path is cleaned by hand (separators
   normalized, `.`/`..` resolved) because `\\?\` turns off the Win32 normalizer,
   and because the Mac build must not use Windows `filepath` rules for the test.
2. **`internal/serve/service_windows.go:191-198`**: `startLogged` now starts
   llama-server with `cmd.Path = paths.Extended(server)` and
   `cmd.Dir = paths.Extended(filepath.Dir(server))`. `cmd.Args[0]` keeps the
   plain path, so the server's own messages and `argv[0]` do not show the
   prefix.
3. **`internal/paths/long_windows_test.go`** (new): table test for `Extended`
   (short path, 247/248 boundary, drive path, UNC, already-prefixed, device,
   `..`, forward slashes, relative, drive-rooted). Windows-only file; compiled
   with `GOOS=windows GOARCH=amd64 go test -c ./internal/paths`.

Proof on this Mac: the same source and test, with only the package line changed,
run under darwin (`go test` in a scratch module) and pass. That covers the
string logic; the `CreateProcess` behaviour it feeds still needs Windows.

Checks run: `gofmt -l .` empty, `go build ./...`, `go vet ./internal/paths
./internal/serve`, `GOOS=windows GOARCH=amd64 go vet ./internal/paths
./internal/serve`, `GOOS=windows GOARCH=amd64 go build ./...`, `GOOS=windows
GOARCH=arm64 go build ./...`, `GOOS=linux go build ./...`.

## Findings

### F1 (major, fixed) `os/exec` never adds `\\?\`, so a long server path cannot start

`internal/serve/service_windows.go:191` (`startLogged`, the normal Windows
server start path) ran `exec.Command(server, ...)` where `server` is
`%USERPROFILE%\.ezlm\bin\llama-server.exe`. Go's `os` package adds the
extended prefix itself (`GOROOT/src/os/file_windows.go:161,227` call
`fixLongPath`), but `os/exec` hands the name to `syscall.StartProcess`, which
calls `CreateProcess` with the path unchanged (`GOROOT/src/syscall/
exec_windows.go:307-441`; there is no `fixLongPath` there, and
`GOROOT/src/os/exec_windows.go` does not add one). On a machine that has not
set `LongPathsEnabled`, a server or `EZLM_HOME` under a deep folder therefore
fails with "The system cannot find the path specified" even though `os.Stat`
on the same path succeeds.

Evidence: Go runtime source as cited; the fix is the `cmd.Path` change above.

### F2 (major) the model and draft paths handed to llama-server are not extended

`internal/serve/config.go:117` and `:132` pass `-m <ModelPath>` and
`-md <DraftPath>` to llama-server. The model folder is user-picked
(`internal/app/settings.go:96-103` `StorageDir`) and GGUF names are long, so a
path over 260 is realistic. llama.cpp opens these with plain `fopen`, which is
exactly the problem the log file already works around by handing over an open
handle (`internal/serve/service_windows.go:174-178` comment: llama.cpp's fopen
cannot even open a non-ANSI name). Nothing in `Args()` adds `\\?\`.
Severity: major, models silently fail to load with a long path.

Proposed fix: in `Config.Args()` (Windows only) pass `paths.Extended(c.ModelPath)`
and the same for `DraftPath`. Caveat to verify on Windows: llama.cpp must accept
the prefix (Win32 `fopen`/`ifstream` generally do, but the path must be
absolute and clean, which `Extended` guarantees). If it does not, the fallback is
a short 8.3 path via `GetShortPathNameW`. Not changed here: `config.go` is not a
`*_windows.go` file and is outside this task's owned paths.

### F3 (major, code-read) the Run key command line stops at 260 characters

`internal/serve/service_windows.go:88-92` (`setRunKey`) writes
`"<exe>" --autostart` to `HKCU\...\Run\EZLM`. Windows documents the Run/RunOnce
command line as limited to 260 characters (the value is stored, but the logon
launch truncates or skips it). A deep install (`%LOCALAPPDATA%\Programs\...`)
or a long profile makes autostart silently stop working, and EZLM has no other
login entry (it deliberately dropped scheduled tasks).

Proposed fix: if the value is 260 bytes or more, write the 8.3 short form of
the executable (`GetShortPathNameW`) into the value. Needs a Windows check to
confirm the limit still applies on Windows 11 and to measure the short form.

### F4 (minor, code-read) the console launcher passes long paths through `cmd.exe` and `start`

`internal/integrate/launch_script.go:366-369` builds
`cmd.exe /d /c start "<title>" /D "<dir>" cmd.exe /k .\<base>`, and the
generated `.cmd` starts with `cd /d "<dir>"` (`launch_script.go:161`). cmd.exe
is not long-path aware in its own path handling (it is not `longPathAware` and
does not use `\\?\`), and `start` has its own 260-character buffer. A launcher
under a deep `%USERPROFILE%` therefore fails to open even though the file
exists. UNC is fine for `cd /d` (cmd supports a UNC current directory); the
`\\?\` prefix must *not* be passed to cmd.exe, which rejects it in many
commands. Proposed fix: if the launcher directory is 248 bytes or more, use the
8.3 short form in the `start` line and in the `.cmd`'s `cd /d`.

### F5 (minor) other `exec.Command` sites have the same F1 root cause

Same missing prefix, not changed (outside owned files or low risk):
`internal/serve/run.go:23` (`RunForeground`), `internal/prereq/prereq.go:53,188`
(installer run from a temp download), `internal/update/update.go:112-118`
(`%TEMP%\EZLM-update` installer), `internal/integrate/node_install.go:93` (tar),
`internal/integrate/dsh.go:372,472` (node/npm). Any of these breaks when
`%TEMP%`, `%LOCALAPPDATA%` or the EZLM home is deep. The same
`paths.Extended` call fixes each.

### F6 (minor) no code builds a `\\?\` path, and `filepath.Join` would break one

Before this change, nothing in `internal/` or `cmd/` produced an extended path
(`grep -rn '\\\\?\\'` found none). If a prefixed path is ever stored and then
passed through `filepath.Join`, Go treats `\\?\C:` as the volume and keeps the
prefix, but any `.`/`..` in it is no longer resolved by Win32; keep prefixed
paths at the process boundary only, as `startLogged` now does.

## UNC notes

- `paths.Home()` / `filepath.Join` handle `\\server\share` correctly; the
  `%EZLM_HOME%` override can be a share.
- `internal/scan/apps.go:192-193` normalizes `\` to `/` before the
  prefix comparison, so `\\server\share\.ezlm` matches correctly.
- `CreateProcess` accepts a UNC `lpCurrentDirectory`, so `cmd.Dir` on a share is
  not itself a bug; cmd.exe children that then use relative paths are (F4).

## Could not verify without Windows

- That `CreateProcess` accepts `\\?\` in `lpApplicationName` and in
  `lpCurrentDirectory` (expected from the Win32 docs; the fallback is to extend
  only `cmd.Path`).
- The Run key 260-character limit on current Windows 11 (F3).
- llama.cpp's handling of `\\?\` model paths (F2).
- cmd.exe/`start` long-path behaviour (F4).

## Open questions

- Should `paths.Extended` also be applied inside `internal/proc` for every child
  EZLM starts, so the fix is one place instead of per call site? `internal/proc`
  is another agent's slice (a13), so it was left alone.
- Should EZLM refuse a model folder whose path plus a GGUF name would exceed 260
  on a machine without long-path support, and say so at pick time? That is a UX
  decision for the Windows app (a10/a11).
