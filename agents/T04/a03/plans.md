# a03 plans: AMD Radeon and Intel Arc through Vulkan

Every row is a profile in `internal/fit/profiles_vulkan_test.go`
(`TestVulkanPlans` pins the balanced plan, `TestVulkanInvariants` checks the
budgets and flags under all three preferences). Budgets are what
`ComputeBudgets` allows after the safety margin. `gpu`/`ram` are the plan's
own needs. "shared" marks a built-in GPU whose memory the driver reports out
of system RAM.

## Balanced (the asserted plan)

| # | Profile | OS | GPU free/total | RAM free/total | Budget gpu/ram | Model | Quant | Ctx | KV | Mode | ngl / ncmoe | gpu / ram used | tok/s | Flags |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | Radeon RX 6500 XT | win | 3/4 | 10/16 | 1.5 / 8.0 | lfm3-1.7b | Q4_K_M | 16k | q4_0 | gpu | 999 / 0 | 1.5 / 0 | 84.4 | `-ctk q4_0 -ctv q4_0 -lm none` |
| 2 | Radeon RX 6600 | win | 7/8 | 24/32 | 5.5 / 21.6 | nemotron-3.5-bolt-74b-a9b | NVIDIA-24GB (IQ2_XXS QAD) | 64k | f16 | hybrid | 999 / 82 of 88 | 5.4 / 18.0 | 34.2 | `-t 8 -ub 2048 -b 2048` |
| 3 | Radeon RX 6700 XT | win | 11/12 | 9/16 | 9.5 / 7.0 | qwen3.8-27b | UD-IQ2_XXS | 32k | f16 | gpu | 999 / 0 | 9.3 / 0 | 28.4 | `-lm none` |
| 4 | Radeon RX 7800 XT | win | 15/16 | 24/32 | 13.2 / 21.6 | qwen3.8-27b | GSQ-RCO-IQ2_XS | 64k | f16 | gpu | 999 / 0 | 12.5 / 0 | 47.7 | `-lm none` |
| 5 | Radeon RX 7900 XTX | win | 22/24 | 48/64 | 19.4 / 43.2 | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | 64k | f16 | hybrid | 999 / 35 of 48 | 19.1 / 37.9 | 53.0 | `-lzm on -t 8 -ub 2048 -b 2048` |
| 6 | RX 7900 XTX, 6 GB held by a game | win | 17/24 | 40/64 | 15.0 / 36.0 | qwen3.8-flash-next | GSQ-RCO-IQ2_XS | 64k | f16 | hybrid | 999 / 38 of 48 | 14.9 / 36.0 | 50.1 | `-lzm on -t 8 -ub 2048 -b 2048` |
| 7 | Radeon 780M (shared) | win | 8/8 | 20/32 | 6.5 / 18.0 | nemotron-3.5-bolt-74b-a9b | NVIDIA-24GB (IQ2_XXS QAD) | 64k | f16 | hybrid | 999 / 78 of 88 | 6.3 / 17.1 | 27.3 | `-t 8 -ub 2048 -b 2048` |
| 8 | Radeon RX 6800 | linux | 15/16 | 26/32 | 14.4 / 23.4 | qwen3.8-27b | GSQ-RCO-IQ3_XXS | 64k | f16 | gpu | 999 / 0 | 14.1 / 0 | 32.7 | none |
| 9 | Radeon RX 7600 | linux | 7/8 | 12/16 | 6.6 / 10.0 | bonsai-27b-1bit | Q1_0 | 64k | q8_0 | gpu | 999 / 0 | 6.3 / 0 | 48.8 | `-ctk q8_0 -ctv q8_0` |
| 10 | Arc A380 | win | 5/6 | 10/16 | 3.5 / 8.0 | bonsai-27b-1bit | Q1_0 | 32k | q4_0 | split | 23 of 64 / 0 | 3.5 / 2.3 | 8.6 | `-ctk q4_0 -ctv q4_0 -t 8` |
| 11 | Arc A750 | win | 7/8 | 24/32 | 5.5 / 21.6 | nemotron-3.5-bolt-74b-a9b | NVIDIA-24GB (IQ2_XXS QAD) | 64k | f16 | hybrid | 999 / 82 of 88 | 5.4 / 18.0 | 37.9 | `-t 8 -ub 2048 -b 2048` |
| 12 | Arc A770 | win | 15/16 | 48/64 | 13.2 / 43.2 | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | 64k | f16 | hybrid | 999 / 42 of 48 | 12.5 / 43.2 | 35.9 | `-lzm on -t 8 -ub 2048 -b 2048` |
| 13 | Arc B580 | win | 11/12 | 20/32 | 9.5 / 18.0 | qwen3.8-27b | UD-IQ2_XXS | 32k | f16 | gpu | 999 / 0 | 9.3 / 0 | 40.4 | `-lm none` |
| 14 | Arc Graphics (shared) | win | 8/8 | 20/32 | 6.5 / 18.0 | nemotron-3.5-bolt-74b-a9b | NVIDIA-24GB (IQ2_XXS QAD) | 64k | f16 | hybrid | 999 / 78 of 88 | 6.3 / 17.1 | 30.2 | `-t 8 -ub 2048 -b 2048` |
| 15 | Arc A770 | linux | 15/16 | 56/64 | 14.4 / 50.4 | qwen3.8-flash-next | GSQ-RCO-IQ3_XXS | 64k | f16 | hybrid | 999 / 40 of 48 | 14.3 / 42.7 | 44.9 | `-lzm on -t 8 -ub 2048 -b 2048` |

