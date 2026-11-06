# T04 / a13: fuzz test for free-memory edge values

Branch `agent/T04-a13`, worktree `~/ezlm-agents/work/subs/T04/a13`.
Slice: cross-cutting fuzz test, `internal/fit/fuzz_free_memory_test.go`.

## What I built

One file, `internal/fit/fuzz_free_memory_test.go`, with `FuzzFit`:

- **Six machine shapes** the fuzzer drives with free memory: desktop CUDA 12 GB,
  laptop CUDA 4 GB, Apple unified 64 GB, Windows Vulkan 16 GB, processor only
  16 GB, 2x CUDA 24 GB. Each shape is built directly as a `scan.Hardware`
  value (no text parsing, no scan).
- **Fuzz inputs:** `kind` (selects the shape), `freeRAM` and `freeVRAM` as
  `int64`. Values are read back as `uint64`, so a negative seed is the absurd
  case `2^64-1`; the target itself never assumes they are sane.
- **Seed corpus (206 added, 194 unique):**
  - the team's real profiles (`realMachines` values: RTX 4070 SUPER, GTX 1650
    laptop, M4 Max 64 GB, Windows RX 7800 XT, CPU-only 16 GB, 2x RTX 3090);
  - edge values for every shape: `0`, `1`, `GB-1`, `GB`, `1 TiB`, `2^64-1`,
    used for free RAM and for free VRAM separately;
  - for every catalog model, one byte below and one byte above the free VRAM
    at which its **smallest** file and its **largest** file start to fit. The
    boundary is found by binary search over the real `ComputeBudgets`
    (`fuzzFreeVRAMForNeed`), so it tracks the margin rule rather than a
    hard-coded formula.
- **Assertions per execution**, for all three preferences (`Balanced`,
  `Smartest`, `Fastest`) and every plan the planner returns:
  - budgets never exceed what was free: `b.GPU <= hw.GPUUsable`,
    `b.RAM <= freeRAM`;
  - the margin holds: `plan.GPUNeed <= b.GPU`, `plan.RAMNeed <= b.RAM`;
  - the plan is self-consistent: context positive and within `CtxMax`, finite
    non-negative speed and effective score, `-ngl` 999 for GPU/Hybrid, `0` for
    CPU, in `(0, Layers)` for Split, `NCPUMoE` within `[0, Layers]`;
  - a GPU-resident plan counts at least its resident weights
    (`GPUNeed >= Quant.Bytes - Quant.LazyBytes`), so a plan cannot "fit" by
    forgetting the weights;
  - `Recommend` agrees with `plans[0]`.
  A panic fails the fuzz target by itself.

## The 30 s run

Command (as the brief specifies):

```
go test -run Fuzz -fuzz FuzzFit -fuzztime 30s ./internal/fit/
```

Output (`agents/T04/a13/fuzz-30s.log`):

```
fuzz: elapsed: 0s, gathering baseline coverage: 194/194 completed, now fuzzing with 11 workers
fuzz: elapsed: 3s, execs: 19142 (6380/sec), new interesting: 51 (total: 245)
fuzz: elapsed: 6s, execs: 39582 (6807/sec), new interesting: 51 (total: 245)
fuzz: elapsed: 9s, execs: 59784 (6741/sec), new interesting: 51 (total: 245)
fuzz: elapsed: 12s, execs: 80403 (6873/sec), new interesting: 51 (total: 245)
fuzz: elapsed: 15s, execs: 96300 (5299/sec), new interesting: 51 (total: 245)
fuzz: elapsed: 18s, execs: 112947 (5549/sec), new interesting: 51 (total: 245)
fuzz: elapsed: 21s, execs: 134882 (7311/sec), new interesting: 51 (total: 245)
fuzz: elapsed: 24s, execs: 157308 (7469/sec), new interesting: 51 (total: 245)
fuzz: elapsed: 27s, execs: 179688 (7467/sec), new interesting: 51 (total: 245)
fuzz: elapsed: 30s, execs: 201566 (7292/sec), new interesting: 51 (total: 245)
PASS
ok  	github.com/picom-code/ezlm/internal/fit	30.376s
```

