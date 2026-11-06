# T14 a04 report: GPT4All

## Scope and method

Desk study of GPT4All's desktop app (`gpt4all-chat`, v3.10.0). No competitor
product was installed or run; no model was loaded. Sources were read on
2026-11-06 and are listed with URLs below. EZLM's code was read only for the
comparison table (`internal/scan`, `internal/fit`, `internal/serve`,
`internal/keys`, `internal/update`, `internal/catalog`).

Complaint counts: GitHub REST search (`repo:nomic-ai/gpt4all is:issue`, sorted
by reactions) and the issue objects themselves, read 2026-11-06. "Reactions" is
the sum of all emoji reactions, overwhelmingly 👍. Counts are public GitHub only
and undercount Discord/Reddit. Method is stated again in the product section.

## Sources read

| Source | URL | Date |
|---|---|---|
| Repo README (installers, platform support, history) | https://github.com/nomic-ai/gpt4all | 2026-11-06 |
| Repo metadata (stars, open issues, last push) | https://api.github.com/repos/nomic-ai/gpt4all | 2026-11-06 |
| Last commits on `main` | https://api.github.com/repos/nomic-ai/gpt4all/commits?per_page=5 | 2026-11-06 |
| Releases v3.10.0 down to v3.6.1 | https://api.github.com/repos/nomic-ai/gpt4all/releases?per_page=6 | 2026-11-06 |
| CHANGELOG (3.1.0 to 3.10.0) | https://github.com/nomic-ai/gpt4all/blob/main/gpt4all-chat/CHANGELOG.md | 2026-11-06 |
| System requirements (WoA contradiction) | https://github.com/nomic-ai/gpt4all/blob/main/gpt4all-chat/system_requirements.md | 2026-11-06 |
| Quickstart | https://docs.gpt4all.io/gpt4all_desktop/quickstart.html | 2026-11-06 |
| Models page (RAM Required table) | https://docs.gpt4all.io/gpt4all_desktop/models.html | 2026-11-06 |
| Settings (device, GPU layers, ctx, server port) | https://docs.gpt4all.io/gpt4all_desktop/settings.html | 2026-11-06 |
| FAQ (API, hardware) | https://docs.gpt4all.io/gpt4all_help/faq.html | 2026-11-06 |
| Troubleshooting (error loading models) | https://docs.gpt4all.io/gpt4all_help/troubleshooting.html | 2026-11-06 |
| Local server source (`server.cpp`) | https://github.com/nomic-ai/gpt4all/blob/main/gpt4all-chat/src/server.cpp | 2026-11-06 |
| Issue searches (top reactions, bug-unconfirmed, Vulkan, AMD) | https://api.github.com/search/issues?q=repo:nomic-ai/gpt4all+is:issue+is:open | 2026-11-06 |

Issue-level sources are linked inline in `products/a04.md`; the main ones are
#3546, #1747, #698, #101, #3576, #1568, #3502, #3455, #2174, #3099, #3540,
#3657, #3491, #2808, #3144, #2768.

## Findings

| # | Finding | Evidence | Severity |
|---|---|---|---|
| F1 | No release since 2025-02-25 and no commit since 2025-05-27; the product is in maintenance freeze | repo metadata, commits API | major (strategic) |
| F2 | New-model support requires an app release; Qwen3 (#3576, 17 reactions), Gemma 3 (#3540), Qwen3.5 (#3657) all open | issue search | major |
| F3 | Local server rejects `stream`, `tools`, `tool_choice`, `response_format`, `stop`, `seed` with explicit errors | `server.cpp` lines ~160-190, ~405-420 | major |
| F4 | Local server has no API key and binds `QHostAddress::LocalHost` only; no setting for the bind address | `server.cpp` line 464, no auth header read | major (vs EZLM) |
| F5 | No hardware scan and no fit check; only a manual GPU-layers/ctx field, and GPU layers > default silently reverts (#2808) | settings docs, #2808 | major |
| F6 | Windows on Arm: separate CPU-only build since v3.7.0; docs contradict themselves on ARM support | CHANGELOG 3.7.0, README vs `system_requirements.md` | minor (docs) |
| F7 | Update/packaging bugs: macOS maintenance-tool crash on Sequoia (fixed 3.7.0), wrong version comparison (fixed 3.2.0), Windows 11 uninstaller broken (#3099) | CHANGELOG, #3099 | minor |
| F8 | No ROCm; AMD/Intel go through Vulkan, support is per-architecture and lags (OLMoE/Granite MoE "no Vulkan" in 3.9.0) | CHANGELOG 3.1.0/3.4.0/3.9.0 | minor |

## What I could not verify

- Whether the desktop app writes or exposes an API key anywhere (source shows the
  server never checks one; the Python bindings and the separate Docker API server
  may differ).
- The exact first-run screen in 3.10.0 (docs are © 2024 and show the 3.x flow;
  no screenshot later than that).
- Whether Continue.dev's empty responses (#2174) are caused by the `stream`/`tools`
  rejection or by something else. The parser evidence makes it plausible, but the
  thread does not prove it. Marked as inference in the product section.
- Intel Arc behaviour specifically: only generic Vulkan evidence found, no Arc
  issue with meaningful engagement.
- Reddit/forum complaint volume. GitHub reactions were the only countable
  primary signal; Discord is not indexable.

## Deliverables

- `agents/T14/products/a04.md`: nine-dimension teardown, comparison table,
  six prioritised recommendations (P1 x3, P2 x2, P3 x1).
- This report.

## Single most valuable recommendation

Make the model catalog refreshable without an app update (P1, M). GPT4All's
largest complaint cluster is model-support lag, and its freeze turned that from
an annoyance into a dead end; EZLM has the same structural coupling in
`internal/catalog` plus `internal/update`.

## Open questions for the lead

1. Should catalog refresh be signed JSON from GitHub releases, or a separate
   hosted endpoint? (Affects `internal/update` design.)
2. Does EZLM want to own MCP now, or only document how to point an MCP client at
   the existing OpenAI URL?
3. For the comparison table: is "no API key, localhost only" a deliberate EZLM
   differentiator to market, or just an implementation detail?
