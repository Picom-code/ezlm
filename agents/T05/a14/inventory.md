# T05 / a14: parser inventory for `internal/scan`

Snapshot: `agent/T05-a14`, base commit `5213370` ("Baseline: EZLM source snapshot 2026-11-06").
Coverage numbers below come from `go test -p 2 -cover ./internal/scan/` on darwin/arm64
(17.7% of statements). Files named `*_windows.go` are not compiled by that run, so their
coverage is reported as "n/a (windows-only)". A cross-compiled test binary cannot run on
macOS, so the only way to execute those paths is on Windows or with a seam (see §5).

## 0. Where the parsers actually live

`run/T05-brief.md` names `exec.go`, `nvidia.go`, `rocm.go`, `sysctl*.go` and `*_darwin.go`.
None of those files exist in this snapshot. The parsers are consolidated into:

| File | Parsers |
|---|---|
| `scan.go` | `Scan` (orchestrator), `osName`, `roomiestDrive`, `FreeAt`, `topMem` |
| `gpu_discrete.go` | `nvidiaScan`, `amdScan`, `splitCSV`, `at`, `parseGPUUsage`, `windowsGPUUsage`, `windowsGPUMem`, `usableAdapters`, `vendorOf`, `otherBandwidth`, `nvidiaBandwidth`, `cudaVersionRe` |
| `display_windows.go` | `displayScan`, `displayAdapters`, `readAdapter`, `adapterMemoryUsed`, `otherGPUMemory` |
| `scan_darwin.go` | `platformScan` (macOS), `appleBandwidth` |
| `scan_other.go` | `platformScan` (non-darwin dispatch) |
| `quick.go` | `QuickMemory` |
| `apps.go` | `MemoryApps`, `installRoot`, `friendlyName`, `describe`, `baseName`, `closableProc`, `systemPath` |
| `apps_windows.go` / `apps_unix.go` | `listProcs`, `gpuUsage`, `windowPIDs`, `fileDescription` |
| `exec_windows.go` / `exec_other.go` | `hide` only (not a parser) |

`apps_windows.go` has no explicit build tag; it is Windows-only through the `_windows.go`
filename suffix. Do not "fix" it by adding a tag, and do not assume it is cross-platform.

## 1. Dispatch order (what runs on which OS)

`Scan` (scan.go:87) calls `platformScan`:

- **darwin** (`scan_darwin.go:9`): no GPU probing command. `memory_pressure`,
  `sysctl -n machdep.cpu.brand_string`, `sysctl -n iogpu.wired_limit_mb`. Intel Mac
  (`h.Arch != "arm64"`) returns early with CPU backend.
- **windows/linux** (`scan_other.go:7`): `nvidiaScan` → `amdScan` → `displayScan` → CPU.
  Each returns `bool` "handled it"; the first `true` wins, and every earlier parser's
  failure is silent.

`QuickMemory` (quick.go:22) is a separate, cheaper path used by the live dashboard
(`internal/app/live.go:50`). It re-implements the `nvidia-smi` memory read and does **not**
share code with `nvidiaScan`.

## 2. Parser inventory

Legend for "On failure": what the caller sees when the input is malformed, absent, or
localized unexpectedly. "Silent" means no `Notes` entry, so the UI cannot explain it.

### 2.1 `nvidia-smi` family

| | |
|---|---|
| Entry point | `nvidiaScan` — gpu_discrete.go:14 |
| Input | stdout of `nvidia-smi --query-gpu=name,memory.total,memory.free,driver_version --format=csv,noheader,nounits`, one line per GPU, memory in MiB |
| Helpers | `splitCSV` (gpu_discrete.go:182), `at` (:175), `nvidiaBandwidth` (:268) |
| On failure | command error → `false`, silent. A line with <3 fields is skipped. A non-numeric or zero `memory.total` is skipped with no note (this is the `[N/A]` / NPU row case). If no GPU survives, returns `false` and the scan falls through to `amdScan`. A found-but-unreadable NVIDIA GPU is therefore reported as "No supported GPU found" (CPU), not as an NVIDIA problem. |
| Secondary input | full `nvidia-smi` header, matched by `cudaVersionRe` (gpu_discrete.go:144) for `CUDAMax`; failure leaves `CUDAMax` empty |
| Third input | `nvidia-smi --query-compute-apps=pid,process_name,used_memory --format=csv,noheader,nounits`; `pid <= 0` skipped, unparseable `used_memory` becomes 0, `process_name` with a comma is not handled (CSV split is naive) |
| Windows extra | `windowsGPUMem` (gpu_discrete.go:70) replaces `[N/A]` per-process VRAM with counters, drops entries above total VRAM, keeps only ≥64 MiB |
| Coverage | **none.** `nvidiaScan`, `splitCSV`, `at`, `nvidiaBandwidth`, `windowsGPUMem` are all 0%. No fixture, no table test. |
| Expected corpus owner | a01 (CSV, N/A, MIG, multi-GPU), a02 (driver drift, no-memory driver) |

