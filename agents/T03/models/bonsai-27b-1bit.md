# bonsai-27b-1bit (prism-ml/Bonsai-27B-gguf, Q1_0)

Read from the GGUF header with range requests: one template, `tokenizer.chat_template`
(7764 bytes), no named templates. Rendered with the same setup as llama.cpp b11960's
`scripts/jinja/jinja-tester.py`. 17 of 21 cases rendered, 4 raised.

## Tools

Tools are supported. Definitions go into a generated system turn:

```
<|im_start|>system
# Tools

You have access to the following functions:

<tools>
{"type": "function", "function": {...}}
</tools>
...
</IMPORTANT><|im_end|>
```

The assistant is told to answer with XML:

```
<tool_call>
<function=get_weather>
<parameter=city>
Paris
</parameter>
</function>
</tool_call>
```

Tool results come back as `<tool_response>...</tool_response>` inside a `user` turn.
Two parallel calls render as two consecutive `<tool_call>` blocks in one assistant
turn, and two results merge into one user turn. A user message must exist before the
tool turns, otherwise the template raises `No user query found in messages.`

## Thinking

Thinking is **on by default**. With no `enable_thinking`, the prompt ends
`<|im_start|>assistant\n thinking\n`. `enable_thinking=false` renders
` thinking\n\n</think>\n\n` (closed, empty). `preserve_thinking` is supported for
history. The `thinking` variable is ignored.

## Reasoning effort: silently ignored

The template never reads `reasoning_effort` (or `reasoning_strength`). All of
`none`, `low`, `medium`, `high`, `xhigh`, `max` render byte-identical output, and so
do the `reasoning.effort` and `thinking.effort` shapes. llama.cpp b11960's
`caps_get()` marks `supports_reasoning_effort` only when the rendered context uses
the value (`common/jinja/caps.cpp`, "reasoning effort" case), so this model reports
no effort levels. Sending effort is harmless but does nothing.

## What breaks

| Case | Result |
|---|---|
| `developer` role | raises `Unexpected message role.` |
| system message after the first turn | raises `System message must be at the beginning.` |
| empty messages | raises `No messages provided.` |
| `tool_call.arguments` as a JSON string | python jinja2 raises `Can only get item pairs from a mapping.` |

## Recommendation

Map OpenAI `developer` messages to `system` before rendering, and send no reasoning
effort: it is ignored, so do not advertise effort control for this model. To turn
thinking off, send `chat_template_kwargs: {"enable_thinking": false}`; to keep
reasoning across turns, send `preserve_thinking: true`.
