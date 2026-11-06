# qwen3.5-9b (unsloth/Qwen3.5-9B-GGUF, UD-IQ2_XXS)

Read from the GGUF header with range requests: one template, `tokenizer.chat_template`
(7816 bytes), no named templates. Rendered with the same setup as llama.cpp b11960's
`scripts/jinja/jinja-tester.py`. 18 of 21 cases rendered, 3 raised.

## Tools

Tools are supported and use the same XML dialect as Bonsai:

```
<tools>
{"type": "function", "function": {...}}
</tools>
```

The assistant is told to answer with `<tool_call><function=NAME><parameter=K>V</parameter></function></tool_call>`.
Results come back as `<tool_response>...</tool_response>` in a `user` turn. Two
parallel calls render as two consecutive `<tool_call>` blocks; consecutive tool
results merge into one user turn.

One difference matters: the template guards parameters with
`tool_call.arguments is mapping`. If a server passes the arguments as a JSON
string (some OpenAI clients do), the guard is false and every parameter is
silently dropped. The rendered call is then:

```
<tool_call>
<function=get_weather>
</function>
</tool_call>
```

The model gets no arguments and the tool call is useless. No exception is raised.

## Thinking

Thinking is **off by default**. With no `enable_thinking` the prompt ends
`<|im_start|>assistant\n thinking\n\n</think>\n\n`. `enable_thinking=true` ends
`<|im_start|>assistant\n thinking\n` and lets the model reason.
The `thinking` variable is ignored. There is no `preserve_thinking` support: this
GGUF ships an older Qwen3.5 template than b11960's bundled
`models/templates/Qwen3.5-4B.jinja`, which defaults thinking on and supports
`preserve_thinking`. llama.cpp uses the GGUF template, not the bundled one, so the
server behaviour follows this file.

## Reasoning effort: silently ignored

The template never reads `reasoning_effort`. `none`, `low`, `medium`, `high`,
`xhigh`, `max` all render byte-identical output, as do `reasoning.effort` and
`thinking.effort`. b11960's `caps_get()` would report
`supports_reasoning_effort=false` and no effort levels (`common/jinja/caps.cpp`).

## What breaks

| Case | Result |
|---|---|
| `developer` role | raises `Unexpected message role.` |
| system message after the first turn | raises `System message must be at the beginning.` |
| empty messages | raises `No messages provided.` |
| `tool_call.arguments` as a JSON string | renders, but drops every parameter silently |

## Recommendation

Send tool arguments as a JSON object, never as a string. Map `developer` to
`system` before rendering. Effort is ignored, so do not advertise effort control
for this model; toggle thinking with `chat_template_kwargs: {"enable_thinking": true}`.
