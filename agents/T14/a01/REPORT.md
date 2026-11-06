# T14 / a01 — LM Studio: report

**Agent:** a01. **Product:** LM Studio. **Branch:** `agent/T14-a01`. **Output:**
`agents/T14/products/a01.md`. Docs only, no code changes, no product installed.

## What I read

Primary (all accessed 2026-11-06):

- <https://lmstudio.ai/docs/app/system-requirements> — supported CPU/GPU matrix.
- <https://lmstudio.ai/docs/app/basics> — first-run flow.
- <https://lmstudio.ai/docs/app/basics/download-model> — Discover tab, quant choice.
- <https://lmstudio.ai/docs/developer/core/server> — server start, API surface.
- <https://lmstudio.ai/docs/developer/core/authentication> — API tokens, 0.4.0+.
- <https://lmstudio.ai/docs/developer/core/server/serve-on-network> — LAN bind.
- <https://lmstudio.ai/llms.txt> — full docs dump; this is where the JIT-loading, idle TTL,
  auto-evict, LM Runtimes, per-model-defaults and speculative-decoding text actually lives (the
  per-page fetches truncate before those sections).
- <https://lmstudio.ai/changelog/lmstudio> and <https://lmstudio.ai/changelog> — release cadence,
  runtime bumps, WoA/ROCm fixes, updater bugs.
- GitHub search API over `lmstudio-ai/lmstudio-bug-tracker`, sorted by `reactions` and by keyword;
  reaction totals and per-keyword `total_count` recorded in the product file with the method.

Secondary: none usable. Reddit's JSON endpoint refused the request (non-JSON body), Discord is not
fetchable, so every count is GitHub-derived.

EZLM code read for comparison (not edited): `internal/fit/fit.go`, `internal/keys/keys.go`,
`internal/serve/config.go`, `internal/update/update.go`, `internal/scan/`.

## Findings that matter to the parent

1. **LM Studio does not do pick-time fitting.** The user chooses model, quant, context and a GPU
   offload fraction; the docs' only advice is "choose 4-bit or higher if your machine is capable".
   Its memory intelligence is *runtime* intelligence (JIT load, idle TTL 60 min, auto-evict) and it
   was built for coding-agent clients by name. EZLM's `internal/fit` planner is strictly ahead on
   the pick; EZLM has nothing on the runtime side. This is the single biggest borrowable idea.
2. **The server surface is the competitive moat.** OpenAI `/v1` plus `/v1/responses`, embeddings,
   **Anthropic `/v1/messages`**, Jev `/v1/systemone`, a native REST API and two SDKs, all on one
   port with named per-permission tokens (0.4.0+) and an explicit "Serve on Local Network" switch
   that warns you to turn auth on first. No TLS, ever (#117, 26 reactions, open since 2024).
3. **Platform support is where the users are angry.** Top-reacted issues are Intel Mac (#9, 508
   reactions, 444 comments), Intel GPU backends (#171 SYCL 59, #581 NPU 45, #2025 OpenVINO 29) and
   ROCm currency (#1394, 30). Keyword counts: vulkan 281, amd 231, intel 114, rocm 109 — all above
   "out of memory" 130. Windows on Arm needed a CUDA-detection fix as late as 0.4.26 build 5.
4. **Engines ship separately from the app.** LM Runtimes are hot-swappable llama.cpp builds since
   0.3.0. Their changelog shows both why it is good (ship a ROCm fix in a day) and why it is hard
   (runtime/model compatibility breaks: #1741, #2037, "fixed runtime discovery reliability").
5. **Install hygiene costs goodwill.** `lms` appends a PATH block to `.bashrc`/`.zshrc` (#656, 56
   reactions, still open). Cheap lesson for EZLM's installer.

## Recommendations handed up (detail in the product file)

| # | recommendation | effort | priority |
|---|---|---|---|
| R1 | Idle TTL + auto-evict for the served model | M | P1 |
| R2 | Show the fit verdict at pick time and refuse over-commit | S | P1 |
| R3 | Per-key scopes and a show-once token flow | S-M | P2 |
| R4 | Hot-swappable engine builds with rollback | L | P2 |
| R5 | Anthropic-compatible `/v1/messages` | M | P3 |

## Not verified / open questions

- Does Discover show a fit indicator today? Docs do not say; a "fits in memory" issue search returns
  one hit. Needs a screenshot or a run to settle. Do not assert it either way in the comparison table.
- Default context length and default GPU-offload fraction on a fresh install: undocumented.
- TTL and auto-evict behaviour under concurrent requests: docs only.
- Reddit/Discord sentiment for LM Studio is missing from this slice; the complaints section is
  GitHub-only and should be topped up by a15 if it has a working Reddit path.
- LM Studio now ships a second app (Bionic 1.2.0, 2026-11-05) that reuses LM Studio for inference
  and adds "LM Link". I recorded it but did not study it; it may matter to a09's comparison row.

## State

- Files: `agents/T14/products/a01.md`, `agents/T14/a01/REPORT.md`.
- No code touched. Sanity checks run after the commit: `gofmt -l .` empty, `go build ./...` OK.
