# T03 a01 report: ternary-bonsai-2-27b and qwen3.8-27b

## What I did

For each model I read the GGUF header with HTTP range requests (the `Reader`/`header`
helpers in `scripts/verify_catalog.py`, no whole-file downloads) and saved every
`tokenizer.chat_template*` value verbatim to `models/<id>.templates.json`. Neither model has
named templates; both have a single `tokenizer.chat_template`.

I then rendered each template with jinja2 3.1.6 (`~/ezlm-agents/run/venv/bin/python`,
`agents/T03/a01/render.py`) using llama.cpp's conventions: `raise_exception` is a stub that
raises, `add_generation_prompt` is true, tool-call arguments are objects (llama.cpp's
`func_args_not_string` normalises the JSON string first), and message content is already
trimmed. The script is committed so the renders can be reproduced.

Both models ship the **same template**, byte-identical to the upstream
`Qwen/Qwen3.8-27B` `chat_template.jinja` (sha256
`c3cf9e34abf4f9e36c2d72165aa9c132d3e2a725b6c2586aaa3a8af9d7a81041`). The Bonsai weights
are a ternary QAT of Qwen3.8-27B and keep its tokenizer and template.

## Render results

18 cases per model, same result for both: 13 render, 5 raise.

| Case | Result |
|---|---|
| `plain` | renders; injects a system turn with the xhigh reasoning instruction |
| `system_user` | renders; reasoning instruction is prepended to the user system message |
| `tool_call` | renders; `<tools>` block in system, XML call, `<tool_response>` result |
| `parallel_tools` | renders; two adjacent `<tool_call>` blocks, results merged into one user turn |
| `thinking_on` | renders (same as `plain`; this is the default) |
| `thinking_off` | renders; no system turn, generation prompt is ` thinking\n\n</think>\n\n` |
| `effort_none` | **raises** `Unexpected reasoning effort none...` |
| `effort_low` | renders; low instruction |
| `effort_medium` | renders; **no instruction**, silently ignored |
| `effort_high` | **raises** `Unexpected reasoning effort high...` |
| `effort_xhigh` | renders; xhigh instruction |
| `effort_max` | **raises** `Unexpected reasoning effort max...` |
| `effort_minimal` (extra) | **raises** `Unexpected reasoning effort minimal...` |
| `developer_role` | **raises** `Unexpected message role.` |
| `effort_high_thinking_off` (extra) | renders; effort is not validated when thinking is off |
| `effort_nested_reasoning` (extra) | renders as xhigh; `reasoning.effort` is ignored |
| `effort_nested_thinking` (extra) | renders as xhigh; `thinking.effort` is ignored |
| `effort_unset` (extra) | renders as xhigh |

## Findings

### F1 (major): `reasoning_effort` values outside `low|medium|xhigh` raise, and the server returns HTTP 400, not 500

Evidence: `models/<id>.renders.json` cases `effort_none`, `effort_high`, `effort_max`,
`effort_minimal`. Template source `Qwen/Qwen3.8-27B chat_template.jinja:46-55` allows only
`('xhigh', 'medium', 'low')` and raises
`Unexpected reasoning effort <value>. Supported types are xhigh (default), medium, and low.`
The b11960 server passes the value straight into `chat_template_kwargs`
(`tools/server/server-common.cpp:1497-1505`), the template raises, and the chat handler
turns any exception into `ERROR_TYPE_INVALID_REQUEST`
(`tools/server/server-context.cpp:5386-5389`), which is HTTP 400
(`tools/server/server-common.cpp:97-100`). T03's task text says HTTP 500; on b11960 it is
400 with `type: invalid_request_error`.

Impact: agent apps that default to `reasoning_effort: "high"` (a common OpenAI-compatible
default) get a 400 on every request to either model. This is the exact failure T03 is about.

### F2 (major): `developer` role is rejected

