# T15 / a14: Go registry, WMI and scan (Windows)

Area: `internal/scan/display_windows.go`, `internal/scan/apps_windows.go`,
`internal/scan/exec_windows.go`; parsing robustness and locale handling. Code
read plus tests that build under `GOOS=windows`. No Windows machine here, so
nothing that needs a real registry, WMI service or version resource was run.

## Findings, by severity

### 1. major: an 8-byte `HardwareInformation.MemorySize` reads as 0

`internal/scan/display_windows.go:104` (before the fix). The older fallback
did `uint64(binary.LittleEndian.Uint32(b))` whenever `len(b) >= 4`. A driver
that writes the size as a binary QWORD (8 bytes) has its high half ignored:
8 GB is `0x2_0000_0000`, so the low DWORD is 0. `readAdapter` then reports
`VRAM: 0` and `usableAdapters` (`gpu_discrete.go:209`, needs 2 GB) drops the
card, so an AMD or Intel machine falls back to the CPU backend with no note.
Fixed by `binaryVRAM` (`display_windows.go:131`), which reads 8 bytes as a
QWORD and 4 as a DWORD. Test: `TestAdapterVRAM`, cases "8-byte binary value"
and "8-byte binary beats a zero 64-bit value".

### 2. major: a zero `qwMemorySize` hides the card

`internal/scan/display_windows.go:98-104` (before the fix). The old
`if/else if` chain stopped at the first value that *existed*, not the first
that *held a size*. A driver that leaves `HardwareInformation.qwMemorySize`
present but 0 (or leaves a 4-byte `MemorySize` of 0) reported 0 bytes and the
card was dropped, even though another value in the same key had the real size.
Fixed by `adapterVRAM` (`display_windows.go:115`), which treats 0 as missing
and tries the next value. Test: `TestAdapterVRAM`, cases "empty 64-bit value
falls through" and "all empty".

### 3. major: process ownership is wrong on Azure AD and Microsoft accounts

`internal/scan/apps_windows.go:38` and `:58` (before the fix). `ownedByMe`
and `listProcs` compared `p.UsernameWithContext` with `user.Current()`
using `strings.EqualFold`. The two APIs name the same account differently:
`os/user` returns the SAM-compatible form (`LAPTOP\patrick`,
`AzureAD\patrick`), while gopsutil's WMI `Win32_Process.GetOwner` returns the
token's domain, which for a Microsoft account is `MicrosoftAccount\...` and
for Azure AD can be `AzureAD\...` (and SAM truncates local names over 20
characters). A mismatch makes `mine` false for every process, so Model mode
offers nothing to close, `Kill` returns `os.ErrPermission`, and `TopMem`
shows nothing as killable. Fixed by `sameUser`/`accountPart`
(`apps_windows.go:36`, `:44`), which compares the account part when the full
spellings differ. Test: `TestSameUser`. Trade-off: two accounts that differ
only in domain now compare equal; a wrong "mine" only leads to a kill that
Windows refuses, not to killing another user's process.

### 4. minor: a long non-ASCII program name was rejected

`internal/scan/apps_windows.go:213`. `len(s) < 60` counted UTF-8 bytes, so a
localized `FileDescription` of 60+ bytes (20 CJK characters, say) fell through
to `ProductName` and then to the en-US entry, mixing languages in the app
list. Now `utf8.RuneCountInString`. No test: it needs a real version
resource; `TestFileDescription` only checks that a description is read at all.

### 5. minor: a stray PowerShell line could become a "device"

