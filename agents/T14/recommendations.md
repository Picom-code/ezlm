# Prioritised recommendations for EZLM

Agent a10, task T14. Written 2026-11-06. Every source was fetched on 2026-11-06; access dates are
in each entry. This list is a desk study of public docs, READMEs and GitHub issue search, checked
against EZLM's own code in this worktree (`agent/T14-a10`). No competitor product was installed.

## How this list was built and ranked

- **Sources.** Official docs and READMEs first (Ollama, LM Studio, Jan, GPT4All, llamafile, LocalAI,
  AnythingLLM). Complaint counts come from GitHub's issue search API, query shown with each count.
  GitHub search counts open issues only and is a lower bound: it misses Discord, Reddit and closed
  threads, and a keyword match can include unrelated issues.
- **Ranking.** Ordered by expected value for the public beta: (1) things that can lose a user's work,
  memory or trust; (2) things that decide whether a coding agent connects on the first try;
  (3) polish that reduces support load; (4) larger bets. Ties go to the smaller effort.
- **Effort.** S is under half a day for one person, M is one to three days, L is more than three days.
- **EZLM area.** Paths are relative to the repo root. Areas being rewritten tonight
  (`internal/integrate/**`, `internal/app/**`, `cmd/**`, `internal/web/static/**`, and the listed UI
  files) are still named because the recommendation is about behaviour, not a patch location.
- **Confidence.** Each item says whether EZLM's current behaviour was read in code (file:line) or
  inferred from docs and UI text. Inferred items need a five-minute check before work starts.
- **Sibling sections.** a01 to a08 had not committed their product sections when this was written
  (all T14 branches were still at the baseline commit). Reconcile this list with their findings at
  merge; anything they evidence more strongly should keep its rank, and anything they contradict
  should be corrected rather than dropped silently.

## Tier 1: before the public beta (1 to 8)

