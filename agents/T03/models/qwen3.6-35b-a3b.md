# qwen3.6-35b-a3b

Repo `Qwen/Qwen3.6-35B-A3B`, first quant file `Qwen3.6-35B-A3B-UD-IQ2_XXS.gguf` from
`unsloth/Qwen3.6-35B-A3B-GGUF`, one template (`tokenizer.chat_template`, no named
variants), read from the GGUF header with range requests. Architecture `qwen35moe`.

## Tools

Same layout as the Qwen3.5 family. Tool definitions go into a synthetic system turn
(`# Tools`, JSON lines inside `<tools>`, fixed instruction block ending in `</IMPORTANT>`),
with a leading system message appended after it. Calls are XML-ish:

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

Two calls in one assistant message render as two consecutive `<tool_call>` blocks. Tool
results are a `user` turn with one `<tool_response>` per result. All of this renders.

## Thinking

Toggled by `enable_thinking`. True or absent appends
`<|im_start|>assistant\n thinking\n`; false appends
`<|im_start|>assistant\n thinking\n\n</think>\n\n`. The template also understands
`preserve_thinking`, but only for assistant history (whether an old ` thinking` block is
kept), not for the generation prompt. A `thinking` variable does nothing.

## Effort

The template never mentions `reasoning_effort`. All six levels render byte-identical to a
plain call, as do the `reasoning.effort` and `thinking.effort` shapes. Effort is silently
ignored: nothing accepted, nothing rejected. b11960's caps detection will report
`supports_reasoning_effort: false` and an empty `reasoning_effort_levels`
(`common/jinja/caps.cpp:569-593`), while the server still renders requests that carry the
field (`common/chat.cpp:1081`). `reasoning_effort: "none"` is converted to
`enable_thinking: false` by the server (`tools/server/server-common.cpp:1497-1503`), which
does turn thinking off.

## What breaks

22 of 23 cases render. The only failure is empty `messages`, which raises
`No messages provided.` (so `continue_final_message` with a one-message conversation
fails, because llama.cpp renders `messages[:-1]`). Otherwise this template is permissive
and silent: unknown roles (for example `narrator`) are dropped without an error,
mid-conversation `system` or `developer` messages are dropped without an error, and a
conversation with no user message renders a lone assistant turn instead of raising.

## Recommendation

Send `enable_thinking: false` rather than `reasoning_effort` to control thinking; effort
levels change nothing here. Because bad roles are dropped silently, validate roles on the
client side or a misplaced system message will simply disappear from the prompt.
