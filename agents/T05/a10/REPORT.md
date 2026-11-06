# T05 / a10: edge machines

Slice: Windows on Arm with a shared-memory GPU, a VM with a passthrough GPU,
a machine reporting 0 bytes free, and a machine with absurdly large memory
(1 TB+).

Branch `agent/T05-a10`, worktree `~/ezlm-agents/work/subs/T05/a10`.

## What I added

- `internal/scan/edge_machines_test.go`: table and per-machine tests for the
  parsers that decide which GPU a machine has and how much of it is usable.
- `internal/scan/testdata/edge/` (11 fixtures, all synthetic reconstructions;
  provenance in `SOURCES.md`).
- Two small parser fixes and one behaviour-preserving extraction in
  `internal/scan/gpu_discrete.go` and `internal/scan/scan.go`.

Parsers covered:

| Parser | File | Fixtures |
|---|---|---|
| `usableAdapters` / `vendorOf` (display-class path) | `gpu_discrete.go:215`, `:243` | `display-*` |
| `nvidiaScan` (CSV query output) | `gpu_discrete.go:14` | `nvidia-smi-*` |
| `amdScan` (`rocm-smi --json`) | `gpu_discrete.go:147` | `rocm-smi-*` |
| `backendFor` (was inline in `Scan`) | `scan.go:194` | via the same fixtures |
| `readAdapter` (registry) | `display_windows.go:75` | format documented in `SOURCES.md`; not executable on macOS |

## Findings

### B1 (major, fixed): free VRAM wraps to ~16 EB when `used > total`

`amdScan` computed `MemFree: total - used` (`gpu_discrete.go:166`). On an APU
or under WDDM, `VRAM Total Used Memory (B)` includes shared system memory and
can exceed `VRAM Total Memory (B)`. The subtraction wraps:

```
--- FAIL: TestEdgeRocmSmiUsedOverTotal
    got {Name:Radeon 780M Graphics Vendor:amd MemTotal:536870912
         MemFree:18446744072635809792 ...}, want 512 MB with nothing free
```

`Scan` sums `MemFree` into `GPUUsable` (`scan.go:138`), so the planner then
believes the machine has 16 EB of VRAM: `internal/fit/fit.go:194` subtracts a
4 % margin and `internal/app/app.go:260` compares a model's ideal size against
it, picking the largest model in the catalog for a 512 MB APU. A single bad
driver reading is enough.

Fix: `clampFree(total, used)` (`gpu_discrete.go:179`) returns 0 when
`used >= total`; `amdScan` uses it. Test `TestEdgeRocmSmiUsedOverTotal` fails
before and passes after.

### B2 (major, fixed): the shared-memory GPU of Windows on Arm is invisible

`vendorOf` (`gpu_discrete.go:243`) knew `nvidia`, `amd` and `intel` only, and
`usableAdapters` dropped every other vendor. A Snapdragon X Elite laptop
reporting `Qualcomm(R) Adreno(TM) X1-85 GPU` with an 8 GB shared aperture
therefore produced no card at all, and `Scan` settled on the CPU backend:

```
--- FAIL: TestEdgeDisplayAdapters/display-woa-snapdragon-x-elite.txt
    got 0 cards: []
--- FAIL: TestEdgeWindowsOnArmBackend
    Windows on Arm with a shared GPU: got backend "cpu", want vulkan
```

The machine still works (CPU is a valid backend), but it loses the Vulkan build
for a GPU that shares its memory and is reported as present.

