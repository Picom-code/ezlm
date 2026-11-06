# T05 a06: localized Windows

Slice: German, Japanese and Chinese display names and counters, decimal
commas, localized "N/A" equivalents, and a non-ASCII user profile path.

Branch `agent/T05-a06`, worktree `~/ezlm-agents/work/subs/T05/a06`.
No parser changes were needed; this slice adds fixtures and table tests.

## What the tests cover

| Parser | File | Fixture | Assertion |
|---|---|---|---|
| `usableAdapters`, `vendorOf`, `otherBandwidth` | `internal/scan/gpu_discrete.go:203`, `:231`, `:244` | `testdata/locale/cases.json` (de-DE, ja-JP, zh-CN, ja-JP built-in only) | localized `DriverDesc` still yields the right vendor (via English `ProviderName`), model, VRAM and bandwidth; NVIDIA entries stay out (nvidia-smi owns them); a 2 GB iGPU is dropped beside a discrete card; built-in-only machines keep the largest share and claim no bandwidth |
| `parseGPUUsage` | `internal/scan/gpu_discrete.go:127` | `testdata/locale/gpu_usage_de.txt`, `_ja.txt`, `_zh.txt` | localized "N/A" lines are ignored; a process's use is summed across `phys_0`/`phys_1`; a value printed with localized separators (`1.073.741.824`) is skipped, not misread |
| `splitCSV` + `nvidiaScan` field handling | `internal/scan/gpu_discrete.go:183`, `:21` | `testdata/locale/nvidia_smi_de.csv`, `_ja.csv`, `_zh.csv` | `[N/A]`, `Nicht zutreffend`, `該当なし` and `不适用` memory fields fail `ParseUint` and the row is skipped; the one valid row still parses; no zero-value GPU |
| `FreeAt` | `internal/scan/scan.go:280` | none (temp dir) | a missing `C:\Users\Müller 太郎 用户\EZLM\models`-style path walks up to its drive and returns free space |

The Windows-only file `locale_windows_test.go` runs the same cases on Windows
and pins the display class path. On other platforms `locale_other_test.go`
runs them, because the parsers under test are shared. `locale_cases_test.go`
holds the table and the fixture loader.

## Findings

1. **Localized DriverDesc depends on an English ProviderName** (minor,
   `internal/scan/gpu_discrete.go:208`, `:231`). `usableAdapters` builds the
   vendor from `DriverDesc + " " + ProviderName`. German, Japanese and
   Chinese DriverDescs keep the Latin model name (`Arc(TM) B580`,
   `RX 7900 XTX`), but the Intel brand itself is localized (`インテル`,
   `英特尔`); on those machines the vendor is found only because the
   registry `ProviderName` is `Intel Corporation`. If an OEM or a partially
   written driver entry leaves `ProviderName` empty or non-English, the card
   is dropped. All fixtures here use the English provider, so this is a risk,
   not a proven bug; a real Japanese/Chinese registry export is needed.
   Not fixed: adding localized vendor aliases would be a guess without one.

2. **Decimal commas do not reach the Windows parsers** (informational,
   `internal/scan/gpu_discrete.go:21`, `:127`). Every Windows value is an
   integer: `nvidia-smi --format=csv,noheader,nounits` prints integers, the
   WMI/PDH `DedicatedUsage` counters are `UInt64`, and the registry VRAM
   values are `QWORD`/`DWORD`. There is no `ParseFloat` in a Windows path
   (the two are macOS `memory_pressure`, `scan_darwin.go:22`,
   `quick.go:34`). A localized separator therefore makes a value be skipped
   (tested with `1.073.741.824`), never misread as a smaller number.

3. **Localized "N/A" is safe for nvidia-smi** (informational,
   `internal/scan/gpu_discrete.go:21-30`). nvidia-smi itself prints `[N/A]`
   in every locale, but a translated word also fails `ParseUint`, and
   `nvidiaScan` skips the whole row. The cost is an omitted GPU (or an
   omitted NPU row), not a zero-value GPU.

4. **Counter names are already locale-proof on the main path**
   (informational, `internal/scan/display_windows.go:142`, `:164`).
   `adapterMemoryUsed` uses `PdhAddEnglishCounterW`, which resolves English
   counter names on any Windows language. The PowerShell fallback in
   `windowsGPUUsage` (`gpu_discrete.go:112-125`) uses `Get-Counter` with the
   English path and is already documented as finding nothing on German or
   Japanese Windows; the WMI query before it is locale-independent.

5. **Non-ASCII profile paths are handled** (informational,
   `internal/scan/scan.go:280`). `FreeAt` walks a missing path up to an
   existing drive with `filepath` only; no code-page conversion is involved,
   and `paths.Home` uses `os.UserHomeDir` (`internal/paths/paths.go:9`).
   The test creates a real temp tree with German, Japanese and Chinese
   characters and checks the result is non-zero.

## What I could not finish

- No Windows machine or Wine here, so the registry/PowerShell entry points
  (`displayAdapters`, `readAdapter`, `adapterMemoryUsed`) could not be run
  against a live localized Windows. The tests compile for Windows
  (`GOOS=windows go test -c`) and exercise the shared parsers they feed.
- The fixtures are synthetic samples built from the documented formats, not
  dumps from real German/Japanese/Chinese machines. `SOURCES.md` says which
  parts come from which document.

## Open questions

- Does any real Japanese or Chinese display driver entry have a localized or
  missing `ProviderName`? A registry export from `SYSTEM\CurrentControlSet\
  Control\Class\{4d36e968-...}\0000` on such a machine would settle it.
- Does any localized nvidia-smi build print a translated `[N/A]`? nvidia-smi
  appears to hardcode `[N/A]`; the parser tolerates both.

## Next steps

- Add a real registry export from a Japanese or Chinese Windows machine to
  `testdata/locale/` and extend the fixture table.
- If the ProviderName risk is confirmed, add localized Intel/AMD brand
  aliases to `vendorOf` and cover them with a case here.
