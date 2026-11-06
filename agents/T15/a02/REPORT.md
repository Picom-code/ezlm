# T15 a02: Unicode and OneDrive-redirected profile folders, spaces in `%USERPROFILE%`

Slice: Windows code review, code reading. Owned: `*_windows.go` files outside the frozen
areas. No Windows machine was available; everything below is either a test that compiles
under `GOOS=windows` or a code-read finding, marked as such.

## What I did

- Read every `*_windows.go` outside the frozen areas (`internal/scan`, `internal/serve`,
  `internal/proc`, `internal/awake`, `internal/prereq`), plus the Windows-tagged files
  under `internal/integrate` (frozen: findings only), `windows/EZLM/Engine.cs`,
  `windows/EZLM/SelfTest.cs` and `windows/installer/EZLM.iss`.
- Found and fixed one real non-ASCII account-name bug in an owned file
  (`internal/scan/apps_windows.go`).
- Added `internal/scan/accounts_windows_test.go` (Windows-only, one test file).
- Verified: `gofmt -l .` empty; `go build ./...`, `go vet ./...`,
  `GOOS=windows GOARCH=amd64 go build ./...`, `GOOS=windows GOARCH=arm64 go build ./...`,
  `GOOS=linux go build ./...` pass; `GOOS=windows GOARCH=amd64 go test -c ./internal/scan`
  compiles; `go test -p 2 ./internal/scan` passes on macOS.

## Findings

### 1. major (fixed): a user name whose lowercase is longer than the name is never "the current user"

`internal/scan/apps_windows.go:53` (before the fix)

```go
me := ""
if u, err := user.Current(); err == nil {
    me = strings.ToLower(u.Username)          // one side lowercased ...
}
...
if u, err := p.UsernameWithContext(ctx); err == nil && me != "" {
    pr.mine = strings.EqualFold(u, me)        // ... compared with the other side as written
}
```

`ownedByMe` (`:32` before the fix) compared the two names as written, but `listProcs`
lowercased only the current user's name. For U+0130 "İ" the lower case is "i̇"
(i + U+0307), which does not fold equal to "İ":

```
"İbrahim"      lower="i̇brahim"      EqualFold(lower,raw)=false EqualFold(raw,raw)=true
"José Müller"  lower="josé müller"  EqualFold(lower,raw)=true
"王小明"        lower="王小明"        EqualFold(lower,raw)=true
```

(measured with `go run` on this Mac, Go 1.27.3).

Effect: for a Windows account such as `DESKTOP-1\İbrahim`, `listProcs` marked no process
as `mine`, so `MemoryApps` returned nothing (the "close apps to free memory" list is
silently empty) and `Kill` (`internal/scan/scan.go:236`) refused his processes with
`os.ErrPermission`. Names with ordinary non-ASCII letters (José, 王, Ł) were unaffected.

Fix (`internal/scan/apps_windows.go:31-37, 47, 53, 67`): one helper, both names kept as
Windows wrote them (Windows compares account names case-insensitively anyway), and an
empty name matches nothing:

```go
func sameUser(a, b string) bool { return a != "" && b != "" && strings.EqualFold(a, b) }
```

Used by `ownedByMe` and `listProcs`. Test: `internal/scan/accounts_windows_test.go`
(non-ASCII names, upper-cased names, empty names, other accounts). It compiles under
`GOOS=windows`; it cannot be executed here.

### 2. minor (code-read): registry PATH entries are expanded twice, so a folder with `$` or a literal `%` loses its tail

`internal/integrate/findtool_windows.go:53,63` (frozen area)

`splitWinPath` turns `%NAME%` into `${NAME}` and hands the whole entry to `os.ExpandEnv`.
`os.ExpandEnv` also expands `$NAME`/`${NAME}`, which Windows never expands in a PATH
value. A per-user PATH entry such as `C:\Users\a\AppData\Roaming\npm` is fine, but
`C:\tools\$cache\bin` becomes `C:\tools\\bin`, and an entry with two literal `%`
(e.g. `D:\100% done\100%`) is treated as a variable and dropped. The tool search then
misses an installed tool and EZLM says it isn't installed.

Proposed fix: expand only `%NAME%` pairs, in place, without `os.ExpandEnv`
(leave `$` and unpaired `%` alone). Keep `winVars` only as the helper for that loop.

### 3. minor (code-read): EZLM's data sits in `%USERPROFILE%` while the app's own data sits in `%LOCALAPPDATA%`

`internal/paths/paths.go:18` vs `windows/EZLM/Engine.cs:29`

The engine keeps `.ezlm` (models, logs, config with the API key, `launch/*.cmd`) under
`%USERPROFILE%`; the C# app keeps `engine.log`/`app.log` under `%LOCALAPPDATA%\EZLM`.
`%LOCALAPPDATA%` is never redirected or synced, `%USERPROFILE%` can be a roaming profile
or a folder-redirected/OneDrive-backed tree on managed machines. In that case multi-GB
model files and the key are written to a network or synced folder, and the app's logs
and the engine's logs are in two different trees (support asks for one).

