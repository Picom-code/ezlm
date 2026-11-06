# ternary-bonsai-2-4b

Source: `tokenizer.chat_template` in `prism-ml/Ternary-Bonsai-2-4B-gguf`, file
`Ternary-Bonsai-2-4B-Q2_0.gguf`, read from the GGUF header with range requests
(7,756 bytes, one unnamed template). Base is Qwen3.5-4B, so the template is the
Qwen ChatML dialect.

## Tools

Definitions go into a synthetic system turn: `# Tools`, then `<tools>` with one
`tool | tojson` line per tool, then a fixed instruction block. The model is told
to answer with:

```
<tool_call>
<function=get_weather>
<parameter=city>
Paris
</parameter>
</function>
</tool_call>
```

Two calls in one assistant message produce two consecutive `<tool_call>` blocks,
so parallel calls are representable. A tool result is folded into a
`<|im_start|>user` turn as `<tool_response>...</tool_response>`, and consecutive
tool results share one user turn.

Arguments matter: the template iterates `tool_call.arguments|items`, so it needs
a JSON object. With the JSON string that llama.cpp passes by default it dies with
`TypeError: Can only get item pairs from a mapping.` The b11960 server avoids
this because the template pipes each argument value through `tojson`, which makes
`chat_template_caps.supports_object_arguments` true, and then
`func_args_not_string` parses the arguments into an object before rendering. A
caller that applies the template without that caps pass (a plain jinja2 renderer,
an older server, a client that renders prompts itself) breaks on any tool call.

## Thinking

Only `enable_thinking` is read. With `add_generation_prompt`, true renders
`<|im_start|>assistant\n thinking\n`, false renders the empty block
`<|im_start|>assistant\n thinking\n\n</think>\n\n`. The toggle has no effect
without `add_generation_prompt`.

## Effort

None. `reasoning_effort` is never referenced: `none`, `low`, `medium`, `high`,
`xhigh`, `max` and `REASONING_EFFORT_UNKNOWN` all render byte-identically to
`thinking_on`. The b11960 server reports `supports_reasoning_effort=false` in
`/props`, and still passes the variable through harmlessly. So effort is
silently ignored, not accepted and not rejected.

## Roles

System is supported as the first message. A `developer` role raises
`RuntimeError: Unexpected message role.` The b11960 server maps `developer` to
`system` before rendering for every template without `<|channel|>`
(`common/chat.cpp:1518`), so the HTTP API accepts developer messages anyway.

## Recommendation

Never rely on reasoning effort for this model: the value has no effect on the
prompt. If you render this template outside llama.cpp's server, pass tool-call
arguments as a JSON object, or every tool call raises. Do not send a developer
role to a renderer that does not map it to system.
