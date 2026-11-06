# a10: `--reasoning-format`, `--reasoning-budget`, thinking controls

Scope: llama.cpp `b11960` (pinned) versus the latest release. `releases/latest` is
`v0.9.0` (2026-11-04); its only asset is `nightly-tag.txt`, which contains `b12110`.
The newest build by date is `b12144`. I checked all three tags. Evidence: source at
`raw.githubusercontent.com/ggml-org/llama.cpp/<tag>/...`, fetched 2026-11-06.

## Verdict

No breaking change for EZLM. The reasoning flag names, values and defaults are
identical at b11960, b12110 (v0.9.0) and b12144. EZLM passes none of these flags,
so it runs on llama.cpp's defaults, and those defaults did not move. `/props`
still reports `chat_template_caps.reasoning_effort_levels`, which is the only
reasoning data EZLM reads back.

Two parser fixes landed between the tags. Both are behind flags EZLM does not
pass, so neither changes EZLM's output today.

## Flag surface, b11960 to b12110/b12144

`common/arg.cpp`, same option text and same `common_params` fields at every tag:

| Flag | b11960 | b12110 | b12144 |
|---|---|---|---|
| `--reasoning-format FORMAT` | 3735 | 3674 | 3674 |
| `-rea`, `--reasoning [on\|off\|auto]` | 3746 | 3685 | 3685 |
| `--reasoning-effort LEVEL` | 3764 | 3703 | 3703 |
| `--reasoning-budget N` | 3776 | 3715 | 3715 |
| `--reasoning-budget-message MESSAGE` | 3784 | 3723 | 3723 |
| `--reasoning-preserve` / `--no-reasoning-preserve` | 3791 | 3730 | 3730 |

`diff arg.cpp b11960 b12110` changes 67 lines, none of them reasoning related: they
are Windows `argv`/UTF-8 and `fs_path_to_utf8` refactors. `b11960..b12144` differs
only in the same areas.

- `--reasoning-format` values: `none`, `auto`, `deepseek`, `deepseek-legacy`
  (`common/chat.cpp:1026` at every tag). No value added or removed. Anything else
  throws `Unknown reasoning format`.
- Default format: `common/common.h:666` sets
  `reasoning_format = COMMON_REASONING_FORMAT_DEEPSEEK`; `AUTO` is documented as
  "same as deepseek" (`common/common.h:428-433`). The help text says
  `(default: auto)` while the field default is `deepseek`; the wire result is the
  same (`message.reasoning_content`). Do not parse the help string.
- Default budget: `-1` = unrestricted (`common/common.h:294`,
  `arg.cpp` help "default: -1"). `0` ends thinking immediately; `N>0` is a token
  budget. `--reasoning-budget-message` default is none.
- Default `--reasoning`: `auto` (`enable_reasoning = -1`,
  `common/common.h:667`). `on`/`off` additionally write the `enable_thinking`
  template kwarg at every tag (`arg.cpp:3750-3755` b11960, `3689-3694` b12110).

## Behaviour changes in the same area

1. **Reasoning budget forcing ends on a prompt end tag** (`common/reasoning-budget.cpp:136-142`).
   b12110 adds `match >= 0` to the completion test, so a `think`/`end_of_think`
   sequence already in the prompt stops forcing instead of running the whole
   forced sequence (PR #31374). Only reachable when a budget `>= 0` is set. EZLM
   sets none, so budget stays `-1` and the forcing path is idle.
2. **qwen3-coder think block is consumed with `reasoning_format none`**
   (`common/parsers/qwen3-coder.cpp:74-95`, PR #31380). With
   `supports_reasoning && !extract_reasoning` and a `response_format`, the parser
   now eats ` think...</think>` before the JSON body instead of leaving it in
   `content`. `extract_reasoning` is false only for `reasoning_format none`.
   EZLM's default is `deepseek`, so the block is still extracted into
   `reasoning_content` and the answer stays bare.

`/props` schema is unchanged for this slice: `chat_template_caps` still comes from
`common_chat_templates_get_caps` (`tools/server/server-context.cpp:5688` b11960,
`:5697` b12110, `:5713` b12144) and still carries
`reasoning_effort_levels` (`common/chat.cpp:1953`, identical at all tags). The
v0.9.0 `voices` key is additive. The `enable_thinking` gate is byte-identical at
b11960 and b12110 (`server-context.cpp:1706-1707`): `--reasoning auto` plus a
template that reads `enable_thinking` decides it.

## What EZLM passes and reads

- `internal/serve/config.go:115-138` builds `-m`, `--alias`, `--host`, `--port`,
  `--api-key`, `-c`, `-ngl`, `--jinja`, `--metrics`, `-np 1`, optional
  `--n-cpu-moe`, `-md`, `--device`, then recipe `ExtraArgs`. No `--reasoning*`
  flag. Recipe flags are appended last, so a recipe can set
  `--reasoning-budget`/`--reasoning-format` and win.
- `internal/serve/props.go:65-76` reads `chat_template_caps.reasoning_effort_levels`
  (dedupe, drop blanks, keep order) and `props.go:80-89` decides `Thinking` from
  the literal string `enable_thinking` in `chat_template`, with a fallback on
  `chat_template_caps.supports_thinking` / `supports_reasoning`.
- `internal/bench/bench.go:127-128` turns thinking off per request with
  `chat_template_kwargs.enable_thinking=false` and `reasoning_effort=low`. That is
  a request-body control, not a server flag, and the server still honours it
  through `--jinja` at both tags.

## Findings

- **minor, pre-existing, not a regression:** the `propThinking` caps fallback
  looks for `supports_thinking` and `supports_reasoning`, but `jinja::caps::to_map`
  (`common/jinja/caps.cpp:108-119`) only emits `supports_preserve_reasoning` and
  `supports_reasoning_effort`. Those two keys never exist at any of the three
  tags, so the fallback is dead and a template that thinks without the literal
  `enable_thinking` (for example one keyed on `thinking`) reads as
  `Thinking=false` and the app writers leave the thinking toggle out.
  `internal/serve/props.go:80-89`; pinned by
  `internal/engine/flags_reasoning_test.go:TestFetchPropsThinkingFallsBackToCapsKey`.
- **minor:** `/props` reports levels only when the template renders them
  distinctly; `reasoning_effort_levels` stays empty otherwise. EZLM already
  treats an empty list as "leave reasoning settings out"
  (`internal/serve/props.go:61-64`), so this is correct but worth knowing when a
  model shows no effort levels after the upgrade.
- **info:** `--chat-template-kwargs` still accepts `enable_thinking` and
  `preserve_reasoning` but logs a deprecation warning and points at
  `--reasoning on|off` / `--reasoning-preserve` (`arg.cpp:3588-3595` b11960,
  `3527-3534` b12110). EZLM does not pass it, so no change needed.

## Test added

`internal/engine/flags_reasoning_test.go` (package `engine_test`, imports
`internal/serve` read-only):

- `TestArgsPassNoReasoningFlags`: EZLM's flag builder passes none of the
  reasoning flags, so the defaults studied here apply.
- `TestArgsReasoningFlagsFromRecipeComeLast`: recipe `ExtraArgs` come last, so a
  recipe override still wins.
- `TestFetchPropsReasoningAtLatestRelease`: a `/props` body in the b12110 shape
  (including the new `voices` key) still yields levels, thinking and build info.
- `TestFetchPropsThinkingFallsBackToCapsKey`: pins the dead caps fallback above.

All four pass. No production code changed.
