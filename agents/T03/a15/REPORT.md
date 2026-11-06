# T03 a15 report

Agent a15. Models: `qwen3.8-flash-next`, `qwen3.6-27b`. Cross-cutting
deliverables: `agents/T03/assemble.py`, `agents/T03/matrix.schema.md`.

## What I did

- Read `tokenizer.chat_template` from the GGUF headers with range requests,
  using `scripts/verify_catalog.py`'s `Reader` / `header()`. Neither model ships
  a named template: only `tokenizer.chat_template` exists. Saved verbatim to
  `models/<id>.templates.json`.
- Rendered both templates with jinja2 3.1.6 (`run/venv`), with
  `raise_exception` and `strftime_now` stubs, and recorded every case in
  `models/<id>.renders.json`: 35 cases per model, including the 10 the brief
  lists plus the dotted effort shapes, late roles, string tool arguments and a
  missing function name.
- Wrote the row (`models/<id>.json`) and the readable section
  (`models/<id>.md`) for each model.
- Wrote `assemble.py` and `matrix.schema.md`, and ran `assemble.py` to produce
  `matrix.json` (2 of 32 catalog models so far; 30 reported missing).

Render totals: qwen3.8-flash-next 28/35 rendered, 7 raised;
qwen3.6-27b 33/35 rendered, 2 raised.

## Findings

**1. `qwen3.8-flash-next` returns HTTP 500 for `reasoning_effort` `none` or
`max`. Severity: major.**
The template reads `reasoning_effort` (default `xhigh`) and raises for anything
not in `xhigh, medium, low`:
`Unexpected reasoning effort none. Supported types are xhigh (default), medium, and low.`
Evidence: `models/qwen3.8-flash-next.renders.json` cases `effort_none` and
`effort_max`; b11960 `common/chat.cpp:1081-1083` passes the value straight
through and `tools/server/server.cpp:75-88` turns a template exception into a
500 with that message. `none` is the value agent apps send to mean "no
reasoning", so this is the exact failure T03 is about.

**2. `qwen3.8-flash-next` remaps `high` to `xhigh`. Severity: minor.**
The template sets `resolved_reasoning_effort = 'xhigh'` when the input is
`high`, so `high` renders the xhigh instruction and never raises. A caller
asking for `high` silently gets the strongest tier.

**3. `qwen3.8-flash-next` `medium` renders no reasoning instruction. Severity:
minor.**
`medium` passes the allow-list but no `elif` branch matches it, so the system
turn is identical to a default render. It is accepted and a no-op.

**4. `qwen3.8-flash-next` ignores `reasoning.effort` and `thinking.effort`.
Severity: minor.**
Both dotted shapes render byte-identical to the default (12 cases, one distinct
output). A client that sends the nested shape gets `xhigh` silently.

**5. `qwen3.6-27b` ignores `reasoning_effort` entirely. Severity: minor.**
All six values render byte-identical output; the template never reads the
variable, `reasoning_strength` or either dotted shape. This is the safe failure
mode: no effort value can 500. But no effort control works either, so the matrix
row lists them under `effort_levels_ignored`, not `accepted`.

**6. `qwen3.6-27b` silently drops a `system` or `developer` message that is not
at the start. Severity: major.**
The message loop is guarded by `message.role != "system" and message.role !=
"developer"` and only the first one or two leading messages are merged, so a
mid-conversation developer message disappears with no error. qwen3.8 raises
`System message must be at the beginning.` for the same input
(`developer_role_late`, `system_role_late` in both renders files).

**7. `qwen3.6-27b` silently drops tool arguments passed as a JSON string.
Severity: major.**
The template only prints parameters when `tool_call.arguments is mapping`; a
string argument produces `<function=get_weather></function>` with no parameters.
qwen3.8 raises a clear error instead. Evidence: `tool_call_string_args` in both
renders files.

**8. `qwen3.6-27b` has no function-name check. Severity: minor.**
A call without `name` dies on `UndefinedError: 'dict object' has no attribute
'name'` from jinja2. That is the Python engine's message, not the template's;
llama.cpp's own template engine may word it differently, so treat the text as
approximate.

**9. b11960 advertises fewer effort levels than it accepts. Severity: minor.**
`common/jinja/caps.cpp` probes `minimal, low, medium, high, xhigh, max`, drops
any render equal to the unknown-effort render, and keeps the highest alias. For
qwen3.8 that yields `reasoning_effort_levels: ["low", "medium", "xhigh"]`:
`high` is missing because it aliases `xhigh`, and the raising levels are absent
by construction. So `chat_template_caps` is not a rejection list; a client that
uses it to avoid 500s will miss that `none` and `max` fail.

## Cross-cutting: assemble.py

`~/ezlm-agents/run/venv/bin/python agents/T03/assemble.py` merges
`models/*.json` into `matrix.json` (`{"version": 1, "models": [...]}`, sorted
by id), prints the table, and reports missing, duplicate and non-catalog ids.
Skips `*.renders.json`, `*.templates.json` and `_`-prefixed files. Duplicate
ids keep the first file in name order and warn. Missing catalog file is a
warning, not a crash. `--check` writes nothing and exits 1 when the matrix is
incomplete. Tested with 0 rows, 1 row, 2 rows, a duplicate, a malformed JSON
file, a file with no id, and no catalog. Schema and key semantics are in
`matrix.schema.md`; the one convention to keep is that a level that renders
without changing the output is *ignored*, not accepted.

## Sources

- GGUF headers, range requests only:
  `https://huggingface.co/ISTA-DASLab/Qwen3.8-Flash-Next-GSQ-RCO-GGUF/resolve/main/Q2_0/Qwen3.8-Flash-Next-GSQ-RCO-Q2_0-00001-of-00002.gguf`
  and
  `https://huggingface.co/unsloth/Qwen3.6-27B-MTP-GGUF/resolve/main/Qwen3.6-27B-UD-IQ2_XXS.gguf`.
- llama.cpp tag b11960: `common/chat.cpp` (reasoning_effort, line 1081),
  `common/jinja/caps.cpp` (effort probing, lines 576-640),
  `tools/server/server.cpp` (exception to 500, lines 60-95).

## Open questions

- llama.cpp renders with its own jinja engine, not Python jinja2. The cases that
  raise from an explicit `raise_exception` are engine-independent; the
  `UndefinedError` for a missing function name is not, so the engine may print
  something else or render empty.
- Whether the server normalises tool-call `arguments` from a string into an
  object before the template sees it. If it does, finding 7 is only a problem
  for callers that bypass the parser.
- `enable_thinking` is always set by b11960 (`common/chat.cpp:1053`), so the
  "undefined means on" branch in both templates is unreachable through the
  server. I did not test a client that omits the field.

## What I could not finish

- Only two of 32 catalog models: the other agents own the rest.
- No live server was run (README forbids loading a model), so all server claims
  come from reading b11960 source.