### 2.2 `rocm-smi` family

| | |
|---|---|
| Entry point | `amdScan` — gpu_discrete.go:147 |
| Input | stdout of `rocm-smi --showmeminfo vram --showproductname --json`, object keyed by `cardN`, values keyed `"VRAM Total Memory (B)"`, `"VRAM Total Used Memory (B)"`, `"Card Series"` (or `"Card series"`) |
| On failure | command error → `false`, silent. `json.Unmarshal` into `map[string]map[string]string` fails → `false`, silent (so a ROCm version that adds a nested object or changes a value type to a number is read as "no AMD GPU"). Unparseable or zero total → card skipped. `used` parse error → 0. |
| Bug candidate | `MemFree: total - used` (gpu_discrete.go:166) is unsigned arithmetic. If `used > total` (APU/GTT accounting, a counter reset, or a driver that reports bytes vs KiB inconsistently) `MemFree` wraps to ~1.8e19. The planner and the dashboard both read `MemFree`. Code reading only: `amdScan` calls `run` directly, so this cannot be proved without a seam. Flagged for a03/a15. |
| Coverage | **none.** `amdScan` 0%. |
| Expected corpus owner | a03 |

### 2.3 Windows display class (AMD/Intel registry)

| | |
|---|---|
| Entry point | `displayScan` — display_windows.go:23 → `displayAdapters` (:49) → `readAdapter` (:75) |
| Input | (a) PowerShell `Get-CimInstance Win32_VideoController \| ForEach-Object { $_.PNPDeviceID }`, one PNP device id per line; (b) registry `HKLM\SYSTEM\CurrentControlSet\Enum\<id>` value `Driver` (`{class}\0001`); (c) registry `HKLM\SYSTEM\CurrentControlSet\Control\Class\{4d36e968-...}\<NNNN>` values `DriverDesc`, `ProviderName`, `HardwareInformation.qwMemorySize` (QWORD bytes), `HardwareInformation.MemorySize` (binary 4 bytes LE, or integer) |
| On failure | PowerShell error → `nil`, silent. An unreadable `Enum` key or missing `Driver` value skips that card silently. `readAdapter` returns `ok=false` when `DriverDesc` is empty, even if VRAM was read. A card with no memory value keeps `VRAM=0` and is dropped by `usableAdapters` unless it is built-in graphics with ≥8 GB. `displayScan` returns `false` when the usable list is empty. |
| Coverage | `usableAdapters` 92.9% and `vendorOf` 100% (display_test.go). `displayAdapters`/`readAdapter`/`adapterMemoryUsed` are covered only by `display_windows_test.go`, a host probe that **logs** and asserts only when `EZLM_EXPECT_GPU` is set. No registry-value fixtures. |
| Expected corpus owner | a04 (WMI/ghost cards), a05 (`qwMemorySize`, 4 GB clamp, overflow), a06 (localized names) |

| | |
|---|---|
| Entry point | `adapterMemoryUsed` — display_windows.go:152 |
| Input | PDH English counter `\GPU Adapter Memory(*)\Dedicated Usage`, `PdhGetFormattedCounterArrayW` buffer |
| On failure | any PDH step fails → `nil`; callers treat `nil` as "no data", so free VRAM equals total VRAM. |
| Coverage | none beyond the Windows probe test. The struct layout at :179 is hand-rolled `unsafe` and is the highest-risk unparsed binary layout in the package. |
| Expected corpus owner | a04/a10 |

| | |
|---|---|
| Entry point | `otherGPUMemory` — display_windows.go:113 (Windows), gpumem_other.go:7 (else, returns 0,0) |
| Input | in-memory `cardSize` plus `adapterMemoryUsed` |
| On failure | returns `0,0`; `QuickMemory` then reports zero GPU memory for AMD/Intel Windows machines. |
| Coverage | 0%. |

### 2.4 macOS

