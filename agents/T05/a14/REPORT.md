# T05 / a14: parser inventory

**Agent:** a14, cross-cutting (parser inventory).
**Branch:** `agent/T05-a14`, worktree `~/ezlm-agents/work/subs/T05/a14`.
**Base:** `5213370` "Baseline: EZLM source snapshot 2026-11-06".
**Deliverable:** `agents/T05/a14/inventory.md` (this report is the companion).

## What I did

1. Read all of `internal/scan` (18 files, 1880 lines) and every test in the package.
2. Ran `go test -p 2 -cover ./internal/scan/` on darwin/arm64 to get per-function coverage
   instead of guessing which parsers are exercised: 17.7% of statements, 8 test functions.
3. Wrote `agents/T05/a14/inventory.md`: dispatch order, every parser entry point with
   `file:line`, its exact input format, its failure behaviour, the tests/fixtures that cover
   it, and a risk-ordered list of the parsers with no coverage, each mapped to the a01–a10
   agent expected to cover it.
4. Verified the two arithmetic/string claims below with a throwaway program (not committed).

No Go code was changed. Nothing outside `agents/T05/a14/` was touched.

## Findings

### F1. No seam for `run`, so every command-output parser is at 0% coverage (major)

`run` is a plain function at `internal/scan/scan.go:250`. `nvidiaScan`
(`gpu_discrete.go:14`), `amdScan` (`gpu_discrete.go:147`), `windowsGPUUsage`
(`gpu_discrete.go:112`), `platformScan` (`scan_darwin.go:9`), `QuickMemory`
(`quick.go:22`) and `displayAdapters` (`display_windows.go:49`) all call it directly, and
none of them can be fed a fixture. Coverage from the baseline run: `nvidiaScan` 0%,
`amdScan` 0%, `windowsGPUUsage` 0%, `platformScan` 0%, `QuickMemory` 0%, `Scan` 0%.

Evidence: the coverage table in `inventory.md` §3, and `grep -n "var run"` finds nothing.
Only the helper that was already extracted, `parseGPUUsage` (`gpu_discrete.go:127`), reaches
100%. That is the pattern the corpus needs: keep acquisition thin, put parsing in a pure
function.

This matters for coordination: a01, a03, a07 and a11 all need this and will otherwise each
invent a different seam and conflict on `internal/scan/*.go`. One shared decision now
(`var run = run` plus `var runCommand = ...`, or extract `parseNvidiaCSV`/`parseROCMJSON`/
`parseFreePercentage`) saves three merge conflicts.

### F2. `amdScan` underflows `MemFree` when a card reports more used than total (major, unproven on hardware)

`gpu_discrete.go:157-166`:

```go
total, _ := strconv.ParseUint(c["VRAM Total Memory (B)"], 10, 64)
used, _ := strconv.ParseUint(c["VRAM Total Used Memory (B)"], 10, 64)
...
h.GPUs = append(h.GPUs, GPU{..., MemTotal: total, MemFree: total - used})
```

Both are `uint64`. If `used > total` the subtraction wraps. Verified arithmetic:
`(512<<20) - (700<<20)` = `18446744073512419328`. `MemFree` feeds `GPUUsable`
(`scan.go:143-147`), the planner, and the live dashboard, so one bad card makes the machine
look like it has 16 EiB free. `displayScan` gets this right with `min(used, g.MemTotal)`
(`display_windows.go:39`).

Reachability is not proved: `amdScan` calls `run`, so it needs the seam from F1. Likely on
APUs, where `VRAM Total Used Memory` can include GTT. a03 owns the fixture; the fix is one
clamp (`if used > total { used = total }`).

### F3. A found-but-unreadable NVIDIA GPU is reported as "no GPU" (minor)

`gpu_discrete.go:24-28` skips any row whose `memory.total` is not a positive integer, and
returns `false` if every row was skipped (`:36-38`). `scan_other.go:7-18` then falls
through `amdScan`/`displayScan` and appends "No supported GPU found: models will run on the
CPU, which is slower." A machine with an NVIDIA card that reports `[N/A]` memory (NPU rows,
some vGPU/MIG configurations) is told it has no GPU at all, with no note naming the card.

Suggestion: when `nvidia-smi` ran and produced rows but none parsed, append a note naming the
raw name field. a01/a02 own the fixtures.

