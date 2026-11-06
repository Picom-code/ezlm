# T04 / a12: the CPU fallback invariant

## Slice

The planner may put a model entirely on the processor only where it actually
fits, and never while a plan that uses the graphics card would fit.

Deliverables on `agent/T04-a12`:

- `internal/fit/invariant_cpu_test.go`: property tests over a generated
  38-machine profile space (1191 CPU plans exercised).
- `internal/fit/fit.go`: one minimal fix, found by a failing test (below).

## What the tests prove

`TestCPUPlanFitsAndIsNotPickedOverAGPU` generates a deterministic grid of
machines: processor-only 4 GB to 1.5 TB, a forced processor-only build with a
card present, CUDA/Vulkan/ROCm cards from 4 GB to 141 GB, a card fully used by
another app (0 GB free), Windows safety floors (1 GB CUDA, 1.5 GB Vulkan) that
leave no GPU budget, unified-memory machines 8 GB to 512 GB, and odd scans
(0 GB free, free > total, total unknown, GPU bigger than RAM, unified with no
wired limit). For every model, every preference and every plan it checks:

1. a CPU plan's independently recounted need (quant bytes minus lazy tables,
   KV and sliding-window cache, runtime overhead) is within the RAM budget;
2. `Plan.RAMNeed` equals that recount, so the plan cannot understate or
   overstate what it loads;
3. a CPU plan reserves nothing on the GPU (`GPUNeed`, `NGL`, `NCPUMoE` all 0);
4. a CPU plan appears only when the machine has no usable GPU (processor-only
   backend, or a GPU budget of 0 after the safety margin);
5. a CPU plan is never chosen when the same weights would fit the GPU budget;
6. no plan uses the GPU on a processor-only backend.

`TestCPUPlansAreNotPickedWhileAGPUPlanFits` pins clause 5 on every generated
machine that has a non-zero GPU budget.

All of this passes after the fix, over 1191 CPU plans.

## Bug found and fixed

**CPU, hybrid and split plans were sized for a speculative draft file they
never load.** `fit.go` added the draft bytes to the weights before the
placement math (`w += draftBytes`, old line 340), but hybrid, split and CPU
plans then drop speculative decoding and reset `Download` to the quant alone
(old lines 475-478). The result: `RAMNeed`/`GPUNeed` and the fit checks
counted a draft file that is never loaded.

Severity: **major** for "the smartest model that fits" (a fitting model is
rejected, so EZLM can say "nothing fits" on a machine where something does),
but conservative, so it never plans an unfitting load. Affected machines: any
CUDA machine for a model with a `draft` entry (10 models in the catalog,
e.g. `qwen3.8-27b` has a 3.16 GB MTP draft). The worst case is a CUDA laptop
whose card is busy with another app (GPU budget 0): the processor fallback is
the only option, and it was sized 0.5 to 3.2 GB too large per model.

Failing test first (`5632692`), then the fix (`ce877df`). Before:

```
windows cuda, 32GB RAM, 0.5GB free of 8GB/balanced: CPU plan for qwen3.8-27b
  reports RAMNeed 15983074102, actual need is 15446203190 (draft bytes counted?)
windows cuda, 32GB RAM, 0.5GB free of 8GB/balanced: CPU plan for gemma-4-31b
  reports RAMNeed 21502136988, actual need is 20445432101 (draft bytes counted?)
invariant_cpu_test.go:232: qwen3.8-27b fits the processor (8244700897 of
  8781571809 bytes) but got no plan
--- FAIL: TestCPUPlanFitsAndIsNotPickedOverAGPU (0.03s)
--- FAIL: TestCPUPlanIgnoresDraftBytes (0.00s)
```

After:

```
--- PASS: TestCPUPlanFitsAndIsNotPickedOverAGPU (0.04s)
    invariant_cpu_test.go:179: 1191 CPU plans checked over 38 profiles
--- PASS: TestCPUPlanIgnoresDraftBytes (0.00s)
    invariant_cpu_test.go:246: qwen3.8-27b UD-IQ2_XXS on the processor: ctx 32768,
      7.8 GB of 8.2 GB RAM, 4.4 tok/s
--- PASS: TestCPUPlansAreNotPickedWhileAGPUPlanFits (0.02s)
```

## The fix

`try` now keeps two sizes: the model weights alone (`wNoSpec`, `ohNoSpec`,
`fullNoSpec`, `activeNoSpec`) and the weights plus the draft file, used only by
the fully GPU-resident case. Hybrid, split and CPU plans are sized and scored
from the weights alone, matching what they load. GPU plans are unchanged.

## Verification

```
gofmt -l .                     # empty
go build ./...
go vet ./...
go test -p 2 ./internal/fit/   # ok, 0.25s
```

## Open questions (not changed)

- On a unified-memory machine whose wired limit reads as 0, `ComputeBudgets`
  returns both budgets as 0, so the planner offers nothing at all, not even a
  processor plan that would fit RAM (`fit.go:174-180`). `ComputeBudgets` is
  shared with the margin slice (a07), so it is left alone.
- A GPU machine gets no processor plan when a GPU plan exists but does not
  fit: `try` returns `false` in the `default` case instead of falling back to
  CPU (`fit.go:462-464`). That looks intentional (a slow processor plan is
  worse than a smaller model), but if a dense model is too big for the card
  and small enough for RAM, it is silently dropped. Product decision.
- `cpuInstead` (`internal/app/app.go:1627`, frozen) decides the runtime
  processor fallback from `Plan.GPUNeed+Plan.RAMNeed` against the RAM budget.
  That check stays conservative after this fix (hybrid plans add the batch
  working set), so no app change is needed.
