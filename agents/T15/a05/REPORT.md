# T15 a05: registry and WMI on localized Windows

Scope: value names vs display names, 32/64-bit registry views, missing keys, WMI classes
that differ by edition. Owned code: `internal/scan/display_windows.go`,
`internal/integrate/findtool_windows.go`, `internal/serve/service_windows.go`, and the
shared registry callers `internal/prereq/prereq.go` and `internal/scan/gpu_discrete.go`
(findings only for the shared files).

No Windows machine was available, so the fix is proved by a unit test that runs on the
Mac and by `GOOS=windows` builds; the remaining items are code-read.

## Findings

### 1. MAJOR (fixed): a card's memory is missed when the driver writes it as REG_BINARY
`internal/scan/display_windows.go:84` (before the fix) read
`HardwareInformation.qwMemorySize` with `GetIntegerValue`, which returns
`ErrUnexpectedType` for anything that is not REG_DWORD or REG_QWORD
(`golang.org/x/sys/windows/registry/value.go:228`). Drivers write that value as an
8-byte REG_BINARY as well as a QWORD. When it is binary, the code fell through to
`HardwareInformation.MemorySize`, the older 4-byte value that older drivers cap at
`0xFFFFFFFF` (the comment at line 87 says so). An Intel Arc with 8, 12 or 16 GB then
reads as 4.0 GB, and a card with only the binary form reads as 0 and is dropped by
`usableAdapters` (`internal/scan/gpu_discrete.go:215`, needs 2 GB for a discrete card).
The scan then reports no AMD or Intel card, or one that is too small for the model the
machine can actually run.
Fix: `adapterVRAM`/`regValue` (`internal/scan/winreg.go`) accept a number or 8/4 bytes
of little-endian binary for either value name, and `readAdapter` reads both values
through `regValueAt` (`internal/scan/display_windows.go:93`). Test:
`TestAdapterVRAMFromEveryDriverType` (`internal/scan/registry_test.go`).

### 2. MINOR (fixed): WMI output split on whitespace
`internal/scan/display_windows.go:56` used `strings.Fields(out)` on the PowerShell
output. On a localized Windows a message can reach stdout, and every word of it became
a device ID and a registry open. `deviceIDs` now takes one device ID per line and skips
lines with spaces (a PnP device instance ID has none). Test:
`TestDeviceIDsSkipsMessagesAndBlankLines`.

### 3. MAJOR (code-read, not fixed): the GPU-memory fallback uses English counter names
`internal/scan/gpu_discrete.go:114-116`. The WMI class
`Win32_PerfRawData_GPUPerformanceCounters_GPUProcessMemory` has English class and
property names on every language, so the primary path is fine. The fallback
`Get-Counter '\GPU Process Memory(*)\Dedicated Usage'` uses the *localized* counter set
name, which on German or Japanese Windows does not exist (the comment above the function
admits this). The class is also missing on Windows Server and on installs without the
GPU performance provider, which is exactly when the fallback runs. Result: no per-process
VRAM, so the dashboard and the "other apps hold X" numbers are empty.
Proposed fix: call `PdhAddEnglishCounterW` with `\GPU Adapter Memory(*)\Dedicated Usage`
and `\GPU Process Memory(*)\Dedicated Usage`, as `adapterMemoryUsed`
(`internal/scan/display_windows.go:152`) already does, instead of shelling out to
`Get-Counter`. The file is shared, so I left it alone.

### 4. MAJOR (code-read, not fixed): the VC++ runtime check depends on the registry view
`internal/prereq/prereq.go:93` runs `reg query HKLM\SOFTWARE\Microsoft\VisualStudio\14.0\VC\Runtimes\x64`.
`reg.exe` is resolved from `System32`, so a 32-bit EZLM process runs the WOW64
`reg.exe` and reads the `Wow6432Node` view, where the `Runtimes\x64` key does not
exist. The check would then say "Not installed" on a machine that has it, and EZLM would
download and run the redistributable at every launch. The x64 and ARM64 builds are
unaffected. Proposed fix: `reg query ... /reg:64` (the value name `Version` and its data
`v14.x.y` are not localized, so the rest of the parse is language-safe).
Two smaller points on the same line: `run` ignores the exit code and the stderr text,
which is localized, so a missing key and a broken `reg.exe` look the same (both give
"Not installed"); and the regexp expects the leading `v` that the redistributable writes,
so a bare `14.44.35211.00` would read as not installed.

### 5. MINOR (code-read, not fixed): uninstall deletes another install's login entry
`internal/serve/service_windows.go:127` deletes the `EZLM` value from
`HKCU\Software\Microsoft\Windows\CurrentVersion\Run` without checking that it points at
this install. A test instance is excluded by `testInstance()`, but two real installs (for
example a per-user copy next to an admin one) share the value. Proposed fix: read the
value and only delete it when it names this executable.
The 32/64-bit view is not a problem here: `HKCU\Software\...\Run` is not redirected.

### 6. MINOR (code-read): `%` in a registry PATH entry
`internal/integrate/findtool_windows.go:63` treats the text between the first two `%`
signs as a variable, so `C:\100% %TEMP%` expands wrongly, and the whole entry is then
passed to `os.ExpandEnv`, which also eats a literal `$`. Unlikely in a PATH, but the
`System32`/`Environment` keys themselves are not redirected, so the 32/64-bit view is
fine there.

## What I changed

- `internal/scan/winreg.go` (new, no build tag so the tests run on the Mac):
  `regValue`, `adapterVRAM`, `deviceIDs`.
- `internal/scan/display_windows.go`: `readAdapter` uses `regValueAt`/`adapterVRAM`;
  `displayAdapters` uses `deviceIDs`.
- `internal/scan/registry_test.go` (new): the two tests above.

Checks: `gofmt -l .` empty, `go build ./...`, `go vet ./...`,
`GOOS=windows GOARCH=amd64 go build ./...`, `GOOS=windows GOARCH=arm64 go build ./...`,
`go test -p 2 ./internal/scan/`.

## Could not verify without Windows

- The actual registry type each vendor writes for `HardwareInformation.qwMemorySize`
  (binary on some Intel and AMD drivers is what the extra cases cover; a QWORD still
  works).
- That `Win32_PerfRawData_GPUPerformanceCounters_GPUProcessMemory` is missing on the
  editions claimed in finding 3.
- The WOW64 behaviour in finding 4 (only reachable from a 386 build).

## Open questions

- Does EZLM ever ship a 386 build? If not, finding 4 is latent only.
- Should `readAdapter` also fall back to `HardwareInformation.MemorySize` when
  `qwMemorySize` is present but zero? A zero QWORD currently wins over a real 4-byte
  value.
