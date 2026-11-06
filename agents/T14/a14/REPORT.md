# T14/a14 REPORT: update mechanisms and their failure modes

Agent a14, 2026-11-06. Docs only; no competitor installed, no model loaded, no server
started. Deliverable: `agents/T14/a14/updates.md`.

## What I did

1. Read EZLM's update path end to end: `internal/update/update.go`,
   `internal/app/settings.go` (update section), `internal/engine/engine.go` (digest
   verification, pinned llama.cpp release), `windows/EZLM/SettingsPane.cs`,
   `macos/Sources/EZLM/Settings.swift` and `macos/Sources/EZLM/Updater.swift`.
2. Desk-researched the eight products from primary sources: official docs, changelogs,
   GitHub release metadata (GitHub releases API, GET) and specific GitHub issues with
   reaction/comment counts (GitHub issues API, GET).
3. Reproduced the version-ordering bug in `update.Newer` with a standalone copy of the
   function (no repo changes).
4. Wrote `updates.md`: per-product mechanism, failure modes, a six-class failure
   taxonomy, EZLM's current mechanism with file:line, and five recommendations with
   effort and priority.

## Sources (all accessed 2026-11-06)

| Source | Used for |
|---|---|
| https://docs.ollama.com/faq | Ollama upgrade path, restart-to-update, Linux install script |
| https://github.com/ollama/ollama/issues/6024 | "Disable auto updates", open, 28 reactions, 9 comments |
| https://lmstudio.ai/changelog | Bionic 1.2.0 (2026-11-05): fixed Mac update relaunching old version; fixed redundant redownload; runtime packs |
| https://lmstudio.ai/docs/app | app vs runtime management surface |
| https://jan.ai/changelog | release cadence (0.8.6/0.8.7/0.8.8, Oct 2026), bundled llama.cpp 0.7.0 |
| https://github.com/janhq/jan/issues/7148 | silent 1.2 GB updater download, open, 2 reactions, 7 comments |
| https://github.com/janhq/jan/issues/7003 | models broken after update, closed, 3 reactions |
| https://docs.gpt4all.io/gpt4all_help/faq.html | install/update model (manual) |
| https://github.com/nomic-ai/gpt4all/issues/3507 | update modal crashes on invalid code signature, open |
| https://github.com/nomic-ai/gpt4all/issues/3326 | installer no longer works as updater, open |
| GitHub releases API `nomic-ai/gpt4all` | v3.10.0 on 2025-02-25 is latest |
| https://docs.msty.app/getting-started/changelog | background auto-update; Local AI service versioned separately |
| https://docs.msty.app/troubleshooting/stuck-at-app-update | documented recovery: kill msty/ollama-runner, reinstall |
| https://docs.anythingllm.com/installation-desktop/update | manual reinstall, brew upgrade, AppImage replace |
| https://docs.anythingllm.com/ | persistent "update now" banner |
| https://github.com/mozilla-ai/llamafile (README) | single-file model, no updater, 0.9 vs 0.10 split |
| GitHub releases API `mozilla-ai/llamafile` | 0.10.5 (2026-08-03), 0.10.6 (2026-09-15) |
| https://localai.io/ | binary/container, lazy backend images |
| GitHub releases API `mudler/LocalAI` | v4.12.0 (2026-10-22), v4.12.1 (2026-10-23) |

Issue counts are GitHub reaction totals at access time; comment counts are GitHub's
`comments` field. Search totals come from GitHub's issue search (`update in:title`).

## Findings

Severity: blocker / major / minor. "EZLM" findings are code observations; the product
findings are public-evidence claims.

1. **major (EZLM): pre-release versions compare as strings, so beta.10 is older than
   beta.2.** `internal/update/update.go:128-158` compares the whole pre-release suffix
   lexicographically. Reproduced 2026-11-06 with the same function body:
   `Newer("0.1.11-beta.2","0.1.11-beta.10") == true`. Since the release feed includes
   pre-releases by design (`internal/update/update.go:19-22`), a beta.10 release would
   never be offered to beta.2 installs. No test covers multi-digit pre-release numbers.
2. **major (EZLM): digest verification is skipped when GitHub publishes no digest.**
   `internal/engine/engine.go:320-324` returns nil if the digest lacks the `sha256:`
   prefix, on the grounds that size was checked. The trust anchor is the same GitHub
   response that supplies the URL, so a compromised release account can serve any
   installer. No signature check exists.
