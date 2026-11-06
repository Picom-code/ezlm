# T15 a04: PowerShell and cmd invocation

Slice: quoting, argument injection, `-EncodedCommand` use, `exec.Command` with `cmd /c`,
and paths with `&` or `%` in them. No Windows machine here: everything below is code reading
plus tests that compile under `GOOS=windows`.

## What I read

- Every Go site that starts a shell: `internal/integrate/launch_script.go` (the `.cmd`
  launcher), `internal/integrate/pty_windows.go` (batch through `cmd /d /s /c`),
  `internal/integrate/terminal_windows.go` (the hidden `cmd.exe` that runs `start`),
  `internal/integrate/installer.go` (`ShellCommand`), `internal/scan/display_windows.go`,
  `internal/scan/gpu_discrete.go`.
- The Windows C# app for shell use: `windows/EZLM/Engine.cs`, `Program.cs`,
  `SettingsPane.cs`. It never starts a shell; `ProcessStartInfo` gets a `FileName` and
  an `Arguments` string, and `UseShellExecute=false` means CreateProcess parses them.
- `windows/installer/EZLM.iss`: no `cmd`/PowerShell invocation.
- The launcher goldens (`internal/integrate/testdata/launch/win-*.cmd`,
  `openers.txt`) and `launch_golden_test.go`.

## Findings

### 1. minor: `launchStartLine` is the one cmd line that does not escape `%`
`internal/integrate/launch_script.go:366-370`

```go
return fmt.Sprintf(`cmd.exe /d /c start "%s" /D "%s" cmd.exe /k .\%s`, t, dir, base)
```

Every other cmd line in the package runs user text through `launchCmdPct`
(`launch_script.go:166-175`, `:197`, `:230-236`); this one interpolates the launch
folder and title raw. cmd.exe expands `%NAME%` in the line it gets after `/c`, so a
profile or `EZLM_HOME` under `C:\Users\100% Pure\50% off` turns `% Pure\50%` into an
empty variable and `start` gets a bad `/D`. A single `%` is harmless; a pair, or a
folder literally named `%USERNAME%`, is not. This is the fallback used on any Windows
without Windows Terminal (the default on Windows 10), so it is the common path, but the
trigger is a rare folder name.

Proposed fix: escape `%` for a cmd *command line*, not a batch file. Doubling is the
batch rule (`%%` -> `%` in a file); on a `/c` line `%%` stays `%%`, so verify on Windows
before choosing. The robust fix is to take the path off the line: have `termStartCmd`
set `cmd.Dir = dir` and drop `/D "%s"` (both in the frozen `terminal_windows.go`), or
write a one-line `.cmd` next to the launcher and `start` that.

### 2. minor: `ptyCommandLine` puts a batch path into a cmd line unescaped
`internal/integrate/pty_windows.go:140-153`

```go
words := []string{`"` + cmd.Path + `"`}
...
return comspec, `cmd.exe /d /s /c "` + strings.Join(words, " ") + `"`
```

`cmd.Path` goes in raw, so a `.cmd`/`.bat` program under a `%`-pair folder name expands
the same way. The arguments are passed with `launchCmdArg(a, 0)`, which is correct here:
this is a command line, and `%` doubling would arrive as `%%` (it is the batch rule that
collapses `%%`). Only a `%NAME%`-shaped argument is at risk, and `%`-shaped model ids
and flags are not realistic today. The quoting itself is right: `/s` makes cmd strip the
first and last quote, the line opens with the extra `"` that `/s` needs, and
`launchCmdArg` already toggles quotes for a `\"` inside an argument.

Proposed fix (if wanted): `launchCmdPct(cmd.Path, 0)` is a no-op, so this needs the same
"keep the path off the cmd line" treatment, or accept it and document it.

### 3. minor: `ShellCommand` passes whole scripts through `-Command`, not `-EncodedCommand`
`internal/integrate/installer.go:24-31`

```go
cmd = exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
```

Go re-quotes an argument that contains spaces and writes an inner `"` as `\"`. powershell.exe
5.1 parses its own command line and its handling of `\"` is the documented trap (it is not
the CRT rule the rest of Windows uses). Today's callers are safe by luck:
`hermesInstallWindows` (`internal/integrate/hermes.go:37-39`) uses only single quotes and
no `"`, and the scan probes have balanced `"` pairs. The next script that needs a `"` will
fail with a parse error at the user, not in a test.