Every row respects `gpu used <= budget` and `ram used <= budget`; no plan
exceeds its budget in any of the three preferences.

## Smartest and Fastest, in short

| # | Smartest | Fastest |
|---|---|---|
| 1 | lfm3-1.7b Q4_K_M, gpu 16k | lfm3-1.7b Q4_K_M, gpu 16k |
| 2 | ternary-bonsai-2-27b Q2_0, split 64k (23/64) | qwen3.6-35b-a3b UD-IQ2_XXS, hybrid 64k (34/40) |
| 3 | ternary-bonsai-2-27b Q2_0, split 64k (33/64) | qwen3.6-35b-a3b UD-Q2_K_XL, hybrid 64k (21/40) |
| 4 | qwen3.8-27b GSQ-RCO-IQ3_S, split 64k (42/64) | qwen3.8-27b UD-IQ2_XXS, gpu 64k |
| 5 | qwen3.8-flash-next GSQ-RCO-IQ3_XXS, hybrid 64k (35/48) | qwen3.8-flash-next GSQ-RCO-IQ2_XS, hybrid 64k (33/48) |
| 6 | qwen3.8-flash-next GSQ-RCO-IQ2_XS, hybrid 64k (38/48) | same, hybrid 32k (37/48) |
| 7 | qwen3.8-27b UD-IQ2_XXS, split 8k (41/64) | qwen3.6-35b-a3b UD-IQ2_XXS, hybrid 64k (30/40) |
| 8 | qwen3.8-27b GSQ-RCO-IQ3_S, split 64k (49/64) | qwen3.8-27b UD-IQ2_XXS, gpu 64k |
| 9 | qwen3.8-27b UD-IQ2_XXS, split 32k (27/64) | qwen3.6-35b-a3b UD-IQ2_XXS, hybrid 64k (30/40) |
| 10 | bonsai-27b-1bit Q1_0, split 32k (23/64) | granite-4.2-3b Q3_K_M, gpu 32k |
| 11 | ternary-bonsai-2-27b Q2_0, split 64k (23/64) | qwen3.6-35b-a3b UD-IQ2_XXS, hybrid 64k (34/40) |
| 12 | qwen3.8-flash-next GSQ-RCO-IQ3_XXS, hybrid 64k (42/48) | qwen3.8-flash-next GSQ-RCO-IQ2_XS, hybrid 64k (40/48) |
| 13 | qwen3.8-27b GSQ-RCO-IQ2_XS, split 64k (30/64) | qwen3.6-35b-a3b UD-IQ3_S, hybrid 64k (23/40) |
| 14 | qwen3.8-27b UD-IQ2_XXS, split 32k (26/64) | qwen3.6-35b-a3b UD-IQ2_XXS, hybrid 64k (30/40) |
| 15 | qwen3.8-flash-next GSQ-RCO-IQ3_XXS, hybrid 64k (40/48) | qwen3.8-flash-next GSQ-RCO-IQ2_XS, hybrid 64k (39/48) |

## What the table shows

- The 12%/1.5 GB Windows Vulkan cushion costs real capacity on small cards. A
  4 GB RX 6500 XT has 3 GB free and a 1.5 GB budget, so the balanced pick is
  a 1.7B model at 16K context; nothing 3B-class fits, even with the processor
  helping. The same card under the 4% Linux cushion would get 2.9 GB.
- 8 GB cards lean on MoE expert offload: the 74B Nemotron (12% active) is
  picked over the 27B dense models on the RX 6600 and Arc A750, with 82 of 88
  expert layers in RAM. The plan keeps 3.6 GB of the RAM budget spare.
- 16 GB cards and up go hybrid on Qwen3.8 Flash Next at 64K context, and the
  plan is within 0.5 GB of both budgets. The Arc A770 16 GB row lands at
  exactly 43.2/43.2 GB of RAM, the tightest fit in the table.
- Vulkan never gets speculative decoding (`specFor` returns "" unless CUDA),
  so no draft download or draft context is charged; `TestVulkanNoSpeculativeDecoding`
  pins that.
- `-lm none` (direct load) is set only on Windows GPU-resident plans, and
  `-t <phys cores>` only when weights are on the processor. Both are pinned.
- Linux Vulkan plans carry no `-lm none` and keep the tighter 4%/0.4 GB
  margin, which is safe because a VRAM overflow fails the load there.

## Open question

On a built-in GPU (Radeon 780M, Arc Graphics) the driver's "free memory" is a
slice of system RAM, but `scan` leaves `Unified` false for it, so the planner
treats the two pools as separate. Row 7 commits 6.3 GB of GPU plus 17.1 GB of
RAM against 20 GB of free RAM. That fits if the 8 GB is a BIOS carve-out that
`RAMAvail` already excludes, and overcommits by 3.4 GB if the share is
dynamic. `scan` only sets `Unified` on Apple Silicon, and the Windows adapter
scan does not say which kind of share an iGPU has, so `internal/fit` cannot
tell. A `GPU.Shared` flag in `scan` would settle it.
