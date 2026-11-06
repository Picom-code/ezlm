# qwen3.8-flash-next

Source: `tokenizer.chat_template` in the GGUF header of
`ISTA-DASLab/Qwen3.8-Flash-Next-GSQ-RCO-GGUF`, file
`Q2_0/Qwen3.8-Flash-Next-GSQ-RCO-Q2_0-00001-of-00002.gguf` (header only, range
requests). No named templates. 35 cases rendered, 28 OK, 7 raised.

## Tools

Tools go in one system turn: a `# Tools` preamble, the definitions as one JSON
object per line inside `<tools>...</tools>`, then the calling instructions.
Calls are emitted as

```
<tool_call>
<function=get_weather>
<parameter=city>
Oslo
</parameter>
</function>
</tool_call>
```

Two calls in one assistant message are separated by a newline and each gets its
own `<tool_call>` block. Results are wrapped in `<tool_response>` inside a
synthetic `user` turn, and consecutive tool messages share one user turn.

Tool arguments must be a mapping. If a client passes `arguments` as a JSON
string the render raises `Tool call arguments for function "get_weather" were
passed as a JSON string. Parse them into an object before calling
apply_chat_template.` A call without a function name raises `Tool call is
missing a function name.`

## Thinking

`enable_thinking` is the switch. `false` emits an already closed
` thinking\n\n</think>\n\n` block in the generation prompt, so the model answers
without reasoning. `true` (or unset) opens ` thinking\n`.

## Effort

The template reads `reasoning_effort` and only that name. `reasoning.effort`
and `thinking.effort` shapes render fine and are silently ignored: the output is
byte-identical to the default.

| effort | result |
|---|---|
| `low` | renders, "Reasoning effort is set to low..." |
| `medium` | renders, no reasoning instruction at all (accepted but a no-op) |
| `high` | renders, remapped to `xhigh` inside the template |
| `xhigh` | renders, "Reasoning effort is set to xhigh..." |
| `none` | raises `Unexpected reasoning effort none. Supported types are xhigh (default), medium, and low.` |
| `max` | raises `Unexpected reasoning effort max. Supported types are xhigh (default), medium, and low.` |

`none` and `max` raise, and b11960 turns any template exception into an HTTP
500 with that message, which is the "Unexpected reasoning effort" failure
agent apps hit. Effort is only validated while thinking is on: with
`enable_thinking: false`, `none` and `max` render and are ignored.

## Roles

`system` and `developer` are both accepted, but only at the start of the
conversation. Consecutive leading system/developer messages are merged into one
system turn. A system or developer message anywhere later raises `System message
must be at the beginning.` The role name itself is not re-emitted for system
messages, so a developer message becomes a plain system turn.

## Recommendation

Never send `reasoning_effort` `none` or `max` to this model: it is a 500, not a
fallback. `high` is safe but is the same prompt as `xhigh`, so prefer `high`
when you want the strong instruction and do not expect a different budget.
`medium` is safe and quiet. If a client insists on `none` or `max`, turn
`enable_thinking` off or drop the field before the request reaches the engine.

## b11960 cross-check

`common/chat.cpp` line 1081-1083 passes `reasoning_effort` through and also
sets `reasoning_strength` (same value). This template reads only
`reasoning_effort`, so `reasoning_strength` is unused here.
`common/jinja/caps.cpp` probes levels `minimal, low, medium, high, xhigh, max`
and drops any level whose render equals the render of an unknown effort. `none`
and `max` raise, `high` renders the same as `xhigh`, so the server advertises
`reasoning_effort_levels: ["low", "medium", "xhigh"]` even though `high` is
accepted. `minimal` raises too, and is not a level this template knows.
