# T04 / a11: explicit flags invariant

**Slice:** explicit `-c`, `-ngl` and offload flags are always set in the plan, never
left to llama-server defaults.
**Worktree:** `~/ezlm-agents/work/subs/T04/a11`, branch `agent/T04-a11`.
**Test file:** `internal/fit/invariant_flags_test.go`.
**Commits:** `98fc11d` (test + fix), plus the commit that adds this report.

## The invariant, as tested

For every plan the planner returns (not only the recommendation), for every
preference, over a generated machine space:

1. `-c` is always set: `Plan.Ctx` is one of the documented ladder values
   (`ctxLadder`), is positive, and never past the model's `CtxMax`.
2. `-ngl` always matches the mode:
   - `GPU` / `Hybrid`: `NGL >= Model.Layers` (999, i.e. every layer on the card);
   - `Split`: `0 < NGL < Model.Layers`;
   - `CPU`: `NGL == 0` (explicit `-ngl 0`, not an omitted flag).
3. The expert-offload flag always matches the mode: `Hybrid` has
   `0 < NCPUMoE <= Layers`; `GPU`, `Split` and `CPU` have `NCPUMoE == 0`.
4. The plan's own `Args()` never repeats a flag that `serve` sets, because
   `serve.Config.Args` appends the recipe after its own flags and a later copy
   would silently win (`-c`, `--ctx-size`, `-ngl`, `--n-gpu-layers`,
   `--n-cpu-moe`, `-np`, `--device`).
5. The effective llama-server command line (built through `serve.Config.Args`,
   the real consumer) has exactly one `-c` with the plan's context, exactly one
   `-ngl` with the plan's value, and `--n-cpu-moe` exactly when the plan
   offloads experts, with the plan's number.
6. A plan whose memory math reserves the 2048-token batch buffer
   (`batchOverhead`, 1.2 GB) also asks for that batch size (`-ub 2048 -b 2048`).

## Generated profile space

50 machines (`flagProfiles()`), deterministic and hermetic (no scan, no disk):

- Apple Silicon M-class, unified memory, 8 to 192 GB, three bandwidth tiers;
- discrete NVIDIA 4, 6, 8, 12, 16, 24, 32 and 48 GB with 16, 32 and 64 GB RAM;
- AMD Vulkan 4 to 24 GB on Windows and Linux, plus an Intel Arc 12 GB;
- two-card machines: 2x12, 2x24, 24+8 and 16+4 GB;
- Windows on Arm with a 96 GB shared-memory GPU and 30 GB RAM;
- CPU-only 4 to 64 GB.

`TestFlagsExplicitOnEveryPlan` checked **3735 plans** over those 50 profiles for
`Balanced`, `Smartest` and `Fastest`. `TestFlagsModeCoverage` proves the space
reaches all four placements, so the invariant is not only exercised on the easy
GPU case: 858 GPU, 186 hybrid, 119 split, 82 CPU plans.

`TestFlagsDegenerateLayerCounts` adds synthetic catalog entries with `layers`
0, 1, 2 and 3 (dense and MoE, 4 GB and 20 GB) to cover metadata that a real
catalog entry can lose.

## Findings

### F1 (minor, fixed): split plans budget 2048-token batches but never ask for them

`try` adds `batchOverhead` (1.2 GB of GPU working memory for 2048-token prompt
batches) to a split plan at `internal/fit/fit.go:442`, with the comment
"2048-token prompt batches, as in hybrid". But `Args()` only emitted
`-ub 2048 -b 2048` for `Hybrid` (`internal/fit/fit.go:539-541` before the fix),
so a split plan reserved 1.2 GB for batches it never requested. The run used
llama-server's default `-ub 512`, so the plan's budget and the actual run
disagreed, and prompt reading was slower than the plan paid for.

Evidence, before the fix (`go test -run TestFlags ./internal/fit/`):

```
--- FAIL: TestFlagsExplicitOnEveryPlan (0.04s)
    RTX 4GB + 16GB RAM/balanced: bonsai-27b-1bit Q1_0/split q8_0: command line has -ub "" x0, want 2048 once (the plan budgets 2048-token batches)
    RTX 4GB + 16GB RAM/balanced: qwen3.5-9b UD-IQ3_XXS/split q4_0: command line has -ub "" x0, want 2048 once ...
```

