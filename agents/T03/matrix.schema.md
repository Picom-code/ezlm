# T03 matrix schema

T03 produces one row per catalog model, one file per row, and a merged matrix.
Everything is JSON with a stable key set so the app can read it without
guessing.

```
agents/T03/models/<id>.json            one matrix row (hand-checked, per agent)
agents/T03/models/<id>.renders.json    every render case, output or exception
agents/T03/models/<id>.templates.json  the raw templates from the GGUF header
agents/T03/models/<id>.md              the readable section for the report
agents/T03/matrix.json                 merged rows, written by assemble.py
```

`assemble.py` reads `models/*.json`, skips `*.renders.json`,
`*.templates.json` and names starting with `_`, and writes `matrix.json` as
`{"version": 1, "models": [...]}` with the rows sorted by `id`.

## Row

Every row is a JSON object. The keys below are required; `assemble.py` warns
when one is missing but still includes the row, so a partial agent does not
block the merge.

| key | type | meaning |
|---|---|---|
| `id` | string | catalog id, the join key. Must match `internal/catalog/catalog.json`. |
| `repo` | string | catalog `repo` of the model. |
| `template_source` | string | where the template came from. `tokenizer.chat_template` is the only source used so far; use `tokenizer.chat_template.<name>` for a named template. |
| `tool_call_format` | string | how a call is emitted, in prose, with the literal markers. |
| `tools_supported` | bool | the template renders tool definitions and calls, and a parser can read the calls back. |
| `thinking_toggle` | string | the variable that turns thinking on and off, e.g. `enable_thinking`; `none` if there is no switch. |
| `effort_levels_accepted` | array of string | values of `reasoning_effort` that rendered **and changed the output**. |
| `effort_levels_rejected` | array of string | values that raised during render. These are the 500s. |
| `system_role` | bool | a `system` message renders as a system turn. |
| `developer_role` | bool | a `developer` message is honoured, not dropped and not an error. |
| `known_template_errors` | array of string | exact exception text, one entry per distinct failure. Copy it, do not paraphrase. |
| `notes` | string | the caveats that do not fit a field: aliases, silent drops, gating. |

Optional keys, used when a row has something more precise to say:

| key | type | meaning |
|---|---|---|
| `effort_levels_ignored` | array of string | values that rendered byte-identical output, so the template does not read the variable. Never list a level in both `accepted` and `ignored`. |
| `effort_aliases` | object | value to the level it renders as, e.g. `{"high": "xhigh"}`. |
| `gguf` | string | the `repo` and file the header was read from, when it differs from the model `repo`. |

## Semantics

- **accepted** means the render succeeded *and* the output differs from the
  default render. A value that renders but is ignored is not accepted.
- **ignored** means the render succeeded and the output is identical to the
  render with no `reasoning_effort` at all. Say "ignored", not "accepted".
- **rejected** means the render raised. Quote the template's own message in
  `known_template_errors`; the server turns that message into the HTTP 500 body.
- Effort values in this matrix are the ones the brief tests: `none`, `low`,
  `medium`, `high`, `xhigh`, `max`. b11960 also probes `minimal`; note it in
  `notes` if it behaves differently.
- Effort is only meaningful when thinking is on. If a template validates effort
  only under `enable_thinking`, say so in `notes`.

## matrix.json

```json
{"version": 1,
 "models": [
   {"id": "...", "repo": "...", "...": "one row as above, no _file key"}
 ]}
```

`assemble.py` prints a compact table and a missing/duplicate report, and exits
0 with any number of rows, including none. `--check` writes nothing and exits 1
if a catalog model has no row, a row is duplicated, or a row id is not in the
catalog.
