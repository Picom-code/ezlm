# T14 a05 report: Msty

Agent: a05. Product: Msty (Msty App v1.x and Msty Studio). Branch `agent/T14-a05`.
Desk study only; no competitor product was installed or run.

## What I read

Primary sources, all accessed 2026-11-06:

- Msty App v1.x docs: onboarding, GPU support matrix, model selector, offline models, existing
  Ollama models, latest Local AI service, LAN access, API-key finding, changelog, and the four
  troubleshooting pages (stuck update, Linux, AMD ROCm on Windows, misc)
  (https://docs.msty.app/, https://docs.msty.app/getting-started/onboarding,
  https://docs.msty.app/getting-started/gpus-support,
  https://docs.msty.app/how-to-guides/make-local-ai-service-available-on-the-network,
  https://docs.msty.app/how-to-guides/get-the-latest-version-of-local-ai-service,
  https://docs.msty.app/troubleshooting/stuck-at-app-update,
  https://docs.msty.app/troubleshooting/amd-rocm-windows-issues).
- Msty Studio docs: welcome, quick start, local models, model matchmaker
  (https://docs.msty.studio/, https://docs.msty.ai/studio/getting-started/quick-start,
  https://docs.msty.ai/studio/managing-models/local-models,
  https://docs.msty.ai/studio/managing-models/model-matchmaker).
- Pricing and positioning: https://msty.ai/pricing, https://msty.ai/.
- Public issue tracker `cloudstack-llc/msty-docs`, read through the GitHub API: repo metadata, the
  50 most recent issues (state, reactions, comment counts, dates) and issue bodies/comments for
  #15, #19, #24, #13, #45, #54, #59.
- EZLM code for comparison: `internal/keys/keys.go`, `internal/serve/config.go`,
  `internal/fit/fit.go`, `internal/serve/procs.go`, `internal/update/`.

## What I found

The product section carries the findings. Summary of the ones that change EZLM's picture:

1. Msty has no memory-fit planner. Sizing is the user's job; the failure mode is silent CPU
   fallback, and the documented remedy is "try a smaller model" (major, product-level).
2. Msty v1.x couples the app and the inference engine (a renamed Ollama binary) and the only
   supported way to update the engine is copying binaries by hand; v1.9 shipped Ollama 0.6.6 in
   April 2025 (major).
3. The LAN endpoint has no documented key and its IP follows the Wi-Fi address, which broke
   remote clients for at least one user (minor to major depending on how it is used).
4. Windows on Arm is explicitly deprioritised: the maintainer cited LanceDB as the blocker
   (major for Arm users; EZLM ships Windows arm64 builds).
5. Studio is a real step up on first run (hardware check, engine recommendation, Light-to-Powerful
   slider) and is the best model to copy for EZLM's "why this model" surface.

## What I changed

Docs only, no code. Added `agents/T14/products/a05.md` (the teardown) and this report. No other
paths touched.

## What I could not verify

- Reddit r/msty returned HTTP 403 to this network on 2026-11-06 (both the JSON API and
  old.reddit.com), so the complaint table uses the msty-docs issue tracker counts and no upvote
  numbers. Stated in the product section rather than guessed.
- Whether Studio's "hardware-fit information" considers VRAM, and whether the v1.x LAN endpoint
  requires any token. Both would need the app installed.
- Whether a Windows arm64 build of Studio exists.
- Msty's own release cadence beyond the v1.9 changelog entry; there is no public release feed for
  the app, only the changelog page.

## Open questions for the merge

- The v1-to-Studio migration is a live risk for any comparison written from old blog posts: v1.x
  and Studio behave differently on first run and on local engines. The comparison table (a09)
  should say which generation it describes.
- Should EZLM copy Msty's "Light to Powerful" slider verbatim, or expose the runner-up plans from
  `internal/fit` as a list? The slider is simpler; the list is more honest about tradeoffs.

## Next steps

- If someone can run the app on a spare machine, capture Studio's actual hardware check and the
  LAN endpoint's auth behaviour; both are open above.
- a10 can take recommendations 1, 2 and 4 from the product section directly; they are P1 and do
  not depend on each other.
