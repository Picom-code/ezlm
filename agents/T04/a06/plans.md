# a06: Windows on Arm with a large shared-memory GPU

Slice: Windows on Arm where the GPU addresses far more memory than Windows
sees (RTX Spark N1X class: 96 GB for the GPU, 30 GB for Windows), and where
another app already holds part of the GPU, so free VRAM is well below total.

All profiles are built directly as `scan.Hardware` values (no parsing) and
pinned in `internal/fit/profiles_edge_test.go` (`TestWindowsOnArmPlans`).
Budgets are `ComputeBudgets(hw, hw.RAMAvail)`.

Reproduce this table:

```sh
SHOW=1 go test -run TestShowWindowsOnArmPlans -v ./internal/fit/
```

## Budgets

| Profile | GPU free | GPU budget | RAM free | RAM budget |
|---|---|---|---|---|
| N1X idle | 90 GB | 82.8 GB (8% cushion) | 20 GB | 18.0 GB (2 GB OS floor) |
| N1X game resident | 56 GB | 51.5 GB | 18 GB | 16.0 GB |
| N1X 50 GB free | 50 GB | 46.0 GB | 20 GB | 18.0 GB |
| N1X 40 GB free | 40 GB | 36.8 GB | 18 GB | 16.0 GB |
| N1X 28 GB free | 28 GB | 25.8 GB | 20 GB | 18.0 GB |
| N1X 12 GB free | 12 GB | 11.0 GB (1 GB floor) | 20 GB | 18.0 GB |
| N1X 8 GB free | 8 GB | 7.0 GB | 20 GB | 18.0 GB |
| N1X 4 GB free | 4 GB | 3.0 GB | 20 GB | 18.0 GB |
| N1X 0 GB free | 0 GB | 0 GB | 20 GB | 18.0 GB |
| Adreno 15 of 16 GB free | 15 GB | 13.2 GB (12% cushion) | 20 GB | 18.0 GB |
| Adreno 24 of 32 GB free | 24 GB | 21.1 GB (12% cushion) | 12 GB | 10.0 GB |

## Balanced plan per profile

| Profile | Model | Quant | Ctx | Mode | Flags | GPU need / budget | RAM need / budget | est tok/s |
|---|---|---|---|---|---|---|---|---|
| N1X idle, 90 GB GPU, 20 GB RAM free | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | 64k | GPU | `-lzm on --spec-type draft-mtp --spec-draft-n-max 2` | 51.7 / 82.8 | 0 / 18.0 | 60.9 |
| N1X, game holds 34 GB (56 free) | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | 64k | Hybrid, 4/48 expert layers in RAM | `-lzm on -t 12 -ub 2048 -b 2048` | 50.9 / 51.5 | 8.8 / 16.0 | 71.2 |
| N1X, 50 GB free (just under GPU-resident) | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | 64k | Hybrid, 9/48 | `-lzm on -t 12 -ub 2048 -b 2048` | 46.0 / 46.0 | 13.7 / 18.0 | 59.9 |
| N1X, 3D app holds 56 GB (40 free) | qwen3.8-flash-next | GSQ-RCO-IQ2_XS | 64k | Hybrid, 14/48, experts paged | `-ctk q8_0 -ctv q8_0 -lzm on -t 12 -ub 2048 -b 2048` | 36.6 / 36.8 | 16.0 / 16.0 | 43.1 |
| N1X, another LLM holds 68 GB (28 free) | qwen3.8-27b | GSQ-RCO-IQ3_XXS-mtp | 64k | GPU | `-lm none --spec-type draft-mtp --spec-draft-n-max 3` | 14.9 / 25.8 | 0 / 18.0 | 25.3 |
| N1X, 20 GB free | qwen3.8-27b | GSQ-RCO-IQ3_XXS-mtp | 64k | GPU | `-lm none --spec-type draft-mtp --spec-draft-n-max 2` | 14.9 / 18.4 | 0 / 18.0 | 25.3 |
| N1X, 12 GB free | ternary-bonsai-2-27b | Q2_0 | 32k | GPU | `-lm none` | 9.6 / 11.0 | 0 / 18.0 | 23.1 |
| N1X, 8 GB free (game resident) | nemotron-3.5-bolt-74b-a9b | NVIDIA-24GB (IQ2_XXS QAD) | 64k | Hybrid, 78/88 | `-t 12 -ub 2048 -b 2048` | 7.0 / 7.0 | 17.9 / 18.0 | 34.9 |
| N1X, 4 GB free | qwen3.6-35b-a3b | UD-Q2_K_XL | 32k | Hybrid, 40/40 | `-ctk q4_0 -ctv q4_0 -t 12 -ub 2048 -b 2048` | 2.9 / 3.0 | 11.0 / 18.0 | 86.7 |
| N1X, GPU fully held (0 free) | qwen3.6-35b-a3b | UD-IQ3_S | 64k | CPU | `-t 12` | 0 / 0 | 14.7 / 18.0 | 28.2 |
| N1X idle GPU, RAM squeezed to 6 GB | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | 64k | GPU | `-lzm on --spec-type draft-mtp --spec-draft-n-max 2` | 51.7 / 82.8 | 0 / 4.0 | 60.9 |
| N1X docked, 64 GB RAM (48 free) | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | 64k | GPU | `-lzm on --spec-type draft-mtp --spec-draft-n-max 2` | 51.7 / 82.8 | 0 / 43.2 | 60.9 |
| Snapdragon X Elite 32 GB, 15 of 16 GB free | qwen3.8-27b | GSQ-RCO-IQ2_XS | 64k | GPU | `-lm none` | 12.5 / 13.2 | 0 / 18.0 | 20.9 |
| Snapdragon X Elite 64 GB, 24 of 32 GB free | qwen3.8-27b | GSQ-RCO-IQ3_XXS | 64k | GPU | `-lm none` | 14.1 / 21.1 | 0 / 10.0 | 17.4 |

