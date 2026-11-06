# T05 a06 fixture sources

All fixtures in `internal/scan/testdata/locale/` are **synthetic samples
built from the documented formats**. No real machine dump was used. The
vendor and model names come from the vendors' public product pages and their
localized branding; the registry layout, counter names and "N/A" spellings
come from the Microsoft documents below.

## Registry / display class

- Display class GUID `{4d36e968-e325-11ce-bfc1-08002be10318}` and the
  `SYSTEM\CurrentControlSet\Control\Class\...` layout:
  <https://learn.microsoft.com/en-us/windows-hardware/drivers/install/system-defined-device-setup-classes-available-to-vendors>
- Display driver values read by `readAdapter` (`DriverDesc`, `ProviderName`,
  `HardwareInformation.qwMemorySize`, `HardwareInformation.MemorySize`):
  <https://learn.microsoft.com/en-us/windows-hardware/drivers/display/>
- The 32-bit `HardwareInformation.MemorySize` clamp is the documented old
  driver form; the 64-bit `qwMemorySize` is the current one. The 4 GB clamp
  itself is covered by a05; this slice only uses the 64-bit form.

## Performance counters / GPU memory

- `PdhAddEnglishCounterW` (English counter names on any Windows language):
  <https://learn.microsoft.com/en-us/windows/win32/api/pdh/nf-pdh-pdhaddenglishcounterw>
- `PDH_FMT_COUNTERVALUE_ITEM_W` (instance name + status + value):
  <https://learn.microsoft.com/en-us/windows/win32/api/pdh/ns-pdh-pdh_fmt_countervalue_item_w>
- `Win32_PerfRawData_GPUPerformanceCounters_GPUProcessMemory`
  (`Name`, `DedicatedUsage`): Windows GPU performance counters, instance
  names of the form `pid_<pid>_luid_..._phys_<n>`.
- PowerShell `Get-Counter` (the fallback path in `windowsGPUUsage`):
  <https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.diagnostics/get-counter>
- `Win32_VideoController` (`PNPDeviceID`):
  <https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-videocontroller>

## Localized names used in the fixtures

The DriverDesc strings are representative, not copied from a dump:

- German: `Intel(R) UHD-Grafik 770`, `AMD Radeon(TM) RX 7800 XT`,
  `NVIDIA GeForce RTX 4070`.
- Japanese: `インテル(R) Arc(TM) B580 グラフィックス`,
  `AMD Radeon(TM) グラフィックス`, `インテル(R) Iris(R) Xe グラフィックス`.
  Intel Japan brands the company as インテル: <https://www.intel.co.jp/>
- Chinese: `英特尔(R) Arc(TM) A770 显卡`, `英特尔(R) UHD 显卡 770`.
  Intel China brands the company as 英特尔: <https://www.intel.cn/>
- Model names: AMD Radeon RX 7800 XT / RX 7900 XTX
  <https://www.amd.com/en/products/graphics/desktops/radeon/7000-series.html>,
  NVIDIA GeForce RTX 4070
  <https://www.nvidia.com/en-us/geforce/graphics-cards/40-series/rtx-4070-4070ti/>.

## "N/A" spellings

- nvidia-smi prints `[N/A]` in every locale (the CSV is not translated);
  the fixtures also carry the common translations `Nicht zutreffend`
  (German), `該当なし` (Japanese) and `不适用` (Chinese) to prove the parser
  skips them safely.
- Counter noise lines use the same translated words; `parseGPUUsage` only
  accepts `<instance> <bytes>` lines whose instance starts with `pid_`.

## Decimal comma

`1.073.741.824` in `gpu_usage_zh.txt` is a synthetic localized rendering of
one gigabyte. It is not produced by the current WMI/PDH path (which prints
`UInt64` without separators); the fixture documents that a localized value
is skipped rather than misread.
