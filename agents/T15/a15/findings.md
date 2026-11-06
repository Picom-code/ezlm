# T15 severity table (a15)

Every Windows finding the T15 review turned up, ranked by severity, each with
`file:line`, the evidence, and a proposed fix.

**How this was built.** a15 read every `*_windows.go`, `windows/EZLM/*.cs` and
`windows/installer/EZLM.iss` in the snapshot and wrote the rows below itself:
the table is a15's own pass over all sixteen slices, organised by the slice
that owns each finding. At the first commit (2026-11-06 12:00 local) no
sibling had committed; by 12:15, a03, a05, a07, a08, a10, a11, a12, a13, a14
and a16 had landed reports. Their findings are merged in the second table
below, with the rows that are already fixed on a sibling branch marked, so the
morning merge does not re-fix them.

**Evidence classes.** *code-read* means the claim comes from reading the code
with no Windows machine to run it on (there is none here). *built* means it
also compiles and has a test that builds under `GOOS=windows`. No finding
below is *proven by a failing test on Windows*; that is the honest state of
T15 and the reason a16's clean-VM checklist matters.

**Severity.** *blocker*: breaks a normal install, loses data, or leaks a secret
on an ordinary Windows 11 box. *major*: breaks a common configuration or a
feature, or weakens security in a way a user can hit. *minor*: edge case,
cosmetic, or only reachable from EZLM's own code.

There is **one blocker**, and it is a08's: nothing EZLM ships is
Authenticode-signed, so Smart App Control blocks the app on a new Windows 11
install. It is not in the table below because it is a release-engineering
task, not a code line; it heads the merged sibling table. My own pass found
no second blocker: the closest candidates are A15-04 (SmartScreen on the
update) and A15-01 (the key on the command line), both ranked major.

## Ranked table

