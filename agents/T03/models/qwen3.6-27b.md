# qwen3.6-27b

Source: `tokenizer.chat_template` in the GGUF header of
`unsloth/Qwen3.6-27B-MTP-GGUF`, file `Qwen3.6-27B-UD-IQ2_XXS.gguf` (header only,
range requests). No named templates. 35 cases rendered, 33 OK, 2 raised.

## Tools

Same shape as the Qwen3.8 family: one system turn with a `# Tools` preamble,
the definitions as one JSON object per line inside `<tools>...</tools>`, then
`<tool_call><function=NAME><parameter=KEY>VALUE</parameter></function>
</tool_call>`. Parallel calls are separate `<tool_call>` blocks in the same
assistant message. Results go in `<tool_response>` inside a synthetic `user`
turn.

The differences from qwen3.8 are all about validation, and all in the unsafe
direction:

- `arguments` passed as a JSON string are silently dropped, so the render
  contains `<function=get_weather>` with no parameters at all. No error.
- `arguments` that is neither a mapping nor a string is silently dropped too.
- A tool call with no function name raises an undefined-name error from the
  template engine (`UndefinedError: 'dict object' has no attribute 'name'`)
  rather than a readable message. Treat this one as engine-dependent: llama.cpp's
  own template engine may report it differently.

## Thinking

`enable_thinking` only. `false` emits a closed ` thinking\n\n</think>\n\n` block
in the generation prompt; anything else opens ` thinking\n`. The template also
understands `preserve_thinking` (and the b11960 aliases `clear_thinking`,
`truncate_history_thinking`, `drop_thinking` are not read here) when replaying
assistant history, and it recovers `reasoning_content` from a `</think>` marker
inside assistant content when the structured field is absent.

## Effort

The template never reads `reasoning_effort`. All six values (`none`, `low`,
`medium`, `high`, `xhigh`, `max`) render byte-identical output, and so do
`reasoning.effort` and `thinking.effort`. The variable is silently ignored, not
accepted: there is no way to ask this model for a different reasoning budget
through the template.

This is the safe failure mode. Sending `max` or `none` here cannot produce the
HTTP 500 that the Qwen3.8 template produces.

## Roles

`system` and `developer` are accepted as the first message, and a second
consecutive system/developer message is merged with it. A system or developer
message later in the conversation is silently dropped: no error, no warning, the
text simply does not reach the prompt. qwen3.8 raises in that case; this one
does not, so a client that appends a mid-conversation developer message loses it
without knowing.

## Recommendation

No effort value can break this model, so do not spend logic avoiding one. The
thing to protect against is a silently dropped late `developer` message and a
silently empty tool call: keep developer messages at the start, and pass tool
arguments as an object, not as a JSON string.

## b11960 cross-check

`common/jinja/caps.cpp` only probes effort levels when the template reads
`reasoning_effort`; this one never does, so b11960 reports
`reasoning_effort_levels: []` and `supports_reasoning_effort: false`. The
`reasoning_strength` variable that `common/chat.cpp` sets alongside
`reasoning_effort` is unused here too. No `--chat-template-file` override is
needed for correct rendering.
