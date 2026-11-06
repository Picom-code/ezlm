# muse-glimmer-30b (meta-models/Muse-Glimmer-30B, GGUF from unsloth/Muse-Glimmer-30B-GGUF)

Source: `tokenizer.chat_template` in the GGUF header of
`Muse-Glimmer-30B-UD-IQ2_XXS.gguf`, read with range requests. One template, no
named variants. It is a GPT-OSS style "harmony" template (`<|start|>`,
`<|message|>`, `<|eot|>`, `<|eom|>`) that emits tool calls in an ATEM
(`<atem:function_calls>`) block and calls itself "Onyx" in its error text.

## Tools

Tool definitions go into the system turn: a prose preamble, one JSON line per
tool namespace, one JSON line per function schema, then a worked example. The
`# Valid recipients:` line grows to include `"<namespace>.*"`.

An assistant tool call renders as its own turn:

```
<|start|>assistant to=get_weather<|message|><atem:function_calls>
<atem:invoke name="get_weather">
<atem:parameter name="city">Paris</atem:parameter>
</atem:invoke>
</atem:function_calls><|eot|>
```

A tool result is `<|start|>tool get_weather<|message|><tool_output name="get_weather">\n{...}\n</tool_output><|eot|>`.
Two tool calls in one assistant message become two `to=` turns separated by
`<|eom|>`; that renders fine.

The one hard error: `tool_call.function.arguments` must be a mapping. A JSON
string (what many OpenAI clients send) raises
`Onyx ATEM chat template requires tool_call.function.arguments to be a dict (mapping); a JSON string cannot be parsed in the HF jinja sandbox.`
llama.cpp normalises to object arguments for this template
(`supports_object_arguments` is detected, common/jinja/caps.cpp:329), so the
server path is safe, but any client or proxy that re-serialises arguments as a
string gets an HTTP 500.

## Thinking and effort

There is no `enable_thinking` or `thinking` handling at all. `thinking_on` and
`thinking_off` render byte-identical prompts, so the toggle is a no-op.

The template reads `reasoning_strength` (default `high`) and prints one line in
the system turn: `Reasoning strength: <value>.` It never validates the value
and never branches on it, so the level is free text the model may or may not
honour.

`reasoning_effort` alone is ignored by the template, but b11960 aliases it:
`caps_apply_reasoning_effort` binds both `reasoning_effort` and
`reasoning_strength` to the same value (common/jinja/caps.cpp:31-36), so an
OpenAI `reasoning_effort` does reach the prompt.

| effort sent | rendered line | note |
|---|---|---|
| `none` | `Reasoning strength: high.` | server drops the kwarg (server-common.cpp:1497-1506), so the template default wins |
| `low` | `Reasoning strength: low.` | rendered verbatim |
| `medium` | `Reasoning strength: medium.` | rendered verbatim |
| `high` | `Reasoning strength: high.` | same as default |
| `xhigh` | `Reasoning strength: xhigh.` | rendered verbatim |
| `max` | `Reasoning strength: max.` | rendered verbatim |
| `minimal` | `Reasoning strength: minimal.` | server probes this level too (caps.cpp:596) |

`none` is the trap: it renders, but it means high, because the default is
`high`. If a caller wants less reasoning it must send a value the model
understands, and there is no way to disable reasoning through this template.

## Roles

`system` and `user` and `tool` and `assistant` render. A `developer` role
falls through every branch: the message is silently dropped and the prompt is
identical to a plain user turn, with no exception. Apps that rely on the
developer role for instructions lose them without any error.

## Recommendation

Never send `reasoning_effort: "none"` to this model expecting less reasoning;
it silently renders as high. If EZLM exposes an effort control, either hide it
for this model or map it onto `reasoning_strength` explicitly, and treat
`none` as "high". Also keep `developer` role messages out: they vanish.

## Server cross-check (llama.cpp b11960)

- `caps_get` probes effort levels `minimal, low, medium, high, xhigh, max`,
  discards any level whose render equals the `REASONING_EFFORT_UNKNOWN` render
  and collapses aliases (common/jinja/caps.cpp:596-640). Every level renders a
  distinct string here, so `/props` will report all six as supported, even
  though the template only pastes the text.
- `supports_reasoning_effort` is decided by whether the effort value is used
  at all (caps.cpp:589); the aliasing in `caps_apply_reasoning_effort` makes
  that true here.
- A template exception is not an `invalid_argument`, so `ex_wrapper` maps it
  to HTTP 500 with the template's own message (tools/server/server.cpp:60-96).
  There is no "Unexpected reasoning effort" string in b11960; that text is not
  in the tree at this tag.
