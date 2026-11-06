# T06/a11 REPORT: HTTP surface (`/props`, `/slots`, `/v1/messages`, OpenAI endpoints)

Slice: the server's HTTP surface as EZLM reads it. Compared pinned `b11960` with the latest
release `v0.9.0` (2026-11-04); `b12144` (newest build by date) was spot-checked and matches
`v0.9.0` for this surface.

Method: `diff -rq` over `tools/server/*.cpp|h|md` fetched at both tags, plus `common/jinja/caps.h`,
`common/jinja/caps.cpp`, `common/chat.cpp`, `common/chat.h`, `common/common.h` and
`common/arg.cpp`. GET only; nothing was built, downloaded as an archive, or run.

## Findings

### 1. `/props` gains `voices`; nothing else EZLM reads changed (severity: none)

`get_res_props` at `tools/server/server-context.cpp:5666` (v0.9.0) emits the same keys as at
`:5657` (b11960). The one addition is `props["voices"]` at `:5709`, set from
`meta.tts_voices` (`server-context.h:30`, built at `server-context.cpp:5227-5235`) and only
when the model has predefined TTS voices. EZLM decodes into a fixed struct
(`internal/serve/props.go:40-52`) and ignores unknown keys, so the upgrade is additive and safe.

Every field EZLM reads is present at both tags with the same name and shape:

| EZLM read | EZLM site | v0.9.0 |
|---|---|---|
| `default_generation_settings.n_ctx` | `internal/serve/props.go:42` | `server-context.cpp:5680` |
| `total_slots` | `props.go:44` | `:5681` |
| `chat_template` | `props.go:45` | `:5696` |
| `chat_template_caps` | `props.go:46` | `:5697` |
| `modalities.vision` | `props.go:47-49` | `:5685-5689` |
| `build_info` | `props.go:50` | `:5700` |
| `chat_template_caps.reasoning_effort_levels` | `props.go:66` | `common/chat.cpp:1953` |

### 2. EZLM's thinking fallback reads caps keys that never existed (severity: minor)

`propThinking` (`internal/serve/props.go:76-84`) accepts `chat_template_caps.supports_thinking`
or `supports_reasoning`. Neither key is emitted: `jinja::caps::to_map()`
(`common/jinja/caps.cpp:108-120`, identical at both tags) reports `supports_tools`,
`supports_tool_calls`, `supports_system_role`, `supports_parallel_tool_calls`,
`supports_preserve_reasoning`, `supports_reasoning_effort`, `supports_string_content`,
`supports_typed_content`, `supports_object_arguments`. `supports_thinking` exists only as a
C++ field in `common/chat.h:310`, never in the JSON.

Effect: for a template that thinks but does not spell `enable_thinking` literally, EZLM records
`thinking=false` and the app hides the thinking toggle. Pre-existing at b11960; the upgrade does
not change it, but it is the one place where EZLM's `/props` parsing is genuinely at risk.
Evidence: `TestFetchPropsThinkingCapsGap` in `internal/serve/props_v090_test.go` pins the
current (wrong) behaviour.

Smallest fix, when `internal/serve/props.go` is out of its rewrite freeze: add
`"supports_reasoning_effort"` (and optionally `"supports_preserve_reasoning"`) to the key list
in `propThinking`, then flip that test.

### 3. `/slots` is read without `--slots` (severity: minor, cross-check with a12)

`internal/app/live.go:127` GETs `/slots`, but `Config.Args()` (`internal/serve/config.go:114-128`)
never passes `--slots`. The endpoint answers only when `endpoint_slots` is true; it defaults to
true at both tags (`common/common.h:688`), and the handler returns HTTP 501 otherwise
(`server-context.cpp:5804-5807`). So progress works today by default only. The slot JSON is
byte-identical between tags (`server-context.cpp:808-834`): `is_processing`, `n_prompt_tokens`,
`n_prompt_tokens_processed`, `n_prompt_tokens_cache`, `next_token[0].n_decoded` all unchanged,
so EZLM's decoder (`live.go:133-140`) is safe. Passing `--slots` explicitly would make it robust;
that is a `config.go` change, outside this slice.

### 4. `/v1/messages` and the OpenAI endpoints are unchanged (severity: none)

`/v1/messages` and `/v1/messages/count_tokens` are registered at the same paths (`server.cpp:190`,
`:208`) and their code did not change. The only `server-chat.cpp` change is the Responses API
`extra_params` merge (`server-chat.cpp:309-320`), which EZLM does not call. `/v1/models`,
`/health` and `/v1/chat/completions` are unchanged.

Other `/tts` changes (`server-context.cpp:6486-6550`: `voice` validated against `/props.voices`,
first voice is the default, `speaker_ref` rejected without a voice encoder) are documented in
`README.md:857-862` but EZLM never calls `/tts`.

## What changed in the repo

- `internal/serve/props_v090_test.go` (new): `TestFetchPropsV090` feeds a complete v0.9.0
  `/props` body, including `voices`, `is_sleeping`, the real caps key set and
  `reasoning_effort_levels`, and asserts context, slots, vision, build and levels still parse.
  `TestFetchPropsThinkingCapsGap` pins the dead caps fallback from finding 2.
- `agents/T06/a11/http.md`: the schema table and the full change list.
- No production code touched; no `engine.go`/`pinned.go` change was needed for this slice.

Note for the merge: `internal/serve/**` is being rewritten tonight (T07/T10). This is a new test
file that only uses the exported `Config`/`FetchProps` surface; if `props.go` is reshaped, the
file may need a one-line adjustment. Nothing else in the slice depends on it.

## Verification

- `gofmt -l internal/serve/props_v090_test.go` empty.
- `go test -p 2 -run TestFetchProps ./internal/serve/` passes (new and existing tests).
- Full `go build ./...`, `go vet ./...`, `go test -p 2 ./internal/serve/` run before the final
  commit.

## Not finished / next steps

- Did not verify the Anthropic `/v1/messages` request/response schema field by field; EZLM does
  not call it, so it is out of EZLM's blast radius. The README diff shows no schema edit.
- Did not exercise a real server (no GPU load allowed); the v0.9.0 body in the test is copied
  from the source at the tag.
- Suggested follow-ups for the upgrade PR: fix finding 2 in `props.go` and add `--slots` to
  `Config.Args()` (finding 3).
