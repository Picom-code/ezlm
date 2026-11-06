# T03 a04 report: chat templates for `ternary-bonsai-2-2b` and `nanbeige4.3-33b`

## What I did

- Read both GGUF headers with HTTP range requests only (no model download), using the `Reader` in `scripts/verify_catalog.py`. Both files are GGUF v3 and carry exactly one template, `tokenizer.chat_template` (no `tokenizer.chat_template.*` variants). Trimmed header evidence is in `headers/`.
- Saved the raw templates byte-for-byte to `agents/T03/models/<id>.templates.json`.
- Rendered each template with the venv's jinja2 (`agents/T03/a04/render.py`) using llama.cpp's context conventions: `messages`, `tools` (OpenAI `{"type":"function","function":...}` wrapper), `add_generation_prompt`, `enable_thinking`, `bos_token`, `eos_token`, and a `raise_exception` stub. Recorded every output or exception verbatim in `<id>.renders.json`.
- 23 cases per model: plain, system+user, tool call, parallel tool calls, string-argument tool calls, JSON tool format, thinking on/off (plus the `thinking` variable variant and `add_generation_prompt=false`), effort none/low/medium/high/xhigh/max plus nested `reasoning.effort`, developer role, image in system, no user query, system not first.
- Wrote the matrix row (`<id>.json`) and the readable section (`<id>.md`) for each model.

## Results

| model | renders | raised | effort accepted | developer role |
|---|---|---|---|---|
| ternary-bonsai-2-2b | 17 | 6 | none (all silently ignored) | raises |
| nanbeige4.3-33b | 20 | 3 | none (all silently ignored) | renders as `developer` turn |

Both models: tools supported (XML `<tool_call>` after a `<tools>` JSON block), thinking toggled by `enable_thinking` only, system role supported, no reasoning-effort support at all.

## Findings

1. **Major (capability gap, both models): `reasoning_effort` is silently ignored.** `none` through `max` and a nested `reasoning.effort` render byte-identical to no effort (Bonsai `7e67ed1e9c52`, Nanbeige `abb9db5e49f2`). A client that offers effort levels is offering a placebo, and llama.cpp b11960 reports `supports_reasoning_effort=false` with an empty level list, so EZLM should advertise no levels. Evidence: `agents/T03/models/*.renders.json` effort cases; `common/jinja/caps.cpp:567-641`.
2. **Major (Bonsai, developer role):** the raw template raises `Jinja Exception: Unexpected message role.` for a `developer` message (`ternary-bonsai-2-2b.templates.json`, final `else` branch). llama-server hides this with `workaround::map_developer_role_to_system` (`common/chat.cpp:1516-1519`), but any direct template consumer sees it.
3. **Major (both, string tool arguments):** both templates iterate `tool_call.arguments|items`, which needs an object; a JSON-string `arguments` value fails. The server only survives because caps reports `supports_object_arguments` and rewrites arguments (`common/chat.cpp:1532-1533`). Any client that builds prompts itself must pass objects.
4. **Minor (Nanbeige): a Chinese default system prompt is injected** (`你是南北阁...`) when the first message is not `system`; with tools it injects a longer Chinese tool-use preamble. Clients that assume an empty prefix will be surprised.
5. **Minor (Bonsai): template rejects** images in the system turn, an empty conversation (`No user query found in messages.`) and a non-first system message. Nanbeige rejects only the non-first system message.
6. **Minor (Nanbeige): `tool_call_format` is an undocumented template variable** that switches tool calls to JSON; it only takes effect through `chat_template_kwargs` (`common/chat.cpp:1062-1068`), and `/props` does not advertise it.
7. **Note: `Unexpected reasoning effort` does not exist in llama.cpp b11960.** I grepped `common/chat.cpp`, `common/jinja/caps.cpp` and every `tools/server/*.cpp` at tag b11960; no such string. In this repo it exists only in the fake server (`internal/fakellm/fakellm.go:232`) and the app's error matching (`internal/integrate/*`). The server forwards non-`none` effort values as template kwargs (`tools/server/server-common.cpp:1497-1505`) and never validates them. For these two models, no effort value can cause that HTTP 500 from the template; the 500 would have to come from a template that raises on an unknown effort (not these).

## Reproducing

```sh
~/ezlm-agents/run/venv/bin/python agents/T03/a04/render.py ternary-bonsai-2-2b
~/ezlm-agents/run/venv/bin/python agents/T03/a04/render.py nanbeige4.3-33b
```

The renderer prints a one-line-per-case summary and rewrites `<id>.renders.json`. Header fetches used `scripts/verify_catalog.py:header()`; trimmed headers are committed, the 1-3 MB raw headers are not.

## Open questions

- Does the Nanbeige model actually understand the `developer` role token it emits, or should the server mapping to `system` be treated as the only correct path? Unverified without loading the model (forbidden here).
- Should the matrix expose "none" as an accepted effort for these models? It changes the prompt only because the server maps it to `enable_thinking=false` (`server-common.cpp:1497-1505`), not because the template reads it. I left `effort_levels_accepted` empty to match llama.cpp caps, and documented the `none` special case in the rows.
- The Bonsai file declares `qwen35` plus PrismML Hadamard metadata; the template is a stock Qwen3.5 one. Whether the ternary build keeps the same tool-call reliability is untested.

## Files

`agents/T03/models/ternary-bonsai-2-2b.{json,md,templates.json,renders.json}`, `agents/T03/models/nanbeige4.3-33b.{json,md,templates.json,renders.json}`, `agents/T03/a04/render.py`, `agents/T03/a04/headers/*.json`.
