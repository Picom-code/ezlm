# T05 a11: fuzz tests for the scan text parsers

Worktree `work/subs/T05/a11`, branch `agent/T05-a11`.

## What this slice did

Added `internal/scan/fuzz_parsers_test.go` with a `testing.F` target for every
pure text parser in `internal/scan`, seeded from the output shapes the team's
corpus is built on (`nvidia-smi` CSV, `rocm-smi --json`, `memory_pressure`,
Windows GPU process counters, and app paths).

The snapshot in this worktree does not have the `nvidia.go`, `rocm.go`,
`sysctl*.go` and `exec.go` files the brief names: the parsers are inline in
`gpu_discrete.go`, `quick.go`, `scan_darwin.go` and `display_windows.go`. To
make the inline ones fuzzable, I moved the parsing loops into small pure
functions with the same behaviour, and called those from the existing entry
points:

| new function | file | replaces inline code in |
|---|---|---|
| `parseNvidiaGPUs` | `gpu_discrete.go` | `nvidiaScan` query-gpu loop |
| `parseNvidiaProcs` | `gpu_discrete.go` | `nvidiaScan` query-compute-apps loop |
| `parseCudaVersion` | `gpu_discrete.go` | `nvidiaScan` header regexp use |
| `parseRocmCards` | `gpu_discrete.go` | `amdScan` JSON loop |
| `parseNvidiaMem` | `quick.go` | `QuickMemory` query-gpu loop |
| `parseFreePercentage` | `quick.go` | `QuickMemory` and `scan_darwin.go` `platformScan` |

`display_windows.go` is Windows-only and reads the registry through
`readAdapter`, so it has no pure text parser to fuzz; a04 and a05 cover it
with fixtures. `displayAdapters` only splits PowerShell output on whitespace.

## Fuzz targets

| target | parser | invariants asserted |
|---|---|---|
| `FuzzSplitCSV` | `splitCSV` | field count is commas+1; every field trimmed |
| `FuzzParseNvidiaGPUs` | `parseNvidiaGPUs` | vendor nvidia; total > 0; free <= total; MiB shifted to bytes; empty in, empty out |
| `FuzzParseNvidiaProcs` | `parseNvidiaProcs` | PID > 0; MiB shifted to bytes |
| `FuzzParseCudaVersion` | `parseCudaVersion` | empty or a dotted numeric version present in the header |
| `FuzzParseRocmCards` | `parseRocmCards` | vendor amd; total > 0; free <= total (no unsigned wrap) |
| `FuzzParseNvidiaMem` | `parseNvidiaMem` | free <= total; MiB shifted to bytes; empty in, zeros out |
| `FuzzParseGPUUsage` | `parseGPUUsage` | values < 1 TiB; no `pid_` instance, no map |
| `FuzzParseFreePercentage` | `parseFreePercentage` | when ok, 0 <= pct <= 100 and input had `%` |
| `FuzzUsableAdapters` | `vendorOf`, `*Bandwidth`, `usableAdapters` | vendor in the known set; bandwidth > 0; no zero-memory card; free == total |
| `FuzzAppPaths` | `baseName`, `installRoot`, `friendlyName` | lowercased base; non-empty root and friendly name; no panic on empty/Unicode/Windows paths |

Seeds are inline so the file is self-contained. Once the corpus fixtures land
under `internal/scan/testdata/`, a13's loader can add them as seeds without
changing the assertions.

## Bugs found and fixed

1. **A huge `nvidia-smi` memory value wrapped to zero and the GPU was
   silently dropped.** `parseNvidiaGPUs` shifted MiB to bytes without a
   range check: `total = 10000000000000000` (1e16) becomes 0 after `<< 20`,
   and the row is skipped as if the card had no memory. Severity: minor
   (real values are small, but a driver glitch loses the card with no note).
   Fixed with `maxMiB = ^uint64(0) >> 20` guards in `parseNvidiaGPUs`,
   `parseNvidiaProcs` and `parseNvidiaMem`. Found by fuzzing; the failing
   input is committed as a regression seed.

