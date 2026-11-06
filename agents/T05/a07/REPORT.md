# T05 / a07: macOS `sysctl` on M1 to M5 and Intel

## What I did

- Built a 12-file corpus under `internal/scan/testdata/` covering Apple M1,
  M1 Max, M2, M2 Pro, M3 Pro, M4 Pro, M5, a synthetic M5 Max, an Intel Mac,
  `memory_pressure` on two machines, and a Rosetta `sysctl.proc_translated`
  capture. `agents/T05/a07/SOURCES.md` has provenance for each one.
- Added `internal/scan/sysctl_apple_test.go` (darwin-only, one file for this
  slice): a per-machine table test that parses each `sysctl -a` excerpt and
  asserts RAM, chip name, core split, bandwidth, Apple-Silicon detection and
  GPU wired-memory limit; plus table tests for the four parse helpers and one
  host smoke test for the `platformScan` wiring.
- Fixed three real bugs and one defensive gap in `internal/scan/scan_darwin.go`.

Run: `go test -p 2 -run 'TestApple|TestParseMemoryPressure|TestSysctlUint|TestPlatformScan' ./internal/scan/`.

## Findings

### 1. 36 GB Macs get 67% of RAM as GPU memory instead of 75% (major, fixed)

`internal/scan/scan_darwin.go:40` (before the change) used
`if h.RAMTotal > 36*GB` for the larger 75% share. Apple's documented split is
67% *below* 36 GB and 75% *at 36 GB and above*, so a 36 GB machine (M2 Max,
M3 Max, M4 Max in 36 GB trim) was told it could wire 25.9 GB instead of 27 GB.
That undercount is what the planner uses to decide whether a model fits.

Evidence: `TestAppleGPUUsable` failed before the fix with
`appleGPUUsable(38654705664, "") = 25898652794, want 28991029248`; after
changing `>` to `>=` it passes. The 36 GB row is in the test on purpose.

### 2. An amd64 build under Rosetta is reported as an Intel Mac (major, fixed)

`platformScan` branched on `h.Arch != "arm64"`, and `Arch` is
`runtime.GOARCH`. The Mac app ships a universal binary
(`macos/scripts/build-app.sh:29-30`), so the amd64 slice running on an M-series
Mac under Rosetta (or `arch -x86_64`) took the Intel path: `Backend = CPU`,
`Unified = false`, no GPU entry, `GPUUsable = 0`, and the note "Intel Mac
detected". The rest of the app already knows better: `integrate.MachineArch()`
(`internal/integrate/arch_darwin.go:12`) checks `sysctl.proc_translated`.

Evidence, captured on this M3 Pro on 2026-11-06:
`arch -x86_64 sysctl -n sysctl.proc_translated` prints `1` while the native
command prints `0`; the brand string is still `Apple M3 Pro`. Fixture:
`testdata/sysctl_proc_translated_rosetta.txt`, covered by
`TestAppleSiliconUnderRosetta`.

Fix: `appleSilicon(arch, procTranslated)` (`scan_darwin.go:67`) treats the
machine as Apple Silicon when the arch is arm64 or when the kernel says the
process is translated. `platformScan` now reads
`sysctl -n sysctl.proc_translated` (`scan_darwin.go:25`).

### 3. Intel Macs skipped the `memory_pressure` estimate (major, fixed)

The `Arch != "arm64"` early return sat above the `memory_pressure` probe, so
Intel Macs kept gopsutil's `Available`, which excludes file cache and
compressed pages. On an Intel Mac with 16 GB and a few GB of cache the engine
sees a much smaller `RAMAvail` than macOS itself would, which pushes the
planner towards a smaller model. The probe is now before the architecture
check (`scan_darwin.go:21`), and its parse moved into `parseMemoryPressure`
(`scan_darwin.go:83`) so it can be tested without running the binary.

Note: the probe cost is one `memory_pressure` call, which the Apple Silicon
path already paid.

### 4. `hw.memsize` was never read (minor, fixed defensively)

`RAMTotal` came only from gopsutil. If that call fails (restricted sandbox,
partial `/proc`-style environment) the engine would think the machine has no
RAM, and every derived number (GPU usable, model fit) collapses to zero.
`platformScan` now falls back to `sysctl -n hw.memsize`
(`scan_darwin.go:10-16`), which is the same key gopsutil reads.

### 5. `QuickMemory` still has the Rosetta bug and a duplicated parse (minor, open)

`internal/scan/quick.go:29` checks `runtime.GOARCH == "arm64"` and
`quick.go:31-38` repeats the `memory_pressure` parse inline. Under Rosetta the
dashboard therefore reports `Unified = false` and falls through to
`nvidia-smi`, so the live memory panel shows no GPU memory on an M-series Mac.
Not fixed here to keep this slice's diff to `scan_darwin.go`; the fix is to
call `appleSilicon` and `parseMemoryPressure` from `quick.go`, which means
moving those two helpers out of the darwin-only file (or adding a darwin
`quickMemory` hook). Next step for the owner of `quick.go`.

### 6. Empty brand string makes a GPU called " GPU" (minor, open)

`scan_darwin.go:42` builds the GPU name as `chip + " GPU"`. If
`machdep.cpu.brand_string` is empty (some macOS VMs, or a stripped
environment) the GPU is named `" GPU"` and the bandwidth falls back to 100.
No fixture proves it yet because I did not want to invent a Mac without the
key; a fallback to `sysctl -n hw.model` would be the fix.

## Coverage

| Parser | Input | Test |
|---|---|---|
| `platformScan` (darwin) | `sysctl -n`, `memory_pressure` | `TestPlatformScanAppleWiring` (host smoke), helpers below |
| `appleSilicon` | `sysctl.proc_translated` | `TestAppleSiliconUnderRosetta`, per-machine table |
| `appleGPUUsable` | `hw.memsize`, `iogpu.wired_limit_mb` | `TestAppleGPUUsable`, per-machine table |
| `appleBandwidth` | `machdep.cpu.brand_string` | per-machine table (M1 to M5, Intel) |
| `parseMemoryPressure` | `memory_pressure` | `TestParseMemoryPressure` |
| `sysctlUint` | `sysctl -n` output | `TestSysctlUint` |

Not covered: gopsutil's own darwin memory/CPU reads (outside this package),
and a real 36 GB machine (the 36 GB case is a table row, not a capture).

## What I could not finish

- No M5 hardware to capture from. The M5 and M5 Max rows are synthetic from
  Apple's published core counts; the bandwidth table's M5 tiers are asserted
  as the parser encodes them, not re-verified against a machine.
- The Rosetta fix is proven at the helper and kernel level, not end to end: I
  cannot run the amd64 slice of the engine on this Mac without building it, and
  the test asserts the detection function, not a full `Scan`.

## Next steps

1. Fix `quick.go` (finding 5) and delete the duplicated parse.
2. Fall back to `hw.model` when the brand string is empty (finding 6).
3. Capture a real 36 GB M3 Max or M4 Max `sysctl -a` and `memory_pressure`
   pair to replace the table-only case.
