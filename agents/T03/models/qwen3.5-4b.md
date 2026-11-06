# qwen3.5-4b (Qwen/Qwen3.5-4B)

Template source: `tokenizer.chat_template` in `unsloth/Qwen3.5-4B-GGUF`, file
`Qwen3.5-4B-UD-IQ2_XXS.gguf` (GGUF v3). There is exactly one template, no named
variants. The 2B model ships a byte-identical template.

## How tools are emitted

Tools are not a separate template. When `tools` is non-empty the template opens a
`<|im_start|>system` turn containing `# Tools`, the JSON schemas inside `<tools>...</tools>`,
the XML call format and an `<IMPORTANT>` reminder, then appends the real system prompt
content. Each assistant tool call becomes:

```
<tool_call>
<function=get_weather>
<parameter=city>
Paris
</parameter>
<parameter=unit>
c
</parameter>
</function>
</tool_call>
```

Parallel calls are separate `<tool_call>` blocks in the same assistant turn, and a run of
tool results is folded into one `user` turn with one `<tool_response>` per result. Both
render correctly.

One trap: parameters only render when `tool_call.function.arguments` is a mapping. With
the OpenAI string form (`"arguments": "{\"city\": \"Paris\"}"`) the template emits
`<function=get_weather>\n</function>` with no parameters at all. b11960 avoids this by
converting string arguments to objects when its caps probe sees object-argument use
(`common/jinja/caps.cpp:325-333`, `common/chat.cpp:1532`), which this template does. Any
other client, or an older server without that workaround, silently loses the arguments.

## How thinking is toggled

`enable_thinking`, and only at the generation prompt:

- `enable_thinking=true` renders `<|im_start|>assistant\n thinking\n`.
- `enable_thinking=false` or absent renders `<|im_start|>assistant\n thinking\n\n</think>\n\n`
  (an empty think block, so the model is nudged to answer without reasoning).
- `reasoning_effort` does nothing. All six levels render byte-identical to the render
  without the variable, and so do `reasoning_strength`, `reasoning: {effort: ...}` and
  `thinking: {effort: ...}`. This template never sees the value.
- The one exception is `reasoning_effort: "none"`: b11960 converts it to
  `enable_thinking=false` before rendering (`tools/server/server-common.cpp:1497-1505`), so it
  still turns thinking off. Every other level has no effect at all.

## What breaks

| Input | Result |
|---|---|
| `developer` role message | `raise_exception: Unexpected message role.` |
| system message at index > 0 | `raise_exception: System message must be at the beginning.` |
| history with no plain user turn (tool-only continuation) | `raise_exception: No user query found in messages.` |

All three are real HTTP 500s for the engine. The developer-role case is mostly hidden in
b11960 because `workaround::map_developer_role_to_system` (`common/chat.cpp:1154`) rewrites
`developer` to `system` first: a leading developer message then works, but a
mid-conversation one becomes a non-first system message and hits the second error.

## Recommendation

Never send `reasoning_effort` to this model and expect a change: it is accepted by the
server (b11960 passes any string through and never raises "Unexpected reasoning effort")
but the template ignores it. To turn thinking off, set `enable_thinking=false` through
`chat_template_kwargs`, and do not put a `developer` message after the first turn.
