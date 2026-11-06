# T06 a10 report: reasoning flags and thinking controls

Slice: `--reasoning-format`, `--reasoning-budget`, and the thinking controls,
between the pinned `b11960` and the latest release. Branch `agent/T06-a10`.

## What I did

1. Read EZLM's usage: `internal/serve/config.go:115-138` (flag builder),
   `internal/serve/props.go:17-89` (`/props` parse), `internal/bench/bench.go:127-128`
   (per-request thinking off), and the app writers that consume `Props`.
2. Fetched llama.cpp source at three tags with GET only: `b11960` (pinned),
   `b12110` (the build named by `v0.9.0`'s `nightly-tag.txt`) and `b12144`
   (newest build by date). Files: `common/arg.cpp`, `common/common.h`,
   `common/chat.cpp`, `common/chat.h`, `common/jinja/caps.h`,
   `common/jinja/caps.cpp`, `common/reasoning-budget.cpp`,
   `common/parsers/qwen3-coder.cpp`, `tools/server/server.cpp`,
   `tools/server/server-context.cpp`.
3. Diffed each file across the tags and checked every reasoning-related hunk
   against EZLM's usage.
4. Added `internal/engine/flags_reasoning_test.go` and wrote
   `agents/T06/a10/reasoning.md` with the full evidence.

Release tags: `releases/latest` = `v0.9.0` (2026-11-04), whose only asset is
`nightly-tag.txt` containing `b12110`. Newest build by date is `b12144`.
Tag commit dates: b11960 2026-10-28, b12110 2026-11-04, b12144 2026-11-05.

## Findings

**No breaking change for EZLM (info).** Flag names, values and defaults for the
reasoning controls are identical at b11960, b12110 and b12144. EZLM passes no
`--reasoning*` flag and reads only
`/props` `chat_template_caps.reasoning_effort_levels`, which is still emitted.
Details and line numbers: `agents/T06/a10/reasoning.md`.

- `minor`, pre-existing, not a regression: `propThinking`
  (`internal/serve/props.go:80-89`) falls back on caps keys
  `supports_thinking` / `supports_reasoning`, but `jinja::caps::to_map`
  (`common/jinja/caps.cpp:108-119`) emits only `supports_preserve_reasoning` and
  `supports_reasoning_effort` at all three tags. The fallback never fires, so a
  template that thinks without the literal `enable_thinking` reads as
  `Thinking=false` and the writers leave the toggle out. Evidence: test
  `TestFetchPropsThinkingFallsBackToCapsKey`; source `caps.cpp:108-119` at
  b11960/b12110/b12144. Not caused by the upgrade, so no fix in this slice
  (`internal/serve` is read-only for T06).
- `minor`: two reasoning parser fixes landed in v0.9.0, both behind flags EZLM
  does not pass. Budget forcing now also ends when an end tag matches
  (`common/reasoning-budget.cpp:136-142`, PR #31374); reachable only with a
  budget `>= 0`, and EZLM leaves the default `-1`. The qwen3-coder parser now
  consumes the think block under `reasoning_format none` with a
  `response_format` (`common/parsers/qwen3-coder.cpp:74-95`, PR #31380);
  EZLM's default format is `deepseek`, so extraction is unchanged.
- `info`: `--chat-template-kwargs` with `enable_thinking` / `preserve_reasoning`
  now logs a deprecation warning pointing at `--reasoning` /
  `--reasoning-preserve` (`arg.cpp:3588-3595` b11960, `3527-3534` b12110).
  EZLM does not pass the flag.
- `info`: the `--reasoning-format` help text says `default: auto` while
  `common/common.h:666` defaults to `deepseek`; `AUTO` is documented as the same
  behaviour, so the wire result is `message.reasoning_content` either way. Do not
  parse the help text.

## What changed and why

- Added `internal/engine/flags_reasoning_test.go` (one test file for this slice)
  covering EZLM's flag builder and `/props` parse against the b12110 shape. No
  production code changed: no fix was needed and `internal/serve` is read-only
  for this task.
- The test lives in `internal/engine` because T06 owns that package; it imports
  `internal/serve` read-only as an external test package.

## What I could not finish / open questions

- I did not run llama-server (hard rule), so the parser fixes are read from
  source, not observed. The claim "no EZLM impact" rests on EZLM never setting
  `--reasoning-format none` and never setting a budget `>= 0`, which the flag
  builder test pins.
- `b12144` has no release of its own; if the pin moves to the newest build
  rather than to `v0.9.0`/`b12110`, the reasoning surface is still identical
  (checked `arg.cpp` b11960..b12144 and the caps chain).

## Next steps

- If EZLM later passes `--reasoning-format none` for the bench JSON test, expect
  qwen3-coder answers to lose the inline think block (a good change) and update
  the bench note.
- Fix the dead caps fallback in `internal/serve/props.go` when that file is free
  (T10 owns it): either drop the two keys or read the template for `thinking`.
