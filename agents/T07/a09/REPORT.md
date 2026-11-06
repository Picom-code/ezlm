# T07 a09: timing-safe key comparison

**Threat:** a local process or a page that can reach the engine API on 127.0.0.1
guesses the per-launch token byte by byte from how long the comparison takes.
A comparison that exits at the first differing byte leaks the length of the
matching prefix; enough samples recover the whole token.

**Scope:** `internal/web/server.go` (the only token check), `internal/keys/**`.

## What I found

1. **The production check was already constant time.** `internal/web/server.go:254`
   (baseline) compared with `subtle.ConstantTimeCompare`, not `==`. There is
   no other token comparison in the engine API: `guard` is the single auth
   path (`server.go:248`), and the token only arrives in the `X-EZLM-Token`
   header or the `?t=` query (`server.go:250-253`). Severity: none, the
   control was already present.

2. **A timing signal from `==` is measurable at the production token length,
   but it is tiny.** Measured on this Mac (M3 Pro, Go 1.27.3), best of
   9 rounds x 200k comparisons, 32-character token, mismatch in the first
   vs the last byte:

   | comparison | first-byte mismatch | last-byte mismatch | ratio |
   |---|---|---|---|
   | `a == b` | 2 ns | 3 ns | 1.50x |
   | `subtle.ConstantTimeCompare` | 14 ns | 14 ns | 1.00x |

   So the signal exists (about 1 ns per comparison) and is detectable in a
   tight loop, but it is swamped by HTTP and JSON costs per request. At 256 KiB
   the same measurement is unambiguous: `==` 2 ns vs 4.8 us, while the
   constant-time function is 108 us vs 82 us (noise, no early exit).

3. **Real (latent) defect: two empty keys match.**
   `subtle.ConstantTimeCompare("", "")` returns 1. If a `Server` is ever
   constructed with an empty `token` (not via `Listen`, which always generates
   32 hex characters, `server.go:79-82`), a request that presents no token
   passes `guard`. Production is safe today, but the guard's safety depends on
   a constructor invariant it does not state. Severity: minor (latent auth
   bypass, no current path). Evidence: `TestEqual/both_empty` plus
   `TestSubtleEmptyGotcha`.

## What I changed

- `internal/keys/keys.go:30` adds `keys.Equal(a, b string) bool`: constant-time
  via `crypto/subtle`, and it rejects an empty key on either side so the
  `("", "")` case can never authenticate.
- `internal/web/server.go:254` uses `keys.Equal(t, s.token)` instead of the
  inline `subtle.ConstantTimeCompare` (one line plus one import). Behaviour for
  a real token is unchanged; an empty configured token now rejects everything.
  This is the only edit to a shared file.
- `internal/keys/timing_test.go` (new):
  - `TestEqual`: identical, first-byte off, last-byte off, prefix, empty
    cases.
  - `TestSubtleEmptyGotcha`: pins why the empty guard exists.
  - `TestEqualNoEarlyExit`: 256 KiB keys, first-byte vs last-byte mismatch
    must be within 5x; fails if `Equal` goes back to `==`.
  - `TestNaiveCompareHasTimingSignal`: the same measurement against `==`,
    proving the method can see an early exit at all (evidence, not a test of
    `Equal`).
  - `BenchmarkEqual32`, `BenchmarkNaiveEqual32`, `BenchmarkEqual256K` for the
    numbers above.

Before: `subtle.ConstantTimeCompare([]byte(t), []byte(s.token)) != 1` and an
empty token authenticated an empty request. After: `!keys.Equal(t, s.token)`,
with the empty case tested.

## Verification

- `gofmt -l .` empty, `go build ./...`, `go vet ./...` clean.
- `go test -p 2 -count=1 ./internal/keys/ ./internal/web/ ./internal/serve/` passes.
- Temporary (uncommitted) end-to-end check of `guard`: good header and good
  query 200; wrong, prefix and empty token 401; a `Server{}` with an empty
  token rejects an empty request. Removed after the run.
- Benchmarks (`go test -run '^$' -bench . -benchtime=300ms ./internal/keys/`):
  `Equal32` 14-16 ns/op (0 allocs), `NaiveEqual32` 2.7 ns/op,
  `Equal256K` 78 us/op.

## Residual risk and open questions

- Length still leaks: `subtle.ConstantTimeCompare` returns 0 immediately for
  unequal lengths. The token length (32) is fixed and public, so this reveals
  nothing useful. Making length constant time too would cost more than it buys.
- The token is still accepted from the query string for EventSource
  (`server.go:251-253`), so it can leak through URLs, history and logs. That is
  a04's slice, not this one.
- HTTP-level timing on loopback is noisy (tens of microseconds per request vs
  a 1 ns comparison difference), so an end-to-end timing attack against the
  original `==` was possible but expensive. The comparison is now constant
  time, so the only remaining source is request handling, which does not
  depend on the token's bytes.
- Should `keys.Equal` be the only comparison for API keys everywhere? The
  llama-server client only sends the key (`internal/serve/client.go:44,178`);
  it never compares one. The apps compare nothing locally. So there is one
  call site today.
