# T03 a16 report: qwen3.5-122b-a10b, granite-4.2-3b, and the b11960 server rules

Worktree `~/ezlm-agents/work/subs/T03/a16`, branch `agent/T03-a16`. Only
`agents/T03/` was touched.

## Models rendered

Templates were read from the GGUF header with HTTP range requests
(`scripts/verify_catalog.py`'s `Reader`), then rendered with jinja2 3.1.6 using
llama.cpp's context (`messages`, `tools`, `add_generation_prompt`, `bos_token`,
`eos_token`, `enable_thinking`, plus template kwargs), a `raise_exception` stub, and a
`tojson` filter matching minja's default separators and `ensure_ascii=False`
(`common/jinja/value.cpp:250-275`). Arguments are passed as objects, as b11960 does
after `workaround::func_args_not_string` (`common/chat.cpp:1532-1534`).

| model | cases | tool calls | thinking | effort accepted |
|---|---|---|---|---|
| granite-4.2-3b | 15/15 rendered | `<tools>` + `<tool_call><function=...>` XML | `enable_thinking` | `low` only |
| qwen3.5-122b-a10b | 15/15 rendered | `<tools>` + `<tool_call><function=...>` XML | `enable_thinking` | none |

Both models raised nothing for any case. Details in `models/<id>.md`.

### granite-4.2-3b (major finding)

`reasoning_effort` is read only as `reasoning_effort == "low"`, which appends
`\n\n{reasoning effort: low}` to the last user message. `minimal`, `medium`, `high`,
`xhigh` and `max` render byte-identically to the default and are silently ignored.
b11960's caps probe (`common/jinja/caps.cpp:624-641`) therefore reports
`reasoning_effort_levels: ["low"]`. Recommendation: only send `low`; do not advertise
higher levels. (Severity: major for the matrix, because a naive row would claim all
levels work.)

### qwen3.5-122b-a10b (major finding)

The template never reads `reasoning_effort` or `reasoning_strength`; all seven effort
renders are identical. b11960 reports `supports_reasoning_effort: false` and an empty
level list. Recommendation: offer no thinking levels. Also, a `developer` message
after the first system message is dropped by the template's loop guard; a leading one
is merged into the system turn. The server maps developer to system first anyway
(`common/chat.cpp:1523`).

## Cross-cutting deliverables

- `agents/T03/llamacpp-notes.md`: how b11960 picks the template, computes
  `chat_template_caps` (`common/jinja/caps.cpp:132`, effort probe at 567-641), exposes
  `reasoning_effort_levels` in `/props` (`common/chat.cpp:1950-1953`), maps
  `--reasoning-effort` (`common/arg.cpp:3764-3774`), `--reasoning-format`
  (`common/chat.cpp:1011-1039`, 1487, 1563) and `--reasoning-budget`
  (`common/arg.cpp:3776-3780`, `common/common_sampling.cpp:315-321`,
  `tools/server/server-context.cpp:3123`), when `--chat-template-file` is needed
  (`common/chat.cpp:884-980`), and what error a rejected input really produces.
- `agents/T03/checks.py`: flags rows that contradict those rules. It passes both rows
  in this slice (one warning for granite's `developer_role`), and was negative-tested
  with a row claiming `low`, `high` and `none` (three failures, exit 1).

## The "Unexpected reasoning effort" error (blocker for the premise, not for T03)

b11960 does not contain that string. All `.cpp`/`.h` under `common/` and
`tools/server/` and all 75 `models/templates/*.jinja` at the tag were searched; the
string is absent from `master` too. In b11960:

- `raise_exception(msg)` throws `Jinja Exception: <msg>` at
  `common/jinja/value.cpp:405`;
- the HTTP wrapper maps any non-`invalid_argument` exception to HTTP 500 with that
  message (`tools/server/server.cpp:61-87`).

So a real server returns `Jinja Exception: <whatever the template raised>`, and the
template text is model-specific (for example
`models/templates/IFM-K2-Horizon.jinja:992` "Unsupported reasoning_effort: ...").
The literal `Jinja Exception: Unexpected reasoning effort <x>` is written by EZLM's own
fake server (`internal/fakellm/fakellm.go:232`) and matched by
`internal/integrate/*.go`; it is not a b11960 string. Neither model in this slice can
produce it: both accept every effort value silently.

## What could not be finished

- Only the first quant of each model was read; named templates (`tool_use`) were
  absent for both, so caps based on a named template could not be compared.
- Renders were done with jinja2, not minja. Differences: jinja2 is stricter about
  undefined, and `raise_exception`/`tojson` were stubbed to minja semantics. Both
  templates rendered identically under both, but an exact byte comparison against a
  live b11960 server was not possible (no server may be started).
- The `effort_none` render sets `reasoning_effort: none` with `enable_thinking: true`,
  which is the template-level view; the server instead erases the kwarg and sets
  `enable_thinking=false` (`tools/server/server-common.cpp:1497-1505`). For both
  models the resulting text is the same as the default either way.

## Next steps

1. Feed `checks.py` into the a15 assembly step, or run it in CI after `matrix.json`
   is built (point `--models-dir` at `agents/T03/models`).
2. Add the remaining catalog models; rows for templates that read `reasoning_effort`
   (K2 Horizon, Kimi K3, DeepSeek V4) are where the accepted/rejected split matters.
3. Correct EZLM's fake server or the app's error matcher if the real-server text
   matters: either match the `Jinja Exception: ` prefix generically, or keep the fake
   string but note it is synthetic.