Proposed fix: encode the script (`-EncodedCommand` with base64 of UTF-16LE, which also
survives any console code page, so it overlaps a03) or write it to a temp `.ps1` and use
`-File` (also better for Defender, a08). `-ExecutionPolicy Bypass` is only needed for
`-File`, not `-Command`.

### 4. minor: the scan probes have the same `-Command` shape and no test
`internal/scan/display_windows.go:50-51`, `internal/scan/gpu_discrete.go:112-121`

Fixed ASCII scripts, no user text, so no injection. They contain `"` and `$`; they work
because the quotes are balanced, which is exactly the property a later edit can break
silently. They also lack `-NonInteractive`, which costs nothing and keeps a probe from
waiting on a prompt. The output decoding (OEM code page) is a03's.

Proposed fix: `-NonInteractive` and, when the script next changes, `-EncodedCommand`.

### 5. info: the owned Go invocation is argv-only and clean
`internal/serve/service_windows.go`

- `schtasks` calls (`:71-72`, `:123`, `:141`, `:209`) pass the task name as one argv
  element, so `&`, `%`, `(`, `)` and spaces in `USERNAME` are literal. No shell.
- `setRunKey` (`:85-101`) writes `syscall.EscapeArg(path) + " start"` to the Run key.
  That is the CreateProcess quoting rule, and the Run key is parsed by CreateProcess, so
  spaces and `&` in the install path are handled. `%` is not expanded by CreateProcess;
  whether Explorer expands `%` in a Run value before CreateProcess is unverified (open
  question 1).
- `startLogged` (`:177-200`) starts the server with argv and hands over an open log
  handle, so a path with `&`/`%` never reaches a shell. Covered by
  `internal/serve/invoke_windows_test.go`.

### 6. info: the `.cmd` launcher escaping is correct (verified, not changed)
`internal/integrate/launch_script.go:143-228, 238-300`

`&`, `|`, `<`, `>`, `^`, `(`, `)` are careted outside quotes and literal inside them;
`%` is doubled once (twice under `call`, where cmd expands a second time); `"` in an
argument is written `\"` for the CRT and the caret is added where cmd sees an unquoted
special. `title`/`echo` text goes through `launchCmdText`. The goldens cover
`win-hermes-percent.cmd`, `win-quote-arg.cmd`, `C:\Users\Jörg\...` and
`C:\Users\Sam & Co (2)\...` (`launch_golden_test.go:127-150`). Nothing to fix.

## What I changed

- Added `internal/serve/invoke_windows_test.go` (builds only on Windows): starts the
  server through `startLogged` with a log folder and a server path holding
  `A & B 100% (x) ^ José`, and checks the log is written. It pins the property that
  matters for this slice on the owned side: EZLM's server start is argv plus a handle,
  never a command line.
- No fix in an owned `*_windows.go` file: findings 1-4 are all in the frozen
  `internal/integrate`, so a patch there would be dropped at the merge. They are listed
  for whoever rewrites that package tonight.

## Not verified without Windows

- Findings 1 and 2: that cmd.exe expands a `%NAME%` pair in a `/c` line (I am confident)
  and what the right escape is on a command line as opposed to a batch file. A one-line
  check on a Windows box: `cmd /c echo 100%%` and `cmd /c echo %TEMP%`.
- That the two tests in `invoke_windows_test.go` pass (they only compile here).
- Whether Explorer expands environment variables in a Run value.

## Open questions

1. Does `HKCU\...\Run` value expansion include `%VAR%`? If it does, an install path with
   `%` breaks autostart and `setRunKey` needs a check.
2. Should `ShellCommand` move to `-EncodedCommand` in this task's patch even though the
   file is frozen, so the Lead can take it in the morning? It is the one change that
   removes a whole class of failure.

## Next steps

1. Take findings 1-3 into the `internal/integrate` rewrite: keep user paths off cmd lines,
   and encode PowerShell scripts.
2. Add a Windows CI leg that runs the `_windows_test.go` files, since none of them run on
   this Mac.
3. When a real Windows box is available, run the `cmd /c` escape checks above and turn
   finding 1 into a test in `launch_golden_test.go`.
