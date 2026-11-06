# a05 fixture sources: Windows display-class registry values

All fixtures live in `internal/scan/testdata/`. Format: one registry value per
line, `Name TYPE data` (`REG_SZ` quoted, `REG_QWORD`/`REG_DWORD` hex with
`0x`, `REG_BINARY` hex bytes); `#` lines are provenance. The test that reads
them is `internal/scan/display_registry_test.go`.

## Real registry data

- `display_amd_rx9070.txt`, `display_amd_igpu.txt`
  Source: odysseus-dev/odysseus issue #6612, "Windows hardware scan picks the
  integrated GPU instead of the discrete card", 2026-10-08.
  https://github.com/odysseus-dev/odysseus/issues/6612
  The report shows the probe's output on Windows 11 Pro 25H2 (Ryzen 7600):
  `AMD Radeon(TM) Graphics | AdapterRAM=536870912 | qwMemorySize=536870912`
  and `AMD Radeon RX 9070 | AdapterRAM=4293918720 | qwMemorySize=17095983104`.
  The fixtures keep the `DriverDesc` name, the AMD `ProviderName` and the
  `qwMemorySize` value. `ProviderName` is the standard AMD string, not quoted
  in the report.

- `display_intel_arc_a770.txt`
  Source: AlexsJones/llmfit issue #1180, 2026-10-22.
  https://github.com/AlexsJones/llmfit/issues/1180
  `llmfit doctor` reports the registry value for an Intel Arc A770 16 GB
  (`qwMemorySize = 16106127360`) next to `Win32_VideoController.AdapterRAM =
  4293918720`. The same issue shows `DriverDesc` = `Intel(R) Arc(TM) A770
  Graphics`. The fixture also carries the 4-byte `MemorySize` sentinel from
  oshi (below), because the parser must prefer `qwMemorySize` when both exist.

- `display_intel_arc_memorysize_4byte.txt`
  Source: oshi/oshi issue #2907 (2025-06-07) and pull request #3086
  (2026-03-10).
  https://github.com/oshi/oshi/issues/2907
  https://github.com/oshi/oshi/pull/3086
  Issue #2907 shows `HardwareInformation.MemorySize` bytes `00 F0 FF 7F` on
  an Arc A770 16 GB and a reported VRAM of 2.00 GB. PR #3086 states the value
  is 32-bit and Windows writes the sentinel `0x7FFFF000` for any card over
  2 GiB, and that `qwMemorySize` is absent on at least some Intel Arc driver
  installations.

## Synthetic cases

- `display_intel_arc_memorysize_qword.txt`: an 8-byte `REG_BINARY`
  `HardwareInformation.MemorySize` holding 16106127360 (15 GiB). Built from
  the documented `REG_BINARY` form of the value; the 8-byte length is the
  case under test, and the 16-byte hex is little-endian. If a real 8-byte
  dump turns up, replace it.

- `display_intel_uhd_770.txt`: `qwMemorySize = 0` plus a 32-bit
  `MemorySize = 0x40000000` (1 GiB) for an Intel UHD 770. The 1 GiB figure
  comes from the `Win32_VideoController` line in llmfit #1180; the zero
  `qwMemorySize` is synthetic. It exercises the fallback when a driver leaves
  the 64-bit value at zero.

- `display_missing_key.txt`: an entry with no values at all (the class subkey
  was removed). Provenance for the case that `qwMemorySize` is not always
  present: R1ftzy/WinFetchPlus issue #1, 2026-07-13,
  https://github.com/R1ftzy/WinFetchPlus/issues/1

- `display_ghost_entry.txt`: a stale display-class entry with `DriverDesc`
  but no `HardwareInformation` values, built from the documented layout of
  the display class key.

## Format references

- Microsoft, registry value types (`REG_SZ`, `REG_DWORD`, `REG_QWORD`,
  `REG_BINARY`): https://learn.microsoft.com/windows/win32/sysinfo/registry-value-types
- Microsoft, display class registry key
  `{4d36e968-e325-11ce-bfc1-08002be10318}`, the key the parser reads:
  https://learn.microsoft.com/windows-hardware/drivers/display/
