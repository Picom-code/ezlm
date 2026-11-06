# a07 REPORT: llamafile teardown

## What I did

Desk study of llamafile (Mozilla.ai) against EZLM, 2026-11-06, for T14. Read the
current docs site, the GitHub README and 0.10 migration notes, the 0.10.6
release feed, the CLI reference, the API/security/agentfile pages, and sampled
the GitHub issue tracker through the search API. Read EZLM's `internal/fit`,
`internal/keys`, `internal/serve` and `internal/scan` for comparison. No
competitor product was installed or run; no model was loaded.

Deliverable: `agents/T14/products/a07.md` (the section, in the brief's shape,
with five prioritised recommendations).

## Sources (all accessed 2026-11-06)

Primary docs, <https://docs.mozilla.ai/llamafile/...>:

- `getting-started/quickstart` (combined mode, 8080, OpenAI/Anthropic endpoints)
- `getting-started/pre-built-llamafiles` (13 files, 1.6-19 GB)
- `using-llamafile/running_llamafile` (--cli/--chat/--server/combined, --jinja)
- `using-llamafile/api` (endpoint list, --api-key, --api-key-file, --alias,
  host/port warning, CORS caveat)
- `using-llamafile/agentfile` (tools, confirmation, exit codes)
- `using-llamafile/../reference/support` (OS/CPU/GPU matrix, CUDA cc 7.5+,
  ROCm not bundled, Vulkan, silent CPU fallback, x86_64-only bundled libs)
- `reference/troubleshooting` (Gatekeeper, binfmt, WSL, CrowdStrike)
- `reference/security` (pledge sandbox, skipped on GPU and combined mode)
- `reference/cli_arguments` (--fit/--fit-target/--fit-ctx listed, -ngl, -lm)

Repository, <https://github.com/mozilla-ai/llamafile>:

- `README.md` (Windows 4 GB executable limit, .exe rename, 0.10 build system)
- `README_0.10.0.md` ("What's missing": Windows GPU, sandbox, localscore, ...)
- releases feed, latest 0.10.6 dated 2026-09-15
- issues via `https://api.github.com/search/issues`:
  #938, #384, #356, #214, #441, #92, #398, #666, #272, #750, #257, #167, #438

## Counting method and its limits

Issue totals are from the GitHub search API on 2026-11-06:
`repo:mozilla-ai/llamafile is:issue` returns 612; adding `windows` returns 155;
adding `rocm` returns 67; adding `"out of memory"` returns 13. GitHub search
matches title and body, so the keyword counts are "issues that mention the
term", not "issues caused by it". Reaction and comment counts are the values
shown in the API at access time and change over time.

Reddit was not sampled: `https://www.reddit.com/search.json` returned HTTP 403
from this host on 2026-11-06. The complaint table is therefore GitHub-only and
should be read as a lower bound. Hacker News and forum threads were not sampled
either.

## What I could not verify

- Whether Windows on Arm runs the AMD64 APE natively or only under x64 emulation.
  The docs say "Windows 10+ (AMD64 only)" but do not describe Arm behaviour, and
  the tracker has no Arm-specific issue. The claim in the product file is
  hedged accordingly.
- Whether 0.10.6 actually bundles working CUDA/Vulkan on Windows. The support
  page says the release binaries bundle both DLLs; `README_0.10.0.md` still
  lists "GPU support for Windows" under "What's missing". The two primary
  sources contradict each other, which is itself evidence of churn. I did not
  test.
- Real-world usage share of llamafile. No download or DAU numbers are public;
  the 612-issue tracker and 16-reaction top issue suggest a modest user base
  compared with Ollama or LM Studio.
- Whether llama.cpp's `--fit` works well in practice. llamafile lists the flags
  but never documents or recommends them; I did not run it.

## Open questions

1. Does EZLM want Anthropic compatibility enough to accept the maintenance cost
   of tracking a second schema (streaming event shapes, tool_use/tool_result)?
   The alternative is a translating proxy, which is more moving parts.
2. EZLM identifies a server by key+model (`internal/serve/port_test.go`). If
   R5 (accept any model name) lands, what stays as the identity check? A
   deployment id in `/props` would be cleaner than the model name.
3. Should EZLM offer a portable single-file mode at all, or is the Mac/Windows
   app plus engine enough? llamafile's only real advantage is zero install.
4. The 4 GB Windows executable ceiling is an APE/Cosmopolitan constraint, not a
   llama.cpp one; if EZLM ever bundles weights, that ceiling applies.

## Findings about EZLM (read-only)

- `internal/fit/fit.go` is materially ahead of llamafile: it enumerates quant,
  context, KV type and placement, applies margins and an OS floor, and ranks by
  effective intelligence with a tok/s floor. llamafile has none of this.
- `internal/serve` has no `/v1/messages` route (grep for `messages`/`anthropic`
  only hits an unrelated integrate test), so Anthropic-native agents cannot use
  an EZLM deployment today.
- `internal/serve/config.go` already models multiple named keys; llamafile has a
  single `--api-key`. Keep this.
- `internal/serve/client.go` verifies a deployment by calling `/v1/models` with
  the Bearer key; that is a good place to also read the backend actually in use
  (R2).

## Next steps

- a10 (recommendations) can lift R1-R5 directly; R1 is the one with the largest
  expected value.
- a12 (servers/keys) should compare the Anthropic surfaces of llamafile and
  LocalAI; I only covered llamafile.
- a11 (platforms) should test the Windows-on-Arm question I could not settle.
- If time allows, re-run the GitHub queries closer to the merge date and refresh
  the counts; they drift.

## Commit

Branch `agent/T14-a07`, files under `agents/T14/` only.
