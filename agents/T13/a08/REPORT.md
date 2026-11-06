# a08 report: updating and uninstalling on both platforms

Section written to `agents/T13/sections/a08.md` (about 990 words). Docs only; no code changed.

## What I covered

- How EZLM checks for updates (on launch, cached one hour, manual check), what the
  Settings > Updates screen shows, and the exact wording when the check fails.
- The download: temp folder, SHA-256 check against GitHub's digest, what happens on a
  mismatch, resume of a complete file.
- Install on Windows (silent installer with `/UPDATE=1`, EZLM quits and is reopened,
  the fallback that reopens the installed copy if setup stops early) and on Mac
  (mount, bundle check, stage, swap, relaunch, the "drag EZLM into Applications"
  fallback).
- Updating while running from the DMG or Downloads (translocation) and what to do.
- Windows uninstall: both routes (Installed apps, and the in-app System button), the
  installer's question and its default answer, what Yes and No each remove, what stays
  behind, and the startup entry.
- Mac uninstall: the in-app button, dragging to the Trash, what stays behind.
- Reinstalling with kept data.

Every claim has a numbered footnote with `file:line` in the section itself.

## Findings

1. **major, both platforms:** "Remove everything" does not remove chat history or
   memory. The engine's uninstall deletes only `~/.ezlm` and model folders
   (`internal/app/app.go:1586-1606`); chats live in `%LOCALAPPDATA%\EZLM`
   (`windows/EZLM/Engine.cs:26-34`, `windows/EZLM/ChatStore.cs:5-7`) and
   `~/Library/Application Support/EZLM` (`macos/Sources/EZLM/ChatData.swift:5-13`).
   The dialog says "This can't be undone" (`windows/EZLM/Panes.cs:543-546`,
   `macos/Sources/EZLM/MainView.swift:536-537`), so a tester who uninstalls for
   privacy leaves the text they typed to the model on disk. The guide tells them to
   delete the folder by hand; T12 may want a wording change ("also delete chats").
2. **minor, Mac:** dragging EZLM to the Trash without pressing "Remove models and
   settings" leaves `~/Library/LaunchAgents/com.ezlm.server.plist`
   (`internal/serve/service_darwin.go:14-18`) and all of `~/.ezlm`. The login item
   then points at an app that is gone. `BETA.md:55` says to press the button first,
   so this is a missed-step case, not a contradiction.
3. **minor, both:** downloaded update installers are never cleaned up. They stay in
   `<temp>/EZLM-update` (`internal/update/update.go:112`), which on Windows is
   `%TEMP%` and on Mac is `$TMPDIR`; the OS clears them eventually. A few hundred MB
   can sit there between updates.
4. **minor, both:** right after the in-app "Remove models and settings", the app
   rescans and `scan.Scan` calls `paths.Ensure()` (`internal/scan/scan.go:103`), which
   recreates an empty `~/.ezlm` with `bin`, `models` and `logs`. The folder reappears
   after the user was told everything was removed.
5. **minor, docs:** `BETA.md:53` does not say the installer's model-deletion question
   defaults to **No** (`windows/installer/EZLM.iss:94-95`, `MB_DEFBUTTON2` with
   `IDNO`). A tester who presses Enter keeps several GB of models.
6. **minor, docs:** `BETA.md:38` promises "your model keeps running" during an
   update. On Windows that is explicit in the UI (`windows/EZLM/SettingsPane.cs:61`)
   and the model server runs from `~/.ezlm/bin`, outside the app folder. On Mac the
   app bundle is replaced while the launchd job keeps the old process alive, which
   should also hold, but I could not run the app to confirm it.

## What I could not verify

- The apps were not run (no Windows machine, and the brief forbids loading a model),
  so UI flows are read from the C#/Swift source, not observed. Wording quoted in the
  section is copied from the code.
- Whether macOS shows an error at login when the launchd plist points at a trashed
  app (finding 2). Plausible from `service_darwin.go:14-18` but not observed.
- Whether Inno Setup's `CloseApplications` reliably closes a running EZLM before the
  in-app update (`windows/installer/EZLM.iss:28-32`); the app closes itself after
  1.2 s (`windows/EZLM/SettingsPane.cs:306`), so the race is narrow.

## Open questions for the Lead

- Should the guide tell testers to delete `%LOCALAPPDATA%\EZLM` /
  `~/Library/Application Support/EZLM` before uninstalling if they want chats gone?
  I put it after the uninstall steps; a14/T12 may prefer a code fix instead.
- The FAQ (a13) should answer "can I delete it" with the two data folders, not just
  the app folder. Cross-check when assembling.

## Commands run

`gofmt -l .` and `go build ./...` were not needed: no Go files were touched. The
worktree is docs only.