**Result: PASS, 201,566 executions, 30.4 s, no panic, no margin violation.**
No failing input was written to `internal/fit/testdata/fuzz/` (Go only writes
failures there), so the tree stays clean.

## Proof that the assertion bites

A green fuzz test is only worth something if it can fail. I temporarily let
the all-on-GPU case over-plan by 1 GB (`fit.go:386`, `full <= b.GPU+uint64(gb)`)
and reran the seed corpus:

```
--- FAIL: FuzzFit (0.06s)
    --- FAIL: FuzzFit/seed#0 (0.00s)
        fuzz_free_memory_test.go:155: desktop CUDA 12GB/balanced/qwen3.8-27b/GSQ-RCO-IQ2_XS-mtp: GPU need 11946032540 above budget 11338713662
    --- FAIL: FuzzFit/seed#1 (0.00s)
        fuzz_free_memory_test.go:155: laptop CUDA 4GB/balanced/granite-4.2-3b/Q4_0: GPU need 3375146707 above budget 2791728743
```

`fit.go` was restored afterwards (`git status` shows only the new test file).

## Findings

**No bugs found.** The budget invariant is enforced structurally in `try`, and
the fuzz run confirms it survives overflow-shaped input (`2^64-1` free memory,
1-byte budgets):

- `ComputeBudgets` (`internal/fit/fit.go:171`) subtracts a margin that is
  never larger than the input, and `sub` (`fit.go:211`) clamps at 0, so a
  budget can never exceed the free memory reported.
- All-on-GPU (`fit.go:386`) only takes the branch when `full <= b.GPU`.
- Hybrid (`fit.go:391`) sets `GPUNeed = fixed + onGPU*perLayer` with
  `onGPU = min(int((b.GPU-fixed)/perLayer), Layers)`, and `RAMNeed =
  min(ram, b.RAM)` (`fit.go:429`), so both sides are inside budget by
  construction.
- Split (`fit.go:437`) picks `onGPU` from `(b.GPU-kvBytes-oh)/perLayer` and
  rejects `ram > b.RAM`.
- CPU (`fit.go:455`) only takes the branch when `full <= b.RAM`.

One observation, not a bug: a hybrid plan whose experts page from SSD reports
`RAMNeed` clamped to `b.RAM` (`fit.go:429`) while the real need may be up to
1.1x `b.RAM` (`fit.go:422`). That is the documented clean-mmap exception, so
the test allows it; the *reported* number is what the UI shows, though, so a16
may want to check whether the UI should show the paged amount. Severity: minor,
UI-only.

## Checks run

- `gofmt -l .` empty
- `go build ./...`, `go vet ./...` clean
- `go test -p 2 -count=1 ./internal/fit/` ok
- `GOOS=windows GOARCH=amd64`, `GOOS=windows GOARCH=arm64`, `GOOS=linux`
  `go build ./...` ok

## What I did not do, and next steps

- I did not fuzz the catalog itself (quant sizes, layers, bandwidth); the
  brief scopes this test to free memory. A follow-up could fuzz `Model` fields
  too, which would exercise `overhead`, `KVBytesPerToken` and the hybrid layer
  math under odd values.
- The boundary seeds cover the default desktop shape only; extending
  `fuzzFreeVRAMForNeed` to the other shapes would need their margin rules.
- a14 (golden files) can reuse the profile shapes and the binary-search helper
  to record boundary plans as reviewable diffs.

## Open questions

- `scan.Hardware` uses `uint64` for free memory, so a negative value cannot
  come from a real scan; the fuzzer reads negatives as `2^64-1` on purpose to
  probe overflow. Is that the intended reading of "negative" in the brief?
- Should `RAMNeed` report the paged need (up to 1.1x budget) rather than the
  clamped one? The plan is safe either way; only the displayed number differs.