| | |
|---|---|
| Entry point | `platformScan` — scan_darwin.go:9 |
| Input | `memory_pressure` line `... free percentage: NN%`; `sysctl -n machdep.cpu.brand_string`; `sysctl -n iogpu.wired_limit_mb` (decimal MB) |
| On failure | any command error is ignored. `free percentage:` absent → `RAMAvail` keeps the gopsutil value. `brand_string` empty → gopsutil CPU name kept. `iogpu.wired_limit_mb` missing or unparsable → the 0.67/0.75 RAM fraction is used. Intel Mac → CPU backend plus a note. |
| Coverage | **none.** `platformScan` 0%, `appleBandwidth` 0%. |
| Expected corpus owner | a07 |

| | |
|---|---|
| Entry point | `appleBandwidth` — scan_darwin.go:56 |
| Input | chip string, e.g. `Apple M4 Pro` |
| On failure | unknown generation (M6, an unlabelled string) → **100 GB/s silently**. Generation and tier are substring matches, so any future name containing `m1`..`m5`/`pro`/`max`/`ultra` is classified. |
| Coverage | **none.** |

### 2.5 CPU / RAM / OS / disk (gopsutil, not text parsing)

| | |
|---|---|
| Entry point | `osName` — scan.go:178 |
| Input | `host.Info()` `Platform` + `PlatformVersion` |
| On failure | error → returns `runtime.GOOS`. Windows strips a leading `Microsoft `. Linux upper-cases the first byte of `Platform` (`name[:1]`), which corrupts a non-ASCII platform name. |
| Coverage | **none.** |
| Expected corpus owner | a06 (localized Windows) / a09 (CPU-only) |

| | |
|---|---|
| Entry points | `Scan` scan.go:87 (RAM/CPU/disk), `roomiestDrive` :156, `FreeAt` :280, `topMem` :208, `hasVendor` :198 |
| Input | gopsutil `mem`/`cpu`/`disk`/`process`, `paths` settings |
| On failure | every error is swallowed and the field keeps its zero value; `Scan` never fails. `Scan` itself 0% covered. |
| Expected corpus owner | a09, a10 |

### 2.6 Process/app text parsers (`apps.go`)

| Parser | Where | Input | On failure | Coverage |
|---|---|---|---|---|
| `installRoot` | apps.go:254 | exe path (Windows or POSIX), env lookup | falls back to the exe's own folder; a bare name returns itself | 86.2% |
| `friendlyName` | apps.go:331 | exe path | `fileDescription` (Windows) → `.app` bundle name → title-cased base name → `"Unknown app"` | 56.2% |
| `describe` | apps.go:357 | `App` (exe base, window flag) | default note | 30.8% |
| `baseName` | apps.go:200 | exe path | "" | 100% |
| `closableProc` | apps.go:183 | `proc` | any rule failure → not closable | 90% |
| `systemPath` | apps.go:234 | exe path | Windows branch uses `SystemRoot`, default `c:\windows` | 50% (the Windows branch is not executed on darwin) |
| `protectedName` | apps.go:231 | base name | — | 100% |
| `fileDescription` | apps_windows.go:147 | PE version resource (binary) | any malformed offset/length → "" | **0%, windows-only** |
| `hex4` | apps_windows.go:199 | uint16 | — | **0%, windows-only** |
| `listProcs` | apps_unix.go:17 / apps_windows.go:41 | gopsutil process table | errors drop the process | 0% |
| `windowPIDs` | apps_unix.go:50 / apps_windows.go:107 | `.app` path or `EnumWindows` | empty map (nothing is closable gently) | 0% |
| `gpuUsage` | apps_windows.go:75 | `windowsGPUUsage` | nil map | 0% |

These are covered by `apps_test.go` (4 tests) for the pure helpers only. `MemoryApps`,
`CloseApp`, `listProcs`, `windowPIDs` have no test at all.

### 2.7 Pure parsers with existing tests

| Parser | Test | What the test pins |
|---|---|---|
| `parseGPUUsage` (gpu_discrete.go:127) | `TestParseGPUUsage` (gpu_usage_test.go) | per-PID sum across cards, >1 TiB garbage dropped, `_Total` ignored, empty input |
| `cudaVersionRe` (gpu_discrete.go:144) | `TestCUDAVersionHeader` (cuda_test.go) | old `CUDA Version:` and new `CUDA UMD Version:` headers |
| `usableAdapters` (gpu_discrete.go:203) | `TestUsableAdapters` (display_test.go) | discrete vs built-in vs virtual vs NVIDIA, ordering, two-card rig |
| `otherBandwidth` (gpu_discrete.go:246) | `TestOtherBandwidth` (display_test.go) | longest-key-wins, unknown → 300 |
| `installRoot`, `closableProc`, `describe`, `friendlyName` | apps_test.go | grouping, safety rules, labels |

