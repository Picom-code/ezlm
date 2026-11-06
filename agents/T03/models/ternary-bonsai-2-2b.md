# Ternary Bonsai 2 2B (`ternary-bonsai-2-2b`)

Source: `tokenizer.chat_template` from `prism-ml/Ternary-Bonsai-2-2B-gguf` / `Ternary-Bonsai-2-2B-Q2_0.gguf` (GGUF v3, header read with range requests). No named `tokenizer.chat_template.*` variants. Raw template: `ternary-bonsai-2-2b.templates.json`; all 23 renders (17 rendered, 6 raised): `ternary-bonsai-2-2b.renders.json`.

## How it works

This is the Qwen3.5 ChatML template with a vision extension.

- **Tools.** The tool definitions are serialized with `tojson` into a `<tools>...</tools>` block inside the system turn, and the instructions tell the model to answer with `<tool_call><function=NAME><parameter=KEY>value</parameter></function></tool_call>`. Tool results are folded into a `user` turn as `<tool_response>...</tool_response>`. Parallel calls are supported: two `tool_calls` in one assistant turn render as two consecutive `<tool_call>` blocks, and consecutive tool results share one user turn.
- **Thinking.** Toggled only by `enable_thinking`. `true` opens `assistant\n thinking\n`; `false` opens `assistant\n<think>\n\n</think>\n\n`. The variable `thinking` is ignored, and `add_generation_prompt=false` simply omits the assistant opener.
- **Effort.** `reasoning_effort` is never read. `none`, `low`, `medium`, `high`, `xhigh`, `max` and a nested `reasoning.effort` all render byte-identical (sha256 prefix `7e67ed1e9c52`). They are silently ignored, not rejected.
- **Roles.** `system` works (and must be first). A `developer` message raises `Unexpected message role.` There is no generic role fallback.

## What breaks

- `developer` role: `Jinja Exception: Unexpected message role.`
- Tool calls with `function.arguments` as a JSON string: the template does `tool_call.arguments|items`, which needs an object. llama-server hides this because its caps analysis reports `supports_object_arguments` and rewrites string arguments to objects before rendering.
- No user turn: `Jinja Exception: No user query found in messages.`
- System message not first: `Jinja Exception: System message must be at the beginning.`
- Image item in a system message: `Jinja Exception: System message cannot contain images.` User images are fine (`<|vision_start|><|image_pad|><|vision_end|>`).

## Recommendation

Treat this model as having no effort levels: send `enable_thinking` true or false, and do not offer `low`/`medium`/`high` in a picker, because the choice changes nothing. Never send a `developer` role directly to the template.

## llama.cpp b11960 cross-check

- `tools/server/server-common.cpp:1497` maps `reasoning_effort: "none"` to `enable_thinking=false` and forwards every other value as a `chat_template_kwargs` entry; because this template ignores the variable, nothing raises. `common/jinja/caps.cpp:567-641` marks `supports_reasoning_effort=false` and returns an empty `reasoning_effort_levels`, which `/props` exposes through `common/chat.cpp:1953`.
- `common/chat.cpp:1516-1519` maps `developer` to `system` for every template that does not contain `<|channel|>`, so through llama-server the developer-role failure above does not surface; it does surface for any client that renders the template itself.
- `common/chat.cpp:1532-1533` applies the object-arguments workaround, which is what keeps `arguments|items` working.
- No `Unexpected reasoning effort` string exists anywhere in `common/chat.cpp`, `common/jinja/caps.cpp` or `tools/server/*.cpp` at b11960. In this EZLM tree it only appears in the fake server (`internal/fakellm/fakellm.go:232`) and the app's error matching (`internal/integrate/*`). For this model no effort value can produce that HTTP 500 from the template.
