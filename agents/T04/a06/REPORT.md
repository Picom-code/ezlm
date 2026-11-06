# T04/a06 report: Windows on Arm, large shared-memory GPU

## What I built

`internal/fit/profiles_edge_test.go` (new, a06's only test file):

- `woaProfiles`: 14 realistic Windows-on-Arm profiles built as `scan.Hardware`
  values. 12 are the RTX Spark N1X class (96 GB addressable by the GPU,
  30 GB left for Windows) at every level of GPU occupancy from idle to fully
  held by another app, plus two Snapdragon X Elite / Adreno laptops.
- `TestWindowsOnArmPlans`: pins the Balanced plan (model, quant, mode,
  context, KV type, NGL, NCPUMoE, lazy/direct/stream, speculative type) for
  every profile, and checks the invariants below for *every* plan offered
  under all three preferences.
- `TestWindowsOnArmBudgets`: pins the Windows cushion math (CUDA 8% / 1 GB
  floor, Vulkan 12% / 1.5 GB floor, RAM 10% / OS floor), that the cushion
  comes from free memory, and that the GPU pool may be larger than RAM.
- `TestWindowsOnArmPartlyOccupiedGPU`: sweeps 21 free-VRAM values from 96 GB
  down to 0 and asserts the pick never exceeds either budget and does not
  fall back to the processor while VRAM is free.
- `TestShowWindowsOnArmPlans`: `SHOW=1` table printer, used to produce
  `agents/T04/a06/plans.md`.

## Invariants asserted

1. `GPUNeed <= GPU budget` and `RAMNeed <= RAM budget` for every plan.
2. GPU-mode plans claim no RAM (`RAMNeed == 0`), so a shared-memory GPU is
   not double-booked against the 30 GB Windows pool.
3. Mode fields are internally consistent: GPU `NGL=999`; hybrid `NGL=999`
   with `0 < NCPUMoE <= Layers`; split `0 < NGL < Layers`; CPU `NGL=0`,
   `GPUNeed=0`, non-zero RAM.
4. `-t` is set to the physical core count whenever the CPU carries weights.
5. Windows load flags match the plan: GPU-resident is direct (`-lm none`) or
   lazy (`-lzm on`), never neither; non-GPU plans are never direct loaded;
   `Args()` agrees with the `DirectLoad`/`Lazy` fields.
6. Context is one the model supports and at least `Budgets.MinCtx`; KV type
   is one of f16/q8_0/q4_0 and f16 for `NoKVQuant` models.
7. Reported speed label matches the tok/s thresholds (`fast >= 30`,
   `good >= 8`, else `slow`), and `Download >= Quant.Bytes`.

## Profiles

The full profile -> plan table, budgets, and the Smartest/Fastest variants
are in `agents/T04/a06/plans.md`. All 14 plans pass.

## Findings

### F1 (minor): the flagship plan on this machine cannot direct-load
`internal/fit/fit.go:388` sets `DirectLoad` only when the quant has no lazy
tables. All five `qwen3.8-flash-next` quants carry `lazy_bytes` of 21-24 GB
(`internal/catalog/catalog.json`), and that is the model the 96 GB / 30 GB
RTX Spark N1X profile picks. So the plan is `-lzm on` without `-lm none`,
which means llama.cpp still memory-maps a 70.6 GB file on a machine with
20 GB of free RAM. `fit.go:43-47` says exactly that mapping a huge file with
little free RAM crashed Windows on an RTX Spark laptop. Pinned by
`TestWindowsOnArmPlans` (idle profile: `lazy=true, direct=false`). Not
changed: whether `-lm none` and `-lzm on` are compatible is a llama.cpp
question, and a wrong guess here would break loading. Open question for the
Lead / the serve owner.

### F2 (minor): partial occupancy can prefer a hybrid over a GPU-resident plan
At 56 GB free VRAM the Balanced pick is hybrid (4/48 expert layers in RAM)
at an estimated 71.2 tok/s, while the same model fully GPU-resident at 90 GB
free is estimated at 60.9 tok/s. The hybrid speed model bills only the
active expert slice against CPU bandwidth. Plausible for a 6B-active MoE,
but it is an estimator claim with no measurement behind it in this test, and
it is the sort of case where the plan with more RAM traffic can look faster.
Pinned in the table so a real run can confirm or refute it.

### F3 (informational): GPU budget can exceed installed RAM
On the N1X profile the GPU budget is 82.8 GB while `RAMTotal` is 30 GB.
That is the scan's model of a shared pool (GPU carves its memory out of the
SoC's LPDDR5X), and GPU-mode plans correctly claim no RAM. But hybrid plans
hold both pools at once (e.g. 36.6 GB GPU + 16.0 GB RAM at 40 GB free), so
they are only safe if the 96 GB GPU carve-out is real and separate. The test
asserts the budget arithmetic and the zero-RAM rule; it cannot verify the
hardware assumption.

## Not finished / next steps

- No real Windows-on-Arm measurements were taken (no model loads allowed),
  so the tok/s figures are estimator output, not calibration.
- Worth a follow-up: a `scan` field for "GPU and CPU share one physical
  pool" so `fit` could cap hybrid plans by the combined pool on SoC machines.
- `agents/T04/a06/plans.md` is regenerated with
  `SHOW=1 go test -run TestShowWindowsOnArmPlans -v ./internal/fit/`.

## Checks

`gofmt -l internal/fit/` empty, `go build ./...`, `go vet ./internal/fit/`,
`go test -p 2 ./internal/fit/` all pass on this branch.
