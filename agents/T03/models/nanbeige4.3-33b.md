# Nanbeige4.3 33B (`nanbeige4.3-33b`)

Source: `tokenizer.chat_template` from `bartowski/Nanbeige_Nanbeige4.3-33B-GGUF` / `Nanbeige_Nanbeige4.3-33B-IQ2_XXS.gguf` (GGUF v3, header read with range requests; the catalog's first quant overrides the repo, so that is the file read). No named `tokenizer.chat_template.*` variants. Raw template: `nanbeige4.3-33b.templates.json`; all 23 renders (20 rendered, 3 raised): `nanbeige4.3-33b.renders.json`.

## How it works

A custom ChatML template, not a Qwen one.

- **Tools.** Tool definitions go into a `<tools>` block with `tojson`, and the default call format is XML: `<tool_call><function=NAME><parameter=KEY>value</parameter></function></tool_call>`. If the template variable `tool_call_format` is set to `json`, both the instructions and the emitted calls switch to `<tool_call>{"name": ..., "arguments": {...}}</tool_call>`. Tool results are folded into a user turn as `<tool_response>...</tool_response>`; parallel calls render as consecutive blocks.
- **Thinking.** Toggled only by `enable_thinking` (true: `assistant\n thinking\n`; false: `assistant\n</think>\n\n`). `preserve_thinking` controls whether `reasoning_content` is kept in earlier assistant turns. The variable `thinking` is ignored.
- **Effort.** `reasoning_effort` is never read. `none`, `low`, `medium`, `high`, `xhigh`, `max` and a nested `reasoning.effort` all render byte-identical (sha256 prefix `abb9db5e49f2`). They are silently ignored, not rejected.
- **Roles.** `system` works but must be first. A `developer` message does not raise: it renders as a `<|im_start|>developer` turn, after the template's own default system prompt.
- **Default system prompt.** When the first message is not a system turn, the template injects `你是南北阁，一款由BOSS直聘自主研发并训练的专业大语言模型。`. With tools it injects a longer Chinese tool-use preamble instead.

## What breaks

- System message not first: `Jinja Exception: System message must be at the beginning.`
- Tool calls with `function.arguments` as a JSON string: `arguments|items` needs an object. llama-server rewrites arguments to objects because caps reports `supports_object_arguments`.
- No user turn is tolerated (the template renders), unlike the Bonsai template.
- A system image item does not raise; it is replaced with `<reminder>You are unable to process this image ...</reminder>`.

## Recommendation

Treat this model as having no effort levels: offer `enable_thinking` only, because any `reasoning_effort` value is silently ignored. Always send an explicit system message, otherwise a Chinese default system prompt is prepended. If a client wants JSON tool calls, pass `tool_call_format=json` through `chat_template_kwargs`; otherwise the XML format is what the template asks for.

## llama.cpp b11960 cross-check

- `tools/server/server-common.cpp:1497` maps `reasoning_effort: "none"` to `enable_thinking=false` and forwards other values as template kwargs. This template ignores `reasoning_effort`, so no value raises; `common/jinja/caps.cpp:567-641` reports `supports_reasoning_effort=false` with no levels.
- `common/chat.cpp:1516-1519` maps `developer` to `system` before rendering (the template has no `<|channel|>`), so the developer turn shown by a raw render never reaches the model through llama-server.
- `tool_call_format` is not a llama.cpp capability; it only takes effect if the client sends it in `chat_template_kwargs` (`common/chat.cpp:1062-1068` merges `extra_context` into the template context). `/props` does not advertise it.
- No `Unexpected reasoning effort` string exists in `common/chat.cpp`, `common/jinja/caps.cpp` or `tools/server/*.cpp` at b11960; it is produced by EZLM's fake server (`internal/fakellm/fakellm.go:232`) and matched in `internal/integrate/*`. This model cannot produce that 500 from its template.
