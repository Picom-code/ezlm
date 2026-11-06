# T15 / a15: severity table

## What I did

I read every `*_windows.go` in the snapshot (`internal/awake`, `internal/proc`,
`internal/serve`, `internal/prereq`, `internal/scan`, `internal/app`,
`internal/integrate`), the Windows-only branches in `internal/serve/config.go`
and `internal/app/settings.go`, `windows/installer/EZLM.iss`, and the Windows
app's C# (`Program.cs`, `Engine.cs`, `Install.cs`, `Shell.cs`, `Taskbar.cs`,
`SettingsPane.cs`, `ChatStore.cs`, `Memory.cs`, `App.xaml.cs`, `Panes.cs`,
`Chat.cs`, `SelfTest.cs`).

Then I ranked what I found in `findings.md`: forty findings, each with
`file:line`, a severity, the evidence and a proposed fix, organised by the
slice that owns it (a01 to a16).

No code changes: this slice owns the table, not the fixes. Every finding
belongs to another agent's file, and the brief says not to edit another
agent's test file or worktree.

## Findings by severity

**One blocker, from a08:** nothing EZLM ships is Authenticode-signed, so Smart
App Control blocks the app and its installer on a new Windows 11 install. Two
further candidates need one clean-VM run before they earn the label: A15-04
(SmartScreen on the unsigned, Mark-of-the-Web self-update) and A15-01 (the API
key on llama-server's command line).

**Major (21):**

- A15-01 `internal/serve/config.go:121` the API key is in the server's argv.
- A15-02 `internal/proc/proc_windows.go:24-37` job-object assignment races the
  first child, and its error is ignored, so a failed check can orphan a
  llama-server holding GPU memory.
- A15-03 `internal/proc/proc_windows.go:39-52` `kill()` never closes the job
  handle.
- A15-04 `windows/EZLM/SettingsPane.cs:302` + `EZLM.iss` unsigned self-update
  meets SmartScreen and Smart App Control.
- A15-05 `internal/serve/service_windows.go:49-53` llama-server is downloaded
  into a hidden profile folder and executed, the shape Defender blocks.
- A15-06 `internal/serve/config.go:91-94` LAN mode raises the firewall prompt;
  Cancel silently breaks the LAN URL.
- A15-07 `internal/engine/engine.go:456,462,492` model paths go to llama.cpp
  without `\\?\`, so a long profile plus a long GGUF name fails to load.
- A15-08 `internal/scan/display_windows.go:50` GPU detection depends on
  PowerShell; a locked-down machine plans a CPU-only install.
- A15-09 `internal/integrate/terminal_windows.go:69` cmd.exe command lines
  are built without cmd escaping (`&`, `%`, `^`).
- A15-10 `internal/serve/service_windows.go:93-96` the `.cmd` fallback does
  not escape `%` for cmd.exe.
- A15-11 `internal/paths/paths.go:14-18` a failed `UserHomeDir` writes the
  key and models into the working directory.
- A15-12 `internal/serve/config.go:85-90` `0o600` is not an ACL on Windows;
  the key file is protected only by inheritance.
- A15-13 `EZLM.iss:28-33` a manual install-over leaves the server stopped.
- A15-14 `windows/EZLM/Engine.cs:79` the token header is mutated on a shared
  `HttpClient` while a stream may be sending.
- A15-15 `windows/EZLM/Engine.cs:98` killing a hung engine leaves its
  llama-server running.
- A15-16 `internal/integrate/terminal_windows.go:63` child output is decoded
  as UTF-8 on an OEM-code-page machine.
- A15-17 `internal/scan/apps_windows.go:32-38` user identity is compared by
  name, which misses Microsoft-account profiles.
- A15-18 `internal/scan/display_windows.go:56` PowerShell output is split on
  whitespace with no shape check.
- A15-19 `internal/scan/display_windows.go:84-90` a binary VRAM value is
  misread, so the plan offloads too few layers.
- A15-20 `internal/scan/display_windows.go:186` the PDH item count is not
  clamped to the buffer.
- A15-21 `EZLM.iss:42-44` only `*.xbf` is cleaned up; an old `Engine\ezlm.exe`
  survives and keeps a stale engine alive.
- A15-22 `windows/EZLM/Program.cs:30-39` the logon helper stays in Task
  Manager for up to ten minutes.

**Minor (18):** A15-23 to A15-40 in `findings.md`: the 386 layout and
256-process cap in `proc.pids`, `ptyCommandLine`'s empty-`Args` panic, the
PTY handle leak and unchecked `ResumeThread`, `%VAR%` expansion in PATH,
the `C:\Windows.old` false positive, the uninstaller's unbounded `Exec`,
the icon path, `USERNAME` captured at init, `Start` accepting a stale
server, the Latin-1 fallback, the blanket `UnhandledException` handler,
the stale `deployError`, `engine.log` growth, non-atomic prefs, the taskbar
COM cache, the model folder ACL, the `IsWow64Process2` fallback, and
`ArchitecturesInstallIn64BitMode`.

## What I changed and why

Nothing in the code. `findings.md` is the deliverable.

## Sibling findings, merged

`findings.md` now carries a second table with every finding a01 to a16
reported, in their words, with status. Ten slices landed before my second
commit (a03, a05, a07, a08, a10, a11, a12, a13, a14, a16); a01, a02, a04,
a06 and a09 had not, so their areas are covered by my own rows only.

Already fixed on a sibling branch, do not re-patch: a13 (job assignment race,
`pids()` layout and 256-process cap), a05 and a14 (VRAM value as binary, WMI
split, SID ownership), a03 (PowerShell output decoded as UTF-8), a12
(`paths.Home()` relative fallback), a08 (updater no longer runs from
`%TEMP%`).

## What I could not finish

- **Five slices had not landed when I stopped** (a01, a02, a04, a06, a09).
  Their areas are represented by my own rows, but their reports may carry
  findings and fixes I have not seen. Merge them the same way.
- **Nothing is proven on Windows.** There is no Windows machine here. Every
  row is code-read; none has a failing test behind it. That is the main gap
  between this table and the quality bar.
- I did not try to decide between two plausible readings of A15-19 (whether a
  given AMD driver writes `HardwareInformation.MemorySize` as `REG_BINARY` or
  `REG_DWORD`); the fix handles both, so it does not matter for the patch.

## Next steps

1. Merge the sibling reports when they land; re-rank if any comes with a
   failing Windows test (that outranks everything here).
2. Run a16's clean-VM checklist on x64 and ARM64, and settle A15-04 and
   A15-02 first: they are the two that could be blockers.
3. Land the cheap, low-risk fixes first: A15-20 (clamp), A15-24 (guard),
   A15-25 (close the handle), A15-26 and A15-27 (path edges), A15-30 and
   A15-31 (service names).
4. Signing (A15-04, A15-05) is a release-engineering task, not a code review
   one; it needs a certificate before the beta.
