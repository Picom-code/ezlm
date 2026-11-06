# gemma-4-31b

Source: `tokenizer.chat_template` in `unsloth/gemma-4-31B-it-GGUF`, file
`gemma-4-31B-it-UD-IQ2_XXS.gguf`, read from the GGUF header with range requests
(18,922 bytes, one unnamed template). The catalog's own `repo`
(`google/gemma-4-31B-it`) has no GGUF file, so the first quant's repo override is
the one that exists.

## Tools

Tool declarations go into the first system turn as one `<|tool>` block each:

```
<|tool>declaration:get_weather{description:<|"|>...<|"|>,parameters:{properties:{city:{description:<|"|>City name.<|"|>,type:<|"|>STRING<|"|>}},required:[<|"|>city<|"|>],type:<|"|>OBJECT<|"|>}}<tool|>
```

The model is expected to answer with the Gemma DSL:

```
<|tool_call>call:get_weather{"city": "Paris", "units": "celsius"}<tool_call|>
```

That is the string-arguments branch, and it is the branch the server uses: the
template never reads `function.arguments.arg`, so `chat_template_caps.
supports_object_arguments` is false and `to_json_oaicompat` keeps arguments as a
JSON string. The template strips the outer braces and re-emits the rest, so the
prompt contains quoted keys and quoted values. The object-arguments branch would
render the canonical unquoted form (`city:<|"|>Paris<|"|>`) but the server never
sends that shape. Two calls in one message become two consecutive
`<|tool_call>` blocks, so parallel calls work. Tool results are
`<|tool_response>response:NAME{value:<|"|>...<|"|>}<tool_response|>` and, in the
OpenAI message shape, are emitted inside the same model turn.

## Thinking

Only `enable_thinking` is read (default false). True adds `<|think|>` at the top
of the first system turn and the generation prompt is a bare `<|turn>model`.
False writes an empty reasoning channel at the generation prompt:
`<|channel>thought\n<channel|>`. The difference is real: `thinking_on` and
`thinking_off` render differently.

## Effort

None. `reasoning_effort` is never referenced: `none`, `low`, `medium`, `high`,
`xhigh`, `max` and `REASONING_EFFORT_UNKNOWN` all render byte-identically to
`thinking_on`. The server reports `supports_reasoning_effort=false` and passes
the variable through harmlessly, so effort is silently ignored, not accepted and
not rejected.

## Roles

System is supported as the first message. `developer` is accepted: as the first
message it is rendered exactly like a system turn. The b11960 server also maps
developer to system for this template (`common/chat.cpp:1518`; the check looks
for `<|channel|>`, and this template only has `<|channel>`).

## Recommendation

Never rely on reasoning effort for this model: it has no effect on the prompt.
Send `enable_thinking` explicitly, since the template defaults it to false. Tool
arguments are rendered from a JSON string with quoted keys inside the call DSL,
which is off the canonical Gemma form; if a client can send a tool call with an
object `arguments` value and render the template itself, that path is cleaner.

## Server cross-check (llama.cpp b11960)

- `common/jinja/caps.cpp:567-641` probes effort with `enable_thinking=true` and
  candidates `minimal, low, medium, high, xhigh, max`; a level counts only if it
  changes the render. For this template nothing changes, so
  `supports_reasoning_effort=false`.
- `common/chat.cpp:1081-1083` sets `reasoning_effort` and `reasoning_strength`
  as template variables whenever the client sends the field, even when the
  template ignores it.
- `tools/server/server-common.cpp:1497-1505` treats `reasoning_effort: "none"`
  as `enable_thinking=false` and removes the kwarg; other values are forwarded
  as `chat_template_kwargs`.
- No "Unexpected reasoning effort" string exists anywhere in the b11960 tree
  (verified with a full-source grep). That HTTP 500 text comes from a template
  that raises it, not from the server, so a model whose template ignores effort
  can never produce it.
