# T06 / a09 report: `--jinja`, `-np`, `--cache-ram`, `--kv-unified`

## Scope and method

Diffed the four server flags between the pinned build **b11960** and the build
the current `releases/latest` (**v0.9.0**, 2026-11-04) pins. `v0.9.0` carries
only `nightly-tag.txt`; its content is `b12110`, so **b12110** is the upgrade
target for this slice. Also spot-checked **b12144** (newest nightly by date).

Read-only sources at each tag: `common/arg.cpp`, `common/common.h`,
`tools/server/server.cpp`, `tools/server/server-context.cpp`,
`tools/server/README.md`, plus `common/chat.cpp` for the jinja path. EZLM side
read only: `internal/serve/config.go`, `internal/serve/props.go`,
`internal/fit/fit.go`. Nothing was executed from llama.cpp; no model was
loaded.

Full detail and the per-tag line-number table: `agents/T06/a09/flags.md`.

## Findings

### F1 (minor, pre-existing): `--cache-ram` is a memory-relevant flag left to its 8 GiB default

EZLM never passes `--cache-ram`, so every deploy inherits
`cache_ram_mib = 8192` (`common/common.h:644` at b11960, b12110 and b12144).
The server creates the prompt cache with that cap at
`tools/server/server-context.cpp:1594-1602` (b12110). The cap is host RAM
used by saved prompts, growing over a session, so on a 16 GB machine running a
large model it eats headroom EZLM has not budgeted.

This contradicts the comment on the builder:
`internal/serve/config.go:113-114` says "Every memory-relevant flag is
explicit: nothing is left to defaults that could overcommit memory." It is not
an upgrade regression (8192 is the default in all three builds), but it is the
one real risk found in this slice.

### F2 (info): `--jinja` is redundant but safe

`use_jinja` defaults to true (`common/common.h:655` in all three builds), so
`internal/serve/config.go:124` changes nothing today. The flag still exists in
b12110 and b12144 (`common/arg.cpp:3666`, `--no-jinja` still present) and
`common/chat.cpp` is byte-identical between b11960 and b12110. Keeping the
explicit flag is the conservative choice: it pins jinja rendering (tool calls,
reasoning controls) against a future default flip. If a future build removes
the flag, EZLM's server fails to start with "invalid argument" rather than
degrading quietly, which is acceptable and easy to spot.

### F3 (info): `-np 1` is load-bearing, and unchanged

The server default is `-1` (auto), set in `common_params_parser_init`
(`common/arg.cpp:1353` in b12110, `:1414` in b11960), and auto expands to
four slots **and** `kv_unified = true` (`tools/server/server.cpp:126-130`,
identical in all three builds). EZLM's explicit `-np 1`
(`internal/serve/config.go:126`) keeps one slot owning the whole `-c` and
leaves `kv_unified` false. If the flag were dropped, `/props.total_slots`
would report 4 (`server-context.cpp:5681` in b12110). EZLM stores that field
(`internal/serve/props.go:44,56`) but nothing consumes it, so the drift would
be silent. The new test pins the flag.

### F4 (info): `--kv-unified` needs no change

Default false (`common/common.h:582`) in all three builds; auto-enabled only
on the `-np` auto path. With one slot the unified and per-slot layouts give
the same context size, so EZLM not passing it is correct. `--cache-idle-slots`
(default true) still requires `--cache-ram` and only clears KV under unified
KV (`server-context.cpp:1663-1670`, `:1914`), so it is a no-op for clearing
in EZLM's one-slot layout.

### F5 (info): b12144 is identical for this slice

`common/arg.cpp` differs from b12110 elsewhere, but the four flags keep the
same definitions, defaults and line numbers (`:1666`, `:1675`, `:2495`,
`:3666`). `server.cpp` is byte-identical; `server-context.cpp` changes are
TTS voice handling only (line numbers shift by +3 in the cache block). So the
study holds for both the release target and the newest nightly.

## What changed

- `internal/engine/flags_core_test.go` (new): pins the invariants EZLM's flag
  builder must keep for this slice: exactly one `--jinja`, exactly one `-np`
  followed by `1`, `-c` equal to the configured context, no `--parallel`
  alias, and recipe `ExtraArgs` appended last. It also checks the four flags
  are not passed twice. A comment records the studied tags and points at
  `flags.md`.
- `agents/T06/a09/flags.md` (new): the per-flag, per-tag evidence table.
- No change to `engine.go`, `pinned.go` or `internal/serve/`; no pinned-tag
  change.

The test lives in `internal/engine` because T06 owns that package and
`internal/serve` is read-only here. It imports `internal/serve` only for
`Config.Args()`; there is no import cycle (`serve` imports `internal/paths`
only).

## Verification

- `go test -p 2 -run 'TestArgs' ./internal/engine/` passes.
- `gofmt -l internal/engine/` is empty.

## Could not finish / open questions

- Whether to pin `--cache-ram` is a product decision (memory headroom vs
  prompt-cache reuse on slot reuse). `internal/serve` belongs to another task,
  so a09 only reports it.
- `--jinja`'s effect on template rendering itself (`common/jinja/`) is a10's
  slice; this report covers the flag, not template semantics.

## Next steps

1. Owning task for `internal/serve`: either pass `--cache-ram 0` (or a small
   explicit MiB value) for the one-user profile, or reword the comment at
   `config.go:113-114` so it is true.
2. Keep `-np 1` and `--jinja` explicit; the new test fails if either is
   dropped or duplicated.
3. When the pin moves to b12110, re-run the test; no flag change is needed
   for these four.
