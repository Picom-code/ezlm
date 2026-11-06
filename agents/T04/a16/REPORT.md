# T04 / a16: internal/fit arithmetic and unit review

Slice: review `internal/fit/` for bugs the profile tests cannot see (overflow,
float rounding, unit confusion GB vs GiB, integer division, context-ladder
boundaries). Test file: `internal/fit/invariant_units_test.go`.
Branch `agent/T04-a16`, base snapshot `5213370`.

All tests run with `go test -p 2 ./internal/fit/`. `gofmt -l .` is empty,
`go build ./...` and `go vet ./internal/fit/` pass.

## Fixed (failing test first, then the smallest fix)

### 1. Context ladder: a model capped below 8192 gets no plan at all (major)

`internal/fit/fit.go:92` (ladder), `internal/fit/fit.go:311` (`bestForModel`)

The ladder is `{65536, 32768, 16384, 8192}` and every rung above `m.CtxMax`
is skipped, so a model with `ctx_max` 4096 (older models, and the catalog is
fetched from `RemoteURL`, so new entries arrive without a release) has **no**
candidate context. `bestForModel` returns `found == false` and the model
silently disappears from every plan list: "nothing fits" on a machine that
runs it easily. Nothing in the UI or logs says why.

Before (`go test -run TestContextLadderPlansSmallContextModels -v`):

```
invariant_units_test.go:51: ctx_max 4096: no plan at all for a model that fits in 8 GB
invariant_units_test.go:51: ctx_max 6144: no plan at all for a model that fits in 8 GB
invariant_units_test.go:51: ctx_max 8191: no plan at all for a model that fits in 8 GB
```

After the fix the same test passes, and 8192/16384 are unchanged. No current
catalog model has `ctx_max < 8192` (checked: the smallest is 65536), so the
pinned plans in `fit_test.go` are unaffected.

Fix: named `ctxRung` type, and when `m.CtxMax` is below the shortest rung,
append `ctxRung{m.CtxMax, 0.5}` (one rung below 8192's 0.6 factor, so the
ordering of factors stays monotone).

### 2. Speed class taken from the rounded rate (minor)

`internal/fit/fit.go:498`

`try` rounded `p.TokPerSec` to one decimal and then classified `Speed` from
the rounded value. A plan at 7.96 tok/s rounds to 8.0, is labelled `good`,
and `usable()` treats it as usable although `MinTokPerSec` is 8.0. The same
rounding crosses `30` (`good` vs `fast`) and the `SmartestMinTokPerSec`
floor in `usable` (`fit.go:252`), which also compares the rounded field.

Before:

```
invariant_units_test.go:85: speed "good" for a true 7.960 tok/s, want slow
invariant_units_test.go:88: a 7.960 tok/s plan counts as usable
```

Fix: classify on the unrounded rate, keep the rounded value in `TokPerSec`
for display and ranking.

### 3. `quantQuality`: the IQ1_M case never ran (minor)

`internal/fit/fit.go:133`

`case strings.HasPrefix(n, "IQ1_M"), strings.HasPrefix(n, "Q1"),
strings.HasPrefix(n, "TQ"): return 0.8` came before the dedicated
`case strings.HasPrefix(n, "IQ1_M"): return 0.82`, so 0.82 was dead code.
`UD-IQ1_M` is a real catalog quant (qwen3.8-flash-next), so the model was
scored at 0.80 instead of 0.82.

Before: `invariant_units_test.go:98: UD-IQ1_M quality 0.80, want 0.82`.

Fix: put the specific IQ1_M case first, and drop the now-unreachable `Q1`
and `TQ1` alternatives from the last case (both were already caught above).

Commit: `c5bff4d T04: plan models capped below the context ladder, classify speed on the real rate`.

## Found, proved, not fixed (report only)

### 4. Hybrid plans clamp `RAMNeed`, so the margin invariant is vacuous (minor)

`internal/fit/fit.go:439`: `p.GPUNeed, p.RAMNeed = fixed+..., min(ram, b.RAM)`.

A hybrid plan may stream experts from SSD when `ram` is up to 1.1x `b.RAM`
(`fit.go:431-437`), but it reports `RAMNeed == b.RAM`. `fit_test.go:55`
asserts `p.RAMNeed > b.RAM` for every plan, so that check can never fail for
a hybrid plan, and `internal/app/app.go:1631`
(`p.GPUNeed+p.RAMNeed > ComputeBudgets(...).RAM`) under-counts what a
streaming plan really uses.

