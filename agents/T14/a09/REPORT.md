# T14 / a09 report: the comparison table

## What I did

Built `agents/T14/comparison.md`: the eight products from the brief plus EZLM as the ninth row, on
the nine dimensions the brief lists. Layout is one table per dimension (rows are products, the last
row is always EZLM) rather than one very wide table, so the EZLM comparison stays readable.

For the EZLM row I read the source in this worktree rather than relying on `BETA.md` marketing:
`internal/fit/fit.go`, `internal/scan/scan.go` and `internal/scan/gpu_discrete.go`,
`internal/serve/config.go`, `internal/keys/keys.go`, `internal/engine/engine.go`,
`internal/update/update.go`, `internal/integrate/types.go`, `internal/prereq/prereq.go`,
`internal/catalog/catalog.go`, plus `BETA.md` for the user-visible promises.

## Sources (all accessed 2026-11-06)

Primary docs only; no product installed or run.

- Ollama: `docs.ollama.com` markdown endpoints (`/faq.md`, `/gpu.md`, `/context-length.md`,
  `/api/openai-compatibility.md`, `/windows.md`). These are clean text and cite well.
- LM Studio: `lmstudio.ai/docs` markdown endpoints (`/app/basics/download-model.md`,
  `/app/system-requirements.md`, `/developer/core/authentication.md`, `/app/api/headless`).
- Jan, GPT4All, llamafile, LocalAI, AnythingLLM: repository READMEs on GitHub (raw).
- Msty: `docs.msty.app` onboarding page and sidebar index (GPU page and Local AI engine page not
  fetched).
- Counts: GitHub issue search API, `sort=reactions`.

## What I found that matters for the recommendations

1. **No competitor plans the fit.** Every product except EZLM leaves "which model and will it fit"
   to the user. LM Studio documents quant naming instead of choosing; Ollama offloads layers and
   tiers context by VRAM but never picks; GPT4All avoids the problem by capping at 3 to 13B.
   EZLM's `internal/fit` (quant x context x KV x placement, hard safety margin) is the differentiator.
2. **EZLM is the only one that requires a key.** LM Studio's tokens are off by default; Ollama
   ignores the key locally ("required but ignored"). Keep the `ezlm-` key requirement.
3. **EZLM is the only one that writes agent config.** OpenCode, Hermes and DSH get detected,
   installed and configured. Everyone else stops at "here is a base URL". Lead the beta with this.
4. **Model breadth is the biggest gap.** LM Studio pulls any GGUF from Hugging Face; EZLM ships a
   curated catalog. That is the most likely "it cannot do X" complaint from a beta user.
5. **AMD on Windows is a documented industry-wide weak spot.** Ollama's own docs say some RX 6000
   systems do not expose ROCm v7 and cap Windows Radeon at one loaded model because VRAM reporting is
   unreliable. Msty ships a troubleshooting page for it. EZLM's `cpuInstead` fallback is the right
   shape; the beta text should be honest that AMD on Windows may land on Vulkan or the CPU.

## Gaps and what I could not verify

- **LM Studio and Jan fit logic.** Not documented on any page I could find. Highest-value follow-up.
- **Complaint counts for seven of eight products.** The GitHub search API allows 10 unauthenticated
  requests a minute; I spent the budget and got Ollama (11,581 issues; top reacted issue 387
  reactions) before the rate limit. Section 8 of `comparison.md` records the exact query to re-run
  and the tracker URL for each product. a15 should own this.
- **Msty GPU support page, Msty Local AI engine, AnythingLLM API key details, Jan and GPT4All
  server auth.** Marked GAP in the table rather than guessed.
- **Windows on Arm coverage** is only documented by LM Studio (explicit support) and EZLM (arm64
  builds in `engine.go`). The rest are GAP, not "broken".
- I did not check EZLM's `windows/` and `macos/` UI for the first-run wording; `BETA.md` is the
  source for what the user is promised, and the UI is being rewritten tonight anyway.

## Open questions

1. Should EZLM's catalog become a thin wrapper over Hugging Face search, with the fit engine as the
   filter, instead of a curated list? It trades curation for breadth and moves the trust problem to
   the fit engine.
2. Ollama's VRAM-tiered context default (4k under 24 GiB) is cheap to copy as a fallback when a
   model has no measured speed. Does `internal/fit` already have an equivalent floor for unknown
   models? Not verified; `calib_test.go` only covers five measured machines.
3. AnythingLLM is a workspace that consumes a local model, so it is plausibly an EZLM integration
   target rather than a competitor. Worth a recommendation in a10.

## Not done

- No code changes (this task owns docs only), so `gofmt -l .` and `go build ./...` are unaffected;
  both were left untouched by this slice.
- Did not fill the seven missing complaint counts; recorded the query instead.