Fix: `vendorOf` returns `qualcomm` for `qualcomm`/`adreno`, `usableAdapters`
accepts it on the same terms as AMD and Intel built-in graphics (8 GB or more
of aperture; bandwidth stays 0 because the memory is the processor's), and
`backendFor` maps `qualcomm` to `Vulkan`. Both tests pass after the fix.

### B3 (minor, fixed): `nvidia-smi` free memory is not clamped to the card

`nvidiaScan` stored `free << 20` unchecked (`gpu_discrete.go:31`). Under WDDM
the driver counts shared system memory in `memory.free`, so a 4 GB laptop card
can report 6 GB free, and `GPUUsable` is inflated by the difference. The
fixture is synthetic, so this one is defensive rather than observed; the
invariant `MemFree <= MemTotal` is what the rest of the code assumes.
Fix: `min(free, total) << 20`. `TestEdgeNvidiaSmiFreeOverTotal` fails before
and passes after.

### Behaviour that was already right (pinned by tests)

- 0 bytes free: the card keeps its size, `GPUUsable` is 0
  (`TestEdgeNvidiaSmiNoFreeMemory`, `TestEdgeRocmSmiZeroFree`). `ComputeBudgets`
  turns that into a negative-looking 0 after the margin is subtracted, which
  the planner treats as "no GPU"; no panic.
- 1 TB+: `memory.total` is MiB and shifted in uint64, so eight 141 GB cards
  (1.1 TB) and a single 1 TiB card are exact
  (`TestEdgeNvidiaSmiTerabyte`), and 64-bit registry values of 96 GB and 48 GB
  survive `usableAdapters` (`display-1tb-workstation.txt`).
- A VM with an NVIDIA card passed through does not get a phantom Vulkan card
  from the display-class path; the guest is read by `nvidia-smi`, as the code
  intends.

## Changes

| File | Change |
|---|---|
| `internal/scan/gpu_discrete.go` | `clampFree` helper; `amdScan` and `nvidiaScan` clamp free VRAM; `vendorOf` knows Qualcomm/Adreno; `usableAdapters` accepts Qualcomm built-in graphics. |
| `internal/scan/scan.go` | the inline vendor-to-backend switch moved to `backendFor(gpus []GPU)` and `hasVendor` now takes `[]GPU`; same result for NVIDIA, AMD and Intel, plus Qualcomm to Vulkan. |
| `internal/scan/edge_machines_test.go`, `testdata/edge/**` | new. |

The `scan.go` change is an extraction, not a policy change, so the WoA backend
decision can be tested without running a full `Scan` on this Mac (where
`displayScan` is compiled out).

## Verification

Run in the worktree, all green:

```
go test -p 2 ./internal/scan/        # ok
go build ./...                       # ok
go vet ./internal/scan/              # ok
gofmt -l internal/scan/              # empty
GOOS=windows GOARCH=arm64 go build ./...
GOOS=windows GOARCH=amd64 go build ./...
GOOS=linux go build ./...
```

The fake `nvidia-smi` and `rocm-smi` are shell scripts at the front of `PATH`
(`fakeTool`, `t.TempDir()`), so the tests are hermetic and never probe this
host; they skip on Windows, where the fake would have to be a `.bat`.

## Open questions

1. **Is Vulkan on Adreno usable in practice?** llama.cpp's Vulkan backend does
   run on Adreno, but it is young, and the Qualcomm driver's reported aperture
   varies by driver version. If it turns out to be slower or unstable than the
   CPU build on Snapdragon X, the fix should become "recognise, but stay on
   CPU", and the threshold should move.
2. **Does a real Adreno report 8 GB or 2 GB?** The test covers both sides of
   the 8 GB built-in threshold; a real capture would settle which branch real
   laptops take.
3. **Shared memory is counted twice on built-in GPUs.** On WoA (and already on
   Ryzen AI Max) `GPUUsable` and `RAMAvail` both describe the same physical
   RAM, and `Unified` is only set on Apple Silicon. `ComputeBudgets` caps the
   unified case at the smaller of the two, but the non-unified path adds
   VRAM and RAM separately. Marking shared-memory adapters as unified would be
   the clean fix; it changes planning for every iGPU, so it is out of scope
   here.
4. **`HardwareInformation.MemorySize` as a binary value** (`display_windows.go:86`)
   is read as a little-endian uint32; a driver writing 8 bytes there is
   truncated to 4 GB. Only reachable on Windows and not exercised here; a05/a15
   territory.

## Next steps

- Capture one real Snapdragon X Elite display-class dump and one real
  `rocm-smi --json` from an APU, and replace the synthetic rows.
- Add a fit-level test (a16) that feeds the 0-free and 16 EB cases into
  `fit.ComputeBudgets` and asserts the plan is CPU-sized.
- Consider the unified-memory flag for shared-memory adapters (open question 3).
