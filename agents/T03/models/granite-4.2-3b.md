# granite-4.2-3b (ibm-granite/granite-4.2-3b)

Template source: `tokenizer.chat_template` in `ibm-granite/granite-4.2-3b-GGUF/granite-4.2-3b-Q2_K.gguf` (9,013 bytes, no named templates).

## Tools

The template unwraps `tool.function` and writes a `# Tools` section into the system turn:
`<tools>` followed by one bare JSON function object per line, then `</tools>`, then the
granite instruction block. Calls are the granite 4.x XML form:

```
<tool_call>
<function=read_file>
<parameter=path>
notes.txt
</parameter>
</function>
</tool_call>
```

Parallel calls are just several `<tool_call>` blocks. Tool results become
`<|im_start|>user\n<tool_response>...` turns (consecutive tool messages share one user
turn). Rendered fine for 1 and 2 calls, including object-valued arguments (b11960
converts `arguments` from a JSON string to an object when the template reads it that way).

Quirk: with no system message the template still emits an empty
`<|im_start|>system\n<|im_end|>` turn.

## Thinking

`enable_thinking` (default true). true ends with `<|im_start|>assistant\n<think>\n`;
false ends with `<|im_start|>assistant\n thinking</think>\n`. The switch works.

## Reasoning effort

`reasoning_effort` is read once, as `reasoning_effort == "low"`. Only `low` changes
the render: it appends `\n\n{reasoning effort: low}` to the last user message.
`minimal`, `medium`, `high`, `xhigh`, `max` produce a byte-identical render to the
default and are silently ignored. b11960 therefore reports
`chat_template_caps.reasoning_effort_levels: ["low"]`.

**Recommendation:** only send `reasoning_effort: "low"` when you want the low-effort
hint; any other level is a no-op, and `none` is handled by the server as
`enable_thinking=false` before the template sees it.

## Roles

`system` works. A `developer` message renders as a literal
`<|im_start|>developer` turn, but b11960 maps `developer` to `system` before rendering
for every template without `<|channel|>` (`common/chat.cpp:1523`), so in practice the
template sees `system`.
