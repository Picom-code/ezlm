# User complaints across the local-LLM field, and where EZLM stands

Agent a15, task T14. Desk study, no competitor installed. All URLs accessed 2026-11-06.

## How the counts were made

Two kinds of number appear below, and they are not comparable with each other:

1. **Reactions and comments on one issue.** Read from the GitHub REST API
   (`/search/issues`), field `reactions.total_count` and `comments`. This is the
   closest public proxy for "how many people cared".
2. **Issues matching a phrase in the title.** Also from `/search/issues`, query
   `repo:<owner>/<repo> is:issue <phrase> in:title`, field `total_count`. These
   counts include open and closed issues and are a proxy for *recurrence*, not
   severity. A phrase like "out of memory" undercounts because many reports say
   "OOM", "killed", "CUDA error" or "crash" instead.

Limits, stated up front:

- Reddit blocked this machine (HTTP 403 on `reddit.com/*.json`), so no Reddit
  upvote counts are used. Forum evidence is therefore thin except for Msty.
- GitHub issue search is not a survey. Silent users, Discord and Discord-like
  communities are invisible. Treat every count as a floor.
- Msty is closed source and has no public issue tracker that resolved on
  2026-11-06 (`community.msty.app` did not resolve, `msty.canny.io` returned
  404). Its section is the weakest in this file and is marked as such.

## Summary: the complaint themes that repeat across products

| Theme | Ollama | LM Studio | Jan | GPT4All | AnythingLLM | llamafile | LocalAI | Msty |
|---|---|---|---|---|---|---|---|---|
| Out of memory / model will not load | 73 "out of memory", 32 "OOM" in title | 36 "failed to load" in title | crash reports, no clean count | 51 open `bug` | n/a | n/a | n/a | n/a |
| Context length surprise / truncation | 51 "context length" in title | present, no clean count | present | #1638 | n/a | #630 | n/a | n/a |
| GPU not detected / wrong backend | 32 "not detected" in title | 41 "ROCm" in title | present | 105 "GPU" in title | n/a | 45 "GPU" in title | docker GPU setup | n/a |
| Download stalls, no resume | present | present | 119 "download" in title | #1228 | present | n/a | #12893 | n/a |
| Install / update / signing friction | Windows install + PATH | macOS Gatekeeper, Linux absent | Flatpak crashes | #3326 installer | Docker complexity | Windows Defender | Docker tags | n/a |
| Paywall / closed source | n/a (open) | free-but-closed | open | open | open core | open | open core | free tier, $149/yr |

"n/a" means the product does not have that failure mode in the same form (for
example, AnythingLLM is a RAG workspace, not a model runner), or no defensible
count was found in the time box. It does not mean "no complaints".

## Ollama

