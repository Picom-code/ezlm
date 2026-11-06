# gemma-4-12b chat template

Source: `tokenizer.chat_template` in `unsloth/gemma-4-12b-it-GGUF/gemma-4-12b-it-UD-IQ2_M.gguf` (GGUF v3 header read with range requests). One template only, no named variants. Rendered with jinja2 3.1.6, 20/20 cases rendered.

## Tools

Tools go in a `<|tool>...<tool|>` block inside a synthetic first system turn, even when the user sent no system message. Declarations use Gemma's own DSL, not JSON: `declaration:get_weather{description:<|"|>...<|"|>,parameters:{properties:{...}}}`.

Calls are emitted as one `<|tool_call>call:NAME{arg:value}<tool_call|>` block per call, so parallel calls are two consecutive blocks with no separator. Results become `<|tool_response>response:NAME{value:<|"|>...<|"|>}<tool_response|>`. A string result is wrapped under a `value:` key; a mapping result is rendered as keys. Parallel calls and their results all rendered correctly.

One soft spot: if a client sends `arguments` as an OpenAI JSON string instead of an object, the template strips the outer braces and pastes the raw JSON, so the call renders as `call:get_weather{"city": "Paris"}` with plain quotes instead of the `<|"|>` markers. It renders (no error) but the quoting differs from what the DSL uses everywhere else, so a strict parser may not match the argument names. Send arguments as an object.

## Thinking

Toggled only by `enable_thinking`, which defaults to **false** (unlike most reasoning models). Absent or false, the generation prompt closes an empty channel: `<|turn>model\n<|channel>thought\n<channel|>`. True prepends `<|think|>` to the first system turn and leaves the model turn open after `<|turn>model\n`. The `thinking` variable is not read, so `thinking: true` does nothing.

## Reasoning effort

The template never mentions `reasoning_effort` or `reasoning`. All six values (none, low, medium, high, xhigh, max) render byte-identically to the plain prompt, and so do the nested `reasoning.effort` and `thinking.effort` shapes. This is silence, not support: at b11960 the server's caps check marks a template as effort-capable only if the template actually reads the variable (`common/jinja/caps.cpp:589`, `effort->stats.used`), so this model reports `supports_reasoning_effort=false` and `reasoning_effort_levels=[]`. Sending an effort is harmless but has no effect.

## Roles

`system` works. A `developer` message is accepted: in position 0 the template treats it exactly like a system message (byte-identical output). A later `developer` message renders as `<|turn>developer`, a role the model was probably not trained on. In practice the server rewrites developer to system before rendering anyway (`common/chat.cpp:1517-1518` calls `workaround::map_developer_role_to_system`, all models except GPT-OSS).

## Recommendation

Do not send `reasoning_effort` to this model; it changes nothing. If you want thinking, send `enable_thinking: true` as a chat-template kwarg, and remember it is off by default here.

## Server cross-check (b11960)

- Missing object keys are falsy, not fatal: `common/jinja/runtime.cpp:903` looks up with the default-value `at()`, so the template's `value['enum']` / `value['nullable']` on properties that omit them is safe.
- `{% generation %}` blocks are ignored by the parser (`common/jinja/parser.cpp:219`), which is what this template would need if it had them. It does not.
- No mismatch found between the template and the server. The only server behaviour worth knowing is the developer-to-system rewrite above.
