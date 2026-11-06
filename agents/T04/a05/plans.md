# a05: processor-only laptops, 8 to 64 GB RAM

Slice: CPU-only laptops with 8 to 64 GB of RAM, no usable GPU. This file
records the profiles, the plan the planner picks for each preference, the
invariants the test asserts, and what the slice found.

Test: `internal/fit/profiles_cpu_test.go`. Run
`go test -p 2 -run CPUOnly ./internal/fit/` (about 0.6 s). Set `SHOW=1` and
run `TestShowCPUPlans` to regenerate the table below.

## Method

Each profile is a `scan.Hardware` built directly, with `Backend: scan.CPU`,
no GPUs, a realistic RAM total and free amount, physical core count and RAM
bandwidth. Budgets come from `ComputeBudgets(hw, hw.RAMAvail)`, the same
call the app makes (`internal/app/app.go:239`). Plans come from
`PlansFor` for `Balanced`, `Smartest` and `Fastest`. The test pins the
model, quant, context and KV type of the recommended plan, and checks the
invariants below for every plan the planner returns, not only the pick.

`Plan.Args()` holds the extra flags; serve always adds `-c <ctx>` and
`-ngl <NGL>` itself (`internal/serve/config.go:122-123`), so a CPU plan is
fully explicit with `-ngl 0`.

## Plan table

`RAM need` is `Plan.RAMNeed / RAMBudget` in GB. `tok/s` is the planner's
estimate. Extra flags are `Plan.Args()` joined.

