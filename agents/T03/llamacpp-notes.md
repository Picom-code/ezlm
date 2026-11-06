# llama.cpp b11960: chat template capabilities, reasoning flags and errors

Sources are the `b11960` tag on `ggml-org/llama.cpp`, read with GET only
(`raw.githubusercontent.com`). Line numbers below are from that tag.

## 1. Where the template comes from

`common_chat_templates_init` (`common/chat.cpp:884`) picks the template:

- `--chat-template-file` / `--chat-template` sets `params.chat_template`, read from
  the file (`common/arg.cpp:3818-3823`). When it is set, only the override is used:
  the GGUF's default and named `tool_use` templates are ignored
  (`common/chat.cpp:889-909`).
- Otherwise the GGUF's `tokenizer.chat_template` is the default template and
  `tokenizer.chat_template.tool_use` (if present) is used for requests that carry
  tools (`common/chat.cpp:891-899`, `common/chat.cpp:1471`).
- If the default is empty or literally `"chatml"`, the `tool_use` template is
  promoted to default, else the built-in `CHATML_TEMPLATE_SRC` is used
  (`common/chat.cpp:904-909`). A model with no GGUF template therefore silently
  gets ChatML, which is the main reason to pass `--chat-template-file`.
- A template that minja cannot parse makes init throw (`common/chat.cpp:962-972`);
  a bad `tool_use` template is only logged and ignored (`common/chat.cpp:974-980`).
- Two source patches are applied before parsing (gpt-oss `<|channel|>` and Mistral
  `[TOOL_CALLS]`, `common/chat.cpp:911-931`).

So a model needs `--chat-template-file` when its GGUF has no template, its template
does not parse in b11960's minja, or the template itself has a bug you must patch
(for example a template that raises on an effort value). Neither model in this slice
needs one: both GGUF templates parse and render.

## 2. How `chat_template_caps` is decided

`chat_template_caps` is `jinja::caps` (`common/chat.cpp:311`,
`common/jinja/caps.h:11-30`). It is computed once when the template is constructed:
`this->caps = jinja::caps_get(prog)` (`common/chat.cpp:1434`), by executing the
template against synthetic inputs and watching which variables the template actually
reads (`common/jinja/caps.cpp:132`). The flags are:

`supports_string_content`, `supports_typed_content`, `supports_tools`,
`supports_tool_calls`, `supports_parallel_tool_calls`, `supports_system_role`,
`supports_preserve_reasoning`, `supports_reasoning_effort`,
`supports_object_arguments` (`common/jinja/caps.h:11-30`,
`common/jinja/caps.cpp:110-119`).

`/props` exposes them as `chat_template_caps`, plus
`chat_template_caps.reasoning_effort_levels` (`common/chat.cpp:1950-1953`). When a
named `tool_use` template exists, its caps win ("take the more expressive template",
`common/chat.cpp:1950-1951`), so `/props` can describe a different template than the
one used for a plain chat request.

### Reasoning effort, exactly

1. `supports_reasoning_effort` is true only if the template reads `reasoning_effort`
   while rendering one user turn (`common/jinja/caps.cpp:567-592`). The variable is
   set as both `reasoning_effort` and `reasoning_strength`
   (`common/jinja/caps.cpp:31-34`).
2. If supported, b11960 renders the same single user turn with
   `minimal, low, medium, high, xhigh, max` (`common/jinja/caps.cpp:596`) and with a
   sentinel `REASONING_EFFORT_UNKNOWN` (`common/jinja/caps.cpp:624`). A level is
   listed only if it renders and its output differs from the sentinel render. Levels
   that render identically are treated as aliases and collapsed to the highest one
   (`common/jinja/caps.cpp:626-641`).
3. Consequences: `none` is never a template level (it is handled before the
   template, see below). A template that reads the variable but renders every value
   the same reports `supports_reasoning_effort: true` with an empty or short level
   list. A template that only checks `== "low"` reports exactly `["low"]`.
4. On the request path the OAI `reasoning_effort` field is parsed in
   `tools/server/server-common.cpp:1497-1505`: `"none"` sets
   `inputs.enable_thinking = false` and erases the kwarg; any other non-empty string
   is copied into `chat_template_kwargs` verbatim. `enable_thinking` may also arrive
   as the string `"true"`/`"false"` in `chat_template_kwargs`
   (`tools/server/server-common.cpp:1487-1495`).
   `--reasoning-effort LEVEL` only changes the server-wide default kwarg
   (`common/arg.cpp:3764-3774`). The kwarg is applied to the render context at
   `common/chat.cpp:1081-1084`. There is no server-side allow-list: an unknown
   level reaches the template unchanged.

