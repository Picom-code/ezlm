# T15 a03: code pages and console encoding

Agent a03. Area: `chcp`, `cmd` output decoding, UTF-8 vs UTF-16 in
PowerShell, and the Go side that reads it.

No Windows machine here. Everything below is code-read; the one fix has a
Windows-only test that builds under `GOOS=windows` (run it on a Windows box).
A local reproduction of the byte-level failure is in Finding 1.

## Findings, ranked

### 1. major: PowerShell installer output is decoded as UTF-8, but PowerShell 5.1 writes the OEM code page

`internal/integrate/installer.go:27` starts PowerShell with
`-NoProfile -ExecutionPolicy Bypass -Command <script>`. Windows PowerShell
5.1 encodes everything it writes to a redirected stdout with
`[Console]::OutputEncoding`, which defaults to the console's OEM code page
(cp437 on US English, cp850 on Western Europe, cp932 on Japanese, ...), not
UTF-8. The Go side then does `string(bytes)`:

- `internal/integrate/log.go:151` (`Run.Exec`, every probe command),
- `internal/integrate/installer.go:40` (`Run.Install`) -> `lineWriter`
  (`installer.go:129-160`), which sends raw bytes to the log and to the UI
  progress line,
- `internal/integrate/check.go:135` (`ErrLine`) picks a line out of that
  text for the message the user sees.

Evidence (local, same bytes PowerShell produces for "Café Müller" at cp437;
cp850 has the same 0x82/0x81 bytes):

```
raw      = 43 61 66 82 20 4d 81 6c 6c 65 72 0d 0a
as UTF-8 = "Caf\x82 M\x81ller\r\n" (valid: false)
```

So on a German, French, Spanish or Japanese Windows, or whenever an
installer echoes a path with é/ü/王 (the common case: `C:\Users\José`), the
integrate log, the "Installing ..." progress line and the error sentence
("the installer stopped with this message: ...") show mojibake. Installers
are exactly where localized text appears (npm warnings, uv, Hermes).

Fix (owner of `internal/integrate`, frozen for me): prepend
`[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false); ` to the
Windows script in `ShellCommand`, and add the same prefix in `Run.Exec`'s
PowerShell callers (or decode the bytes with `GetConsoleOutputCP` +
`golang.org/x/text/encoding`, which would add a dependency). The prefix
changes nothing on PowerShell 7, which is UTF-8 already.

### 2. major (same class, owned code): `scan.run` returns raw bytes

`internal/scan/scan.go:250-255` returns `string(out)` from `cmd.Output()`.
`internal/scan/display_windows.go:50` runs PowerShell through it, and so do
`internal/scan/gpu_discrete.go:113,116` (`windowsGPUUsage`). The current
fields are ASCII (PNPDeviceID, `pid_..._luid_...` instance names), so this
is latent rather than visible today, but the helper is the only path for any
future PowerShell query and the failure mode above is silent.

Fixed here for the adapter query: added `runPS` in
`internal/scan/exec_windows.go` (sets
`[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)` first) and
used it in `display_windows.go`. `gpu_discrete.go` is a14's file (it is not
a `_windows.go` file), so its two calls are left for them; the change is
`run(...)` -> `runPS(ctx, script)` with the same arguments.

Test: `internal/scan/console_windows_test.go` (Windows-only). It fails
before the fix (PowerShell returns `Caf\x82` for `Café`) and passes after.

### 3. minor: the launcher's `chcp` failure is not checked

`internal/integrate/launch_script.go:147` writes `chcp 65001 >nul` as the
second line, then all later lines (which carry UTF-8 paths) in a batch file
without a BOM. That ordering is correct: the first two lines are ASCII and
cmd.exe re-reads the file for every line in the new code page. But if
`chcp.com` is missing or blocked (policy, a stripped image), the errorlevel
is ignored and cmd.exe keeps reading the UTF-8 bytes as the OEM page: a
workspace under `C:\Users\José` starts the wrong folder or fails to `cd`.
The .cmd is still parsed and the app runs, just from the wrong place.