| # | Slice | Sev | File:line | Finding | Proposed fix |
|---|---|---|---|---|---|
| A15-01 | a06 | major | `internal/serve/config.go:121`, `:163` | The API key is passed to llama-server as `--api-key <key>` on its command line, so it is visible in Task Manager's Command line column, in `Get-CimInstance Win32_Process`, in EDR/telemetry logs, and in any crash dump of the server. | Pass the key through a file (`--api-key-file`, which llama.cpp supports) or an environment variable, and keep it out of `Args()`. |
| A15-02 | a13 | major | `internal/proc/proc_windows.go:24-37` | `track` starts the process first and only then assigns it to the job object, so a fast child (cmd.exe spawning node) can create a grandchild before assignment and that grandchild escapes `kill()`. `AssignProcessToJobObject`'s error is also ignored: when EZLM itself runs inside a non-breakaway job (Task Scheduler, CI, some terminals) the assignment fails and tree kill silently degrades to killing one process. A failed check can then leave a llama-server holding GPU memory. | Start with `CREATE_SUSPENDED`, assign to the job, then `ResumeThread`; return the assignment error so callers can fall back to walking the child list. |
| A15-03 | a13 | major | `internal/proc/proc_windows.go:39-52` | `kill()` uses `TerminateJobObject`, which kills every process in the job but leaves the job handle open; `close()` only closes. If a caller calls `kill()` and never `close()` (or `release()`), the handle leaks for the life of the app, and with `KILL_ON_JOB_CLOSE` the job keeps its processes alive until it does close. | Have `kill()` call `close()` itself, or document and enforce the pair. |
| A15-04 | a08 | major | `windows/EZLM/SettingsPane.cs:302`, `windows/installer/EZLM.iss:24-33` | Self-update downloads `EZLM-Setup-*.exe` to `%TEMP%` and starts it with `UseShellExecute = true`. Nothing in the `.iss` signs the installer (`SignTool` is absent), so the download carries Mark-of-the-Web: SmartScreen prompts on every update, and on a machine with Smart App Control on (new consumer Windows 11 installs) the unsigned, MOTW-marked exe is blocked outright with no "run anyway". The user sees a generic "Couldn't start the installer". | Sign the installer and `EZLM.exe`/`ezlm.exe` with an Authenticode certificate; until then, catch the SAC/SmartScreen case and say so, and offer a manual download link. |
| A15-05 | a08 | major | `internal/serve/service_windows.go:49-53` | The engine runs `llama-server.exe` from `%USERPROFILE%\.ezlm\bin`, a hidden dot-folder in the profile, downloaded from the network. That is the shape Defender's behaviour models flag, and the code comments show earlier designs were already blocked. There is no exclusion, no signature, and the failure surfaces only as "the model didn't start". | Sign the binaries, or ship llama-server inside the signed installer rather than downloading it into the profile; at minimum detect a blocked/quarantined binary and tell the user which file to allow. |
| A15-06 | a08 | major | `internal/serve/config.go:91-94` (`Host()`), `internal/serve/config.go:121` | With LAN mode on, llama-server binds `0.0.0.0`, so the first LAN start raises the Windows Defender Firewall prompt for an unsigned exe in the profile. If the user clicks Cancel (the common reflex), LAN silently does not work and EZLM keeps handing out a LAN URL that nothing can reach. Updates replace the binary, so the rule's program path can go stale and prompt again. | Add the firewall rule for the server's path during install, or detect that the port is unreachable from the LAN address and show one sentence with the next action. |
| A15-07 | a01 | major | `internal/engine/engine.go:456`, `:462`, `:492`, `internal/paths/paths.go:29-34` | Model paths are built as `%USERPROFILE%\.ezlm\models\<model id>\<gguf name>` and passed to llama-server as `-m <path>` with no `\\?\` prefix. Go's own file calls cope with >260 characters, but llama.cpp opens the model with a narrow `fopen`, so a long OneDrive-nested profile plus a long GGUF name fails to load with no useful message. | Prefix absolute paths handed to llama-server with `\\?\` (or shorten the on-disk name to a fixed short id); assert the built path length in a Windows test. |
| A15-08 | a14 | major | `internal/scan/display_windows.go:50-51` | GPU detection for AMD and Intel shells out to `powershell -NoProfile -Command Get-CimInstance ...` on the scan path. If PowerShell is removed or restricted by policy (AppLocker, Constrained Language, "remove PowerShell 2.0" images), the scan finds no GPU and EZLM plans a CPU-only install: a smaller model and a slower answer, with no explanation. | Read `Win32_VideoController` through WMI directly (`github.com/StackExchange/wmi` is not a dependency, so use a `wbem` COM call or the registry `Enum\PCI` walk alone), and treat a PowerShell failure as "unknown", not "no GPU". |
| A15-09 | a04 | major | `internal/integrate/terminal_windows.go:69`, `internal/integrate/launch.go:89-120` | `termStartCmd` hands cmd.exe a raw `CmdLine` built by `launchStartLine`. Any `&`, `^`, `|`, `%` or `"` in the launcher path, the model name or the user's profile path is parsed by cmd.exe as syntax, so a title like `Qwen & Llama` breaks the launch or runs a fragment. | Quote every argument for cmd.exe (double the quotes, escape `^&|<>%`), and prefer `start "" "<exe>" <args>` with a quoted exe; test a path containing `&` and `%`. |
| A15-10 | a04 | major | `internal/serve/service_windows.go:93-96` | The HKCU Run value is built with `syscall.EscapeArg`, which quotes for `CommandLineToArgvW` but does not escape cmd.exe metacharacters. The value is executed directly (not through cmd.exe), so this is correct today, but the same helper is used for the `.cmd` fallback path, where a `%` in the path would be expanded by cmd.exe at logon. | Keep `EscapeArg` for the Run value, and make the `.cmd` writer escape `%` as `%%`; add a test with `%USERNAME%` in the install path. |
| A15-11 | a12 | major | `internal/paths/paths.go:14-18` | If `os.UserHomeDir()` fails, `Home()` returns `"."`, so the config (with the API key), models and logs land in whatever the process's working directory is, which for a shortcut can be `C:\Windows\System32`. Nothing checks the fallback. | Return an error or fall back to `%LOCALAPPDATA%\EZLM`, and log the substitution. |
| A15-12 | a06 | major | `internal/serve/config.go:85-90` | `config.json` (the API key) is written with mode `0o600`. Go on Windows maps that to the read-only attribute only; the file's ACL is inherited from `%USERPROFILE%`, so the protection is incidental, and a profile redirected to a share with a permissive ACL exposes the key. | Set an explicit DACL on `config.json` (owner, SYSTEM, Administrators only) with `SetNamedSecurityInfo`, or store the key with DPAPI (`CryptProtectData`) per user. |
| A15-13 | a09 | major | `windows/installer/EZLM.iss:28-33` | `CloseApplications=yes` with `RestartApplications=no`: a manual "run setup over the running app" makes Restart Manager close the engine helper, and nothing restarts it, so after the update the server is down until the next logon. The in-app update path compensates (`SettingsPane.StopEngineHelpers`), the manual path does not. | `RestartApplications=yes`, or have the installer's `[Run]` entry start the engine when a config exists. |
| A15-14 | a10 | major | `windows/EZLM/Engine.cs:79-80` | `StartAsync` mutates `http.DefaultRequestHeaders` (Remove then Add of `X-EZLM-Token`) while `ListenAsync` may have a request in flight on the same `HttpClient`. The headers collection is not documented thread-safe for concurrent mutation during a send. | Set the header per request (`req.Headers.Add`) from a token field, and store the token in the `Engine` instead of on the shared client. |
| A15-15 | a10 | major | `windows/EZLM/Engine.cs:98-105` | `StopProcess` kills the engine process but not llama-server. If the engine is hung (the very case where `Kill()` runs), the server keeps the model and GPU memory after the window closes, and the next launch reports "already running". | Kill the engine's job/child tree, or have the engine expose a `shutdown` call and only fall back to `Kill` after it times out. |
| A15-16 | a03 | major | `internal/integrate/terminal_windows.go:63-69`, `internal/integrate/findtool_windows.go:48-60` | Text that cmd.exe or PowerShell prints is read back without a code-page decision. `run()` captures bytes and callers treat them as UTF-8; on a Chinese/Japanese/Russian Windows with the OEM code page, non-ASCII output (model names, error text, `%PATH%` entries) arrives mojibake, and `splitWinPath` then looks for folders that do not exist. | Decode child output with `golang.org/x/text/encoding` chosen from `GetConsoleOutputCP`/`GetOEMCP`, or force the child to UTF-8 (`chcp 65001` for cmd, `$OutputEncoding` for PowerShell). |
| A15-17 | a02 | major | `internal/scan/apps_windows.go:32-38`, `:57-59` | "Is this process mine" compares gopsutil's `Username` (`DOMAIN\user`) with `user.Current().Username` using `EqualFold`. On a Microsoft-account profile, or when one side reports `AzureAD\...` and the other `COMPUTER\...`, the comparison fails, so EZLM never treats the user's apps as the user's and never closes them gently. | Compare the user's SID (`windows.Token.GetTokenUser`) instead of the name string. |
| A15-18 | a14 | major | `internal/scan/display_windows.go:56` | `strings.Fields(out)` splits the PowerShell output on whitespace. A `Win32_VideoController.PNPDeviceID` has no spaces today, but the parse silently drops any line that does, and a localized PowerShell banner or warning on stdout becomes a bogus device id (the registry open then fails, so it degrades quietly). | Ask for `-NoLogo`, emit one id per line, and skip anything that does not start with `PCI\`/`USB\`. |
| A15-19 | a05 | major | `internal/scan/display_windows.go:84-90` | `HardwareInformation.qwMemorySize` is read with `GetIntegerValue`, which fails on a `REG_BINARY` value; the fallback reads a 4-byte binary as VRAM, so a 12 GB card whose driver stores the size as binary reports 4 GB, and the plan then offloads too few layers. | Read `HardwareInformation.MemorySize` with `GetValue` and switch on the type, and prefer the larger of the two when both parse. |
| A15-20 | a14 | major | `internal/scan/display_windows.go:186` | The PDH item array is walked with `unsafe.Slice(..., count)` using the count PDH reported, with no check that `size >= count*24`. A driver that reports a larger count than the buffer (or a struct-layout change) reads past the end of the Go buffer. | Clamp `count` to `size/unsafe.Sizeof(item{})` before slicing. |
| A15-21 | a09 | major | `windows/installer/EZLM.iss:42-44` | `[InstallDelete]` removes only `*.xbf`. Renamed or dropped files from earlier builds (`Engine\*.dll`, old `Assets\*.png`, an old `ezlm.exe` self-copy) stay in `{app}` forever, and an old `Engine\ezlm.exe` is exactly the file that keeps a stale engine alive after an update. | Add `Type: filesandordirs; Name: "{app}\Engine"` (and `{app}\Assets`) to `[InstallDelete]`. |
| A15-22 | a11 | major | `windows/EZLM/Program.cs:30-39` | The `--autostart` path starts the engine and then blocks for up to ten minutes (`WaitForExit(10 * 60 * 1000)`), with no window. Task Manager shows a second `EZLM.exe` with no UI for ten minutes after every logon, and a user who "ends" it kills the prewarm it was doing. | Start the engine detached and exit immediately; the engine already survives on its own. |
| A15-23 | a13 | minor | `internal/proc/proc_windows.go:70-82` | `JOBOBJECT_BASIC_PROCESS_ID_LIST` is walked as `[]uintptr` and the count is read at offset 4. That is right on 64-bit, where the two DWORDs are followed by padding, but wrong on 386, where the ids begin at offset 4. The list is also silently truncated at 256 processes. | Declare the struct (`count uint32; inList uint32; ids [256]uintptr`) and let Go place it; loop on `moreData` to get the full list. |
| A15-24 | a13 | minor | `internal/integrate/pty_windows.go:141` | `append([]string{cmd.Path}, cmd.Args[1:]...)` panics when `cmd.Args` is empty (a caller that sets only `Path`). | Guard `len(cmd.Args) > 0`. |
| A15-25 | a13 | minor | `internal/integrate/pty_windows.go:242-246` | `Close` closes `p.process` only when the program exits within five seconds; on timeout the process handle leaks, and `ResumeThread`'s failure at `:122` is ignored, in which case `Wait` blocks forever on a suspended process. | Always close the handle (defer outside the select); check `ResumeThread` and kill on failure. |
| A15-26 | a12 | minor | `internal/integrate/findtool_windows.go:48-60` | An unset variable inside a PATH entry (`%JAVA_HOME%\bin` with no `JAVA_HOME`) expands to `\bin`, which `filepath.Clean` keeps as a root-relative path, so the search looks in `C:\bin` instead of skipping the entry. | Drop an entry when expansion leaves it starting with a separator and no volume. |
| A15-27 | a12 | minor | `internal/app/settings.go:257-261` | The "not inside the Windows folder" check is a lower-cased `HasPrefix` with no separator, so `C:\Windows.old` and `C:\WindowsApps` are rejected as "inside the Windows folder". | Compare against `root + \` (or use `inDir`). |
| A15-28 | a09 | minor | `windows/installer/EZLM.iss:107-109` | The uninstall step runs `ezlm.exe uninstall` / `stop --for-uninstall` with `ewWaitUntilTerminated` and no timeout, so a hung engine hangs the uninstall, and the uninstaller's own files are already half removed. | Use `ewWaitUntilTerminated` with a shorter-lived helper, or `ewNoWait` and let the engine finish on its own. |
| A15-29 | a09 | minor | `windows/installer/EZLM.iss:21` | `SetupIconFile={#SrcX64}\Assets\EZLM.ico` fails to compile when only the ARM64 publish dir is defined. | Use `{#SrcArm64}` when building the ARM64 setup, or take the icon from a path both builds have. |
| A15-30 | a05 | minor | `internal/serve/service_windows.go:18` | `taskNames` captures `os.Getenv("USERNAME")` at package init. Under a service or a stripped environment it becomes `EZLM Server ()`, and the legacy task named for the real user is never disabled. | Build the name when it is used, and fall back to the token's user name. |
| A15-31 | a13 | minor | `internal/serve/service_windows.go:159-162` | `Start` returns early when any of EZLM's PIDs is running, without checking that it is this config's server (right model, right port). A stale server from a previous model blocks the start and the app shows "running" for the wrong model. | Compare the running server's `--port`/`-m` (or its config mtime) before treating it as current. |
| A15-32 | a03 | minor | `windows/EZLM/Chat.cs:1063-1071` | The BOM sniffer falls back to Latin-1 for anything that is not valid UTF-8, which is close to Windows-1252 but not identical (0x80-0x9F differ), so a CP-1252 text file with a curly quote shows a control character. | Decode with `Encoding.GetEncoding(1252)` (the default ANSI code page) rather than Latin-1. |
| A15-33 | a10 | minor | `windows/EZLM/App.xaml.cs:12` | `UnhandledException` sets `e.Handled = true` for every exception, including ones thrown while the visual tree is being built, which leaves a half-built window and no crash report the user can act on. | Handle only the known exceptions; otherwise log and rethrow. |
| A15-34 | a10 | minor | `windows/EZLM/Install.cs:228` | `ContinueWith(_ => Ui(() => ShowDeployError(deployError)))` reads the mutable field after the refresh completes; a second deploy started in between shows the new error on the old panel. | Capture the message in a local before the continuation. |
| A15-35 | a10 | minor | `windows/EZLM/Engine.cs:84-96` | `engine.log` is rotated only at engine start, so one long session (a model download with retries) grows it without bound; it is also where a support report reads from. | Rotate at a size threshold while writing, not only at open. |
| A15-36 | a10 | minor | `windows/EZLM/Memory.cs:34` | Preferences are written with a plain `File.WriteAllText`, while facts next to them use a temp file and a rename. A crash mid-write leaves a truncated `prefs.json` that fails to parse and silently resets the user's settings. | Write through a temp file and `File.Move(..., true)`, as `ChatStore.Save` does. |
| A15-37 | a11 | minor | `windows/EZLM/Taskbar.cs:34-40` | The taskbar COM object is cached in a static and created on first use, without checking that the call is on the UI thread. Today every caller is on the UI thread; a future background caller gets a silent `RPC_E_WRONG_THREAD` that `List()` turns into `broken = true`, disabling the taskbar bar for the whole session. | Assert the thread, or marshal through `Shell.dq`. |
| A15-38 | a01 | minor | `internal/paths/paths.go:39-45` | `Ensure` creates `Home`, `bin`, `models` and `logs` with `0o755`; on Windows the mode is ignored, so the folders take the profile's inherited ACL, and a redirected or shared profile exposes the model folder to whoever the share allows. | Set an explicit DACL on `Home` at creation, as for `config.json`. |
| A15-39 | a14 | minor | `internal/integrate/arch_windows.go:11-21` | `IsWow64Process2` failure falls back to `runtime.GOARCH`, which reports the build's architecture, not the machine's. On Windows 10 before 1709 an x64 EZLM on ARM64 would claim `amd64` and pick the x64 toolchain asset. `MinVersion=10.0.19041` makes this mostly theoretical. | Fall back to `GetNativeSystemInfo`, and note the emulation case in the log. |
| A15-40 | a16 | minor | `windows/installer/EZLM.iss:16-18` | `PrivilegesRequired=lowest` with `ArchitecturesInstallIn64BitMode=x64compatible or arm64` and `DefaultDirName={localappdata}\...`: the 64-bit mode only changes the registry view, which nothing here depends on, but it does make an ARM64 install look like an x64 one to any future `{pf}` use. | Drop `ArchitecturesInstallIn64BitMode` (or scope it to the registry entries that need it) and keep `ArchitecturesAllowed`. |

## Sibling findings, merged

The table above is a15's own read. This table is what the other slices
reported, in their words, with the status at their last commit. Detail and
evidence live in `agents/T15/<slice>/REPORT.md` on that slice's branch; the
`file:line` here is the one in their report.

| Slice | Sev | Area | Finding | Status |
|---|---|---|---|---|
| a08 | **blocker** | signing | Nothing EZLM ships is Authenticode-signed, so Smart App Control blocks the app and its installer outright on new Windows 11 installs. | open (release-engineering) |
| a08 | major | Hermes | Hermes installs by piping a remote script into `powershell -ExecutionPolicy Bypass`. | open |
| a08 | major | update | The updater runs an unsigned setup from `%TEMP%` and closes EZLM 1.2 s later. | **fixed**: `internal/update/update.go` downloads into `EZLM_HOME\update`; Windows tests added |
| a08 | major | update | No publisher check on the downloaded installer, and no zone identifier either way. | open |
| a08 | major | engine | Downloaded executables run from the profile (my A15-05). | open (report only) |
| a08 | minor | self-test | Check workspaces live in `%TEMP%`, and the agent CLI runs with that cwd. | open |
| a08 | minor | update | Stale `%TEMP%\EZLM-update` copies are never removed. | open |
| a13 | major | proc | A job that was never joined made a timeout hang, and leftovers survived. | **fixed** |
| a13 | major | proc | The job is assigned after the process starts, so a fast grandchild escapes (my A15-02). | **fixed**: suspended start, assign, resume |
| a13 | minor | proc | `pids()` read the wrong offset on 32-bit Windows (my A15-23). | **fixed**: portable parser in `internal/proc/jobpids.go`, 32- and 64-bit tests |
| a13 | minor | proc | A job with more than 256 processes reported no leftovers (my A15-23). | **fixed**: `ERROR_MORE_DATA` retry |
| a13 | minor | proc | The package's only Windows-relevant test skipped Windows. | **fixed** |
| a13 | minor | proc | `hide()`/`hidden()` throw away a caller's `SysProcAttr`. | open |
| a13 | minor | serve | `startLogged` never reaps the server (frozen file). | open |
| a05 | major | scan | A card's memory is missed when the driver writes it as `REG_BINARY` (my A15-19). | **fixed**: `internal/scan/winreg.go`, `adapterVRAM` |
| a05 | minor | scan | WMI output split on whitespace (my A15-18). | **fixed**: `deviceIDs` |
| a05 | major | scan | The GPU-memory fallback uses English counter names (PDH). | open |
| a05 | major | scan | The VC++ runtime check depends on the registry view. | open |
| a05 | minor | serve | Uninstall deletes another install's login entry. | open |
| a05 | minor | findtool | `%` in a registry PATH entry (my A15-26). | open |
| a14 | major | scan | An 8-byte `HardwareInformation.MemorySize` reads as 0 (same class as my A15-19). | **fixed** |
| a14 | major | scan | A zero `qwMemorySize` hides the card. | **fixed** |
| a14 | major | scan | Process ownership is wrong on Azure AD and Microsoft accounts (my A15-17). | **fixed**: SID comparison |
| a14 | minor | scan | A long non-ASCII program name was rejected. | fixed |
| a14 | minor | scan | A stray PowerShell line could become a "device" (my A15-18). | fixed |
| a14 | minor | scan | `killPID` checks the install root, not the program. | open |
| a14 | minor | scan | The one-time AMD/Intel card lookup is never retried. | open |
| a03 | major | encoding | PowerShell installer output is decoded as UTF-8, but PowerShell 5.1 writes the OEM code page. | **fixed**: `runPS` in `internal/scan/exec_windows.go`, Windows tests |
| a03 | major | encoding | `scan.run` returns raw bytes (my A15-16). | **fixed** |
| a03 | minor | encoding | The launcher's `chcp` failure is not checked. | open |
| a03 | minor | C# | The attachment decoder falls back to Latin-1, not Windows-1252 (my A15-32). | open |
| a03 | minor | C# | UTF-16 without a BOM is detected by zero high bytes only. | open |
| a03 | minor | C# | The C# engine reader leaves the child's encoding to the console. | open |
| a12 | major | paths | `paths.Home()` fell back to a relative path, so a Run-key launch could write the key and models into `C:\Windows\System32` (my A15-11). | **fixed**: `%LOCALAPPDATA%\EZLM`, then `%APPDATA%` |
| a12 | minor | paths | `EZLM_HOME` was not cleaned. | **fixed** |
| a07 | major | arch | An x64 engine in the arm64 app quietly downloads an x64 llama.cpp. | open |
| a07 | major | arch | `vcArch()` picks the VC redist for the build, not for the llama-server that runs. | open |
| a07 | minor | arch | An x64 EZLM on Arm loses unified-memory detection. | open |
| a07 | minor | arch | `cuda13=false` still selects a CUDA 13 build on arm64. | open |
| a07 | minor | report | The report header names the build architecture (my A15-39). | open |
| a10 | major | C# | Settings pane: a failed storage read loops forever. | open |
| a10 | major | C# | Overview "Stop server": a failed status refresh skips the whole update. | open |
| a10 | minor | C# | The quiet memory look races `NewChat` and writes a stale `memFrom`. | open |
| a10 | minor | C# | Streaming re-parses the whole answer 15 times a second. | open |
| a10 | minor | C# | `Shell.Log` is not thread-safe, so the log can lose the entry that matters. | open |
| a10 | minor | C# | Integrate/key events rebuild the whole pane per event. | open |
| a10 | minor | C# | Flyouts shown after an `await` on a possibly detached anchor. | open |
| a10 | minor | C# | The model test has no timeout. | open |
| a10 | minor | C# | A failed cancel leaves Cancel and Esc dead. | open |
| a10 | minor | C# | `SaveChat` writes the whole conversation on the UI thread. | open |
| a11 | major | C# | "Report this problem" inside the install dialog does nothing (silent no-op). | open |
| a11 | minor | C# | The taskbar's red error bar can stay forever when the window is in front (my A15-37). | open |
| a11 | minor | C# | The Logs pane never follows new lines; `logScroll` is dead. | open |
| a11 | minor | C# | The browser fallback for a report can fail and still claim success. | open |
| a11 | minor | C# | An update download shows no taskbar progress. | open |
| a11 | minor | C# | The report dialog reads up to 2 MB of `app.log` on the UI thread (my A15-35). | open |
| a11 | minor | C# | `--selftest` crashes instead of returning 1 when it cannot write its results. | open |
| a11 | minor | C# | `PickFiles` null check is dead code. | open |
| a16 | major | docs | "Remove everything" leaves chats and memory behind. | open |
| a16 | minor | docs | The update picker is architecture-blind. | open |
| a16 | minor | docs | The uninstaller ignores whether the engine removed the data. | open |
| a16 | minor | docs | Uninstall decides what to delete from the folder's name. | open |
| a16 | minor | docs | `BETA.md` points at the wrong folder for keys and settings. | open |
| a16 | minor | docs | The download name in `BETA.md` does not match the built asset. | open |
| a16 | minor | docs | The Windows version floor is not in the docs. | open |

**Cross-reference: rows above that a sibling has already fixed.** A15-02 and
A15-23 (a13), A15-18 and A15-19 (a05, a14), A15-11 (a12), A15-16 (a03),
A15-32 (a03, still open). Do not re-patch those; review the sibling's commit
instead.

## The two blocker candidates

**A15-01 (key on the command line).** `serve.Config.Args()` returns
`--api-key <key>`; `serve.RunForeground` and `startLogged` pass that slice to
`exec.Command`, so the key is in `lpCommandLine` for the whole life of the
server. On Windows that string is readable from Task Manager (Command line
column), `Get-CimInstance Win32_Process`, Process Explorer, and every EDR
agent's process log. It would be a blocker if the key were shared across
machines; it is a per-install loopback key, so it is ranked major. Fix:
`--api-key-file` with the file's DACL set, or an environment variable the
server reads.

**A15-04 (Smart App Control).** On a Windows 11 machine with Smart App Control
enforced, an unsigned, Mark-of-the-Web exe is blocked with no override. EZLM's
self-update path is exactly that: download to `%TEMP%`, `UseShellExecute =
true`. This needs one clean-VM run to confirm before it is called a blocker.

## What would change the ranking

- A Windows test that reproduces A15-02 (orphaned llama-server after a failed
  check) would promote it to blocker: it leaves GPU memory held with no UI to
  free it.
- Any confirmation of A15-04 on a SAC machine promotes it to blocker.
- If sibling agents' reports add a finding with a failing Windows test, it
  outranks everything here: this table is code-read only.