## Smartest and Fastest, where they differ from Balanced

| Profile | Smartest | Fastest |
|---|---|---|
| N1X idle | flash-next IQ3_XXS, GPU (same as Balanced) | flash-next IQ2_XS, GPU, 46.9 GB, 67.6 tok/s |
| N1X 56 GB free | flash-next IQ3_XXS hybrid 4/48 | same |
| N1X 50 GB free | flash-next IQ3_XXS hybrid 9/48 | flash-next IQ2_XS hybrid 5/48, 76.1 tok/s |
| N1X 40 GB free | flash-next IQ2_XS hybrid 14/48 | flash-next Q2_0 hybrid 13/48, 32k, 60.3 tok/s |
| N1X 28 GB free | qwen3.8-27b IQ3_S-mtp, GPU, 16.5 GB | qwen3.6-35b-a3b UD-IQ4_XS (MTP), GPU, 19.4 GB, 133 tok/s |
| N1X 20 GB free | qwen3.8-27b IQ3_S-mtp, GPU | qwen3.6-35b-a3b UD-IQ4_XS, hybrid 6/40 |
| N1X 12 GB free | qwen3.8-27b IQ2_S-mtp, split 33/64, 5.0 tok/s | qwen3.6-35b-a3b UD-IQ3_S, hybrid 18/40, 124.6 tok/s |
| N1X 8 GB free | ternary-bonsai-2-27b Q2_0, split 27/64, 5.7 tok/s | qwen3.6-35b-a3b UD-IQ2_XXS, hybrid 28/40 |
| N1X 4 GB free | qwen3.6-35b-a3b UD-Q2_K_XL hybrid 40/40 | qwen3.6-35b-a3b UD-IQ2_XXS hybrid 40/40, q8_0 |
| N1X 0 GB free | qwen3.8-27b IQ3_S-mtp, CPU, 2.7 tok/s | qwen3.6-35b-a3b UD-IQ2_XXS, CPU, 35.9 tok/s |
| Snapdragon 32 GB, 15 GB free | qwen3.8-27b IQ3_S, split 42/64, 5.0 tok/s | qwen3.6-35b-a3b UD-IQ3_S, hybrid 11/30, 153.9 tok/s |
| Snapdragon 64 GB, 24 GB free | qwen3.8-27b IQ3_S, GPU, 15.6 GB | qwen3.6-35b-a3b UD-IQ4_XS, GPU, 18.5 GB, 118.9 tok/s |

## What the profiles prove

1. The Windows GPU cushion is taken from **free** VRAM, not total, so a GPU
   another app partly occupies shrinks the budget and the plan moves to a
   smaller quant, then to hybrid expert offload, then to a smaller model, then
   to the processor. Every step stays inside the budget.
2. On a shared-memory GPU the GPU pool can be larger than all of RAM (82.8 GB
   budget vs 30 GB installed). A GPU-resident plan therefore must not also
   claim RAM for the same weights; `RAMNeed` is 0 for every GPU-mode plan,
   which the test asserts.
3. Hybrid plans are the only ones that hold both pools at once. On this
   machine they are bounded by the 18 GB RAM budget and the code pages
   experts from SSD when they exceed it (40 GB profile: `stream` true).
4. Windows flag handling holds: a GPU-resident plan is either direct-loaded
   (`-lm none`) or lazy (`-lzm on`); a hybrid/split/CPU plan is never direct
   loaded, and the flags match the plan fields.
5. When another app holds all VRAM the planner falls back to the processor
   and still sets `-t` to the physical core count.

## Finding: the flagship GPU plan on this machine cannot direct-load

`qwen3.8-flash-next` is the only model big enough for the 96 GB GPU, and all
five of its quant files carry lazy lookup tables (`lazy_bytes` 21-24 GB in
`internal/catalog/catalog.json`). `fit.go:388` sets `DirectLoad` only when
`!lazy`, so the plan the RTX Spark N1X actually gets is
`-lzm on` **without** `-lm none`. That means llama.cpp still memory-maps a
70.6 GB file on a machine with 20 GB of free RAM, which is the exact
situation the `DirectLoad` comment says crashed Windows on an RTX Spark
laptop (`fit.go:43-47`). Whether `-lm none` and `-lzm on` can be combined is
a llama.cpp question this test cannot settle, so the behavior is pinned, not
changed. Recorded as an open question in `REPORT.md`.

## Finding: partial GPU occupancy can turn a GPU plan into a slower hybrid

At 56 GB free the Balanced pick is hybrid (4/48 expert layers in RAM) and
the estimate is *higher* than the fully GPU-resident plan at 90 GB free
(71.2 vs 60.9 tok/s) because the hybrid speed model reads the small active
expert slice from RAM instead of streaming it from VRAM. That is plausible
for a 6B-active MoE, but it means the estimator prefers a partly-offloaded
plan over a GPU-resident one when the card gets tight. Pinned in the table
so the Lead can compare it against real measurements.
