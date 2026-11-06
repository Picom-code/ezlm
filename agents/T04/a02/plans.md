# T04 / a02: multiple GPUs (2x and 3x, mixed sizes, mixed free memory)

Test file: `internal/fit/profiles_multi_test.go` (15 profiles). One fix in
`internal/fit/fit.go` (multi-GPU bandwidth), with the failing test above it.

All numbers below are `ComputeBudgets(hw, hw.RAMAvail)` followed by
`PlansFor(..., Balanced)`. Budgets are GB = GiB. Test file pins the Balanced
model, quant, mode and context for every profile, plus the safety invariants.

## Profiles and asserted plans

| # | Machine (cards: free/total GB, bandwidth) | GPU budget | Balanced plan | ctx | mode | ngl / ncmoe | GPU need | RAM need | tok/s |
|---|---|---|---|---|---|---|---|---|---|
| 1 | 2x RTX 3090 24GB, NVLink, 64GB DDR4 (23+23, 936) | 44.2 | qwen3.8-flash-next GSQ-RCO-IQ3_XXS | 64k | hybrid | 999 / 11 | 44.0 | 15.7 | 95.6 |
| 2 | 2x RTX 4090 24GB, 128GB DDR5 (23+23, 1008) | 44.2 | qwen3.8-flash-next GSQ-RCO-IQ3_XXS | 64k | hybrid | 999 / 11 | 44.0 | 15.7 | 97.6 |
| 3 | 2x RTX 5090 32GB, 128GB DDR5 (31+31, 1792) | 59.5 | qwen3.8-flash-next GSQ-RCO-IQ3_XXS | 64k | gpu | 999 / 0 | 51.7 | 0.0 | 399.7 |
| 4 | 2x RTX 3060 12GB, 32GB DDR4 (11+11, 360) | 21.1 | qwen3.8-27b GSQ-RCO-IQ3_XXS-mtp | 64k | gpu | 999 / 0 | 14.9 | 0.0 | 33.3 |
| 5 | 2x RTX 3080 10GB, 32GB DDR4 (9+9, 760) | 17.3 | qwen3.8-27b GSQ-RCO-IQ3_S-mtp | 64k | gpu | 999 / 0 | 16.5 | 0.0 | 60.6 |
| 6 | 3x RTX 3090 24GB, 128GB DDR4 (23+23+23, 936) | 66.2 | qwen3.8-flash-next GSQ-RCO-IQ3_XXS | 64k | gpu | 999 / 0 | 51.7 | 0.0 | 208.8 |
| 7 | 3x RTX 4090 24GB, 256GB DDR5 (23+23+23, 1008) | 66.2 | qwen3.8-flash-next GSQ-RCO-IQ3_XXS | 64k | gpu | 999 / 0 | 51.7 | 0.0 | 224.8 |
| 8 | 2x RTX PRO 6000 96GB, 256GB DDR5 (94+94, 1792) | 180.5 | qwen3.8-flash-next GSQ-RCO-IQ3_XXS | 64k | gpu | 999 / 0 | 51.7 | 0.0 | 399.7 |
| 9 | 2x H100 80GB, 512GB DDR5 (78+78, 3350) | 149.8 | qwen3.8-flash-next GSQ-RCO-IQ3_XXS | 64k | gpu | 999 / 0 | 51.7 | 0.0 | 747.2 |
| 10 | 2x RTX A6000 48GB, 128GB DDR4 (46+46, 768) | 88.3 | qwen3.8-flash-next GSQ-RCO-IQ3_XXS | 64k | gpu | 999 / 0 | 51.7 | 0.0 | 171.3 |
| 11 | Mixed: RTX 4090 24GB + RTX 3060 12GB, 64GB (23+11) | 32.6 | qwen3.8-flash-next GSQ-RCO-IQ3_XXS | 64k | hybrid | 999 / 23 | 32.1 | 27.5 | 52.0 |
| 12 | Mixed free: 2x 24GB, second card busy (23+8, 1008) | 29.8 | qwen3.8-flash-next GSQ-RCO-IQ3_XXS | 64k | hybrid | 999 / 26 | 29.2 | 30.5 | 50.5 |
| 13 | Mixed free: RTX 4090 + RTX 3090 (23+6, 1008/936) | 27.8 | qwen3.8-flash-next GSQ-RCO-IQ3_XXS | 64k | hybrid | 999 / 28 | 27.2 | 32.5 | 47.3 |
| 14 | 3x mixed: 24GB + 12GB + 8GB (22+11+7, 1008/360/272) | 38.4 | qwen3.8-flash-next GSQ-RCO-IQ3_XXS | 64k | hybrid | 999 / 17 | 38.1 | 21.6 | 60.5 |
| 15 | 2x GTX 1080 Ti 11GB, SLI bridge, 32GB (10+10, 484) | 19.2 | qwen3.8-27b GSQ-RCO-IQ3_S-mtp | 64k | gpu | 999 / 0 | 16.5 | 0.0 | 38.6 |

