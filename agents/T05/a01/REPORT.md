# T05 a01: nvidia-smi CSV corpus

## What I covered

Parser: `nvidiaScan` in `internal/scan/gpu_discrete.go:14`, which reads

```
nvidia-smi --query-gpu=name,memory.total,memory.free,driver_version --format=csv,noheader,nounits
nvidia-smi --query-compute-apps=pid,process_name,used_memory --format=csv,noheader,nounits
```

I extracted the two pure parsers `parseNVIDIAGPUs` (`gpu_discrete.go:47`) and
`parseNVIDIAComputeApps` (`gpu_discrete.go:90`) so the CSV can be table-tested
without running nvidia-smi on the host. `nvidiaScan` keeps its behaviour:
same commands, same order, same enrichment through `procFor`.

Tests: `internal/scan/nvidia_mig_test.go` (table over every fixture) with
fixtures in `internal/scan/testdata/nvidia/`. Provenance in
`agents/T05/a01/SOURCES.md`.

## Fixtures

| Fixture | Case | Result asserted |
|---|---|---|
| `query_gpu_dual_4090.csv` | multiple GPUs, one in use | two GPUs; first has 23000 MiB free, second 24564 MiB |
| `query_gpu_laptop_4060.csv` | laptop GPU, CRLF | one GPU, 8188/7600 MiB, driver parsed |
| `query_gpu_a100_mig.csv` | MIG enabled | one parent A100, 81920 MiB total and free |
| `query_gpu_gb10_na.csv` | `[N/A]` memory, unified | one GB10 sized from RAM; the NPU row is not a CUDA device |
| `query_gpu_wddm_na.csv` | `[N/A]` memory, WDDM | no GPU (documented, see finding 3) |
| `query_gpu_free_na.csv` | free `[N/A]`, total known | card kept with free = total |
| `compute_apps_busy.csv` | process list with `[N/A]` | python3 with 6 GiB, Xorg with 0 bytes, invalid PID skipped |

## Findings

1. **Major (fixed): a GB10/Jetson with `[N/A]` memory was dropped, so the
   machine scanned as CPU-only.** `nvidia-smi` prints `[N/A]` for
   `memory.total`/`memory.free` on unified-memory NVIDIA parts (GB10, DGX
   Spark, Jetson/Tegra) because they have no framebuffer of their own
   (llama-swap PR #965). The old loop did `continue` on any parse failure, so
   `h.GPUs` stayed empty, `nvidiaScan` returned false and `platformScan`
   ended with `Backend = CPU` and "No supported GPU found", even though
   EZLM's bandwidth table already knows `gb10`, `n1x` and `spark`.
   Evidence: before the fix, the old loop over `query_gpu_gb10_na.csv`
   returned 0 GPUs (temporary test, output `old loop on GB10: 0 GPUs: []`).
   Fix: for names `gb10`, `n1x`, `spark`, `jetson`, `tegra`, fall back to
   `h.RAMTotal`/`h.RAMAvail` (the scan sets them before `platformScan`), the
   same way macOS reports unified memory. A row whose free memory is unknown
   but whose total is known is now kept with free = total instead of dropped.

2. **Major (not fixed, design): on a MIG-enabled GPU the parser reports the
   parent card's whole memory, not the slice a CUDA job gets.** Measured on an
   A100-SXM4-80GB with a 1g.10gb slice, `--query-gpu=memory.total` printed
   `81920` (bioengine PR #226); `--query-gpu` enumerates physical GPUs, while
   `nvidia-smi -L` lists the MIG devices. `query_gpu_a100_mig.csv` pins the
   current behaviour. Consequence: EZLM can plan a model larger than one MIG
   slice and llama.cpp then OOMs or fits only part of the layers. The fix is
   to read `mig.mode.current` and the `-L` profile names (`MIG 3g.40gb`) and
   use the slice size when MIG is on; that needs its own fixtures and is
   bigger than this slice, so it is left as an open question. A container
   with `CUDA_VISIBLE_DEVICES=MIG-...` has the same over-report.

3. **Minor (left to a02): a WDDM card that reports no memory at all is still
   dropped.** The YOLOX report (`query_gpu_wddm_na.csv`) shows a Quadro RTX
   5000 whose `memory.total` and `memory.used` are both `[N/A]` under WDDM;
   the card exists and can run CUDA, but the scan treats the machine as
   CPU-only. The a02 slice owns "a driver that reports no memory"; the
   fixture and the assertion here document the current behaviour so a02 can
   change it.

4. **Minor: a process with `[N/A]` used memory shows as 0 B.** On Linux the
   `--query-compute-apps` row is kept with `Mem: 0` (`gpu_discrete.go:90`);
   Windows later filters sub-64 MiB entries, Linux does not. Informational
   only, not fixed.

5. **Minor: laptop bandwidth uses the desktop lookup.** "RTX 4060 Laptop
   GPU" matches `4060` and gets 272 GB/s (desktop) rather than the laptop's
   ~256 GB/s. This is only a speed estimate, so the test pins 272; no fix.

## Files changed

- `internal/scan/gpu_discrete.go`: extracted `parseNVIDIAGPUs`,
  `parseNVIDIAComputeApps`, added `unifiedNVIDIA`; `nvidiaScan` calls them.
  No command-line change.
- `internal/scan/nvidia_mig_test.go`: new table tests.
- `internal/scan/testdata/nvidia/*`: seven small fixtures.
- `agents/T05/a01/SOURCES.md`, `agents/T05/a01/REPORT.md`.

## Verification

```
gofmt -l internal/scan        # empty
go build ./...                # ok
go vet ./internal/scan        # ok
go test -p 2 ./internal/scan/ # ok
```

## Open questions

- Should MIG-enabled hosts size for the largest MIG slice (or the parent) and
  should the UI say "MIG"? Needs a product decision; see finding 2.
- Should a WDDM card with no memory fall back to system RAM (as GB10 does),
  or to the Windows display-class registry value? a02/a05 territory.
- The GB10 fallback uses all system RAM. On a Jetson with a small carve-out
  that may overstate usable VRAM; a `nvidia-smi -q` carve-out read would be
  more exact.