493 of the 3735 plans failed on `-ub` and `-b`; all were `Split`.

Fix (`internal/fit/fit.go:541`): emit `-ub 2048 -b 2048` for `Split` as well as
`Hybrid`. Flags only; `GPUNeed` already included `batchOverhead`, so nothing
about plan selection changes and the budget now matches the run.

After: `TestFlagsExplicitOnEveryPlan` passes (3735 plans), no other fit test
changed.

### F2 (minor, fixed): a catalog entry without a layer count plans an offload it never sets

With `catalog.Model.Layers == 0` (the JSON field is omitted or lost), `try`
divided by the layer count:

- a MoE model produced `Mode == Hybrid`, `NGL == 999`, **`NCPUMoE == 0`**, so
  `serve.Config.Args` emitted no `--n-cpu-moe` and llama-server put every
  expert on the card, while `Recipe()` said "experts of 0/0 layers in system
  memory". `GPUNeed` came out of `uint64(2*Inf)` arithmetic as a meaningless
  2.93 GB;
- a dense model produced `Mode == Split` with **`NGL == 0`** (`-ngl 0`, a plain
  CPU load) because `onGPU < m.Layers/3` is `0 < 0` when `Layers < 3`.

Evidence, before the fix (`TestFlagsDegenerateLayerCounts`):

```
--- FAIL: TestFlagsDegenerateLayerCounts (0.00s)
    layers=0 moe=false 20GB: synthetic Q4_K_M/split f16: split plan offloads 0 of 0 layers
    layers=0 moe=true 20GB: synthetic Q4_K_M/hybrid f16: hybrid plan offloads 0 of 0 layers' experts
    layers=1 moe=false 20GB: synthetic Q4_K_M/split f16: split plan offloads 0 of 1 layers
    layers=2 moe=false 20GB: synthetic Q4_K_M/split f16: split plan offloads 0 of 2 layers
```

Severity is minor today (every entry in `internal/catalog/catalog.json` has
`layers`, 24 to 88), but the consequence is a plan that says "some layers off
the card" while the flags say "none", which is exactly the failure mode this
invariant exists to catch, and it is one data-entry mistake away.

Fix (`internal/fit/fit.go:391,439,448`): the hybrid and split branches require
`m.Layers > 0` (a model without a layer count can only be fully resident or on
the processor), and the split guard requires `onGPU >= 1` so a "split" plan
always puts at least one layer on the card. A `Layers == 0` model now either
gets the fully-resident GPU plan or no plan at all.

After: `TestFlagsDegenerateLayerCounts` passes; the full fit package passes.

## Checked and found correct

- `serve.Config.Args` (`internal/serve/config.go:115-137`) always sets `-c`,
  `-ngl` and, when `NCPUMoE > 0`, `--n-cpu-moe`, before the recipe flags. Both
  plan consumers copy all three fields: `internal/app/app.go:763-768`
  (deploy) and `internal/app/app.go:1354` (model switch).
- `-ngl 999` is at least the layer count of every catalog model (max 88), so it
  is explicit "all layers". The test asserts `NGL >= Layers` rather than the
  literal 999.
- No quant `args` in the catalog repeat a flag `serve` sets (all five `args`
  keys are empty today); the test now keeps it that way.
- GPU plans do not pass `-ub`/`-b`, and their budget has no `batchOverhead`:
  llama-server's smaller default is safe there.
- CPU-only plans with `PhysCores == 0` omit `-t`; llama-server's own default
  is the physical core count, so no memory consequence. Left as is.

## Open questions

- `NGL == 999` is a magic "all layers" sentinel. It works (llama.cpp clamps),
  but `p.Model.Layers` would be self-documenting. Changing it touches
  `serve.Config` consumers and `Recipe()`, so it was left alone.
- `Plan.Args()` deliberately omits `-c`/`-ngl`; `serve` owns them. A future
  caller that builds a `serve.Config` without copying `Plan.Ctx`/`NGL` would
  silently fall back to llama-server defaults. There is no such caller today,
  but nothing in the type system enforces it.

## Not finished

Nothing in this slice. The wider `internal/fit` fuzz/golden work belongs to
other agents; this file stays independent of their test files and testdata.
