# T05 a02: nvidia-smi across driver versions

Slice: `nvidia-smi` output drift between old and new drivers, the `--xml`
path, and a driver that reports no memory.

Files touched:

- `internal/scan/nvidia_versions_test.go` (new)
- `internal/scan/testdata/nvidia-smi/` (11 fixtures, see `SOURCES.md`)
- `internal/scan/gpu_discrete.go` (parser extracted and one bug fixed)

## Parser covered

`nvidiaScan` in `internal/scan/gpu_discrete.go` runs
`nvidia-smi --query-gpu=name,memory.total,memory.free,driver_version
--format=csv,noheader,nounits`, then `nvidia-smi` for the CUDA version and
`--query-compute-apps` for processes. The CSV row parsing is now
`parseNvidiaGPUs`, a pure function so the format can be tested without a GPU
or a fake binary.

The XML path is **not read**: EZLM never passes `--xml`/`-x`, and
`parseNvidiaGPUs` only understands the CSV query. `xml_r550_rtx4090.xml`
exists to pin that down (`TestNvidiaSmiXMLIsNotRead`).

## Bug found and fixed

**Unknown memory cost the machine its GPU** (major).

Baseline `internal/scan/gpu_discrete.go:24-28`:

```go
total, err1 := strconv.ParseUint(f[1], 10, 64)
free, err2 := strconv.ParseUint(f[2], 10, 64)
if err1 != nil || err2 != nil || total == 0 {
    continue // e.g. an "NVIDIA NPU" row reporting [N/A]
}
```

Any row whose memory fields did not parse was dropped whole. On a
MIG-enabled or virtualised GPU, `nvidia-smi` reports `memory.total` and
`memory.free` as `[N/A]` (NVIDIA documents unsupported data as `N/A`), so the
only NVIDIA card disappeared. If it was the only row, `nvidiaScan` returned
false, `platformScan` fell through to the CPU-only path, and the user got
"No supported GPU found" plus CPU inference on a working CUDA machine. A card
with a known total and an `[N/A]` free number was dropped the same way.

Fix (`parseNvidiaGPUs`): keep the card when only the memory fields are
unreadable, with the unknown value as 0 (not a made-up number), and still
skip `NVIDIA NPU` rows, which are not CUDA GPUs. A row that parses a total of
0 is still skipped. The GPU name, driver version and bandwidth estimate are
kept, so `nvidiaScan` keeps `Backend = CUDA`.

Tests: `TestNvidiaSmiNoMemoryKeepsGPU`, `TestNvidiaSmiNPURowIsSkipped`, plus
the `csv_r580_no_memory_mig.txt` and `csv_r450_free_unknown.txt` cases in
`TestNvidiaSmiCSVFormatDrift`. Run against the baseline row loop copied verbatim, the
`csv_r580_no_memory_mig.txt` fixture yields 0 GPUs; with the fix it yields 1.

## Format drift covered

| Case | Fixture | What it proves |
|---|---|---|
| R340 name without `NVIDIA` prefix | `csv_r340_gtx780_legacy.txt` | vendor is set by the parser, not the name; unknown cards get the 400 GB/s fallback |
| R470, R535, R550 field layout | `csv_r470_*`, `csv_r535_*`, `csv_r550_*` | four fields, MiB without units, two cards, differing free values |
| Windows CRLF | `csv_r550_windows_crlf.txt` | `\r` is trimmed, two rows parse |
| MIG / no memory | `csv_r580_no_memory_mig.txt` | card kept with 0/0 |
| Total known, free `[N/A]` | `csv_r450_free_unknown.txt` | total kept, free 0 |
| NPU row next to the GPU | `csv_r580_gb10_with_npu_row.txt` | NPU skipped, GPU kept |
| CUDA header rename | `header_r470.txt`, `header_r580.txt` | `CUDA Version:` and `CUDA UMD Version:` both parse |
| Header without CUDA | `header_r340_legacy.txt` | `CUDAMax` stays empty, no crash |

`TestNvidiaSmiCUDAHeaderDrift` also covers the rename that
`internal/scan/cuda_test.go` already asserted inline; it now has fixtures.

## Findings not fixed

1. **No fallback for drivers without `--query-gpu`** (minor). Very old
   (pre-R270) drivers, and `nvidia-smi` on some WSL and vGPU guests, fail the
   CSV query; `nvidiaScan` then returns false and the machine is treated as
   CPU-only even though `nvidia-smi -q` or `--xml` would have worked. Fixing
   it means a second output format; out of scope for this slice.
2. **`--query-compute-apps` used memory `[N/A]`** (minor, untested). Under
   the Windows WDDM model the used-memory column can be `[N/A]`; the parser
   keeps the PID with 0 bytes, which is later topped up by
   `windowsGPUMem`. Looks harmless, not verified with a fixture here because
   the process path needs a live PID.
3. **Name containing a comma** (minor, speculative). `splitCSV` splits on
   every comma and does not honour CSV quoting. No NVIDIA product name found
   with a comma, so no fixture; worth a fuzz case (a11).

## Open questions for the Lead

- **Overlap with a01.** a01 owns MIG output. My fix changes MIG behaviour:
  an `[N/A]`-memory A100 is now kept as a CUDA GPU with 0 bytes instead of
  being dropped. That is deliberate, but a01's tests should assert the same
  outcome or the merge will disagree.
- **Unknown memory means `GPUUsable = 0`.** The card is detected and CUDA is
  chosen, but the planner sizes the model from RAM only. On a MIG-enabled
  A100 that is a very small model. Should the scan fall back to
  `nvidia-smi -q -d MEMORY` for the number, or is CUDA-with-RAM-budget the
  intended compromise?
- **NPU row detection is a name heuristic** (`strings.Contains(name, "NPU")`).
  The code comment says the row is called `NVIDIA NPU`; I could not confirm
  the exact string on a GB10 without the hardware. If the name differs, the
  row would now be kept as a 0 GB GPU.

## Verification

- `gofmt -l internal/scan` (empty), `go vet ./internal/scan/`,
  `go test -p 2 ./internal/scan/` pass in this worktree.
- No host probe: every case runs from a committed fixture.
