## gemma-4-e2b

**Source:** `tokenizer.chat_template` from the GGUF header of
`unsloth/gemma-4-E2B-it-GGUF/gemma-4-E2B-it-UD-IQ2_M.gguf` (18808 bytes, one
template, no named variants). The E4B file ships the same template byte for
byte, so the two models share every rendering result below.

**Tools** use the Gemma 4 DSL, not JSON. Declarations go inside the first
`<|turn>system` block:

```
<|tool>declaration:get_weather{description:<|"|>...<|"|>,parameters:{properties:{...},required:[<|"|>city<|"|>],type:<|"|>OBJECT<|"|>}}<tool|>
```

An assistant tool call is `<|tool_call>call:get_weather{city:<|"|>Paris<|"|>}<tool_call|>`,
a tool result is `<|tool_response>response:get_weather{value:<|"|>...<|"|>}<tool_response|>`.
Parallel calls are concatenated and followed by the matching tool responses.
After a tool result with thinking off the prompt ends at `<tool_response|>`
with no new `<|turn>model`, which is the native Gemma 4 continuation.

**Thinking** is toggled by `enable_thinking` (also reads `preserve_thinking`).
`true` prepends `<|turn>system\n<|think|>\n<turn|>` and, after a tool result,
opens `<|channel>thought\n`. `false` renders the same as passing nothing.

**Reasoning effort does nothing.** No `reasoning_effort` or
`reasoning_strength` branch exists in the template. All six levels and the
nested `reasoning.effort` / `thinking.effort` shapes render byte-identically
to the no-effort baseline (sha256 prefix `ef01b2c2`). `reasoning_effort:
"none"` works only because `tools/server/server-common.cpp:1497-1505`
(b11960) maps it to `enable_thinking=false`.

**Roles:** `system` and `developer` both work; a leading `developer` message
becomes `<|turn>system`, identical to `system`.

**Server cross-check (b11960).** `common/jinja/caps.cpp:31-34` binds
`reasoning_effort`/`reasoning_strength`; `caps.cpp:567-643` sets
`supports_reasoning_effort` only if the template reads the value, so this
model reports false and `reasoning_effort_levels=[]`. `common/chat.cpp:1081-1083`
is the only consumer. `Unexpected reasoning effort` is absent from the whole
b11960 tree, so that build silently ignores an effort this template does not
use.

**Recommendation:** advertise no effort levels for this model and drive
thinking with `enable_thinking` only. Sending `max` produces a normal prompt
with no reasoning and no error, so a client cannot tell it was ignored.
