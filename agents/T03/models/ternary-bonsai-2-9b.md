# ternary-bonsai-2-9b

Repo `prism-ml/Ternary-Bonsai-2-9B-gguf`, file `Ternary-Bonsai-2-9B-Q2_0.gguf`, one
template (`tokenizer.chat_template`, no named variants), read from the GGUF header with
range requests. Architecture `qwen35`, base Qwen3.5-9B.

## Tools

Tool definitions go into a synthetic system turn: `# Tools`, then each definition as a
single JSON line inside `<tools>...</tools>`, then a fixed instruction block that ends in
`</IMPORTANT>`. If the conversation also has a leading system message, its content is
appended after `</IMPORTANT>` in the same turn. Calls are emitted as XML-ish blocks, not
JSON:

```
<|im_start|>assistant
 thinking

</think>

<tool_call>
<function=get_weather>
<parameter=city>
Paris
</parameter>
</function>
</tool_call><|im_end|>
```

Two calls in one assistant message render as two consecutive `<tool_call>` blocks with no
separator. Tool results are wrapped as a `user` turn:
`<|im_start|>user\n<tool_response>\n...\n</tool_response><|im_end>`. Parallel tool
results share one user turn with one `<tool_response>` per result. All of this renders.

## Thinking

Toggled by `enable_thinking`. True or absent appends
`<|im_start|>assistant\n thinking\n`; false appends
`<|im_start|>assistant\n thinking\n\n</think>\n\n`. The template has no `thinking` or
`reasoning` variable, so sending those kwarg names does nothing.

## Effort

The template never mentions `reasoning_effort`. All six levels (none, low, medium, high,
xhigh, max) render byte-identical to a plain call, and so do the `reasoning.effort` and
`thinking.effort` shapes. Effort is silently ignored: nothing is accepted and nothing is
rejected. llama.cpp b11960 will therefore report `supports_reasoning_effort: false` and an
empty `reasoning_effort_levels` for this model (`common/jinja/caps.cpp:569-593`), and a
request that sends `reasoning_effort` will still render, because the server only puts the
value into the template context (`common/chat.cpp:1081`).

One exception: the server turns `reasoning_effort: "none"` into `enable_thinking: false`
before rendering (`tools/server/server-common.cpp:1497-1503`), so "none" does switch
thinking off here even though the template itself ignores the variable.

## What breaks

- `developer` role raises `Unexpected message role.` The server maps developer to system
  before rendering for every model except GPT-OSS (`common/chat.cpp:1517`), so through the
  server API it is fine; a direct template render is not.
- Any other role (for example `narrator`) raises the same error.
- A system message that is not `messages[0]` raises `System message must be at the
  beginning.`
- A conversation with no user message raises `No user query found in messages.`
- Empty `messages` raises `No messages provided.` That matters for
  `continue_final_message` with a one-message conversation, where llama.cpp renders
  `messages[:-1]` (`common/chat.cpp:1501-1502`) and the template then sees an empty list.

18 of 23 render cases pass. The five failures are the developer role, the unknown role,
the mid-conversation system message, the no-user conversation and empty messages; all five
are the template's own `raise_exception` calls.

## Recommendation

Send `enable_thinking: false` to turn reasoning off and do not rely on `reasoning_effort`
for this model: no level changes the prompt, so a client that asks for "low" gets full
thinking. Do not send a `developer` role when rendering the template outside the server.
