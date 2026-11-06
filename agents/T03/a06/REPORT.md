# T03 a06 report: bonsai-27b-1bit and qwen3.5-9b

Agent a06 of T03. Branch `agent/T03-a06`. Models: `bonsai-27b-1bit`
(`prism-ml/Bonsai-27B-gguf`, `Bonsai-27B-Q1_0.gguf`) and `qwen3.5-9b`
(`unsloth/Qwen3.5-9B-GGUF`, `Qwen3.5-9B-UD-IQ2_XXS.gguf`).

## What I did

1. Read both `tokenizer.chat_template` values from the GGUF headers with HTTP
   range requests (the `Reader` class in `scripts/verify_catalog.py`), header only,
   no weights. Neither file has named templates (`tokenizer.chat_template.tool_use`
   etc.), so `template_source` is `tokenizer.chat_template` for both. The raw
   strings are stored byte-for-byte in `agents/T03/models/<id>.templates.json`.
2. Rendered 21 cases per model with `agents/T03/a06/render.py`, using the same
   environment as llama.cpp b11960's `scripts/jinja/jinja-tester.py`:
   `ImmutableSandboxedEnvironment(trim_blocks, lstrip_blocks, loopcontrols)`,
   `raise_exception` raising `RuntimeError`, `tojson` as `json.dumps(..., ensure_ascii=False)`.
   Exact outputs and exact exception text are in `<id>.renders.json`.
3. Wrote one matrix row (`<id>.json`) and a readable section (`<id>.md`) per model.

Results: bonsai 17/21 rendered, 4 raised; qwen3.5-9b 18/21 rendered, 3 raised.

## Findings

| # | Severity | Finding | Evidence |
|---|---|---|---|
| 1 | major | Both templates raise `Unexpected message role.` for a `developer` role message. An OpenAI-style `developer` message becomes an HTTP 500, so the engine must map it to `system`. | renders `developer_role` in both `<id>.renders.json`; template branch `{%- else %}{{- raise_exception('Unexpected message role.') }}` |
| 2 | major | `qwen3.5-9b` silently drops every tool parameter when `tool_call.arguments` is a JSON string rather than an object, producing `<function=get_weather>` with no `<parameter>` blocks and no error. | renders `tool_call_string_args`; template guard `{%- if tool_call.arguments is mapping %}` in `qwen3.5-9b.templates.json` |
| 3 | major | Neither template reads `reasoning_effort` (nor `reasoning_strength`). All six levels render byte-identical output, so effort is silently ignored. b11960's caps would report `supports_reasoning_effort=false`. | renders `effort_*` and `effort_high_*`; `common/jinja/caps.cpp` "reasoning effort" case only marks support when the value is used |
| 4 | minor | The default thinking state differs: `bonsai-27b-1bit` defaults thinking ON (`<|im_start|>assistant\n thinking\n`), `qwen3.5-9b` defaults OFF (`...\n thinking\n\n</think>\n\n`). Same client settings give different behaviour. | renders `plain`, `thinking_on`, `thinking_off` |
| 5 | minor | Both raise `System message must be at the beginning.` if a system message is not first, and `No messages provided.` on an empty list. | renders `system_not_first`, `empty_messages` |
| 6 | minor | `bonsai-27b-1bit` has no `is mapping` guard: a string `tool_call.arguments` raises `Can only get item pairs from a mapping.` in python jinja2. minja's `items` filter on a string was not verified. | render `tool_call_string_args` (bonsai) |

Things that do work in both: tools with object arguments, parallel tool calls
(consecutive `<tool_call>` blocks, results merged into one `<tool_response>` user
turn), system role as the first message, `enable_thinking` true/false.

## Cross-check against llama.cpp b11960

- `common/chat.cpp` `common_chat_templates_init` (L884-L976) uses
  `tokenizer.chat_template.tool_use` from the GGUF as the tool-use override and
  otherwise falls back to the default template. Neither GGUF has that named
  template, so the files rendered here are what the server uses, including for
  tool calls. No `--chat-template-file` override is needed for either model.
- `common/chat.cpp` L1081-L1083 forwards the top-level `reasoning_effort` string
  into the template as `reasoning_effort` and `reasoning_strength`
  (`common/jinja/caps.cpp` `caps_apply_reasoning_effort`). Since the templates
  never read either variable, nothing changes.
- `tools/server/server-common.cpp` L1497-L1505 special-cases the OAI value
  `"none"`: it sets `enable_thinking=false` and erases `reasoning_effort`. So for
  these two models `reasoning_effort: "none"` is the one level with an effect,
  and it works by disabling thinking, not through the template. `low` to `max`
  are inert.
- `common/jinja/caps.cpp` only reports `supports_reasoning_effort` when the
  rendered context uses the value, and only reports a level when it renders
  differently from an unknown level. Both models therefore advertise no effort
  levels.
- The b11960 tree bundles `models/templates/Qwen3.5-4B.jinja`, which matches the
  bonsai template (thinking on by default, `preserve_thinking`, `|items`). The
  qwen3.5-9b GGUF ships an older variant (thinking off by default, `is mapping`
  guard, no `preserve_thinking`). llama.cpp prefers the GGUF template, so the
  server follows the GGUF.
- The exact string "Unexpected reasoning effort" is not produced by either
  template: they have no effort check at all. For these models the failure mode is
  silence, not a 500. A 500 with that text can only come from a template that
  raises on an unknown level (not these two).

## What I changed

Only `agents/T03/` data and helper files: `models/<id>.templates.json`,
`.renders.json`, `.json`, `.md`, plus `a06/render.py` and this report. No Go
files, no catalog changes.

## What I could not finish

- minja is not python jinja2. The recorded outputs follow the official
  `jinja-tester.py` environment; minja's `tojson` (nlohmann dump) and `items`
  filter edge cases may differ in whitespace or on string arguments. The tool
  format, thinking toggle and effort behaviour do not depend on those differences.
- I did not run llama-server (forbidden) or capture a live HTTP 500 for a
  `developer` role; the claim is from rendering the template with the server's
  context.

## Next steps

- In the engine's OpenAI adapter, map `developer` to `system` for both models.
- Normalise tool arguments to a JSON object before rendering (especially for
  qwen3.5-9b, where a string silently empties the call).
- Do not advertise effort control for either model; `none` only means "no thinking".
- Have a16's checks confirm `supports_reasoning_effort=false` rows have no
  accepted effort levels.
