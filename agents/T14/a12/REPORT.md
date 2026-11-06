# T14 a12 report: OpenAI-compatible server and key handling

## What this slice covers

URLs, ports, key formats, auth defaults, LAN exposure, CORS, and the
operational sharp edges of the eight products, compared with EZLM's server
(`internal/serve`, `internal/keys`, `internal/app`). Output is
`agents/T14/a12/servers.md`.

## Method

Desk study, no product installed or run. Primary sources only: vendor docs
pages, and the AnythingLLM repository where the docs page was truncated by
navigation. All pages fetched **2026-11-06**. EZLM behaviour was read from
the `agent/T14-a12` worktree at commit `main` + no changes.

## Sources (all accessed 2026-11-06)

- LM Studio server: https://lmstudio.ai/docs/developer/core/server
- LM Studio OpenAI compatibility: https://lmstudio.ai/docs/developer/openai-compat
- LM Studio auth: https://lmstudio.ai/docs/developer/core/authentication
- LM Studio LAN: https://lmstudio.ai/docs/developer/core/server/serve-on-network
- LM Studio settings: https://lmstudio.ai/docs/developer/core/server/settings
- Ollama FAQ (bind, CORS, concurrency): https://docs.ollama.com/faq
- Ollama auth: https://docs.ollama.com/api/authentication
- Ollama OpenAI compatibility: https://docs.ollama.com/api/openai-compatibility
- Jan local API server: https://docs.jan.ai/desktop/api-server
- Jan API reference: https://docs.jan.ai/desktop/api-preference
- GPT4All API server: https://docs.gpt4all.io/gpt4all_api_server/home.html
- Msty network access: https://docs.msty.app/how-to-guides/make-local-ai-service-available-on-the-network
- Msty docs source: https://github.com/cloudstack-llc/msty-docs (raw page above)
- AnythingLLM OpenAI endpoints: https://github.com/Mintplex-Labs/anything-llm/blob/master/server/endpoints/api/openai/index.js
- AnythingLLM key check: https://github.com/Mintplex-Labs/anything-llm/blob/master/server/utils/middleware/validApiKey.js
- LocalAI quickstart: https://localai.io/docs/basics/getting_started/
- LocalAI auth: https://localai.io/docs/features/authentication/
- llamafile docs: https://docs.mozilla.ai/llamafile/using-llamafile/running_llamafile
- llamafile README: https://github.com/mozilla-ai/llamafile

## EZLM files read

- `internal/keys/keys.go`: `ezlm-` + 40 base62 chars, `crypto/rand`.
- `internal/serve/config.go`: named keys, `0600` config, `--host`,
  `--port`, `--api-key`, `--metrics`, `-np 1`, `LocalURL`, `LANURL`,
  `Host()`, `FreePort`.
- `internal/app/app.go`: `SetLAN`, key add/replace/delete, port fallback at
  or above 8080, `LocalURL`/`LANURL` in app state.
- `internal/integrate`: clients are given `BaseURL`, `Key`, `ModelID`, so
  the URL and key are already a first-class concept in EZLM.

## Findings

1. **Blocker for the beta narrative (positive): EZLM is the only product of
   the eight that requires a key from first run.** LM Studio, Ollama, Jan,
   GPT4All, Msty, LocalAI, and llamafile all default to an unauthenticated
   localhost API. Ollama and LocalAI can be exposed on `0.0.0.0` with no
   key by following their own docs. EZLM should say this out loud.
2. **Major: LAN exposure is under-explained everywhere, including EZLM.**
   LM Studio's and Jan's docs warn; Ollama's FAQ treats
   `OLLAMA_HOST=0.0.0.0:11434` as routine. EZLM forces a key on LAN, which
   is better, but the UI does not explain the risk or show the exact URL
   prominently. Recommendation R2.
3. **Major: EZLM has no layer above `llama-server`.** No per-key
   attribution, no `/v1/responses` (Codex), no `/v1/messages` (Anthropic
   clients), no CORS or Host policy. LM Studio and Jan ship that layer.
   Recommendation R3.
4. **Minor: key rotation needs a restart and the UI does not say so.**
   `--api-key` is fixed at process start. LM Studio and LocalAI can change
   keys live. Recommendation R4.
5. **Minor: 8080 is a collision-prone default.** EZLM's free-port fallback
   is good; the fallback must be visible and used in every snippet.
   Recommendation R5.
6. **Minor: "model" naming.** AnythingLLM uses workspace slugs as model
   ids, Jan and LM Studio use real model ids, Ollama uses tags. Coding
   agents send arbitrary names. EZLM's `--alias` plus exact-id config is
   correct; document it. Recommendation R6.

## What I could not verify

- AnythingLLM's desktop port (3001) and the exact API key format. The
  endpoints and the 403 behaviour are from the repository.
- Msty's Local AI endpoint path, port, and whether it is OpenAI-compatible.
  The public page only covers the network toggle and the Msty Remote
  provider.
- llamafile's default host/port and `--api-key` semantics; inherited from
  `llama-server` and not re-verified today.
- Whether `llama-server` compares API keys in constant time or logs them.
- Whether `llama-server` sends permissive CORS headers by default.

## Open questions

- Should EZLM's gateway (R3) be a separate package or part of
  `internal/serve`? It overlaps with T07's Host-header work, so coordinate.
- Do we want per-key expiry, or is "revoke and restart" enough for a
  single-user app?
- Should the default port move off 8080 to reduce collisions with LocalAI
  and dev servers, or is a visible fallback enough?

## Next steps

1. Turn R1 and R2 into UI copy plus a screenshot-ready Connect card.
2. Spike R3 (a ~200-line `net/http` proxy) and measure whether it breaks
   streaming, tool calls, or the app's live metrics.
3. Add a `/v1/models` and auth smoke test to the beta self-check.
