# a08 fixture sources

Every fixture is a small text capture of `system_profiler` output. Names are
`system_profiler_<datatype>_<machine>.txt` under `internal/scan/testdata/`.
Serial numbers, hardware UUIDs and provisioning UDIDs are replaced with
`REDACTED` / zeroes.

| Fixture | Source |
|---|---|
| `system_profiler_displays_apple_m3_pro.txt` | Captured on this Mac (MacBook Pro, Apple M3 Pro, macOS 26.3.1, 2026-11-06) with `system_profiler SPDisplaysDataType`. Real output, unmodified. |
| `system_profiler_hardware_apple_m3_pro.txt` | Captured on the same Mac with `system_profiler SPHardwareDataType`; serial, hardware UUID and UDID redacted. Real field values and wording, including the macOS 26 capitalised "5 Performance and 6 Efficiency". |
| `system_profiler_displays_apple_headless.txt` | Synthetic, built from the documented text format: an Apple M2 Mac mini with no display attached, so no `Displays:` section. Field set matches the captured M3 Pro output (minus the display). |
| `system_profiler_displays_intel_egpu_headless.txt` | Synthetic, built from the documented text format: Intel Mac with built-in UHD 630, a Radeon Pro 5500M and an external Radeon RX 580 over PCIe, with no display attached. Shows the `VRAM (Total)` / `VRAM (Dynamic, Max)` split. |
| `system_profiler_displays_intel_igpu_only.txt` | Synthetic, built from the documented text format: Intel MacBook Air with Iris Plus Graphics only, in clamshell (headless). |
| `system_profiler_displays_intel_legacy_metal.txt` | Synthetic, built from the documented text format: macOS 10.14 to 11 printed `Metal: Supported, feature set macOS GPUFamily2 v1` instead of `Metal Support: Metal N`, and some cards print VRAM in MB. ATI is used as the vendor name, as older Macs do. |
| `system_profiler_hardware_intel.txt` | Synthetic, built from the documented text format: MacBookPro15,1 with `Processor Name` and `Total Number of Cores: 6` instead of `Chip`. |

Format references:

- `system_profiler(8)` command, data types and `-detailLevel`:
  https://ss64.com/mac/system_profiler.html (fetched 2026-11-06).
- Apple Support, "Get system information about your Mac":
  https://support.apple.com/guide/mac-help/get-system-information-about-your-mac-syspr35536/mac
  (the same System Information panes that `system_profiler` prints; the page
  would not render for a text fetch, so it is cited for the panes, not for
  byte-exact output).
- Apple Silicon field set and indentation: verified against the captured
  M3 Pro output above.

The Intel and legacy fixtures are synthetic because no Intel Mac or eGPU was
available on this host. They are not random: every line follows the same
four-space card block, six-space field layout as the captured Apple Silicon
output, and uses field names seen in public `system_profiler` dumps. They
should be replaced with real captures when an Intel Mac and an eGPU are at
hand.
