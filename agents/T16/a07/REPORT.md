# T16 a07: 8 GB Macs

Area: the smallest-model path, the warnings the wizard shows on a small Mac,
and what the app does when nothing fits.

Branch `agent/T16-a07`, worktree `~/ezlm-agents/work/subs/T16/a07`.
Reviewed: `macos/Sources/EZLM/Wizard.swift`, `AppModel.swift` (read only),
`ModelMode.swift`, `Models.swift` (read only), `internal/fit/fit.go` (read
only), `internal/scan/scan_darwin.go`, `internal/scan/scan.go` (read only).

## What the smallest-model path does on 8 GB

`internal/fit/fit.go:214` (`osFloor`) leaves 1 GB for the OS on an 8 GB Mac,
and `ComputeBudgets` takes 10% on top (`fit.go:170-179`). On a unified Mac the
GPU and RAM budget are the same number: `min(GPUUsable, RAMAvail)` minus that
margin. I printed the picks with a throwaway `go run` program over the real
catalog (removed afterwards; `internal/fit` is not mine to edit):

```
total  8GB avail  1.0GB budgets gpu 0.00 ram 0.00 -> NOTHING FITS
total  8GB avail  1.5GB budgets gpu 0.50 ram 0.50 -> NOTHING FITS
total  8GB avail  2.0GB budgets gpu 1.00 ram 1.00 -> ternary-bonsai-2-2b Q2_0 gpu 16k ~80.1 t/s
total  8GB avail  2.5GB budgets gpu 1.50 ram 1.50 -> lfm3-1.7b Q4_K_M gpu 16k ~39.8 t/s
total  8GB avail  3.0GB budgets gpu 2.00 ram 2.00 -> ternary-bonsai-2-4b Q2_0 gpu 32k ~36.1 t/s
total  8GB avail  3.5GB budgets gpu 2.50 ram 2.50 -> granite-4.2-3b Q3_K_M gpu 16k ~23.9 t/s
total  8GB avail  4.0GB budgets gpu 3.00 ram 3.00 -> granite-4.2-3b Q3_K_M gpu 32k ~23.9 t/s
```

So the ladder degrades the way it should: a fresh 8 GB M1 gets a 3B at Q3 and
32k context (~24 words/s), a busy one falls to 1.7B or a ternary 2B, and the
cliff is at about 2 GB free. The existing `Mac M1 8GB (4GB free)` row in
`internal/fit/profiles_test.go:74` already covers the healthy case.

## Findings

### 1. Major, fixed: the "nothing fits" hint points at a list that is not there on a Mac

`macos/Sources/EZLM/Wizard.swift:190-191` (before this change) said "Quit an
app on the right, then scan again", but the right column only drew
`hw.gpuProcs` (`Wizard.swift:231`). On a unified Mac the engine never fills
`gpu_procs`: `internal/scan/scan_darwin.go:39` sets `Unified = true`, and
`internal/scan/scan.go:148-149` puts the biggest memory users in `top_mem`
instead. Evidence: the `Hardware` JSON decoded by `Models.swift:37-38`, plus
`Screens.swift:245`, which already falls back to `top_mem` for the same reason.
On the machine where this matters most (an 8 GB Mac with 1.5 GB free, where
nothing fits) the wizard showed the sentence and an empty right column.

Fix: a small `MemoryUsers` helper in `Wizard.swift` picks the graphics list
when the engine sends one, else the top memory users, with the matching
heading. The no-fit line now only says "on the right" when a list is actually
shown, and otherwise says "Close a few apps, then scan again." "Free up memory"
(Model mode) is unchanged and still the better route.

Test: `macos/Tests/EZLMTests/SmallMacTests.swift` (4 tests, new file, mine).
It covers the Mac case (top_mem listed, heading, hint), the discrete-GPU case,
the no-list case, and decoding the engine's 8 GB Mac JSON with `top_mem`.

### 2. Minor, report only: nothing fits under about 2 GB free, and the app has no last-resort plan

`fit.go:170-179` and `fit.go:214` make the budget `avail - max(10%, 1 GB)`, so
at 1.5 GB free the budget is 0.5 GB and even `ternary-bonsai-2-2b Q2_0` at 8k
does not fit. The wizard then offers Scan again and Free up memory, and the
unlock line (`internal/app/app.go:243-251`) offers the model that 85% free
would run, about 6 GB on an 8 GB Mac. That is honest, but there is no "run the
smallest one anyway" choice. `fit.go` is outside my ownership, so this is a
finding for whoever owns it. A 8192-context floor or a 1.5 GB budget floor
would cover the busy-8 GB case.

### 3. Minor, report only: only two memory users are listed

`Wizard.swift:233` shows `prefix(2)` of up to six rows the engine sends
(`internal/scan/scan.go:208`, `topMem(ctx, 6)`). On an 8 GB Mac the app
holding the memory is often third. Model mode lists them all, so this is
cosmetic.

## Not verified

- No real 8 GB Mac here, and no engine launch allowed. The numbers above come
  from the real catalog and `ComputeBudgets`, not from a scan of this machine.
- The `memory_pressure` path (`internal/scan/scan_darwin.go:14-25`) decides
  `ramAvail` on a real Mac; a Mac under memory pressure may report a very
  different free number than `vm_stat`. Needs a device.
- Model mode's app list (`GET /api/apps`) on a small Mac was not exercised.

## Open questions

- Should a Mac with under 2 GB free be allowed to run a 2B at 8192 context
  rather than be told nothing fits? That is a fit policy call, not a UI one.
- Should the wizard's no-fit state show the unlock model more prominently
  (it is a single dim line today) now that the list beside it is populated?

## Checks

`cd macos && swift build` and `swift test`: pass (26 tests, 0 failures).
`gofmt -l .` empty, `go build ./...`, `go vet ./...`: pass.
