# T05 a03: rocm-smi across ROCm versions, AMD iGPUs, no ROCm

Parser covered: `amdScan` in `internal/scan/gpu_discrete.go` (runs
`rocm-smi --showmeminfo vram --showproductname --json` and reads the card
objects). The JSON parsing now lives in `parseROCmSMI` so it can be tested
without a GPU. Test file: `internal/scan/rocm_versions_test.go`. Fixtures:
`internal/scan/testdata/rocm-*`, provenance in `SOURCES.md`.

## What changed

- Extracted the rocm-smi JSON reading into `parseROCmSMI(out string) []GPU`.
  `amdScan` keeps the same command and the same "no cards means no AMD GPU"
  result, but reads through a `rocmSMI` hook a test can replace.
- Four parser fixes (findings 1 to 5 below), each with a test that failed
  before it.
- No new dependencies. `gofmt -l internal/scan` is empty.

## Fixtures

| Fixture | Machine | Expectation |
|---|---|---|
| `rocm-5.7-rx6800xt.json` | ROCm 5.7, RX 6800 XT | lower-case `Card series` read; 16 GiB; 512 GB/s |
| `rocm-6.1-rx7900xt.json` | ROCm 6.1, RX 7900 XT (real capture) | title-case `Card Series` read; 20 GiB; 800 GB/s |
| `rocm-6.4-multi-gpu.json` | two Radeon cards | both read, in `cardN` order |
| `rocm-7.1-strix-halo-apu.json` | Strix Halo APU (real capture) | name is not `N/A`; 1 GiB carveout |
| `rocm-6.2-igpu-780m.json` | Phoenix iGPU (780M) | name kept; no invented bandwidth |
| `rocm-6.0-no-gpu.json` | ROCm installed, no AMD card | no GPUs, no backend change |
| `rocm-6.0-low-power-banner.json` | warning line before JSON | card still read |
| `rocm-6.4-used-over-total.json` | stale used counter | free is 0, not 18 EB |

## Findings

### 1. A warning line before the JSON hides the card (major, fixed)

`amdScan` called `json.Unmarshal` on the whole stdout. On machines where
rocm-smi prints a low-power warning before the JSON (reported in guaardvark
PR #362), the unmarshal fails, `amdScan` returns false, and `platformScan`
falls through to "No supported GPU found" and the CPU backend. The GPU is
there and works; EZLM just does not see it.

Evidence (before the fix): `rocm_versions_test.go:101: banner hid the card: []`.

Fix: `parseROCmSMI` starts at the first `{` and ends at the last `}`, so
leading warnings and trailing noise are ignored. Invalid JSON still yields no
GPUs.

### 2. Used > total wraps free memory (major, fixed)

`MemFree: total - used` is unsigned. A stale or shared `VRAM Total Used
Memory (B)` counter larger than the total wraps to about 18 EB.

Evidence (before the fix): `free memory wrapped: {MemTotal:8573157376
MemFree:18446744073282708992}`.

Fix: clamp `used` to `total`, so free is 0.

### 3. Strix Halo APUs report `"Card Series": "N/A"` (major, fixed)

The real Strix Halo capture in marimo issue #9237 has `"Card Series": "N/A"`
(and `Card SKU: STRXLGEN`). `amdScan` used the value as-is, so the hardware
report and the UI showed a GPU called `N/A`.

Evidence (before the fix): `GPUs [{Name:N/A ...}]`.

Fix: an empty or `N/A` name becomes `AMD Radeon Graphics`, the name AMD uses
for built-in graphics. This is also what the Windows display path shows for
the same hardware.

### 4. AMD GPUs on Linux got no bandwidth (major, fixed)

`amdScan` never set `GPU.Bandwidth`, unlike the Windows display path
(`usableAdapters` -> `otherBandwidth`). `internal/fit/fit.go:376` uses
`g.Bandwidth` and falls back to `h.CPUBandwidth` (50 GB/s) when it is 0, so a
7900 XT was planned at 50 GB/s instead of 800: a roughly 16x speed
underestimate and a smaller model or CPU fallback than the card deserves.

Evidence (before the fix): `Bandwidth:0` for the 6800 XT and 7900 XT
fixtures.

Fix: discrete names (the same `discreteRe` the Windows path uses) get
`otherBandwidth(name)`. Built-in graphics keep 0, because they use the
processor's memory bandwidth; that matches the Windows path's comment.

### 5. Multi-GPU order was map order (minor, fixed)

`amdScan` iterated the `map[string]map[string]string` directly, so the order
of cards in `Hardware.GPUs` changed from run to run. The planner takes the
maximum, so plans were stable, but the hardware report and any UI list were
not.

Fix: sort the card keys by their number (`card2` before `card10`). The
multi-GPU fixture pins the order.

### 6. APU memory is under-read: the GTT pool is ignored (major, not fixed)

On a UMA APU (Strix Halo, Phoenix), `--showmeminfo vram` reports only the
BIOS carveout. The real capture shows 1 GiB total with 976 MiB already used,
while the machine also has `GTT Total Memory (B) = 134217728000` (125 GiB)
and `GTT Total Used Memory (B) = 41290534912` (38 GiB). The scan therefore
reports a ~47 MiB free GPU, and the fit planner will pick the CPU or a tiny
model on a 128 GiB Strix Halo laptop.

The fix is not in this patch because it changes the command and the memory
model: `--showmeminfo vram gtt` would add the GTT keys, and using GTT as GPU
memory risks counting the same RAM twice (`Hardware.RAMAvail` already covers
it) unless the planner knows the GPU is unified. `Hardware.Unified` is Apple
only today. Open question for the planner owner: should an AMD APU set
`Unified = true` and report GTT free as `GPUUsable`, capped by `RAMAvail`?

Related: koboldcpp issue #1104 (2024-08-30) describes the same iGPU
reserved-memory problem.

### 7. Instinct cards get 0 bandwidth (minor, not fixed)

`otherBandwidth` has no MI entries, so an MI210/MI300X read from rocm-smi
gets 0 and the planner falls back to 50 GB/s. Adding MI bandwidths to the
lookup is small but outside this slice.

### 8. The low-power warning normally goes to stderr (note)

In current `rocm_smi.py` the warning is `logging.warning`, which goes to
stderr and does not disturb `cmd.Output()`. PR #362 reports it on stdout on
some machines/versions, which finding 1 covers.

## Verification

Before the fix, the new tests failed with the evidence quoted above. After:

```
gofmt -l internal/scan   # empty
go test -p 2 ./internal/scan/   # ok
```

`go build ./...` and `go vet ./...` were run before the last commit.

## Open questions

- Finding 6: how should AMD APUs report usable memory to the planner?
- Finding 7: add Instinct bandwidths, or leave them to the CPU fallback?
- ROCm 4.x was not captured; its key spelling matches 5.x per the source, but
  no real output was found.
