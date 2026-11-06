# T03 / a05 REPORT: bonsai-8b-1bit and gemma-4-26b-a4b

Slice: render the chat templates of `bonsai-8b-1bit` and `gemma-4-26b-a4b`
with jinja2 under llama.cpp b11960 conventions and write one matrix row each.

## What I did

- Read `tokenizer.chat_template` from each GGUF header with HTTP range requests
  (`scripts/verify_catalog.py` `header()`), never the weights. Both models have
  exactly one template and no `tokenizer.chat_template.*` named templates.
  Files: `agents/T03/models/<id>.templates.json`.
- Rendered 21 cases per model with jinja2 3.1.6
  (`~/ezlm-agents/run/venv/bin/python`, `agents/T03/a05/render.py`): plain,
  system+user, tool call, parallel tools, `enable_thinking` on/off,
  `reasoning_effort` none/minimal/low/medium/high/xhigh/max, developer role,
  plus `add_generation_prompt=false`, an assistant reasoning-history turn,
  nested `reasoning.effort` / `thinking.effort` shapes, `thinking` true/false,
  and object tool arguments. All 21/21 rendered on both models; no case raised.
  Files: `agents/T03/models/<id>.renders.json`.
- Wrote `agents/T03/models/<id>.json` (matrix row) and `<id>.md` (readable
  section).
- Cross-checked llama.cpp b11960 sources (tag `b11960`, raw.githubusercontent
  GETs and the source tarball, not committed).

## Findings

1. **bonsai-8b-1bit ignores `reasoning_effort` completely** (severity: major for
   effort-based routing). `reasoning_effort`, `reasoning_strength` and the
   nested shapes are absent from the template; all renders are byte-identical to
   `plain` (evidence: `bonsai-8b-1bit.renders.json`, `effort_*` entries equal
   `plain`). b11960 detects this with the same rule: it marks
   `supports_reasoning_effort` only when the `reasoning_effort` value is read
   (`common/jinja/caps.cpp:589`), and the template bindings are
   `reasoning_effort`/`reasoning_strength` (`common/jinja/caps.cpp:31`).
2. **bonsai-8b-1bit drops `developer` role messages silently** (severity: major
   if agents send developer instructions). The template branches on `user`,
   `assistant`, `tool` and a non-first `system` only; a `developer` message
   matches nothing. Render `developer_role` shows the user turn but no
   developer text (evidence: `bonsai-8b-1bit.renders.json`).
3. **bonsai-8b-1bit has no thinking toggle** (severity: major). No
   `enable_thinking`/`thinking` variable; `thinking_on` and `thinking_off`
   render byte-identical, and the generation prompt always ends with an empty
   ` thinking\n\n</think>` block.
4. **gemma-4-26b-a4b ignores `reasoning_effort` too** (severity: major for
   effort-based routing). It reads `enable_thinking`, `preserve_thinking` and
   `bos_token` only; all effort renders equal `plain`. So b11960 reports
   `supports_reasoning_effort = false` and `reasoning_effort_levels = []` for
   this model.
5. **gemma-4-26b-a4b renders `developer` as a system turn** (severity: minor,
   informational). It is accepted and merged into `<|turn>system`, so it works
   but is indistinguishable from a system message to the model.
6. **No template-raised errors on either model** (severity: informational).
   Neither template contains a `raise_exception` path, so neither can produce
   the "Unexpected reasoning effort" HTTP 500 from its own template.
7. **b11960 server never rejects an effort string itself** (severity: minor,
   corrects the task premise). `tools/server/server-common.cpp:1497-1505` maps
   `reasoning_effort: "none"` to `enable_thinking = false` and otherwise passes
   the string through as a template kwarg; there is no "Unexpected reasoning
   effort" text anywhere in the b11960 source tree. That error must come from a
   template's own `raise_exception` (for example `models/templates/IFM-K2-Horizon.jinja:992`
   raises "Unsupported reasoning_effort" and `tencent-Hy3.jinja:42-46` raises a
   similar message) or from the client. Neither of my two models does this.

## Server cross-check details (b11960)

- Capability probe: `common/jinja/caps.h:21-24` defines
  `supports_reasoning_effort` and `reasoning_effort_levels`;
  `common/jinja/caps.cpp:567-641` probes by rendering with
  `reasoning_effort`/`reasoning_strength` bound and keeping only levels whose
  render differs from an unknown-level render. Candidate levels are
  `minimal, low, medium, high, xhigh, max` (`caps.cpp:596`), so a model that
  reads the variable cannot report `none` (the server handles `none` itself).
- Apply path: `common/chat.cpp:1081-1083` binds the effort into the template
  context; `tools/server/server-common.cpp:1497` is where the OAI field is
  parsed.
- `enable_thinking` defaults to true on the server
  (`tools/server/server-common.h:357`) and is always passed
  (`common/chat.cpp:1057`); the template may ignore it.
- Tool arguments: b11960 renders message tool-call arguments as a JSON string
  (`common/chat.cpp`, `to_json_oaicompat`) and parses them back to an object
  only when the template's caps say `supports_object_arguments`
  (`common/chat.cpp:1532-1533`, `workaround::func_args_not_string` at
  `common/chat.cpp:1191`). gemma-4-26b-a4b reads arguments as a mapping, so it
  gets objects; bonsai-8b-1bit reads them as a string, so it gets strings. Both
  shapes render for both models (see `tool_call_object_args` in the renders
  files), but only the shape above is what the server sends.

## What I changed

Only `agents/T03/` files (this report, `render.py`, and the four files per
model). No Go code, no catalog edits.

## Not finished / open questions

- I did not run a live b11960 server (`/props` `chat_template_caps`) for these
  models: that would load a model, which the rules forbid. The caps values
  above are derived from the source and from the render results.
- `bos_token` for gemma is inferred as `<bos>` from the Gemma tokenizer
  (bos token id 2 in the header); only gemma's template uses it, and only at
  the very start of the prompt.
- Both models' templates are chat-only; whether the model actually emits the
  DSL/JSON tool-call format well is a model-quality question outside this task.

## Next steps

- a15: include both rows; treat `effort_levels_accepted: []` with a non-empty
  `effort_levels_ignored` as "template ignores effort", not "no effort
  support".
- a16: the "Unexpected reasoning effort" error is template-side, not
  server-side, in b11960; `checks.py` should not expect the server to reject an
  effort value.
