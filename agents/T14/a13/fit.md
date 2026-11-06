# Model recommendation and memory-fit logic across the products

Agent a13, T14 competitive teardown. All URLs accessed 2026-11-06. Desk study: no
competitor was installed, no model was loaded. EZLM references are to
`internal/fit/fit.go` and `internal/app/app.go` in this worktree (branch
`agent/T14-a13`).

Scope: how each product decides (a) which model to offer a user and (b) whether it
fits in memory, plus where EZLM's planner is stronger or weaker. Server/keys and
platform detail live in a12/a11.

## Short version

| Product | Recommends a model? | Fit check | Granularity | Honest about uncertainty? |
|---|---|---|---|---|
| LM Studio | Curated hub, no per-machine ranking | Yes, on demand (`lms load --estimate-only`), plus automatic GPU offload | Estimated GPU + total memory, context and vision aware, checked against "resource guardrails" | Yes: "may be loaded" wording, estimate is a preview |
| Ollama | No: user picks a library tag | Auto: `-ngl auto`, backend picks layers; VRAM prediction is deliberately conservative | Layers offloaded; no up-front verdict | Partly: warns via logs, not the UI |
| Jan | Yes: Hub with Fits / May be slow / Won't fit pills and Small/Balanced/Large quant tiers | Yes, from hardware profile; computed locally without downloading | Model + quant tier, three-state | Yes: three states, not a number |
| GPT4All | No ranking; static "RAM Required" per model | Static number, not machine-derived; default-model pick uses a hardcoded <16 GB rule | Model | No |
| Msty | Featured list by use case | Inherits Ollama's | As Ollama | As Ollama |
| AnythingLLM | No | Generic 16 GB RAM / 8-12 GB VRAM guidance; fit delegated to the engine | None | n/a |
| llamafile | No: you pick a file | None automatic; docs tell you to pass `-ngl 9999` and choose a context that "still fits your memory" | User's job | No |
| LocalAI | Gallery + per-model config | Yes: `/api/models/vram-estimate`, "Fit it for me", 95% usable limit, auto/custom placement | Per model, per GPU, weights + working memory + KV at chosen context | Yes: "Fits in GPU" / "Spills to CPU" / "Too many layers" |
| **EZLM** | **Yes: ranked plans across the whole catalog** | **Yes: exhaustive quant × context × KV × placement search** | **Per plan: GPU need, RAM need, speed, quality-adjusted score, download size** | **Mostly: `Unlock` shows what freeing memory would allow** |

## Per product

### LM Studio

