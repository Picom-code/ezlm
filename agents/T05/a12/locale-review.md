# T05/a12 locale review of the Windows scan code

Scope: every `*_windows.go` in `internal/scan` (`display_windows.go`,
`apps_windows.go`, `exec_windows.go`), the Windows-only parts of the shared
parsers (`gpu_discrete.go`), and the Windows C# call sites under
`windows/EZLM` that read scan results. Method: read the code, then probe the
pure parsers with inputs as PowerShell, .NET and WMI actually format them for
German, French, Japanese and Turkish locales. Two real parser bugs were found,
fixed and covered by `internal/scan/locale_review_test.go`.

Severity: **blocker** = wrong hardware decision on a common machine,
**major** = wrong numbers or a feature silently off on a non-English Windows,
**minor** = cosmetic or only reachable in a rare configuration,
**info** = checked, no action.

## Findings

### 1. major (fixed, tested): locale-formatted byte counts in the GPU counter fallback

`internal/scan/gpu_discrete.go:117` asks PowerShell for
`(Get-Counter '\GPU Process Memory(*)\Dedicated Usage')... "$($_.InstanceName) $([int64]$_.CookedValue)"`
and `parseGPUUsage` (`gpu_discrete.go:127`) read the value with
`strconv.ParseUint(f[1], 10, 64)` after `strings.Fields`.

PowerShell formats interpolated values for the current user locale. Byte
counts come out with group separators: `1.073.741.824` on German Windows,
`1 073 741 824` (U+202F or U+00A0) on French Windows, and with a decimal
comma if a driver reports a fractional value. `strings.Fields` splits the
no-break-space forms into several fields, and the old `len(f) != 2` guard
dropped the line; the dotted and comma forms failed `ParseUint`. Every case
became a **silent zero**: the process showed no graphics memory instead of
the real number, so the planner believed the card was idle.

Fixed in `parseGPUUsage` and the new `localeInt` helper (`gpu_discrete.go:154`):
the value fields are joined, group separators are removed, and a decimal
comma is truncated (byte counts are whole). The English fast path is
unchanged. Proven by `TestParseGPUUsageLocaleNumbers`,
`TestParseGPUUsageLocaleSums` and `TestParseGPUUsageUnreadableValue`: the
first two fail on the old code (`got 0 bytes`) and pass now.

### 2. major (not fixed, Windows-only): localized counter names in the same fallback

`internal/scan/gpu_discrete.go:117` uses `Get-Counter '\GPU Process Memory(*)\Dedicated Usage'`.
Unlike `PdhAddEnglishCounterW` (used correctly in `display_windows.go:142`
and `display_windows.go:162`), `Get-Counter` resolves counter-set and
counter names in the **display language**: German Windows knows
"GPU-Prozessspeicher", Japanese Windows knows the Japanese names. On those
systems the fallback returns nothing at all. `parseGPUUsage` then returns an
empty, non-nil map, `windowsGPUMem` (`gpu_discrete.go:70`) treats that as
"no process uses the GPU", and `displayScan` (`display_windows.go:32-41`)
sets `MemFree = MemTotal`. On a German or Japanese PC where the primary WMI
query (`gpu_discrete.go:114`, English class and property names, locale-safe)
also fails, an AMD or Intel card is reported as completely free while games
hold most of it.

The code comment at `gpu_discrete.go:108-111` already states the limitation.
The cheap correct fix is to read `\GPU Process Memory(*)\Dedicated Usage`
with `PdhAddEnglishCounterW` from Go, like `adapterMemoryUsed` does, instead
of going through `Get-Counter`. That is ~40 lines of `unsafe` and cannot be
exercised on this Mac, so it is left as an open question (see below).

### 3. minor (fixed, tested): CSV fields quoted because they contain a comma

`splitCSV` (`gpu_discrete.go:215`) split on every comma. nvidia-smi quotes
fields that contain a comma, which on Windows is common for
`--query-compute-apps=process_name` (`C:\Program Files\Foo, Inc\app.exe`).
The quoted path was split in two, shifting every later field: the PID was
still read, but `used_memory` landed in the wrong field and parsed as zero.
Fixed by honouring double quotes and `""` escapes; plain lines are unchanged.
Proven by `TestSplitCSVQuotedComma` (fails before: 4 fields instead of 3).
Related to the locale theme because it is the same "comma means two things"
class of bug; found while reviewing the CSV path.

### 4. minor: PowerShell stdout is decoded as UTF-8, Windows PowerShell writes the OEM code page

