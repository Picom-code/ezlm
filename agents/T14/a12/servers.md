# T14 a12: the OpenAI-compatible server and key handling across the products

Scope: URLs, ports, key formats, auth defaults, LAN exposure, CORS, and the
operational sharp edges, compared with EZLM. Desk study only; no product was
installed or run.

All URLs accessed **2026-11-06**. Claims are marked **[doc]** when they come
from a vendor page, **[code]** when read from the vendor repository, and
**[unverified]** when I could not confirm them from a primary source today.

## Short version

| Product | Default base URL | Port | Auth out of the box | Key format | LAN |
|---|---|---|---|---|---|
| EZLM | `http://127.0.0.1:8080/v1` | 8080, first free at or above | **required** | `ezlm-` + 40 base62 chars | toggle, `0.0.0.0`, key still required |
| LM Studio | `http://localhost:1234/v1` | 1234 | none (tokens optional, 0.4.0+) | opaque token, named, permissions | toggle, warns to enable auth |
| Ollama | `http://localhost:11434/v1` | 11434 | none locally | none locally; cloud key `OLLAMA_API_KEY` | `OLLAMA_HOST=0.0.0.0:11434`, still no auth |
| Jan | `http://127.0.0.1:1337/v1` | 1337 | none by default | any string you type, empty disables | host setting, warns |
| GPT4All | `http://localhost:4891/v1` | 4891 | none | none | **none**, localhost only |
| Msty | not published in the page I read | shown in app | none documented | none documented | toggle "Make Service Available on Network" |
| AnythingLLM | `http://localhost:3001` + `/v1/openai/...` **[unverified port]** | 3001 | **required** for the developer API | opaque secret, Bearer, 403 on bad key | desktop is local; Docker is exposed |
| LocalAI | `http://localhost:8080/v1` | 8080 | **none** unless configured | shared `sk-` style keys or per-user `lai-...` | Docker publishes 0.0.0.0 by default |
| llamafile | `http://127.0.0.1:8080/v1` **[unverified]** | 8080 | none unless `--api-key` | arbitrary string | `--host 0.0.0.0` |

## Per product

### LM Studio

- Server is opt-in: Developer tab, "Start server", or `lms server start`.
  Base URL examples use `http://localhost:1234/v1`. OpenAI surface:
  `/v1/models`, `/v1/responses`, `/v1/chat/completions`, `/v1/completions`,
  `/v1/embeddings`, plus a separate REST API at `/api/v1` and an
  Anthropic-compatible surface. `/v1/responses` is what makes Codex work.
  Source: https://lmstudio.ai/docs/developer/openai-compat,
  https://lmstudio.ai/docs/developer/core/server (accessed 2026-11-06).
- **Auth is off by default.** "By default, LM Studio does not require
  authentication for API requests." Since 0.4.0 you can toggle "Require
  Authentication" and create named API tokens with permissions, shown once,
  editable and deletable. Source:
  https://lmstudio.ai/docs/developer/core/authentication (accessed 2026-11-06).
- LAN is a switch. "Any bind other than `127.0.0.1` exposes the server beyond
  localhost; we recommend enabling authentication. Only do this if you know
  what you're doing." CLI equivalent `lms server start --bind 0.0.0.0`.
  Source: https://lmstudio.ai/docs/developer/core/server/serve-on-network
  (accessed 2026-11-06).
- Server settings also expose CORS (a switch, default not stated), JIT model
  loading, and MCP access. Source:
  https://lmstudio.ai/docs/developer/core/server/settings (accessed 2026-11-06).
- Takeaway: mature token model (named tokens, permissions, delete) but auth
  is still opt-in and the docs put the burden on the user when binding LAN.

### Ollama

