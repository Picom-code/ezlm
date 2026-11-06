# mistral-small-4-119b (mistralai/Mistral-Small-4-119B-2603, GGUF from unsloth/Mistral-Small-4-119B-2603-GGUF)

Source: `tokenizer.chat_template` in the GGUF header of
`Mistral-Small-4-119B-2603-UD-IQ1_M.gguf`, read with range requests. One
template, no named variants. It is the Mistral v3/v7 "Unsloth fixed" template:
`<s>[SYSTEM_PROMPT]...[/SYSTEM_PROMPT][AVAILABLE_TOOLS]...[MODEL_SETTINGS]...[INST]...[/INST]`.

## Tools

Definitions are one JSON array inside `[AVAILABLE_TOOLS]...[/AVAILABLE_TOOLS]`,
emitted after the system prompt. Calls are inline in the assistant turn:

```
[TOOL_CALLS]get_weather[ARGS]{"city": "Paris"}</s>
```

Two calls in one assistant message render as two `[TOOL_CALLS]` blocks, and
that works. Tool results are `[TOOL_RESULTS]{"temp_c": 18}[/TOOL_RESULTS]`,
text only. Arguments may be a mapping or a JSON string; both render (a string
is passed through, an object is serialised with `tojson`).

## Thinking and effort

This template validates the effort itself:

```
{%- if reasoning_effort not in ['none', 'high'] %}
    raise_exception('reasoning_effort must be either "none" or "high"')
```

and always writes `[MODEL_SETTINGS]{"reasoning_effort": "<value>"}[/MODEL_SETTINGS]`
into the prompt. There is no `enable_thinking` handling; thinking on and off
render identically.

| effort sent | result |
|---|---|
| `none` | renders, `{"reasoning_effort": "none"}` (also the default when absent) |
| `high` | renders, `{"reasoning_effort": "high"}` |
| `low`, `medium`, `xhigh`, `max`, `minimal` | raises `reasoning_effort must be either "none" or "high"` |

At b11960 the server drops a `reasoning_effort` of `"none"` from the template
kwargs and turns `enable_thinking` off (server-common.cpp:1497-1506), so `none`
reaches the template as the default and renders correctly.

The exception is a plain runtime error, not an invalid request, so
`ex_wrapper` maps it to HTTP 500 with the template's message
(tools/server/server.cpp:60-96). The prompt cache also sees a different prompt
for each effort, so switching between none and high invalidates it.

## Roles and structure

`system` works only as `messages[0]`; a system message later in the history
raises the alternating-roles error. `developer` is not a known role and raises
`Only user, assistant and tool roles are supported, got developer.` So an
OpenAI `developer` message must be rewritten to `system` (first message) before
this model, or the request 500s.

After the optional system message, non-tool assistant turns must alternate
with user turns, otherwise the template raises
`After the optional system message, conversation roles must alternate user and assistant roles except for tool calls and results.`

When no system message is passed, the template injects a ~3.4 kB default
"Le Chat" system prompt that names the model, the knowledge cutoff and the
current date, and describes tool use. That is a real token cost on every
request and it changes daily (the date comes from `strftime_now`).

## Recommendation

Never send `low`, `medium`, `xhigh` or `max` to this model: it is an HTTP 500,
not a downgrade. Clamp effort to `none` or `high`, and translate any
`developer` role to the first `system` message. Two effort levels only, so an
effort slider in the app should show two positions for this model.

## Template bug

The Unsloth date fix raises `Unsloth custom template does not support years >
2032` for any date after 2032, so the template stops rendering in 2033 unless
it is replaced with `--chat-template-file`.

## Server cross-check (llama.cpp b11960)

- `caps_apply_reasoning_effort` binds `reasoning_effort` and
  `reasoning_strength` (common/jinja/caps.cpp:31-36); this template uses only
  the first.
- `caps_get` probes `minimal, low, medium, high, xhigh, max`, drops levels
  whose render equals the unknown-level render, and keeps the highest of any
  aliases (caps.cpp:596-640). Here only `high` survives, so `/props` should
  report `reasoning_effort_levels: ["high"]`.
- There is no "Unexpected reasoning effort" string at b11960; the 500 body
  carries the template's own `raise_exception` text.
