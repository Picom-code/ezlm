# T03 / a14 report: granite-4.2-30b and granite-4.2-8b

Agent a14. Models: `granite-4.2-30b` (Q2_K header) and `granite-4.2-8b`
(Q3_K_M header). Both branches commit on `agent/T03-a14`.

## What I did

- Read `tokenizer.chat_template` from the GGUF headers with HTTP range requests
  through `scripts/verify_catalog.py` (`Reader`), never downloading the files.
  Both models have exactly one template (`tokenizer.chat_template`, 9013
  bytes) and the two are byte-identical. Raw copies:
  `agents/T03/models/<id>.templates.json`.
- Rendered every template with `agents/T03/a14/render.py` (Python jinja2),
  mirroring what `common/chat.cpp` passes at b11960: `messages`, `tools`,
  `add_generation_prompt`, `enable_thinking`, `bos_token`, `eos_token`,
  `datetime`, `date_string`, plus `reasoning_effort` for the effort cases. The
  harness provides `raise_exception` and a `tojson` filter with llama.cpp's
  signature (`ensure_ascii` keyword, `", "` / `": "` separators); plain
  jinja2's `tojson` rejects the template's `tojson(ensure_ascii=False)` call.
  This is a harness detail, not a model bug.
- 19 cases per model: plain, system+user, tool call + result, parallel calls,
  thinking on/off, `thinking` variable variants, effort none/minimal/low/
  medium/high/xhigh/max, reasoning-effort object shapes, developer role, and
  no generation prompt. All 19 rendered on both models.
  Evidence: `agents/T03/models/<id>.renders.json` (exact output per case).

## Findings

1. **Only `reasoning_effort: "low"` is read; every other level is silently
   ignored.** The template sets `low_effort = reasoning_effort == "low"`
   (template line 8) and only uses it to append `{reasoning effort: low}` to
   the last user turn. `none`, `minimal`, `medium`, `high`, `xhigh` and `max`
   render byte-identical to no effort at all. Severity: **minor** for
   correctness (no error), **major** for behaviour expectations if the app
   exposes effort: `high`/`xhigh`/`max` change nothing on these models.
2. **b11960 has no "Unexpected reasoning effort" error.** The server stores
   any `reasoning_effort` string in `chat_template_kwargs`
   (`tools/server/server-common.cpp:1497-1505`) and
   `common/chat.cpp:1081-1083` passes it to the template; a grep of the whole
   b11960 tree finds no such message. The 500 the brief describes must come
   from a different build/tag or from a template that raises on unknown
   values. For granite, no effort value can 500. Severity: **minor** (brief
   premise correction).
3. **`reasoning_effort: "none"` is special-cased by the server, not the
   template.** `server-common.cpp:1500-1503` sets `enable_thinking = false`
   and erases the kwarg. The template itself ignores the literal `"none"`.
   The CLI `--reasoning-effort none` (`common/arg.cpp:3764-3773`) does *not*
   go through that path: it is passed straight to the template, which ignores
   it, so thinking stays on. Severity: **minor** (app may rely on none
   disabling thinking).
4. **The template always emits a system turn, even with no system message.**
   `system_message` is set to `""` in the no-system branch and then tested
   with `is defined`, which is always true. A plain user chat renders
   `\n<|im_start|>system\n<|im_end|>\n<|im_start|>user\n...`. Severity:
   **minor** (wasted tokens, unexpected prompt shape), not a failure.
5. **Whitespace leaks into the prompt.** A leading newline before the first
   system turn, and `\n        \n            ` before an assistant tool-call
   turn. Severity: **minor**.
6. **Developer role is accepted by the template but rewritten by the server.**
   The template's else branch renders `<|im_start|>developer\n...`; b11960
   rewrites `developer` to `system` before rendering
   (`common/chat.cpp:1517-1518`), so through the server the branch is never
   used. `developer_role: true` is still correct (honoured, not dropped).
   Severity: note.
7. **Tool arguments as a JSON string raise**
   `Can only get item pairs from a mapping.` from the `arguments|items` loop.
   b11960 converts arguments to a mapping first when caps say object arguments
   are supported, so this is only reachable with a hand-rolled prompt.
   Severity: **minor**.

## b11960 caps cross-check

From `common/jinja/caps.cpp` the expected `/props` caps for both models are:
`supports_tools=true`, `supports_tool_calls=true`,
`supports_parallel_tool_calls=true`, `supports_system_role=true`,
`supports_preserve_reasoning=true` (the template honours
`truncate_history_thinking`), `supports_reasoning_effort=true`, and
`reasoning_effort_levels=["low"]` (only `low` differs from the unknown-value
render; `minimal`/`medium`/`high`/`xhigh`/`max` collapse onto the fallback).
`caps.cpp:598-645` is the level probe; `value.cpp:250-276` is `tojson`.

## Sources

- `https://huggingface.co/ibm-granite/granite-4.2-30b-GGUF/resolve/main/granite-4.2-30b-Q2_K.gguf`
- `https://huggingface.co/ibm-granite/granite-4.2-8b-GGUF/resolve/main/granite-4.2-8b-Q3_K_M.gguf`
- llama.cpp tag b11960: `common/chat.cpp`, `common/jinja/caps.cpp`,
  `common/jinja/value.cpp`, `tools/server/server-common.cpp`,
  `tools/server/README.md:1022,1449`, `common/arg.cpp:3764-3773`.

## Open questions

- Do llama.cpp's jinja runtime and Python jinja2 produce identical whitespace
  for this template? The leading newline and stray spaces are recorded from
  Python jinja2; the server's own runtime is the authority and was not run
  here (no model load allowed).
- Does the app set effort per request or via `--reasoning-effort`? The two
  paths disagree for `none` (see finding 3).
- Is the always-present empty system turn intentional on IBM's side?

## Files

`agents/T03/models/granite-4.2-30b.{json,md,renders.json,templates.json}`,
`agents/T03/models/granite-4.2-8b.{json,md,renders.json,templates.json}`,
`agents/T03/a14/render.py`, this report.
