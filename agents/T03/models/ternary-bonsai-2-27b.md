## ternary-bonsai-2-27b

Source: `tokenizer.chat_template` from the GGUF header of
`prism-ml/Ternary-Bonsai-2-27B-gguf` / `Ternary-Bonsai-2-27B-Q2_0.gguf` (read with HTTP
range requests). No named templates. The template is byte-identical to the upstream
`Qwen/Qwen3.8-27B` `chat_template.jinja` (sha256 `c3cf9e34...`), so it is a Qwen3.8-family
template even though the weights are ternary.

### Tools

Tools are advertised as JSON lines inside a `<tools>` block in the system prompt, and the
model is told to emit calls as XML:

```
<tool_call>
<function=get_weather>
<parameter=city>
Paris
</parameter>
</function>
</tool_call>
```

Two calls in one assistant turn are two adjacent `<tool_call>` blocks. Tool results are
wrapped as `<tool_response>...</tool_response>` inside a `user` turn, and consecutive
results are merged into one user turn. Tool arguments are read with `|items`, so llama.cpp
must normalise `function.arguments` from a JSON string to an object first; it does
(`func_args_not_string`, `common/chat.cpp`). A string that is not valid JSON aborts the
request before rendering.

### Thinking

`enable_thinking` is the toggle. It is on by default (`enable_thinking is undefined or
true`), and when on the default effort is `xhigh`. A plain user message therefore gets an
injected system turn:

```
<|im_start|>system
Reasoning effort is set to xhigh. Please think carefully through the task, ...
<|im_end|>
```

With `enable_thinking=false` there is no system turn and the generation prompt closes an
empty think block: `<|im_start|>assistant\n thinking\n\n</think>\n\n`.

### Effort

The template only accepts `low`, `medium` and `xhigh` (template lines 46-55). `none`,
`high`, `max` and `minimal` raise:

```
Unexpected reasoning effort high. Supported types are xhigh (default), medium, and low.
```

`medium` renders but emits no instruction text, so it is accepted and silently ignored:
the prompt is the same as thinking-on with no effort hint. Effort is only checked while
thinking is on, so `reasoning_effort=high` with `enable_thinking=false` renders fine.

The nested OpenAI shapes `reasoning.effort` and `thinking.effort` are ignored; only the
top-level `reasoning_effort` is read.

### What breaks

- `developer` role: `Unexpected message role.` (template line 160).
- `system` not first: `System message must be at the beginning.`
- No user message: `No user query found in messages.`
- Images or videos in a system message: `System message cannot contain images.`

### Server cross-check (b11960)

- `reasoning_effort=none` never reaches the template: `server-common.cpp:1497-1505` maps it
  to `enable_thinking=false` and erases the kwarg. A client sending `none` gets a
  thinking-off prompt, not an error.
- `high`, `max` and `minimal` are passed through as `chat_template_kwargs`, the template
  raises, and the server returns HTTP 400 `invalid_request_error` with the exact template
  text (`server-context.cpp:5386-5389`, `server-common.cpp:97-100`). The task description
  says HTTP 500; on b11960 it is 400.
- `/props` exposes `chat_template_caps` (`server-context.cpp:5246`, `5688`) and the
  auto-detected `reasoning_effort_levels` are `low`, `medium`, `xhigh`
  (`common/jinja/caps.cpp:592-641`). `none` is not in that list even though the HTTP layer
  accepts it.
- `--reasoning-format` does not change the prompt; it only controls parsing of the output.
  This template's output uses ` thinking` and the XML tool-call tags, which the default
  parser handles.

### Recommendation

Never send `reasoning_effort` values other than `low`, `medium` or `xhigh` to this model
while thinking is on: `high` is the common client default and returns HTTP 400. Treat
`medium` as "thinking on with no effort hint", not as a distinct level. If a client needs
to disable thinking, send `enable_thinking=false` (or `reasoning_effort=none`, which the
server maps to the same thing).
