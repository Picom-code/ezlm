# T04 a03: AMD Radeon and Intel Arc through Vulkan

Slice: hardware group a03. Deliverable: `internal/fit/profiles_vulkan_test.go`
(15 realistic Vulkan profiles with asserted plans and invariants) plus
`agents/T04/a03/plans.md` (the profile -> plan table for review).

## What I built

`profiles_vulkan_test.go` builds `scan.Hardware` values directly (no text
parsing) for AMD Radeon and Intel Arc, Windows and Linux, 4 GB to 24 GB, plus
two built-in GPUs. Nothing in `internal/fit/` was changed: every profile
plans and stays inside its budgets, so no fix was needed for this slice.

Tests:

- `TestVulkanPlans` pins the balanced plan (model, quant, context, KV type,
  mode) for all 15 profiles.
- `TestVulkanInvariants` runs all three preferences on every profile and
  checks: a plan exists, the card is used (never a processor-only fallback),
  `GPUNeed <= GPU budget` and `RAMNeed <= RAM budget`, the plan records the
  budgets the planner was given, the placement fields are explicit (GPU: ngl
  999 and no expert offload; hybrid: ncmoe in 1..layers; split: ngl in
  1..layers), and `-t` is set exactly when weights live in RAM.
- `TestVulkanNoSpeculativeDecoding` pins that Vulkan plans never carry a draft
  model or `--spec-type` (the planner enables spec decoding for CUDA only).
- `TestVulkanWindowsFlags` pins `-lm none` only on Windows GPU-resident plans,
  and `-ub 2048 -b 2048 -t <phys cores>` on hybrid plans.
- `TestVulkanMargin` pins the margins: Windows Vulkan 12% with a 1.5 GB
  floor, Linux Vulkan 4% with a 0.4 GB floor, and the floor (not 12%) winning
  on a 4 GB card.
- `TestVulkanShowPlans` prints the table (no network, no files).

All 15 profiles produce a plan under every preference. The full table is in
`plans.md`.

## Findings

1. **Minor, by design but worth knowing: the 1.5 GB Windows Vulkan floor
   costs small cards a lot** (`internal/fit/fit.go:186-193`). A 4 GB RX 6500
   XT with 3 GB free gets a 1.5 GB budget, so the balanced pick is
   `lfm3-1.7b Q4_K_M` at 16K context (`plans.md` row 1). A 3B-class model
   does not fit even in split mode, while the 4% Linux cushion would leave
   2.9 GB. This is safe (never overcommits), just conservative; no change
   proposed because the wide cushion is what protects Windows from silent
   VRAM spill.
2. **Open question, possible major: shared-memory built-in GPUs may be
   double counted** (`internal/fit/fit.go:174` and `:391`, `:437`).
   `ComputeBudgets` treats GPU and RAM as separate pools unless
   `h.Unified`, and `scan` only sets `Unified` on Apple Silicon. On a Radeon
   780M or Arc Graphics the driver's free memory is a slice of system RAM,
   so a hybrid plan can commit both: row 7 of `plans.md` asks for 6.3 GB of
   GPU plus 17.1 GB of RAM against 20 GB of free RAM (23.4 GB total). That
   fits if the 8 GB share is a BIOS carve-out already excluded from
   `RAMAvail`, and overcommits by 3.4 GB if it is dynamic shared memory.
   Windows adapter scan (`internal/scan/gpu_discrete.go:190-220`) does not
   record which kind it is, so `internal/fit` cannot tell; a `GPU.Shared`
   flag in `scan` would fix it. I did not add a failing test because the
   expected value depends on that unanswered question; the numbers are
   pinned in `plans.md` instead.
3. **Minor: vendor-prefixed quant names skip the quant table**
   (`internal/fit/fit.go:142-151`). `normQuant` strips only `GSQ-RCO-`,
   `UD-` and `IMATRIX-`, so `NVIDIA-24GB (IQ2_XXS QAD)` falls through to the
   unknown-quant 0.9 instead of its base type's 0.88. I checked the effect
   by forcing 0.88 on the RX 6600 profile: the same model is still picked
   (the plan moves to `UD-IQ2_M` at 30.1 tok/s instead of the QAD file at
   34.2). No visible bug today, but any future vendor prefix with a different
   base type would be scored wrongly.
4. **Not a bug, recorded because it matters for Vulkan:** speculative
   decoding is CUDA-only (`internal/fit/fit.go:501-503`), so Vulkan plans
   never charge draft bytes or draft context. This is conservative and
   correct for the current evidence; `TestVulkanNoSpeculativeDecoding` will
   fail loudly if someone turns spec decoding on for Vulkan without
   accounting for the draft.

## Checks run

```
gofmt -l .                        (empty)
go build ./...                    ok
go vet ./...                      ok
go test -p 2 ./internal/fit/      ok
```

`go test -p 2 ./...` was not run for this slice to keep the shared machine
light; the package test above is the relevant one and no file outside
`internal/fit/` and `agents/T04/` was touched.

## Next steps

- Decide the shared-memory question (finding 2) and, if the share is
  dynamic, add `GPU.Shared` to `scan` and treat it as unified in
  `ComputeBudgets`.
- If the 4 GB card result (finding 1) is judged too conservative, scale the
  Windows Vulkan floor with card size instead of a flat 1.5 GB; that is a
  product call, not a correctness fix.
