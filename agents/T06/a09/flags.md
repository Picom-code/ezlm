# a09: `--jinja`, `-np`, `--cache-ram`, `--kv-unified`

Tags compared: **b11960** (EZLM's pin) and **b12110** (the build `v0.9.0`, the
current `releases/latest`, pins through `nightly-tag.txt`). **b12144** (newest
nightly by date) was spot-checked for the same four flags and is identical.

Sources fetched over HTTPS from `raw.githubusercontent.com/ggml-org/llama.cpp/<tag>/`:
`common/arg.cpp`, `common/common.h`, `tools/server/server.cpp`,
`tools/server/server-context.cpp`, `tools/server/README.md`.

## EZLM's current usage

`internal/serve/config.go:115-138` builds the command line.

| flag | EZLM | where |
|---|---|---|
| `--jinja` | always passed, no value | `internal/serve/config.go:124` |
| `-np 1` | always passed, one slot | `internal/serve/config.go:126` |
| `--cache-ram` | never passed, llama.cpp default applies | none |
| `--kv-unified` | never passed, llama.cpp default applies | none |

`Config.ExtraArgs` (recipe flags from `internal/fit/fit.go:518`) are appended
after the fixed flags (`internal/serve/config.go:137`). Today `Plan.Args()`
only emits `-ctk/-ctv`, `--spec-type/--spec-draft-n-max`, `-lzm`, `-lm`, `-t`,
`-ub/-b` and quant args, so no recipe flag collides with the four here.

## llama.cpp defaults and behaviour, by tag

### `--jinja`

| | b11960 | b12110 (v0.9.0) | b12144 |
|---|---|---|---|
| definition | `common/arg.cpp:3727` | `common/arg.cpp:3666` | `common/arg.cpp:3666` |
| default | `use_jinja = true` (`common/common.h:655`) | same (`:655`) | same (`:655`) |
| negation | `--no-jinja` | `--no-jinja` | `--no-jinja` |
| env | `LLAMA_ARG_JINJA` | same | same |

Default is already **on** in both tags, so EZLM's explicit `--jinja` is
redundant but harmless. `/props` publishes `chat_template` and
`chat_template_caps` unconditionally (`server-context.cpp:5696-5697` in
b12110; `:5687-5688` in b11960), and only `chat_template_tool_use` is gated
on `params.use_jinja` (`:5711-5715` / `:5699-5703`). `common/chat.cpp` is
byte-identical between b11960 and b12110, so the flag's meaning did not move.

**Verdict:** no change, no breakage. Keeping the explicit flag is the safer
choice: it pins template rendering against a future default flip.

### `-np` / `--parallel`

| | b11960 | b12110 (v0.9.0) | b12144 |
|---|---|---|---|
| server definition | `common/arg.cpp:2556` | `common/arg.cpp:2495` | `common/arg.cpp:2495` |
| common default | `n_parallel = 1` (`common/common.h:464`) | same | same |
| server default | `n_parallel = -1` (auto) set in `common_params_parser_init`, `common/arg.cpp:1414` | same, `common/arg.cpp:1353` | same, `common/arg.cpp:1353` |
| auto expansion | `-1` becomes 4 slots **and** `kv_unified = true`, `tools/server/server.cpp:126-130` | identical | identical (server.cpp byte-identical) |
| validation | `-np 0` throws | same | same |
| env | `LLAMA_ARG_N_PARALLEL` | same | same |

EZLM passes `-np 1`, which skips the auto path entirely: one slot,
`kv_unified` stays false, and the single slot owns the whole `-c`. If the
flag were ever dropped, the server would silently run four slots with a
unified KV cache, `-c` would be shared four ways, and `/props.total_slots`
would report 4 (`server-context.cpp:5681` in b12110, `:5672` in b11960).
EZLM stores `total_slots` (`internal/serve/props.go:44,56`) but nothing
consumes it yet, so the drift would be invisible in the app.

**Verdict:** no change, no breakage. `-np 1` must stay explicit.

### `--cache-ram` / `-cram`

| | b11960 | b12110 (v0.9.0) | b12144 |
|---|---|---|---|
| definition | `common/arg.cpp:1727` | `common/arg.cpp:1666` | `common/arg.cpp:1666` |
| default | `cache_ram_mib = 8192` (`common/common.h:644`) | same | same |
| sentinel | `0` disables, `-1` no limit | same | same |
| prompt cache construction | `server-context.cpp:1594-1602` | same lines | same (shifted +3 by TTS code) |
| env | `LLAMA_ARG_CACHE_RAM` | same | same |

The 8192 MiB is a **cap on the host-RAM prompt cache**, not a preallocation:
the cache grows as prompts are saved and evicted. EZLM never passes the flag,
so every deploy inherits the 8 GiB cap. On a 16 GB machine running a large
model this is host-RAM headroom EZLM has not budgeted; on a 32 GB machine it
is usually fine. The cache is also disabled automatically for decision models
unless the flag was explicit (`server-context.cpp:1588-1591`), which does not
apply to EZLM's chat models.

`--cache-idle-slots` defaults to true and warns/disables itself only when
`--cache-ram 0` is given (`server-context.cpp:1663-1670`); clearing idle
slots additionally needs a unified KV cache (`:1914`), so with `-np 1` it
saves a cache copy but never clears the slot's KV.

**Verdict:** defaults unchanged across the upgrade, so the upgrade does not
break anything. It is a pre-existing gap against the comment at
`internal/serve/config.go:113-114` ("Every memory-relevant flag is explicit"),
because this one is not.

### `--kv-unified` / `-kvu`

| | b11960 | b12110 (v0.9.0) | b12144 |
|---|---|---|---|
| definition | `common/arg.cpp:1736` | `common/arg.cpp:1675` | `common/arg.cpp:1675` |
| default | `kv_unified = false` (`common/common.h:582`) | same | same |
| auto-enable | only when `-np` is auto (`server.cpp:126-130`) | same | same |
| negation | `--no-kv-unified` | same | same |
| env | `LLAMA_ARG_KV_UNIFIED` | same | same |

With `-np 1` the unified and per-slot layouts are equivalent in context
size, so EZLM not passing it is fine. The help text ("default: enabled if
number of slots is auto") is accurate for both tags.

**Verdict:** no change, no breakage.

## Summary

| flag | renamed? | default changed? | behaviour changed? | EZLM broken? |
|---|---|---|---|---|
| `--jinja` | no | no (on) | no | no |
| `-np` | no | no (`-1` auto on the server) | no | no |
| `--cache-ram` | no | no (8192 MiB) | no | no (latent memory gap) |
| `--kv-unified` | no | no (off) | no | no |

The only action for the upgrade is a decision, not a fix: pin `--cache-ram`
explicitly (for a one-user deployment `--cache-ram 0` trades prompt-cache
reuse for host RAM) or amend the comment at `internal/serve/config.go:113-114`
so it no longer claims every memory-relevant flag is explicit. `internal/serve`
is read-only for T06, so this is left to the owning task.

The flag-builder invariants EZLM depends on are pinned by
`internal/engine/flags_core_test.go`.
