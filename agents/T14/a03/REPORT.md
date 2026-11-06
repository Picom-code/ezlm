# T14 a03 REPORT: Jan

Agent a03, product Jan. Docs only; no code changes. Branch `agent/T14-a03`.
All sources accessed 2026-11-06.

## What I read

- Jan release notes 0.8.5 to 0.8.8, from the GitHub releases API
  (https://api.github.com/repos/janhq/jan/releases?per_page=6) and the tag pages
  (https://github.com/janhq/jan/releases/tag/v0.8.8 and .../v0.8.7, .../v0.8.6, .../v0.8.5).
- Jan docs, fetched as markdown from the docs site's `/llms.mdx/.../content.md` export:
  quickstart (https://docs.jan.ai/desktop/quickstart), local API server
  (https://docs.jan.ai/desktop/api-server), model management
  (https://docs.jan.ai/desktop/manage-models), Windows install
  (https://docs.jan.ai/desktop/install/windows), troubleshooting
  (https://docs.jan.ai/desktop/troubleshooting), llama.cpp engine
  (https://docs.jan.ai/desktop/local-engine/llama-cpp), settings
  (https://docs.jan.ai/desktop/settings).
- Repo README, https://raw.githubusercontent.com/janhq/jan/main/README.md.
- GitHub issue data via the search and issue APIs, 2026-11-06: 396 open, 3,703 closed issues;
  counts per term (`appimage` 163, `AMD OR ROCm` 217, `vulkan` 167, `"failed to load" model` 77)
  and four issues read in full (#7728, #5367, #8145, #9137).
- Hacker News via the Algolia API (https://hn.algolia.com/api/v1/search?query=jan.ai) for
  reception counts only.
- EZLM code read for comparison: `internal/serve/config.go`, `internal/keys/keys.go`,
  `internal/fit/fit.go`, `internal/update/update.go`.

## Findings

1. **EZLM's LAN server accepts any Host header.** `Config.LAN` (`internal/serve/config.go:34`)
   only switches the bind address (`Config.Host`, used at `internal/serve/config.go:119`); nothing
   checks the `Host` or `Origin` header. Jan had exactly this hole and fixed it with Trusted Hosts
   in v0.8.5 after GHSA-x6p8-7cp8-c3p6 (https://github.com/janhq/jan/releases/tag/v0.8.5).
   Severity: major. Not proven with a test here (docs-only task), but the code path is one grep:
   `grep -n "Host\|Origin" internal/serve/*.go` finds no request-side check.

2. **EZLM already beats Jan on API keys.** EZLM always passes `--api-key` to llama-server
   (`internal/serve/config.go:121`) with a generated `ezlm-` + 40 base62 key
   (`internal/keys/keys.go:12`). Jan's key is optional and empty by default
   (https://docs.jan.ai/desktop/api-server), so a default Jan install has an open local endpoint.
   Severity: minor, and in EZLM's favour. Recommendation R3 says: keep it, and show it.

3. **Jan's fit pill is a static estimate, EZLM's fit engine is a planner.** Jan's Hub marks each
   model Fits / May be slow / Won't fit with "no data ... downloaded to determine fit status"
   (https://docs.jan.ai/desktop/manage-models). EZLM's `internal/fit/fit.go` enumerates
   quant × context × KV cache × placement and ranks by effective intelligence with a hard memory
   margin. Severity: minor for EZLM, but it is the best UX story EZLM has and it is not surfaced
   before the download.

4. **Jan's update failure is silent and still open.** #9137, https://github.com/janhq/jan/issues/9137,
   filed during 0.8.5 QA, listed under Known issues in 0.8.8. The dialog closes, the reason is
   only in the log, no retry. Severity: major for Jan. EZLM relevance: check how
   `internal/update/update.go` surfaces an error before claiming EZLM is better.

5. **Jan ships a guided first-run wizard only since v0.8.5, and its docs still say otherwise.**
   v0.8.5 notes describe welcome, engine setup, first model, consent; the quickstart page still
   says the default model downloads automatically on first launch. Severity: minor, docs drift.
   Lesson for EZLM: a one-click deploy still needs to say what it is about to download and how big
   it is.

6. **Jan's bundled-engine model has a real cost.** Since v0.8.5 the engine ships in the installer
   and updates with the app; the 0.8.8 installers are 116 MB (macOS universal), 572 MB (Windows
   x64) and 1.06 GB (Linux .deb). Severity: minor. EZLM downloads a pinned engine, which is
   cheaper to update but has more version states to support.

7. **Platform support is narrower than the download page suggests.** Windows on Arm has an ARM64
   installer (`Jan_0.8.8_arm64-setup.exe`, 647 downloads vs 28,833 x64) but the docs only name
   NVIDIA, AMD and Intel Arc GPUs, all x86 vendors. Intel Macs lost local models in v0.8.0. AMD
   gets Vulkan only; HIP requires a separate llama-server. Severity: minor, useful expectation
   setting for EZLM's platform matrix.

## What I changed

Only `agents/T14/products/a03.md` (new) and this report. No EZLM code, no other agent's files.

## What I could not verify

- Reddit counts. `www.reddit.com` and `api.reddit.com` search JSON both returned a login/HTML
  wall from this machine, so no Reddit upvote or comment counts are claimed. Only the HN launch
  thread (194 points, 60 comments, https://news.ycombinator.com/item?id=39782876) is counted.
- Whether Jan's fit pill accounts for context length or KV cache. The docs only say it is computed
  from model metadata with no download; the exact inputs are in the app, which I did not install
  (desk study).
- Whether Windows on Arm gets any GPU acceleration in Jan. The docs do not say, and the release
  notes do not mention Adreno. Treated as CPU-only in the product file, flagged as an assumption.
- Exact issue counts are search-API totals at 2026-11-06 and include closed issues. GitHub
  secondary rate limits interrupted some queries (GPU detection, download, slow), so those three
  themes have no count in the product file.

## Open questions

- Does EZLM's update UI show the failure reason inline today? `internal/update/update.go` returns
  an `Info.Error`; whether the app renders it needs a read of `internal/app` (not done here).
- Should EZLM adopt a Trusted Hosts list, or is rejecting non-private Host headers enough when LAN
  is on? The stricter version is cheaper and covers reverse proxies less well.
- Jan's consent page in first-run setup: what is consented to? Not visible in the docs; worth a
  read of the 0.8.5 changelog entry before copying the idea.