| Profile (RAM total, free) | Budget GB | Pref | Model | Quant | Ctx | KV | RAM need | tok/s | Extra flags |
|---|---|---|---|---|---|---|---|---|---|
| ThinkPad T480, 8 GB, 4 GB free, 4 cores, DDR4-2400 (34 GB/s) | 3.0 | balanced | granite-4.2-3b | Q3_K_M | 32k | q4_0 | 2.9 / 3.0 | 9.9 | -ctk q4_0 -ctv q4_0 -t 4 |
| | | smartest | granite-4.2-3b | Q3_K_M | 32k | q4_0 | 2.9 / 3.0 | 9.9 | -ctk q4_0 -ctv q4_0 -t 4 |
| | | fastest | granite-4.2-3b | Q3_K_M | 32k | q4_0 | 2.9 / 3.0 | 9.9 | -ctk q4_0 -ctv q4_0 -t 4 |
| Dell XPS 13, 8 GB, 5 GB free, 4 cores, LPDDR4x (40 GB/s) | 4.0 | balanced | granite-4.2-3b | Q4_0 | 64k | q4_0 | 4.0 / 4.0 | 10.1 | -ctk q4_0 -ctv q4_0 -t 4 |
| | | smartest | granite-4.2-3b | Q4_0 | 64k | q4_0 | 4.0 / 4.0 | 10.1 | -ctk q4_0 -ctv q4_0 -t 4 |
| | | fastest | granite-4.2-3b | Q3_K_M | 64k | q4_0 | 3.7 / 4.0 | 11.7 | -ctk q4_0 -ctv q4_0 -t 4 |
| Old 8 GB laptop, 2 GB free, 4 cores (34 GB/s) | 1.0 | balanced | ternary-bonsai-2-2b | Q2_0 | 16k | q8_0 | 1.0 / 1.0 | 33.4 | -ctk q8_0 -ctv q8_0 -t 4 |
| | | smartest | ternary-bonsai-2-2b | Q2_0 | 16k | q8_0 | 1.0 / 1.0 | 33.4 | -ctk q8_0 -ctv q8_0 -t 4 |
| | | fastest | ternary-bonsai-2-2b | Q2_0 | 16k | q8_0 | 1.0 / 1.0 | 33.4 | -ctk q8_0 -ctv q8_0 -t 4 |
| 8 GB laptop thrashing, 1 GB free, 4 cores (34 GB/s) | 0.0 | balanced | nothing fits | | | | | | |
| | | smartest | nothing fits | | | | | | |
| | | fastest | nothing fits | | | | | | |
| Budget laptop, 12 GB, 9 GB free, 6 cores (38 GB/s) | 7.5 | balanced | granite-4.2-3b | Q4_0 | 64k | q8_0 | 5.2 / 7.5 | 9.6 | -ctk q8_0 -ctv q8_0 -t 6 |
| | | smartest | qwen3.8-27b | UD-IQ2_XXS | 16k | q4_0 | 7.5 / 7.5 | 2.8 | -ctk q4_0 -ctv q4_0 -t 6 |
| | | fastest | granite-4.2-3b | Q3_K_M | 64k | f16 | 7.3 / 7.5 | 11.1 | -t 6 |
| Dev laptop, 16 GB, 11 GB free, 8 cores, DDR4 (45 GB/s) | 9.0 | balanced | granite-4.2-3b | Q4_0 | 64k | f16 | 7.6 / 9.0 | 11.3 | -t 8 |
| | | smartest | ternary-bonsai-2-27b | Q2_0 | 64k | q4_0 | 8.9 / 9.0 | 3.2 | -ctk q4_0 -ctv q4_0 -t 8 |
| | | fastest | granite-4.2-3b | Q3_K_M | 64k | f16 | 7.3 / 9.0 | 13.2 | -t 8 |
| Windows 11 laptop, 16 GB, 8 GB free, 8 cores (50 GB/s) | 6.0 | balanced | granite-4.2-3b | Q4_0 | 64k | q8_0 | 5.2 / 6.0 | 12.6 | -ctk q8_0 -ctv q8_0 -t 8 |
| | | smartest | bonsai-27b-1bit | Q1_0 | 64k | q4_0 | 5.3 / 6.0 | 7.1 | -ctk q4_0 -ctv q4_0 -t 8 |
| | | fastest | granite-4.2-3b | Q3_K_M | 64k | q8_0 | 4.9 / 6.0 | 14.6 | -ctk q8_0 -ctv q8_0 -t 8 |
| Fresh Linux laptop, 16 GB, 13 GB free, 8 cores (55 GB/s) | 11.0 | balanced | qwen3.6-35b-a3b | UD-IQ2_XXS | 32k | q8_0 | 10.9 / 11.0 | 32.9 | -ctk q8_0 -ctv q8_0 -t 8 |
| | | smartest | qwen3.8-27b | GSQ-RCO-IQ2_XS | 64k | q8_0 | 10.6 / 11.0 | 3.5 | -ctk q8_0 -ctv q8_0 -t 8 |
| | | fastest | qwen3.6-35b-a3b | UD-IQ2_XXS | 32k | q8_0 | 10.9 / 11.0 | 32.9 | -ctk q8_0 -ctv q8_0 -t 8 |
| Framework 13 AMD, 24 GB, 18 GB free, 12 cores (60 GB/s) | 16.0 | balanced | qwen3.6-35b-a3b | UD-IQ3_S | 64k | f16 | 14.7 / 16.0 | 28.2 | -t 12 |
| | | smartest | qwen3.8-27b | GSQ-RCO-IQ3_S | 64k | f16 | 15.6 / 16.0 | 2.7 | -t 12 |
| | | fastest | qwen3.6-35b-a3b | UD-IQ2_XXS | 64k | f16 | 11.9 / 16.0 | 35.9 | -t 12 |
| DDR5 laptop, 32 GB, 12 GB free, 12 cores, browser and Docker (70 GB/s) | 10.0 | balanced | bonsai-27b-1bit | Q1_0 | 64k | f16 | 8.1 / 10.0 | 9.9 | -t 12 |
| | | smartest | qwen3.8-27b | GSQ-RCO-IQ2_XS | 64k | q4_0 | 9.6 / 10.0 | 4.5 | -ctk q4_0 -ctv q4_0 -t 12 |
| | | fastest | bonsai-27b-1bit | Q1_0 | 64k | f16 | 8.1 / 10.0 | 9.9 | -t 12 |
| DDR5 laptop, 32 GB, 24 GB free, 12 cores (77 GB/s) | 21.6 | balanced | nemotron-3.5-bolt-74b-a9b | NVIDIA-24GB (IQ2_XXS QAD) | 64k | q8_0 | 21.5 / 21.6 | 15.6 | -ctk q8_0 -ctv q8_0 -t 12 |
| | | smartest | qwen3.8-27b | GSQ-RCO-IQ3_S | 64k | f16 | 15.6 / 21.6 | 3.5 | -t 12 |
| | | fastest | qwen3.6-35b-a3b | UD-IQ2_XXS | 64k | f16 | 11.9 / 21.6 | 46.1 | -t 12 |
| Mobile workstation, 32 GB, 28 GB free, 16 cores (70 GB/s) | 25.2 | balanced | nemotron-3.5-bolt-74b-a9b | NVIDIA-24GB (IQ2_XXS QAD) | 64k | f16 | 21.8 / 25.2 | 14.2 | -t 16 |
| | | smartest | qwen3.8-27b | GSQ-RCO-IQ3_S | 64k | f16 | 15.6 / 25.2 | 3.2 | -t 16 |
| | | fastest | qwen3.6-35b-a3b | UD-IQ2_XXS | 64k | f16 | 11.9 / 25.2 | 41.9 | -t 16 |
| Linux workstation, 48 GB, 40 GB free, 16 cores (77 GB/s) | 36.0 | balanced | nemotron-3.5-bolt-74b-a9b | UD-IQ3_XXS | 64k | f16 | 30.8 / 36.0 | 10.9 | -t 16 |
| | | smartest | nanbeige4.3-33b | Q4_K_M | 64k | f16 | 33.1 / 36.0 | 2.0 | -t 16 |
| | | fastest | qwen3.6-35b-a3b | UD-IQ2_XXS | 64k | f16 | 11.9 / 36.0 | 46.1 | -t 16 |
| 64 GB desktop, 18 GB free, 16 cores, many apps (83 GB/s) | 16.0 | balanced | qwen3.6-35b-a3b | UD-IQ3_S | 64k | f16 | 14.7 / 16.0 | 39.0 | -t 16 |
| | | smartest | qwen3.8-27b | GSQ-RCO-IQ3_S | 64k | f16 | 15.6 / 16.0 | 3.8 | -t 16 |
| | | fastest | qwen3.6-35b-a3b | UD-IQ2_XXS | 64k | f16 | 11.9 / 16.0 | 49.6 | -t 16 |
| Creator laptop, 64 GB, 52 GB free, 16 cores, DDR5 (83 GB/s) | 46.8 | balanced | qwen3.8-flash-next | GSQ-RCO-IQ2_XS | 64k | f16 | 44.3 / 46.8 | 15.6 | -lzm on -t 16 |
| | | smartest | qwen3.8-flash-next | GSQ-RCO-IQ2_XS | 64k | f16 | 44.3 / 46.8 | 15.6 | -lzm on -t 16 |
| | | fastest | qwen3.8-flash-next | GSQ-RCO-IQ2_XS | 64k | f16 | 44.3 / 46.8 | 15.6 | -lzm on -t 16 |
| CPU-only server, 64 GB, 60 GB free, 32 cores (100 GB/s) | 54.0 | balanced | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | 64k | f16 | 49.1 / 54.0 | 16.9 | -lzm on -t 32 |
| | | smartest | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | 64k | f16 | 49.1 / 54.0 | 16.9 | -lzm on -t 32 |
| | | fastest | qwen3.8-flash-next | GSQ-RCO-IQ2_XS | 64k | f16 | 44.3 / 54.0 | 18.8 | -lzm on -t 32 |