Evidence: `models/<id>.renders.json` case `developer_role`; template line 160 raises
`Unexpected message role.` The server has a system-role fallback
(`system_message_not_supported`, `common/chat.cpp:1165-1178`) but nothing for `developer`,
so a client that sends `developer` gets a 400.

### F3 (minor): thinking is on by default and injects a system turn with `xhigh`

Evidence: `plain` and `effort_unset` renders. `enable_thinking` is undefined in a normal
request, which the template treats as true (line 46), and `reasoning_effort` defaults to
`xhigh` (line 47), so the prompt starts with
`<|im_start|>system\nReasoning effort is set to xhigh...<|im_end|>` even when the client
sent only a user message. With an existing system message the instruction is prepended to
it (line 80), which is fine, but clients that count system turns will see one more.

### F4 (minor): `medium` is accepted but has no effect

Evidence: `effort_medium` render. The value passes the allow-list check (line 48) but no
`reasoning_instructions` branch matches `medium` (lines 51-55), so the prompt is identical
to thinking-on with no effort hint. b11960's capability probe still lists `medium` as a
supported level (`common/jinja/caps.cpp:592-641`), so a client that picks `medium` from
`/props` gets no behavioural change.

### F5 (minor): nested effort shapes are ignored

Evidence: `effort_nested_reasoning` and `effort_nested_thinking` render the xhigh default.
The template reads only the top-level `reasoning_effort`. Clients that send
`{"reasoning": {"effort": "high"}}` silently get xhigh rather than an error.

### F6 (minor): `reasoning_effort=none` is special-cased before the template

Evidence: `tools/server/server-common.cpp:1497-1505` maps `none` to `enable_thinking=false`
and removes the kwarg, so the template's own rejection of `none` never fires over HTTP. The
capability list from `/props` does not include `none` (`common/jinja/caps.cpp:592-641`), so
a client that validates against `/props` will refuse a value the server actually accepts.

## What I changed

Only new files under `agents/T03/`:
`models/ternary-bonsai-2-27b.{json,renders.json,templates.json,md}`,
`models/qwen3.8-27b.{json,renders.json,templates.json,md}`,
`a01/render.py` and this report. No Go, no catalog.

## What I could not finish

- I did not run a real llama-server (forbidden), so the 400-vs-500 claim comes from reading
  b11960 source, not from a live request. The template exception text is verified by the
  render harness.
- Vision and audio content parts, `preserve_thinking` / `clear_thinking` /
  `truncate_history_thinking`, and `add_vision_id` are read by the template but were not
  rendered. The catalog marks both models as vision-capable; `render_content` handles
  `image`/`image_url`/`video` items and raises `System message cannot contain images.` for
  images in a system message.
- The other qwen3.8-27b quant repos were not checked for template drift; the first quant's
  repo is the one the catalog installs.

## Open questions

- Should the matrix record HTTP-level acceptance (where `none` works) or template-level
  acceptance (where it raises)? I recorded template-level in `effort_levels_*` and put the
  server special case in `notes`, because a15 merges the rows as-is.
- T03 says the failure is HTTP 500. If the apps are built against an older llama.cpp tag
  where the chat-template exception was a server error, the docs should say 400 for b11960
  and note the tag.

## Sources

- GGUF templates: `https://huggingface.co/prism-ml/Ternary-Bonsai-2-27B-gguf/resolve/main/Ternary-Bonsai-2-27B-Q2_0.gguf`,
  `https://huggingface.co/ISTA-DASLab/Qwen3.8-27B-GSQ-RCO-GGUF/resolve/main/Qwen3.8-27B-GSQ-RCO-IQ2_XS.gguf`
- Upstream template: `https://huggingface.co/Qwen/Qwen3.8-27B/resolve/main/chat_template.jinja`
- Server code at tag b11960: `tools/server/server-common.cpp`, `tools/server/server-context.cpp`,
  `tools/server/server-chat.cpp`, `common/chat.cpp`, `common/jinja/caps.cpp`
  (raw.githubusercontent.com, GET only).
