# T14/a14: update mechanisms and their failure modes

Desk study, 2026-11-06. All links accessed 2026-11-06. Nothing was installed or run;
evidence is public docs, changelogs, release metadata and GitHub issue counts.

Scope: how each of the eight teardown products ships updates, what breaks, and what EZLM
should copy or avoid. Issue counts below are GitHub reactions at access time, read from
the issue API (`reactions.total_count`); comment counts are GitHub's `comments` field.

## Summary table

| Product | Update path | User consent | Rollback / pin | Main failure class |
|---|---|---|---|---|
| Ollama | Auto-download on macOS/Windows, "Restart to update"; Linux re-run install script | None; no off switch | No pin, no rollback | Downloaded but not applied; forced updates |
| LM Studio | In-app updater, optional; runtimes updated separately | Yes (prompt) | No pin; previous version redownloadable | Updater installed wrong version (fixed 2026-11-05) |
| Jan | Auto-download installer, in-app restart | No (issue #7148) | No | Silent 1.2 GB download |
| GPT4All | Manual installer; in-app "update available" modal | Yes | No | Signed/unsigned mismatch crashes update |
| Msty | Background auto-update | Yes (background) | No | Stuck at update screen; needs process kill + reinstall |
| AnythingLLM | Manual reinstall (dmg/exe/AppImage), `brew upgrade`, Docker pull | Manual by definition | Yes (keep old AppImage/image tag) | No in-app updater; user must notice |
| llamafile | Replace one binary by hand | Manual | Yes (keep old file) | No notification at all |
| LocalAI | Pull new binary/image; backends pulled at runtime | Manual | Yes (image tag) | Config/backend drift across versions |

## Per product

### Ollama

Mechanism: macOS and Windows builds download updates in the background and apply them
only when the user picks "Restart to update" from the tray/menu bar; Linux has no
in-app updater and the documented upgrade is re-running the install script
(<https://docs.ollama.com/faq>, "How can I upgrade Ollama?"). Docker installs upgrade by
pulling a new image tag.

Failure modes:
- Update staged but never applied. The top-voted "update" issue is exactly this:
  "It had downloaded the update and it was placed in a temporary folder, but restarting
  ollama didn't update it", asking for a manual update command and an off switch
  (<https://github.com/ollama/ollama/issues/6024>, open, 28 reactions, 9 comments,
  opened 2024-07-28). It is still open, so the missing controls are still missing.
- No user control over update timing or version. Same issue: users want to pin a version
  because a newer build can change model behaviour. Ollama exposes no stable/beta channel
  and no "skip this version".
- Linux is a different product operationally: users who installed via the script get no
  notification and no version check; the server can run a stale binary indefinitely.
- Counts: 259 open+closed issues match `update in:title` in `ollama/ollama`
  (GitHub search, 2026-11-06); only a minority are about the updater, but #6024 is the
  highest-reaction one, which says the updater is the sore point rather than a niche bug.

Takeaway for EZLM: "downloaded" is not "installed". The restart-to-apply step needs a
visible, persistent state and a fallback (relaunch prompt, not a tray item users miss).

### LM Studio

Mechanism: in-app updater with a prompt; the app and its runtimes are separate update
surfaces. The changelog lists "llama.cpp 2.67.0 extension packs" alongside app changes
(<https://lmstudio.ai/changelog>), and runtimes are managed in-app (Ctrl/Cmd+Shift+R
opens runtime management, <https://lmstudio.ai/docs/app>).

Failure modes (both fixed, both in the 2026-11-05 Bionic 1.2.0 notes,
<https://lmstudio.ai/changelog>):
- "Fixed Mac updates reopening the current version instead of installing the update" —
  the classic updater bug: the user believes they updated, the old build relaunches.
- "Fixed app updater redownloaded unnecessarily" — bandwidth and trust cost when the
  updater loses track of what it already downloaded.
Release cadence is very fast (Bionic 1.1.10 then 1.2.0 within days), so the updater is
on the hot path for every user; a one-release updater regression hits everyone at once.

Takeaway for EZLM: verify the running version after relaunch and show it. A relaunch
that silently opens the old build is indistinguishable from a successful update.

### Jan

Mechanism: Electron-style auto-updater. The installer is downloaded to
`%LOCALAPPDATA%\jan-updater\installer.exe` and applied on restart; releases are frequent
(0.8.6 on 2026-10-09, 0.8.7 on 2026-10-20, 0.8.8 on 2026-10-28,
<https://jan.ai/changelog>).

Failure modes:
- Downloads without consent: "Jan auto downloads updates without permission (drained
  1.2GB)" — the reporter found a 1.2 GB `installer.exe` after a month of light use
  (<https://github.com/janhq/jan/issues/7148>, open, 2 reactions, 7 comments).
- Updates bundle a new inference engine, so an app update can change model behaviour:
  "Llama.cpp models don't work after update to v.0.7.3"
  (<https://github.com/janhq/jan/issues/7003>, closed, 3 reactions). Engine and app move
  together with no way to keep the old engine.

Takeaway for EZLM: a full installer on a metered connection is a real cost (EZLM's
installer carries an engine too). Ask before downloading, or make the download clearly
opt-in and resumable.

### GPT4All

Mechanism: manual installer download; the app shows a "new version available" modal at
startup. There is no auto-update. Releases are infrequent: v3.10.0 on 2025-02-25 is the
latest release in `nomic-ai/gpt4all` (GitHub releases API, 2026-11-06).

Failure modes:
- The in-app update path crashes: "A modal appears at startup, indicating that a new
  version is available. Click on the Update button... The application crashes
  immediately", caused by an invalid code signature
  (<https://github.com/nomic-ai/gpt4all/issues/3507>, open, 1 reaction). Updating and
  uninstalling both fail, so the user is stuck on the old build.
- The installer stopped working as an updater: the documented silent update flags
  (`--accept-licenses --default-answer --confirm-command update`) broke in v3.5.0
  (<https://github.com/nomic-ai/gpt4all/issues/3326>, open, 2 reactions).
- Stale build for a year means no update prompt is trustworthy either way; users cannot
  tell "up to date" from "abandoned".

Takeaway for EZLM: an update prompt that can crash the app is worse than no prompt.
Test the update path on the exact signed artifact that ships.

### Msty

Mechanism: background auto-update. "Msty updates itself automatically in the background.
Let it run for a few minutes" (<https://docs.msty.app/getting-started/changelog>). The
bundled "Local AI" service is versioned and updated separately from the app: the
changelog lists "Local AI version 0.6.6", and there is a how-to "Get the latest version
of Local AI service" (<https://docs.msty.app/>).

Failure modes:
- The vendor documents a "Stuck at App Update" recovery page: stop the Local AI service,
  kill every `msty` process in Task Manager/Activity Monitor, kill `ollama-runner`, then
  Check for Updates and restart, and if that fails reinstall manually
  (<https://docs.msty.app/troubleshooting/stuck-at-app-update>). A recovery page in the
  docs is a shipped failure mode: the updater cannot replace files held open by its own
  helper processes.
- Two update surfaces (app and Local AI service) mean the app can update while the local
  server stays old, or the reverse, with no single version shown to the user.

Takeaway for EZLM: EZLM already stops helper engines by exact path before running the
installer (`windows/EZLM/SettingsPane.cs:268-283,301-302`), which is the fix Msty
documents as a manual workaround. Keep that; add a one-click "repair update" path so the
user never has to open Task Manager.

### AnythingLLM

Mechanism: documented updates are manual reinstalls. macOS: download the new dmg and
reinstall (or `brew upgrade --cask anythingllm` if installed by brew); Windows: download
the new exe and reinstall; Linux: run the new AppImage and delete the old one
(<https://docs.anythingllm.com/installation-desktop/update>). The self-hosted Docker
install updates by pulling a new image.

Failure modes:
- No in-app updater is documented for Desktop, so discovery is the user's job. The docs
  site itself carries a persistent "v1.19.0 is live! Update now" banner
  (<https://docs.anythingllm.com/>), which is the only notification channel described.
- Reinstall-over-existing is the update mechanism, so any installer regression is an
  update regression; there is no separate update code path to test.
- Docker users must read release notes to know whether a schema/data migration is needed;
  nothing in the docs automates it.

Takeaway for EZLM: manual reinstall is acceptable for a beta, but it must preserve
models, keys and connected apps. AnythingLLM's explicit promise ("persisting your
storage and progress") is the sentence EZLM's update UI should also make.

### llamafile

Mechanism: none. A llamafile is a single executable; updating means downloading the new
release and replacing the file. Releases keep old versions available, and each llamafile
states which llama.cpp server it bundles
(<https://github.com/mozilla-ai/llamafile>, README; releases 0.10.5 on 2026-08-03 and
0.10.6 on 2026-09-15, GitHub releases API).

Failure modes:
- No notification and no version check, so a user can run a year-old build with known
  model-support gaps and never know. The README's answer to version confusion is
  documentation, not software.
- The "classic" 0.9.* line and the 0.10.* line are not interchangeable (README points
  users to `README_0.10.0.md` for the behaviour changes), so a manual upgrade can change
  flags and defaults with no migration.

Takeaway for EZLM: a single binary with no updater is the low-effort baseline EZLM
already beats. The lesson is the version surface: llamafile tells the user which engine
is inside; EZLM should show app version, engine build and model in one place.

### LocalAI

Mechanism: manual. Users run a binary or a container image and upgrade by pulling a new
tag; model backends are separate images pulled on demand at runtime
(<https://localai.io/>, "A small core pulls each engine in as a separate backend, only
when a model asks for it"). Latest release at access: v4.12.1 on 2026-10-23, with
per-platform binaries and a launcher (GitHub releases API).

Failure modes:
- Backend drift: a new core version may expect a newer backend image, and the backend is
  fetched lazily on first model load, so the failure appears at request time, not at
  upgrade time.
- Config compatibility: LocalAI model configs are user-edited YAML; the project moves
  fast (v4.12.0 on 2026-10-22, v4.12.1 on 2026-10-23), so old configs are the migration
  burden. No automatic config migration is documented.
- Container users get no in-app signal; they learn about releases from GitHub.

Takeaway for EZLM: lazy-downloaded runtime pieces are an update surface too. If EZLM
ever splits the engine from the app, it needs one "everything is at version X" view.

## Cross-cutting failure taxonomy

1. **Staged but not applied.** The most common and most damaging class: Ollama #6024
   (28 reactions), LM Studio's fixed Mac relaunch bug. Users believe they are patched
   when they are not, which is a security problem as much as a UX one.
2. **In-use files block replacement.** Msty documents killing `msty` and `ollama-runner`
   before updating; GPT4All's installer-as-updater broke. Any product with a running
   inference server has this problem; the fix is to stop only your own child processes
   by exact path, which EZLM already does.
3. **Consent and bandwidth.** Jan downloaded 1.2 GB unasked (#7148); Ollama has no off
   switch (#6024). Installers that carry an inference engine are large.
4. **Signature and permission failures.** GPT4All's update modal crashes on an invalid
   code signature (#3507); macOS apps outside a writable `/Applications` cannot be
   swapped. EZLM's macOS updater falls back to opening the dmg in Finder when the swap
   fails (`macos/Sources/EZLM/Settings.swift:65-101`), which is the right shape.
5. **Two update surfaces.** App plus runtime/engine (LM Studio, Msty, LocalAI, Jan)
   drifts. EZLM has the same shape: app plus pinned llama.cpp build b11960
   (`internal/engine/pinned.go:9`) plus models. One version view is needed.
6. **No rollback, no pin.** None of the eight products offers a first-class "go back to
   the version that worked" button; llamafile and AnythingLLM only allow it because the
   user keeps the old file. For a public beta, rollback is the cheapest safety net.

## What EZLM does today (code read)

- Checks `https://api.github.com/repos/picom-code/ezlm/releases?per_page=10`, skips
  drafts, and picks the newest release with a Windows `EZLM-Setup*.exe` or macOS
  `EZLM*.dmg` asset (`internal/update/update.go:22,45-54,56-108`). Pre-releases are
  included on purpose (comment at line 19), so beta users and stable users get the same
  feed.
- No Linux asset is recognised (`assetFor` handles only windows and darwin,
  `internal/update/update.go:45-54`), so the engine on Linux has no self-update path.
- Caches the answer for an hour, but re-checks on every call while the last check ended
  in an error (`internal/app/settings.go:33-41`), so an offline machine pays a 10 s
  timeout per UI refresh.
- Downloads to `os.TempDir()/EZLM-update`, resumes, and verifies SHA-256 when GitHub
  publishes a digest (`internal/update/update.go:111-124`). When the digest is absent
  the check is skipped and only size was checked (`internal/engine/engine.go:320-324`).
- Windows: silent Inno install with `/CLOSEAPPLICATIONS`, after stopping helper engines
  by exact path (`windows/EZLM/SettingsPane.cs:268-283,301-302`). The model server is
  left running during the swap by design.
- macOS: mount dmg, `ditto` to a staging name, swap with rollback, relaunch via a shell
  watcher; bundle-ID check only, then a Finder fallback
  (`macos/Sources/EZLM/Updater.swift`, `macos/Sources/EZLM/Settings.swift:65-101`).
- Version compare `Newer()` compares the pre-release suffix lexicographically
  (`internal/update/update.go:128-158`). Reproduced 2026-11-06: `Newer("0.1.11-beta.2",
  "0.1.11-beta.10")` returns true, i.e. beta.2 is treated as newer than beta.10. A
  beta.10 release would never be offered to a beta.2 install.
- No channel selection, no "skip this version", no rollback, no staged rollout, and no
  signature beyond GitHub's digest. The update is optional to download but the banner
  appears automatically (`windows/EZLM/Shell.cs:592`,
  `macos/Sources/EZLM/AppModel.swift:706-708`).

## Recommendations for EZLM

**R1. Fix pre-release ordering and add a test (S, P1).** `Newer()` must compare
dot-separated numeric pre-release identifiers numerically, not as a string. Evidence:
reproduction above; the beta feed is the default feed (`internal/update/update.go:19-22`).
A beta.10 build that nobody on beta.2 is offered is a silent stall in the update channel.

**R2. Add "stay on this version" and a stable/beta channel (S-M, P1).** Let the user
pin, skip one release, or opt into betas; default stable users to no pre-releases.
Evidence: Ollama #6024 (28 reactions, still open, asks for exactly this) and Jan #7148
(silent 1.2 GB download). EZLM's own feed mixes both channels today.

**R3. Sign the update, not just hash it (M, P1).** A SHA-256 fetched from the same
GitHub API as the asset protects against corruption, not against a compromised release
account, and EZLM skips verification entirely when the digest is missing
(`internal/engine/engine.go:323`). Sign the installer with a key shipped in the app and
fail closed. Evidence: GPT4All #3507 shows how badly a signature mismatch ends for the
user, and that is the benign version of this failure.

**R4. Keep the previous installer and offer "go back" (S-M, P2).** Keep the last known
good installer (or a copy of the app bundle) until the new version has started once and
reported healthy, then offer a one-click rollback. Evidence: none of the eight products
offers rollback; LM Studio's "reopening the current version" bug and Msty's stuck-update
recovery both end in "reinstall manually". For a public beta this is the cheapest
insurance against a bad release.

**R5. Show one version line for app, engine and model after every update (S, P2).** After
relaunch, display the running app version, the llama.cpp build and the model, and warn
if the running model server is older than the app expects. Evidence: the two-surface
drift in LM Studio (app vs runtime packs), Msty (app vs Local AI service) and Jan
(app vs bundled llama.cpp, #7003). EZLM updates the app while leaving the model server
running (`windows/EZLM/SettingsPane.cs:297-303`), so it has exactly this skew window.

## What I could not verify

- LM Studio's exact consent flow (prompt vs silent) beyond the changelog and docs; no
  public doc page dedicated to the updater was found.
- Whether AnythingLLM Desktop has an undocumented in-app update check; the docs only
  describe manual reinstall.
- Ollama's macOS/Windows auto-update implementation (no public design doc); behaviour is
  taken from the FAQ and issue reports.
- Download sizes: only Jan's 1.2 GB is reported by a user; EZLM's own installer size was
  not measured (no build was produced).
