# T04 a09: context fallback order

**Slice:** the chosen context falls back in the documented order (64K, 32K,
16K, 8K), is one the model supports (`ctx_max`) and the caller requires
(`Budgets.MinCtx`), and is one the machine can hold after the safety margin.

**Deliverable:** `internal/fit/invariant_context_test.go` (3 passing property
tests, 1 skipped test that proves a bug). No production change.

## What the tests prove

`TestContextLadderIsDocumentedOrder` pins the ladder itself: exactly the four
rungs `{65536, 32768, 16384, 8192}`, largest first, with a strictly falling
quality factor (`internal/fit/fit.go:89-92`). A reorder or an accidental
smallest-first loop fails this test.

`TestContextChosenIsSupportedAndHoldable` generates 118 machine profiles (see
below) and checks every plan returned by `PlansFor` for all three preferences
(8181 plans):

- the context is one of the ladder rungs (not an arbitrary number);
- `ctx <= model.CtxMax` when the model states one (`fit.go:309`);
- `ctx >= Budgets.MinCtx` (Hermes's 64K floor, `fit.go:85,309`);
- `GPUNeed <= Budgets.GPU` and `RAMNeed <= Budgets.RAM`, i.e. the plan is
  inside the budget after the documented margin (`fit.go:382-464`).

`TestContextFallsBackInOrder` checks the order rather than just the set. For
plans that live fully on the GPU (`Mode == GPU`) or fully on the processor
(`Mode == CPU`) speed does not depend on the context, so the plan must use the
largest rung that fits at the same quant and KV cache type. A larger rung that
still fits in the same mode means the planner fell back too far. Hybrid and
split plans are excluded on purpose: their speed depends on the context, so a
smaller context can legitimately win (measured example: a 64K hybrid plan
loses to a 32K GPU plan on a 6 GB card).

Profile space (`ctxProfiles`), all built from `scan.Hardware` values:

- CPU only, 4 to 256 GB RAM, Linux;
- CUDA, 2 to 94 GB free VRAM crossed with 8 to 128 GB RAM (skewed to keep
  RAM >= VRAM), laptop and desktop bandwidths;
- Vulkan on Linux and Windows, 4 to 24 GB, so the different Windows margin
  (`fit.go:186-193`) is exercised;
- Apple unified memory, 8 to 512 GB, at 20%, 50% and 90% free;
- three representative machines with `MinCtx` = 8K, 32K and 64K.

Runtime is about 0.6 s for the whole file.

## Finding 1 (major, not fixed): a quantized KV cache that would put the model on the GPU is never considered

`bestForModel` tries KV cache types in quality order (f16, q8_0, q4_0) and
stops at the first one that fits (`internal/fit/fit.go:312-326`). The comment
justifies this with "a smaller KV type at the same ctx would only lose
quality". That is not true: the smaller cache frees memory and can move the
plan from split/hybrid (slow) to fully on the GPU (fast).

Counterexample, proven by the skipped test (remove the `t.Skip` to see it):

```
RTX 4070 SUPER class profile (11 GB free VRAM, 24 GB free RAM), Balanced
nanbeige4.3-33b IQ2_XXS at 8K:
  chosen   KV f16  -> split,  8.9 tok/s, utility 8.99
  skipped  KV q4_0 -> gpu,   32.2 tok/s, utility 10.34
```

3.6x the speed for a 1.5% quality loss (`kvQuality["q4_0"] = 0.985`,
`fit.go:167`), and 15% better utility by the planner's own ranking. The same
pattern shows up on other profiles, e.g. `nanbeige4.3-33b IQ2_XXS` at 16K:
f16 split 7.4 tok/s vs q4_0 GPU 32.2 tok/s.

Smallest fix: delete the `break` at `fit.go:325` so the ranking decides
between every KV type that fits. Verified locally: the package tests pass
except two picks pinned to real runs in `fit_test.go:42-47`:

- RTX 4070 SUPER 12GB + 62GB, Balanced: `qwen3.8-flash-next` -> `qwen3.8-27b`
- RTX 3060 12GB + 32GB, Balanced: `ternary-bonsai-2-27b` -> `qwen3.8-27b`

Both changes are plausible improvements (a 27B fully on the card at 32K), but
they contradict picks "checked against real-world runs", and this agent cannot
load a model to check them. The test is therefore `t.Skip`ped with the
counterexample, per the brief. **Owner of the fix should run the 4070S and
3060 profiles on real hardware, then delete the break.**

`internal/fit/fit_test.go` and `agents/T04/a14`'s golden file would need to be
updated with the fix; mention this to the Lead so the goldens are regenerated
after the merge, not before.

## Finding 2 (minor, open question): a caller minimum above 64K gets no plan at all

The ladder tops out at 65536 (`fit.go:92`) and every rung below
`Budgets.MinCtx` is skipped (`fit.go:309`). `internal/app/app.go:534,569,1336`
pass the running server's `cfg.Ctx` as `MinCtx`. A config with a context above
64K (hand-edited, or a future tool that asks for more) therefore gets
`PlanForQuant` -> `ok == false` on every machine, even a 512 GB Mac that can
hold 128K.

Evidence (probe, not committed): `Budgets{MinCtx: 131072}` on an RTX 4070
SUPER profile gives `len(PlansFor(...)) == 0`; the same model at `MinCtx: 0`
plans 64K fine.

A model whose `CtxMax` is below 8192 (old 2K/4K models) likewise gets no plan,
because the ladder has no rung under 8K. The beta catalog has no such model
(`ctx_max` is >= 65536 for all 32 entries), so this is latent, not live.

Possible fix if the Lead wants it: when `MinCtx` exceeds the ladder top, add a
rung at `MinCtx` (the app already rounds requested contexts up to a power of
two in `ctxFor`, `internal/app/connect.go:197-202`). Not done here because it
changes the documented ladder shape that `TestContextLadderIsDocumentedOrder`
pins, and no current caller asks for more than 64K.

## Not in scope, noticed while reading

- `bestForModel`'s `break` also means only the first KV type that fits is
  ranked; the fix above covers it.
- `ctxLadder` factors (1, 0.94, 0.85, 0.6) are a quality model, not a memory
  model; the memory checks above are independent of them.

## Verification

```
gofmt -l .                          # empty
go build ./...                      # ok
go vet ./...                        # ok
go test -p 2 ./internal/fit/        # ok (0.4 s)
```

Commit: `T04 a09: prove context ladder order, support and memory fit`.