- Binds `127.0.0.1:11434` by default; `OLLAMA_HOST` changes the bind. The
  local API "does not require authentication". API keys exist only for
  `ollama.com` cloud inference (`OLLAMA_API_KEY`, `Authorization: Bearer`).
  Sources: https://docs.ollama.com/faq (accessed 2026-11-06),
  https://docs.ollama.com/api/authentication (accessed 2026-11-06).
- OpenAI compatibility lives at `http://localhost:11434/v1`. The docs state
  plainly that the local client "requires an API key value, but Ollama
  ignores it", so any non-empty string works. Source:
  https://docs.ollama.com/api/openai-compatibility (accessed 2026-11-06).
- CORS: requests from `127.0.0.1` and `0.0.0.0` origins are allowed by
  default; `OLLAMA_ORIGINS` adds more, including browser extension schemes.
  Source: https://docs.ollama.com/faq (accessed 2026-11-06).
- Operational defaults that surprise people: 4096-token context,
  `OLLAMA_NUM_PARALLEL=1`, up to 3 loaded models, 5-minute keep-alive.
  Sources: https://docs.ollama.com/faq, https://docs.ollama.com/context-length
  (accessed 2026-11-06).
- LAN exposure is the sharp edge: setting `OLLAMA_HOST=0.0.0.0:11434` opens
  an unauthenticated inference and model-management API to the network. The
  docs treat this as a normal configuration step, not a security event.

### Jan

- Default `http://127.0.0.1:1337/v1`. Server host is a setting:
  `127.0.0.1` (default) or `0.0.0.0`, with the docs saying "Use this with
  caution" for the latter. Port and API prefix (`/v1`) are configurable.
  Source: https://docs.jan.ai/desktop/api-server (accessed 2026-11-06).
- API key: "Set any string... Leave the field empty to disable
  authentication (not recommended when binding to `0.0.0.0`)." Sent as
  `Authorization: Bearer`. There is also a Trusted Hosts allowlist, a request
  timeout, and CORS enabled by default. Source: same page.
- Endpoints: `/v1/models`, `/v1/chat/completions`, and an
  Anthropic-compatible `/v1/messages`; `/v1/responses` is "coming soon".
  Source: https://docs.jan.ai/desktop/api-preference (accessed 2026-11-06).
- The server needs a local model loaded or selected before it starts, so the
  URL is not usable until the app has a model. Source: api-server page.
- Takeaway: good defaults and the only product here that documents a Host
  allowlist, but the key is free text and easy to leave empty.

### GPT4All

- Off by default. Enable in Settings > Application > Advanced > "Enable Local
  API Server"; port 4891 default. Base URL `http://localhost:4891/v1`.
  Source: https://docs.gpt4all.io/gpt4all_api_server/home.html (accessed
  2026-11-06).
- "The server only accepts HTTP connections (not HTTPS) and only listens on
  localhost (127.0.0.1) (e.g. not to the IPv6 localhost address `::1`)."
  There is no LAN option and no authentication at all. Source: same page.
- Endpoints: `/v1/models`, `/v1/models/<name>`, `/v1/completions`,
  `/v1/chat/completions`. Source: same page.
- Takeaway: simplest possible, and the weakest story for a multi-device or
  public-beta user. Also a real IPv6 gap.

### Msty

- "Msty's Local AI service is designed to be accessible only from the
  computer where it is installed. This is a security feature." A toggle under
  Settings > Local AI > Service Configs > "Make Service Available on
  Network" rebinds it and Msty shows the LAN IP and port under Local AI
  Service > Service Endpoint > Network IP. The docs never mention an API key
  for the Local AI service. Source:
  https://docs.msty.app/how-to-guides/make-local-ai-service-available-on-the-network
  (accessed 2026-11-06).
- On a second device, Msty consumes the service through its own "Msty Remote"
  provider rather than a generic OpenAI client, which suggests the endpoint is
  Msty-shaped, not a plain `/v1` URL. The public page does not state the
  port, the base path, or whether the endpoint is OpenAI-compatible.
  **[unverified]**
