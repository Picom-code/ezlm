## gemma-4-e4b

**Source:** `tokenizer.chat_template` from the GGUF header of
`unsloth/gemma-4-E4B-it-GGUF/gemma-4-E4B-it-UD-IQ2_M.gguf` (18808 bytes, one
template, no named variants). The E2B file ships the same template byte for
byte, so both rows describe one template.

**Tools** use the Gemma 4 DSL, not JSON. Declarations go inside the first
`<|turn>system` block:

```
<|tool>declaration:get_weather{description:<|"|>...<|"|>,parameters:{properties:{...},required:[<|"|>city<|"|>],type:<|"|>OBJECT<|"|>}}<tool|>
```

An assistant tool call is `<|tool_call>call:get_weather{city:<|"|>Paris<|"|>}<tool_call|>`,
a tool result is `<|tool_response>response:get_weather{value:<|"|>...<|"|>}<tool_response|>`.
Parallel calls are concatenated `<|tool_call>...<tool_call|><|tool_call>...<tool_call|>`
followed by the matching `<|tool_response>` blocks. After a tool result with
thinking off the prompt ends at `<tool_response|>` with no new `<|turn>model`,
which is the native Gemma 4 continuation the model expects. String arguments
that are pre-serialized JSON are spliced in without the outer braces.

**Thinking** is toggled by `enable_thinking` (the template also reads
`preserve_thinking`). `true` prepends `<|turn>system\n<|think|>\n<turn|>` and,
after a tool result, opens `<|channel>thought\n` for the next turn. `false` is
byte-identical to passing nothing, and the template defaults to off, but
llama.cpp passes `enable_thinking=true` by default (`common/chat.h:274`).

**Reasoning effort does nothing.** The template never references
`reasoning_effort` or `reasoning_strength`, so all six levels (`none`, `low`,
`medium`, `high`, `xhigh`, `max`) and the nested `reasoning.effort` /
`thinking.effort` shapes render byte-identically to the no-effort baseline
(sha256 prefix `ef01b2c2`). `reasoning_effort: "none"` only changes anything
because the server maps it to `enable_thinking=false` before rendering
(`tools/server/server-common.cpp:1497-1505` at b11960).

**Roles:** `system` and `developer` both work; a leading `developer` message
is rendered as `<|turn>system`, identical to `system`.

**Server cross-check (b11960).** `common/jinja/caps.cpp:31-34` binds both
`reasoning_effort` and `reasoning_strength`; `caps.cpp:567-643` only reports
`supports_reasoning_effort` when the template actually reads the value
(`stats.used`), so this model reports `supports_reasoning_effort=false` and an
empty `reasoning_effort_levels`. `common/chat.cpp:1081-1083` applies the
effort only through that binding. The string `Unexpected reasoning effort`
does not occur anywhere in the b11960 tree: on that build an unsupported
effort is silently ignored, not an HTTP 500. The 500 in EZLM's integration
contract must come from templates that raise on an unknown level, not from
Gemma 4.

**Recommendation:** advertise no reasoning-effort levels for this model. Do not
use effort to turn thinking on or off; send `enable_thinking` (or
`reasoning_effort: "none"`, which the server converts) instead. A client that
sends `high` will get a normal render and no reasoning, with no error to
explain the silence.