`TestHybridRAMNeedIsClampedToBudget` proves it with a real plan:
`gpu 20 GiB: 14 layers of experts in RAM weigh 8.23 GiB, plan reports 8.00 GiB (1.03x free RAM)`.
`GPUNeed` is also bounded by construction (`onGPU = (b.GPU-fixed)/perLayer`),
so the GPU half of the same assertion is close to vacuous too.

Not fixed because changing `RAMNeed` to the true value is a cross-package
behaviour change (`internal/app`, `internal/serve` display) and
`fit_test.go` would then need its invariant to allow the 1.1x page factor.
Suggested next step: report the true expert bytes in a new field (or stop
clamping and let callers apply the 1.1x rule), and fix the app check to
compare against `RAM` plus the page factor.

### 5. "GB" in this package means GiB, and one threshold is off by 7% (minor)

`internal/fit/fit.go:20` (`gb = float64(scan.GB)`, and `scan.GB = 1<<30`),
`internal/fit/fit.go:538`.

Every comment says GB (and `scan.GPU.Bandwidth` is documented GB/s, which is
decimal), but all memory maths is GiB. That is consistent internally, so no
plan is wrong today, but the 24 GiB threshold for `--spec-draft-n-max 3`
means "more than ~25.8 GB of VRAM": a 24 GB card never reaches it, because a
24 GB card is 22.4 GiB. If the intent was "24 GB cards and up", the constant
should be `22 * scan.GB` (or `24e9`). `TestBudgetUnitsAreGiB` pins the GiB
reading of `osFloor` and the three margin floors so a future switch to
decimal GB fails loudly.

### 6. Integer division truncates the KV cache by less than one byte per token (minor)

`internal/catalog/catalog.go:98` and `:116`: `elems * 34 / 32`,
`elems * 18 / 32`.

`elems` is `2*layers*heads*dim`, not a multiple of 32 in general, so q8_0 and
q4_0 per-token sizes round down by up to 31/32 of a byte. At 64K context that
is at most 64 KB per plan, i.e. far below the margin. Not worth a fix; noted
so nobody "fixes" it into a float and loses exactness. `f16` is exact, and
`TestOverheadAndKVPerTokenUnits` pins all three to within one byte per token.

### 7. Small split edge cases (minor, not reachable from the catalog)

- `internal/fit/fit.go:456`: `onGPU < m.Layers/3` is an integer division, so
  a model with 1 or 2 layers accepts `onGPU == 0` as a Split plan (all layers
  on the CPU, but costed as a split).
- `internal/fit/fit.go:456`: `perLayer := float64(w)/float64(m.Layers)`
  divides by zero if a catalog entry has `layers: 0`; the `+Inf` is then
  converted to `int` (implementation-defined, `minint` on amd64) and the
  guard happens to reject the plan. Worth a `Layers <= 0` guard for clarity.
- `internal/fit/fit.go:346`: `w -= q.LazyBytes` underflows to ~2^64 if a
  catalog quant has `lazy_bytes > bytes`; the model then never plans. All
  five lazy quants today have `lazy_bytes` around 34% of `bytes`.

### 8. Overflow: nothing reachable

`margin`, `sub`, `overhead` and `specOverhead` were checked at
`avail = 0, 1, 1 MiB, 1 GiB, 1 TiB, MaxUint64` with total RAM the same and
all four backends (`TestBudgetsNoOverflow`). Budgets stay within their
inputs, `sub` never wraps, and `margin`'s float64 conversion stays below
2^64 because `frac <= 0.12`. `overhead`'s `uint64(ctx)*4<<10` parses as
`(ctx*4)<<10` = 4096 bytes per token, which the test pins; the same for
`specOverhead` (2048 bytes per token). `weights/100` cannot overflow below
~1.8e19 bytes of weights.

## Open questions

- Should a plan be *dropped* when no rung fits (`m.CtxMax < 8192` was the
  only such case) or planned at `m.CtxMax`? I chose the latter with factor
  0.5; a different factor changes how these models rank against bigger-context
  ones.
- Is `RAMNeed` meant to be "what the plan needs" or "what the budget allows"?
  The clamp at `fit.go:439` says the second, the field name and the app check
  say the first.