- Msty Studio (the paid successor) has a separate docs site
  (https://docs.msty.ai/studio/getting-started, accessed 2026-11-06) whose
  nav includes a "Vibe CLI Proxy"; I did not find a public API-server page.
- Takeaway: the only product here that treats network exposure as a
  first-class, explicit action with a service restart, and the only one with
  no documented key story. Weakest fit for "point your coding agent at it".

### AnythingLLM

- Developer API and OpenAI-compatible endpoints are served by the app's own
  Node server, not by a model runtime. Routes read from the repo:
  `GET /v1/openai/models`, `POST /v1/openai/chat/completions`,
  `POST /v1/openai/embeddings`, `POST /v1/openai/images/generations`,
  `GET /v1/openai/vector_stores`. Source:
  https://github.com/Mintplex-Labs/anything-llm/blob/master/server/endpoints/api/openai/index.js
  (accessed 2026-11-06).
- Auth is required and enforced by `validApiKey`: it reads
  `Authorization: Bearer <key>` and returns **403** `{"error":"No valid api
  key found."}` when the header is missing or the secret is unknown. Source:
  https://github.com/Mintplex-Labs/anything-llm/blob/master/server/utils/middleware/validApiKey.js
  (accessed 2026-11-06).
- The "model" is a **workspace slug**, not an LLM name: the handler does
  `Workspace.get({ slug: String(model) })`, returns 401 for an unknown
  workspace, and `/v1/openai/models` lists workspaces. A client that sends
  `gpt-4o` fails. Source: openai/index.js above.
- Key lifecycle is managed in the app (Settings > API Keys) and the secret is
  an opaque value, not a vendor-prefixed token. **[unverified format]**
- Desktop binds the local server; Docker publishes it. The desktop UI/API
  port is 3001 by convention but I did not re-verify it today.
  **[unverified]**
- Takeaway: a good example of requiring a key for a local HTTP API, and a bad
  example of overloading "model" to mean "workspace", which breaks generic
  OpenAI clients and coding agents.

### LocalAI

- OpenAI-compatible at `http://localhost:8080` (`/v1/chat/completions`),
  plus Anthropic Messages, Open Responses, and many non-text endpoints.
  Source: https://localai.io/docs/basics/getting_started/ (accessed
  2026-11-06).
- **No auth unless configured.** Two modes: legacy shared keys via
  `LOCALAI_API_KEY=key1,key2` (full admin access, no roles), or the user
  system via `LOCALAI_AUTH=true` with roles, OAuth/OIDC, expiring per-user
  keys. Accepted headers: `Authorization: Bearer`, `x-api-key`,
  `xi-api-key`, or a `token` cookie. Per-user keys look like `lai-...` and
  are shown once. Source:
  https://localai.io/docs/features/authentication/ (accessed 2026-11-06).
- With auth configured, the HTTP surface is "private by default": only
  health, auth bootstrap, discovery, and static assets are anonymous.
  Source: same page.
- The quickstart's own security note: "If you are exposing LocalAI
  remotely, make sure you protect the API endpoints adequately." The Docker
  command it gives (`docker run -p 8080:8080 ...`) publishes on all
  interfaces with no key, which is the dangerous default. Source:
  https://localai.io/docs/basics/getting_started/ (accessed 2026-11-06).
- Takeaway: the richest auth model of the eight, and the worst default
  posture in Docker. EZLM should copy the per-key expiry/pause idea, not the
  default.

### llamafile

- Single-file llama.cpp server. The project docs cover running and prebuilt
  files (https://docs.mozilla.ai/llamafile/using-llamafile/running_llamafile,
  accessed 2026-11-06) but the page I fetched did not expose the flag table
  in the truncated text.
- Defaults are inherited from `llama-server`: `127.0.0.1:8080`, an optional
  `--api-key`, `--host 0.0.0.0` to expose on the LAN, OpenAI-compatible
  `/v1`. **[unverified today]**
