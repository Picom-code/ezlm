# nemotron-3.5-lightning-30b-a3b

Source: `tokenizer.chat_template` from the first catalog quant
(`unsloth/NVIDIA-Nemotron-3.5-Lightning-30B-A3B-GGUF`,
`...-UD-IQ3_XXS.gguf`); the BF16 repo has no GGUF of its own. There are no
named templates. All 14 render cases passed, none raised.

The template is ChatML: every turn is `<|im_start|>role\ncontent<|im_end|>\n`.

## Tools

Tool definitions are injected into the system turn as a `<tools>` XML block
(`<function><name>`, `<description>`, `<parameters><parameter>...`). The model
is asked to answer a call with

```
<tool_call>
<function=NAME>
<parameter=KEY>
VALUE
</parameter>
</function>
</tool_call>
```

one block per call, so two calls in one assistant turn render as two
consecutive `<tool_call>` blocks. Tool results go back in a **user** turn as
`<tool_response>CONTENT</tool_response>`; several results share one user turn.
When a system message is present the tools block is appended to it, not to a
separate turn.

## Thinking

One switch: `enable_thinking`. `true` (the default) ends the prompt with
`<|im_start|>assistant\n<think>\n`; `false` ends it with
`<|im_start|>assistant\n thinking</think>`. Assistant history that has no
`reasoning_content` is wrapped as ` thinking</think>content`, and history
reasoning is truncated for turns before the last user message unless
`truncate_history_thinking=false` (the server sets that from
`preserve_reasoning`).

## Effort

`reasoning_effort` and `reasoning_strength` are never read. All six values
render byte-identical to a request with no effort, so none of them is
"accepted". `minimal`, which b11960 also probes, is ignored too.

One server-level exception: b11960 turns OAI `reasoning_effort: "none"` into
`enable_thinking=false` before rendering (`tools/server/server-common.cpp`), so
`none` does disable thinking, but through the server and not this template.

## What breaks

Nothing raises. Two quirks:

- With no system message the template still emits an empty system turn
  (`<|im_start|>system\n<|im_end|>\n`), because `system_message` is set to `""`
  and the later `is defined` test passes. Every prompt therefore carries one
  extra empty turn.
- A `developer` role is rendered as `<|im_start|>developer`, but b11960 maps
  `developer` to `system` before rendering for every model except GPT-OSS, so
  in practice it arrives as a system turn.

## b11960 cross-check

- `common/jinja/caps.cpp:31` binds `reasoning_effort` and `reasoning_strength`;
  `:583` sets `supports_reasoning_effort` only if the template's stats show the
  variable was used. It is not, so the server reports
  `supports_reasoning_effort=false` and no effort levels.
- `tools/server/server-common.cpp:1497` parses OAI `reasoning_effort`; `none`
  clears the kwarg and sets `enable_thinking=false`, anything else is passed to
  the template as a kwarg.
- There is no "Unexpected reasoning effort" string in b11960's
  `common/chat.cpp`, `common/jinja/caps.cpp` or `tools/server/*.cpp`. That 500
  comes from templates that call `raise_exception` themselves; this one never
  does, so it cannot produce that error.
- `common/chat.cpp:1154` `map_developer_role_to_system` (called at `:1518`)
  explains the developer-role behaviour above.
- The template reads `tool_call.arguments|items`, so b11960 sets
  `supports_object_arguments` and converts the OAI argument string to an object
  (`common/chat.cpp:1532`). Passing a raw JSON string would iterate its
  characters and break the call.

- The template matches b11960's Qwen3-Coder tool-call detector
  (`common/chat.cpp:1412`, `src.find("<tool_call>")` + `<function=` + `<parameter=`),
  so the server parses calls with `common_chat_params_init_qwen3_coder` rather than the
  generic autoparser. The detector's comment names Nemotron Nano 3 as a user of the same
  format; these two templates match it too.

## Recommendation

Do not send `reasoning_effort` to this model expecting a change; it is silently
ignored. Turn thinking off with `enable_thinking=false`, and send tool calls in
OAI shape with object arguments.
