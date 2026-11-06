# T04 / a15 report: shared hardware profile table

**Slice:** the team's shared profile table: `internal/fit/testdata/profiles.json`
with 60+ realistic machines in one canonical form, and
`internal/fit/profiles_test.go` loading it and proving every profile plans.

**Status:** complete. Commit `519480b` (code + table). Test run:
`go test -p 2 ./internal/fit/` passes.

## What I built

### `internal/fit/testdata/profiles.json`

74 profiles, one JSON object per machine, grouped by the six families from the
brief:

| family | count | covers |
|---|---|---|
| nvidia | 15 | 4 to 96 GB cards, laptop and desktop, Windows and Linux |
| multi-gpu | 10 | 2x/3x/8x, mixed sizes, mixed free memory, NVLink, one busy card |
| vulkan | 10 | AMD Radeon and Intel Arc, Windows and Linux |
| apple | 19 | M1 to M5, 8 to 512 GB unified, wired-limit `gpu_usable_gb` |
| cpu | 8 | 8 to 1536 GB RAM, 4 to 96 physical cores |
| edge | 12 | Windows on Arm 96 GB GPU / 30 GB RAM, occupied GPUs, low disk, no GPU, low free RAM |

Canonical form (documented by the file's `note` and the loader): every memory
value is an integer in GiB (`scan.GB` is `1 << 30`, so GiB, not GB), and each
profile carries exactly the fields the planner reads:

```json
{
  "name": "...", "family": "...", "os": "...", "arch": "...",
  "backend": "cuda|vulkan|metal|cpu", "unified": false,
  "ram_total_gb": 64, "ram_avail_gb": 50, "disk_free_gb": 1000,
  "cpu_bandwidth_gbs": 60, "phys_cores": 16, "gpu_usable_gb": 23,
  "gpus": [{"name": "...", "vendor": "...", "mem_total_gb": 24,
             "mem_free_gb": 23, "bandwidth_gbs": 1008}]
}
```

`gpu_usable_gb` is the sum of what the GPU may use (for discrete cards the
free VRAM, for Apple the wired limit). It is data, not derived, so a profile
can model a GPU partly occupied by another app by lowering `mem_free_gb` and
`gpu_usable_gb` independently of `mem_total_gb`.

### `internal/fit/profiles_test.go`

Added, keeping the existing `realMachines`/`ladder` helpers so other agents
that reference them still compile:

- `hwProfile` / `hwProfile.Hardware()`: table row to `*scan.Hardware`.
- `loadHwProfiles(t)`: reads and validates the table version.
- `TestProfilesTable`: at least 60 profiles, unique names, all six families
  present, `ram_avail <= ram_total`, free VRAM `<=` total, known backend,
  unified only on Metal, CPU backend has no GPUs.
- `TestProfilesPlan`: for every profile and every preference (Balanced,
  Smartest, Fastest) a plan exists, `GPUNeed <= GPU budget`,
  `RAMNeed <= RAM budget`, model/quant/mode/context set, context within the
  model's maximum, and `-ngl` explicit (999 for GPU and hybrid plans).
  74 profiles x 3 preferences = 222 plans, all pass.
- `TestShowProfiles` (gated on `SHOW=1`) prints the review table below:
  `SHOW=1 go test -run ShowProfiles -v ./internal/fit/`.

**Reuse by other agents:** after merge, any `_test.go` in package `fit` can
call `loadHwProfiles(t)` and `prof.Hardware()`; the names are deliberately
specific to avoid collisions.

## Invariants proved

1. Every one of the 74 profiles plans successfully for all three preferences.
2. No plan exceeds the post-margin GPU or RAM budget (`TestProfilesPlan`).
3. Every plan names a quant, a mode and a context, and the context is one the
   model supports.
4. GPU/hybrid plans always carry an explicit `-ngl` value (999), never a
   llama-server default.

## Plans table (Balanced, from `SHOW=1`)

| profile | family | model | quant | mode | ctx | kv | speed | tok/s | need |
|---|---|---|---|---|---|---|---|---|---|
| GTX 1650 4 GB desktop | nvidia | granite-4.2-3b | Q3_K_M | gpu | 16k | q4_0 | fast | 44.9 | 2.5 |
| GTX 1650 4 GB laptop | nvidia | lfm3-1.7b | Q4_K_M | gpu | 64k | q4_0 | fast | 65.6 | 1.8 |
| RTX 3050 6 GB laptop | nvidia | bonsai-27b-1bit | Q1_0 | split | 32k | q8_0 | good | 10.1 | 6.2 |
| RTX 4060 8 GB desktop | nvidia | bonsai-27b-1bit | Q1_0 | gpu | 64k | q8_0 | fast | 46.1 | 6.3 |
| RTX 4060 8 GB laptop | nvidia | nemotron-3.5-bolt-74b-a9b | NVIDIA-24GB | (IQ2_XXS | QAD) | hybrid | 64k | 33.1 | 24.8 |
| RTX 3070 8 GB desktop | nvidia | nemotron-3.5-bolt-74b-a9b | NVIDIA-24GB | (IQ2_XXS | QAD) | hybrid | 64k | 36.5 | 24.8 |
| RTX 3060 12 GB desktop | nvidia | ternary-bonsai-2-27b | Q2_0 | gpu | 32k | f16 | fast | 30.4 | 9.6 |
| RTX 4070 12 GB desktop | nvidia | ternary-bonsai-2-27b | Q2_0 | gpu | 32k | f16 | fast | 42.6 | 9.6 |
| RTX 4070 SUPER 12 GB desktop | nvidia | qwen3.8-flash-next | GSQ-RCO-Q2_0 | hybrid | 16k | q4_0 | good | 22.8 | 48.3 |
| RTX 4080 16 GB desktop | nvidia | qwen3.8-27b | GSQ-RCO-IQ2_XS-mtp | gpu | 64k | f16 | fast | 79.0 | 13.3 |
| RTX 4060 Ti 16 GB desktop | nvidia | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | hybrid | 64k | f16 | good | 28.8 | 59.3 |
| RTX 3090 24 GB desktop | nvidia | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | hybrid | 64k | f16 | fast | 39.9 | 59.7 |
| RTX 4090 24 GB desktop | nvidia | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | hybrid | 64k | f16 | fast | 40.1 | 59.7 |
| RTX 5090 32 GB desktop | nvidia | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | hybrid | 64k | f16 | fast | 51.2 | 59.7 |
| RTX PRO 6000 96 GB workstation | nvidia | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | gpu | 64k | f16 | fast | 399.7 | 51.7 |
| 2x RTX 3060 12 GB | multi-gpu | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | hybrid | 64k | f16 | fast | 34.7 | 59.7 |
| 2x RTX 4090 24 GB | multi-gpu | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | hybrid | 64k | f16 | fast | 137.7 | 59.7 |
| 3x RTX 3090 24 GB | multi-gpu | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | gpu | 64k | f16 | fast | 208.8 | 51.7 |
| 2x RTX PRO 6000 96 GB | multi-gpu | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | gpu | 64k | f16 | fast | 399.7 | 51.7 |
| RTX 4090 24 GB + RTX 3060 12 GB mixed | multi-gpu | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | hybrid | 64k | f16 | fast | 53.9 | 59.7 |
| 2x RTX 5090 32 GB | multi-gpu | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | gpu | 64k | f16 | fast | 399.7 | 51.7 |
| 8x H200 141 GB server | multi-gpu | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | gpu | 64k | f16 | fast | 1070.7 | 51.7 |
| 2x RTX 2080 Ti 11 GB NVLink | multi-gpu | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | hybrid | 64k | f16 | fast | 35.8 | 59.7 |
| 3x mixed 24/16/8 GB | multi-gpu | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | hybrid | 64k | f16 | fast | 91.9 | 59.7 |
| 2x A100 80 GB | multi-gpu | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | gpu | 64k | f16 | fast | 454.8 | 51.7 |
| Radeon RX 6600 8 GB | vulkan | qwen3.6-35b-a3b | UD-Q2_K_XL | hybrid | 64k | q4_0 | fast | 55.3 | 13.4 |
| Radeon RX 6700 XT 12 GB | vulkan | qwen3.8-27b | UD-IQ2_XXS | gpu | 32k | f16 | fast | 34.0 | 9.3 |
| Radeon RX 7800 XT 16 GB | vulkan | qwen3.8-27b | GSQ-RCO-IQ2_XS | gpu | 64k | f16 | fast | 47.7 | 12.5 |
| Radeon RX 7900 XTX 24 GB Windows | vulkan | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | hybrid | 64k | f16 | fast | 41.7 | 57.0 |
| Radeon RX 7900 XTX 24 GB Linux | vulkan | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | hybrid | 64k | f16 | fast | 44.0 | 57.0 |
| Radeon RX 6800 XT 16 GB Linux | vulkan | qwen3.8-27b | GSQ-RCO-IQ3_XXS | gpu | 64k | f16 | fast | 32.7 | 14.1 |
| Arc A380 6 GB | vulkan | bonsai-27b-1bit | Q1_0 | split | 32k | q4_0 | good | 10.1 | 5.7 |
| Arc A770 16 GB | vulkan | qwen3.8-27b | GSQ-RCO-IQ2_XS | gpu | 64k | f16 | fast | 42.8 | 12.5 |
| Arc B580 12 GB | vulkan | qwen3.8-27b | GSQ-RCO-IQ2_XS | gpu | 32k | f16 | fast | 34.9 | 10.4 |
| Radeon RX 7600 8 GB Linux | vulkan | bonsai-27b-1bit | Q1_0 | gpu | 64k | q8_0 | fast | 48.8 | 6.3 |
| MacBook Air M1 8 GB | apple | granite-4.2-3b | Q3_K_M | gpu | 32k | q4_0 | good | 23.9 | 2.9 |
| MacBook Air M1 16 GB | apple | bonsai-27b-1bit | Q1_0 | gpu | 64k | q8_0 | good | 11.5 | 6.3 |
| MacBook Pro M1 Pro 16 GB | apple | qwen3.8-27b | UD-IQ2_XXS | gpu | 32k | q4_0 | good | 17.7 | 7.8 |
| Mac mini M2 8 GB | apple | granite-4.2-3b | Q4_0 | gpu | 64k | q4_0 | fast | 30.3 | 4.0 |
| MacBook Air M2 16 GB | apple | qwen3.8-27b | UD-IQ2_XXS | gpu | 32k | q4_0 | good | 8.9 | 7.8 |
| MacBook Pro M2 Pro 32 GB | apple | qwen3.8-27b | GSQ-RCO-IQ3_XXS | gpu | 64k | f16 | good | 12.8 | 14.1 |
| MacBook Air M3 16 GB | apple | qwen3.8-27b | UD-IQ2_XXS | gpu | 32k | q4_0 | good | 8.9 | 7.8 |
| MacBook Pro M3 Pro 18 GB | apple | qwen3.8-27b | GSQ-RCO-IQ2_XS | gpu | 64k | q4_0 | good | 11.5 | 9.6 |
| MacBook Pro M3 Pro 36 GB | apple | qwen3.8-27b | GSQ-RCO-IQ3_XXS | gpu | 64k | f16 | good | 9.6 | 14.1 |
| MacBook Pro M3 Max 64 GB | apple | qwen3.8-flash-next | GSQ-RCO-IQ2_XS | gpu | 32k | q8_0 | fast | 36.0 | 43.1 |
| MacBook Air M4 24 GB | apple | qwen3.8-27b | GSQ-RCO-IQ2_XS | gpu | 64k | f16 | good | 9.2 | 12.5 |
| MacBook Pro M4 Pro 48 GB | apple | qwen3.8-27b | GSQ-RCO-IQ3_XXS | gpu | 64k | f16 | good | 17.4 | 14.1 |
| MacBook Pro M4 Max 64 GB | apple | qwen3.8-flash-next | GSQ-RCO-IQ2_XS | gpu | 32k | q8_0 | fast | 49.1 | 43.1 |
| MacBook Pro M4 Max 128 GB | apple | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | gpu | 64k | f16 | fast | 44.1 | 49.1 |
| MacBook Pro M5 32 GB | apple | qwen3.8-27b | GSQ-RCO-IQ3_XXS | gpu | 64k | f16 | good | 9.6 | 14.1 |
| MacBook Pro M5 Max 64 GB | apple | qwen3.8-flash-next | GSQ-RCO-IQ2_XS | gpu | 32k | q8_0 | fast | 54.0 | 43.1 |
| Mac Studio M3 Ultra 192 GB | apple | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | gpu | 64k | f16 | fast | 66.1 | 49.1 |
| Mac Studio M3 Ultra 256 GB | apple | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | gpu | 64k | f16 | fast | 66.1 | 49.1 |
| Mac Studio M3 Ultra 512 GB | apple | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | gpu | 64k | f16 | fast | 66.1 | 49.1 |
| Laptop 8 GB, processor only | cpu | granite-4.2-3b | Q3_K_M | cpu | 32k | q4_0 | good | 9.9 | 2.9 |
| Laptop 12 GB, processor only | cpu | granite-4.2-3b | Q4_0 | cpu | 64k | q4_0 | good | 10.1 | 4.0 |
| Laptop 16 GB, processor only | cpu | granite-4.2-3b | Q4_0 | cpu | 64k | q8_0 | good | 12.6 | 5.2 |
| Laptop 24 GB, processor only | cpu | qwen3.6-35b-a3b | UD-IQ2_XXS | cpu | 64k | f16 | fast | 32.9 | 11.9 |
| Desktop 32 GB, processor only | cpu | qwen3.6-35b-a3b | UD-IQ3_S | cpu | 64k | f16 | good | 28.2 | 14.7 |
| Desktop 48 GB, processor only | cpu | nemotron-3.5-bolt-74b-a9b | UD-IQ3_XXS | cpu | 64k | f16 | good | 9.9 | 30.8 |
| Desktop 64 GB, processor only | cpu | qwen3.8-flash-next | GSQ-RCO-IQ2_XS | cpu | 32k | q8_0 | good | 15.0 | 43.1 |
| EPYC server 1.5 TB, processor only | cpu | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | cpu | 64k | f16 | fast | 77.6 | 49.1 |
| Windows on Arm, 96 GB shared GPU, 30 GB RAM | edge | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | gpu | 64k | f16 | fast | 60.9 | 51.7 |
| Windows on Arm, 96 GB shared GPU, 12 GB free RAM | edge | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | gpu | 64k | f16 | fast | 60.9 | 51.7 |
| RTX 4090 24 GB, 6 GB free after a game | edge | nemotron-3.5-bolt-74b-a9b | NVIDIA-24GB | (IQ2_XXS | QAD) | hybrid | 64k | 35.8 | 24.8 |
| RTX 3060 12 GB, 2 GB free after apps | edge | ternary-bonsai-2-2b | Q2_0 | gpu | 16k | q8_0 | fast | 424.2 | 1.0 |
| RTX 5090 32 GB, 10 GB free after a game | edge | qwen3.8-flash-next | GSQ-RCO-IQ2_XS | hybrid | 64k | f16 | good | 26.2 | 52.0 |
| RTX 3080 10 GB, 3 GB free after apps | edge | lfm3-1.7b | Q4_K_M | gpu | 64k | q4_0 | fast | 445.3 | 1.8 |
| MacBook Pro M4 Max 64 GB, 12 GB free | edge | qwen3.8-27b | GSQ-RCO-IQ2_XS | gpu | 64k | q4_0 | fast | 41.8 | 9.6 |
| Mac mini M2 8 GB, 3 GB free | edge | ternary-bonsai-2-4b | Q2_0 | gpu | 32k | q4_0 | fast | 53.2 | 1.9 |
| RTX 4070 12 GB with 5 GB disk free | edge | qwen3.8-flash-next | GSQ-RCO-Q2_0 | hybrid | 16k | q4_0 | good | 22.8 | 48.3 |
| Gaming laptop 32 GB, no GPU detected | edge | qwen3.6-35b-a3b | UD-IQ3_S | cpu | 64k | f16 | good | 28.2 | 14.7 |
| 2 GPUs, second one busy: 24+8 GB, 3 GB free | edge | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | hybrid | 64k | f16 | fast | 39.1 | 59.7 |
| Mac Studio M3 Ultra 192 GB, 100 GB free | edge | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | gpu | 64k | f16 | fast | 66.1 | 49.1 |

## Observations for the Lead (not fixed here; other slices own them)

- **Windows on Arm shared memory (edge, a06):** the two 96 GB GPU / 30 GB RAM
  profiles plan `qwen3.8-flash-next` fully on the GPU at 51.7 GB, even with
  only 12 GB free system RAM. `ComputeBudgets` treats a non-unified GPU as a
  separate pool, so it only checks the 90 GB `GPUUsable`. If that 96 GB really
  comes out of the same 30 GB, the plan overcommits physical memory. This
  matches the machine already pinned in `fit_test.go` (`realMachines`), so it
  is intended behavior or a pre-existing gap, not something this table
  introduced. a06 should confirm with a failing test.
- **Multi-GPU speed (a02):** several multi-GPU profiles plan the same
  `TokPerSec` as one fast card (e.g. 2x/3x/8x flash-next all ~400 tok/s) while
  memory is summed. Speed uses `max(GPU bandwidth)`, so more cards add
  capacity, not speed. Worth a dedicated monotonicity/capacity test in a02.
- **Low disk:** the 5 GB-free profile still recommends a 48 GB download with
  `DiskOK=false`, consistent with `Recommend` treating disk as a warning. Good.

## Open questions

- Should `profiles.json` also carry expected picks (golden)? a14 can layer a
  `golden_profiles.json` on top keyed by `name` without changing this schema.
- The table is capped at 74 profiles; adding more is a data-only change and
  `TestProfilesTable` keeps the families honest.

## Could not finish / next steps

- Nothing in this slice is unfinished.
- Before the morning merge, run `go test -p 2 ./internal/fit/` after combining
  other agents' files; if two agents both define a `profile` type, rename to
  the `hwProfile` names used here.
