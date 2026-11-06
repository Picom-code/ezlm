# T14 a15 report: user complaints mining

## Scope

Mined public complaints for the eight products in the T14 brief (LM Studio,
Ollama, Jan, GPT4All, Msty, AnythingLLM, llamafile, LocalAI), ranked them by
public counts where a count exists, and mapped each class against EZLM's code to
say whether EZLM avoids or shares it.

Deliverable: `agents/T14/a15/complaints.md`.

## Method

- GitHub REST search API, unauthenticated, GET only:
  - `repo:<r> is:issue <phrase> in:title` for recurrence counts (`total_count`);
  - `sort=reactions&order=desc` for the most-reacted issues;
  - `reactions.total_count` and `comments` per issue.
- Repos used: `ollama/ollama`, `lmstudio-ai/lmstudio-bug-tracker`, `janhq/jan`,
  `nomic-ai/gpt4all`, `Mintplex-Labs/anything-llm`, `mozilla-ai/llamafile`
  (the repo moved from `Mozilla-Ocho/llamafile`), `mudler/LocalAI`.
- Msty: `msty.ai/pricing/`, `docs.msty.app/getting-started/onboarding`.
- EZLM side: read `internal/fit`, `internal/serve`, `internal/keys`,
  `internal/engine`, `internal/scan`, `internal/update`, `BETA.md`.
- All URLs accessed 2026-11-06.

## Counts and their limits

- The title-phrase counts are all-states (open + closed) and undercount, because
  the same failure is reported under many words (OOM / killed / CUDA error).
- Reactions over-weight feature requests. In Ollama the top three reacted
  issues are features, not bugs; the file separates the two.
- Reddit returned HTTP 403 to this machine, so no Reddit upvote counts are used.
  This is the main gap; the brief expected Reddit and forum threads.
- Msty has no public tracker; its section is qualitative and says so.

## What was verified

Every product claim in `complaints.md` has an issue number and URL, plus a count
from the API or a stated absence of one. EZLM claims cite file:line, checked by
reading the file:

- `internal/fit/fit.go:1-8`, `:171` (ComputeBudgets), `:198` (margin)
- `internal/keys/keys.go:12-24`
- `internal/serve/config.go:93-97` (bind), `:121` (`--api-key`), `:122` (`-c`)
- `internal/engine/download.go:130` (Range resume)
- `internal/engine/engine.go:111`, `:218` (arm64 artifacts, CUDA 13 on arm64)
- `internal/update/update.go:56`, `:111`
- `BETA.md` known limits

## Findings that matter most

1. **The memory-fit complaint is the field's biggest recurring runtime pain**
   (Ollama 73 "out of memory" titles, 32 "OOM"; LM Studio 36 "failed to load";
   GPT4All 51 open bugs). EZLM's planner is the strongest structural answer, but
   it is unproven on AMD/Intel and on Windows on Arm.
2. **Downloads are the biggest single-issue complaint** (Jan #7728 is the top
   bug by reactions, 32 comments). EZLM already resumes; the gap is making
   progress and size visible.
3. **Signing is the most likely first-run complaint for EZLM's beta**, because
   it shares the unsigned-binary tax with llamafile (14 Defender-related issues)
   and LM Studio/macOS.
4. **AMD/ROCm is the clearest platform gap in the field** (LM Studio 41 ROCm
   titles; llamafile 45 GPU titles). EZLM has no ROCm path; Vulkan is the whole
   answer, so its Vulkan results are the risk.
5. **The "no Linux build" request is universal** and EZLM will get it too.

## Not verified / could not do

- No Reddit, Discord, or forum counts (Reddit 403; Msty community DNS failed).
- Msty complaint ranking is not counted.
- No sampling of issue bodies, so "out of memory" is not split into user error
  vs. tool-suggested model. Flagged as an open question in the deliverable.
- AnythingLLM's `bug` label is barely used (1 open issue), so its counts are
  weak; the file relies on feature-request reactions there.

## Recommendations handed to a10

The five in `complaints.md` (signing, visible plan numbers, visible resumable
download, Vulkan matrix, movable chat storage) are ranked P0/P0/P1/P1/P2 with
effort S/M/L and evidence links, ready to merge into `recommendations.md`.

## Open questions

- Does Ollama's OOM cluster on user-chosen oversized models or on suggested
  ones? A 20-issue sample would tell whether EZLM's planner advantage is real.
- Would a signed-but-unnotarized macOS build still trip Gatekeeper for beta
  users? Not testable here.
