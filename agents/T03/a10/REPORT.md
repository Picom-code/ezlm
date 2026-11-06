# T03 / a10 report: gpt-oss-120b and gpt-oss-20b

## What I did

- Read `tokenizer.chat_template` from the GGUF header of the first catalog quant, with HTTP range
  requests only (`scripts/verify_catalog.py` `header()`):
  `ggml-org/gpt-oss-120b-GGUF/gpt-oss-120b-MXFP4.gguf` and
  `ggml-org/gpt-oss-20b-GGUF/gpt-oss-20b-MXFP4.gguf`. No whole file downloaded.
- Saved the raw template to `models/<id>.templates.json` (exact text; one named template only,
  `tokenizer.chat_template`; no `tokenizer.chat_template.tool_use`).
- Rendered every required case with Python jinja2 3.1.6 and llama.cpp conventions
  (`raise_exception`, `strftime_now`, plain-string `tojson` as in `common/jinja/value.cpp`).
  Script: `agents/T03/a10/render.py`; output: `models/<id>.renders.json`.
- Cross-checked llama.cpp b11960: `common/chat.cpp`, `common/jinja/caps.cpp`, `common/chat.h`,
  `common/parsers/gpt-oss.cpp`, `tools/server/server-common.cpp`, `tools/server/server-chat.cpp`,
  and `models/templates/openai-gpt-oss-120b.jinja`.

Both models ship the **same** 16,714-byte template, sha256
`a4c9919cbbd4acdd51ccffe22da049264b1b73e59055fa58811a99efbd7c8146`, byte-identical to b11960's
built-in `openai-gpt-oss-120b.jinja`.

## Render results

24 cases ok, 2 raised (raw template), 1 extra b11960-patched case ok, identical for both models.

| case | result |
|---|---|
| plain, system_user, tool_call, parallel_tools | ok |
| thinking_on / thinking_off | ok, **identical output** (kwarg ignored) |
| effort none/minimal/low/medium/high/xhigh/max | all ok, all distinct (`Reasoning: <value>`) |
| effort via `reasoning.effort` / `thinking.effort` | ok but **ignored** (output = default medium) |
| developer_role (first message) | ok, rendered as `# Instructions` |
| developer_role_second | ok but the developer content is **silently dropped** |
| no_generation_prompt | ok |
| tool_args_string | ok but arguments double-encoded (`"{\"city\": ...}"`) |
| tool_result_first | raises `Message has tool role, but there was no previous assistant message with a tool call!` |
| assistant_channel_tags | raises the `<|channel|>` in content error; ok with b11960's `{%- if false %}` patch |

## Findings

1. **Major - parallel tool calls are silently truncated.** The template renders only
   `message.tool_calls[0]` (`gpt-oss template, "We assume max 1 tool call per message"`), so a second
   call is dropped and both results are attributed to the first call. Evidence:
   `models/gpt-oss-120b.renders.json` `parallel_tools` shows one `to=functions.get_weather` call and
   two `functions.get_weather` results, never `get_time`. b11960 agrees: `common/jinja/caps.cpp`
   sets `supports_parallel_tool_calls=false` when `messages[1].tool_calls[1].function` is unused.

2. **Major - `reasoning_effort: "none"` does not turn thinking off for gpt-oss.**
   `tools/server/server-common.cpp:1497-1504` intercepts `"none"`, sets `enable_thinking=false` and
   erases the kwarg. This template never reads `enable_thinking` (thinking_on and thinking_off render
   identically), so the prompt keeps `Reasoning: medium`. The only working switch is
   `reasoning_budget_tokens: 0`.

3. **Major - a developer message after the first is lost.** Only `messages[0]` of role
   system/developer becomes `# Instructions`; later developer messages are not rendered at all
   (`developer_role_second` output contains only "You are helpful."). b11960 maps developer to
   system for every model except gpt-oss (`common/chat.cpp:1517`), so gpt-oss is uniquely exposed.

4. **Minor - effort levels are interpolated, never validated.** All of none, minimal, low, medium,
   high, xhigh, max render and produce different text, so the template cannot raise on an unknown
   effort. But the model was trained on low/medium/high; minimal/xhigh/max are out of distribution.
   Recommendation: send low, medium or high only.

5. **Minor - empty `messages` raises `UndefinedError`** because the template reads `messages[0]`
   unconditionally.

6. **Minor - renderer difference, not a model bug.** Python jinja2 3.1's `|tojson` returns `Markup`,
   so `"...<|channel|>..." + content|tojson + "..."` HTML-escapes the literals
   (`&lt;|channel|&gt;`). llama.cpp's jinja (`common/jinja/value.cpp` `tojson`) returns a plain
   string. My renders use the llama.cpp behaviour.

7. **Minor - tool arguments must be an object.** A JSON string argument is double-encoded by
   `|tojson`; b11960 normally converts it first (`common/chat.cpp` `func_args_not_string`, gated on
   `supports_object_arguments`), so this only bites raw-template users.

8. **Info - b11960 patches the channel-tag error out** of this template in
   `common_chat_templates_init` (`common/chat.cpp`, replaces the `{%- if ... %}` with
   `{%- if false %}`), so the first raise_exception case is dead server-side.

9. **Info - premise mismatch: "Unexpected reasoning effort" does not exist in b11960.** A grep of
   `common/` and `tools/server/` finds no such error. The server passes any `reasoning_effort` string
   straight to the template (`server-common.cpp:1497-1504`, `common/chat.cpp:1081-1083`) and the
   template accepts any string. b11960's caps report `reasoning_effort_levels` =
   minimal/low/medium/high/xhigh/max (`common/jinja/caps.cpp:596-640`) but that is informational,
   not enforced. If a 500 "Unexpected reasoning effort" is seen, it comes from a client or a
   different build, not from this template.

## Recommendation

For gpt-oss: one tool call per assistant turn; only low/medium/high effort; use
`reasoning_budget_tokens: 0` to disable thinking, never `reasoning_effort: "none"`; keep the
developer instruction as the first message.

## Sources

- `https://huggingface.co/ggml-org/gpt-oss-120b-GGUF/resolve/main/gpt-oss-120b-MXFP4.gguf` (header only)
- `https://huggingface.co/ggml-org/gpt-oss-20b-GGUF/resolve/main/gpt-oss-20b-MXFP4.gguf` (header only)
- `https://raw.githubusercontent.com/ggml-org/llama.cpp/b11960/{common/chat.cpp,common/chat.h,common/jinja/caps.cpp,common/jinja/value.cpp,common/parsers/gpt-oss.cpp,tools/server/server-common.cpp,models/templates/openai-gpt-oss-120b.jinja}`

## Open questions

- Does EZLM or the client app send `reasoning_effort: "none"` and expect thinking off? If so, that is
  a real bug on gpt-oss; the fix belongs in the server config (budget 0), not the template.
- Does any EZLM path send parallel tool calls to gpt-oss? The template cannot represent them.
