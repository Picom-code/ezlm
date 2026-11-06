# T03 a07 report: gemma-4-e4b and gemma-4-e2b

Agent a07, branch `agent/T03-a07`. Slice: `gemma-4-e4b`, `gemma-4-e2b`.

## What I did

- Read `tokenizer.chat_template` from the GGUF header with HTTP range requests
  (`scripts/verify_catalog.py: Reader`), first quant of each model:
  `unsloth/gemma-4-E4B-it-GGUF/gemma-4-E4B-it-UD-IQ2_M.gguf` and
  `unsloth/gemma-4-E2B-it-GGUF/gemma-4-E2B-it-UD-IQ2_M.gguf`. No weights
  downloaded.
- Saved the raw templates in `models/<id>.templates.json`. Both files ship the
  same 18808-byte template; there are no `tokenizer.chat_template.*` variants.
- Rendered the template with Python jinja2 3.1.6 in
  `~/ezlm-agents/run/venv`, llama.cpp context (`bos_token`, `eos_token`,
  `messages`, `tools`, `add_generation_prompt=true`, `raise_exception` stub),
  20 cases per model: the 13 required cases plus probes for nested effort
  shapes, thinking with tools, reasoning_content, and no generation prompt.
  All 20 rendered OK for both models, and the two models render identically.
  Outputs and digests are in `models/<id>.renders.json`.
- One harness note: with jinja2 `StrictUndefined` the template raises
  `UndefinedError: 'dict object' has no attribute 'enum'` for a string tool
  parameter that has no `enum` (template line 15). That is not server
  behaviour: minja treats a missing object key as undefined and undefined is
  falsy (`common/jinja/runtime.cpp` object access, `common/jinja/value.cpp`
  "undefined == false"), so I rendered with plain `Undefined` to match
  llama.cpp. Reported so nobody mistakes it for a real failure.
- Cross-checked llama.cpp b11960: `common/chat.cpp`, `common/jinja/caps.cpp`,
  `common/chat.h`, `tools/server/server-common.cpp`, plus a full-tree grep of
  the b11960 tarball for the error text.

## Findings

1. **major: reasoning effort is silently ignored by both models.**
   `gemma-4-e4b`/`gemma-4-e2b` templates contain no `reasoning_effort` or
   `reasoning_strength` branch. All six levels and the nested
   `reasoning.effort` / `thinking.effort` shapes render byte-identically to
   the no-effort baseline (sha256 prefix `ef01b2c2`; see
   `models/<id>.renders.json`). A client that sends `high` or `max` gets a
   normal prompt, no reasoning and no error. Evidence: `renders.json`,
   template lines 177-385.

2. **major: b11960 reports no effort levels for these models, so the 500
   "Unexpected reasoning effort" cannot come from them at this build.**
   `common/jinja/caps.cpp:567-643` sets `supports_reasoning_effort` only when
   the template actually reads `reasoning_effort` (`stats.used`), and
   `caps.cpp:31-34` is the only place the value is bound. `common/chat.cpp:1081-1083`
   applies it through that binding. The string `Unexpected reasoning effort`
   does not occur anywhere in the b11960 tree (grepped the full tag tarball),
   and `server-common.cpp:1497-1505` only special-cases `none` and otherwise
   forwards the value as a template kwarg. So at b11960 the failure mode for
   these two models is silence, not HTTP 500. EZLM's `internal/integrate/*`
   `ErrorText` match on that 500 must be fed by other models' templates that
   call `raise_exception`.

3. **minor: `reasoning_effort: "none"` works only by server mapping.**
   `tools/server/server-common.cpp:1497-1505` turns it into
   `enable_thinking=false` and drops the kwarg; the template itself has no
   `none` branch. If a future server stops doing that, `none` would be a no-op
   too.

4. **minor: developer role is accepted but flattened to system.**
   A leading `developer` message renders as `<|turn>system` and is
   byte-identical to a `system` message (digest `3f0e8a365d11`), so the model
   cannot tell them apart. `system_role=true` and `developer_role=true` in the
   row mean "accepted", not "distinct".

5. **minor: template defaults thinking off, server defaults it on.**
   Template line 180 `enable_thinking | default(false)`, but
   `common/chat.h:274` defaults `enable_thinking=true`, so the server's
   default is thinking on. Direct jinja2 users who omit the variable get the
   opposite of the server.

6. **minor: tool-call DSL is not JSON.** Calls are
   `<|tool_call>call:name{key:value}<tool_call|>` and results are
   `<|tool_response>response:name{...}<tool_response|>`; string arguments are
   spliced in without the outer JSON braces (template lines 250-261). Any
   integration that expects JSON tool syntax for this model is wrong.

## Recommendations

- Matrix: `tools_supported=true`, `tool_call_format=gemma4-dsl`,
  `thinking_toggle=enable_thinking`, `effort_levels_accepted=[]`,
  `effort_levels_rejected=[]`, `system_role=true`, `developer_role=true`.
- Agent apps should offer no reasoning-effort selector for these models and
  drive thinking with `enable_thinking` (or `reasoning_effort: "none"` at the
  OAI layer, which the server converts).
- Do not treat an empty accepted-effort list as a broken model: these
  templates render every value, they just do not use it.

## Sources

- GGUF headers: HF range requests to the two `unsloth/...-it-GGUF` repos.
- llama.cpp b11960 (`https://github.com/ggml-org/llama.cpp/tree/b11960`):
  `common/chat.cpp:1081-1083`, `common/chat.h:274`,
  `common/jinja/caps.cpp:31-34,567-643`, `tools/server/server-common.cpp:1497-1505`.
- Harness: `agents/T03/a07/fetch.py` (header fetch), `agents/T03/a07/render.py`
  (renders, deterministic).

## Open questions

- Which llama.cpp build emits `Jinja Exception: Unexpected reasoning effort`?
  It is not b11960, so the app's error mapping may be tested against a build
  EZLM no longer ships. a16 owns the authoritative answer.
- Should the catalog carry an explicit `reasoning_effort_levels` field per
  model so clients stop guessing? This slice shows the template alone is not
  enough to know whether a level is honoured.
