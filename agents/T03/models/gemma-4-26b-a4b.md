# gemma-4-26b-a4b (Gemma 4 26B A4B, `unsloth/gemma-4-26B-A4B-it-GGUF`)

Read from the GGUF header of `gemma-4-26B-A4B-it-UD-IQ2_XXS.gguf` (one
`tokenizer.chat_template`, no named templates). Render data:
`gemma-4-26b-a4b.renders.json`; raw template: `gemma-4-26b-a4b.templates.json`.
All 21 cases rendered; none raised.

## How tools are emitted

Gemma's own DSL, not JSON:

```
<|tool>declaration:get_weather{description:<|"|>Get the current weather for a city<|"|>,parameters:{properties:{city:{description:<|"|>City name<|"|>,type:<|"|>STRING<|"|>}},required:[<|"|>city<|"|>],type:<|"|>OBJECT<|"|>}}<tool|>
```

An assistant tool call is
`<|tool_call>call:get_weather{city:<|"|>Paris<|"|>}<tool_call|>`; two calls in
one message render as two adjacent `<|tool_call>` blocks (parallel calls are
representable). The tool result follows in the same model turn as
`<|tool_response>response:get_weather{value:<|"|>{"temp_c": 21}<|"|>}<tool_response|>`.
Declarations and the system/developer message share one `<|turn>system` block.

The template can take arguments as a mapping. llama.cpp b11960 sees that (its
capability probe reads `arguments.arg`) and parses the OpenAI JSON string to an
object before rendering, which produces the exact DSL form above. If a caller
passes the arguments as a string anyway, the template strips the outer braces
and inserts the raw text, which is still close to valid DSL but not identical.

## How thinking is toggled

`enable_thinking` (default false in the template; llama.cpp b11960 passes true
by default). True puts `<|think|>` at the top of the system turn and leaves the
`<|turn>model` open; false closes an empty
`<|channel>thought\n<channel|>` before generation. `preserve_thinking`
(default false) decides whether `reasoning_content` in history is kept for
turns before the last user message.

## What breaks

- Nothing raised. `reasoning_effort` is not read anywhere, so all six effort
  levels render byte-identical to `plain`; b11960 reports
  `supports_reasoning_effort = false` for this template.
- An empty `tools` list plus `enable_thinking = true` still emits a system turn
  containing only `<|think|>`; harmless but it means the first prompt tokens
  differ from a thinking-off run even with no system message.

## Recommendation

Do not send `reasoning_effort` to this model; control thinking with
`enable_thinking` (or the server's `reasoning_effort: "none"`, which b11960
turns into `enable_thinking = false`). Send tool arguments as JSON (the server
parses them); send a `developer` message only if you accept that it becomes part
of the system turn.