### F4. `amdScan` reads JSON into `map[string]map[string]string` (minor)

`gpu_discrete.go:152-155`. Any ROCm version that nests a level deeper or emits a numeric
value instead of a string fails `json.Unmarshal` and is reported as "no AMD GPU", silently.
a03 is testing exactly this; if 6.x drifts, this is where it breaks.

### F5. `osName` upper-cases the first byte, not the first rune (minor)

`scan.go:190-193`: `name = strings.ToUpper(name[:1]) + name[1:]`. Verified: `"éclipse"`
becomes `"\xc3\xa9"`-prefixed garbage, i.e. `"�\xa9clipse"` (the first byte of the two-byte
UTF-8 rune is upper-cased in isolation). Reachability depends on gopsutil returning a
non-ASCII `Platform` on Linux, which is unusual (it is normally `ubuntu`, `debian`). Cheap
fix: `strings.ToUpper(name[:1])` → operate on the first rune. Low priority.

### F6. `appleBandwidth` silently returns 100 GB/s for an unknown chip (minor)

`scan_darwin.go:56-83`. Unknown generation → `return 100`. This is a speed estimate only,
but it is fed to the planner, so a future M-series chip is under- or over-promised with no
trace. No test at all for the table. a07 should table-test the M1–M5 rows; whether 100 is
the right fallback is an open question.

### F7. The only Windows display test is a host probe, not a fixture test (minor)

`display_windows_test.go:14` calls `displayAdapters`/`adapterMemoryUsed` against the real
machine, logs the results, and asserts only when `EZLM_EXPECT_GPU` is set. It cannot run on
macOS at all (windows-only test file). `readAdapter`'s three-step VRAM fallback
(`qwMemorySize` → 4-byte binary → integer, `display_windows.go:84-90`) and the
`adapterMemoryUsed` `unsafe` struct layout (`:179-184`) have no fixture coverage anywhere.
a04/a05 need either a registry seam or a pure `vramFromValues(qw, binary, integer)` helper.

### F8. The brief's file names do not match the snapshot (note)

`run/T05-brief.md` names `exec.go`, `nvidia.go`, `rocm.go`, `display_*.go`, `sysctl*.go`,
`*_windows.go`, `*_darwin.go`. In this snapshot the parsers are in `gpu_discrete.go`,
`display_windows.go`, `scan_darwin.go`, `quick.go` and `apps*.go`; there is no `exec.go`,
`nvidia.go`, `rocm.go` or `sysctl*.go`. Also, `apps_windows.go` carries no explicit build
tag and is Windows-only through its filename suffix. Agents reading the brief literally will
look for files that do not exist.

## What I changed

- Added `agents/T05/a14/inventory.md`.
- Added `agents/T05/a14/REPORT.md` (this file).

No parser changes: my slice is inventory, and every fix belongs to the agent that owns the
fixture (a01, a03, a05, a07).

## What I could not finish

- **No fixtures in this worktree.** `internal/scan/testdata/` does not exist at the
  baseline commit; a01–a10 and a13 add it on their own branches. The inventory records
  coverage as baseline tests + 8 test functions, and names the corpus owner per gap.
  Regenerate §2/§3 after the morning merge.
- **Windows-only parsers could not be executed.** A cross-compiled test binary does not run
  on macOS, so `displayAdapters`, `readAdapter`, `adapterMemoryUsed`, `fileDescription` and
  `hex4` are reported as "windows-only, n/a" rather than measured.
- **F2 is code reading plus arithmetic, not a failing test**, because `amdScan` has no seam.
  The one-line clamp is safe to apply without it, but the proof belongs to a03.

## Next steps

1. Lead decision: pick one seam (`var run = run` or pure parse functions) before a01/a03/a07/
   a11 write tests, so they do not conflict on `internal/scan/*.go`.
2. a03: fixture where `used > total`; then clamp `MemFree` in `amdScan`.
3. a05: prove or disprove the `qwMemorySize`-in-MB concern (open question 3 in the
   inventory) and the 4 GB binary clamp.
4. a07: table-test `appleBandwidth` and the two `sysctl`/`memory_pressure` readers.
5. a15: the inventory's §3 list is a ready-made bug-hunt checklist; F2 and F5 are the two
   code-level bugs it should take first.
