# T03 a02: ternary-bonsai-2-9b and qwen3.6-35b-a3b

Agent `a02` of task T03. Slice: the two models `ternary-bonsai-2-9b` and
`qwen3.6-35b-a3b`.

## What I did

1. Read the chat template from the GGUF header only, with HTTP range requests, using the
   `Reader` class in `scripts/verify_catalog.py` (no weights downloaded). Files:
   - `ternary-bonsai-2-9b`: `prism-ml/Ternary-Bonsai-2-9B-gguf`,
     `Ternary-Bonsai-2-9B-Q2_0.gguf` (first quant Q2_0), architecture `qwen35`.
   - `qwen3.6-35b-a3b`: first quant UD-IQ2_XXS lives in `unsloth/Qwen3.6-35B-A3B-GGUF`,
     file `Qwen3.6-35B-A3B-UD-IQ2_XXS.gguf`, architecture `qwen35moe`. The catalog row's
     `repo` (`Qwen/Qwen3.6-35B-A3B`) is the upstream repo; the quant's own `repo` field
     overrides it, which is what the engine resolves.
   - Both files have exactly one template, `tokenizer.chat_template`; no
     `tokenizer.chat_template.*` variants. Raw templates are in
     `agents/T03/models/<id>.templates.json`, byte-exact.
2. Rendered each template with jinja2 (`~/ezlm-agents/run/venv/bin/python`,
   `agents/T03/a02/render.py`) using llama.cpp's conventions: a `raise_exception` global
   that raises, `messages`/`tools`/`add_generation_prompt`, tool calls with parsed
   (object) arguments, and `enable_thinking` passed explicitly. 23 cases per model: the
   14 required cases plus probes for `thinking`, `reasoning.effort`, `thinking.effort`,
   system-plus-tools, a mid-conversation system message, an unknown role, no user message
   and empty messages. Every result, exact output or exact exception text, is in
   `agents/T03/models/<id>.renders.json`.
3. Cross-checked llama.cpp b11960 (`common/chat.cpp`, `common/jinja/caps.cpp`,
   `tools/server/server-common.cpp`, fetched with GET only) for the developer-role
   workaround, the reasoning-effort plumbing and caps detection.

## Results

| model | cases | failures | tools | thinking | effort |
|---|---|---|---|---|---|
| ternary-bonsai-2-9b | 18/23 | 5 (all its own raises) | yes | `enable_thinking` | all 6 ignored |
| qwen3.6-35b-a3b | 22/23 | 1 (empty messages) | yes | `enable_thinking` | all 6 ignored |

## Findings

1. **Both templates ignore `reasoning_effort` completely.** Neither template contains the
   string `effort`. All six levels (none, low, medium, high, xhigh, max) and the
   `reasoning.effort` / `thinking.effort` shapes render byte-identical to a plain call
   (`renders.json`: `effort_* == thinking_on`). Severity **major** for app behaviour: a
   client that sends effort gets no change and no error, so "thinking harder" silently
   does nothing. It is not the HTTP 500 "Unexpected reasoning effort" failure mode; that
   needs a template that raises, and these do not.
   - b11960 agrees: caps sets `supports_reasoning_effort` only when the variable is
     actually read during the probe render (`common/jinja/caps.cpp:569-593`), so both
     models report `supports_reasoning_effort: false` and an empty
     `reasoning_effort_levels` (`common/chat.cpp:1953`). The server still passes the value
     into the template context (`common/chat.cpp:1081-1083`), so a request with effort
     renders fine.
2. **`reasoning_effort: "none"` means "thinking off", not "template sees none".** The
   server converts it to `enable_thinking = false` and erases the kwarg
   (`tools/server/server-common.cpp:1497-1503`). Both templates then emit the empty
   ` thinking\n\n</think>\n\n` block. So the one effort level with an effect on these two
   models is `none`, and it works only through the server, not through a bare template
   render. Severity **minor** (behaviour worth documenting; also means effort none and
   `enable_thinking: false` are the same request).
3. **`ternary-bonsai-2-9b` raises on the `developer` role** (`Unexpected message role.`,
   template line "else raise"). Severity **minor**, because b11960 maps developer to
   system before rendering for every model except GPT-OSS (`common/chat.cpp:1517-1518`),
   so the server path works. A direct render, or a future server change to that
   workaround, would 500.
4. **`ternary-bonsai-2-9b` raises on a system message that is not `messages[0]`**
   (`System message must be at the beginning.`) and on any role it does not know. Severity
   **minor**: apps that append a system reminder mid-conversation get a 500 instead of a
   dropped message. qwen3.6 is the opposite: it silently drops both
   (`probe_system_mid`, `probe_unknown_role`), which is a silent prompt loss.
5. **Both templates raise `No messages provided.` on an empty message list.** llama.cpp
   renders `messages[:-1]` for `continue_final_message` (`common/chat.cpp:1501-1502`), so
   prefilling from a one-message conversation 500s. Severity **minor**.
6. **`ternary-bonsai-2-9b` raises `No user query found in messages.` when there is no user
   turn; qwen3.6 renders a lone assistant turn.** Severity **minor**; matters for
   assistant-prefill and for tool-loop histories that start with a system + assistant
   message.
7. **Tools render correctly on both.** Definitions as JSON lines in a `<tools>` block in a
   synthetic system turn, leading system content appended after `</IMPORTANT>`, calls as
   `<tool_call><function=NAME><parameter=K>v</parameter></function></tool_call>`, parallel
   calls as consecutive `<tool_call>` blocks, results as `<tool_response>` inside a `user`
   turn. b11960 caps will detect `supports_tools` and `supports_tool_calls` for both
   (`common/jinja/caps.cpp:305-330`). No `--chat-template-file` override is needed for
   either model.

## What I could not do

- No live llama-server (hard rule: never load a model), so server behaviour is read from
  b11960 source, not observed. The `Unexpected reasoning effort` text quoted in the task
  description does not appear anywhere in b11960 `common/` or `tools/server/`; the closest
  mechanism is a template's own `raise_exception` reaching the client as a 500. Worth a
  second pair of eyes in a16.
- Only the first quant of each model was read. `qwen3.6-35b-a3b`'s quants all come from
  the unsloth repo, but I did not confirm that every quant file carries the same template.
- The `prism.*` header keys (Hadamard transforms) are not relevant to chat templates and
  were not checked.

## Open questions

- Should EZLM strip or rewrite `reasoning_effort` for models whose template ignores it, so
  apps do not believe effort took effect? That is a product decision, not a template fact.
- `qwen3.6-35b-a3b`'s template reads `preserve_thinking`; b11960 exposes
  `preserve_reasoning` (`common/chat.cpp:1078`), which is a different kwarg name. A client
  that sends `preserve_thinking` reaches the template (it is a `chat_template_kwargs`
  entry) but the server's own preserve-reasoning path uses `preserve_reasoning`. Verify in
  a16.

## Next steps

- Fold both rows into `matrix.json` (a15).
- If a b11960 binary is available later, replay the `tool_call`, `parallel_tools` and
  `thinking_off` prompts through `/v1/chat/completions` and diff the rendered prompt
  against `renders.json`.