### R01. Unload the model when it is idle, and say when it will unload
**Effort:** M. **Area:** `internal/serve` (config, lifecycle), `internal/app`.
**Do:** add an idle timeout (default around 15 minutes, user-settable) that stops llama-server, and
show "model unloads after N minutes idle" in the UI and `ezlm status`.
**Why:** LM Studio auto-unloads JIT-loaded models after an inactivity period and documents the
behaviour as a feature ([LM Studio headless docs](https://lmstudio.ai/docs/developer/core/headless),
2026-11-06). Ollama's keep-alive has the same role. A desktop that starts EZLM at login and never
releases `-c` context memory is the difference between "my laptop is fine" and "EZLM ate my RAM".
**EZLM today (verified):** `internal/serve/config.go:113` builds explicit `-c`, `-ngl`, `-np 1`
args; nothing in `internal/serve` implements an idle stop, and `BETA.md` says the server starts at
sign-in and stays up.

### R02. Pick the update asset by OS and CPU architecture, not by filename prefix
**Effort:** S. **Area:** `internal/update/update.go`.
**Do:** match `windows/amd64`, `windows/arm64` and `darwin/arm64` assets explicitly, and fail closed
with a readable message when no asset matches this machine.
**Why:** GPT4All ships a separate Windows ARM installer and Jan ships a separate Linux arm64 build
([GPT4All README](https://raw.githubusercontent.com/nomic-ai/gpt4all/main/README.md),
[Jan README](https://raw.githubusercontent.com/menloresearch/jan/main/README.md), 2026-11-06).
Windows on Arm is explicitly in EZLM's beta audience (`BETA.md`).
**EZLM today (verified):** `internal/update/update.go:43-51` accepts any file starting
`EZLM-Setup` and ending `.exe` for every Windows machine, so an Arm laptop can be offered the x64
installer.

### R03. Recover from a runtime out-of-memory, not only from a Vulkan allocation failure
**Effort:** M. **Area:** `internal/app` (fallback), `internal/fit`.
**Do:** when the server dies with an allocation error, retry once with a smaller context (and then
the next quant down), and tell the user what changed. Keep the existing Vulkan-to-CPU path.
**Why:** memory failures are the largest single complaint bucket in the two biggest projects:
85 open Ollama issues match "out of memory" and 313 open LM Studio bug-tracker issues match
"memory" ([Ollama search](https://api.github.com/search/issues?q=repo%3Aollama%2Follama+is%3Aissue+is%3Aopen+%22out+of+memory%22),
[LM Studio search](https://api.github.com/search/issues?q=repo%3Almstudio-ai%2Flmstudio-bug-tracker+is%3Aissue+is%3Aopen+memory),
2026-11-06; one example: [lmstudio-bug-tracker#2240](https://github.com/lmstudio-ai/lmstudio-bug-tracker/issues/2240)).
**EZLM today (verified):** `internal/app/app.go:1623` has `cpuInstead`, and `:1641-1653` matches
Vulkan and allocation strings, so the retry exists for one backend and one failure class.

### R04. Store API keys hashed, show them once, and record last use
**Effort:** M. **Area:** `internal/keys`, `internal/serve/config.go`, keys UI.
**Do:** keep only a hash and a short prefix in `config.json`, show the full key at creation and on
explicit reveal, and add a last-used timestamp per key.
**Why:** AnythingLLM's open feature request #6661 asks for exactly this (hash at rest, shown once,
last-used, expiry, scopes) and has 41 sibling open issues matching "api key"
([#6661](https://github.com/Mintplex-Labs/anything-llm/issues/6661),
[search](https://api.github.com/search/issues?q=repo%3Amintplex-labs%2Fanything-llm+is%3Aissue+is%3Aopen+%22api+key%22),
2026-11-06). Ollama shows the opposite extreme: its local server accepts any key value
([Ollama OpenAI compatibility](https://docs.ollama.com/api/openai-compatibility), 2026-11-06).
**EZLM today (verified):** `internal/serve/config.go:37-50` persists `Key` and every `Keys[].Key`
in clear text; `internal/keys/keys.go:9` generates a good 238-bit `ezlm-` key.

### R05. Make LAN mode refuse to run without a key and say what it exposes
**Effort:** S. **Area:** `internal/serve` (`Host`, `LAN`), `internal/web` (host check).
**Do:** when LAN is on, require at least one key, show the exact URL and the warning "anyone on this
network can use your model and see your prompts", and keep a host/origin allowlist.
**Why:** Ollama binds loopback by default and ignores the key on local requests, so tutorials that
flip `OLLAMA_HOST=0.0.0.0` expose an unauthenticated model
([Ollama OpenAI compatibility](https://docs.ollama.com/api/openai-compatibility), 2026-11-06).
LM Studio makes "serve on local network" a deliberate toggle
([LM Studio server docs](https://lmstudio.ai/docs/developer/core/headless), 2026-11-06).
**EZLM today (verified):** `Config.LAN` and `POST /api/lan` exist (`internal/web/server.go:111`);
`keyList` is always passed to llama-server (`internal/serve/config.go:118`), but whether LAN can be
enabled with zero keys is not checked here. T07 owns the security review; this is the product side.

### R06. Let `/v1/models` list downloaded models and load one on demand
**Effort:** L. **Area:** `internal/serve`, `internal/app`.
**Do:** keep the server up across model switches; load the requested model on the first request,
unload the previous one, and list every downloaded model in `/v1/models`.
**Why:** LM Studio's JIT loading is documented as the normal way to use it as a service, and it is
what makes `model: "..."` meaningful to agents
([LM Studio headless docs](https://lmstudio.ai/docs/developer/core/headless), 2026-11-06).
**EZLM today (verified):** the deployed config holds one `ModelID`/`ModelPath` and passes one
`--alias` (`internal/serve/config.go:113-116`), so a model switch restarts the server and the alias
set changes. `ezlm use` exists (`cmd/ezlm/main.go:68`), so the pieces are present.

### R07. Show the real context window and warn before a request is truncated
**Effort:** S. **Area:** `internal/serve`, `internal/app`, chat UI.
**Do:** display the deployed context next to the model name, add a per-request `num_ctx` override,
and put a visible note in the chat when a prompt is cut.
**Why:** Ollama defaults to 4096 tokens and users lose output to silent truncation until they find
`OLLAMA_CONTEXT_LENGTH` ([Ollama FAQ](https://docs.ollama.com/faq), 2026-11-06). EZLM already
computes context per model in the planner, so this is mostly a display and override job.
**EZLM today (verified):** `-c` is always explicit (`internal/serve/config.go:118`); the chat and
status surfaces show the recipe, but a per-request override was not found.

### R08. Pre-flight the download: free disk, size, speed estimate, and a resume that survives a reboot
**Effort:** S. **Area:** `internal/engine` (download), `internal/app`.
**Do:** before starting, check free space on the chosen drive against the exact byte count, show size
and an ETA that uses measured throughput, and make resume work after the app is closed.
**Why:** 128 open GPT4All issues match "download", including #1235 about two default model folders
and a download that Ctrl-C did not stop
([search](https://api.github.com/search/issues?q=repo%3Anomic-ai%2Fgpt4all+is%3Aissue+is%3Aopen+download),
[#1235](https://github.com/nomic-ai/gpt4all/issues/1235), 2026-11-06). EZLM's 2 to 60 GB first
download is its longest and most fragile moment.
**EZLM today (verified):** `fit.Plan` already carries `Download`, `Partial` and `DiskOK`
(`internal/fit/fit.go:46-66`), so the data exists; the UI/CLI check is the gap.

## Tier 2: first-try agent connections (9 to 18)

### R09. Add Claude Code, Codex and Copilot CLI as connect targets
**Effort:** M. **Area:** `internal/integrate` (registry and tool files).
**Do:** treat the three most-used terminal agents as first-class targets, with the same
detect/configure/test/open pipeline as OpenCode, Hermes and DSH.
**Why:** Ollama now ships `ollama launch claude`, `launch codex` and `launch copilot-cli` as its
headline coding flow ([Ollama README](https://raw.githubusercontent.com/ollama/ollama/main/README.md),
2026-11-06). LM Studio documents Codex explicitly
([LM Studio OpenAI compatibility](https://lmstudio.ai/docs/developer/openai-compat), 2026-11-06).
**EZLM today (verified):** `internal/integrate/CONTRACT.md` lists opencode, hermes, dsh, launch and
core workstreams only.

### R10. Implement `POST /v1/responses` for Codex
**Effort:** M. **Area:** `internal/serve` (proxy/shape), `internal/app`.
**Do:** pass Responses API calls through to llama-server or translate them, and cover it with a
contract test.
**Why:** LM Studio says Codex works because it implements `/v1/responses`, alongside chat
completions, embeddings and Anthropic messages
([LM Studio OpenAI compatibility](https://lmstudio.ai/docs/developer/openai-compat), 2026-11-06).
**EZLM today (inferred):** the server is llama-server behind a key; llama.cpp has some Responses
support, but EZLM does not test or advertise it.

### R11. Add an Anthropic-compatible `/v1/messages` shim
**Effort:** M. **Area:** `internal/serve`.
**Do:** translate Anthropic messages to the OpenAI shape llama-server speaks, including tool blocks
and streaming.
**Why:** LM Studio and LocalAI both advertise Anthropic compatibility
([LM Studio docs](https://lmstudio.ai/docs/developer/openai-compat),
[LocalAI README](https://raw.githubusercontent.com/mudler/LocalAI/master/README.md), 2026-11-06).
Claude Code is the agent most likely to be tried first, and it speaks Anthropic.
**EZLM today (inferred):** no `/v1/messages` route appears in `internal/serve` or `internal/web`.

### R12. Make the connection test a saved, visible result
**Effort:** S. **Area:** `internal/bench`, `internal/app`, connection UI.
**Do:** after connecting an app, run the existing bench (speed, tool call, instruction, recall) and
keep the result on the connection card with a timestamp and a re-run button.
**Why:** LM Studio documents tool use and structured output as tested, endpoint-level features
([LM Studio OpenAI compatibility](https://lmstudio.ai/docs/developer/openai-compat), 2026-11-06);
Ollama has a tool-calling capability page ([Ollama docs](https://docs.ollama.com/faq), 2026-11-06).
Users judge a local model by whether their agent can edit a file, not by tok/s.
**EZLM today (verified):** `internal/bench/bench.go` implements those checks and is imported by
`internal/app/app.go:19`, so this is a surfacing job, not a new test harness.

### R13. Put server state in the tray or menu bar
**Effort:** M. **Area:** `windows/EZLM`, `macos/Sources/EZLM` (read-only tonight, recommend).
**Do:** show loaded model, tok/s and a Stop server action from the tray/menu bar, and make closing
the window keep the server up with a clear indicator.
**Why:** Ollama's update flow is "click the taskbar or menubar item, then Restart to update", which
means the tray is the product's control surface
([Ollama FAQ](https://docs.ollama.com/faq), 2026-11-06). LM Studio can run the server on login and
minimise to the tray ([LM Studio headless docs](https://lmstudio.ai/docs/developer/core/headless),
2026-11-06).
**EZLM today (inferred):** `BETA.md` tells users to use Task Manager to disable startup and a Stop
server button in the app; no tray state is described.

### R14. Offer "later" and release notes on update, and separate beta from stable
**Effort:** S. **Area:** `internal/update`, update UI.
**Do:** an update prompt that shows notes and version, allows "later", and remembers a skipped
version; add a stable/beta channel switch.
**Why:** Ollama auto-downloads and then asks for a restart, which is convenient but gives no channel
control ([Ollama FAQ](https://docs.ollama.com/faq), 2026-11-06). LocalAI's unsigned macOS build
needs a manual quarantine command, a reminder that update UX is where trust is won or lost
([LocalAI README](https://raw.githubusercontent.com/mudler/LocalAI/master/README.md), 2026-11-06).
**EZLM today (verified):** `internal/update/update.go:19` reads the ten newest releases, pre-releases
included; `Info` has no channel or skipped-version field.

### R15. Explain the pick with the numbers that produced it
**Effort:** S. **Area:** `internal/fit`, `internal/app` (status/recipe).
**Do:** one panel: "This model: 7B Q4, needs 5.1 GB VRAM and 1.8 GB RAM, expected 22 tok/s; your
card has 8 GB". Link it to the mode (GPU/hybrid/split/CPU).
**Why:** competitors publish simple RAM tiers (Jan: 8 GB for 3B, 16 GB for 7B, 32 GB for 13B) and
users calibrate on them ([Jan README](https://raw.githubusercontent.com/menloresearch/jan/main/README.md),
2026-11-06). EZLM's planner is stronger, so hiding its reasoning wastes the advantage.
**EZLM today (verified):** `fit.Plan` carries `GPUNeed`, `GPUBudget`, `RAMNeed`, `RAMBudget`,
`TokPerSec`, `Speed` and `Effective` (`internal/fit/fit.go:52-62`); `Config.Recipe` stores a
human-readable summary (`internal/serve/config.go:55`).

### R16. Ship a tiny starter model so the first chat works before the big download
**Effort:** M. **Area:** `internal/engine`, `internal/catalog`, installer.
**Do:** bundle (or fetch first, at a few hundred MB) a small instruct model, let chat work
immediately, and upgrade in the background.
**Why:** GPT4All's pitch is "download the application and get started, no GPU required"
([GPT4All README](https://raw.githubusercontent.com/nomic-ai/gpt4all/main/README.md), 2026-11-06);
llamafile ships one small default model with the binary
([llamafile README](https://raw.githubusercontent.com/mozilla-ai/llamafile/main/README.md),
2026-11-06). EZLM's first useful moment currently waits on gigabytes.
**EZLM today (inferred):** `BETA.md` says install downloads "often 2 to 60 GB" before Ready.

### R17. Show the backend and the fallback reason, and keep it in the report
**Effort:** S. **Area:** `internal/app` (fallback), `internal/scan`, report details.
**Do:** label every deployment "CUDA", "Vulkan", "Metal" or "CPU", and when a fallback happened say
why in one sentence. Include it in problem reports.
**Why:** GPT4All's Vulkan path only covers Q4_0 and Q4_1 quantizations, so users silently lose the
GPU on other quants ([GPT4All README](https://raw.githubusercontent.com/nomic-ai/gpt4all/main/README.md),
2026-11-06). LocalAI maintains a per-backend compatibility table because this confuses everyone
([LocalAI README](https://raw.githubusercontent.com/mudler/LocalAI/master/README.md), 2026-11-06).
**EZLM today (verified):** `Config.CPUFallback` holds the reason (`internal/serve/config.go:52`)
and `internal/app/app.go:1623` sets it for Vulkan failures; the UI copy is the gap.

### R18. Publish per-model capability badges, not just a score
**Effort:** M. **Area:** `internal/catalog/catalog.json`, `internal/catalog`, UI.
**Do:** mark each model Tools, Vision, Thinking, 64K context and licence; let the planner prefer a
capability when the user asks for it (for example "I want to use Claude Code" implies tools).
**Why:** Ollama documents thinking, vision, tool calling and structured output as separate
capabilities ([Ollama docs](https://docs.ollama.com/faq), 2026-11-06); LM Studio documents tool use
and structured output as endpoint features
([LM Studio docs](https://lmstudio.ai/docs/developer/openai-compat), 2026-11-06). "Smartest" is not
the same as "works with my agent".
**EZLM today (inferred):** `catalog.json` and `fit` rank by intelligence and speed; Hermes already
has a 64K gate (`internal/integrate/CONTRACT.md` section 2), so the capability concept exists.

## Tier 3: support load and trust (19 to 26)

### R19. One models folder, an "open folder" button, and import an existing GGUF
**Effort:** M. **Area:** `internal/paths` (settings), `internal/app` (settings).
**Do:** keep one canonical models directory, show it, open it in Explorer/Finder, and let a user
point at a GGUF they already have instead of downloading it again.
**Why:** GPT4All's #1235 is exactly this confusion: two default folders and a download the user
could not stop ([#1235](https://github.com/nomic-ai/gpt4all/issues/1235), 2026-11-06).
**EZLM today (verified):** `internal/paths/settings.go:13` has a settable `ModelsDir` and
`internal/app/settings.go:217` scans for `.gguf`, so the pieces exist; discoverability is the gap.

### R20. Expose usage per key: requests, tokens, last used
**Effort:** M. **Area:** `internal/serve` (metrics), `internal/keys`, UI.
**Do:** read llama-server's `--metrics` output per key where possible, and at minimum record last
used and request count.
**Why:** LocalAI advertises per-API-key and per-user usage attribution and quotas
([LocalAI README](https://raw.githubusercontent.com/mudler/LocalAI/master/README.md), 2026-11-06);
AnythingLLM users ask for the same visibility ([#6661](https://github.com/Mintplex-Labs/anything-llm/issues/6661),
2026-11-06). "Which app is using my GPU?" is a normal desktop question.
**EZLM today (verified):** `--metrics` is passed (`internal/serve/config.go:122`), so the data
exists at the engine level.

### R21. Export and import chats and memory
**Effort:** S. **Area:** `internal/app` (chat store), `internal/paths`.
**Do:** a JSON or Markdown export of chats and memory, and an import that merges rather than
overwrites.
**Why:** AnythingLLM treats memories as a first-class, user-managed feature
([AnythingLLM README](https://raw.githubusercontent.com/Mintplex-Labs/anything-llm/master/README.md),
2026-11-06); portability is what makes users trust "it stays on your computer".
**EZLM today (verified):** `BETA.md` documents chats and memory in `%LOCALAPPDATA%\EZLM`; no
export path is described.

### R22. Health and readiness endpoints a script can trust
**Effort:** S. **Area:** `internal/serve` (HTTP surface).
**Do:** `GET /healthz` and `GET /readyz` that report model loaded, context, backend and version
without a key on loopback (or with the key if LAN).
**Why:** LM Studio documents a REST surface with load state and rich stats
([LM Studio REST overview](https://lmstudio.ai/docs/developer/openai-compat), 2026-11-06); Ollama
has `/api/ps` and `/api/version` ([Ollama docs](https://docs.ollama.com/api/openai-compatibility),
2026-11-06). Agents and install scripts need a cheap check.
**EZLM today (verified):** the app has `/api/live` (`internal/web/server.go:220`) and `ezlm status`,
but the OpenAI port itself has only llama-server's endpoints.

### R23. Make problem reports previewable, and say what is not sent
**Effort:** S. **Area:** `internal/report`, report UI.
**Do:** show the exact text before sending, with a clear line for hardware summary and logs, and the
sentence "keys, files and prompts are not included".
**Why:** AnythingLLM documents its telemetry event by event and offers an opt-out
([AnythingLLM README](https://raw.githubusercontent.com/Mintplex-Labs/anything-llm/master/README.md),
2026-11-06). EZLM posts reports publicly, so the preview is the trust feature.
**EZLM today (verified):** `internal/report/report.go` builds a GitHub issue and truncates long
bodies; `BETA.md` promises keys, files and prompts are never included.

### R24. Put the engine's known limits where the user hits them
**Effort:** S. **Area:** `internal/prereq`, `internal/scan`, UI copy.
**Do:** one page per backend: what works, what is slower, what is untested, with the exact symptom
strings ("Running on the processor") and the fix.
**Why:** GPT4All's Vulkan quant limit and LocalAI's backend matrix are the same lesson: users need a
map, not a diagnostic hunt ([GPT4All README](https://raw.githubusercontent.com/nomic-ai/gpt4all/main/README.md),
[LocalAI README](https://raw.githubusercontent.com/mudler/LocalAI/master/README.md), 2026-11-06).
a11 covers the platform detail; this is the product placement.
**EZLM today (inferred):** `internal/prereq/prereq.go` already produces warn/action strings for old
CUDA drivers; there is no single limits page.

### R25. Document headless and server-only use, and install it as a service
**Effort:** S. **Area:** `cmd/ezlm` (`run-server`), `internal/serve` (service files), docs.
**Do:** a short page: `ezlm run-server`, how to install it as a login service on each OS, how to
stop it, and what the URL/key are.
**Why:** LM Studio ships `llmster` and documents startup tasks; Ollama is a background service by
default ([LM Studio headless](https://lmstudio.ai/docs/developer/core/headless),
[Ollama README](https://raw.githubusercontent.com/ollama/ollama/main/README.md), 2026-11-06).
**EZLM today (verified):** `ezlm run-server` exists (`cmd/ezlm/main.go:188`), and Windows uses a
scheduled task (`internal/serve/run.go:11-14`), but the beta guide does not mention it.

### R26. Keep the previous version and verify the digest before running the installer
**Effort:** M. **Area:** `internal/update`, installer.
**Do:** verify `Release.Digest` before launch, keep the last installer for rollback, and offer
"go back" if the new version fails to start twice.
**Why:** update failures are the most expensive kind for a beta: LocalAI's macOS DMG is unsigned and
needs a quarantine command, and users hit it every release
([LocalAI README](https://raw.githubusercontent.com/mudler/LocalAI/master/README.md), 2026-11-06).
**EZLM today (verified):** `Release.Digest` is parsed (`internal/update/update.go:31`) and BETA.md
says the update "checks it"; rollback is not described. T08 owns the hardening.

## Tier 4: bigger bets (27 to 31)

### R27. Folder-level local documents with citations
**Effort:** L. **Area:** `internal/app` (chat), `internal/web`, new index package.
**Do:** point at a folder, index text/PDF/DOCX, and answer with source citations in chat.
**Why:** GPT4All's LocalDocs and AnythingLLM's document pipeline are the two most-copied features in
this category ([GPT4All README](https://raw.githubusercontent.com/nomic-ai/gpt4all/main/README.md),
[AnythingLLM README](https://raw.githubusercontent.com/Mintplex-Labs/anything-llm/master/README.md),
2026-11-06). EZLM currently attaches one text file per message (`BETA.md`).
**EZLM today (verified):** `BETA.md` describes "+ file (or drag a file in) attaches a text file".

### R28. MCP client support for chat and agents
**Effort:** L. **Area:** `internal/integrate`, `internal/app`.
**Do:** a small MCP settings screen that lists servers and tools, plus a way for chat to call them.
**Why:** Jan lists MCP as a headline feature, AnythingLLM documents MCP compatibility, and LocalAI
has MCP support ([Jan README](https://raw.githubusercontent.com/menloresearch/jan/main/README.md),
[AnythingLLM README](https://raw.githubusercontent.com/Mintplex-Labs/anything-llm/master/README.md),
[LocalAI README](https://raw.githubusercontent.com/mudler/LocalAI/master/README.md), 2026-11-06).
**EZLM today (inferred):** no MCP mention in `BETA.md` or `internal/integrate/CONTRACT.md`.

### R29. Offer a smaller-download mode at first run
**Effort:** M. **Area:** `internal/fit`, `internal/catalog`, first-run UI.
**Do:** a visible "smaller download (about 1 GB)" choice next to Balanced/Smartest/Fastest, with the
quality trade-off in one line.
**Why:** Jan's guidance is organised by RAM tier, and llamafile's default is its smallest model,
because download size is the first blocker on a laptop
([Jan README](https://raw.githubusercontent.com/menloresearch/jan/main/README.md),
[llamafile README](https://raw.githubusercontent.com/mozilla-ai/llamafile/main/README.md),
2026-11-06).
**EZLM today (verified):** the three prefs are Balanced, Smartest and Fastest
(`internal/fit/fit.go:69-73`); none of them is framed as download size.

### R30. Make the OpenAI surface honest: embeddings, structured output, tool streaming
**Effort:** M. **Area:** `internal/serve` (contract tests), docs.
**Do:** a compatibility table on the website and in-app: which endpoints work, which are partial,
which are untested, each with a test.
**Why:** LM Studio lists exactly which endpoints it supports and warns that clients differ
([LM Studio OpenAI compatibility](https://lmstudio.ai/docs/developer/openai-compat), 2026-11-06);
Ollama calls its support "a subset" ([Ollama OpenAI compatibility](https://docs.ollama.com/api/openai-compatibility),
2026-11-06). Users find the gaps one 400 at a time.
**EZLM today (inferred):** EZLM proxies llama-server; no endpoint matrix is published.

### R31. Signed builds, or a guided Smart App Control path
**Effort:** L. **Area:** `windows/installer`, `macos` packaging, `BETA.md`.
**Do:** sign and notarise, or detect Smart App Control and show the exact steps before the installer
is blocked with no "Run anyway" button.
**Why:** LocalAI's macOS DMG is unsigned and documents the quarantine command as a required step
([LocalAI README](https://raw.githubusercontent.com/mudler/LocalAI/master/README.md), 2026-11-06);
EZLM's own `BETA.md` says Smart App Control blocks it outright. Every unsigned beta loses a slice of
users at this step.
**EZLM today (verified):** `BETA.md` "Known limits" describes the blocker; there is no detection.

## Quick index

| Rank | Item | Effort | Area |
|---|---|---|---|
| 1 | Idle unload | M | `internal/serve` |
| 2 | Arch-aware update asset | S | `internal/update` |
| 3 | Runtime OOM retry | M | `internal/app`, `internal/fit` |
| 4 | Hashed keys, shown once | M | `internal/keys`, `internal/serve` |
| 5 | LAN key guardrails | S | `internal/serve`, `internal/web` |
| 6 | JIT model loading | L | `internal/serve`, `internal/app` |
| 7 | Context truth | S | `internal/serve`, UI |
| 8 | Download pre-flight | S | `internal/engine` |
| 9 | Claude Code/Codex/Copilot targets | M | `internal/integrate` |
| 10 | `/v1/responses` | M | `internal/serve` |
| 11 | `/v1/messages` | M | `internal/serve` |
| 12 | Saved connection test | S | `internal/bench`, `internal/app` |
| 13 | Tray/menu bar state | M | `windows/EZLM`, `macos` |
| 14 | Update prompt and channel | S | `internal/update` |
| 15 | Explain the pick | S | `internal/fit` |
| 16 | Starter model | M | `internal/engine`, `internal/catalog` |
| 17 | Backend and fallback reason | S | `internal/app`, `internal/scan` |
| 18 | Capability badges | M | `internal/catalog` |
| 19 | One models folder and import | M | `internal/paths`, `internal/app` |
| 20 | Per-key usage | M | `internal/serve`, `internal/keys` |
| 21 | Chat/memory export | S | `internal/app` |
| 22 | Health endpoints | S | `internal/serve` |
| 23 | Report preview | S | `internal/report` |
| 24 | Backend limits page | S | `internal/prereq` |
| 25 | Headless docs and service | S | `cmd/ezlm`, `internal/serve` |
| 26 | Digest check and rollback | M | `internal/update` |
| 27 | Folder documents with citations | L | `internal/app`, `internal/web` |
| 28 | MCP client | L | `internal/integrate`, `internal/app` |
| 29 | Smaller-download mode | M | `internal/fit` |
| 30 | OpenAI surface matrix | M | `internal/serve` |
| 31 | Signed builds | L | installer, `BETA.md` |
