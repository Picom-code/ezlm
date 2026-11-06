# T14 report support: the teardown narrative and the five changes worth making

Agent a16. Desk study, no competitor installed. All sources read on 2026-11-06.
EZLM claims point at this worktree (baseline commit `5213370`).

This file is the synthesis layer for T14. The product sections (a01 to a08), the
comparison table (a09), the recommendation list (a10) and the platform, server,
fit, update and complaint sections (a11 to a15) hold the detail. This file says
what the eight products add up to, and which five changes EZLM should make first.

A note on method: the first draft of this file assumed EZLM's picker was a bare
list. Reading `windows/EZLM/Panes.cs` and `macos/Sources/EZLM/MainView.swift`
showed otherwise. The recommendations below are written against what the code
actually renders, not against the BETA.md summary.

## 1. The narrative

The eight products fall into four archetypes, and the archetype explains almost
every difference in the first-run flow.

**Runtime-first: Ollama.** One binary, one command, a model registry behind it.
There is no hardware-aware recommendation step: `ollama run <name>` pulls the
tag you named, and the default context is 4096 tokens
([FAQ](https://docs.ollama.com/faq)). The local API is unauthenticated by
design: "Local requests do not [need an API key]"
([API introduction](https://docs.ollama.com/api/introduction)). Ollama wins on
time-to-first-token and loses on "which model should I run".

**Model-hub GUI: LM Studio and Jan.** Both put a model browser in front of the
user. Jan's Hub marks each model with whether it fits your hardware
([Jan quickstart](https://docs.jan.ai/desktop/quickstart)); LM Studio tells you
to "choose a 4-bit option or higher if your machine is capable enough" and
leaves the choice there ([Download an LLM](https://lmstudio.ai/docs/app/basics/download-model)).
The fit signal exists but the decision is the user's.

**Single file: llamafile.** Distribute and run a model as one executable
([repo](https://github.com/mozilla-ai/llamafile)). No installer, no model
directory, no updater. The lowest-friction demo and the highest-friction
upgrade path.

**Platform: GPT4All, AnythingLLM, LocalAI, Msty.** Chat is table stakes; the
product is documents, agents, multi-user auth or a model router. Their local
server is a feature of the platform, not the reason people install it. GPT4All's
server listens on localhost only and has no key
([API server](https://docs.gpt4all.io/gpt4all_api_server/home.html)); LocalAI
adds `LOCALAI_API_KEY` but one key "grant[s] full admin access with no role
separation" ([quickstart](https://localai.io/docs/basics/getting_started/)).

What all eight converge on: a model downloader, a chat window, and an
OpenAI-compatible endpoint on localhost. What they diverge on: **who decides
what runs** (user or app), and **how much surface** ships with it.

EZLM sits in the opposite corner from the hubs: the app decides, and the surface
stays narrow (deploy, chat, wire OpenCode and Hermes). That is defensible, and
the picker already carries the argument: each plan card shows smarts and speed
bars, words per second, context, quant, and where the model runs
(`windows/EZLM/Panes.cs:67-92`, `Plan.How` and `CtxLine` in
`windows/EZLM/Models.cs:72-99`). What the card does not show is the number that
decides whether the first run is pleasant: how much memory is left over. And
what the app does not offer is control over the one parameter users most often
need to trade: context.

So the teardown's headline is not "EZLM should copy a model browser". It is
"EZLM already made the decision; finish making it legible and adjustable, and
remove the install and start-up failures that stop people seeing it".

## 2. What was learned

1. **Fit is the product, and everyone else makes the user do it.** Jan shows a
   fit badge per model; LM Studio gives quant guidance; Ollama does neither.
   Ollama's issue tracker has 466 open issues mentioning VRAM (GitHub search,
   2026-11-06, see the appendix). EZLM already goes further than all three by
   ranking plans (`internal/fit/fit.go:232-300`) and rendering the ranking
   (`Panes.cs:67`). The remaining gap is headroom, not ranking.
2. **Context is the silent failure mode.** Ollama ships a 4096-token default and
   documents the override as an environment variable. Users then report that the
   model "forgets" or that an agent breaks. EZLM tries 64K, 32K, 16K and 8K in
   that order and knows Hermes refuses below 64K
   (`internal/fit/fit.go:88-92`), shows the chosen size on every card and on the
   Overview, and has a Hermes trade-off dialog (`Panes.cs:576-578`). The gap is
   that the user cannot choose a different point on that ladder after the fact.
3. **Local API auth is mostly absent.** Ollama: no key locally. GPT4All: localhost
   only, no key. LocalAI: one key, full admin. EZLM is ahead: named keys
   (`internal/serve/config.go:21`), `--api-key` passed to llama-server
   (`:121`), an explicit LAN toggle (`:93`, `:103`) and new/rotate/revoke in both
   apps (`Panes.cs:689-717`, `MainView.swift:600-640`). What is missing is proof
   that another device can reach the server, and any record of which key is in
   use.
4. **Platform breadth is a tax.** AnythingLLM, Jan and LocalAI all carry
   documents, agents, cloud providers and MCP. It wins reviews and costs
   maintenance. EZLM should keep saying no and spend the saved effort on the
   deploy path.
5. **Hardware coverage is both moat and complaint sink.** LM Studio supports
   Windows on ARM but not Intel Macs, and requires AVX2 on x64
   ([system requirements](https://lmstudio.ai/docs/app/system-requirements)).
   Ollama's ROCm support is an explicit card list, with `HSA_OVERRIDE_GFX_VERSION`
   as the escape hatch for everything else ([hardware support](https://docs.ollama.com/gpu)).
   EZLM's Vulkan-then-CPU fallback with a visible "Running on the processor"
   note (BETA.md) is the right shape. It should stay loud.
6. **Updates are a trust surface.** Ollama auto-downloads and offers "Restart to
   update" ([FAQ](https://docs.ollama.com/faq)). EZLM checks GitHub releases
   unauthenticated (`internal/update/update.go:22`) and ships unsigned
   installers; BETA.md says Smart App Control blocks them with no "Run anyway".
   For a public beta, install conversion matters more than feature count.
7. **Downloads dominate the first run.** Models are 2 to 60 GB (BETA.md). EZLM
   already resumes after a stall, sends `Range` and verifies digests
   (`internal/engine/download.go:130`, `internal/engine/pinned.go:9`), and the
   picker shows download size and a "Needs space" tag (`Panes.cs:78`,
   `:458-459`). That is better than most of the field and should be said out
   loud in the release notes.
8. **"Fits" is not "fast".** EZLM's utility function deliberately trades speed
   for intelligence above about 40 tok/s (`internal/fit/fit.go:271-286`). Users
   judge a local model by tokens per second first. The picker shows a speed bar
   and words per second, which is the right call.

## 3. The five changes with the best expected value

Ranked by expected value, with effort and the EZLM area each touches. The first
two are cheap because the data already exists; the last three are about the
paths that stop people reaching the product.

| # | Change | Effort | Primary area |
|---|---|---|---|
| 1 | Sign and notarize the builds, and make the update check offline-tolerant | M code + L signing | `internal/update`, installers |
| 2 | Put memory headroom and disk preflight on the plan cards | S | `internal/fit`, `Panes.cs`, `Screens.swift` |
| 3 | Give context a control, with its memory cost and per-agent presets | S/M | `internal/fit`, `internal/serve`, Settings |
| 4 | Turn "server couldn't start" into a diagnosis with one-click remedies | M | `internal/serve`, `internal/scan`, app |
| 5 | Add a LAN reachability test and key last-used | S/M | `internal/serve`, Keys pane |

### 3.1 Sign and notarize the builds, and make the update check offline-tolerant (effort M code, L signing)

**Change.** Sign the Windows build and notarize the macOS build. On the update
path, cache the last successful check, treat a rate limit or an offline machine
as a normal state with a timestamp ("checked 3 hours ago"), verify the installer
digest before running it, and offer "restart to update" rather than a silent
relaunch.

**Evidence.** Ollama's update story is the bar: automatic download, then a
"Restart to update" menu item ([FAQ](https://docs.ollama.com/faq)). EZLM checks
`api.github.com` unauthenticated (`internal/update/update.go:22`), which is
limited to 60 requests per hour per IP and is shared by everyone behind the same
NAT. BETA.md is explicit: on Windows 11 with Smart App Control, the unsigned
build is blocked with no "Run anyway". That is an install-blocking bug, not a
polish item.

**Why it pays.** Every blocked install is a user who never sees the fit engine.
Signing is the highest-leverage non-code change available, and the rate-limit
handling is a few lines.

**Risk.** Signing costs money and key management. Start with Windows, where the
block is hard, and notarize macOS in the same release so the two installers do
not drift.

**Success measure.** Install completion rate, and no raw HTTP error ever shown
in the Updates pane.

### 3.2 Put memory headroom and disk preflight on the plan cards (effort S)

**Change.** On each plan card, next to the speed bar, show the memory the plan
will use and the memory it leaves free ("9.2 of 12 GB graphics memory, 2.8 GB
free"). Show the download size against free disk on the same line, and make the
"Needs space" tag say how much is missing. Keep the full breakdown behind
"Details".

**Evidence.** LM Studio makes the user pick a quant with one sentence of
guidance ([Download an LLM](https://lmstudio.ai/docs/app/basics/download-model)).
Jan shows a fit badge but not the margin ([quickstart](https://docs.jan.ai/desktop/quickstart)).
Ollama does not model fit at all, and 466 open issues mention VRAM. EZLM
computes every number needed: `Plan.GPUNeed`, `RAMNeed`, `GPUBudget`,
`RAMBudget`, `Download`, `DiskOK` (`internal/fit/fit.go:50-63`), and the card
already renders `Plan.How`, `CtxLine` and `QuantLine` (`Panes.cs:67-92`). This
is a rendering change, not a planner change.

**Why it pays.** "Will this be fast enough and will it fit" is the question the
picker exists to answer. The bars answer speed; the headroom number answers
fit. It also gives support a single screenshot to ask for.

**Risk.** Headroom is an estimate and users will treat it as a promise. Label it
"about" and round conservatively, as `Plan` already does with margins
(`internal/fit/fit.go:198`).

**Success measure.** Fewer reports of "it picked a model that runs badly", and
users choosing a different card when headroom is thin.

### 3.3 Give context a control, with its memory cost and per-agent presets (effort S/M)

**Change.** After a model is picked, let the user move context along the same
ladder the planner uses (8K, 16K, 32K, 64K), showing the memory cost ("32K,
about 2.1 GB of cache") and the predicted speed at each step. Add named presets
for the integrations EZLM ships: OpenCode (32K) and Hermes (64K). When Hermes
is wired to a model that cannot reach 64K, say so before the download, not after.

**Evidence.** Ollama's documented default is 4096 tokens and the fix is an
environment variable ([FAQ](https://docs.ollama.com/faq)). That one default is
behind a large share of "the model forgets" reports. EZLM has the ladder
(`internal/fit/fit.go:88-92`), the Hermes constraint (`MinCtx`, `:85`), the
trade-off copy (`Panes.cs:576-578`) and a live `/props` read of the context the
server actually loaded (`internal/serve/props.go:18`). It lacks the control.

**Why it pays.** Context is the one parameter that changes both memory use and
agent compatibility, and EZLM already knows the safe range. Exposing it converts
a hidden planner decision into a user choice without adding a new subsystem.

**Risk.** Users pick 128K and blame EZLM for the slowdown. Show the predicted
speed at each step and cap the ladder at what fits.

**Success measure.** Context changed deliberately rather than never, and fewer
Hermes setup failures.

### 3.4 Turn "server couldn't start" into a diagnosis with one-click remedies (effort M)

**Change.** When the health check fails, classify the cause and offer one action
per cause: port taken (show the process, pick another port), stale process (stop
by PID), driver or VRAM shortfall (retry with smaller context or quant, or run on
the processor), missing model file (re-download). Keep "Copy details for
support" next to every branch.

**Evidence.** The dominant complaint shape across the field is "it downloaded
but the model does not load" or "the endpoint is unreachable" (Ollama's 466 open
VRAM issues; per-product counts in a15). EZLM today says "The server couldn't
start", appends `StartError`, and offers "Try again" (`Panes.cs:317-319`,
`Shell.cs:174`). That is a retry, not a diagnosis. The primitives exist:
`PortFree`/`FreePort` (`internal/serve/config.go:146`, `:174`), `StopAndWait`
(`:201`), the `CPUFallback` reason (`:47`) and the stopped flag (`:193`).

**Why it pays.** A beta's reputation is set by the worst first run, and this is
where first runs fail. It also reduces the support load that lands on a small
team.

**Risk.** Auto-retry can mask a real bug. Log every attempt and keep the
diagnosis readable rather than clever.

**Success measure.** Share of failed deploys recovered without a GitHub issue.

### 3.5 Add a LAN reachability test and key last-used (effort S/M)

**Change.** In the Keys pane, add "Test from another device": show the LAN URL,
a copyable curl, and a check that the request arrived. Record and display a
last-used time per key. Keep the existing new, rotate and revoke actions as they
are.

**Evidence.** Ollama's local API needs no key at all
([API introduction](https://docs.ollama.com/api/introduction)); GPT4All's
server is localhost-only with no key
([API server](https://docs.gpt4all.io/gpt4all_api_server/home.html)); LocalAI
has one key that grants full admin
([quickstart](https://localai.io/docs/basics/getting_started/)). EZLM's named
keys are already better than all three, and rotate/revoke already update the
wired apps (`Panes.cs:705-711`). The gap is verification and observability.

**Why it pays.** The key is the artifact users paste into other apps; the LAN URL
is the one thing that silently does not work (firewall, wrong adapter, VPN). A
test that proves reachability turns a class of "it doesn't work" into a
self-serve fix.

**Risk.** Last-used needs a request counter; a proxy or a log scrape adds
complexity. If that is too much, ship the reachability test alone and note the
last-used as a follow-up.

**Success measure.** LAN setup completed on the first try, and stale keys
identified and rotated without a support request.

## 4. What not to copy

- **A model browser.** LM Studio and Jan win on catalogue breadth; EZLM wins on
  not asking. Keep the picker to ranked cards and a small "more models" path.
- **RAG, agents, cloud providers.** AnythingLLM, Jan and LocalAI show the
  maintenance cost. EZLM's narrow surface is an advantage while the team is
  small.
- **Unauthenticated localhost as a default.** Convenient, and the reason "any
  local process can use my model" is a recurring thread. EZLM already does
  better; do not regress for convenience.
- **A single shared API key.** LocalAI's admin-everything key is the cautionary
  case.

## 5. Limits and open questions

- Only one GitHub issue count survived the unauthenticated search rate limit
  (Ollama VRAM, 466). The per-product counts belong to a15; this file does not
  duplicate them.
- Msty was not fetched successfully (the site redirects cross-origin). Its
  claims here are absent rather than negative.
- llamafile was read from the repository landing page only; its GPU behaviour on
  Windows on Arm is a11's job.
- The Windows and macOS UI files I read are on the "being rewritten tonight"
  list. The gaps above are real in this snapshot; a rewrite may already close
  some of them. Re-check `Panes.cs`, `Screens.swift` and `MainView.swift`
  before scheduling.
- Open question: should the headroom line be on the card or one tap deeper? My
  read is the card, because the card is where the choice happens, but five
  numbers on a card can become noise.
- Open question: is code signing funded for the beta, or is Smart App Control
  documented as a known limit until GA? If the latter, the installer should say
  so before the download, not after.
- Open question: does EZLM want a stable machine-profile identifier so support
  reports can be grouped? `internal/report` already redacts keys; a profile hash
  would help triage without new personal data.

## Appendix: sources and counting

All accessed 2026-11-06.

| Source | Used for |
|---|---|
| https://docs.ollama.com/faq | 4096 default context, auto-update and restart |
| https://docs.ollama.com/api/introduction | local requests need no key |
| https://docs.ollama.com/gpu | ROCm card list, HSA override, driver floors |
| https://lmstudio.ai/docs/app/system-requirements | Windows ARM, no Intel Mac, AVX2 |
| https://lmstudio.ai/docs/app/basics/download-model | user chooses the quant |
| https://lmstudio.ai/docs/developer/core/server | local server and network serving |
| https://docs.jan.ai/desktop/quickstart | default model on first launch, per-model fit badge |
| https://docs.gpt4all.io/gpt4all_api_server/home.html | localhost only, no key, port 4891 |
| https://localai.io/docs/basics/getting_started/ | `LOCALAI_API_KEY`, admin access, backend detection |
| https://docs.anythingllm.com | platform breadth (RAG, agents, model router) |
| https://github.com/mozilla-ai/llamafile | single-file distribution |

Counting method: GitHub issue search, `is:issue is:open` plus a keyword, run
against the public search API on 2026-11-06. Query for the one reported count:
`https://github.com/ollama/ollama/issues?q=is%3Aissue+is%3Aopen+VRAM` (466
results). Four further queries (Ollama "context", LM Studio "GPU", Jan "model
load", GPT4All "GPU") returned HTTP 403 rate-limit errors from the shared IP, so
they are not reported. Treat 466 as a lower bound on one product's fit-related
issue volume, not as a cross-product comparison.
