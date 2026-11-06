# qwen3.5-122b-a10b (Qwen/Qwen3.5-122B-A10B)

Template source: `tokenizer.chat_template` in
`unsloth/Qwen3.5-122B-A10B-MTP-GGUF/Qwen3.5-122B-A10B-UD-IQ2_XXS.gguf` (7,992 bytes,
no named templates). Same shape as the built-in `models/templates/Qwen3.5-4B.jinja` in
llama.cpp b11960.

## Tools

`tools` are emitted as a `# Tools` block in the system turn. Unlike granite, each tool
is the full OpenAI object (`{"type":"function","function":{"name":...}}`), not the bare
function. Calls use the qwen3-coder XML form:

```
<tool_call>
<function=read_file>
<parameter=path>
notes.txt
</parameter>
</function>
</tool_call>
```

Parallel calls render as consecutive `<tool_call>` blocks. Tool results become
`<|im_start|>user\n<tool_response>...` turns. All tool cases rendered fine.

Note: the template tests `{%- if tools and ... %}` and jinja sees `tools` as undefined
when the request has none; b11960 treats undefined as falsy, so no error. Strict jinja2
would raise, which is a renderer artifact, not a server bug.

## Thinking

`enable_thinking` (default true). true ends with `<|im_start|>assistant\n<think>\n`;
false ends with `<|im_start|>assistant\n<think>\n\n</think>\n\n`. The switch works.

## Reasoning effort

The template never reads `reasoning_effort` or `reasoning_strength`. Every level
(`none` through `max`) rendered byte-identically to the default. b11960's capability
probe therefore sets `supports_reasoning_effort: false` and
`chat_template_caps.reasoning_effort_levels: []` for this model.

**Recommendation:** do not advertise thinking levels for this model. Any
`reasoning_effort` value is accepted by the server and silently ignored by the template;
`none` only works because the server turns it into `enable_thinking=false`.

## Roles

`system` works. A leading `developer` message is merged into the system turn. A
`developer` message later in the list is dropped entirely by the
`message.role != "developer"` loop guard. b11960 maps `developer` to `system` before
rendering anyway (`common/chat.cpp:1523`).