`internal/scan/display_windows.go:60` (before the fix). `strings.Fields(out)`
split on any whitespace, so a warning line, a blank line, or a device path
containing a space became one or two bogus IDs. Harmless (the registry open
fails), but it hides the real cause when no card is found. Fixed by
`deviceIDs` (`display_windows.go:78`): one ID per line, must contain `\`, no
inner whitespace, leading BOM dropped. Test: `TestDeviceIDs`.

### 6. minor: `killPID` checks the install root, not the program

`internal/scan/apps_windows.go:149-157`. After a PID is reused, it compares
`installRoot` of the old and new exe, so a recycled PID of any other exe in
the same install folder (a Chrome crashpad handler for chrome.exe) passes the
check and gets terminated. Proposed fix: compare the cleaned exe paths
case-insensitively, with the root comparison only as a fallback. Not changed:
on a path reported in 8.3 or a different long-name form the stricter check
would skip the kill and leave the app open.

### 7. minor: the one-time AMD/Intel card lookup is never retried

`internal/scan/display_windows.go:163`. `otherGPUMemory` sets
`otherGPU.asked = true` before the background PowerShell read finishes, and
`displayScan` only sets `total` on a successful scan. If that first read
fails (PowerShell slow, WMI busy) the dashboard shows no GPU memory for the
rest of the session. Proposed fix: only set `asked` from the goroutine when
it has a result, and keep a `tried` flag for the "don't spawn twice" part.

### 8. note: `exec_windows.go` is fine

`hide` sets `HideWindow` plus `CREATE_NO_WINDOW` (0x08000000), which is the
right pair for a GUI-subsystem parent: `HideWindow` covers GUI children,
`CREATE_NO_WINDOW` stops a console flash for `powershell`/`nvidia-smi`, and
`cmd.Output()` still gets the pipes. One caveat: it overwrites
`cmd.SysProcAttr`, so a caller that sets its own flags before `hide` loses
them. No caller does today.

### 9. note: WMI and registry names are locale-safe

The WMI class `Win32_PerfRawData_GPUPerformanceCounters_GPUProcessMemory`
and its `Name`/`DedicatedUsage` properties, `Win32_VideoController.PNPDeviceID`,
the display class key and its `DriverDesc`/`ProviderName`/
`HardwareInformation.*` values are all locale-independent, and
`parseGPUUsage` (`gpu_discrete.go:127`) accepts the `pid_...` instance names
on any language. The `Get-Counter '\GPU Process Memory(*)\Dedicated Usage'`
fallback (`gpu_discrete.go:116`) is the one localized path: it needs the
English counter path, and `PdhAddEnglishCounterW` in `display_windows.go:162`
is used instead where it can be. That fallback is a03's area, not changed
here.

## What changed

- `internal/scan/display_windows.go`: `deviceIDs`, `adapterVRAM`,
  `binaryVRAM`; `displayAdapters` and `readAdapter` use them.
- `internal/scan/apps_windows.go`: `sameUser`, `accountPart`;
  `ownedByMe`/`listProcs` use `sameUser`; `fileDescription` counts runes.
- `internal/scan/registry_windows_test.go` (new): `TestAdapterVRAM`,
  `TestDeviceIDs`, `TestSameUser`, `TestFileDescription`.

All of it is inside files a14 owns. No change to `gpu_discrete.go`,
`exec_windows.go`, `windows/**` or any frozen area.

## What I could not verify

- Every finding above is code-read; the new tests build under
  `GOOS=windows GOARCH=amd64` and `arm64` but were not run. The pure helpers
  (`adapterVRAM`, `binaryVRAM`, `deviceIDs`, `sameUser`, `accountPart`) were
  copied into a scratch package and run on macOS, so the assertions are known
  to match the code; the Windows-only paths (registry, version resource) were
  not exercised.
- Whether real drivers write `HardwareInformation.MemorySize` as 4 or 8 bytes,
  and whether a 4-byte value is bytes or MB. The fix treats it as bytes, as
  the old comment did. If some Intel drivers write MB, a 4 GB card reads as
  4096 bytes and is dropped; worth checking on real hardware (see below).
- Whether `Win32_VideoController` on a multi-GPU laptop lists the same card
  twice. If it does, `usableAdapters` would add it twice.
- `PdhGetFormattedCounterArrayW` struct layout on 386 (not shipped) and the
  `unsafe.Slice` over the PDH buffer; fine for amd64/arm64 by inspection.

## Next steps

1. On a Windows VM: run `go test ./internal/scan/ -run 'AdapterVRAM|DeviceIDs|SameUser|FileDescription'`
   and `EZLM_EXPECT_GPU=1 go test ./internal/scan/ -run TestReadsThisPCsGraphics`.
2. Check `HardwareInformation.MemorySize` on an Intel iGPU and an older AMD
   card (`reg query "HKLM\SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}\0001"`).
3. Consider reading the display adapters from
   `Enum\PCI` + the class key directly instead of PowerShell: it removes the
   `Get-CimInstance` startup cost from every scan and the dependency on
   PowerShell being allowed by policy (a14 finding, not fixed here).