## Invariants asserted

For every plan returned for every profile and preference
(`TestCPUOnlyInvariants`), and across a 28-machine RAM grid
(`TestCPUOnlyFreeMemoryGrid`):

1. The plan is processor-only: `Mode == CPU`, `NGL == 0`, `GPUNeed == 0`.
2. It fits: `RAMNeed <= RAMBudget` and `RAMBudget == ComputeBudgets(...).RAM`.
3. The budget leaves a margin: `RAM < RAMAvail` whenever RAM is free.
4. The context is one the model supports: `0 < Ctx <= CtxMax`.
5. The KV type is one of f16, q8_0, q4_0, and f16 for models that cannot
   take a quantized cache (`gpt-oss-*`).
6. Threads are the physical core count and appear as `-t` in `Args()`.
7. For non-lazy quants, `RAMNeed` is at least the quant's file size, so the
   plan counts the weights it maps.
8. `Download` is at least the quant's file size.
9. The speed label matches the number (`fast` >= 30, `good` >= 8, `slow`
   below 8).
10. The planner is deterministic: the same input gives the same pick.

`TestCPUOnlyPlanTable` additionally pins the model, quant, context and KV
type per profile and preference, and asserts that the 1 GB-free 8 GB profile
recommends nothing (budget 0).

## Findings