Proposed fix: `chcp 65001 >nul 2>&1 || (echo EZLM couldn't switch this
window to UTF-8. & pause & exit /b 1)`. Low priority: Windows 10/11 always
has chcp.com.

### 4. minor: C# attachment decoding falls back to Latin-1, not Windows-1252

`windows/EZLM/Chat.cs:1071`: `Encoding.Latin1` is ISO-8859-1. Bytes
0x80-0x9F, which Windows-1252 uses for €, “ ”, –, … and which an Excel CSV
on Windows contains, become C1 control characters. The comment says "close
to Windows-1252"; it is close, but those are the characters people paste.
Proposed fix: `CodePagesEncodingProvider.Instance.GetEncoding(1252)` (after
`Encoding.RegisterProvider(CodePagesEncodingProvider.Instance)`), or map
0x80-0x9F explicitly.

### 5. minor: UTF-16 without a BOM is detected by zero high bytes only

`windows/EZLM/Chat.cs:1066-1069`: a file is taken as UTF-16 when at least
90% of the high bytes in the first 8000 bytes are zero. That is true for
Latin text in UTF-16LE, but a UTF-16LE file of CJK text (王 = 0x8B 0x73)
has no zero high bytes at all, so `zeros == 0` and the file falls through
to the strict UTF-8 decode, fails, and becomes Latin-1 mojibake. Not a
problem for PowerShell redirection (`>` writes UTF-16LE *with* a BOM, which
is handled at line 1064) but it is for hand-saved UTF-16 files.
Proposed fix: when the bytes are not valid UTF-8 and the length is even,
try `Encoding.Unicode` and accept it if the decoded text has no C0/C1
control characters other than tab/newline.

### 6. minor: the C# engine reader leaves the child's encoding to the console

`windows/EZLM/Engine.cs:56-67` redirects stdout/stderr but never sets
`StandardOutputEncoding`/`StandardErrorEncoding`; .NET then decodes with
`Console.OutputEncoding`, the OEM code page, while the Go engine writes
UTF-8. The hello line is ASCII today (URL and token), so nothing breaks
yet; it breaks the day the line carries a non-ASCII path. Fix: set both to
`Encoding.UTF8`.

## Checked and correct (no change)

- `internal/integrate/launch_script.go:133-147`: the .cmd is UTF-8 without
  a BOM and `chcp 65001` is before any non-ASCII line. Golden tests in
  `internal/integrate/testdata/launch` pin the bytes.
- `internal/integrate/pty_windows.go:160-196`: the environment block is
  UTF-16LE via `utf16.Encode([]rune(...))`, which does emit surrogate
  pairs, with the double NUL terminator and case-insensitive last-wins
  de-duplication. ConPTY output is UTF-8 and is read as bytes.
- `windows/EZLM/Chat.cs:1062-1064`: UTF-8 BOM, UTF-16LE and UTF-16BE BOM
  are all handled before the heuristic.

## What I could not verify

- No Windows machine: `console_windows_test.go` builds (checked with
  `GOOS=windows go vet`) but was not run. The PowerShell 5.1 OEM default is
  documented/known behavior and is asserted by that test on a real box.
- Whether any current PowerShell query in `internal/scan` can actually
  return non-ASCII (PNPDeviceID and PDH instance names are ASCII by spec).
  The fix is for the helper, not for a demonstrated live string.

## Open questions

- Does anything else feed `Run.Install` output to a file that a Windows app
  reads back as UTF-8? The log is only read by EZLM (Go), so the prefix fix
  is enough; no re-encoding of old logs.
- Should the C# app instead call `chcp 65001` for its own console? The
  engine is GUI-subsystem, so it has no console to switch.

## Files changed

- `internal/scan/exec_windows.go`: `runPS`.
- `internal/scan/display_windows.go`: adapter probe uses `runPS`.
- `internal/scan/console_windows_test.go`: new, Windows-only tests.
