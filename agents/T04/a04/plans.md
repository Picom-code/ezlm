# a04: Apple Silicon, M1 to M5, 8 GB to 192 GB unified memory

Test file: `internal/fit/profiles_apple_test.go`.
Run: `go test -p 2 -run TestApple ./internal/fit/`.

Eighteen `scan.Hardware` values built the way `internal/scan/scan_darwin.go`
reports them: `Backend: metal`, `Unified: true`, `GPUUsable` at the wired
limit (2/3 of RAM up to 36 GB, 3/4 above), `RAMAvail` from `memory_pressure`,
and the bandwidth from `appleBandwidth` for that chip tier. Free memory is
what a real machine shows after login with its usual apps open, not total RAM.

## Plans the planner produces (default preference: balanced)

Budget is the usable memory after the margin, `min(wired limit, free) - max(10%, OS floor)`.
Flags are `Plan.Args()`; `-c` and `-ngl` are added by serve, not by the plan.

| Profile | RAM / free GB | Budget GB | Model | Quant | Ctx | KV | GPU need GB | tok/s | Flags |
|---|---|---|---|---|---|---|---|---|---|
| M1 8GB, 5GB free | 8 / 5 | 4.0 | granite-4.2-3b | Q4_0 | 65536 | q4_0 | 4.0 | 20.6 | -ctk q4_0 -ctv q4_0 |
| M1 8GB, 3GB free (Xcode open) | 8 / 3 | 2.0 | ternary-bonsai-2-4b | Q2_0 | 32768 | q4_0 | 1.9 | 36.1 | -ctk q4_0 -ctv q4_0 |
| M1 16GB, 10GB free | 16 / 10 | 8.0 | bonsai-27b-1bit | Q1_0 | 65536 | q8_0 | 6.3 | 11.5 | -ctk q8_0 -ctv q8_0 |
| M1 Pro 16GB, 12GB free | 16 / 12 | 8.7 | qwen3.8-27b | UD-IQ2_XXS | 65536 | q4_0 | 8.5 | 17.7 | -ctk q4_0 -ctv q4_0 |
| M2 16GB, 11GB free | 16 / 11 | 8.7 | qwen3.8-27b | UD-IQ2_XXS | 65536 | q4_0 | 8.5 | 8.9 | -ctk q4_0 -ctv q4_0 |
| M2 Pro 16GB, 12GB free | 16 / 12 | 8.7 | qwen3.8-27b | UD-IQ2_XXS | 65536 | q4_0 | 8.5 | 17.7 | -ctk q4_0 -ctv q4_0 |
| M3 24GB, 18GB free | 24 / 18 | 14.1 | ternary-bonsai-2-27b | Q2_0 | 65536 | f16 | 11.7 | 8.4 | none |
| M3 Pro 36GB, 28GB free | 36 / 28 | 21.7 | qwen3.8-27b | GSQ-RCO-IQ3_XXS | 65536 | f16 | 14.1 | 9.6 | none |
| M4 16GB, 12GB free | 16 / 12 | 8.7 | qwen3.8-27b | UD-IQ2_XXS | 65536 | q4_0 | 8.5 | 10.6 | -ctk q4_0 -ctv q4_0 |
| M4 Pro 24GB, 18GB free | 24 / 18 | 14.1 | qwen3.8-27b | GSQ-RCO-IQ3_XXS | 65536 | f16 | 14.1 | 17.4 | none |
| M4 Max 64GB, 50GB free | 64 / 50 | 43.2 | qwen3.8-flash-next | GSQ-RCO-IQ2_XS | 32768 | q8_0 | 43.1 | 49.1 | -ctk q8_0 -ctv q8_0 -lzm on |
| M4 Max 128GB, 110GB free | 128 / 110 | 86.4 | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | 65536 | f16 | 73.4 | 15.6 | none |
| M4 Max 128GB, 40GB free (apps open) | 128 / 40 | 36.0 | qwen3.8-27b | GSQ-RCO-IQ3_XXS | 65536 | f16 | 14.1 | 34.8 | none |
| M5 16GB, 12GB free | 16 / 12 | 8.7 | qwen3.8-27b | UD-IQ2_XXS | 65536 | q4_0 | 8.5 | 13.6 | -ctk q4_0 -ctv q4_0 |
| M5 Pro 32GB, 24GB free | 32 / 24 | 19.3 | qwen3.8-27b | GSQ-RCO-IQ3_XXS | 65536 | f16 | 14.1 | 19.6 | none |
| M5 Max 128GB, 110GB free | 128 / 110 | 86.4 | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | 65536 | f16 | 73.4 | 17.5 | none |
| M2 Ultra 192GB, 170GB free | 192 / 170 | 129.6 | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | 65536 | f16 | 73.4 | 22.8 | none |
| M3 Ultra 192GB, 165GB free | 192 / 165 | 129.6 | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | 65536 | f16 | 73.4 | 23.4 | none |

Non-default preferences pinned in `TestApplePreferencesPinned`: M1 8GB fastest
= granite-4.2-3b Q3_K_M, M3 24GB fastest = qwen3.6-35b-a3b UD-IQ2_XXS,
M4 Pro 24GB smartest = qwen3.8-27b GSQ-RCO-IQ3_S, M4 Max 128GB (40GB free)
smartest = nanbeige4.3-33b Q6_K_S, M5 Max 128GB fastest = qwen3.8-27b UD-IQ2_XXS.

