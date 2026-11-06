# T03 a03: chat template matrix for two catalog models

Agent a03. Models: `ternary-bonsai-2-4b`, `gemma-4-31b`. Branch `agent/T03-a03`.

## What I did

For each model I read the GGUF header with HTTP range requests only (no weights
downloaded) using the `Reader` class from `scripts/verify_catalog.py`, and saved
every `tokenizer.chat_template*` key verbatim under
`agents/T03/models/<id>.templates.json`. Both models carry exactly one unnamed
template.

I then rendered each template with Python jinja2 3.1.6 following llama.cpp
b11960's conventions (`common/chat.cpp`): the context always has `messages`,
`bos_token`, `eos_token`, `enable_thinking`; `add_generation_prompt` only when
true; `tools` only when the request has tools; `reasoning_effort` as an extra
variable; `tojson` is a plain JSON dump; `raise_exception` aborts the render.
Tool-call arguments were rendered in both shapes, JSON string and object,
because llama.cpp only converts to an object when the template's caps say so.
`bos_token` and `eos_token` were empty strings, matching llama.cpp's own caps
probe, so the leading `{{- bos_token }}` in the gemma template is absent from
the saved renders. 17 cases per model; every output and every exception text is
in `agents/T03/models/<id>.renders.json`.

Results: ternary-bonsai-2-4b 14/17 render, 3 raise. gemma-4-31b 17/17 render.

## Findings

1. **Neither template reads `reasoning_effort` at all (major, task premise).**
   For both models, `none`, `low`, `medium`, `high`, `xhigh`, `max` and
   `REASONING_EFFORT_UNKNOWN` render byte-identically to `thinking_on`.
   Evidence: `effort_*` cases in both `.renders.json`, `effort_distinct_renders`
   is 2 (the thinking-on render and the server's `none` render), not 7.
   llama.cpp's own caps probe would report `supports_reasoning_effort=false`
   (`common/jinja/caps.cpp:583-591`), and the server still passes the variable
   through harmlessly (`common/chat.cpp:1081-1083`). So for these two models
   effort is silently ignored: not accepted, not rejected, no 500. The risk is
   a client that believes it changed reasoning depth when it did not.

2. **b11960 has no "Unexpected reasoning effort" error (major, cross-check).**
   I grepped the full b11960 source tree (tarball from codeload) for the string:
   no match. The server accepts any effort string, and `reasoning_effort: "none"`
   is special-cased to `enable_thinking=false` with the kwarg removed
   (`tools/server/server-common.cpp:1497-1505`). Any HTTP 500 with that text must
   come from a model template that raises it, or from a different llama.cpp
   build. For my two models it cannot happen.

3. **ternary-bonsai-2-4b raises on JSON-string tool arguments (minor).**
   The template iterates `tool_call.arguments|items`, so a string raises
   `TypeError: Can only get item pairs from a mapping.` Evidence:
   `tool_call__args_string` and `parallel_tools__args_string` in
   `ternary-bonsai-2-4b.renders.json`. The b11960 server is saved by its caps
   pass: the template pipes each argument value through `tojson`, which marks
   the argument value as used during caps analysis
   (`common/jinja/caps.cpp:326-330`), so `supports_object_arguments` is set and
   `func_args_not_string` parses the arguments into an object before rendering
   (`common/chat.cpp:1191-1209`, called at `common/chat.cpp:1532-1534`).
   That is inference from the caps code, not an end-to-end server run. A caller
   that applies this template without llama.cpp's caps pass (plain jinja2,
   `llama_chat_apply_template`, a client-side prompt builder) breaks on every
   tool call.

4. **ternary-bonsai-2-4b rejects the developer role at template level (minor).**
   `RuntimeError: Unexpected message role.` Evidence: `developer_role` case.
   The server maps `developer` to `system` before rendering for every template
   without `<|channel|>` (`common/chat.cpp:1154-1164`, called at
   `common/chat.cpp:1518`), so the HTTP API accepts developer messages. A
   renderer that skips the mapping does not.

5. **gemma-4-31b renders the string-arguments branch with quoted keys (minor).**
   The server keeps arguments as a JSON string because the template never reads
   `function.arguments.arg`, so `supports_object_arguments` is false. The
   template's string branch strips the outer braces and emits
   `call:get_weather{"city": "Paris", "units": "celsius"}`, while its
   object-arguments branch would emit the canonical Gemma form
   `call:get_weather{city:<|"|>Paris<|"|>}`. Evidence:
   `tool_call__args_string` vs `tool_call__args_object` in
   `gemma-4-31b.renders.json`. Prompt-only difference; whether it costs
   accuracy is unproven.

6. **gemma-4-31b's catalog `repo` has no GGUF (minor, catalog).**
   `internal/catalog/catalog.json` gives `"repo": "google/gemma-4-31B-it"`, but
   the files (`gemma-4-31B-it-UD-IQ2_XXS.gguf`) live in
   `unsloth/gemma-4-31B-it-GGUF`, which is the quant's own `repo` override. A
   consumer that reads only the model-level `repo` will not find the file. This
   matches `scripts/verify_catalog.py`, which already prefers the quant repo.

## Recommendations

- EZLM should send `enable_thinking` explicitly for both models, and must not
  promise that `reasoning_effort` does anything for them.
- Any EZLM code that renders a template itself (not through llama-server) must
  pass tool-call arguments as objects for ternary-bonsai-2-4b and map developer
  to system, or it will raise.
- Treat "Unexpected reasoning effort" 500s as a template property, not a server
  rule, when triaging user reports on b11960.

## What I could not finish / open questions

- I did not run a real llama-server (forbidden: no model loads). The
  `supports_object_arguments=true` claim for ternary-bonsai-2-4b is inferred
  from the caps stats code, not observed in `/props`.
- I did not test images, video, `tool_choice`, `json_schema`, `preserve_thinking`
  or `clear_thinking`; neither template was exercised on those paths.
- Open: does EZLM ever render chat templates outside llama-server? If yes, the
  object-arguments requirement matters in production.

## Reproduce

```sh
cd ~/ezlm-agents/work/subs/T03/a03
~/ezlm-agents/run/venv/bin/python agents/T03/a03/fetch_templates.py ternary-bonsai-2-4b
~/ezlm-agents/run/venv/bin/python agents/T03/a03/fetch_templates.py gemma-4-31b
~/ezlm-agents/run/venv/bin/python agents/T03/a03/render.py ternary-bonsai-2-4b
~/ezlm-agents/run/venv/bin/python agents/T03/a03/render.py gemma-4-31b
```

## Sources

- GGUF headers: `https://huggingface.co/prism-ml/Ternary-Bonsai-2-4B-gguf/resolve/main/Ternary-Bonsai-2-4B-Q2_0.gguf`,
  `https://huggingface.co/unsloth/gemma-4-31B-it-GGUF/resolve/main/gemma-4-31B-it-UD-IQ2_XXS.gguf`
- llama.cpp tag b11960: `common/chat.cpp`, `common/chat.h`,
  `common/jinja/caps.cpp`, `common/jinja/caps.h`,
  `tools/server/server-common.cpp`, `tools/server/server-context.cpp`,
  `tools/server/server-chat.cpp` (raw.githubusercontent.com, GET only).