## 3. Parsers with no coverage (the point of this document)

Ordered by how much damage a misparse does. "Corpus" is the a01–a10 agent expected to
cover it; "seam" says what must be added first.

| # | Parser | Risk of misparse | Corpus | Needs |
|---|---|---|---|---|
| 1 | `nvidiaScan` CSV loop + `splitCSV`/`at` | silent CPU fallback on a real NVIDIA box; `[N/A]` and MIG rows already drop out | a01, a02 | extract `parseNvidiaCSV(string) []GPU` (precedent: `parseGPUUsage`) or a `run` seam |
| 2 | `amdScan` JSON | silent CPU fallback when ROCm renames a key; `MemFree` underflow | a03 | same seam |
| 3 | `readAdapter` registry fallback chain | 4 GB clamp, wrong `qwMemorySize` units, ghost cards | a05 | a registry-reader seam or a pure `vramFromValues(qw uint64, binary []byte, integer uint64)` helper |
| 4 | `adapterMemoryUsed` PDH layout | wrong VRAM-in-use; `unsafe` struct | a04 | Windows-only, or a fake buffer decoder |
| 5 | `appleBandwidth` | wrong bandwidth → wrong speed estimate; unknown chip → 100 | a07 | pure function, trivial to table-test today |
| 6 | `platformScan` (darwin) | `RAMAvail`/`GPUUsable` wrong on new macOS | a07 | pure `parseFreePercentage(string)` / `parseWiredLimit(string)` |
| 7 | `osName` | mislabelled OS on localized Windows | a06 | pure function over `(platform, version)` |
| 8 | `QuickMemory` nvidia sum | dashboard shows 0 or doubled VRAM | a01, a10 | same seam as #1 |
| 9 | `windowsGPUUsage` PowerShell fallback | per-app VRAM missing on localized Windows (documented in the comment at gpu_discrete.go:107) | a12 | seam |
| 10 | `fileDescription` | app names fall back to exe names | a04, a12 | Windows-only; low value |
| 11 | `nvidiaBandwidth` | wrong speed estimate only | a01 | pure function, table test |
| 12 | `windowsGPUMem` filtering | over/under-counted per-app VRAM | a10 | seam |
| 13 | `otherGPUMemory` | zero GPU memory on AMD/Intel Windows dashboard | a04 | seam |

Not parsers, but also at 0%: `Scan`, `MemoryApps`, `CloseApp`, `Kill`, `topMem`,
`roomiestDrive`, `FreeAt`, `listProcs`, `windowPIDs`. They touch the host, so they need
fakes rather than fixtures.

## 4. Fixtures present in this worktree

**None.** There is no `internal/scan/testdata/` at this commit. a01–a10 and a13 add
fixtures and the loader on their own branches; this inventory therefore records coverage
as of the baseline plus the eight existing test functions. When the corpus lands, §2 and §3
should be regenerated against it (the tables are the checklist).

## 5. Structural finding: there is no seam for `run`

`run` (scan.go:250) is a plain function; nothing in the package can substitute it. Every
command-output parser (`nvidiaScan`, `amdScan`, `windowsGPUUsage`, `platformScan`,
`QuickMemory`, `displayAdapters`) calls it directly, which is why all of them sit at 0%.
`parseGPUUsage` shows the house style for fixing this: keep the acquisition thin and put
the parsing in a pure function. Applying that to the CSV, JSON and sysctl readers is the
single change that unlocks the whole corpus. It must be coordinated: a01, a03, a07 and a11
will otherwise each add a different seam and conflict on `internal/scan/*.go`.

## 6. Open questions

1. Should `nvidiaScan` emit a `Note` when `nvidia-smi` succeeds but no GPU row parses
   (currently indistinguishable from "no NVIDIA GPU")? Same for `amdScan` on a JSON error.
2. Is `used > total` reachable on a real ROCm APU? If yes, `MemFree` should be clamped
   (`if used > total { used = total }`). a03 owns the fixture.
3. `readAdapter` prefers `qwMemorySize` over the binary value. On drivers that write
   `qwMemorySize` in MB instead of bytes, every card is ~1/1MiB of its real size and is
   dropped by `usableAdapters`. a05 owns the evidence.
4. Should `appleBandwidth`'s unknown-chip fallback be lower than 100 GB/s? It feeds the
   speed estimate, so a too-high value over-promises tokens/s on a future chip.
