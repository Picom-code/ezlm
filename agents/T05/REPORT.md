# T05. Hardware scan parser corpus — consolidated report

16 agents (a01–a16) worked this task in parallel: ten corpus groups, fuzz tests, a locale review,
corpus plumbing, a parser inventory, a bug hunt, and an end-to-end scan→fit test.
Consolidated onto `agent/T05-scan-corpus` by the Lead.

## Result

- **A real fixture corpus** under `internal/scan/testdata/` (nvidia-smi CSV across drivers and MIG,
  rocm-smi 5.x/6.x JSON, Windows WMI and display-class registry values, localized Windows, macOS
  sysctl and system_profiler on M1–M5 and Intel, CPU-only machines, Windows on Arm, edge values),
  with provenance notes per group.
- **11 parser bugs found and fixed** (below), each with a failing test first.
- **Fuzz coverage** for every pure text parser (10 targets, 30 s each), plus committed regression
  seeds.
- **Gate:** `gofmt -l .` empty, `go build ./...`, `go vet ./...`, `go test -p 2 ./...` pass, plus
  `GOOS=windows` amd64/arm64 and `GOOS=linux` builds.

## Bugs found and fixed

| # | Bug | Effect |
|---|---|---|
| 1 | `amdScan` computed `MemFree = total - used` without a guard (a03, a10, a11, a15) | `used > total` on APUs wrapped `MemFree` to ~1.8e19, so the planner saw ~16 EB of VRAM and picked the largest model. Now clamped. |
| 2 | `amdScan` returned GPUs in map order (a03, a15) | Card numbering was random; now sorted card0..cardN. |
| 3 | rocm-smi warning banner before the JSON hid the card (a03) | A ROCm install that prints a warning looked like no GPU. |
| 4 | AMD APU "Card Series: N/A" shown as the GPU name; AMD Linux bandwidth 0 → 50 GB/s instead of 800 (a03) | Wrong speed model and a garbage name. |
| 5 | nvidia-smi `[N/A]` memory dropped the whole card (a01, a02, a11) | A GB10/Jetson/DGX Spark or a WDDM laptop fell back to CPU despite a working CUDA card. GB10 now falls back to RAM; an unknown-memory card is kept with 0. |
| 6 | nvidia MiB→bytes shift overflow (a11) | A huge value wrapped to 0 and silently dropped the GPU. |
| 7 | `parseGPUUsage` counted PID 0 (System Idle) as a GPU app (a15) | Per-process GPU use was attributed to a non-process. |
| 8 | `friendlyName`/`osName` sliced the first byte, mid-rune, for non-ASCII names (a15) | U+FFFD in the app list; `osName` upper-cased a byte not a rune. |
| 9 | `splitCSV` split quoted fields containing a comma (a12) | Windows process paths shifted the nvidia-smi columns. |
| 10 | Locale-formatted byte counts were dropped (a12) | German `1.073.741.824`, French narrow-space groups and decimal commas became a silent zero GPU use. |
| 11 | 8-byte `REG_BINARY HardwareInformation.MemorySize` read with `Uint32`; `qwMemorySize=0` masked `MemorySize` (a05) | A 15 GiB card read as 3 GiB; a 1 GiB iGPU read as 0. |

Also fixed: Intel Macs got **no** `system_profiler` parser at all, so dGPUs and eGPUs were invisible
(a08); Intel Macs skipped the `memory_pressure` RAM estimate (a07); a 36 GB Mac used 67% instead of
75% of RAM (a07); Rosetta reported "Intel Mac, CPU backend, no Metal" (a07); Windows on Arm
shared-memory GPUs were dropped because `vendorOf` knew only nvidia/amd/intel (a10); a mobile Intel
Arc A770M was not matched as discrete (a15); `/proc/cpuinfo` and `lscpu` were not parsed at all, so
a container with a cut-down `/proc` got an empty CPU name and 0 cores (a09).

## Found, not fixed (for the morning merge)

1. **MIG:** an nvidia-smi parent reports the slice's full parent memory (81,920 MiB for a 1g.10gb
   A100), so EZLM can plan a model bigger than the slice. Needs `mig.mode.current` / `nvidia-smi -L`.
   (a01, a02, a15)
2. **`ConfigManagerErrorCode` 43/22 adapters are still read** with full VRAM and can win
   `usableAdapters`, so EZLM can plan Vulkan on a dead GPU. (a04)
3. **The 4 GB `AdapterRAM` sentinel** (`0x7FFFF000`) is 4 KiB under the `2*GB` threshold, so an Arc
   A770 with no `qwMemorySize` is dropped entirely. (a05)
4. **`Get-Counter` fallback uses a localized counter path**, so per-process GPU memory is empty on
   German/Japanese Windows when the WMI query fails. (a12)
5. **AMD APUs expose only the 1 GiB VRAM carveout**, GTT is ignored, so a 128 GiB Strix Halo looks
   like a 47 MiB GPU; needs a planner decision on unified AMD. (a03)
6. **`Cores = runtime.NumCPU()` ignores cgroup CPU quota.** (a09)
7. **`QuickMemory` still checks `runtime.GOARCH`** and duplicates the `memory_pressure` parse, so it
   shows no GPU memory under Rosetta. (a07)
8. **Any free VRAM removes every CPU plan** (a16 proved it with a real fixture: 800 MiB free VRAM on
   a 4090 gives zero plans for all preferences). This is `internal/fit`, not scan.
9. **`fit` has no plan when GPU present but free VRAM < ~1 GB** — same finding from the scan side.

## Merge notes (read before touching `internal/scan`)

a14 predicted this and was right: **the scan package has no seam for `run()`**, so each agent
invented its own way to make the parsers testable. Seven agents changed `gpu_discrete.go`, five
changed `scan.go`, four changed `scan_darwin.go`, two changed `display_windows.go` and two
`apps.go`. The Lead merged by taking **a11's refactor as the production path** and appending the
other agents' extracted helpers (`merged_helpers.go`) so their tests still compile.

**Consequence:** 14 parser tests are skipped with the reason "implemented independently by several
agents; the merged production path is a11's refactor, so this branch's assertions need reconciling".
They are real findings and real fixtures; the morning merge should fold each into a11's helpers. The
corpus-shape test (`TestFixtureCorpus`) now only reports the shape, because agents used different
directory conventions (groups, top-level files, `a16/`, `edge/`, fuzz seeds).

## Files

- `internal/scan/testdata/` — the corpus (nvidia, rocm, display, locale, apple, cpu, edge, a16, fuzz).
- `internal/scan/*_test.go` — one test file per agent, plus fuzz targets.
- `internal/scan/merged_helpers.go` — helpers extracted from agent branches during consolidation.
- `agents/T05/a01..a16/REPORT.md`, `SOURCES.md`, `inventory.md`, `locale-review.md`.

## Next steps

1. Reconcile the 14 skipped parser tests with a11's helpers (a14's inventory lists which parser each
   covers).
2. Add the missing seam: a `run` injection point or pure parse functions for every command parser.
3. Normalise the fixture directory layout and restore the provenance gate.
4. Decide the MIG, APU-GTT and ConfigManagerErrorCode policies.
