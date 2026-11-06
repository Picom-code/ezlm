# nemotron-3.5-super-vl-120b-a12b

Template read from `tokenizer.chat_template` in the first catalog quant
(`unsloth/NVIDIA-Nemotron-3.5-Super-VL-120B-A12B-GGUF`, UD-IQ1_M shard 1 of 2,
12,927 chars, ChatML format). 23 render cases, 22 rendered, 1 raised.

## Tools

Tools are XML, not JSON. Definitions are rendered into the system turn as a
`<tools><function><name>..</name><description>..</description><parameters>
<parameter><name>..</name><type>..</type><description>..</description>
</parameter></parameters></function></tools>` block, followed by a fixed
instruction telling the model to answer with
`<tool_call><function=name><parameter=key>value</parameter></function></tool_call>`.
Tool results are wrapped as `<tool_response>..</tool_response>` inside a user
turn; parallel results share one user turn and render as consecutive
`<tool_response>` blocks. Parallel calls in one assistant turn render fine.

The parameter rendering is a little sloppy: the macro leaves blank lines and
indentation around `<required>`, but the format is valid.

## Thinking

Thinking is a boolean, `enable_thinking` (default true). With it true the prompt
ends `<|im_start|>assistant\n thinking\n`; with it false it ends
`<|im_start|>assistant\n thinking</think>`.

Effort is not a level. The template only reads the boolean `medium_effort`, and
when true it appends `\n\n{reasoning effort: efficient}` to the last user
message. `reasoning_effort`, `reasoning_strength`, `reasoning.effort` and
`thinking.effort` are all ignored, and all six effort levels render exactly the
same prompt.

History reasoning is controlled by `truncate_history_thinking` (default true):
older assistant `reasoning_content` is dropped unless it is set false, which is
what b11960 does when `preserve_reasoning` is on.

## What breaks

- An assistant tool call whose `arguments` is a JSON string instead of an
  object raises in the `arguments|items` filter. b11960 passes history through
  unchanged, so a client that echoes string arguments gets a 500. Send objects.
- A typed content block of type `thinking` is silently dropped; the reasoning
  history has to arrive as `reasoning_content`.
- The template always emits a system turn, so a request without a system
  message still gets `<|im_start|>system\n<|im_end|>`.
- A `developer` message renders as `<|im_start|>developer\n..<|im_end|>` through
  the generic else branch. It does not raise, but the template never documents
  that role.

## Server mismatch (b11960)

No mismatch for effort: b11960 puts `reasoning_effort` and `reasoning_strength`
into the template context (`common/jinja/caps.cpp:31-35`) and the template
ignores both, so caps correctly reports `supports_reasoning_effort=false`. The
effort the template actually honours, `medium_effort`, is not set by the server
and has to be sent through `chat_template_kwargs`.

## Recommendation

Toggle thinking with `enable_thinking`, not with `reasoning_effort`. If you want
the efficient-effort marker, send `chat_template_kwargs: {"medium_effort": true}`.
Send tool arguments as objects, and send reasoning history as
`reasoning_content`, not as a `thinking` content block.
