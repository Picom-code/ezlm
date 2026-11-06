## granite-4.2-8b

IBM, Apache-2.0, 8.8B dense, 128K context. The chat template is the only
template in the GGUF header (`tokenizer.chat_template`, 9013 bytes) and is
byte-identical to the 30B model's template.

**Tools.** Tool definitions are rendered as one JSON object per line inside a
`<tools>` block in the system turn, followed by a long instruction block that
tells the model to answer with
`<tool_call><function=NAME><parameter=KEY>VALUE</parameter></function></tool_call>`.
Results come back wrapped in `<tool_response>` inside a `user` turn. The
template reads `message.tool_calls[].function.arguments` as a mapping, so
nested values are JSON-encoded with `tojson`; a JSON string instead of an
object raises `Can only get item pairs from a mapping.` Parallel calls render
fine, one `<tool_call>` block each.

**Thinking.** `enable_thinking` (default true) is the only switch. False makes
the generation prompt end with `<think></think>` (an empty closed think block)
instead of an open `<think>`. In history, every assistant turn that has no
think tags gets `<think></think>` prepended, and `truncate_history_thinking`
(default true) drops reasoning from earlier turns.

**Effort.** Only `reasoning_effort: "low"` is read: it appends
`{reasoning effort: low}` to the last user message. `none`, `minimal`,
`medium`, `high`, `xhigh` and `max` all render exactly the same prompt as no
effort at all, so they are silently ignored rather than rejected. b11960's
caps probe agrees: `reasoning_effort_levels` is `["low"]`. The server maps
`reasoning_effort: "none"` to `enable_thinking: false` before rendering, so
"none" does turn thinking off, just not through the effort path.

**What breaks.** Nothing in the required cases: all 19 render cases succeed.
Two quirks are worth knowing. First, a system turn is always emitted, even for
a plain user-only chat, because the template sets `system_message = ""` and
then checks `is defined`; the prompt starts with an empty
`<|im_start|>system<|im_end|>` turn. Second, rendering leaks whitespace into
the prompt (a leading newline, and stray spaces/newlines before an assistant
tool-call turn).

**Recommendation.** Send only `reasoning_effort: "low"` if you want a
noticeable change; do not expect `medium`, `high` or `max` to do anything for
this model. Use `enable_thinking` for on/off. There is no "Unexpected
reasoning effort" 500 for this template on b11960: any effort string renders.
