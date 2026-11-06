# T03. Chat template capability matrix — consolidated report

16 agents (a01–a16) worked this task in parallel, two catalog models each, so all 32 entries in
`internal/catalog/catalog.json` were rendered from their real GGUF chat templates with jinja2.
Consolidated onto `agent/T03-template-matrix` by the Lead.

## Result

- **32/32 models covered** in `agents/T03/matrix.json` (one row per model) and
  `agents/T03/models/<id>.{json,md,renders.json,templates.json}`.
- Each model was rendered for 14–35 cases: plain, system+user, tool definition + assistant tool call
  + tool result, two parallel tool calls, `enable_thinking` on/off, `reasoning_effort` at
  none/low/medium/high/xhigh/max (plus `reasoning.effort` and `thinking.effort` shapes), and a
  `developer` role. Every render records either the exact output or the exact exception.
- **`agents/T03/checks.py`** cross-checks each row against b11960's server rules; it passes most
  rows and flags 8 rows for a follow-up (see "Known discrepancies").
- `agents/T03/llamacpp-notes.md` documents how b11960 chooses and applies templates, with file:line.

## The headline correction

**There is no "Unexpected reasoning effort" error in llama.cpp b11960.** Three agents grepped the
whole tag independently (common/, tools/server/, all 75 bundled `.jinja` files): zero matches. What
b11960 does is:

- `tools/server/server-common.cpp:1497-1505` turns `reasoning_effort: "none"` into
  `enable_thinking=false` and removes the kwarg; every other string is passed to the template
  unchanged;
- a template that does not read the variable renders byte-identically (silent no-op);
- a template that raises produces HTTP 500 `Jinja Exception: <template raise text>`
  (`common/jinja/value.cpp:405`, `tools/server/server.cpp:61-87`).

The literal "Unexpected reasoning effort" string lives in **EZLM's own fakellm**
(`internal/fakellm/fakellm.go:232`) and is matched in `internal/integrate`, so EZLM's integration
tests can pass against a message no real server produces. The brief's premise was inverted: real
servers fail differently.

## Effort handling, by model

| Behaviour | Models |
|---|---|
| **Ignores `reasoning_effort` entirely** (all levels render identically; silent no-op) | bonsai-8b-1bit, devstral-small-2-24b, gemma-4-12b, gemma-4-26b-a4b, gemma-4-31b, gemma-4-e2b, gemma-4-e4b, lfm3-1.7b, nanbeige4.3-33b, nemotron-3.5-bolt-74b-a9b, nemotron-3.5-lightning-30b-a3b, nemotron-3.5-super-vl-120b-a12b, qwen3.5-122b-a10b, qwen3.5-2b, qwen3.5-4b, qwen3.5-9b, qwen3.6-27b, qwen3.6-35b-a3b, ternary-bonsai-2-2b/2-4b/2-9b |
| **Honours a subset** | granite-4.2-3b/8b/30b: only `low` changes output. qwen3.8-27b and ternary-bonsai-2-27b: `low`, `medium`, `xhigh` accepted; **`none`, `high`, `max`, `minimal` raise** ("Unexpected reasoning effort <v>. Supported types are xhigh (default), medium, and low."). qwen3.8-flash-next: `low`–`xhigh` accepted, `none` and `max` raise. muse-glimmer-30b: accepts all six but silently drops the kwarg for `none`/`high`. |
| **Strictly validates** | **mistral-small-4-119b accepts only `none` or `high`** and raises "reasoning_effort must be either \"none\" or \"high\"" for anything else (HTTP 500). |
| **No template effort at all** | gpt-oss-120b/20b: accepts every string, only low/medium/high are in-distribution; `reasoning_effort: "none"` does **not** disable thinking (use `reasoning_budget_tokens: 0`). |

**The actionable risk:** `high` is the most common client default (it is EZLM's own default) and
`qwen3.8-27b`, `ternary-bonsai-2-27b` and `mistral-small-4-119b` reject or mishandle it. EZLM should
send per-model effort, or nothing at all.

## Other findings

1. **Developer role.** b11960 maps `developer` → `system` before rendering
   (`common/chat.cpp:1517-1518`), except for GPT-OSS. Several templates raise
   "Unexpected message role." on a raw developer message, so any client that bypasses the mapping
   gets a 500.
2. **String tool arguments.** Templates that do `tool_call.arguments|items` raise on a JSON string.
   b11960 masks this for models whose caps say `supports_object_arguments` (it rewrites the
   arguments), so it is latent rather than live. **gpt-oss silently truncates parallel tool calls**
   to the first one (`supports_parallel_tool_calls=false`).
3. **Thinking defaults differ under the same `enable_thinking` kwarg** (e.g. gemma-4-e2b/e4b off,
   lfm3-1.7b on), so "no kwarg" is not a safe default.
4. **`--chat-template-file` is needed** for models whose GGUF template is not the one the server
   would choose; `llamacpp-notes.md` lists the selection rules and the exceptions.
5. **Catalog `repo` vs quant `repo`:** for several models the GGUF lives in a separate repo
   (`quants[0].repo`), and the brief's "model repo + first quant file" recipe 404s. Use
   `quants[0].repo || repo`.

## Known discrepancies (follow-up)

`agents/T03/checks.py` reports 45 mismatches across 32 rows. They are conventions, not coverage gaps:

- 22 rows have "no successful baseline render" (the plain render raised), so effort comparison is
  impossible for them.
- 8 rows (bonsai-27b-1bit, devstral-small-2-24b, gemma-4-31b, nemotron-3.5-bolt-74b-a9b,
  nemotron-3.5-lightning-30b-a3b, nemotron-3.5-super-vl-120b-a12b, qwen3.5-9b,
  ternary-bonsai-2-4b) list an effort as ignored while `checks.py` sees a byte difference. The
  difference comes from a code path the server does not take; the row's classification is the
  considered answer, but it should be re-rendered once to confirm.
- 5 rows mark `developer_role=true` although the template never names `developer` (b11960 maps it
  first, so the row is describing the server path, not the template path).

Treat the matrix as authoritative for "what the template does" and `checks.py` as a lint with known
false positives.

## Files

- `agents/T03/matrix.json` — 32 rows, the deliverable.
- `agents/T03/models/<id>.{json,md,renders.json,templates.json}` — per-model row, readable notes,
  every render result, the raw templates.
- `agents/T03/assemble.py`, `matrix.schema.md`, `checks.py`, `llamacpp-notes.md`.
- `agents/T03/a01..a16/REPORT.md` — one report per agent.

## Next steps

1. Fix the premise in EZLM's integration tests: the fake server's "Unexpected reasoning effort" does
   not match real b11960 behaviour.
2. Decide a per-model effort policy (or send none), starting with the three models that reject
   `high`.
3. Re-render the 8 disputed rows with the server's own template selection to settle ignored vs
   accepted.
