# T14 a06 report: AnythingLLM

## What I did

Desk study of AnythingLLM v1.19.0 (Mintplex Labs), 2026-11-06. Read the official docs (built-in LLM, API access, desktop overview, system requirements, Windows install, update, MCP, all-features), the public repo (endpoint source, API key middleware, repo metadata), the v1.19.0 release notes, and the GitHub issue tracker via the search API. Compared against EZLM's `BETA.md`, `internal/fit`, `internal/serve`, `internal/keys` and `internal/update` in this worktree. No product installed, nothing executed.

Output: `agents/T14/products/a06.md` (nine brief sections plus five recommendations).

## Sources (all accessed 2026-11-06)

- https://docs.anythingllm.com/setup/llm-configuration/local/built-in — Lemonade engine, Vulkan/CUDA/Metal, XDNA2 NPU, model table, Ollama→Lemonade migration deletes models.
- https://docs.anythingllm.com/installation-desktop/overview — desktop vs Docker.
- https://docs.anythingllm.com/installation-desktop/system-requirements — 16 GB RAM, 8-12 GB+ VRAM.
- https://docs.anythingllm.com/installation-desktop/update — manual reinstall / brew, no auto-update.
- https://docs.anythingllm.com/features/api — developer API and keys.
- https://docs.anythingllm.com/mcp-compatibility/overview — MCP is inbound (agent tools), not an MCP server.
- https://github.com/Mintplex-Labs/anything-llm (repo metadata; 68,006 stars, 343 open issues, MIT).
- https://github.com/Mintplex-Labs/anything-llm/releases/tag/v1.19.0 (published 2026-10-26).
- `server/endpoints/api/openai/index.js`, `server/endpoints/api/index.js`, `server/utils/middleware/validApiKey.js`, `server/index.js` on `master`.
- Issues: [#2962](https://github.com/Mintplex-Labs/anything-llm/issues/2962) (69 comments), [#3194](https://github.com/Mintplex-Labs/anything-llm/issues/3194) (26), [#3087](https://github.com/Mintplex-Labs/anything-llm/issues/3087) (24 reactions, 21 comments), [#3753](https://github.com/Mintplex-Labs/anything-llm/issues/3753) (18 reactions), [#4017](https://github.com/Mintplex-Labs/anything-llm/issues/4017) (23 comments), [#6714](https://github.com/Mintplex-Labs/anything-llm/issues/6714), [#1077](https://github.com/Mintplex-Labs/anything-llm/issues/1077).

Counting method: GitHub search API `GET /search/issues?q=repo:Mintplex-Labs/anything-llm+is:issue&sort=comments|reactions&order=desc`, then per-issue `comments` and `reactions.total_count`. Unauthenticated search is rate-limited to roughly 10 requests/minute, so the sample is the first 15-20 per sort, not an exhaustive census. 1,753 issues matched `is:issue`; 4,155 matched the broader query, which suggests the two counts were taken minutes apart under different defaults and should be treated as approximate.

## What I could not verify

- Whether the desktop onboarding has an explicit pre-download memory-fit check or warning. The docs describe a model table and three recommendations by GPU/memory, but not a pass/fail fit test. The "no fit check" statement in the product file is inference from the docs' silence, marked as such.
- Whether the OpenAI-compatible endpoint gained `tools` support after #3753 closed. The source I read still destructures only `model`, `messages`, `temperature`, `stream`, which is strong but not conclusive (helpers may inject tools). A one-request test against a running instance would settle it; that was out of scope for a desk study.
- AnythingLLM Desktop's exact LAN behaviour. Docker publishes port 3001; desktop was not documented as binding beyond localhost in the pages I read.
- Exact Windows-on-Arm installer support (arm64 build vs x64 under emulation). The QNN provider and the issue reports imply arm64, but I did not find the release asset list.

## Open questions for the lead

- Is the "developer API" key surface (unscoped, all workspaces) something EZLM should copy for parity, or is one key per install (EZLM's current `ezlm-` key) the better beta story? I recommend keeping one key.
- Should T14's cross-cutting server section (a12) treat AnythingLLM's workspace-as-model mapping as the canonical anti-pattern? It is the clearest example in the eight products of an OpenAI-compatible endpoint that is not actually model-compatible.

## Commit

See `git log --oneline -1` on `agent/T14-a06`; final commit message starts `T14: AnythingLLM teardown`.
