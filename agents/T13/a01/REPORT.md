# T13 a01 report: Install on Windows

Section: `agents/T13/sections/a01.md`, about 1200 words. Covers the installer, the
SmartScreen path, Smart App Control, antivirus false positives, the one UAC prompt, and how to
check that the install worked. Docs only; no code changed.

## What I verified in the code

- The installer is Inno Setup, per user, no admin prompt, fixed folder
  `%LOCALAPPDATA%\Programs\EZLM`, one file for x64 and ARM64, Windows 10 2004 minimum:
  `windows/installer/EZLM.iss:1-33`, `:36`, `:39-51`.
- Setup opens EZLM when it finishes (`windows/installer/EZLM.iss:51`) and closes a running EZLM
  through the Restart Manager (`:28`).
- A newer installer keeps models and settings: models live under EZLM's home, not the program
  folder (`internal/paths/paths.go:29-33`), and `[InstallDelete]` only removes `*.xbf`
  (`windows/installer/EZLM.iss:42-44`).
- The C++ runtime prompt is real and is documented in the code itself: "Windows asks for
  permission once" (`internal/prereq/prereq.go:91-107`), installed silently with UAC
  (`:180-194`, `internal/app/app.go:646-655`).
- Antivirus wording comes from `windows/EZLM/Engine.cs:75-76` and
  `internal/app/app.go:1531-1541`. The folder to allow is `paths.Bin()`
  (`internal/engine/engine.go:228`).
- Verification steps map to real UI: "Nothing is set up yet" (`windows/EZLM/Panes.cs:113`),
  System page facts (`Panes.cs:529-532`), version line on Settings
  (`windows/EZLM/SettingsPane.cs:52,63-65`), Start server (`Panes.cs:215`), logs
  (`windows/EZLM/Shell.cs:188-196`, `windows/EZLM/Engine.cs:24-33`).

## Findings

1. **Minor, doc inconsistency.** `BETA.md:8` names the download "EZLM-Setup.exe", but the build
   produces `EZLM-Setup-<version>.exe` (`windows/installer/EZLM.iss:4,19`). The updater accepts
   any `EZLM-Setup*.exe` (`internal/update/update.go:45-50`). My section uses the versioned name
   and an example. Root `BETA.md` untouched (owned by the Lead).
2. **Minor, two different EZLM folders.** The app keeps logs and chats in `%LOCALAPPDATA%\EZLM`
   (`windows/EZLM/Engine.cs:24-33`), while the engine's home is `%USERPROFILE%\.ezlm`
   (`internal/paths/paths.go:9-18`). The Windows app never sets `EZLM_HOME`, so the llama.cpp
   binaries sit in `%USERPROFILE%\.ezlm\bin`. Antivirus advice must name both folders, or testers
   will allow only one. Any other section that gives a data folder should use the same pair.
3. **Minor, no signing step in the repo.** There is no `signtool` call anywhere under `windows/`,
   so "not code-signed" is consistent with the build. However
   `macos/scripts/build-app.sh:72` says releases are "signed with a Developer ID and notarized",
   which contradicts `BETA.md:16` ("not notarized by Apple yet"). Cross-section issue for a02.
   I did not resolve it.
4. **Minor, installer naming vs. uninstall question.** Uninstall asks whether to delete models,
   default No (`windows/installer/EZLM.iss:91-97`), and removes the login Run value (`:57`).
   a08 should own this; my section only points at uninstall in passing.
5. **Note, in-app update path.** The app runs the downloaded installer as
   `/SILENT /SUPPRESSMSGBOXES /NORESTART /SP- /CLOSEAPPLICATIONS /UPDATE=1`
   (`windows/EZLM/SettingsPane.cs:302`) and setup reopens EZLM even when the update stops early
   (`windows/installer/EZLM.iss:75-89`). Relevant to a08, not to my section.

## What I could not verify

- The exact SmartScreen dialog text and button order on current Windows 10 and 11. It is OS
  behaviour, not in the repo. I used the wording already in `BETA.md:8` ("Windows protected your
  PC", "More info", "Run anyway").
- The Smart App Control claim in `BETA.md:59` ("no Run anyway button"). Windows can allow an app
  while Smart App Control is in Evaluation mode, so I wrote "usually no Run anyway button" and
  told the reader to check the setting. No code path handles Smart App Control.
- The exact Inno Setup message when Windows is older than 10.0.19041 (`EZLM.iss:33`). I wrote
  "needs Windows 10 version 2004 or later" without quoting a dialog.
- Whether installed files keep Mark of the Web. I described the Properties > Unblock route as a
  fallback only, since it is standard Windows behaviour for downloaded files.

## Open questions

- Should the public guide recommend antivirus exclusions at all? It reduces protection and the
  right folders differ between the app and the engine (finding 2).
- Does the beta download page stay on GitHub releases (`internal/update/update.go:22`), or will
  there be a short link? My section links GitHub directly.
- `version.Version` is `0.1.11-beta` (`internal/version/version.go:5`) while the installer says
  `0.1.11` (`EZLM.iss:4`). The Settings line will therefore show `0.1.11-beta`. Worth one
  sentence in a08 if update versions are discussed.

## Checks run

`gofmt -l .` empty, `go build ./...` passed. No Go files changed. Branch `agent/T13-a01`, commit `T13: document the Windows install, SmartScreen and antivirus path`.