Every plan is `f16` KV and 65536 context. Smartest differs from Balanced only
on profile 4 (qwen3.8-27b GSQ-RCO-IQ3_S-mtp, 16.5 GB, 28.7 tok/s). Fastest
swaps to the IQ2_XS / IQ2_XS-mtp quant everywhere and drops the 51.7 GB model
where the pool is small (profiles 12 and 13 pick qwen3.8-27b at 13.3 GB).

## What the tests assert

1. **Budget is pooled, not one card.** `b.GPU` equals sum(free VRAM) less the
   Linux margin (4%, at least 0.4 GB), and is strictly greater than the
   largest free card. A regression to `max()` or a per-card margin fails.
2. **Budget follows free memory, not total.** Profile 12 (23+8 GB free of
   2x24 GB) gets 29.8 GB, not the 44.2 GB an idle pair would get. The RAM
   budget is free RAM less 10% (at least the OS floor).
3. **Safety margin.** For every plan of every profile, `GPUNeed <= b.GPU` and
   `RAMNeed <= b.RAM`. A machine with 44.2 GB of budget is never planned for
   a 51.7 GB fully-resident load (profiles 1 and 2 correctly go hybrid).
4. **Pooling is used.** On the 13 profiles whose pick is larger than their
   biggest card, the plan's `GPUNeed` exceeds that card's free VRAM, so it
   can only work by spanning cards (e.g. 2x3060: 14.9 GB on two 11 GB cards).
   Profiles 8 and 9 are the exceptions: the catalog's largest model (51.7 GB)
   fits on one 94/78 GB card, so no plan needs the second card.
5. **Monotonic in per-card free memory.** Sweeping the busy card of profile 12
   from 2 to 23 GB free never lowers the pick's effective score.
6. **A third card helps.** 3x3090 has a larger budget than 2x3090 and the
   pick is never less capable (both pick flash-next; 2x is hybrid, 3x fits it
   fully on the cards).
7. **Smartest is never worse than Balanced** on any of the 15 machines.

## Bug found and fixed: a slow second card was free speed

`fit.go` took `gpuBW = max(bandwidth)` over all cards. Layer split runs every
token through every card in turn, so bandwidths add in series: the effective
bandwidth is the free-memory-weighted harmonic mean. Equal cards come out at
one card's bandwidth (correct: a second identical card buys memory, not
speed), but a mixed pair was credited with the fast card's bandwidth.

Failing test `TestMultiGPUMixedCardsCostSpeed`: same model, quant and context
(qwen3.8-27b GSQ-RCO-IQ3_S-mtp, GPU mode) on a single RTX 4090 and on
4090+3060:

```
profiles_multi_test.go:248: 4090+3060 claims 80.4 tok/s, the single 4090 80.4: the slower card is free
```

After the fix in `internal/fit/fit.go` (the `gpuBW` block in `try`) the mixed
machine is slower, as it must be. The fix is exact for one card and for equal
cards, so no other pinned number moved: the whole `internal/fit` package,
including `TestCalibration` and `TestShowLadder`, still passes. On the 15
profiles the only speeds that changed are the mixed-bandwidth ones
(4090+3060: 55.9 -> 52.0, 3x mixed: 71.0 -> 60.5, 4090+3090: 47.4 -> 47.3).

## Open questions

- No plan ever uses more than ~52 GB because that is the catalog's largest
  model, so 2x96 GB and 2x80 GB machines cannot show whether very large pools
  stay safe. Not a planner bug.
- The planner has no notion of `--tensor-split` or split mode; it relies on
  llama.cpp's default split by free memory. That default is what makes the
  summed budget correct. If EZLM ever sets a split mode or tensor split
  itself, the budget rule here must be revisited.
- Per-card runtime overhead (CUDA context, roughly 0.3 GB per card) is
  counted once in `overhead`, not once per card. On 3x small cards the 4%
  pooled margin (about 1.5 GB) covers it today, but a 4x 8 GB machine would
  be tight. No failing test yet: it needs a model that lands within ~1 GB of
  the budget.