3. **major (product, cross-cutting): "downloaded" is not "installed".** Ollama's
   highest-reaction update issue is a staged update that a restart does not apply
   (https://github.com/ollama/ollama/issues/6024, open, 28 reactions); LM Studio shipped
   a fix on 2026-11-05 for Mac updates reopening the old version
   (https://lmstudio.ai/changelog). EZLM verifies the running version nowhere after
   relaunch, so it is exposed to the same class.
4. **major (product): no user control over update timing or channel.** Ollama #6024 asks
   for a manual update and an off switch; Jan #7148 reports a 1.2 GB silent download.
   EZLM offers every release to every user, pre-releases included, with no pin or skip.
5. **major (product): in-use helper processes block the updater.** Msty's documented
   recovery is to kill `msty` and `ollama-runner` by hand
   (https://docs.msty.app/troubleshooting/stuck-at-app-update). EZLM already stops
   helper engines by exact path (`windows/EZLM/SettingsPane.cs:268-283`), which is the
   correct fix; this is a strength to keep.
6. **major (product): signature mismatch breaks the update path.** GPT4All's update modal
   crashes on an invalid code signature, and the installer-as-updater CLI broke in
   v3.5.0 (https://github.com/nomic-ai/gpt4all/issues/3507,
   https://github.com/nomic-ai/gpt4all/issues/3326). EZLM's macOS updater checks only
   the bundle ID before swapping (`macos/Sources/EZLM/Updater.swift`), so a
   wrongly-signed but same-ID bundle would be installed.
7. **minor (EZLM): Linux has no self-update path.** `assetFor` recognises only Windows
   and macOS assets (`internal/update/update.go:45-54`); a Linux engine build can never
   update itself, and the docs do not describe a manual upgrade for it.
8. **minor (EZLM): an offline machine retries the update check on every UI refresh.**
   `internal/app/settings.go:33-41` re-checks whenever the cached result carries an
   error, each with a 10 s timeout; the error is cached for no time at all.
9. **minor (EZLM): update leaves a running model server behind.** The Windows path
   deliberately keeps llama-server running while the app and engine files are replaced
   (`windows/EZLM/SettingsPane.cs:297-303`); after relaunch there is no check that the
   running server build matches the new app's expectations.
10. **minor (product): manual-reinstall-only updates.** AnythingLLM documents reinstall
    as the update mechanism (https://docs.anythingllm.com/installation-desktop/update);
    llamafile has no updater at all (README). Both preserve user data by promise, which
    is the sentence EZLM should also make explicit.

## Recommendations handed to a10/a16

R1 fix pre-release ordering (S, P1); R2 add pin/skip and a stable/beta channel (S-M, P1);
R3 sign the update and fail closed (M, P1); R4 keep the previous installer and offer
rollback (S-M, P2); R5 show one version line for app, engine and model after update
(S, P2). Full evidence and reasoning in `updates.md`.

## What changed

Docs only: `agents/T14/a14/updates.md` and this report. No code, no EZLM files.

## Could not finish / could not verify

- LM Studio's exact consent flow and whether updates can be deferred; no dedicated
  updater doc page was found.
- Whether AnythingLLM Desktop has an undocumented in-app update check.
- Ollama's updater implementation details (no public design doc).
- EZLM installer sizes and real download times (no build produced; time-boxed desk study).
- GitHub API rate limit (60 requests/hour, shared IP) capped how many issue threads could
  be counted; counts are a sample, not an exhaustive census.

## Open questions

- Should EZLM ship one installer that carries the engine, or split app and engine updates?
  Splitting adds the two-surface drift problem seen in LM Studio, Msty and LocalAI.
- Is a bad release recoverable for a non-technical user today? If not, rollback (R4) is
  the highest-value item after R1.
- Does the Windows installer require elevation? If it installs per-user, updates are
  silent; if per-machine, an update can prompt for admin mid-flow. Not verified.

## Next steps

1. Land R1 with a table-driven test on `Newer` (multi-digit pre-release, `v` prefix,
   build metadata).
2. Add a post-relaunch version check and a "you are on the old version" banner (R5).
3. Prototype signed manifests (R3) and keep-last-installer rollback (R4) behind a flag.
