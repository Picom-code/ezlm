# T04 / a10: MoE expert offload math

Branch `agent/T04-a10`, worktree `~/ezlm-agents/work/subs/T04/a10`.
Slice: the MoE expert offload math. Experts moved to system RAM
(`--n-cpu-moe`) must fit in RAM, not VRAM, and the plan's own RAM/VRAM
numbers must agree with the budgets `ComputeBudgets` produced.

## What I built

`internal/fit/invariant_moe_test.go`, three tests over a generated profile
space (118 machines built directly as `scan.Hardware`, no parsing):

- **`TestMoEOffloadFitsRAM`** sweeps every MoE model in the catalog, every
  quant, the context ladder (64K, 32K, 16K, 8K) and every KV type the model
  accepts. For each hybrid plan it recomputes the split (dense bytes vs one
  layer of experts) and asserts:
  - the layers kept on the card fit the VRAM budget (`GPUNeed <= b.GPU`);
  - the expert bytes moved to RAM fit the RAM budget, or exceed it by at
    most the documented 10% paging allowance and set `Stream`, and `Stream`
    is set exactly when they do exceed it;
  - `RAMNeed` equals the bytes the plan expects resident in RAM (`off` when
    it fits, `b.RAM` when paging), so the plan cannot under- or over-report
    the offload;
  - `NCPUMoE` is a real layer count (`1..Layers`).
  Result: **30,668 hybrid plans checked (1,605 paging), no violation after
  the fix below.** Before the fix this test fails on the same draft bug (the
  planner's `RAMNeed` includes draft bytes the plan drops).
- **`TestMoEHybridIgnoresDraft`** compares each hybrid plan for a MoE model
  that has a draft model (`m.Draft != nil`, CUDA only) with the same model,
  same quant, same machine and the draft removed. A hybrid plan drops
  speculative decoding, so the two plans must be identical in `NCPUMoE`,
  `GPUNeed`, `RAMNeed` and `Stream`. 3,875 plans compared.
- **`TestMoEHybridRealDesktop`** pins the calibrated desktop (RTX 4070
  SUPER 12 GB, 42 GB free RAM) with Flash Next in the catalog's first quant
  at 32K context: the plan must be hybrid and must not move when the draft
  is removed.

Profiles: 4 to 48 GB cards with 8 to 128 GB RAM on the Linux/Windows CUDA
margins, the wider Windows Vulkan margin, two 24 GB cards, a machine whose
free RAM is much lower than its installed RAM, Apple unified memory (no
hybrid there), and processor-only boxes. Realistic VRAM/RAM pairings only.

## Bug found and fixed: hybrid plans budget a draft they then discard

**Severity: major** (wrong RAM/VRAM numbers, one to two extra expert layers
offloaded, and on the calibrated desktop the flagship model was refused
outright). It is conservative, never unsafe.

`try` adds the speculative draft model's bytes to `w` and its working
memory to `oh` before choosing a mode (`internal/fit/fit.go:339-345`), then
for hybrid, split and CPU plans clears `p.Spec` and resets `p.Download` to
the main file only (`fit.go:471-478`). The hybrid split math therefore
counted a 2.2 GB draft (Flash Next) or 0.46 GB (Gemma 4 26B A4B) that the
plan never downloads or loads, inflating `RAMNeed`/`GPUNeed` and pushing
one to two more layers of experts into RAM than needed.

Evidence, failing before the fix:

```
$ git stash  # fix removed
--- FAIL: TestMoEHybridIgnoresDraft (0.00s)
    ... gemma-4-26b-a4b UD-IQ2_XXS ctx 65536: draft 461766816 bytes changes
    a plan that drops it: ncmoe 30 vs 28, VRAM 5880063169 vs 5888865980,
    RAM 9619812318 vs 8579235849, stream false vs false
--- FAIL: TestMoEHybridRealDesktop (0.00s)
    invariant_moe_test.go:292: expected a hybrid plan, got ok=false mode=
```

On the real RTX 4070 SUPER + 62 GB profile, `try` returned **no plan at
all** for `qwen3.8-flash-next GSQ-RCO-Q2_0` at 32K: the phantom 2.2 GB
draft pushed the expert bytes over the 1.1x RAM gate.

Fix (`internal/fit/fit.go:394-402`, 9 lines): in the hybrid branch, when a
spec is present, take the draft's bytes out of `w`, recompute `oh` without
the spec overhead, and recompute `activeBytes` before the split. A second
guard (`fit.go:417-423`) keeps at least one layer in RAM: with the draft
removed the whole model can now fit the card, but this branch was entered on
the draft-inclusive check and a "hybrid" plan with nothing offloaded would
carry the hybrid speed model. After the fix:

```
--- PASS: TestMoEOffloadFitsRAM
    30668 hybrid plans checked (1605 paging), 118 profiles
--- PASS: TestMoEHybridIgnoresDraft
    3875 hybrid plans checked against the same model without its draft
    (255 more fit the card without it)
--- PASS: TestMoEHybridRealDesktop
    qwen3.8-flash-next GSQ-RCO-Q2_0: ncmoe 42/48, VRAM 10.43 GB,
    RAM 37.80 GB, draft 2199652672 bytes dropped
```

`go test -p 2 ./internal/fit/` still passes in full, so no pinned pick in
`fit_test.go`/`profiles_test.go` moved.

## Related finding, not fixed (outside this slice): split plans too

The same overcount exists in the dense **split** branch (`fit.go:446-462`):
it budgets the draft in `w` and then drops `p.Spec`. Measured on the same
4070 SUPER profile with `qwen3.8-27b GSQ-RCO-IQ2_S` (draft 3.16 GB):
`NGL 35, RAMNeed 5.63 GB` with the draft vs `NGL 50, RAMNeed 2.03 GB`
without it, so 15 layers stay off the card for a draft the plan does not
use. The CPU branch has the same shape but needs a CUDA machine with zero
usable VRAM to reach. Same one-line fix applies (drop `draftBytes` and the
spec overhead when the mode drops spec); left out here to keep this patch
inside the MoE slice and to avoid moving dense-model picks another agent
may have pinned. Recommend the lead apply it in the split branch too.

## Open questions

1. `RAMNeed` is `min(ram, b.RAM)` for paging plans (`fit.go:436`), so a
   plan that pages 10% of its experts reports only the resident part. The
   test encodes that as the contract, but if the UI should warn about the
   paged excess, `RAMNeed` should report `ram` and the budget check should
   allow `Stream`.
2. The paging allowance is 1.1x in the code (`fit.go:425`) but the comment
   in `fit_test.go:54` says 1.3x. Which is current? The 1.1x constant is
   what the test asserts.
3. 255 generated cases where the model fits the card without its draft but
   the draft makes the GPU check fail, so the planner picks hybrid instead
   of a GPU plan without speculative decoding. A "retry GPU without spec"
   case before the hybrid branch would recover them; not done here because
   it moves dense-model picks too.

## Gates

`gofmt -l internal/fit/` (empty), `go build ./...`, `go vet ./...`,
`go test -p 2 ./internal/fit/` all pass. No files written at runtime, no
network, no model loaded.
