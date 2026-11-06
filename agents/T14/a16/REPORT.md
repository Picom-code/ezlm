# T14 a16 report: report support

## What I did

- Read `~/ezlm-agents/README.md`, the T14 brief and the T14 section of
  `TASKS.md`.
- Read EZLM's planner, server, catalog and update code to check what the
  teardown recommendations would actually change: `internal/fit/fit.go`,
  `internal/serve/config.go`, `internal/serve/props.go`,
  `internal/catalog/catalog.go`, `internal/update/update.go`,
  `internal/engine/download.go`, and the Windows and macOS picker, keys and
  server panes.
- Read the primary docs for the eight products on 2026-11-06, plus one GitHub
  issue count that survived the search rate limit. Sources and access date are
  in `REPORT-support.md`.
- Wrote `agents/T14/a16/REPORT-support.md`: the teardown narrative, eight
  lessons, the five highest-expected-value changes, what not to copy, and the
  limits.
- No code changes. `agents/T14/` only.

## What I found

Severity is my judgement for a public beta. These are code-reading findings, not
runtime reproductions; a UI gap cannot have a failing test, so each cites the
file and line that shows the current behaviour.

1. **Unsigned builds block installs outright on Windows 11 with Smart App
   Control.** BETA.md says the block has no "Run anyway". Severity: major
   (documented, affects a whole class of machines). Evidence: `BETA.md:59`.
2. **The update check calls `api.github.com` unauthenticated.** That is 60
   requests per hour per IP, shared behind a NAT, and the failure is shown as an
   error string. Severity: minor. Evidence: `internal/update/update.go:22`,
   `windows/EZLM/SettingsPane.cs:25-28`.
3. **The plan card does not show memory headroom or the disk shortfall.** The
   planner computes `GPUNeed`, `RAMNeed`, `GPUBudget`, `RAMBudget`, `Download`
   and `DiskOK` (`internal/fit/fit.go:50-63`) and the card renders smarts,
   speed, words/s, context, quant and placement (`windows/EZLM/Panes.cs:67-92`,
   `windows/EZLM/Models.cs:72-99`), but not the margin or how much space is
   missing. Severity: minor. It is the cheapest place to answer "will it fit".
4. **Context is chosen by the planner and then fixed.** The ladder is in
   `internal/fit/fit.go:88-92`; the Hermes 64K trade-off is explained in
   `windows/EZLM/Panes.cs:576-578`; `/props` reports the loaded context
   (`internal/serve/props.go:18`). There is no way to move along the ladder
   after the pick. Severity: minor.
5. **"The server couldn't start" is a retry, not a diagnosis.** The UI shows a
   generic message plus `StartError` and a Try again button
   (`windows/EZLM/Panes.cs:317-319`, `windows/EZLM/Shell.cs:174`). The pieces
   for a diagnosis exist (`internal/serve/config.go:146`, `:174`, `:193`,
   `:201`, `:47`). Severity: major for beta support load.
6. **LAN reachability and key use are not observable.** New, rotate and revoke
   already exist and update wired apps (`windows/EZLM/Panes.cs:689-717`,
   `macos/Sources/EZLM/MainView.swift:600-640`), but there is no test that
   another device can reach the server and no last-used time. Severity: minor.

Things EZLM already does better than the field, worth protecting: named keys
with rotation (`internal/serve/config.go:21-30`), resumable downloads with
stall detection and digests (`internal/engine/download.go:130`,
`internal/engine/pinned.go:9`), a remote catalog that only accepts a newer
version (`internal/catalog/catalog.go:148`), and the Hermes context trade-off
copy.

## What I changed and why

Added `agents/T14/a16/REPORT-support.md`. It is the narrative and the ranked
five, not a second copy of the product sections. The first draft assumed the
picker was a bare list; reading `Panes.cs` and `MainView.swift` corrected that,
so the recommendations target headroom, context control, diagnosis, LAN proof
and signing instead of "add a model browser".

## What I could not finish

- Msty: the site redirects cross-origin and `web_fetch` does not follow it. No
  Msty claims are made.
- Per-product complaint counts: only the Ollama VRAM count (466) got through
  before the unauthenticated GitHub search limit. a15 owns the full counts.
- llamafile: repository landing page only; no platform detail.
- No competitor was installed, per the brief, so every claim is from docs or the
  issue tracker.

## Open questions

- The Windows and macOS UI files I read are on the rewrite list. Re-check the
  five gaps against the rewritten panes before scheduling.
- Is code signing funded for the beta, or is Smart App Control a documented
  known limit until GA?
- Should headroom be on the card or one tap deeper? Five numbers on a card can
  become noise.
- Would a stable machine-profile hash in `internal/report` help triage without
  adding personal data?

## Next steps

1. Merge the a16 five into `agents/T14/recommendations.md` (a10) so there is one
   ranked list, not two.
2. Hand the signing item to whoever owns release engineering; it is the only
   recommendation that blocks installation rather than degrading it.
3. Have a10 and a15 reconcile the issue counts before the report goes out.
