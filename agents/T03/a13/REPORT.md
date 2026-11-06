# T03 a13: Nemotron 3.5 Bolt 74B A9B and Nemotron 3.5 Lightning 30B A3B

Agent a13 of T03. Both models are NVIDIA Nemotron 3.5 MoE models in the
catalog with `tool_calling: true`. I read each `tokenizer.chat_template` from
the GGUF header with HTTP range requests (no weights downloaded), rendered the
T03 case list with jinja2 3.1.6 following llama.cpp b11960's conventions, and
wrote one matrix row per model.

## What I rendered

| model | template | cases | passed | failed |
|---|---|---|---|---|
| nemotron-3.5-lightning-30b-a3b | `tokenizer.chat_template` | 14 | 14 | 0 |
| nemotron-3.5-bolt-74b-a9b | `tokenizer.chat_template` | 16 | 16 | 0 |

Cases: `plain`, `system_user`, `tool_call`, `parallel_tools`, `thinking_on`,
`thinking_off`, `developer_role`, `effort_{none,low,medium,high,xhigh,max}`.
Extras: `extra_medium_effort_{true,false}` (Bolt only) and
`extra_preserve_reasoning`. Neither GGUF has a named template
(`tokenizer.chat_template.*`), so there is one template per model.

The two templates are the same ChatML template; Bolt adds a vision-aware
`render_content` macro and the `medium_effort` flag.

Rendering conventions matched b11960: `raise_exception` stub, `tojson` with no
HTML escaping, the `items` filter, tool-call arguments as objects (llama.cpp
sets `supports_object_arguments` for these templates), and the context keys
`messages`, `tools`, `add_generation_prompt`, `enable_thinking`,
`reasoning_effort`/`reasoning_strength`, `truncate_history_thinking`,
`bos_token`/`eos_token`.

## Findings

### F1 (minor) Neither template reads `reasoning_effort`; all six levels are silently ignored

Evidence: rendering `plain` with `enable_thinking=true` and each of
`reasoning_effort` = none/low/medium/high/xhigh/max produces byte-identical
output to `thinking_on` (see `same_as_thinking_on` in each
`agents/T03/models/<id>.renders.json`). The templates never mention
`reasoning_effort` or `reasoning_strength`.

Consequence: these models cannot raise the "Unexpected reasoning effort" 500;
they also cannot honour an effort request. `effort_levels_accepted` and
`effort_levels_rejected` are both empty; all six levels are in
`effort_levels_ignored`. `minimal` (which b11960 also probes) is ignored too.

One server-level exception: b11960 maps OAI `reasoning_effort: "none"` to
`enable_thinking=false` (`tools/server/server-common.cpp:1497`), so `none`
does disable thinking through the server.

### F2 (minor) Bolt's real effort knob is the boolean `medium_effort`, which OAI `reasoning_effort` cannot reach

Evidence: with `medium_effort=true` the template appends
`\n\n{reasoning effort: efficient}` to the last user message; with `false` the
output is unchanged (`extra_medium_effort_true` / `_false` in
`nemotron-3.5-bolt-74b-a9b.renders.json`). The only way to set it is
`chat_template_kwargs: {"medium_effort": true}`.

### F3 (minor) Both templates always emit an empty system turn when no system message is given

Evidence: `plain` renders
`<|im_start|>system\n<|im_end|>\n<|im_start|>user\n...`. The template sets
`system_message = ""` in the else branch and then tests `system_message is
defined`, which is always true.

### F4 (minor) The developer role is rendered by the template but remapped to system by b11960

Evidence: `developer_role` renders `<|im_start|>developer\n...`, but
`common/chat.cpp:1154` `map_developer_role_to_system` is called at `:1518`
for every model except GPT-OSS, so the server sends it as a system turn. The
row records `developer_role: true` (the template honours the role) with this
caveat in `notes`.

### F5 (minor, process) The brief's URL recipe 404s for both models