- System requirements are advice, not a fit check: 16 GB+ RAM recommended, 4 GB+
  dedicated VRAM recommended, Apple Silicon only, Windows x64 and ARM
  (<https://lmstudio.ai/docs/app/system-requirements>).
- The fit machinery is real and inspectable: `lms load` has `--estimate-only`,
  which prints `Estimated GPU Memory` and `Estimated Total Memory` and a verdict
  ("This model may be loaded based on your resource guardrails settings"). The
  estimator accounts for context length, flash attention and vision models. If
  `--gpu` is not given, "LM Studio will automatically determine optimal GPU
  usage" (<https://lmstudio.ai/docs/cli/local-models/load>).
- Model discovery is curated, not machine-ranked: the hub is a catalogue, and the
  user is expected to know their hardware. No public doc says the hub sorts by
  what fits.
- Takeaway for EZLM: LM Studio separates "estimate" (cheap, repeatable, CLI) from
  "load" (expensive). EZLM computes the same numbers inside `Scan` but has no
  cheap, scriptable estimate for an arbitrary model/quant before download.

### Ollama

- There is no recommendation step. `ollama run <model>` pulls a default tag and
  the user is responsible for knowing whether it fits; the library shows file
  size, not a verdict.
- Fit is delegated to llama.cpp: `NumGPU == -1` (the default) means Ollama does
  not pass `-ngl` at all and lets llama-server auto-detect
  (<https://github.com/ollama/ollama/blob/main/llm/llama_server.go#L438-L444>).
- Ollama's own estimate is a conservative over-estimate used for scheduling, not a
  user-facing promise: `PredictServerVRAM` uses the model file size as a proxy
  for weights plus a rough f16 KV estimate, and the comment says it
  "intentionally overestimates to avoid VRAM contention"
  (<https://github.com/ollama/ollama/blob/main/llm/llama_server.go#L2851-L2873>).
- Context defaults are the quiet fit trap: the FAQ says Ollama uses 4096 tokens
  by default, overridable with `OLLAMA_CONTEXT_LENGTH` or `num_ctx`
  (<https://docs.ollama.com/faq>). A model can "fit" at 4K and then be unusable
  for a coding agent that sends 10K+ prompts, which is exactly the failure EZLM's
  64K floor is designed to prevent (`internal/fit/fit.go:89-92`).
- Takeaway: Ollama is the strongest counter-example to EZLM's approach (just let
  the backend decide), and the weakest on the thing EZLM sells (a model that is
  actually good for the job).

### Jan

- The most directly comparable design to EZLM's recommendation step. The Hub
  shows a coloured fit pill per model, **Fits / May be slow / Won't fit**, based
  on the user's hardware; quants are grouped **Small / Balanced / Large** with a
  **Recommended** tag on the default download, and "No data is downloaded to
  determine fit status"
  (<https://docs.jan.ai/desktop/manage-models>).
- Jan ships a default model and downloads it on first launch, so the first run
  never asks the user to choose
  (<https://docs.jan.ai/desktop/quickstart>).
- Fit is model-level, not plan-level: Jan does not appear to document choosing a
  context size, KV quant, GPU-layer count or MoE placement to make a model fit.
  Its answer is "this model may be slow", where EZLM's is "this quant at this
  context with these layers".
- Takeaway: the three-state pill is a better honesty device than a single
  recommendation, and cheap to copy. EZLM's `Speed` field (`fast`/`good`/`slow`,
  `internal/fit/fit.go:487-494`) is the same idea but is not the headline.

### GPT4All

- Fit is a static catalogue field, not a measurement: the docs table lists
  `Filesize`, `RAM Required`, parameters and quant per model (for example
  "Llama 3 Instruct, 4.66 GB, 8 GB RAM")
  (<https://docs.gpt4all.io/gpt4all_desktop/models.html>).
- The code confirms it: `ramrequired` is loaded from the model JSON
  (`gpt4all-chat/src/modellist.cpp:1632`) and the automatic default model is
  "the first installed model that requires less than 16GB"
  (`gpt4all-chat/src/modellist.cpp:672`,
  <https://github.com/nomic-ai/gpt4all/blob/main/gpt4all-chat/src/modellist.cpp#L669-L673>).
  That 16 is a hardcoded constant, not the machine's RAM, so an 8 GB machine and
  a 64 GB machine get the same default rule.
- Takeaway: a static RAM number is cheap and useful, but it drifts from reality
  (context, KV cache and GPU offload all move the number). EZLM's computed
  `RAMNeed`/`GPUNeed` are strictly better if surfaced.

### Msty

- Msty's local engine is Ollama; its own GPU support page says "Msty (and Ollama,
  the underlying Local AI engine)"
  (<https://docs.msty.app/getting-started/gpus-support>). Fit behaviour is
  therefore Ollama's: auto layer offload, no recommendation.
- Its model discovery is editorial: Featured Models grouped by task (coding,
  text, multilingual, image-to-text) plus a Model Hub over Ollama and Hugging
  Face, with size as the only guidance
  (<https://docs.msty.app/how-to-guides/download-offline-models>). The model
  selector is about pinning and organising, not fitting
  (<https://docs.msty.app/features/model-selector>).
- Takeaway: "hand-picked by task" is a useful complement to "best that fits".
  EZLM's catalog has `Score` and `ToolCalling` but no task grouping.

### AnythingLLM

- Not a runtime first: it is a RAG/workspace app that connects to a local engine
  (built-in llama.cpp, Ollama, LM Studio, LocalAI, KobaldCPP, oMLX) or a cloud
  provider. Fit guidance is one generic sentence: 16 GB RAM, 8-core CPU, and on
  Windows "a GPU is recommended ... (8-12GB+ VRAM is great!)"
  (<https://docs.anythingllm.com/installation-desktop/system-requirements>).
- No per-model fit check in the docs; the user is expected to size the model
  against the engine they connected.
- Takeaway: the weakest fit story of the eight, and the least relevant to EZLM's
  one-click promise. Its value to EZLM is as an integration target, not a model.

### llamafile

- No recommendation and no fit logic. The docs' own examples pass `-ngl 9999`
  explicitly, and for agent use they tell the reader to "choose a context size
  which is large enough (but still fits your memory)"
  (<https://docs.mozilla.ai/llamafile/using-llamafile/running_llamafile>).
- Windows has a hard packaging limit worth noting for EZLM's own distribution
  choices: "Only executables under 4GB can run on Windows, so any llamafile above
  4GB won't work" (<https://github.com/mozilla-ai/llamafile#readme>).
- Takeaway: the single-file model is the opposite bet to EZLM. It wins on
  portability and loses exactly where EZLM wins: the user must know what fits.

### LocalAI

- The closest competitor to EZLM's planner, and the one with the most explicit
  fit vocabulary. Model YAML defaults: `gpu_layers: 99999999` (all layers
  requested), `context_size: 512`, `mmap: true`, `low_vram: false`.
  `context_size: -1` uses the model's full trained context "raw max, no VRAM
  capping; a warning is logged if it may not fit detected VRAM"
  (<https://localai.io/docs/advanced/model-configuration/>).
- Placement is Auto / CPU only / Custom. In Auto the `llama-cpp` backend "lowers
  that to what fits in the free device memory". The UI has per-GPU and system
  memory bars, `POST /api/models/vram-estimate` for one total at a chosen
  context and layer count, a **Fit it for me** button that searches for the
  largest `gpu_layers` that fits, and a "usable limit is 95 percent of the free
  memory". Verdicts are words: "Fits in GPU", "Spills to CPU", "Too many layers
  for the GPU", and "does not state a speed".
- MoE placement is manual (`cpu_moe:true`, `n_cpu_moe:12`) plus an optional
  `moe_cache_mib` GPU expert cache, with the doc warning that a cache that is
  too small can thrash
  (<https://localai.io/docs/advanced/model-configuration/>).
- Takeaway: LocalAI has the best *explainability* (per-device bars, words not
  scores) and the worst *defaults* (512 context, manual MoE, no speed claim).
  EZLM's 64K floor and automatic `-ncmoe` are real advantages; LocalAI's
  estimate endpoint and "fit it for me" affordance are the things to copy.

## Where EZLM's planner is better

1. **It ranks by usefulness, not by fitting.** `PlansFor` scores every
   quant × context × KV × placement candidate as
   `Score × quantQuality × kvQuality × ctxFactor`
   (`internal/fit/fit.go:295-330`, effective score at `:317`) and ranks with a
   speed-aware utility (`:259-283`). Jan's pill, Ollama's `-ngl auto` and
   LM Studio's estimator answer "can it run", not "what is the best thing that
   runs". This is EZLM's main differentiator and should stay the headline.
2. **It plans the placement, not just the offload.** Hybrid MoE (`-ncmoe`
   computed from layer size and batch overhead, `:391-435`), dense split
   (`:437-453`) and CPU-only (`:455-460`) are chosen per model. LocalAI needs
   `cpu_moe`/`n_cpu_moe` by hand; LM Studio exposes a GPU-offload fraction;
   Jan and GPT4All do not document per-layer placement at all.
3. **It reserves memory for the OS and the user's apps, differently per OS.** The
   unified-memory cap, the Linux 4% margin, the Windows 8% margin and the Vulkan
   12% margin (`:171-196`) encode failure modes the competitors' docs do not
   mention. The comment records the observed Windows silent spill (27B at
   8.7 tok/s instead of ~50, then a crash).
4. **It knows what the model is for.** The 64K context ladder (`:89-92`) and the
   `MinCtx` floor for Hermes/OpenCode (set in `internal/app/app.go:371` and
   `:534`) mean the recommended plan can serve a coding agent. Ollama's 4K
   default and LocalAI's 512 default do not.
5. **It answers "what if I freed memory".** `Scan` computes an `Unlock` plan
   with the RAM that closing apps would free, and a separate GPU-busy case, with
   the GB delta (`internal/app/app.go:243-270`). LM Studio has guardrails;
   nobody else documents a "close these to unlock that" plan.
6. **Quality is quant-aware.** `quantQuality` and `minQuality` (`:97-165`)
   stop a 2-bit quant of a big model from beating a 4-bit quant of a smaller one
   on the strength of parameter count alone. No other product's docs describe
   this; Jan only groups quants into Small/Balanced/Large.

## Where EZLM's planner is worse

1. **No cheap, scriptable estimate for an arbitrary file.** LM Studio's
   `lms load --estimate-only` and LocalAI's `/api/models/vram-estimate` can
   answer "would this fit at 32K?" without a scan-and-plan over the whole
   catalog. EZLM's numbers exist only inside a full `Scan` (`app.go:239`) or a
   `PlanForQuant` call with a plan already in hand (`fit.go:577-581`). Effort to
   close: small.
2. **The context ladder bottoms out at 8192.** A machine that could run a model
   at 4096 (small VRAM, 8 GB laptop, a 30B at Q4 on 6 GB) gets no plan at all
   rather than a slow-but-working one (`fit.go:89-92`). LocalAI will run at 512;
   Ollama at 4096; llamafile at whatever you pass. This is a real hole for the
   low end, and it interacts badly with `MinCtx`: with `MinCtx` set the filter
   is `c.n < b.MinCtx` (`:309`), so every rung below the floor disappears.
3. **No partial offload on unified memory.** `Hybrid` and `Split` both require
   `!h.Unified` (`:391`, `:437`), and on unified machines `ComputeBudgets`
   returns the same value for GPU and RAM (`:174-179`). So a model between the
   GPU wired limit and total RAM is rejected on Apple Silicon, where llama.cpp
   would happily run it with some layers on the GPU. On a 32 GB Mac that is
   roughly the 24-32 GB model band.
4. **The speed number is a hand-calibrated claim.** `TokPerSec` comes from
   bandwidth constants, per-backend fudge factors and a MoE sparsity exponent
   (`:360-373`, `:386-460`), tuned from a handful of measured machines named in
   the comments (RTX 4070 SUPER, RTX 4090, RTX Spark, M3 Max). LocalAI
   deliberately "does not state a speed"; EZLM states one to one decimal place
   (`:486`). When it is wrong it is wrong in the confident direction, and the
   tests (`internal/fit/fit_test.go`, `profiles_test.go`) check plumbing, not
   accuracy against real loads.
5. **Full-GPU plans omit the batch compute buffer.** `batchOverhead` (1.2 GB) is
   added in hybrid (`:403`) and split (`:440`) but not in the all-on-GPU case,
   where `full = w + kvBytes + oh` and `oh` is 320 MB + 4 KB/token + 1% of
   weights (`:219-221`, `:382-389`). At 64K context that is ~0.6 GB of
   allowance for compute buffers on a 2048-token prompt batch. If that is low,
   the Windows "silent spill" the code comments warn about is still reachable on
   the plans EZLM is most confident about. **Unproven**: needs a measured
   llama-server load, which this desk study could not do.
6. **Dead code in the quality table.** `case strings.HasPrefix(n, "IQ1_M")`
   at `fit.go:132` is unreachable because the case at `:130` already matches
   `IQ1_M` and returns 0.8. The intended 0.82 never applies. Minor, but it means
   the table does not say what it looks like it says.

## Recommendations for EZLM

1. **Add a one-shot estimate command and API.** `ezlm fit <model> [--quant]
   [--ctx]` printing GPU need, RAM need, speed, recipe and a verdict against the
   current budgets, plus the same as a local API call. Reuse `PlanForQuant` and
   `ComputeBudgets`; do not rescan the catalog. Evidence: LM Studio
   `--estimate-only` (<https://lmstudio.ai/docs/cli/local-models/load>), LocalAI
   `POST /api/models/vram-estimate`
   (<https://localai.io/docs/advanced/model-configuration/>). Effort S.
   Priority high: it makes the planner inspectable and testable without a UI.
2. **Add a 4096 rung (and a 2048 last resort) to the context ladder, and stop
   dropping plans below `MinCtx`.** Mark them "slow" or "tight" rather than
   removing them, so a 6-8 GB machine still gets a recommendation. Evidence:
   Ollama's 4K default, LocalAI's 512 default, and EZLM's own `MinCtx` filter
   (`fit.go:89-92`, `:309`). Effort S. Priority high: it is the low-end market.
3. **Allow partial offload on unified memory.** When `h.Unified` and the model
   does not fit `b.GPU`, fall through to a split/hybrid plan using total RAM as
   the CPU budget instead of rejecting the model. Evidence: `fit.go:391`/`:437`
   exclude unified machines; llama.cpp supports `-ngl` below layer count on
   Metal. Effort M. Priority medium-high: it is the difference between "no model
   fits" and "runs at 12 tok/s" on 32 GB Macs.
4. **Make the fit verdict a first-class UI state, with the unlock plan.** Show
   `Fits` / `Fits, slow` / `Won't fit` per candidate the way Jan does, and put
   the existing `Unlock` plan ("close 6 GB of apps to run Qwen3.6 35B A3B") next
   to it. Evidence: Jan's fit pills
   (<https://docs.jan.ai/desktop/manage-models>), LocalAI's word verdicts, and
   `app.go:243-270` which already computes the unlock case. Effort S-M.
   Priority medium: cheap, and it turns a hidden strength into a visible one.
5. **Add `batchOverhead` to full-GPU plans, or calibrate it.** Compute the
   full-GPU need with the same 2048-token batch allowance used in hybrid/split,
   and add a test that compares predicted need to llama-server's reported buffer
   sizes on one known machine. Evidence: `fit.go:219-229`, `:382-389`,
   `:403`, `:440`. Effort S. Priority medium: protects the Windows path from the
   spill the comments already describe. Mark as unproven until measured.

Minor: fix the unreachable `IQ1_M` case at `fit.go:132` (effort S).

## What I could not verify

- LM Studio's in-app fit indicator (as opposed to the CLI estimator). The docs
  I read describe `--estimate-only` and automatic GPU usage; I did not find a
  doc for a per-model badge in the desktop UI, so the table says "curated hub".
- Whether Jan's fit pill uses available or total RAM, and whether it accounts
  for context length. The docs only say it is computed locally from hardware.
- GPT4All's runtime behaviour when a model exceeds RAM (refusal, warning, or
  swap). The static `ramrequired` field is verified; the UI enforcement is not.
- Msty's own fit UI, if any, beyond the inherited Ollama behaviour.
- Whether EZLM's `Unlock` is shown in both apps or only in the engine's JSON
  (`app.go:249-251`). The field exists; I did not trace the UI.
- The `batchOverhead` gap is a code reading, not a measurement (see above).

## Open questions

- Should EZLM ever recommend a plan it knows will page from SSD (`Stream`), or is
  a smaller model always better? Today `Stream` is allowed up to 1.1× free RAM
  (`fit.go:420-427`) with a `pageFactor` penalty.
- The 8K context factor (0.6) is a bigger quality penalty than moving from Q4 to
  Q2 (0.985 to 0.88). Is that the right trade for a chat user who will never
  send 8K tokens?
- Is `m.Score` comparable across families (different benchmarks, `ScoreSource`
  per model)? Ranking depends on it more than on any memory number.
