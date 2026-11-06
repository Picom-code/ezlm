# bonsai-8b-1bit (Bonsai 8B 1-bit, `prism-ml/Bonsai-8B-gguf`)

Read from the GGUF header of `Bonsai-8B-Q1_0.gguf` (one `tokenizer.chat_template`,
no named templates). Render data: `bonsai-8b-1bit.renders.json`; raw template:
`bonsai-8b-1bit.templates.json`. All 21 cases rendered; none raised.

## How tools are emitted

Tools are declared in a system block, one JSON object per line:

```
<|im_start|>system
# Tools
...
<tools>
{"function": {"description": "...", "name": "get_weather", ...}, "type": "function"}
</tools>
...
<tool_call>
{"name": <function-name>, "arguments": <args-json-object>}
</tool_call><|im_end|>
```

An assistant tool call is a `<tool_call>` block with a JSON object
`{"name": ..., "arguments": ...}`; two calls in one message render as two
adjacent `<tool_call>` blocks, so parallel calls are representable. A tool result
is wrapped in `<tool_response> ... </tool_response>` inside a `user` turn, and
consecutive tool results share one user turn.

The tools preamble only appears when `tools` is set, and the caller's system
message is included in that block only when it is `messages[0]`.

## How thinking is toggled

It is not. The template reads only `messages`, `tools` and
`add_generation_prompt`; there is no `enable_thinking`/`thinking` and no
`reasoning_effort`. `thinking_on` and `thinking_off` render byte-identical, and
every effort value renders byte-identical to `plain`. The generation prompt
always ends with an empty ` thinking\n\n</think>` block. History
`reasoning_content` is rendered as a think block only for assistant turns after
the last user message.

## What breaks

- A `developer` role message is silently dropped: the role matches no branch of
  the template, so its text never reaches the model.
- A system message after `messages[0]` is rendered as an inline
  `<|im_start|>system` turn, which the model may treat as a second system
  prompt.

## Recommendation

Never send `reasoning_effort` to this model and expect a change: it is ignored,
so effort-based routing (for example "high for hard prompts") has no effect.
Keep thinking control in the agent, not in the template. If a developer role
matters, fold its text into the first system message.
