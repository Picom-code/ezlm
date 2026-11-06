# T15. Windows code review — consolidated report

16 agents (a01–a16) worked this task in parallel: nine Windows-bug categories, two C# reviews, three
Go reviews, the severity table and the manual test checklist. Consolidated onto
`agent/T15-windows-review` by the Lead. **No Windows machine was available**, so every finding that
could not be proved by a test that compiles under `GOOS=windows` is marked code-read.

## Result

- **1 blocker, 9 major, ~20 minor** findings, in `agents/T15/a15/findings.md` (ranked, with
  file:line, status and proposed fix).
- **11 fixes** in Go files this task owns, each with a test that builds under `GOOS=windows`.
- **Gate:** `gofmt -l .` empty, `go build ./...`, `go vet ./...`, `go test -p 2 ./...` pass, plus
  `GOOS=windows` amd64/arm64 and `GOOS=linux` builds.

## The blocker

**Nothing EZLM ships is Authenticode-signed.** `EZLM.iss` has no `SignTool` and there is no signing
step, so **Smart App Control on Windows 11 blocks `EZLM.exe`, `ezlm.exe`, `llama-server.exe` and
every update setup**. The updater also starts an unsigned setup from `%TEMP%` and closes EZLM 1.2 s
later; `Process.Start` does not throw when SmartScreen blocks it, so the user is left with no window
and no update (`SettingsPane.cs:295-308`). Release engineering, not a code line.

## Major findings

| Area | Finding | Status |
|---|---|---|
| **Registry / VRAM** (a05, a14) | The adapter size read `HardwareInformation.MemorySize` as a DWORD even when it is an 8-byte `REG_BINARY`, and a zero `qwMemorySize` stopped the fallback chain, so an 8–16 GB Arc read as 4 GB or 0 and was dropped. | **Fixed** (`internal/scan/winreg.go`, `display_windows.go`) |
| **Code page** (a03) | PowerShell 5.1 writes redirected stdout in the OEM code page; `internal/integrate` (frozen) decodes it as UTF-8, so the log, UI progress and installer error text show mojibake for any non-ASCII output or user path. | **Fixed for the adapter probe** (`runPS`); frozen sites reported |
| **Console encoding in scan** (a03) | The same class in `scan.run`. | Fixed for the probe; `gpu_discrete.go` one-line swaps left for its owner |
| **Account-name folding** (a02) | `apps_windows.go` lowercased only the current user's name before `EqualFold`; a user named `İbrahim` had no processes marked "mine" (MemoryApps empty, Kill refused). | **Fixed** (`sameUser`) |
| **Job-object race** (a13) | `track()` ignored job-assignment failures, so a deadline hung forever and `close()` killed nothing. | **Fixed** (root handle + `TerminateProcess` fallback) |
| **Job assignment after Start** (a13) | A grandchild spawned between `Start` and `AssignProcessToJobObject` escapes the job. | Reported (needs `CREATE_SUSPENDED`) |
| **API key in argv** (a06) | On Windows any local user reads another process's command line, so a second account could read the key. | **Fixed** for `startLogged` (env var); `run.go run-server` still passes it in argv |
| **Path length** (a01) | `os/exec` never adds `\\?\`, so a server or model path over 260 chars fails; the Run key has the same 260-char limit. | **Fixed** for `startLogged` (`paths.Extended`); model/draft paths reported |
| **Autostart** (a09) | A CLI-only install's Run entry wrote `ezlm start`, which clears the stopped flag, so a stopped server came back at every sign-in. | **Fixed** (`start --login`) |
| **Windows on Arm** (a07) | `engine.Install` picks llama.cpp by `runtime.GOARCH`, so an x64 engine in the arm64 app downloads an x64 engine and runs it under emulation; `integrate/arch_windows.go` already has the right `IsWow64Process2` check. | Reported (needs `scan.MachineArch()`) |
| **32-bit job PID list** (a13) | Parsed with a 64-bit layout. | **Fixed** (`jobpids.go`, both layouts tested) |
| **C# storage loop** (a10) | A failed `GET /api/storage` leaves storage null and re-triggers `ShowSection()` — an unbounded rebuild+request loop. | Reported (frozen) |
| **C# dead report path** (a11) | "Report this problem" in the install-failure dialog is a silent no-op (`Install.cs:166` while `dialogOpen` holds). | Reported (frozen) |
| **Beta-report redaction** (a12) | Only the exact `%USERPROFILE%` spelling is replaced, so `C:/Users/Name`, lowercase or `\\?\` forms leak the Windows user name into a public GitHub issue. | Reported (frozen `internal/app`) |

## Also fixed

`paths.Home()` no longer falls back to a relative `./.ezlm` on Windows and cleans `EZLM_HOME`
(a12); the updater downloads into `EZLM_HOME\update` instead of `%TEMP%` (a08); `binaryVRAM` handles
both 4- and 8-byte registry values (a05/a14); WMI output is no longer split on whitespace (a05);
`fileDescription` counts runes, not bytes (a14); the owner comparison handles Microsoft/AzureAD
accounts (a14); `internal/proc` gained its first Windows test (a13).

## Found, not fixed (with proposed fixes in the reports)

- `internal/integrate` (frozen): PowerShell `-Command` with inline scripts, `%` expansion in the
  launcher's `start` title and `/D`, `Get-Counter` English counter names on localized Windows,
  `findtool_windows.go` `%`/`$` double-expansion, relative Startup shortcut when `%APPDATA%` is unset.
- `internal/prereq` (no owner): `reg query` reads the WOW64 view from a 386 build, and its
  version regex never matches `reg query` output, so the VC++ redist always looks missing.
- `EZLM.iss`: uninstall ignores the engine's exit code and has no timeout, so "delete my models" can
  silently leave `~/.ezlm`; `IsArm64` payload choice under x64 emulation; the installer is never
  compiled by CI (no ISCC and no `.github` in the snapshot).
- `serve/service_windows.go`: the server handle is never reaped (one leak per start); `pids()` still
  reads a job list that can be truncated.

## Merge notes

Three shared files needed hand-repair: `apps_windows.go` (a02 + a14), `display_windows.go`
(a03 + a05 + a14), `service_windows.go` (a01 + a06 + a09). The merge had to reconcile two
independent `adapterVRAM`/`deviceIDs` helpers (a05's `winreg.go` won) and then remove the dead
`binaryVRAM` path. All three Windows cross builds pass after the repair.

## Files

- `internal/scan/winreg.go`, `display_windows.go`, `apps_windows.go`, `exec_windows.go`,
  `registry_windows_test.go`, `accounts_windows_test.go`, `console_windows_test.go`.
- `internal/proc/proc_windows.go`, `jobpids.go`, `proc_windows_test.go`.
- `internal/serve/service_windows.go`, `runkey.go`, `acl_windows_test.go`, `invoke_windows_test.go`.
- `internal/paths/long_windows_test.go`, `paths_windows_test.go`.
- `internal/update/update.go` (download dir), `internal/engine/arch_windows_test.go`.
- `agents/T15/a15/findings.md`, `agents/T15/a16/windows-checklist.md` (538 lines, sections A–P),
  and one report per agent.

## Next steps

1. **Sign and notarize** — the one blocker.
2. Add `scan.MachineArch()` and use it in `engine.Install` and `prereq.vcArch`.
3. Fix the frozen `internal/integrate` encoding and quoting sites.
4. Run `agents/T15/a16/windows-checklist.md` on a clean x64 and ARM64 VM before the beta.
