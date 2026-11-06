# T04 a14: golden plans per machine profile

Slice: golden files (stretch). Deliverable: a generator plus
`internal/fit/testdata/golden_profiles.json`, so a planner change shows up as a
reviewable diff instead of a quietly different pick.

## What I built

- `internal/fit/golden_test.go`
  - `TestGoldenProfiles` pins, for every profile and every preference
    (balanced, smartest, fastest), the recommended plan: model, quant, context,
    KV type, mode, `-ngl`, `-ncmoe`, spec type, tok/s and the full extra flags
    from `Plan.Args()`.
  - `-update-golden` is the generator:
    `go test ./internal/fit -run TestGoldenProfiles -update-golden`.
  - The file is sorted and written one compact JSON object per line, so an
    intended change touches only the lines it changes.
  - It fails on three things: a changed pick (got/want on one line each), a
    profile that lost its entry, and a stale entry (profile renamed or removed).
    The error prints the exact `-update-golden` command.
  - It also picks up `testdata/profiles.json` (a15's shared table) when it
    exists, in any of three shapes: `{name: hardware}`, `{"profiles": {...}}`,
    or `[{name, hardware}]`. An unrecognised shape is skipped with a log line,
    never a failure, because that file is owned by a15. After a15 merges, run
    the update command once to extend the golden file to the whole team table.
- `internal/fit/testdata/golden_profiles.json`: 23 profiles x 3 preferences =
  69 plans, 14 KB. Deliberately only the decision, not the byte accounting
  (the invariant tests own that), and `null` records "nothing fits".

## Profiles covered

Groups from the brief, with the balanced pick each one gets:

| profile | balanced pick | ctx | kv | mode | tok/s |
|---|---|---|---|---|---|
| nvidia-gtx-1650-4gb | granite-4.2-3b Q3_K_M | 16384 | q4_0 | gpu | 45 |
| nvidia-rtx-3050-laptop-6gb | bonsai-27b-1bit Q1_0 | 32768 | q8_0 | split | 10 |
| nvidia-rtx-4060-8gb | bonsai-27b-1bit Q1_0 | 65536 | q8_0 | split | 14 |
| nvidia-rtx-4070-super-12gb | ternary-bonsai-2-27b Q2_0 | 32768 | f16 | gpu | 43 |
| nvidia-rtx-4090-24gb | qwen3.8-flash-next IQ3_XXS | 65536 | f16 | hybrid | 40 |
| nvidia-rtx-5090-32gb | qwen3.8-flash-next IQ3_XXS | 65536 | f16 | hybrid | 51 |
| nvidia-rtx-4070-super-12gb-busy (4 GB free) | qwen3.6-35b-a3b Q2_K_XL | 32768 | q4_0 | hybrid | 92 |
| nvidia-2x-rtx-3060-12gb | qwen3.8-flash-next IQ3_XXS | 65536 | f16 | hybrid | 82 |
| nvidia-2x-rtx-4090-24gb-one-busy | qwen3.8-flash-next IQ3_XXS | 65536 | f16 | hybrid | 137 |
| nvidia-3x-mixed-3090-4090 | qwen3.8-flash-next IQ3_XXS | 65536 | f16 | gpu | 209 |
| nvidia-8x-h200 | qwen3.8-flash-next IQ3_XXS | 65536 | f16 | gpu | 1071 |
| amd-radeon-rx-7800-xt-16gb | qwen3.8-27b IQ2_XS | 65536 | f16 | gpu | 48 |
| amd-radeon-rx-6600-8gb | bonsai-27b-1bit Q1_0 | 65536 | q8_0 | gpu | 38 |
| intel-arc-a770-16gb | qwen3.8-27b IQ3_XXS | 32768 | f16 | gpu | 36 |
| apple-m1-8gb | granite-4.2-3b Q3_K_M | 32768 | q4_0 | gpu | 24 |
| apple-m3-pro-18gb | bonsai-27b-1bit Q1_0 | 65536 | q8_0 | gpu | 25 |
| apple-m4-max-64gb | qwen3.8-flash-next IQ2_XS | 32768 | q8_0 | gpu | 49 |
| apple-m4-max-128gb | qwen3.8-flash-next IQ3_XXS | 65536 | f16 | gpu | 44 |
| apple-m5-max-128gb | qwen3.8-flash-next IQ3_XXS | 65536 | f16 | gpu | 50 |
| apple-m3-ultra-256gb | qwen3.8-flash-next IQ3_XXS | 65536 | f16 | gpu | 66 |
| cpu-only-16gb | granite-4.2-3b Q4_0 | 65536 | q8_0 | cpu | 13 |
| cpu-only-64gb | qwen3.8-flash-next IQ2_XS | 65536 | f16 | cpu | 11 |
| windows-arm-rtx-spark-96gb-gpu-30gb-ram | qwen3.8-flash-next IQ3_XXS | 65536 | f16 | gpu | 61 |

All 69 smartest/fastest entries are in the golden file next to these.

## Evidence the test bites

Changed one `tok` value in the golden file (38 to 39), ran the test, then
regenerated:

```
--- FAIL: TestGoldenProfiles (0.02s)
    planner picks differ from testdata/golden_profiles.json for 1 of 69 entries.
    amd-radeon-rx-6600-8gb/balanced:
      got  {"model":"bonsai-27b-1bit",...,"tok":38,...}
      want {"model":"bonsai-27b-1bit",...,"tok":39,...}
```

Restored with `-update-golden`: `ok ... 0.214s`. A stale-entry check is the
same mechanism, keyed on the profile name.

## Checks run

- `gofmt -l .` empty.
- `go vet ./internal/fit/` clean.
- `go test -p 2 ./internal/fit/` pass (0.25 s), including the existing
  calibration and real-machine tests.
- Spot check: every plan for every golden profile and preference is within
  `ComputeBudgets` (no `GPUNeed > GPU` or `RAMNeed > RAM`), so the pinned plans
  are not hiding a budget violation. That invariant is a07's to own; this was
  only a sanity check of the pinned rows.

## Findings

No planner bug found by this slice: a golden file pins behaviour, it does not
judge it. Two observations for the Lead, not bugs in my slice:

1. `nvidia-rtx-4070-super-12gb-busy` (12 GB card, 4 GB free) is the only
   profile that picks a 35B MoE hybrid while the same card with 11 GB free
   picks a 27B model at 43 tok/s. That is the intended trade (a busy card
   cannot hold the 27B even split), but it is the first row to look at if a07
   finds a margin problem.
2. Multi-GPU profiles are pinned as one combined VRAM pool, which matches how
   llama.cpp splits layers, but the hybrid speed estimate uses the fastest
   single card's bandwidth, not the sum. Conservative, not a fitting bug.

## Open questions

- a15's `testdata/profiles.json` schema is not written yet. `sharedProfiles`
  accepts the three shapes above; if a15 picks a fourth, add it there. Until
  then the golden file covers only the 23 profiles defined in this test.
- Should tok/s stay in the golden file? It makes calibration tweaks visible,
  which is useful, but it also makes a speed-only change a 69-line diff. I kept
  it; drop the `tok` field if the diff gets too noisy.