The brief says to read `repo` from the catalog and the first quant's `file`.
For these models the first quant lives in a different repo, so the model repo
plus that file is a 404:

- `nvidia/NVIDIA-Nemotron-3.5-Bolt-74B-A9B-BF16` +
  `NVIDIA-Nemotron-3.5-Bolt-74B-A9B-24GB.gguf` -> HTTP 404
- `nvidia/NVIDIA-Nemotron-3.5-Lightning-30B-A3B-BF16` +
  `NVIDIA-Nemotron-3.5-Lightning-30B-A3B-UD-IQ3_XXS.gguf` -> HTTP 404

Use `quants[0].repo` (falling back to the model `repo`) plus
`quants[0].files[0]`, as `scripts/verify_catalog.py:109` already does. The
rows keep the catalog model `repo` in `repo` and the real header source in
`gguf`.

### F6 (info) b11960 has no server-side "Unexpected reasoning effort" string

I fetched `common/chat.cpp`, `common/jinja/caps.cpp`, `common/jinja/caps.h`,
`tools/server/server.cpp`, `server-common.cpp`, `server-chat.cpp`,
`server-task.cpp` and `server-http.cpp` at tag b11960. None contains the
string. The 500 body must come from a template's own `raise_exception`; the
Nemotron templates never raise, so they cannot produce it.

Relevant b11960 code:

- `common/jinja/caps.cpp:31` binds `reasoning_effort` and
  `reasoning_strength`; `:583` sets `supports_reasoning_effort` only when the
  template's stats show the variable was used. Not used here, so caps report
  no effort support.
- `tools/server/server-common.cpp:1497` parses the OAI field; `none` clears
  the kwarg and sets `enable_thinking=false`, any other value becomes a
  template kwarg.
- `common/chat.cpp:1532` converts tool-call argument strings to objects when
  `supports_object_arguments` is set, which these templates trigger by reading
  `tool_call.arguments|items`. A renderer that passes a raw JSON string will
  iterate its characters and produce a broken call.

### F7 (info) b11960 recognises this tool-call format as its Qwen3-Coder format

Evidence: the template source contains the three markers the detector looks for
(`<tool_call>`, `<function=`, `<parameter=`), so `common/chat.cpp:1412-1420` selects
`common_chat_params_init_qwen3_coder` instead of the generic autoparser. The detector's
comment names Nemotron Nano 3 as another user of the same format. Consequence: tool calls
emitted in this format are parsed by a dedicated, well-tested path, so no
`--chat-template-file` override is needed for tools.

## Files

- `agents/T03/models/nemotron-3.5-bolt-74b-a9b.{json,renders.json,templates.json,md}`
- `agents/T03/models/nemotron-3.5-lightning-30b-a3b.{json,renders.json,templates.json,md}`
- `agents/T03/a13/render.py` (reproduces everything; run from the worktree root)
- `agents/T03/a13/REPORT.md` (this file)

No code outside `agents/T03/` was changed.

## Checks

`gofmt -l .` empty and `go build ./...` passes (no Go files touched). Python
helper runs under `~/ezlm-agents/run/venv/bin/python`; no `__pycache__`
committed.

## Open questions

- Should the matrix treat Bolt's `medium_effort` as an effort level? I kept it
  out of `effort_levels_*` because it is a different, boolean variable, and
  described it in `notes`.
- `supports_reasoning_effort=false` in b11960 means an app that sends
  `reasoning_effort` gets no warning at all. Whether EZLM should warn on
  "effort requested but ignored" is a product decision, not a template one.

## Next steps

- a15: merge the two rows; both have the required keys plus
  `effort_levels_ignored` and `gguf`.
- a16: `checks.py` should not flag these rows; the templates cannot raise and
  have no effort levels. It may want to special-case Bolt's `medium_effort` in
  the notes if it scans for effort knobs.
- If EZLM ever exposes an effort control, map it to `enable_thinking` for both
  models, and to `medium_effort` as well for Bolt.
