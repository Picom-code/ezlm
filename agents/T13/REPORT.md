# T13. Beta tester guide and troubleshooting — consolidated report

16 agents (a01–a16) worked this task in parallel: eight guide sections, four troubleshooting
sections, the FAQ, the "unclear message" list, the assembly script, and a code-checked read-through.
Consolidated onto `agent/T13-beta-docs` by the Lead.

Goal: docs a non-technical tester can follow, and a troubleshooting guide keyed by the exact messages
EZLM shows.

## Result

- **`agents/T13/BETA.md`** — 10,783 words, assembled from the eight guide sections (Windows install,
  macOS install, first run, picking a model, keys and LAN, connecting OpenCode/Hermes/DeepSeek
  Harness, chat, updating and uninstalling).
- **`agents/T13/TROUBLESHOOTING.md`** — 11,414 words, 27 entries assembled from the four
  error-path sections, each with the exact message, what it means, and what to do.
- **`agents/T13/FAQ.md`** — 1,750 words, 15 questions.
- **Every behaviour claim carries a `file:line` citation.** Docs only: no code was changed.
- `agents/T13/a14/unclear-messages.md` — 32 messages that were too vague to document, with proposed
  wording (feeds T12).
- `agents/T13/a16/review.md` — the cross-check, which found six contradictions between sections and
  the code; they are listed below.

## What the read-through caught (fix before publishing)

1. **The installer's filename** is `EZLM-Setup-<version>.exe`, not `EZLM-Setup.exe` as `BETA.md`
   said (`windows/installer/EZLM.iss:4,19`).
2. **Uninstall "Remove everything" also deletes settings and keys** (`EZLM.iss:94`,
   `internal/app/app.go:1586-1605`); `BETA.md` said models only.
3. **The Mac app is universal and runs on Intel**, though slower (`macos/scripts/build-app.sh:12`,
   `internal/scan/scan_darwin.go:10-13`); `BETA.md` said Apple Silicon only.
4. **Windows minimum is build 19041 / 2004** (`EZLM.iss:30`), not "Windows 10".
5. **A Mac starts the model at login from a launchd agent** (`internal/serve/service_darwin.go:44-70`);
   no section told a Mac tester that, or how to stop it.
6. **Two EZLM folders**: app logs/chats in `%LOCALAPPDATA%\EZLM`, engine home in
   `%USERPROFILE%\.ezlm` (`windows/EZLM/Engine.cs:24-33`, `internal/paths/paths.go:9-18`). Sections
   that name "the data folder" must name both.

Also worth knowing: the 9-character key display, the "Set it up" / "Resume and set it up" button
labels, and the four different Hermes 64K wordings.

## Findings that belong to other tasks

- **`Remove everything` never deletes chat history or memory** on either platform
  (`app.go:1586-1606`) — a real gap the guide has to describe as it is (T07/T10 territory).
- **`Hardware.notes` is never rendered by either app** (`windows/EZLM/Models.cs:28`,
  `macos/Sources/EZLM/Models.swift:39`), so "No supported GPU found" is invisible to app testers
  (T05).
- **All catalog failures are silent** (`internal/catalog/catalog.go:148-181`) (T01).
- **An unknown disk size reads as "enough room"** (`internal/fit/fit.go:321`,
  `internal/scan/scan.go:280-289`), so a failed disk read kills the low-disk warning (T04/T05).
- **The app says a download will "pause" when the disk fills** but it stops until "Set it up again"
  (`app.go:735` vs `743-744`) (T12).
- **`app.go:744` names a button "Set it up again" that does not exist** (real label: "Set it up" /
  "Resume and set it up") (T12).
- **The Mac choose screen has no "Saves to" / Change row** that Windows has (T16).

## Open decisions for the Lead

1. **Signing:** `build-app.sh:72` says releases are Developer ID signed and notarized, while
   `BETA.md:16` says the beta is not notarized and the repo has no notarize step. The guide is
   written conditionally; one of the two must be fixed.
2. **"Open llama.cpp's own chat in the browser"** — keep it in the guide, or drop it as
   developer-flavoured?
3. **Owners for the rewrites:** `internal/app`, `internal/bench` and `internal/integrate` are named
   by no task, so several "unclear message" entries have no owner for T12's rewrite pass.

## Merge notes

This was the cleanest task of the sixteen: no source conflicts, only docs. The assembly script
reported 17 "gaps" (mostly section headings in a12 that are not troubleshooting entries — "Notes for
the assembler", "Messages that appear only in the log"); they are expected, not defects. The script
is idempotent: rerunning produces byte-identical files.

## Files

- `agents/T13/BETA.md`, `TROUBLESHOOTING.md`, `FAQ.md` — the deliverables.
- `agents/T13/sections/a01..a13.md` — the sources (edit these, then rerun the assembler).
- `agents/T13/assemble.py` — the assembler (`--check` validates, `--report` writes the gap report).
- `agents/T13/a14/unclear-messages.md`, `agents/T13/a16/review.md`, and one report per agent.

## Next steps

1. Apply the six corrections above, then rerun `assemble.py`.
2. Resolve the signing question, then make the macOS install section definite.
3. Feed `a14`'s 32 messages into T12's rewrite pass.
