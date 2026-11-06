# T06/a11: llama.cpp HTTP surface, b11960 to v0.9.0

Scope: `/props`, `/slots`, `/v1/messages` and the OpenAI-compatible endpoints, as far as EZLM
reads them. Compared tag `b11960` (pinned) with tag `v0.9.0` (`releases/latest`, 2026-11-04).
Newest build by date, `b12144`, was spot-checked and is the same for this surface.

Method: fetched `tools/server/*.cpp|h|md` and `common/jinja/caps.{h,cpp}` at both tags from
raw.githubusercontent.com and diffed them (`diff -rq`). Four source files differ under
`tools/server/`: `server-chat.cpp`, `server-context.cpp`, `server-context.h`, `server-decision.cpp`,
`server-models.cpp`, `server-tools.cpp`, plus `README.md`. None of the differences touch a JSON field
EZLM parses, except the additive `/props.voices` below.

## Schema EZLM reads, and what changed

| Endpoint | Field EZLM reads | EZLM site | b11960 | v0.9.0 | Change |
|---|---|---|---|---|---|
| GET `/props` | `default_generation_settings.n_ctx` | `internal/serve/props.go:42` | `server-context.cpp:5671` | `:5680` | none |
| GET `/props` | `total_slots` | `props.go:44` | `:5672` | `:5681` | none |
| GET `/props` | `chat_template` | `props.go:45` | `:5687` | `:5696` | none |
| GET `/props` | `chat_template_caps.reasoning_effort_levels` | `props.go:66` | `common/chat.cpp:1953` | same | none |
| GET `/props` | `chat_template_caps.supports_thinking` / `supports_reasoning` | `props.go:80-84` | absent from `caps::to_map()` | absent | none (already dead, see finding 2) |
| GET `/props` | `modalities.vision` | `props.go:47-49` | `:5676-5680` | `:5685-5689` | none |
| GET `/props` | `build_info` | `props.go:50` | `:5691` | `:5700` | none |
| GET `/props` | (new) `voices` | not read | absent | `:5708-5710` | additive, audio models only |
| GET `/slots` | `is_processing`, `n_prompt_tokens`, `n_prompt_tokens_processed`, `n_prompt_tokens_cache`, `next_token[0].n_decoded` | `internal/app/live.go:133-140` | `server-context.cpp:808-834` | `:808-834` | none (identical) |
| GET `/health` | status 200 | `internal/serve/client.go:29` | `server.cpp:171` | `:171` | none |
| GET `/v1/models` | `data[].id` | `client.go:43-56` | `server-context.cpp:5645-5653` | same | none |
| POST `/v1/chat/completions` | (EZLM only proxies it) | `internal/fakellm` fake | `server-chat.cpp` | `server-chat.cpp` | none for EZLM |
| POST `/v1/messages` | not used by EZLM | - | `server.cpp:190` | `:190` | none (README text only) |

`/v1/messages` is unchanged in code between the tags: the only README edits are the `/tts` `voice`
and `speaker_ref` semantics and a new `extra_params` field on the Responses API
(`README.md:1655-1691`), which EZLM does not call.

## What actually changed (full list, for the record)

1. `/props` gains `voices` (array of strings) when the loaded audio-generation model has predefined
   voices (`server-context.cpp:5704-5710`, `server-context.h:30`, `server-context.cpp:5227-5235`).
   EZLM decodes into a struct and ignores unknown keys, so this is additive.
2. `/tts` now validates `voice` against `/props.voices` and defaults to the first voice
   (`server-context.cpp:6486-6550`). EZLM does not call `/tts`.
3. Responses API accepts `extra_params` and merges it into the chat-completions body
   (`server-chat.cpp:309-320`). EZLM does not call `/v1/responses`.
4. `server-models.cpp:408` replaces `fs_path_to_utf8()` with `.string()`; no wire change.
5. `server-decision.cpp`, `server-tools.cpp`, `server-context.cpp` changes are internal
   (speculative-decoding rollback, TTS voice plumbing, checkpoint ownership).

## Verdict

The HTTP surface EZLM reads is source-identical between `b11960` and `v0.9.0`. Nothing in the
upgrade breaks parsing. Two pre-existing parsing risks are worth fixing while the pin moves, both
already present at `b11960`:

- `/props.chat_template_caps` has never contained `supports_thinking` or `supports_reasoning`;
  the real key is `supports_reasoning_effort`. EZLM's caps fallback is dead code
  (`internal/serve/props.go:80-84`).
- EZLM reads `/slots` but never passes `--slots`; it works only because
  `endpoint_slots = true` by default (`common/common.h:688` at both tags).

## Evidence

- `internal/serve/props_v090_test.go`: `TestFetchPropsV090` parses a full v0.9.0 `/props` body
  (including `voices`, `is_sleeping`, the real caps keys) and asserts every field EZLM uses.
  `TestFetchPropsThinkingCapsGap` pins the dead-caps-fallback behaviour.
- Raw diffs were produced from `tools/server` and `common/jinja` at both tags; no archive was
  downloaded and no server was started.