- Takeaway: same runtime family as EZLM, so EZLM's security posture is
  essentially "llamafile plus a generated key and a LAN toggle". That is a
  meaningful improvement, and worth saying in the beta notes.

## EZLM today (read from the worktree, for comparison)

- Keys: `internal/keys/keys.go` generates `"ezlm-"` plus 40 base62
  characters (about 238 bits) from `crypto/rand`. Grep-friendly prefix, no
  expiry, no per-key permissions.
- Config: `internal/serve/config.go` stores `Keys []APIKey` (id, name, key,
  created) in `~/.ezlm/config.json`, written `0600`, with `Key` mirroring
  `Keys[0]` for older configs. Multiple named keys are supported.
- Launch: `Args()` always passes `--host`, `--port`, `--api-key` (all keys
  comma-separated), `--alias`, `--metrics`, `-np 1`. Host is `0.0.0.0` only
  when `LAN` is true, otherwise `127.0.0.1`.
- URLs: `LocalURL()` is `http://127.0.0.1:<port>/v1`; `LANURL()` is the
  first LAN IP plus port, empty when LAN is off. Both are surfaced in the
  app state (`internal/app/app.go`).
- Port: starts at 8080, then `serve.FreePort(max(cfg.Port, 8080))` picks the
  next free one (`internal/app/app.go:797`).
- Rotation: `app.go` can replace a key or delete one by id and persist; a
  running `llama-server` only learns the new list on restart.
- Auth is on from the first run, before the user asks for it. None of the
  eight products does that except AnythingLLM (which is a workspace app, not
  a model server).

## Cross-cutting findings

1. **Auth-on-by-default is EZLM's clearest differentiator.** LM Studio,
   Ollama, Jan, GPT4All, Msty, LocalAI, and llamafile all ship an
   unauthenticated localhost API by default. Ollama and LocalAI can be
   exposed on `0.0.0.0` with no key by following their own docs. EZLM's
   generated key costs one extra step for the user and removes a whole class
   of beta-support and security incidents. Keep it, and say it in the
   onboarding copy.
2. **The LAN toggle is where every product's docs get nervous.** LM Studio
   says "we recommend enabling authentication"; Jan says "use with
   caution"; LocalAI says "make sure you protect the API endpoints". EZLM
   already requires a key when LAN is on, which is better than all of them.
   The remaining gap is communication: show the exact LAN URL, a warning
   that anyone on the network with the key can use the GPU, and a firewall
   hint.
3. **Ports are a collision surface.** 8080 is shared by LocalAI and
   llama.cpp-family servers and is a very common dev port; 1234, 11434,
   1337, 4891, 3001 are all claimed. EZLM's free-port fallback is better
   than fixed ports, but only if every place that shows the URL uses the
   actual chosen port (it does, via `LocalURL()`), and only if the UI says
   the port changed.
4. **"Model" means different things.** LM Studio and Jan want a real model
   id; AnythingLLM wants a workspace slug; Ollama wants a tag like
   `llama3.2`; GPT4All wants a display name. Coding agents hardcode
   `gpt-4o`, `claude-sonnet-4`, etc., so every product has a
   model-name-mapping problem. EZLM's `--alias` plus writing the exact id
   into each client config is the right approach; document that the id is
   the only accepted value.
5. **A proxy is the missing layer.** Everything EZLM wants next (per-key
   attribution, `/v1/responses` for Codex, `/v1/messages` for Anthropic
   clients, CORS policy, Host-header checks, rate limits) lives above
   `llama-server`, not inside it. LM Studio and Jan already ship that layer.
   EZLM currently hands clients straight to `llama-server`.
6. **Key rotation needs a restart story.** LM Studio can add and delete
   tokens live; LocalAI can pause and expire keys; EZLM rewrites
   `--api-key` and needs a server restart to apply it. That is acceptable
   for a single-user app, but the UI should say so, and the old key should
   keep working until the restart actually happens.