OneDrive Known Folder Move only redirects Desktop/Documents/Pictures, so this is not the
common OneDrive case, but the two roots are still inconsistent and `%LOCALAPPDATA%\EZLM`
is the Windows convention for per-user app data. Proposed fix: on Windows default
`paths.Home()` to `%LOCALAPPDATA%\EZLM` (keep `EZLM_HOME` as the override and migrate an
existing `%USERPROFILE%\.ezlm`), and point the C# `Engine.DataDir` at the same folder.
`paths.go` is not a `*_windows.go` file, so this is a report finding, not a patch here.

### 4. minor (code-read): the `start` title is expanded and mangled by cmd.exe

`internal/integrate/launch_script.go:366-371` (frozen area)

The fallback console opens with `cmd.exe /d /c start "<title>" /D "<dir>" ...`. The
title is the workspace folder name (user data), quoted but not escaped for cmd: a title
containing a `%NAME%` pair is expanded (a folder named `%TEMP%` becomes
`C:\Users\a\AppData\Local\Temp`), and `"` is replaced with `'`. Cosmetic, and only
the window title. The batch launcher's own `title` line (`:148`) is unquoted and uses
`launchCmdText`, which is correct there.

### 5. minor (code-read): `%` in the launch folder is expanded by cmd.exe

`internal/integrate/launch_script.go:369` (frozen area)

`/D "<dir>"` quotes the folder, but cmd.exe still expands `%NAME%` inside double quotes.
A workspace or profile folder whose name contains a pair of `%` (for example a literal
`%USERPROFILE%` folder name, or `D:\100% done\100%`) is rewritten before `start` sees
it. The same line appends `.\<base>` unquoted; that is safe only because launcher ids
are restricted to `[a-z0-9-]` (`launchID`). Proposed fix: double `%` in `dir` (as
`launchCmdPct(dir, 1)`) and quote `.\<base>`.

### 6. minor (code-read): Startup-folder entry written relative to the working directory when `%APPDATA%` is unset

`internal/integrate/hermes.go:309` (frozen area)

`filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Startup")`
with an empty `APPDATA` yields a relative path, so the shortcut lands in EZLM's working
directory. Rare (a normal desktop session always has `APPDATA`), but a `%APPDATA%`
fallback of `%USERPROFILE%\AppData\Roaming` would make it explicit.

## What is fine (checked, no finding)

- `internal/serve/service_windows.go`: `startLogged` hands llama-server an open handle to
  the log instead of `--log-file` (the comment at `:173` names the ANSI-code-page
  problem), and the existing `startlog_windows_test.go` covers a `José Müller 王` folder.
  `setRunKey` quotes the executable with `syscall.EscapeArg`, which keeps spaces and
  non-ASCII intact in the `HKCU\...\Run` value; task names from `%USERNAME%` are passed
  as separate `schtasks` arguments, which Go quotes correctly.
- `windows/EZLM/Engine.cs:29` uses `SpecialFolder.LocalApplicationData` (OneDrive-safe),
  and `windows/installer/EZLM.iss` uses `{localappdata}`, `{autodesktop}` and
  `{autoprograms}`, which Inno resolves through the shell, so redirected Desktop and
  Start Menu folders are handled. C# is UTF-16 throughout; no `%USERPROFILE%` string
  concatenation and no `Path.Combine` misuse found.
- `internal/integrate/launch_script.go` writes the Windows launcher as UTF-8, starts it
  with `chcp 65001` before any path appears, and doubles `%`; `launchWTArgs` passes the
  folder as a separate argv element, so spaces and non-ASCII survive Windows Terminal.
- `internal/serve/service_windows.go:44`, `internal/proc/proc_windows.go`,
  `internal/prereq/hide_windows.go`, `internal/scan/exec_windows.go` only set
  `HideWindow`/`CREATE_NO_WINDOW`; no path handling.
- `internal/scan/apps_windows.go` `fileDescription` decodes the version resource as
  UTF-16 and queries every translation, so a non-ASCII product name is read correctly.

## Could not verify without Windows

- Whether `schtasks.exe` on a localized or non-ASCII Windows matches the legacy task
  names `"EZLM Server (<USERNAME>)"`; it is Unicode and takes wide argv, so it should,
  but this is untested.
- Whether llama.cpp opens the model file with a wide (`_wfopen`) call. `startLogged`'s
  comment covers `--log-file`; `--model` is passed as an argument and is fine at the
  CreateProcess layer, but a model under `C:\Users\José\...` depends on llama.cpp.
  Worth one manual check on the beta checklist.
- The Windows-only test file could only be compiled (`go test -c`), not run.

## Open questions

- Should `paths.Home()` move to `%LOCALAPPDATA%\EZLM` on Windows (finding 3), with a
  one-time migration of `%USERPROFILE%\.ezlm`? It changes where existing users' models
  are found, so it needs an owner.
- Is a roaming-profile install (network `%USERPROFILE%`) in scope for the beta? If yes,
  the multi-GB model folder should be chosen from the drive list by free space, which
  `internal/app/settings.go` already does.

## Next steps

- a12 (Go path handling) should own finding 3, since it is in `internal/paths`.
- a04 (PowerShell/cmd invocation) should own findings 2, 4, 5 and 6; all are in the
  frozen `internal/integrate` area.
- a09/a10: no OneDrive-redirected Desktop/Start Menu bug found in the installer or C#.
