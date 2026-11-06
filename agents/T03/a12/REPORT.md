# T03 a12: chat template capability matrix, two models

Agent a12. Models: `devstral-small-2-24b`, `nemotron-3.5-super-vl-120b-a12b`.
Worktree `~/ezlm-agents/work/subs/T03/a12`, branch `agent/T03-a12`.

## What I did

1. Read both templates from the GGUF header only, with HTTP range requests,
   using `Reader`/`header()` from `scripts/verify_catalog.py`
   (`agents/T03/a12/fetch.py`). Saved the raw strings in
   `agents/T03/models/<id>.templates.json`.
2. Rendered each template with jinja2 using llama.cpp b11960 conventions
   (`agents/T03/a12/render.py`): context with `messages`, `tools`,
   `add_generation_prompt`, `bos_token`, `eos_token`, plus per-case template
   variables; `raise_exception` throwing `Jinja Exception: <msg>` like minja;
   `tojson` matching minja (no HTML escaping, default separators); `strftime_now`
   pinned to 2026-11-06T12:00:00 so renders are reproducible. Full output and
   exact exception text for every case is in `agents/T03/models/<id>.renders.json`.
3. Wrote one matrix row and one readable section per model.
4. Cross-checked b11960 `common/jinja/caps.cpp`, `common/chat.cpp`,
   `common/parsers/ministral3.cpp`, `tools/server/server-common.cpp`.

Render totals: devstral 20/23 cases rendered, 3 raised; nemotron 22/23 rendered,
1 raised. Cases: plain, system+user, tool call, parallel tools, thinking on/off,
effort none/low/medium/high/xhigh/max, `reasoning.effort`, `thinking.effort`,
developer role, `medium_effort`, no generation prompt, assistant reasoning
history (plain, typed `thinking` block, preserved), consecutive user turns,
string tool arguments.

## Findings

### devstral-small-2-24b

- **Major: no thinking or effort control at all.** The template never reads
  `enable_thinking`, `thinking`, `reasoning_effort`, `reasoning_strength` or
  `medium_effort`, and never emits `[THINK]`. All six effort levels and both
  thinking flags produce byte-identical prompts. Sending `reasoning_effort` is
  harmless but has no effect.
- **Major: reasoning history is a 500.** b11960 detects this template as
  Ministral 3 (`common/chat.cpp:1258-1264`) and
  `common_chat_params_init_ministral_3` (`common/parsers/ministral3.cpp:12-49`)
  rewrites an assistant message with `reasoning_content` into a content block of
  type `thinking`. The template raises `Only text chunks are supported in
  assistant message contents.` for that block (renders.json case
  `assistant_typed_thinking`). The server also sets `supports_thinking = true`
  with `[THINK]`/`[/THINK]` tags for a template that has no thinking markers
  (`common/parsers/ministral3.cpp:54-56`).
- **Major: role handling is strict.** A `developer` message raises `Only user,
  assistant and tool roles are supported, got developer.` Two user turns in a row
  raise `After the optional system message, conversation roles must alternate
  user and assistant roles except for tool calls and results.` Only `messages[0]`
  may be a system message.
- **Minor: default system prompt.** With no system message the template injects
  a ~2.5 KB default Le Chat prompt that embeds today's and yesterday's date
  (`strftime_now`). Prompt token counts and content change daily.
- **Minor: date bug.** On the 1st of a month with a year above 2032 the template
  raises `Unsloth custom template does not support years > 2032.`
- Tools: `[AVAILABLE_TOOLS]<json>[/AVAILABLE_TOOLS]`, calls
  `[TOOL_CALLS]<name>[ARGS]<json>`, results `[TOOL_RESULTS]..[/TOOL_RESULTS]`.
  Parallel calls render. The template's `Assistant message must have a string or
  a list of chunks` raise is patched to `if false` by b11960 for `[TOOL_CALLS]`
  templates (`common/chat.cpp:928-935`), so it will not fire on the server.

### nemotron-3.5-super-vl-120b-a12b

- **Major: effort levels are silently ignored.** No `reasoning_effort` or
  `reasoning_strength` variable. All six levels render identically. The only
  effort knob is the boolean `medium_effort`, which appends
  `\n\n{reasoning effort: efficient}` to the last user message, and the server
  never sets it. Use `chat_template_kwargs: {"medium_effort": true}`.
- **Major: string tool arguments raise.** An assistant `tool_calls[].function.arguments`
  sent as a JSON string raises in `arguments|items` (`TypeError: Can only get
  item pairs from a mapping` in jinja2; minja raises
  `Function 'items' expected value of type value_object`). History is passed
  through unchanged, so echoing string arguments is a 500.
- **Minor: typed `thinking` blocks are dropped** by `render_content`; reasoning
  history must be sent as `reasoning_content`.
- **Minor: always emits a system turn.** No system message still produces
  `<|im_start|>system\n<|im_end|>`.
- **Minor: developer role renders** as `<|im_start|>developer` through the
  generic else branch, without raising, but the template does not document it.
- Thinking: `enable_thinking` boolean, default true; on ends the prompt with
  ` thinking\n`, off with `</think>`. History reasoning is truncated unless
  `truncate_history_thinking=false` (b11960 sets that from `preserve_reasoning`).
- Tools: XML `<tools>` block in the system turn, `<tool_call><function=..>
  <parameter=..>` in the assistant turn, `<tool_response>` blocks in a user turn.
  Parallel calls and parallel results render.

## Server cross-check (b11960)

- b11960 does **not** reject reasoning effort itself. It forwards the raw string
  as `reasoning_effort` (and binds the same value to `reasoning_strength`,
  `common/jinja/caps.cpp:31-35`); `reasoning_effort: "none"` instead sets
  `enable_thinking=false` and drops the kwarg
  (`tools/server/server-common.cpp:1497-1505`). There is no "Unexpected
  reasoning effort" text anywhere in `common/` or `tools/server/`, so that HTTP
  500 must come from a model template's own `raise_exception`, not the server.
- Capability analysis (`common/jinja/caps.cpp:567-641`) only probes the levels
  `minimal, low, medium, high, xhigh, max`, with `enable_thinking=true`, and
  drops levels that render the same as an unknown level or as an earlier level.
  It reports `supports_reasoning_effort=false` for both of my models, which
  matches the renders.
- Neither of my templates needs `--chat-template-file`: both are self-contained
  and render under b11960's jinja implementation.

## Changed files

- `agents/T03/models/devstral-small-2-24b.{json,md,templates.json,renders.json}`
- `agents/T03/models/nemotron-3.5-super-vl-120b-a12b.{json,md,templates.json,renders.json}`
- `agents/T03/a12/{fetch.py,render.py,REPORT.md}`

## Open questions

- Does the EZLM client ever send `reasoning_content` back in history? If yes,
  Devstral 2 needs the thinking block stripped or the model needs a
  `--chat-template-file` override that accepts `thinking` blocks.
- Should the matrix record `supports_preserve_reasoning`? b11960 caps report it
  (nemotron true via `truncate_history_thinking`, devstral false); the row
  schema in the brief has no field for it.
- `minja`'s `tojson` has no HTML escaping and its `items` error text differs
  from jinja2's; the renders record jinja2's text, with minja's noted in the
  `.md` files. Exact server error strings were not produced because no model was
  loaded (README rule).
