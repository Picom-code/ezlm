# T03 / a08 report: qwen3.5-4b and qwen3.5-2b

## What I did

Two models: `qwen3.5-4b` (`Qwen/Qwen3.5-4B`) and `qwen3.5-2b` (`Qwen/Qwen3.5-2B`).
Catalog quants point at `unsloth/Qwen3.5-{4,2}B-GGUF`; I read the first quant's file
(`Qwen3.5-{4,2}B-UD-IQ2_XXS.gguf`) with HTTP range requests through the `Reader` in
`scripts/verify_catalog.py` (`agents/T03/a08/fetch_templates.py`). Nothing was downloaded
whole and no model was loaded.

Both GGUFs carry exactly one template, `tokenizer.chat_template` (7816 bytes), and the two
are byte-identical. Raw templates are saved in `agents/T03/models/<id>.templates.json`.

I rendered them with jinja2 3.1.6 (`agents/T03/a08/render.py`) using llama.cpp b11960's
conventions: `messages`, `tools`, `bos_token`, `eos_token`, `add_generation_prompt` always
present, a `raise_exception` stub that aborts the render, and a compact `tojson` matching
nlohmann's `dump()` output. 20 cases per model: the brief's 15 plus five extras that
llama.cpp or agent apps really produce. Results: 16 rendered, 4 raised, identical for both
models (`agents/T03/models/<id>.renders.json`).

## Findings

1. **`reasoning_effort` is silently ignored (major).** All six levels render
   byte-identical to the render with no effort variable, as do `reasoning_strength`,
   `reasoning: {effort: ...}` and `thinking: {effort: ...}`. The template never names any
   of them. It does not reject them, so there is no 500, but there is also no effect.
   Severity is major for EZLM because a UI that offers effort levels will look broken
   rather than error. Evidence: `agents/T03/models/qwen3.5-4b.renders.json`, cases
   `effort_*`, `changes_render: false`.

2. **`developer` role raises a 500 (major for agent apps).** `raise_exception: Unexpected
   message role.` This is the template's catch-all `else` branch for any role outside
   system/user/assistant/tool. b11960 rewrites `developer` to `system` first
   (`common/chat.cpp:1154`), so a leading developer message survives, but a
   mid-conversation one becomes a non-first system message and hits finding 3 instead. Both
   paths are 500s from the engine's point of view.

3. **A system message after index 0 raises (major).** `raise_exception: System message must
   be at the beginning.` Agent apps that append a system/developer reminder mid-history get
   an HTTP 500 rather than a merged prompt. llama.cpp only merges a *first* system message
   into the next turn when the template does not support system at all
   (`common/chat.cpp:1166`), which is not this case.

4. **Tool arguments are dropped unless they are a mapping (minor, latent).** With the
   OpenAI string form (`"arguments": "{\"city\": \"Paris\"}"`) the template emits
   `<function=get_weather>\n</function>` with no parameters; with an object it emits the
   `<parameter=...>` block. b11960 protects itself by converting string arguments to
   objects when its caps probe sees object-argument use (`common/jinja/caps.cpp:325-332`,
   applied at `common/chat.cpp:1533`), and this template does use
   `tool_call.arguments[...]`, so in practice b11960 renders parameters. Any client that
   bypasses the server's workaround, or an older build without it, silently loses every
   argument.

5. **A history with no plain user turn raises (minor).** `raise_exception: No user query
   found in messages.` A tool-only continuation (assistant tool call + tool result, no user
   message) is a 500. Most agent loops keep the user turn, so this is rare.

6. **The T03 premise needs correcting (major for the matrix).** TASKS.md says a rejected
   reasoning effort shows up as HTTP 500 "Unexpected reasoning effort". That string does
   not exist anywhere in llama.cpp b11960 (`common/`, `tools/server/`). b11960 never
   validates the level: `tools/server/server-common.cpp:1497-1505` passes any non-empty
   string into the template, and only special-cases `"none"` by setting
   `enable_thinking=false` and dropping the kwarg. So a 500 can only come from a
   template's own `raise_exception` in an effort `if` chain, not from the server. The
   matrix should record "ignored" separately from "rejected".

7. **For these two models, effort `"none"` does work (minor, positive).** Because of that
   server special case, `reasoning_effort: "none"` reaches the template as
   `enable_thinking=false` and renders the empty think block. The other five levels do
   nothing. So the honest matrix cell is: `none` accepted (via thinking off), low through
   max ignored.

## How the template behaves (summary)

- Tools: a `<|im_start|>system` turn with `# Tools`, JSON schemas inside `<tools>`,
  the XML call format, then the real system prompt. Calls are
  `<tool_call><function=NAME><parameter=K>V</parameter></function></tool_call>`, one block
  per call; parallel calls and a run of tool results both render correctly.
- Thinking: `enable_thinking` only, at the generation prompt. `true` gives
  `<|im_start|>assistant\n thinking\n`; `false` or absent gives
  `<|im_start|>assistant\n thinking\n\n</think>\n\n`.
- Roles: system (first only), user, assistant, tool. Developer is not handled.

## Changed files

- `agents/T03/models/qwen3.5-4b.json`, `.md`, `.templates.json`, `.renders.json`
- `agents/T03/models/qwen3.5-2b.json`, `.md`, `.templates.json`, `.renders.json`
- `agents/T03/a08/fetch_templates.py`, `agents/T03/a08/render.py`, this report

## Open questions

- The 4B and 2B templates are byte-identical, which is suspicious for two different
  parameter counts. It is what the GGUFs actually carry, so I recorded it, but the Lead may
  want to spot-check a second quant of each before the matrix claims per-model behaviour.
- I could not run a server (hard rule), so the b11960 claims above are from source plus
  template rendering, not from an HTTP request. The object-arguments workaround in
  particular is inferred from `caps.cpp` and `chat.cpp`, not observed on the wire.
- Named templates (`tokenizer.chat_template.*`) do not exist in these GGUFs, so there is
  no tool-use or TTS variant to compare.