2. **`amdScan` wrapped free memory when rocm-smi reported used > total.**
   `gpu_discrete.go`, old `amdScan`: `MemFree: total - used`. With
   `{"VRAM Total Memory (B)": "100", "VRAM Total Used Memory (B)": "200"}`
   the result is `18446744073709551516` bytes, and the planner would think the
   card is empty. Severity: minor (needs a driver/counter glitch, but the
   result is absurd and silent). Fixed by clamping `used` to `total` in
   `parseRocmCards`; seed present in `FuzzParseRocmCards`.

3. **`cudaVersionRe` accepted `CUDA Version: ..`.** `[0-9.]+` matched a
   version of dots only. Severity: minor (cosmetic in the UI's CUDA line).
   Tightened to `[0-9]+(?:\.[0-9]+)*`; seed present in `FuzzParseCudaVersion`.

4. **`parseNvidiaMem` could report free > total.** A crafted or garbled row
   (`0, 5`) produced `GPUTotal=0, GPUFree=5 MiB`, and `QuickMemory` then
   threw the free value away and fell back to the AMD/Intel path. Severity:
   minor. Clamped in `parseNvidiaMem`; seed present.

5. **`friendlyName(".app/")` returned an empty name.** `apps.go` took
   `filepath.Base(exe[:i+4])` and trimmed `.app` without checking the result,
   so a path whose `.app` bundle name is empty produced `""`, which the app
   list would show as a blank row. Severity: minor. Now falls through to the
   file-name path when the bundle name is empty. Found by `FuzzAppPaths`; the
   input is committed as a regression seed.

6. **`parseFreePercentage` accepted out-of-range values** such as `101%` or
   `-1%` from a garbled `memory_pressure` line, which then scaled
   `RAMAvail` past `RAMTotal`. Severity: minor. Now returns not-ok outside
   0..100. The same parser also rejects a decimal comma (`62,5%`), which is a
   locale note for a12 rather than a fix here. A missing percent sign
   (`free percentage: 62`) is accepted on purpose, so the fuzz assertion only
   checks the range.

Also added `free > total` clamping in `parseNvidiaGPUs` so a driver that
reports free above total cannot make the planner over-allocate. No behaviour
change for real `nvidia-smi` output.

## Fuzz runs

Each target ran for 30s with `-parallel=4` on this Mac:

```
go test -run '^$' -fuzz '^FuzzX$' -fuzztime=30s -parallel=4 ./internal/scan/
```

| target | first run | after fixes |
|---|---|---|
| `FuzzSplitCSV` | PASS, 4,644,611 execs | same |
| `FuzzParseNvidiaGPUs` | FAIL (MiB overflow) | PASS, 4,029,178 execs |
| `FuzzParseNvidiaProcs` | PASS, 4,160,268 execs | same |
| `FuzzParseCudaVersion` | PASS, 4,234,490 execs | same |
| `FuzzParseRocmCards` | PASS, 3,737,717 execs | same |
| `FuzzParseNvidiaMem` | PASS, 3,731,886 execs | same |
| `FuzzParseGPUUsage` | PASS, 4,392,448 execs | same |
| `FuzzParseFreePercentage` | FAIL (over-strict assertion) | PASS, 4,551,620 execs |
| `FuzzUsableAdapters` | PASS, 426,343 execs | same |
| `FuzzAppPaths` | FAIL (empty name for `.app/`) | PASS, 2,552,869 execs |

No failures were left running. The three failing inputs are committed under
`internal/scan/testdata/fuzz/` so the ordinary `go test` run replays them.

## Verification

- `gofmt -l internal/scan/` empty
- `go vet ./internal/scan/` clean
- `go test -p 2 ./internal/scan/` passes
- `go build ./...` passes
- `GOOS=windows GOARCH=amd64 go build ./...`, `GOOS=windows GOARCH=arm64 go build ./...` and `GOOS=linux go build ./...` pass

## Open questions

- `parseGPUUsage` still keeps `pid_0_...` (system) entries; Windows reports
  them, and they are filtered later by `procFor`, so I left it.
- `readAdapter` on Windows uses `GetBinaryValue` for the old 4-byte memory
  value; that path needs Windows to exercise, so it is not fuzzed here.
- Whether the corpus should add seeds for the `nvidia-smi` header text as well
  as the query rows; the header regexp is covered, the driver string is not.
