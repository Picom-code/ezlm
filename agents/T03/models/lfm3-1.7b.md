# lfm3-1.7b chat template

Source: `tokenizer.chat_template` in `LiquidAI/LFM3-1.7B-GGUF/LFM3-1.7B-QAD-Q4_0.gguf` (GGUF v3 header read with range requests). One template only, no named variants. Rendered with jinja2 3.1.6, 19/20 cases rendered; the one failure is the template's own `raise_exception` and is explained below.

## Tools

ChatML turns: `<|im_start|>role\n...<|im_end|>\n`. Tool definitions are appended to the system prompt as a `<tools>` block of one JSON object per line (the OpenAI `function` object, without the `{"type": "function"}` wrapper), then a synthetic system turn is emitted even when the user sent no system message.

Calls are `<tool_call>\n<function=NAME>\n<parameter=key>\nvalue\n</parameter>\n</function>\n</tool_call>`, one block per call, separated by a blank line for parallel calls. Parallel calls and their results rendered correctly. Results are `<|im_start|>tool\n<tool_response>\n...\n</tool_response><|im_end|>`; consecutive `tool` messages with the same `tool_call_id` are merged into one response block.

Tool arguments must be a mapping. If a client sends `arguments` as an OpenAI JSON string, the template calls `raise_exception` and rendering fails with:

```
Tool call arguments must be a mapping, got a JSON-encoded string: parse arguments with json.loads() before applying the chat template
```

That is a real HTTP 500 on the server if the string reaches the template. b11960 converts string arguments to objects first, but only when the template's caps report `supports_object_arguments` (`common/chat.cpp:1532-1534` calls `workaround::func_args_not_string`), which is detected by checking whether the template reads the argument values. This template does read them, so it is probably converted, but I could not run the server to confirm. Open question.

## Thinking

Toggled by `enable_thinking`, which defaults to **true**. True (or absent) ends the prompt with `<|im_start|>assistant\n thinking`; false ends with `<|im_start|>assistant\n thinking</think>`, the empty closed block that suppresses reasoning. The `thinking` variable is not read, so `thinking: false` does nothing. `preserve_thinking` controls whether history turns keep their ` thinking` blocks, not the generation prompt.

## Reasoning effort

The template never reads `reasoning_effort` or `reasoning`. All six values (none, low, medium, high, xhigh, max) render byte-identically to the plain prompt, and so do `reasoning.effort` and `thinking.effort`. Silence, not support: at b11960 caps report `supports_reasoning_effort=false` and `reasoning_effort_levels=[]` (`common/jinja/caps.cpp:589`). Sending an effort is harmless and has no effect.

## Roles

`system` works, and a position-0 system message is the only one that is merged with the `<tools>` block. A `developer` message is accepted by the template but is not treated as a system prompt: it renders as its own `<|im_start|>developer` turn. In practice b11960 rewrites developer to system before rendering (`common/chat.cpp:1517-1518`, all models except GPT-OSS), so a developer message becomes the system prompt. Do not rely on a developer message landing anywhere but first.

## Recommendation

Send tool arguments as objects, not JSON strings, and do not send `reasoning_effort`; it changes nothing. Thinking is on by default, so pass `enable_thinking: false` as a chat-template kwarg when you want a direct answer.

## Server cross-check (b11960)

- `{% generation %}` / `{% endgeneration %}`: this template uses them. b11960's parser ignores both tags (`common/jinja/parser.cpp:219-220`), so the server renders it fine; plain jinja2 raises `Encountered unknown tag 'generation'`, so my harness strips them before rendering (recorded in the renders file under `preprocessing`). Nothing to fix on the server.
- `loop.previtem` / `loop.nextitem` (used for `additional_tools` merging) exist at b11960 (`common/jinja/runtime.cpp:621-622`), and `| tojson` is a builtin (`common/jinja/value.cpp:464`), so the tool paths should work.
- No mismatch found beyond the developer-role rewrite above.
