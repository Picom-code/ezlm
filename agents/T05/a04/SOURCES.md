# a04 sources: Win32_VideoController WMI output

The parser under test is `displayAdapters` in `internal/scan/display_windows.go`.
It runs:

    Get-CimInstance Win32_VideoController -ErrorAction SilentlyContinue |
      ForEach-Object { $_.PNPDeviceID }

so the fixtures in `internal/scan/testdata/display_wmi_*.txt` are the device ID
list that command prints, one line per adapter. The two `display_wmi_*` dumps
that contain a full record use the `Format-List` field layout, which is what the
same class gives when a human or a bug report dumps it.

## Where each fixture comes from

| Fixture | What it is | Source |
|---|---|---|
| `display_wmi_desktop_amd.txt` | One discrete Radeon RX 7800 XT (Navi 32, `DEV_747E`) in a desktop. | Synthetic, built from the documented PNP device instance format (`PCI\VEN_xxxx&DEV_xxxx&SUBSYS_...&REV_..\...`) and the RX 7800 XT device ID. |
| `display_wmi_two_adapters.txt` | Intel Arc B580 (`DEV_E20B`, Battlemage) plus a Ryzen Raphael iGPU (`DEV_164E`). | Synthetic, same format; device IDs from the PCI ID database entries for Battlemage and Raphael. |
| `display_wmi_ghost_egpu.txt` | Laptop whose eGPU was unplugged: Windows lists only the iGPU, while the class key keeps the removed card's entry. | Synthetic. The behaviour (class entries outlive the card, `Driver` value links device instance to class entry) is documented by Microsoft: <https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-videocontroller> (InfFilename `MappingStrings` shows `SYSTEM\CurrentControlSet\Control\Class\{4D36E968-...}\0000`) and the device instance `Driver` value documented at <https://learn.microsoft.com/en-us/windows-hardware/drivers/install/device-instance-ids>. |
| `display_wmi_null_id.txt` | An adapter with no PNPDeviceID (blank line) next to one with an ID. | Synthetic. `PNPDeviceID` is a plain read-only string, so PowerShell prints an empty line for a null value. |
| `display_wmi_vm_virtio.txt` | VM with a virtio-gpu adapter only (`VEN_1AF4`, `DEV_1110`). | Synthetic; QEMU/virtio device ID `1af4:1110`. |
| `display_wmi_none.txt` | No adapters at all (headless server / container). | Synthetic: the command prints nothing. |
| `display_wmi_adapterram_4gb.txt` | Full record for a 24 GB RTX 4090 whose `AdapterRAM` reads 4293918720. | Synthetic record built from the class definition at the Microsoft page above: `AdapterRAM` is `uint32` with `Units("bytes")`, so it cannot exceed 4294967295 bytes (about 4 GB). 4293918720 is the value such cards report (4095 MiB, the classic 4 GB clamp). |
| `display_wmi_config_error_43.txt` | Full record for an RX 7800 XT with `ConfigManagerErrorCode` 43. | Synthetic record; error codes are from the same Microsoft page (43 is not in the public list, which ends at 31, but 22 "This device is disabled" and the general `ConfigManagerErrorCode` property are). |

## Caveats

- No fixture was captured from real hardware by this agent (the work machine is a
  Mac). Device IDs and record layouts are documented values, not a capture.
- The class-entry values (`DriverDesc`, `ProviderName`, `qwMemorySize`) are
  registry data, not WMI output, and belong to a05's slice; they appear here
  only in the test's fake `Driver` lookup.
