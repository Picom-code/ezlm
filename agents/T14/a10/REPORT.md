# T14 a10: prioritised recommendations

## What I did

Read `README.md`, the T14 section of `TASKS.md` and `run/T14-brief.md`. Read EZLM's code in
`~/ezlm-agents/work/subs/T14/a10` to check what already exists before recommending anything
(`internal/serve`, `internal/update`, `internal/keys`, `internal/fit`, `internal/catalog`,
`internal/integrate/CONTRACT.md`, `internal/web`, `internal/bench`, `internal/report`, `BETA.md`,
`cmd/ezlm/main.go`). Then fetched public primary sources for seven of the eight competitors and
wrote `agents/T14/recommendations.md`: 31 ranked recommendations, each with an evidence URL and
access date, an effort (S/M/L) and the EZLM file area it touches.

No code changes. Docs only, as the brief requires.

## Sources (all fetched 2026-11-06)

| Source | URL | Used for |
|---|---|---|
| Ollama README | https://raw.githubusercontent.com/ollama/ollama/main/README.md | `ollama launch` integrations, REST defaults |
| Ollama FAQ | https://docs.ollama.com/faq | auto-update, 4096-token default |
| Ollama OpenAI compatibility | https://docs.ollama.com/api/openai-compatibility | local server ignores the API key |
| LM Studio OpenAI compatibility | https://lmstudio.ai/docs/developer/openai-compat | endpoints, `/v1/responses`, Codex |
| LM Studio headless | https://lmstudio.ai/docs/developer/core/headless | llmster, JIT loading, auto-evict, startup |
| Jan README | https://raw.githubusercontent.com/menloresearch/jan/main/README.md | :1337 server, HF models, MCP, RAM tiers |
| GPT4All README | https://raw.githubusercontent.com/nomic-ai/gpt4all/main/README.md | separate ARM installer, Vulkan quant limits, LocalDocs |
| llamafile README | https://raw.githubusercontent.com/mozilla-ai/llamafile/main/README.md | single-file model, 4 GB Windows limit |
| LocalAI README | https://raw.githubusercontent.com/mudler/LocalAI/master/README.md | backends, per-key quotas, unsigned DMG |
| AnythingLLM README | https://raw.githubusercontent.com/Mintplex-Labs/anything-llm/master/README.md | built-in engine, telemetry, memories, MCP |

Complaint counts were taken from GitHub's issue search API (open issues only) on 2026-11-06:
Ollama "out of memory" 85, LM Studio bug tracker "memory" 313, GPT4All "download" 128,
AnythingLLM "api key" 41. Queries are written out in `recommendations.md` so the counts can be
reproduced. These are lower bounds: they miss Discord, Reddit and closed issues, and keyword
matching includes some unrelated reports.

## Findings in EZLM (file:line, severity)

1. **Update assets are not chosen by CPU architecture. Major.**
   `internal/update/update.go:43-51` accepts any release file whose name starts `EZLM-Setup` and
   ends `.exe` on every Windows machine, and any `EZLM*.dmg` on every Mac. EZLM supports Windows on
   Arm (`BETA.md`), and GPT4All and Jan both ship separate ARM builds, so an Arm laptop can be
   offered an x64 installer. Recommendation R02.
2. **API keys are stored in clear text in `config.json`. Major.**
   `internal/serve/config.go` (`Save`, around line 80) writes every `Keys[].Key` verbatim; the file
   is mode 0600, which is good, but a copied config or a backup exposes live keys. AnythingLLM's
   open #6661 asks for hashes, one-time display and last-used timestamps. Recommendation R04.
3. **The model is never unloaded while idle. Major for laptops.**
   `internal/serve/config.go:113-123` pins `-c` context and `-np 1`; `BETA.md` says the server
   starts at sign-in and stays up. LM Studio auto-evicts JIT-loaded models after inactivity and
   documents it. Recommendation R01.
4. **No Responses or Anthropic endpoint on the OpenAI port. Major for agent adoption.**
   LM Studio supports `/v1/responses` (Codex) and `/v1/messages`; EZLM's port is llama-server plus
   a key. Recommendations R10 and R11. Inferred from the absence of routes in `internal/serve`.
5. **The bench exists but its result is not kept on the connection. Minor.**
   `internal/bench/bench.go` runs speed, tool-call, instruction and recall checks and is imported by
   `internal/app/app.go:19`; a saved per-connection result with a re-run button is a small win.
   Recommendation R12.
6. **Capability data is thinner than the planner. Minor.**
   `fit.Plan` has GPU/RAM budgets and effective score, and `Config.Recipe` has a summary, but
   model capability (tools, vision, thinking, 64K) is not a first-class catalog field, so "smartest"
   can beat "works with my agent". Recommendation R18.
7. **Comparison, not a bug: EZLM's key handling is ahead of Ollama.** Ollama's local server
   accepts any key value; EZLM generates 238-bit keys and passes them to llama-server. The gap is
   lifecycle (hashing, last-used), not generation.

## What I could not verify

- **Sibling product sections.** All T14 branches (`agent/T14-a01` to `a16`) were still at the
  baseline commit when I checked, so there was nothing to pull from. My list is built from primary
  sources I fetched directly. It should be reconciled with a01 to a16 at merge.
- **Msty.** I did not fetch Msty docs in this slice, so no recommendation rests on it; a05 covers it.
- **UI-only behaviour.** Whether the Windows/Mac apps show a key once, or persist a skipped update,
  was inferred from `BETA.md` and the Go config, not read in the UI code.
- **llama.cpp's current Responses/Anthropic support.** I did not read the engine source, so R10/R11
  effort could be smaller if llama-server already implements them.
- **Counts.** GitHub search counts are a snapshot of open issues on one day and keyword-based; they
  are evidence of scale, not a precise complaint ranking. a15 is doing the careful mining.

## Open questions

1. Is the Windows installer architecture-specific or universal? If universal, R02 drops to S and
   becomes a naming/test fix rather than a new asset pipeline.
2. Does the beta intend to keep one deployed model at a time, or is JIT switching (R06) in scope?
   That answer changes three other recommendations (R07, R18, R29).
3. Should LAN mode require a key by policy, or only warn? T07 owns the security decision; R05 is the
   product copy that goes with it.

## Next steps

- Reconcile ranks against a01 to a08 when their sections land; correct anything they evidence
  better, and add Msty items if a05 finds something EZLM should copy.
- Turn R01, R02 and R04 into issues with tests: idle timeout in `internal/serve`, a table test for
  `assetFor` with arm64 releases, and a config round-trip test that asserts no plaintext key is
  written.
- Hand R06, R10 and R11 to the app/serve owners as a single "agent compatibility" milestone.

## Commits

- `T14: ranked recommendations for EZLM` (this commit): `agents/T14/recommendations.md`,
  `agents/T14/a10/REPORT.md`.
