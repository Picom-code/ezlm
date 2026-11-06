# T04 / a07: safety margin invariant

Slice: planned bytes never exceed usable memory after the documented margin,
for every generated profile and every model the planner picks.

## What I built

`internal/fit/invariant_margin_test.go`, a property test over a generated
profile space (seeded `math/rand`, reproducible) plus 6 hand-built edge
profiles. The generator covers what `scan.Scan` produces: processor-only
machines, unified Apple Silicon (Metal), discrete CUDA/Vulkan/ROCm cards with
1 to 3 GPUs, cards partly occupied by another app (free well below total),
Windows and Linux, RAM from 4 GB to 1.5 TB, and failed-probe values (zero
RAM, zero GPU, a 1 MB machine, a 1 TB fake GPU). The catalog is the real
`internal/catalog/catalog.json` via `EZLM_CATALOG`.

Tests:

| Test | Property |
|---|---|
| `TestMarginBudgets` | `ComputeBudgets` leaves exactly the documented margin: 10% of free RAM with `osFloor` floor; discrete GPU 4%/0.4 GB, Windows 8%/1 GB, Windows Vulkan 12%/1.5 GB; unified = one pool capped at `GPUUsable` and free RAM. Budgets never exceed real memory, including when `availRAM` is `RAMTotal` (the app's "close your apps" path). |
| `TestMarginPlansNeverExceedBudgets` | For every plan from `PlansFor` under Balanced/Smartest/Fastest, both with `RAMAvail` and `RAMTotal`: `GPUNeed <= GPUBudget`, `RAMNeed <= RAMBudget`, plan budgets equal the computed budgets, and needs fit the machine's real `GPUUsable`/available RAM. 30,972 plans over 206 profiles. |
| `TestMarginStreamStaysNearBudget` | The only documented exception is MoE expert weights paged from SSD (`Stream`). It is only set on Hybrid plans, and the recomputed expert bytes stay within the documented 1.1x RAM budget (72 streamed plans). |
| `TestMarginLargestPlan` | The recommended (deployed) plan for every profile and preference is inside the budget. |
| `TestMarginHybridIgnoresUnusedDraft` | Regression test for the bug below. |

Result: all pass. The margin invariant holds for the current planner.

## Bug found and fixed

**Hybrid plans reserved the draft model's bytes although hybrid mode drops
speculative decoding.** Severity: minor (over-conservative: it overstates
memory and can reject a model that would fit; it never plans a load that is
too big).

`try()` (`internal/fit/fit.go`) added `draftBytes` to the resident weights
and added `specOverhead(ctx)` to the compute buffers before choosing a mode.
The mode is chosen with those bytes included, but later, when the plan is
Hybrid/Split/CPU, speculative decoding is dropped (`p.Spec = ""`,
`p.Download = q.Bytes`) while `GPUNeed`/`RAMNeed` keep the draft bytes. On a
CUDA machine that means a hybrid plan reserves the draft file plus its
context/verify overhead for a draft that is never run.

Evidence (failing test before the fix, RTX 4090 24 GB + 96 GB):

```
nemotron-3.5-lightning-30b-a3b UD-Q5_K_XL ctx=8192: hybrid plan reserves unused draft bytes: gpu 23574342903 vs 23482800025, ram 11565837355 vs 10028136939
nemotron-3.5-lightning-30b-a3b Q8_0 ctx=32768: hybrid plan reserves unused draft bytes: gpu 23686902425 vs 23177848096, ram 16559125959 vs 15388605348
... 73 hybrid plans checked, many off by 0.5-3.5 GB
```

Smallest fix in `internal/fit/fit.go`:

- keep the layout weights `w` without `draftBytes`; use `wWithDraft` only for
  the fully GPU-resident need (`fullGPU = wWithDraft + kv + oh` plus
  `specOverhead` when a draft runs);
- `oh` is computed from `w`; `specOverhead` is added only to `fullGPU`;
- Hybrid and Split use `w` (and `memActiveBytes` for the dense part), CPU
  uses `fullNoSpec = w + kv + oh`.

Non-CUDA machines and models without a draft are unaffected (`draftBytes` is
0). `go test -p 2 ./internal/fit/` passes after the fix, including the
baseline `TestRealMachines` picks.

## Other findings (not fixed, not my slice to change)

- `internal/fit/fit_test.go:54` says streamed expert weights are "capped at
  1.3x RAM"; `internal/fit/fit.go:422` enforces 1.1x. The comment is stale.
  Severity: minor (documentation only).
- `internal/app/app.go:1631` (`cpuInstead`) tests
  `p.GPUNeed+p.RAMNeed > ComputeBudgets(hw, hw.RAMAvail).RAM`, summing VRAM
  and system RAM against the RAM budget. On a discrete GPU those are separate
  pools, so this can wrongly refuse the CPU fallback for a plan that fits.
  Severity: minor; outside `internal/fit`, so reported only.
- On a fully GPU-resident plan, `GPUNeed` does not include
  `batchOverhead` (1.2 GB) while Hybrid/Split do. The comment says that
  buffer is for the modes that pass `-ub 2048`, so this may be intended
  (GPU-resident plans use the default batch); not provable without running
  llama.cpp, so open question.

## Open questions

- Should `Stream` plans report the unclamped expert bytes somewhere, so the
  UI can show "paging from SSD" accurately? Today `RAMNeed` is clamped to the
  budget, which hides the 1.1x overage from callers.
- The generator assumes `GPUUsable` is authoritative (it is what `scan.Scan`
  sets). A platform that sets `GPUs` but leaves `GPUUsable` zero gets no GPU
  plan; that is safe, not a margin bug.

## Commands run

```
go test -p 2 -run 'TestMargin' -v ./internal/fit/   # pass
go test -p 2 ./internal/fit/                        # ok
gofmt -l internal/fit/                              # empty
go vet ./internal/fit/                              # ok
go build ./...                                      # ok
```
