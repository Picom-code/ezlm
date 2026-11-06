# T04 / a08: monotonicity of the fit planner

Slice: more free memory must never yield a less capable pick. Branch
`agent/T04-a08`, worktree `~/ezlm-agents/work/subs/T04/a08`.

## What I built

`internal/fit/invariant_monotonic_test.go` is a property test over a generated
profile space, not a handful of cases:

- 11 machine shapes with fixed totals: GTX 1650 4 GB + 16 GB, RTX 3060
  12 GB + 32 GB, RTX 4090 24 GB + 96 GB, RTX PRO 6000 96 GB + 256 GB,
  48 GB VRAM (2x RTX 4090) + 128 GB, Mac M1 8 GB, Mac M4 Max 64 GB,
  Mac Studio M3 Ultra 256 GB, processor-only 8 GB, processor-only 64 GB,
  RX 7800 XT 16 GB + 32 GB on Windows Vulkan.
- Three monotone free-memory paths per shape, 48 points each: all memory
  freed together, GPU frees up with RAM fixed, RAM frees up with the GPU
  fixed. That is 1,056 generated points per run.
- `TestMonotonicRandomWalk`: 400 extra random monotone pairs between random
  free-memory values, so a boundary between the grid points cannot hide.
- `TestMonotonicCapability` (the invariant): under `Smartest`, the
  recommendation's `Effective` score (model score after quant, KV and context
  losses, the planner's own "effective intelligence") never falls as free
  memory grows. It also checks that a plan never disappears when memory grows.
- `TestMonotonicUtility`: under `Smartest`, the optimised score never falls.
- `TestMonotonicCPUFallback`: names the CPU fallback bug below and skips
  until it is fixed (a12 owns that invariant).
- `TestMonotonicBalancedUtility`: names the Balanced/Fastest cache-speed bug
  below and skips until it is fixed.

Capability = `Plan.Effective` (`fit.go:305`). Under `Balanced` and `Fastest`
the planner deliberately trades capability for speed, so the strict
capability rule is asserted for `Smartest`; the other preferences are
covered by the two named-bug tests.

## Bugs found

### 1. Major: a newly usable KV cache hides the faster one, so more memory picks a worse model (fixed)

`internal/fit/fit.go:330` (old: `break // a smaller KV type at the same ctx
would only lose quality`). The KV loop breaks at the first cache type that
fits. That is only safe when the resulting plan is usable: when the first
fitting cache is too slow (under 5 tok/s for `Smartest`), a smaller cache
leaves room for more layers on the GPU and can be the only usable plan.

Failing before the fix (`go test -run TestMonotonicCapability`):

```
RTX 3060 12GB + 32GB / all memory freed together / Smartest:
  more free memory gave a less capable pick at gpu free 7.75G, ram free 20.67G
  (was gpu free 7.50G, ram free 20.00G):
    was: qwen3.8-27b GSQ-RCO-IQ2_XS-mtp, ctx 65536, q4_0 KV, split mode, 5.2 tok/s, effective 18.611
    now: ternary-bonsai-2-27b Q2_0, ctx 65536, q8_0 KV, split mode, 6.2 tok/s, effective 17.964
```

At 7.50 G the q4_0 cache was the first usable plan. At 7.75 G the larger
q8_0 cache became feasible but ran at 4.9 tok/s (below the floor), the loop
broke on it, and the planner fell back to a smaller model. Seven such
counterexamples before the fix, zero after (same test file):

```
before: FAIL TestMonotonicCapability, 7 capability drops + 1 "no plan at all"
after:  ok  github.com/picom-code/ezlm/internal/fit
```

Fix: keep the largest cache that fits, but only stop looking once the plan is
actually usable (`fit.go:325-332`). When it is not usable, try the smaller
caches; they can free enough VRAM to put more layers on the GPU. The smallest
counterexample to the old rule was `qwen3.8-27b GSQ-RCO-IQ2_S-mtp` on an
RTX 4090 at 9.5 G to 10.0 G: effective 19.057 to 18.894.