## 3. `--reasoning-format`

`--reasoning-format` selects how generated text is split into `reasoning_content`
and `content`; it is not a template variable. Values `none`, `auto`, `deepseek`,
`deepseek-legacy` (`common/chat.cpp:1011-1039`). The CLI sets the default
(`common/arg.cpp`, `params.reasoning_format`), the request body may override it with
`"reasoning_format"` (`tools/server/server-common.cpp:1469-1471`), and it is passed
into the parser at `common/chat.cpp:1487`. `none` forces pure content and disables
separate reasoning/tool rendering (`common/chat.cpp:1563`). Templates that cannot be
autoparsed get a specialized parser (`common/chat.cpp:1258-1330`), so the format
flag changes what the client sees, never what the template renders.

## 4. `--reasoning-budget`

`--reasoning-budget N` sets `sampling.reasoning_budget_tokens` (`common/arg.cpp:3776-3780`);
`-1` is unlimited, `0` ends thinking immediately. It is a sampling-layer limit over
the template's own thinking block, not a template variable:

- the session's thinking start/end tags come from the autoparser's analysis of the
  template (`common/chat.cpp:1443-1450`); the budget is only passed on when the
  template has end tags (`tools/server/server-common.cpp:1517-1531`);
- the sampler is initialized with those tags and the token limit
  (`common/common_sampling.cpp:315-321`), and the server force-injects the end tag
  when the budget is exhausted (`tools/server/server-context.cpp:3123`), optionally
  with `--reasoning-budget-message`.

Per-request overrides are `reasoning_budget_tokens` / `thinking_budget_tokens`
(`tools/server/server-common.cpp:1517-1521`).

## 5. The error text

**"Unexpected reasoning effort" does not appear anywhere in b11960.** I fetched every
`.cpp`/`.h` under `common/` and `tools/server/` plus all 75 `models/templates/*.jinja`
at the tag and searched for the string; there is no match (also absent on `master`
`common/chat.cpp` and `common/jinja/caps.cpp`). The server does not validate effort
levels at all.

What b11960 really does when a template rejects an input:

- `raise_exception(msg)` throws `raised_exception("Jinja Exception: " + msg)` at
  `common/jinja/value.cpp:405`;
- the request wrapper turns any non-`invalid_argument` exception into HTTP 500 with
  `format_error_response(e.what(), ERROR_TYPE_SERVER)`
  (`tools/server/server.cpp:61-87`), so the client sees
  `{"error":{"code":500,"message":"Jinja Exception: <template text>","type":"server_error"}}`.

So the exact message is whatever the template passed to `raise_exception`, always
prefixed `Jinja Exception: `. Examples at the tag:
`models/templates/IFM-K2-Horizon.jinja:992` ("Unsupported reasoning_effort: ...") and
`models/templates/tencent-Hy3.jinja:44`.

EZLM's own fake server is where the literal string comes from:
`internal/fakellm/fakellm.go:232` writes
`"Jinja Exception: Unexpected reasoning effort " + e`, and
`internal/integrate/dsh.go:32`, `opencode.go:34`, `hermes.go:22` match it. A real
b11960 server only produces that text if a model's GGUF template says it. Neither
model in this slice does; for them a bad effort is silently ignored, never a 500.

## 6. Rules a matrix row must not contradict

- `effort_levels_accepted` may only contain levels from
  `minimal, low, medium, high, xhigh, max`; `none` is server-side thinking-off, not
  a template level (`common/jinja/caps.cpp:596`,
  `tools/server/server-common.cpp:1497-1505`).
- An accepted level must render differently from the default; otherwise b11960's caps
  would not report it (`common/jinja/caps.cpp:624-641`).
- A row may not claim effort support when the template never reads
  `reasoning_effort`/`reasoning_strength` (`common/jinja/caps.cpp:567-592`).
- `tools_supported`, `developer_role`, `system_role` and the thinking toggle must
  match the template source (`common/jinja/caps.cpp:230-470` for tools,
  `common/chat.cpp:1523` for the developer-to-system mapping).
