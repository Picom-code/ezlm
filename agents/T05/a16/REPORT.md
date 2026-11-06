# T05 / a16: scan to fit, end to end

## What this slice does

`internal/scan/scan_to_fit_test.go` (package `scan_test`) takes ten realistic
machine fixtures, runs them through the real scan parsers, then hands the
resulting `scan.Hardware` to `fit.ComputeBudgets` + `fit.PlansFor` +
`fit.Recommend` for all three preferences. It asserts:

- the parser filled the fields the planner depends on (backend, GPU name,
  vendor, free VRAM, unified flag, notes);
- the planner produces at least one plan and a recommendation, and no plan
  exceeds its budget;
- nothing panics on any fixture (a panic fails the subtest).

`internal/scan/fixture_a16_test.go` is a small test-only shim. The scan
parsers are command-driven (`run("nvidia-smi", ...)`, `run("rocm-smi", ...)`),
so the test installs fake executables on `PATH` and the shim calls the real
`nvidiaScan` / `amdScan` / `usableAdapters` in the same order `platformScan`
uses, then applies `Scan`'s `GPUUsable` derivation. This keeps the production
code untouched and still exercises the actual parsers, not a copy.

## Fixtures (`internal/scan/testdata/a16/`)

Each file is a small sectioned text file (`# section` headers); sections are
served to the fake tools. They are synthetic cases built from the documented
output formats of `nvidia-smi --query-gpu=... --format=csv,noheader,nounits`,
`nvidia-smi --query-compute-apps=...`, `rocm-smi --showmeminfo vram
--showproductname --json`, the Windows display-adapter list and macOS `sysctl`
(no public issue URL is cited because no fixture was copied verbatim).

| fixture | machine | what it proves |
|---|---|---|
| `nvidia-4070-desktop.txt` | RTX 4070 SUPER 12 GB, Linux, one Xorg process | normal CUDA path |
| `nvidia-4060-laptop.txt` | RTX 4060 Laptop 8 GB, Windows, `[N/A]` compute row | WDDM `[N/A]` does not break parsing |
| `nvidia-a100-mig.txt` | two A100 80 GB, MIG on, extra `NVIDIA NPU, [N/A]` row | `[N/A]` row is skipped, both real cards counted |
| `nvidia-4090-busy.txt` | RTX 4090 24 GB, 6 GB free, 20 GB RAM available | busy card still gets a plan |
| `nvidia-4090-full.txt` | RTX 4090 24 GB, 800 MB free | no plan today, no panic (see finding 1) |
| `rocm-mi300x.txt` | MI300X 192 GB, rocm-smi 6.x JSON | ROCm JSON path, bytes are not MiB |
| `rocm-780m.txt` | Radeon 780M iGPU, 512 MB carve-out | iGPU counted as Vulkan, CPU plan still produced |
| `cpu-only-16gb.txt` | Ryzen 7 5700G, no GPU tools | CPU backend and note |
| `windows-arc.txt` | Arc B580 12 GB + UHD 770 | `usableAdapters` picks the discrete card only |
| `apple-m5max.txt` | M5 Max 128 GB | unified memory, 75% wired limit, Metal |
| `apple-m1-8gb.txt` | M1 8 GB | smallest Apple Silicon, unified budget |

The Apple cases are built by the test from `sysctl` text with the same rules
as `internal/scan/scan_darwin.go` (0.67 of RAM, 0.75 above 36 GB, or
`iogpu.wired_limit_mb`), because that file is build-tagged `darwin` and the
suite must run on Linux too.

## Findings

1. **major, not fixed (outside T05's owned paths): the planner returns no plan
   at all when a GPU exists but has less than ~1 GB free.** With an RTX 4090
   reporting 800 MiB free and 20 GB RAM available (`nvidia-4090-full.txt`),
   `fit.PlansFor` returns an empty slice for `Balanced`, `Smartest` and
   `Fastest`, so `Recommend` reports "nothing fits". In `internal/fit/fit.go`
   `try()` the GPU branch needs `full <= b.GPU`, the hybrid and split branches
   need `batchOverhead` (1.2 GB) to fit in `b.GPU`, and the CPU branch is
   guarded by `!hasGPU` (fit.go:386, fit.go:404, fit.go:441, fit.go:455), so a machine with a busy
   card has no fallback. A CPU-only plan would run. The subtest is marked
   `skipPlan` and asserts no panic. Suggested fix: when `hasGPU` and the GPU
   budget cannot hold `batchOverhead`, fall through to the CPU branch.
2. **minor, observation:** `internal/scan/scan_darwin.go:9` never calls
   `nvidiaScan`/`amdScan`, so an Intel Mac with an eGPU is planned as CPU-only.
   Not exercised here (no fixture can change `runtime.GOARCH`).

## Commands run

- `gofmt -l internal/scan/` (empty)
- `go vet ./internal/scan/ ./internal/fit/`
- `go build ./...`
- `go test -p 2 ./internal/scan/ ./internal/fit/`

## Open questions

- The other a01-a10 corpora live on their own branches, so this suite uses its
  own fixtures. When the branches merge, the fixtures can be replaced by the
  shared corpus without changing the test shape (the section format is local to
  this file).
- Is the 800 MB case worth a planner fix, or does the app always close apps
  first? `internal/app/app.go:239` plans with the raw scan, so the empty-plan
  path is reachable.
