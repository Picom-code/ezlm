# T14 a02 report: Ollama

Agent: a02. Product: Ollama. Worktree `~/ezlm-agents/work/subs/T14/a02`, branch
`agent/T14-a02`. Desk study, 2026-11-06; no competitor product installed or run.

Deliverable: `agents/T14/products/a02.md` (product section in the brief's shape, plus five
prioritised recommendations and one stretch item).

## What I read

Primary sources (all accessed 2026-11-06):

- https://docs.ollama.com/quickstart — first-run flow, desktop apps, `ollama launch`.
- https://docs.ollama.com/cli — `ollama run`, `ollama launch` and supported integrations.
- https://docs.ollama.com/faq — update mechanism, default context, host binding, CORS,
  parallel/model-loading limits, `keep_alive`.
- https://docs.ollama.com/context-length — VRAM-tiered default context, `ollama ps`.
- https://docs.ollama.com/gpu — CUDA/ROCm/Metal/Vulkan support, card lists, Vulkan VRAM
  reporting caveat, `HSA_OVERRIDE_GFX_VERSION`.
- https://docs.ollama.com/windows — install layout, ROCm-on-Windows limits, RDNA2/Vulkan
  fallback.
- https://docs.ollama.com/troubleshooting — log locations, LLM library selection.
- https://docs.ollama.com/api/authentication — local server needs no auth; API keys are
  cloud-only.
- https://docs.ollama.com/api/openai-compatibility — `/v1` surface and the ignored key.
- https://docs.ollama.com/integrations/claude-code and https://ollama.com/integrations —
  agent/IDE integration surface.
- https://github.com/ollama/ollama/releases and the GitHub releases API — current version
  v0.40.15 (published 2026-11-03), asset list (including `ollama-windows-arm64.zip`).
- GitHub issue search API (`repo:ollama/ollama is:issue`, sorted by comments and reactions)
  and individual issue pages for the complaint table.

EZLM comparison code read (worktree): `internal/fit/fit.go` (ctx ladder, budgets, margins),
`internal/serve/config.go` (explicit flags, API key, loopback/LAN URLs),
`internal/keys/keys.go` (key format), `internal/update/update.go` (check-only updates),
`internal/scan/apps.go` (detects a running Ollama as a competing process).

## How the complaint counts were made

GitHub REST search for `repo:ollama/ollama is:issue`, sorted by `comments` and by
`reactions`, plus the issues API for individual totals. Numbers in the table are the API's
`reactions.total_count` and `comments` at fetch time, all-time, not de-duplicated across
threads. That is a proxy for complaint volume, not a survey: reaction counts reward old
issues, and a closed issue may have been fixed. Reddit was not counted because
`reddit.com/r/ollama/top.json` returned an HTML block page from this network; the report
says so and lists it as an open question.

## Key findings

1. **Context default is the big fit gap.** Ollama sizes context from total VRAM
   (<24 GiB → 4k, 24–48 → 32k, ≥48 → 256k) while its own docs say coding/agent tasks need
   64k. On every consumer GPU under 24 GB the default is 4k. EZLM's ladder starts at 64k and
   falls to 8k only when memory forces it (`internal/fit/fit.go:89`), so EZLM's planner is
   better; EZLM should show the chosen context and the memory cost.
2. **No local auth.** The local server is keyless by design; LAN exposure is a documented
   env var (`OLLAMA_HOST`) and still keyless. EZLM already generates an `ezlm-` key and
   passes `--api-key` (`internal/keys/keys.go:12`, `internal/serve/config.go:121`).
3. **`ollama launch` is the onboarding idea worth stealing.** One command wires Claude Code,
   Codex, OpenCode or VS Code to the local server, with 20+ documented integrations. EZLM
   hands over a URL and a key.
4. **Platform gaps are counted and open:** Intel Arc 139 reactions (open since 2023),
   Snapdragon X Elite GPU/NPU 81 reactions (open since 2024). Windows Arm has a native CLI
   asset but no GPU acceleration documented.
5. **Vulkan is a double-edged fallback.** It is the default Windows/Linux backend now and
   the only Intel path, but available VRAM may be approximate, so scheduling is best effort.
   Fit logic must keep headroom on those machines.

## What I could not verify

- Whether the desktop app recommends a model from detected memory or only presents the
  library. The quickstart shows a user choice; not installed, so not observed.
- The exact default `keep_alive` (5 minutes is widely reported) is not stated on the docs
  pages fetched today; the FAQ documents the parameter, not the default.
- Whether local-server auth is still absent in v0.40.15 beyond the authentication page's
  statement that the local API "does not require authentication"; no binary inspected.
- Reddit/forum complaint counts (blocked JSON endpoint).

## Open questions for the merge

- a09/a10/a15 should treat the complaint table as GitHub-only and say so; a browser pass
  could add r/ollama numbers.
- If EZLM adopts an `ollama launch`-style flow, decide whether EZLM writes the agent's config
  file or only prints env vars; the former is more magic, the latter easier to undo.

## Sanity checks

Docs-only change; `gofmt -l .` empty and `go build ./...` clean in the worktree before the
final commit (no Go files touched).