7. **CORS defaults are inconsistent.** Jan enables CORS by default; LM
   Studio makes it a switch; Ollama allows localhost origins. A browser
   page on `localhost:3000` calling EZLM on `localhost:8080` will hit
   whatever `llama-server` does by default, which is not documented in
   EZLM's UI. Worth an explicit setting and a test.

## Recommendations for EZLM

Priority is P1 (before public beta), P2 (next), P3 (nice to have).
Effort is S (under a day), M (a few days), L (a week or more).

**R1. Keep the generated key, and make it the first thing on screen. P1, S.**
Evidence: seven of eight products are unauthenticated by default; the two
with a key (AnythingLLM, LocalAI-with-config) require the user to set it up.
EZLM already generates `ezlm-...` on first run. Add a single "Connect" card
with the base URL, the key (copy and reveal), a `curl /v1/models` that
works, and a one-line note that the key is required. Touches
`internal/app` UI plus the existing `LocalURL`/`Key` state. No server change.

**R2. Make the LAN switch explain itself. P1, S.**
Evidence: LM Studio's and Jan's own docs warn when binding non-localhost;
Ollama and LocalAI Docker let people expose an unauthenticated API. EZLM
already forces a key, so this is copy plus one URL. On enable, show the
exact `LANURL()`, the key to use, "anyone on this network with this key can
use this machine's GPU", and a firewall hint. On disable, say the URL
stopped working. Touches `internal/app/app.go` SetLAN and the UI.

**R3. Add a thin gateway in front of `llama-server`. P2, M.**
Evidence: LM Studio implements `/v1/responses` (needed by Codex) and
Anthropic `/v1/messages`; Jan ships `/v1/messages`; LocalAI ships both.
EZLM exposes only llama-server's surface. A small local HTTP proxy would
also give per-key attribution, request logs, CORS and Host-header policy,
and graceful key rotation, none of which `llama-server` can do. Touches
`internal/serve` and `internal/app` wiring. This is the largest change here
and should be coordinated with T07.

**R4. Give keys a lifecycle, not just a string. P2, S to M.**
Evidence: LM Studio tokens are named with permissions and deletable;
LocalAI per-user keys can be paused, expired (`30d`/`90d`/`1y`), and are
shown once. EZLM has named keys already. Add "created", "last used"
(needs R3), "copy", "revoke", and a clear "restart to apply" state when the
list changes. Do not add permissions yet; single-user app.

**R5. Pick the port, then tell the truth about it everywhere. P2, S.**
Evidence: 8080 is contested (LocalAI, llama.cpp, dev servers). EZLM already
falls back to the next free port. Make the fallback visible ("8080 was in
use, using 8081"), persist it, and make sure every snippet, client config,
and QR/deep link uses the actual port. Add a test that a busy 8080 produces
a coherent URL and key pair.

**R6. Document and test the model id contract. P3, S.**
Evidence: AnythingLLM returns 401 when the "model" is not a workspace slug;
Jan returns 404 for an unknown model id; every client needs the exact id.
EZLM sets `--alias` to the model id and writes it into each integration.
Add one doc line: "Use exactly this model id", and a `/v1/models` smoke
test in the beta self-check.

## Open questions

- Does `llama-server`'s `--api-key` compare keys in constant time, and does
  it log them? Not verified from the binary; worth a code read in llama.cpp.
- Does `llama-server` send permissive CORS headers by default? EZLM's UI
  does not expose CORS, and I did not test it.
- AnythingLLM's desktop port and exact key format were not re-verified
  today; the endpoint and 403 behaviour are from the repo.
- Msty's Local AI endpoint path, port, and whether it is OpenAI-compatible
  are not documented on the public page I read.
- llamafile's flag defaults are inherited from llama.cpp and were not
  re-verified on 2026-11-06.
