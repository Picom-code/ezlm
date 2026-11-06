# T03 / a11: muse-glimmer-30b and mistral-small-4-119b

Agent a11, branch `agent/T03-a11`. Slice: two models.

- `muse-glimmer-30b` (repo `meta-models/Muse-Glimmer-30B`, GGUF from
  `unsloth/Muse-Glimmer-30B-GGUF`, file `Muse-Glimmer-30B-UD-IQ2_XXS.gguf`)
- `mistral-small-4-119b` (repo `mistralai/Mistral-Small-4-119B-2603`, GGUF
  from `unsloth/Mistral-Small-4-119B-2603-GGUF`, file
  `Mistral-Small-4-119B-2603-UD-IQ1_M.gguf`)

## What I did

1. Read `tokenizer.chat_template` from each GGUF header with HTTP range
   requests, using the `Reader` class in `scripts/verify_catalog.py`
   (`agents/T03/a11/fetch_templates.py`). Neither model has named
   `tokenizer.chat_template.*` variants. Raw text is in
   `agents/T03/models/<id>.templates.json`.
2. Rendered every case with the venv's jinja2 3.1.6
   (`agents/T03/a11/render_templates.py`), using an
   `ImmutableSandboxedEnvironment(trim_blocks=True, lstrip_blocks=True)`, a
   `raise_exception` that raises, `strftime_now`, a `tojson` with sorted keys
   and no ASCII escapes, and the model's real `bos_token`/`eos_token` from the
   GGUF tokenizer metadata. Exact outputs and exception text are in
   `agents/T03/models/<id>.renders.json`.
3. Emulated b11960's request context, including the two server behaviours that
   change the result: `reasoning_effort` is aliased onto `reasoning_strength`
   (common/jinja/caps.cpp:31-36), and `reasoning_effort: "none"` is dropped
   before the template runs (tools/server/server-common.cpp:1497-1506).
4. Cross-checked the b11960 sources: `common/jinja/caps.cpp`,
   `common/chat.cpp`, `tools/server/server.cpp`, `tools/server/server-common.cpp`,
   `tools/server/server-context.cpp` (raw.githubusercontent.com, GET only).

## Results

| model | cases rendered | cases raised |
|---|---|---|
| muse-glimmer-30b | 22 / 24 | 2 (both: tool arguments as a JSON string) |
| mistral-small-4-119b | 13 / 20 | 7 (effort low/medium/xhigh/max/minimal, developer role, system not first) |

Rows: `agents/T03/models/muse-glimmer-30b.json`,
`agents/T03/models/mistral-small-4-119b.json`. Readable sections:
`agents/T03/models/<id>.md`.

## Findings

1. **blocker (mistral-small-4-119b): only `none` and `high` are valid
   efforts.** The template raises
   `reasoning_effort must be either "none" or "high"` for low, medium, xhigh,
   max and minimal. A template exception is not an `invalid_argument`, so
   `ex_wrapper` turns it into HTTP 500 with that text
   (tools/server/server.cpp:60-96). Evidence: `renders.json` cases
   `effort_low`, `effort_medium`, `effort_xhigh`, `effort_max`,
   `effort_minimal`.
2. **major (muse-glimmer-30b): `reasoning_effort: "none"` silently means
   high.** The server strips the `none` kwarg and the template's default is
   `high`, so `none` renders `Reasoning strength: high.` (renders.json
   `effort_none`, byte-identical to `plain`). There is no way to turn
   reasoning off for this model.
3. **major (muse-glimmer-30b): a `developer` role is silently dropped.** The
   template has no branch for it and no `else`, so the message disappears and
   the prompt equals a plain user turn (renders.json `developer_role`,
   `same_as_plain: true`). No error, so an app cannot tell its instructions
   were lost.
4. **major (mistral-small-4-119b): a `developer` role raises.** It is an
   unknown role, so the request 500s with `Only user, assistant and tool roles
   are supported, got developer.` An app that sends OpenAI `developer` messages
   must rewrite them to the first `system` message.
5. **minor (muse-glimmer-30b): tool arguments must be a mapping.** A JSON
   string raises
   `Onyx ATEM chat template requires tool_call.function.arguments to be a dict (mapping); ...`.
   llama.cpp normalises object arguments for this template
   (common/jinja/caps.cpp:329), so the server path is fine, but a proxy that
   re-serialises arguments as strings breaks.
6. **minor (mistral-small-4-119b): a `system` message is only valid as
   `messages[0]`.** Anywhere else it trips the alternating-roles check and
   raises. Non-tool assistant turns must also alternate with user turns.
7. **minor (mistral-small-4-119b): ~3.4 kB default system prompt** is injected
   when the request has no system message, and it changes daily
   (`strftime_now`), so it perturbs prompt caching.
8. **minor (mistral-small-4-119b): the template stops rendering in 2033.** The
   Unsloth date fix raises `Unsloth custom template does not support years >
   2032`.
9. **minor (muse-glimmer-30b): no thinking toggle at all.** `enable_thinking`
   and `thinking` are ignored; thinking on/off render identical prompts. Effort
   is a free-text line with no validation, so every level including garbage
   renders, and b11960's `caps_get` will report all six levels as supported
   even though the template only pastes the string (caps.cpp:596-640).

## Note on the brief's error text

There is no "Unexpected reasoning effort" string in llama.cpp at b11960
(`grep` over `common/chat.cpp`, `common/jinja/caps.cpp`,
`tools/server/server*.cpp` finds none). The 500 body carries the template's own
`raise_exception` message instead. If a caller is matching on that string, it
will never match.

## Sources

- `agents/T03/models/<id>.templates.json` (verbatim GGUF header templates)
- `agents/T03/models/<id>.renders.json` (verbatim renders and errors)
- llama.cpp b11960: `common/jinja/caps.cpp` (31-36, 329, 583-640),
  `common/chat.cpp` (1081-1085, 1536-1539),
  `tools/server/server-common.cpp` (1497-1506),
  `tools/server/server.cpp` (60-96)

## Open questions

- Do the models actually honour the free-text `Reasoning strength:` line, or is
  it inert? Only a live run can tell, and we cannot load models here.
- Is `none` for muse intended to mean "no reasoning" upstream? The template
  default of `high` suggests the model was trained to always reason.
- Should EZLM clamp effort per model (matrix-driven) or let the 500 happen?

## Not done

- No live server run (forbidden here), so the HTTP status/body mapping is from
  source reading, not from a request.
- Did not probe `chat_template_kwargs` shapes beyond `reasoning_effort`,
  `reasoning_strength`, `thinking`, `reasoning.effort`.
