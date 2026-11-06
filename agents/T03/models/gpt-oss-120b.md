# gpt-oss-120b chat template

Source: `tokenizer.chat_template` in `ggml-org/gpt-oss-120b-GGUF/gpt-oss-120b-MXFP4.gguf`
(read from the GGUF header with range requests). sha256 of the template text:
`a4c9919cbbd4acdd51ccffe22da049264b1b73e59055fa58811a99efbd7c8146`. It is byte-identical to
`models/templates/openai-gpt-oss-120b.jinja` at llama.cpp b11960 and to the gpt-oss-20b template.

## Tools

Tool definitions are rendered as a TypeScript namespace inside the developer turn
(`## functions`, `namespace functions { type NAME = (_: {...}) => any; }`). A call is
`<|start|>assistant to=functions.NAME<|channel|>commentary json<|message|>{"city": "Paris"}<|call|>`.
A tool result is `<|start|>functions.NAME to=assistant<|channel|>commentary<|message|>"18 C, clear"<|end|>`,
with the result JSON-encoded, so string results are quoted. `tools_supported` is true.

The template renders only `message.tool_calls[0]`. A second parallel call in the same assistant
message is silently dropped, and both tool results are then attributed to the first call. llama.cpp
b11960 detects this: `common/jinja/caps.cpp` marks `supports_parallel_tool_calls=false` when
`messages[1].tool_calls[1].function` is unused, so the server reports it rather than hiding it.

Arguments must be an object. Passing the OpenAI wire form (a JSON string) makes `|tojson`
double-encode it (`"{\"city\": \"Paris\"}"`); b11960 normally avoids this by parsing the string to an
object first (`common/chat.cpp`, `func_args_not_string`, gated on `supports_object_arguments`).

## Thinking and reasoning effort

The template never reads `enable_thinking` or `thinking`; rendering with either true or false gives
the same prompt. Thinking is expressed only as a `Reasoning: <value>` line in the system message,
defaulting to `medium`. Every value is interpolated verbatim and never validated, so none, minimal,
low, medium, high, xhigh and max all render (and all differ). The model was trained on low, medium
and high; xhigh, max and minimal are out of distribution.

`reasoning_effort: "none"` on the OpenAI field is intercepted before the template:
`tools/server/server-common.cpp:1497-1504` sets `enable_thinking=false` and erases the kwarg. This
template ignores `enable_thinking`, so the prompt keeps `Reasoning: medium` and reasoning is not
actually shortened. To turn thinking off for gpt-oss, use `reasoning_budget_tokens: 0`, which forces
the analysis channel to end.

## Roles

A first `system` or `developer` message becomes the developer `# Instructions` block; system is
always emitted from the fixed identity text. A `developer` message that is not first is silently
dropped (verified: a system+developer+user conversation renders only the system content). b11960 maps
developer to system for all models except gpt-oss (`common/chat.cpp:1517`), so gpt-oss is the one
model where a later developer turn is lost.

## What breaks

- Assistant content containing `<|channel|>...<|message|>` raises a long `raise_exception` message.
  b11960 patches that check to `{%- if false %}` in `common_chat_templates_init` (`common/chat.cpp`),
  so the error is dead server-side but still fires in plain jinja2.
- A tool message with no preceding assistant tool call raises
  `Message has tool role, but there was no previous assistant message with a tool call!`
- An empty `messages` list raises (the template reads `messages[0]` unconditionally).
- Python jinja2 3.1 returns `Markup` from `|tojson`, so concatenating it with literal text
  HTML-escapes the literals (`&lt;|channel|&gt;`). llama.cpp's own jinja returns a plain string,
  so this is a renderer difference, not a model-template bug; the renders here use the llama.cpp
  behaviour.

## Recommendation

Never rely on parallel tool calls with gpt-oss; issue one call per assistant turn. Stick to
`reasoning_effort` low, medium or high, and use `reasoning_budget_tokens: 0`, not
`reasoning_effort: "none"`, when thinking must be off.