Open issues at the time of the query: 2,637. Highest-reacted *bug*-labelled
issues: [#3185](https://github.com/ollama/ollama/issues/3185) "doesn't distribute
notice licenses in its release artifacts" (297 reactions, 59 comments) and
[#5245](https://github.com/ollama/ollama/issues/5245) "Allow importing multi-file
GGUF models" (221 reactions, 110 comments). Both are packaging/licensing
complaints rather than runtime ones, which fits Ollama's position: it is the
default, so its complaints are about control and correctness at the edges.

The runtime complaints that matter for EZLM:

- **Out of memory and crashes instead of an error.** 73 issues with "out of
  memory" and 32 with "OOM" in the title. Example:
  [#17089](https://github.com/ollama/ollama/issues/17089) "Ollama crashes on CUDA
  out of memory instead of returning error" (open). Ollama loads what the user
  asked for and lets the driver decide; there is no planner that refuses a
  model that cannot fit.
- **Context length is a trap.** 51 issues with "context length" in the title,
  including [#9323](https://github.com/ollama/ollama/issues/9323) "automatically
  set context length to model context length" and
  [#9774](https://github.com/ollama/ollama/issues/9774) "Estimate of VRAM needs
  based on context length and quantization". Users discover after the fact that
  the default window silently truncates prompts.
- **GPU detection gaps.** 32 issues with "not detected" in the title, for
  example Intel Iris Xe under Vulkan
  ([#18482](https://github.com/ollama/ollama/issues/18482)) and Jetson
  ([#18067](https://github.com/ollama/ollama/issues/18067)). AMD NPU support is
  the 8th most-reacted open issue overall
  ([#5186](https://github.com/ollama/ollama/issues/5186), 168 reactions).
- **Home-directory clutter.** [#228](https://github.com/ollama/ollama/issues/228)
  "Please don't clutter the user home directory" has 192 reactions and 36
  comments, one of the highest-reacted issues in the repo. Ollama's model store
  is not where users expect it and is hard to move.
- **No authentication by default.** Ollama binds to localhost but exposes an
  unauthenticated API; every tutorial that sets `OLLAMA_HOST=0.0.0.0` puts an
  open model endpoint on the LAN. This is a known footgun rather than a tracked
  bug, so it has no count here.

EZLM verdict: **avoids** the memory and context classes by construction (see
below), and **avoids** the auth footgun (every server gets a key). It **shares**
the home-directory concern in miniature: it writes `~/.ezlm/config.json` on
macOS/Linux (Windows uses `%LOCALAPPDATA%`), and the models folder is
user-selectable, so the big bytes are movable.

## LM Studio

Open issues in the bug tracker: 2,040. The tracker is mostly feature requests,
so reactions concentrate on platform gaps rather than crashes:

- [#9](https://github.com/lmstudio-ai/lmstudio-bug-tracker/issues/9) "Please add
  support for Intel Mac" (508 reactions, 444 comments) is the single largest
  thread in the tracker. Intel Mac users are still waiting.
- [#656](https://github.com/lmstudio-ai/lmstudio-bug-tracker/issues/656) "Avoid
  LM Studio to change .bashrc and .zshrc" (56 reactions). The installer edits
  shell profiles without asking.
- [#784](https://github.com/lmstudio-ai/lmstudio-bug-tracker/issues/784) Linux
  packaging / Flathub (57 reactions). No first-class Linux build.
- [#1394](https://github.com/lmstudio-ai/lmstudio-bug-tracker/issues/1394)
  "Update ROCm" (30 reactions, 29 comments) is the top *bug*-labelled issue:
  AMD users are stuck on an old ROCm runtime.
- **Failed to load** is the most common load complaint: 36 issues with "failed to
  load" in the title, e.g.
  [#688](https://github.com/lmstudio-ai/lmstudio-bug-tracker/issues/688)
  (23 comments). "Out of memory" in the title is rarer (3) because LM Studio
  estimates and warns, but users still hit it with large contexts.
- **ROCm** appears in 41 issue titles. This is the clearest AMD pain signal in
  the field.

EZLM verdict: **avoids** the Intel-Mac gap only in the sense that EZLM does not
target Intel Macs at all (Apple Silicon only, per `BETA.md`), and **shares** the
"no Linux build" gap. EZLM **avoids** the shell-profile edit (installer does not
touch `.bashrc`) and **avoids** the failed-load class with an explicit planner.
It **shares** the unsigned-app/Gatekeeper friction on macOS.

## Jan

Open issues: 565 by repo metadata, 396 returned by the issue search. The
highest-reacted open bug is about downloads:

- [#7728](https://github.com/janhq/jan/issues/7728) "bug: Jan does not download
  models in version v0.7.8" (10 reactions, 32 comments). Top bug by reactions.
- [#7066](https://github.com/janhq/jan/issues/7066) "bug: Jan should resume model
  downloads when online" and
  [#8303](https://github.com/janhq/jan/issues/8303) "Stuck download". 119 issues
  have "download" in the title.
- Crashes: 45 issues with "crash" in the title, including
  [#9173](https://github.com/janhq/jan/issues/9173) "0.8.5 Loading models results
  in a hard crash" and Flatpak crashes
  ([#9286](https://github.com/janhq/jan/issues/9286),
  [#8873](https://github.com/janhq/jan/issues/8873)).
- The bug tracker is thin on reactions overall (top bug has 10), which is itself
  a signal: Jan's community reports in Discord, not GitHub.

EZLM verdict: **avoids** the download class with HTTP range resume and stall
detection (`internal/engine/download.go:130`, tests
`download_stall_test.go`, `download_neterr_test.go`). EZLM **shares** the
"unsigned app" and "large first download" friction, and has no Linux build,
where Jan at least ships Flatpak (with crashes).

## GPT4All

Open issues: 733; 51 open `bug`-labelled. The tracker is ageing, and users say so:

- [#3708](https://github.com/nomic-ai/gpt4all/issues/3708) "Use Unsloth -
  GPT4All is no longer maintained" (open). 97 issues match "abandoned",
  "maintenance" or "no longer" in the title.
- [#2768](https://github.com/nomic-ai/gpt4all/issues/2768) "Vulkan:
  Meta-Llama-3.1-8b-128k slow generation" (15 comments). 22 issues with "slow"
  in the title.
- [#1803](https://github.com/nomic-ai/gpt4all/issues/1803) "Model loaded into GPU
  ram but 0% GPU usage" is the classic backend-selection failure; 105 issues have
  "GPU" in the title.
- [#1228](https://github.com/nomic-ai/gpt4all/issues/1228) "Model download
  mechanism needs some improvement" (4 reactions).
- [#3326](https://github.com/nomic-ai/gpt4all/issues/3326) "Installer no longer
  works as updater".

EZLM verdict: **avoids** the "no GPU usage" class by detecting the backend and
recording a CPU fallback reason (`internal/serve/config.go`, `CPUFallback`), and
**avoids** the update-by-installer bug with an in-app updater
(`internal/update/update.go:56`). EZLM **shares** the "one big first download"
complaint and has no Linux build, while GPT4All at least publishes Linux builds.

## AnythingLLM

Open issues: 318. The tracker is dominated by feature requests; only one open
issue carries the plain `bug` label, so complaint counts here are weak.

- [#3087](https://github.com/Mintplex-Labs/anything-llm/issues/3087) "Specify
  `storage` folder location on installation flow" (24 reactions, Windows
  desktop). Users cannot choose where the vector store and documents live.
- [#7039](https://github.com/Mintplex-Labs/anything-llm/issues/7039) "division by
  zero panic in lance-encoding during RAG queries".
- Docker is the recurring deployment pain: 164 issues with "docker" in the title,
  mostly setup and persistence requests, e.g.
  [#6903](https://github.com/Mintplex-Labs/anything-llm/issues/6903) "Persist
  Application Configuration Across Docker Container Recreations".
- Embeddings are the second theme: 103 issues with "embedding" in the title,
  including Hugging Face compatibility
  ([#3943](https://github.com/Mintplex-Labs/anything-llm/issues/3943)) and
  document/embedding desync ([#6998](https://github.com/Mintplex-Labs/anything-llm/issues/6998)).

EZLM verdict: not a direct competitor (workspace + RAG vs. one-click runner), so
"shares" is limited. The transferable lesson is **storage location**: AnythingLLM
users want to choose where data lives, and EZLM already does that for models
(`Settings` models folder, `BETA.md`). EZLM does not yet offer the same choice
for chat history and memory, which live under `%LOCALAPPDATA%\EZLM` or
`~/Library/Application Support/EZLM`.

## llamafile

Open issues: 212; 107 open `bug`-labelled. Complaints are concentrated on
Windows and GPUs:

- Windows and WSL: 48 issues with "windows" in the title.
  [#356](https://github.com/mozilla-ai/llamafile/issues/356) "All Sorts of Issues
  Executing (WSL and Windows)" (17 comments).
- GPU: 45 issues with "GPU" in the title.
  [#416](https://github.com/mozilla-ai/llamafile/issues/416) "Can't run on AMD
  GPU, while llama.cpp does";
  [#965](https://github.com/mozilla-ai/llamafile/issues/965) "No usable GPU
  found, --gpu-layers option will be ignored but AMD ...";
  [#1048](https://github.com/mozilla-ai/llamafile/issues/1048) NVIDIA mobile GPU
  failures.
- Windows-specific runtime bugs:
  [#1083](https://github.com/mozilla-ai/llamafile/issues/1083) Vulkan crash at
  startup with an offloaded multimodal projector;
  [#1084](https://github.com/mozilla-ai/llamafile/issues/1084) exit codes
  multiplied by 256.
- Antivirus: 14 issues match "defender", "antivirus" or "false positive".
  [#15](https://github.com/mozilla-ai/llamafile/issues/15) "Security" (5
  reactions) is the umbrella thread. A single-file executable that unpacks itself
  is heuristically suspicious; this is the price of the one-file design.
- Prompt formatting: [#630](https://github.com/mozilla-ai/llamafile/issues/630)
  "Why llamafile don't remove end token like `<|eot_id|>`" (4 reactions).

EZLM verdict: **shares** the Windows-unsigned/antivirus class (EZLM is not
code-signed, `BETA.md`), and **shares** the AMD/Intel Vulkan uncertainty. EZLM
**avoids** the "no model management" problem because it has a catalog and a
planner; llamafile users pick a file and a `--gpu-layers` count themselves.

## LocalAI

Open issues: 181; 42 open `bug`-labelled. The recurring theme is deployment
complexity, not model quality:

- Docker and backends: 85 issues with "docker" in the title.
  [#9162](https://github.com/mudler/LocalAI/issues/9162) "install python based
  backends in v4.0.0 nvidia-12 docker image not successful";
  [#9138](https://github.com/mudler/LocalAI/issues/9138) FlashAttention2 missing
  from hipblas images.
- Model lifecycle: [#12562](https://github.com/mudler/LocalAI/issues/12562)
  "4.11.0 can't delete models while downloading other models" (10 comments);
  [#12893](https://github.com/mudler/LocalAI/issues/12893) "Backend installation
  fails with 'mkdir //downloading: permission denied'".
- Gallery mismatch: [#12081](https://github.com/mudler/LocalAI/issues/12081)
  "localai cannot run the recommended model"; the gallery recommends models no
  backend supports
  ([#11681](https://github.com/mudler/LocalAI/issues/11681)).
- Release regressions: [#8108](https://github.com/mudler/LocalAI/issues/8108)
  "Release today (just released) breaking when launching on CUDA hardware".

EZLM verdict: **avoids** the Docker/backend-install class entirely (native
installer, one engine build chosen by the scanner). EZLM **shares** the
"gallery recommends something that does not run" risk in principle; its defense
is the fit planner, which only offers plans whose memory budgets pass, and a
download that resumes. That defense is only as good as the hardware table, which
is where AMD/Intel users are least tested (`BETA.md` asks for reports).

## Msty (weakest evidence)

Closed source, no public issue tracker. Sources that did resolve on 2026-11-06:
the [pricing page](https://msty.ai/pricing/) and
[docs onboarding](https://docs.msty.app/getting-started/onboarding). The pricing
page shows a free tier, Aurum at USD 149/user/year and a USD 349 lifetime
licence. Complaints visible from the product shape, not from counted threads:

- **Paywall on the interesting features.** Web access, Azure/Bedrock providers,
  assistants and "power-user features" are Aurum-only. Users who arrive for a
  free local chat and want the workspace features hit the licence.
- **Closed source.** No way to audit what leaves the machine, which is exactly
  the anxiety a local-LLM buyer has.
- **Linux.** No Linux build was advertised on the pages read.

No reaction or comment counts are claimed for Msty. Treat this section as
qualitative. A follow-up with Reddit or the Msty Discord would be needed to rank
these against the others.

## What EZLM already avoids, by design

These are the strongest differentiators found in this pass, each with the code
that backs it:

1. **Memory fit is decided before download, with a safety margin.** The planner
   enumerates quant × context × KV type × placement and keeps an OS floor
   (`internal/fit/fit.go:1-8`, `ComputeBudgets` at `:171`, `margin` at `:198`).
   This is the direct answer to Ollama's 73 "out of memory" and LM Studio's 36
   "failed to load" titles. Caveat: it is a model of the machine, not a
   measurement, and the margin constants are only as good as the hardware table.
2. **The context window is part of the plan, not a default.** `Plan.Ctx` is
   chosen per model and passed explicitly as `-c`
   (`internal/serve/config.go:122`), and `BETA.md` warns when a model cannot
   give Hermes 64K without slowing down. Ollama's 51 "context length" issues
   are the counter-example.
3. **Every server has a key, and LAN is opt-in.** `keys.New()` returns
   `ezlm-` plus 40 base62 characters (`internal/keys/keys.go:12-24`), passed as
   `--api-key` (`internal/serve/config.go:121`); the default bind is
   `127.0.0.1` and `0.0.0.0` only when `LAN` is set (`:93-97`). Ollama's
   unauthenticated API is the thing EZLM does not reproduce.
4. **Downloads resume.** Range requests plus stall detection
   (`internal/engine/download.go:130`) with tests for both the stall and the
   network-error path. Jan's top bug (#7728, 32 comments) and GPT4All's #1228
   are exactly this.
5. **Windows on Arm is a first-class build.** The engine picks
   `win-...-arm64` artifacts and CUDA 13 on arm64
   (`internal/engine/engine.go:111`, `:218`). LM Studio's largest thread is
   still Intel Mac support; GPT4All's Arm story is Linux-only in practice.
6. **In-app update that keeps the model.** `update.Check` / `update.Download`
   (`internal/update/update.go:56`, `:111`), one click per `BETA.md`. GPT4All's
   #3326 "installer no longer works as updater" is the failure it avoids.

## What EZLM shares, and should expect to be complained about

1. **Unsigned binaries.** Windows SmartScreen and macOS Gatekeeper, and Windows
   11 Smart App Control blocking outright (`BETA.md`, "Known limits"). Every
   desktop product in this list pays this tax; llamafile pays it twice because
   the binary also looks like malware to antivirus (14 Defender-related issues).
   This is the most likely first-run complaint for EZLM's beta.
2. **A large first download on slow internet.** Called out in `BETA.md`. Jan and
   GPT4All show that resume alone does not stop the complaint; progress
   visibility and a size estimate up front do more.
3. **AMD and Intel GPU rough edges.** EZLM's answer is Vulkan plus a CPU
   fallback, and `BETA.md` asks AMD/Intel users to report. LM Studio has 41
   ROCm titles and 30 reactions on "Update ROCm"; llamafile has 45 GPU titles.
   EZLM has no ROCm path, so its Vulkan result on Radeon is the whole story.
4. **No Linux build.** LM Studio (57 reactions), GPT4All, and Msty all leave
   Linux users to workarounds. EZLM is Windows and macOS only, so it inherits
   the same request. It is a scope choice, not a bug, but it will be asked for.
5. **Storage location for data, not just models.** AnythingLLM's #3087 (24
   reactions) is the same request EZLM will get for chat history and memory once
   users have more than a few chats.

## Recommendations for EZLM (complaint-driven)

| # | Recommendation | Evidence | Effort | Priority |
|---|---|---|---|---|
| 1 | Sign the Windows installer and notarize the macOS app before public beta. It is the single most common first-run blocker class in this field and EZLM currently has no answer. | `BETA.md` known limits; llamafile #15; general SmartScreen friction | M | P0 |
| 2 | Show the chosen plan's numbers in the UI ("needs X GB VRAM / Y GB RAM, Z GB free"), and refuse to start a plan that no longer fits at launch. Preempts the OOM/failed-to-load complaint instead of explaining it after. | Ollama 73 "out of memory"; LM Studio 36 "failed to load"; `internal/fit/fit.go` | S | P0 |
| 3 | Surface download size and resumable progress before the first byte, with a checksum after resume. EZLM already resumes; make it visible. | Jan #7728 (32 comments), #7066; GPT4All #1228 | S | P1 |
| 4 | Publish a tested AMD/Intel Vulkan matrix (which cards, which driver, what speed) and make the CPU-fallback reason visible on the Overview page. | LM Studio 41 ROCm titles; llamafile #416, #965; `BETA.md` | M | P1 |
| 5 | Let users move chat history and memory, not only models, and say where they are. | AnythingLLM #3087 (24 reactions); `BETA.md` storage section | S | P2 |

## Open questions

- Do Ollama's OOM reports cluster on models larger than the machine (user error)
  or on models the tool itself suggested (tool error)? A sample of 20 issues
  would settle whether EZLM's planner advantage is real or theoretical.
- Msty's real complaint ranking needs Reddit or Discord access, which this
  machine did not have.
- The "reactions" proxy over-weights feature requests. A cleaner study would
  filter to issues closed as "not planned" or labelled `bug` and sample titles.
