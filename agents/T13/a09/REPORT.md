# a09 report: server, engine and update messages

**Section:** `agents/T13/sections/a09.md` (about 3,100 words, 44 entries)
**Scope:** every error path in `internal/serve`, `internal/engine` and `internal/update`.
Docs only; no code changed.

## What I covered

I read all 11 non-test Go files in the three packages, then followed every message to
the place a tester sees it. The section is grouped the way a tester meets the
messages:

| Group | Entries | Source |
|---|---|---|
| Starting and running the server | 17 | `internal/serve/client.go`, `config.go`, `service_*.go`, `run.go` |
| Installing the AI engine | 8 | `internal/engine/engine.go`, `pinned.go` |
| Downloading a model or the engine | 12 | `internal/engine/download.go` |
| Updating EZLM | 5 | `internal/update/update.go` |
| Not user-facing | 4 | `props.go`, `procs.go`, `client.go`, `download.go` |
| App-level wrappers, cross-referenced | 3 | `internal/app/app.go`, `internal/app/connect.go` |

Every entry has the message text, what it means, what to do, and a `file:line`
citation. Placeholders are in angle brackets. I documented the seven log phrases
that `internal/serve/client.go:134` turns into a red message, the four download
status families in `internal/engine/download.go:239`, the four plain network
explanations in `:210`, and the three update check failures in
`internal/update/update.go:68`, `:73`, `:90`.

Where the message reaches the screen matters for the wording, so I checked both apps:
a failed setup step prints the message under the red step
(`windows/EZLM/Install.cs:278`, `macos/Sources/EZLM/AppModel.swift:571`), the failure
panel offers **Back to the start** and **Report this problem**
(`windows/EZLM/Install.cs:154`), and update errors are appended to a version line in
Settings (`windows/EZLM/SettingsPane.cs:66`, `macos/Sources/EZLM/Settings.swift:296`).

## Findings

All are minor unless marked. These are in files a09 does not own, so they are
reported, not changed.

1. **The "disk filled up" message names a button that does not exist** (minor).
   `internal/app/app.go:744` says `press Set it up again`, but the buttons are
   **Set it up**, **Set it up (no download)** and **Resume and set it up**
   (`windows/EZLM/Shell.cs:415`, `macos/Sources/EZLM/AppModel.swift:387`). A tester
   looks for a button that is not on screen. Suggested wording: `press Resume and set
   it up`.
2. **The damaged settings message has no next action** (minor).
   `internal/serve/config.go:69` prints the raw JSON error, for example
   `EZLM's settings file (C:\Users\me\.ezlm\config.json) is damaged: invalid
   character '}' looking for beginning of object key string`, and stops. Nothing says
   what to do, and the setup screen cannot start. Suggested wording: `EZLM's settings
   file (<path>) is damaged. Set the model up again to rebuild it.`
3. **The update error line reads badly on Windows** (minor).
   `windows/EZLM/SettingsPane.cs:66` renders `EZLM 0.1.1. couldn't reach GitHub to
   check for updates.` (two periods, lowercase start), while the Mac capitalises the
   sentence and avoids the double period (`macos/Sources/EZLM/Settings.swift:296`).
   The source message is lowercase because it is a sentence fragment
   (`internal/update/update.go:68`).
4. **Two near-identical test messages** (minor, candidate for a14).
   `test request returned no answer` (`internal/serve/client.go:202`) and `the model
   answered with nothing` (`:208`) sound the same to a tester but have different
   causes: an empty `choices` list versus an empty `content`. Both tell the tester to
   set the model up again.
5. **`downloading <file> failed: <status>` is the least explanatory download message**
   (minor, candidate for a14). `internal/engine/download.go:245` shows a bare status
   such as `416 Requested Range Not Satisfiable` with no action. Suggested: `the
   download server refused part of <file> (<status>). Set it up again, and report
   this if it repeats.`
6. **Platform-specific advice in shared code** (minor).
   `internal/serve/config.go:196` tells the user to `end "llama-server" in Task
   Manager`, which only exists on Windows, but the message is returned on macOS and
   Linux too. Suggested: name the platform's own tool, or say `end llama-server in
   your task manager`.
7. **`no llama.cpp build for <os>/<arch> (<backend>) in release <tag>`** (minor).
   `internal/engine/engine.go:189` uses Go's internal names (`windows/amd64
   (CUDA)`), which a tester cannot act on. Suggested: `EZLM has no build for this
   computer's graphics card (<backend>) in this engine release. Report this problem.`
8. **`llama-server not found in <archive>`** (minor). `internal/engine/engine.go:261`
   names the archive and gives no action. Suggested: `The engine download did not
   contain the server program. Set it up again, and report this if it repeats.`

## What I could not verify

- **No real server, deploy or model load.** Per README rules I did not start
  llama-server or load a model. Behaviour comes from the code and from the package
  tests (`internal/serve/stopped_test.go`, `internal/engine/download_*_test.go`,
  `internal/update/update_test.go`).
- **Exact launchd and systemd output.** The quoted examples (`Bootstrap failed: 5:
  Input/output error`, `Unit ezlm.service not found`) are what those tools usually
  print, not strings observed in this worktree. The wrapper format
  (`launchctl bootstrap: <output>`, `systemctl <args>: <output>`) is exact.
- **The "about 13 seconds" in the stopped-server entry** is 8 seconds of grace plus
  five one-second checks (`internal/serve/client.go:126`), not a measured time.
- **Update check on a Mac.** I read `macos/Sources/EZLM/Settings.swift:292` but did
  not run the Mac app, so the rendered sentence is derived, not seen.
- **Rendered wrapping.** Long messages (the `ErrStillRunning` sentence is 220
  characters) are shown in a 13pt label; I did not check they wrap without clipping
  in the failure panel.

## Open questions

1. Should `agents/T13/BETA.md` mention `EZLM_LLAMA=latest` at all? Three engine
   messages are only reachable with it, so the guide could either document them as
   advanced or leave them out.
2. Should the damaged-settings path offer a **Rebuild settings** button instead of
   relying on the tester to find **Set it up**? That needs an app change (T12/T14).
3. On Windows the failure panel says **Back to the start** and returns to the scan;
   on a Mac it is the same. Should the troubleshooting guide always spell out that
   two-step path (back, then set up), or just say "set it up again"?
