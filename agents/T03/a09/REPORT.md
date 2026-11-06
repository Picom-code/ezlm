# T03 / a09: gemma-4-12b and lfm3-1.7b chat templates

Models: `gemma-4-12b` (unsloth/gemma-4-12b-it-GGUF, UD-IQ2_M) and `lfm3-1.7b` (LiquidAI/LFM3-1.7B-GGUF, QAD-Q4_0).
Both GGUF headers were read with HTTP range requests only (`scripts/verify_catalog.py` `Reader`), no model download.

## Method

- `agents/T03/a09/fetch_templates.py` writes `agents/T03/models/<id>.templates.json` (raw `tokenizer.chat_template`, plus any `tokenizer.chat_template.*`; both models have only the one).
- `agents/T03/a09/render.py` renders 20 cases with jinja2 3.1.6 from `~/ezlm-agents/run/venv`, with llama.cpp's context: `messages`, `tools`, `add_generation_prompt`, `bos_token`, `eos_token`, a `raise_exception` stub that raises, and `strftime_now`. Results are in `agents/T03/models/<id>.renders.json`, with the exact output or exact exception text per case.
- Every effort and thinking case is compared byte-for-byte with the plain baseline render, so "silently ignored" is proved by identical output rather than by absence of an error. Extra cases: JSON-string tool arguments, `thinking` variable, `reasoning.effort` / `thinking.effort`, `add_generation_prompt: false`.
- jinja2 differences from llama.cpp's jinja: `{% generation %}` tags are stripped (see below); missing object keys are falsy in both implementations, so the default `Undefined` matches the server (`common/jinja/runtime.cpp:903` uses the default-value `at()`).

## Results

| model | cases | effort accepted | effort ignored | thinking toggle | tools | developer |
|---|---|---|---|---|---|---|
| gemma-4-12b | 20/20 | none | all six | `enable_thinking`, default **false** | yes, parallel yes | as system when first |
| lfm3-1.7b | 19/20 (1 template raise) | none | all six | `enable_thinking`, default **true** | yes, parallel yes | own turn; server maps to system |

## Findings

1. **major (both models): neither template reads `reasoning_effort`.** All six values render byte-identically to the plain prompt, including the nested `reasoning.effort` and `thinking.effort` shapes. b11960 caps will report `supports_reasoning_effort=false` and `reasoning_effort_levels=[]`. Sending an effort is a no-op, not an error.
2. **major (lfm3-1.7b): JSON-string tool arguments raise.** `agents/T03/models/lfm3-1.7b.renders.json` case `tool_call_string_args` records `Tool call arguments must be a mapping, got a JSON-encoded string...`. Any client that sends `function.arguments` as a string gets a 500 unless b11960's `func_args_not_string` workaround fires (`common/chat.cpp:1532-1534`, gated on caps `supports_object_arguments`). Open question: whether caps detect this template as object-argument capable; could not run the server.
3. **minor (gemma-4-12b): string tool arguments render with plain JSON quotes.** The template strips the outer braces and pastes the raw JSON, so the call renders `call:get_weather{"city": "Paris"}` instead of the `<|"|>` marker form used for object arguments. It renders, but a strict parser may not match the argument names.
4. **minor (lfm3-1.7b): `{% generation %}` blocks.** The template contains them. b11960's parser ignores the tags (`common/jinja/parser.cpp:219-220`), so the server is fine; plain jinja2 needs them stripped, which my harness does. Not a server bug.
5. **note (both): thinking defaults differ.** gemma-4-12b is off by default (empty closed thought channel), lfm3-1.7b is on by default (open ` thinking`). Same kwarg name, opposite default: `enable_thinking`.
6. **note (both): developer role.** b11960 rewrites developer to system before rendering (`common/chat.cpp:1517-1518` -> `workaround::map_developer_role_to_system`), for all models except GPT-OSS. gemma-4-12b also handles developer itself when it is the first message; lfm3-1.7b only special-cases `system`, so a developer message would otherwise render as a `developer` turn.

## Server cross-check (b11960)

- Caps: `common/jinja/caps.cpp:589` sets `supports_reasoning_effort` from whether the template *reads* `reasoning_effort` (`effort->stats.used`); `caps.cpp:596` tries `minimal, low, medium, high, xhigh, max` and keeps only levels whose render differs from the unknown fallback. `none` is not in the server's candidate list.
- `caps_apply_reasoning_effort` sets both `reasoning_effort` and `reasoning_strength` (`common/jinja/caps.cpp:31-35`).
- `common/chat.cpp:1081-1083` is the only place a request's `reasoning_effort` reaches the template context.
- **There is no "Unexpected reasoning effort" check at b11960.** It appears nowhere in `common/chat.cpp`, `common/jinja/*` or `tools/server/*.cpp`; an effort the template ignores renders unchanged and returns 200. The HTTP 500 in the task description must come from a template's own `raise_exception` (some other model's template) or a different llama.cpp version. Worth correcting in the matrix preamble.
- No `--chat-template-file` override is needed for either of my models.

## What I could not finish

- Running b11960 to confirm the object-arguments workaround actually fires for lfm3-1.7b (no server allowed).
- Testing a non-first `developer` message against the server, which rewrites it anyway.

## Next steps

- a16 should encode rule 1 (effort ignored => empty `effort_levels_accepted`, never `rejected`) and the `none`-not-a-server-candidate fact in `checks.py`.
- Whoever writes the app guidance should never send `reasoning_effort` to these two models, and should send tool arguments as objects.