No plan in this slice exceeded its budget, so there is no failing-then-fixed
bug here. Three smaller things are worth the Lead's attention.

1. **minor, scan: processor bandwidth is only measured on macOS.**
   `internal/scan/scan.go:91` initialises `CPUBandwidth: 50` and only
   `internal/scan/scan_darwin.go:51` replaces it. On Windows and Linux every
   processor-only machine is planned at 50 GB/s, so a DDR4-2400 laptop and a
   DDR5-6400 laptop look identical to the planner. The table feeds each
   machine's real bandwidth (34 to 100 GB/s) to exercise the planner's
   range; the profiles marked 50 GB/s are what the app will actually see.
   Effect is speed labels, not fit: slow-RAM laptops are shown as faster
   than they are, fast ones as slower.

2. **minor, open question: CPU plans do not reserve prompt-batch working
   memory.** Hybrid (`fit.go:403`) and Split (`fit.go:440`) add
   `batchOverhead` (1.2 GB, `fit.go:225`) for 2048-token prompt batches; the
   CPU branch (`fit.go:455-460`) adds only `overhead()`, and so does the GPU
   branch (its `full` at `fit.go:381` has no batch term). The comment at
   `fit.go:223-225` ties the constant to modes that pass `-ub 2048`, which
   only Hybrid does (`fit.go:540`), so the CPU omission may be deliberate
   (llama.cpp's default ubatch is smaller). If it is not, the two profiles
   that sit exactly on the budget (Dell XPS 13 at 4.0/4.0 GB, 12 GB laptop
   smartest at 7.5/7.5 GB) would push into the OS floor while reading a long
   prompt. Needs a load test to settle; the README forbids loading models
   here. The same inconsistency is a GPU-slice question.

3. **minor, open question: lazy lookup tables reserve no page cache on CPU.**
   Hybrid adds `q.LazyBytes / 5` to `RAMNeed` for lookup-table rows that
   pass through the page cache (`fit.go:411-414`), but the CPU branch does
   not. The 64 GB / 52 GB profile maps 68 GB of which 22.6 GB is lazy and
   plans 44.3 of 46.8 GB. The pages are clean and reclaimable, so this is a
   speed risk, not an out-of-memory risk, but the headroom is smaller than
   the number suggests.

Boundary cases the table pins on purpose: the Dell XPS 13 balanced plan and
the 12 GB smartest plan sit exactly on the budget; the 48 GB smartest plan
picks nanbeige4.3-33b at exactly the 2.0 tok/s `SmartestMinCPU` floor; and
the 8 GB laptop with 1 GB free has a zero budget and correctly gets no plan.

## Open questions

- Is the 1.2 GB `batchOverhead` meant to apply to CPU plans too (finding 2)?
- Should `ComputeBudgets` treat a failed `RAMAvail` read (0 bytes) as "unknown"
  rather than "no free memory"? Today that yields a zero budget and no plans.
  Not covered here because every profile has a plausible free value.

## Not done

- No fix was needed, so nothing in `internal/fit/` changed.
- Only `./internal/fit/` was run, as the Lead asked; the full repo test suite
  was not run from this worktree.
