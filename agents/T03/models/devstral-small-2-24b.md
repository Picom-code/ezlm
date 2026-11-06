# devstral-small-2-24b

Template read from `tokenizer.chat_template` in the first catalog quant
(`unsloth/Devstral-Small-2-24B-Instruct-2512-GGUF`, UD-IQ3_XXS, 13,673 chars,
Mistral `[SYSTEM_PROMPT]`/`[INST]` format). 23 render cases, 20 rendered, 3 raised.

## Tools

Tools are JSON, not XML. The definitions go in a `[AVAILABLE_TOOLS]<json>[/AVAILABLE_TOOLS]`
block after the system prompt, and each assistant call is
`[TOOL_CALLS]<name>[ARGS]<json-arguments>`. Tool results come back as
`[TOOL_RESULTS]<text>[/TOOL_RESULTS]`. Parallel calls work: two `[TOOL_CALLS]`
blocks in one assistant turn render, and two results render as two
`[TOOL_RESULTS]` blocks.

## Thinking

There is no thinking control. The template never reads `enable_thinking`,
`thinking`, `reasoning_effort`, `reasoning_strength` or `medium_effort`, and it
never emits `[THINK]`. `add_generation_prompt` adds nothing: generation starts
right after the last `[/INST]`, which is correct for the Mistral format.

## What breaks

- A `developer` role raises `Only user, assistant and tool roles are supported,
  got developer.`
- Two user turns in a row raise `After the optional system message, conversation
  roles must alternate user and assistant roles except for tool calls and
  results.` Agent apps that append a second user message without an assistant
  turn will hit this.
- An assistant content block of type `thinking` raises `Only text chunks are
  supported in assistant message contents.` b11960 turns an assistant message
  with `reasoning_content` into exactly that block, so echoing reasoning history
  back to Devstral is a 500.
- A system message that is not `messages[0]` is treated as a normal turn and
  raises the role error.
- On the first of a month with a `strftime_now` year above 2032 the template
  raises `Unsloth custom template does not support years > 2032.` (minor, future
  date bug).
- Without a system message the template injects a ~2.5 KB default Le Chat
  system prompt that embeds today's and yesterday's date. Token counts and
  behaviour differ from a bare `[INST]` prompt, and the date changes daily.

## Server mismatch (b11960)

b11960 detects this template as Ministral 3 (`[SYSTEM_PROMPT]` + `[TOOL_CALLS]`
+ `[ARGS]`, no `[CALL_ID]`, `common/chat.cpp:1258-1264`) and runs
`common_chat_params_init_ministral_3` (`common/parsers/ministral3.cpp`). That
handler sets `supports_thinking = true` with `[THINK]`/`[/THINK]` tags and
rewrites system and assistant content into typed blocks, even though the
template never mentions thinking. The `Assistant message must have a string or
a list of chunks` raise is patched to `if false` for `[TOOL_CALLS]` templates
(`common/chat.cpp:928-935`), but the `thinking` block raise is not, which is how
reasoning history becomes a 500.

## Recommendation

Do not send `reasoning_effort` to this model, and do not send reasoning history:
the template ignores effort and raises on a `thinking` content block. Keep one
system message at `messages[0]`, alternate user and assistant turns, and expect
the built-in default system prompt when you send none.
