# T14 / a13 report: model recommendation and memory-fit logic

## What I did

Read the fit and scan code in this worktree (`internal/fit/fit.go`,
`internal/app/app.go`, `internal/catalog/catalog.go`, `internal/scan/scan.go`)
and compared it with public documentation for all eight products. Desk study
only: no competitor installed, no model loaded, no llama-server started.

Deliverable: `agents/T14/a13/fit.md` (comparison table, per-product fit logic,
where EZLM is better and worse, five prioritised recommendations).

## Sources (all accessed 2026-11-06)

- LM Studio system requirements: https://lmstudio.ai/docs/app/system-requirements
- LM Studio `lms load` (`--estimate-only`, auto GPU): https://lmstudio.ai/docs/cli/local-models/load
- LM Studio changelog (context for runtime/estimator changes): https://lmstudio.ai/changelog
- Ollama FAQ (default context 4096): https://docs.ollama.com/faq
- Ollama GPU support: https://docs.ollama.com/gpu
- Ollama source, `-ngl auto` default: https://github.com/ollama/ollama/blob/main/llm/llama_server.go#L438-L444
- Ollama source, conservative `PredictServerVRAM`: https://github.com/ollama/ollama/blob/main/llm/llama_server.go#L2851-L2873
- Jan Hub fit pills: https://docs.jan.ai/desktop/manage-models
- Jan quickstart (default model on first launch): https://docs.jan.ai/desktop/quickstart
- GPT4All models and static RAM Required: https://docs.gpt4all.io/gpt4all_desktop/models.html
- GPT4All default-model rule (<16 GB): https://github.com/nomic-ai/gpt4all/blob/main/gpt4all-chat/src/modellist.cpp#L669-L673
- Msty GPU support (Ollama is the engine): https://docs.msty.app/getting-started/gpus-support
- Msty offline models: https://docs.msty.app/how-to-guides/download-offline-models
- Msty model selector: https://docs.msty.app/features/model-selector
- AnythingLLM system requirements: https://docs.anythingllm.com/installation-desktop/system-requirements
- llamafile README (Windows 4 GB executable limit): https://github.com/mozilla-ai/llamafile#readme
- llamafile running guide (`-ngl 9999`, context "still fits your memory"): https://docs.mozilla.ai/llamafile/using-llamafile/running_llamafile
- LocalAI model configuration (gpu_layers, context_size, placement, VRAM estimate, Fit it for me): https://localai.io/docs/advanced/model-configuration/

## Findings that matter most

1. EZLM is the only product that ranks by quality-adjusted, speed-aware utility
   across quant/context/KV/placement; the others answer "does it fit" (LM Studio,
   Jan, LocalAI) or nothing at all (Ollama, llamafile, AnythingLLM). This is the
   product's edge and the fit logic should be presented as such.
2. EZLM's context ladder stops at 8192 (`fit.go:89-92`). Small machines that
   could run at 4096 get no plan, while Ollama defaults to 4096 and LocalAI to
   512. Recommend adding a 4K rung and a "tight" verdict.
3. On unified memory, `Hybrid`/`Split` require `!h.Unified` (`fit.go:391`,
   `:437`) and both budgets are the wired-limit value (`:174-179`), so models
   between the wired limit and total RAM are rejected on Apple Silicon. Likely
   the largest real-world miss in the planner.
4. Full-GPU plans omit `batchOverhead` (1.2 GB), which hybrid/split include
   (`fit.go:219-229`, `:382-389` vs `:403`, `:440`). Unproven but consistent
   with the Windows silent-spill failure the code comments describe.
5. Minor bug: unreachable `IQ1_M` case at `fit.go:132` (matched by `:130`).
6. LM Studio's `--estimate-only` and LocalAI's `/api/models/vram-estimate` +
   "Fit it for me" are the two affordances worth copying: cheap, repeatable
   estimates separate from the expensive load.

No code was changed (T14 owns docs only).

## What I could not finish or verify

- LM Studio desktop fit badge; Jan's use of available vs total RAM and whether
  context is in the pill; GPT4All's runtime enforcement of `ramrequired`; Msty's
  own fit UI. All marked in `fit.md`.
- The `batchOverhead` claim needs a measured llama-server load, which the rules
  forbid here. It is written up as unproven, not as a bug.
- EZLM `Unlock` UI wiring (engine computes it in `app.go:243-270`; UI not traced).

## Next steps

- a10 can take recommendations 1-5 from `fit.md` directly; each has evidence,
  effort and priority.
- Whoever owns `internal/fit` next should add the 4K rung and the unified split
  fallback first, with tests using synthetic hardware profiles (the existing
  `profiles_test.go` pattern).
- a11 (platforms) should cross-check the unified-memory gap against the Mac app.
