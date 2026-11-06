# T05 a04: Windows `Win32_VideoController` WMI output

## What this slice covers

`displayAdapters` in `internal/scan/display_windows.go` is the only code that
reads WMI. It runs `Get-CimInstance Win32_VideoController` and keeps one
`PNPDeviceID` per line, then follows each device's `Driver` value to the
display-class entry that holds the card's name and memory
(`internal/scan/display_windows.go:48`).

Fixtures: `internal/scan/testdata/display_wmi_*.txt` (provenance in
`SOURCES.md`). Tests: `internal/scan/display_wmi_test.go`.

| Case | Fixture | Result |
|---|---|---|
| normal desktop, one Radeon | `display_wmi_desktop_amd.txt` | one ID, read as-is |
| ghost / disconnected card | `display_wmi_ghost_egpu.txt` | the removed card's class entry is never reached; only the iGPU is read |
| `AdapterRAM` wrong (4 GB clamp) | `display_wmi_adapterram_4gb.txt` | 24 GB card reports 4293918720 bytes; the parser uses `qwMemorySize`, so the clamp is not hit |
| multiple adapters | `display_wmi_two_adapters.txt` | both IDs, in Windows' order |
| no adapter | `display_wmi_none.txt` | no IDs, `displayScan` falls through to the next backend |
| null `PNPDeviceID`, CRLF, repeats | `display_wmi_null_id.txt` + inline case | blank lines, CRLF and padding are dropped, repeats collapse |
| VM, virtio-gpu only | `display_wmi_vm_virtio.txt` | one ID, but no AMD/Intel vendor, so no usable card |
| failed driver (code 43) | `display_wmi_config_error_43.txt` | listed by WMI; still counted (finding 1) |

## Change

`internal/scan/display_wmi.go` (new, no build tag) now holds the parsing:
`videoControllerIDs`, `adapterKeys` and `adapterKeyFromDriver`.
`displayAdapters` calls them, with the registry lookups passed in as a
function. Behaviour is the same for well-formed output; the move makes the
WMI half of the Windows display path testable on the Linux/macOS test hosts,
because everything else in `display_windows.go` is behind `//go:build windows`.
No new dependencies.

The old code did `strings.Fields(out)`, which would turn a device ID
containing a space into two bogus IDs (both then dropped silently). Parsing
line by line is now explicit and a blank or padded line is handled.

## Findings

**1. A failed or disabled adapter is still counted as usable (major, open).**
`internal/scan/display_windows.go:50` asks only for `PNPDeviceID`. WMI lists
every video controller known to the machine, including ones whose driver is
not running: `ConfigManagerErrorCode` 43 ("Windows has stopped this device")
or 22 ("This device is disabled"), see
<https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-videocontroller>.
The class entry and its `qwMemorySize` stay in place, so such a card is read
with its full memory and can win `usableAdapters`
(`internal/scan/gpu_discrete.go:203`), and EZLM would plan a Vulkan deployment
for a card that cannot run it. Evidence: `display_wmi_config_error_43.txt`
and `TestVideoControllerListsBrokenAdapter`, which shows the record is listed
all the same. Not fixed here: the fix is in the PowerShell query (add
`Where-Object { $_.ConfigManagerErrorCode -eq 0 }`) and this host cannot run
Windows to check it against real hardware. Suggested next step: add the filter
behind a hardware test on a Windows box with a disabled adapter.

**2. The `AdapterRAM` clamp is avoided, but only by accident of choice (minor).**
`Win32_VideoController.AdapterRAM` is a `uint32` of bytes, so it cannot
report more than 4294967295 bytes (about 4 GB); a 24 GB RTX 4090 reads back
4293918720 (4095 MiB). `readAdapter` uses the registry's
`HardwareInformation.qwMemorySize` instead, so the clamp does not bite today.
`TestAdapterRAMClampsAt4GB` pins the numbers so a future change that switches
to `AdapterRAM` fails a test instead of silently planning a 4 GB machine.
Severity minor: no current user impact.

**3. Ghost cards are handled correctly (no bug).** The class key
`{4d36e968-...}` keeps an entry for a card that was taken out, but entries are
only reached through the `Driver` value of an adapter WMI lists, so the old
card's memory cannot come back. `TestAdapterKeys` pins this, including the
re-plugged case and an adapter whose instance key is missing.

## Could not finish / open questions

- No Windows machine was available, so nothing here was checked against a real
  `Get-CimInstance` run: the fixtures are documented-format synthetics.
  `TestReadsThisPCsGraphics` (already in the tree) is the hardware check.
- Should a card with `ConfigManagerErrorCode` 22 (disabled on purpose) be
  offered at all? Filtering to 0 drops it; that seems right, but it is a
  product call.
- `displayAdapters` returns nil on any PowerShell error, so a machine where
  CIM is broken looks like a machine with no GPU. Not tested here (it needs a
  broken WMI repository); it fails safe to the CPU backend.

## Checks run

`gofmt -l internal/scan/` (empty), `go build ./...`, `go vet ./internal/scan/`,
`go test -p 2 ./internal/scan/`, `GOOS=windows GOARCH=amd64 go build ./...`,
`GOOS=windows GOARCH=arm64 go build ./...`, `GOOS=windows go vet ./internal/scan/`.