## Invariants asserted

`TestAppleProfiles`, for all 18 profiles, on every plan the planner returns
(not only the recommendation):

1. A plan exists, and its mode is `gpu` with `ngl=999`, `n_cpu_moe=0` and
   `RAMNeed=0`. Unified memory means there is no second pool to offload
   layers or experts to, so hybrid, split and CPU plans must never appear.
2. `GPUNeed <= GPU budget` and `RAMNeed <= RAM budget`.
3. The budget is `min(wired limit, free) - max(10%, OS floor)` and never above
   the wired limit or free memory (`TestAppleBudgetFormula`).
4. The plan leaves the OS floor free: `free - GPUNeed >= osFloor`
   (1 GB on an 8 GB Mac, 2 GB from 16 GB up).
5. The plan records the budget it was planned against, and on a unified
   machine `GPUBudget == RAMBudget`.
6. No speculative decoding, no direct load, no `-t`: those are CUDA/Windows
   recipes. `Download` is exactly the quant's bytes (no draft file).
7. The context is a ladder rung the model supports, and the quant is at or
   above the model's quality floor (single-file models excepted).
8. `Effective` and `TokPerSec` are positive, and `Speed` matches the
   documented 30/8 tok/s thresholds.
9. More usable memory never yields a less capable plan
   (`TestAppleMoreFreeMemoryIsBetter`, walking the ladder by budget).
10. `Args()` carries a matched `-ctk`/`-ctv` pair when the cache is
    quantized, `-lzm on` exactly when the plan is lazy, and nothing else.

The recommended plan is also pinned (model, quant, context, KV type) on all
18 profiles, so any ranking change shows up as a diff here.

## Finding 1 (fixed): lookup tables paged from SSD when the whole file fits

`internal/fit/fit.go:335-338` subtracts `LazyBytes` from the resident
weights whenever a quant has them, with no check that the machine needs the
saving. `-lzm on` then makes every token read those tables back from the SSD.

Before the fix, `TestAppleLazyOnlyWhenNeeded` failed on four profiles:

```
M4 Max 128GB, 110GB free: flash-next GSQ-RCO-IQ3_XXS pages 24.0 GB of lookup
tables from the SSD, but the whole file needs 73.4 GB of the 86.4 GB budget
M5 Max 128GB, 110GB free: same
M2 Ultra 192GB, 170GB free: same
M3 Ultra 192GB, 165GB free: same
```

The M2 Ultra left 80 GB of its 129.6 GB budget unused while reading 24 GB of
tables from disk on every token.

Fix (12 lines in `try`, `internal/fit/fit.go:343-357`): when the whole file
plus KV cache and overhead fits the budget the plan is made against, keep the
tables resident and drop lazy mode; otherwise fall back to lazy as before.
After the fix all 18 Apple plans are resident except the M4 Max 64GB, where
43.1 GB of 43.2 GB is all the room there is and paging 21.5 GB is genuinely
required.

Side effect, recorded because it is large: the estimated speed of a resident
`flash-next` on a big Mac drops (M2 Ultra 64.6 -> 22.8 tok/s, M4 Max 128GB
44.1 -> 15.6 tok/s). That is the existing sparsity model in `try`
(`activeFrac` is a share of resident weights, so a smaller resident file
looked less sparse and faster). The new numbers are closer to the measured
27.4 tok/s for a fully resident flash-next on an M3 Max (catalog research,
`calib_test.go`), so the change also removes an optimistic estimate. All
pre-existing tests in `internal/fit` still pass.

## Observation 2: big Macs are limited by the catalog, not by memory

A 192 GB Mac has a 129.6 GB budget but the largest `flash-next` file is
70.6 GB (73.4 GB resident with a 64K f16 cache), so 56 GB of the budget goes
unused. Nothing in the catalog scores higher: `nemotron-3.5-super-vl-120b-a12b`
fits at Q4_0 (67 GB) but scores 13.6 against flash-next's 27.0. Not a planner
bug; it is a gap in the catalog for large Apple machines.

## Observation 3: a lowered wired limit hides models the CPU could still run

`ComputeBudgets` returns `Budgets{GPU: g, RAM: g}` on unified memory
(`internal/fit/fit.go:174-180`), and `try` only reaches the CPU branch when
`hasGPU` is false. On Apple, `hasGPU` is true whenever the budget is above
zero, so a user who lowers `iogpu.wired_limit_mb` (say 24 GB on a 64 GB Mac)
gets small plans only, even though a 27B would fit in RAM on the CPU. The
planner is not wrong (nothing overcommits), it just has no CPU fallback on
Metal. Left alone; changing it would change ranking for every Apple machine.

## Open questions

- The `-lzm on` flag is EZLM-specific. If it is not actually honoured by the
  installed llama.cpp, the lazy plans on the M4 Max 64GB would load 63.4 GB
  into a 43.2 GB budget. Worth a check in serve, outside this slice.
- Should the M4 Max 64GB plan (43.1 of 43.2 GB, 32K context) be pushed to a
  smaller quant for a roomier context? It is within the documented margin, so
  the tests allow it, but it is the tightest plan in the table.