`run` (`scan.go:250`) returns `cmd.Output()` bytes as a Go string. Windows
PowerShell 5.1 writes redirected stdout in the console output code page
(CP850 in Germany, CP932 in Japan, CP936 in China), not UTF-8. All three
current queries (`display_windows.go:51`, `gpu_discrete.go:114`,
`gpu_discrete.go:117`) emit ASCII only, so nothing breaks today. The moment
a localized name is added to one of them (a `DriverDesc`, a process name),
it arrives as mojibake. Registry strings (`DriverDesc`, `ProviderName`) are
read as UTF-16 via `GetStringValue` and are unaffected. No action now; worth
a note when the queries grow.

### 5. minor: version-resource language fallback is English-only

`apps_windows.go:173` falls back to `040904b0`, `040904e4`, `000004b0`
(en-US, language-neutral) when the `\VarFileInfo\Translation` block cannot
be read. A program whose version resource has only, say, a German
translation then shows no name (`""`) and the app list falls back to the exe
name. Cosmetic, and the first translation block is used whenever it exists.

### 6. info: localized `DriverDesc` does not break card detection

Vendors ship localized INF strings, so `DriverDesc` can be
"Intel(R) Arc(TM) B580 グラフィックス". `vendorOf` (`gpu_discrete.go:278`)
is called with `a.Name + " " + a.Provider` (`gpu_discrete.go:253`), and
`ProviderName` ("Intel Corporation", "Advanced Micro Devices, Inc.") is not
translated, so the vendor survives. `discreteRe` matches model numbers
(`B580`, `RX 7800 XT`), which are not translated either. Covered by
`TestUsableAdaptersLocalizedNames`.

### 7. info: Go case folding is locale-independent, so no Turkish-I bug

`strings.ToLower`/`ToUpper` in `gpu_discrete.go:279`, `scan.go:161` and
`apps.go` use Go's locale-independent mapping, unlike C# `ToLower()` under
tr-TR (where "INTEL" becomes "ıntel"). Covered by
`TestVendorOfCaseFolding`. This is worth remembering because the C# side
must keep using the `*Invariant` variants.

### 8. info: the C# call sites are clean

- `Models.cs:11-34` mirrors the engine JSON with `ulong`/`double`; parsing
  goes through `System.Text.Json` with `Engine.cs:35-39`
  (`AllowReadingFromString`), which parses numbers invariantly. A German
  engine or app locale cannot change the value.
- Every case conversion is invariant: `Ui.cs:459,480,500,585`,
  `Chat.cs:754`, `Memory.cs:86,162`, `BigType.cs:44`.
- Comparisons use `StringComparison.OrdinalIgnoreCase`
  (`SettingsPane.cs:140`, `Chat.cs:1042`, `Shell.cs` paths).
- `Fmt.Gb` (`Models.cs:322`) formats with the current culture on purpose:
  a German user should see "5,0 GB". Display only, never parsed back.
- `Shell.cs:206` logs the scan with the same JSON options.
No locale assumptions to fix on the C# side.

### 9. info: nvidia-smi output is not localized

nvidia-smi prints `[N/A]` in English and uses `.` as the decimal point
regardless of the Windows display language. `nvidiaScan`
(`gpu_discrete.go:24-27`) already turns an unparseable field into a skipped
row, which is the right handling for both `N/A` and `[N/A]`. `QuickMemory`
(`quick.go:49`) has the same shape. No action.

## What changed

- `internal/scan/gpu_discrete.go`: `parseGPUUsage` accepts the value across
  several whitespace-separated fields and falls back to `localeInt`;
  `localeInt` normalizes locale group separators and decimal commas;
  `splitCSV` honours quoted fields.
- `internal/scan/locale_review_test.go`: table tests for the cases above.

Verified on the worktree: `go test -p 2 ./internal/scan/` passes, and the
new tests fail against the old `gpu_discrete.go` (checked by reverting the
file and rerunning, then reapplying).

## Open questions

1. Replace the `Get-Counter` fallback (finding 2) with `PdhAddEnglishCounterW`
   so per-process GPU memory also works on German and Japanese Windows.
   Needs a Windows machine to verify; the primary WMI path masks it today.
2. Should an unreadable counter reading mean "unknown" rather than "zero"?
   `displayScan` currently reports `MemFree = MemTotal` when the usage map is
   empty, which is optimistic. A `Hardware` note would tell the user why the
   free-memory figure is missing.
3. If a PowerShell query ever returns localized text, decode it with the
   console code page (finding 4) or force UTF-8 with
   `[Console]::OutputEncoding = [Text.Encoding]::UTF8`.