### 2. Major: the CPU fallback disappears as soon as any VRAM is free (not fixed, a12 owns it)

`internal/fit/fit.go:462` (`case !hasGPU`). `try` only considers a
processor-only plan when `b.GPU == 0`. A machine with lots of free RAM and a
little free VRAM therefore loses every plan that would run on the processor,
and can lose every plan at all:

```
RTX PRO 6000 96GB + 256GB, Smartest, GPU free 0G -> 2G:
  was: qwen3.8-flash-next GSQ-RCO-IQ3_XXS, ctx 65536, f16 KV, cpu mode, 9.7 tok/s, effective 26.595
  now: lfm3-1.7b Q4_K_M, ctx 32768, q4_0 KV, gpu mode, 1049.9 tok/s, effective 1.368
```

and on an RTX 4090 24 GB + 96 GB, GPU free 0 G has a plan, GPU free 0.5 G
has none at all. `TestMonotonicCPUFallback` reproduces it and skips with the
counterexample. A second form: on a 32 GB machine, qwen3.8-27b has a CPU plan
with effective 20.0 at GPU free 0.26 G, but only a split plan with effective
18.856 at 8.74 G, because CPU is never a competing placement once a GPU plan
fits. Fixing this means making CPU a candidate placement per model (the
package doc at `fit.go:3-6` already says CPU is one of the placements tried),
which changes many picks; a12 should decide it.

### 3. Minor: Balanced/Fastest utility can fall when a larger cache becomes usable (not fixed)

Same `break`, different effect: a smaller cache can put more layers on the
GPU and be faster, and once a larger cache is usable the planner never
compares them. Example (RTX 3060 12 GB + 32 GB, Balanced, 8.75 G to 9.0 G):
`ternary-bonsai-2-27b Q2_0` goes from 10.1 tok/s (effective 17.730,
utility 15.03) to 8.4 tok/s (effective 17.964, utility 14.90). The pick is
more capable but slower, and the planner's own utility dropped, so it did not
choose the best plan it had. Fixing it means ranking every cache size instead
of keeping the largest that fits. I tried removing the `break` entirely: it
fixes these drops, but it changes the pinned real-machine picks in
`fit_test.go` (RTX 4070 SUPER Balanced moves from qwen3.8-flash-next to
qwen3.8-27b; RTX 3060 Balanced moves from ternary-bonsai-2-27b to
qwen3.8-27b), so it is a planner decision, not a test-only fix.
`TestMonotonicBalancedUtility` names it and skips.

## Verification

- `gofmt -l .` empty, `go vet ./...` clean, `go build ./...` clean.
- `go test -p 2 ./internal/fit/` passes, including the pinned real-machine
  picks.
- Before/after evidence above; the failing run is reproducible by restoring
  the old `break` line at `fit.go:330`.

## Open questions

- Should `Smartest` apply the 2 tok/s processor floor on a machine that has a
  GPU? The comment says the low floor is for processor-only machines, but
  `usable` uses it for every CPU plan. Once a12 makes CPU a candidate
  placement this decides whether a 2.7 tok/s CPU plan can outrank a 5.4 tok/s
  GPU plan.
- Is the "keep the largest KV that fits" rule a deliberate quality floor?
  If yes, the Balanced/Fastest utility drop is expected; if no, rank cache
  sizes by utility.
- `p.Stream` multiplies `Effective` by 0.999 (`fit.go:310`), which makes the
  capability metric depend on speed. It is small, but it means two identical
  models can differ in "effective" score for a speed reason.

## Next steps

1. a12 (or the Lead) decides the CPU placement question; then un-skip
   `TestMonotonicCPUFallback`.
2. If the cache ranking change is accepted, remove the `break` and update the
   pinned picks in `fit_test.go`; un-skip `TestMonotonicBalancedUtility`.
3. a15 can reuse `monoShapes` for the shared profile table if useful.
