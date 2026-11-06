# T16 a03: app translocation and running from the DMG

Reviewed: `macos/Sources/EZLM/Updater.swift`, `Settings.swift` (updates), `Wizard.swift`,
`MainView.swift` (Overview), `ChatData.swift`, `Engine.swift`, `EZLMApp.swift`,
`scripts/build-app.sh`, `scripts/make-dmg.sh`, `BETA.md`, and the engine side of paths
(`internal/serve/service_darwin.go`, `~/.ezlm`).

## What already worked

- The DMG (`scripts/make-dmg.sh`) has the `/Applications` symlink, so installing is a drag.
- Every write the app makes is outside the bundle: chats, memory and prefs go to
  `~/Library/Application Support/EZLM` (`ChatData.swift:8`), the engine to `~/.ezlm`, the
  launch agent to `~/Library/LaunchAgents`. A read-only, translocated bundle still works.
- The engine is spawned from `Bundle.main` Resources (`Engine.swift:29`) and only ever read.
- The first-run wizard shows `MoveToApplicationsNote` on every step (`Wizard.swift:100`) and
  Overview shows it too (`MainView.swift:146`), so a user who never drags the app is told.

## Findings

1. **Major: an update from the download cannot install, and the fallback leaves the old app
   running.** Running from the DMG or from macOS's App Translocation copy, `Bundle.main.bundleURL`
   is read-only. `Updater.install` staged the new copy next to it (`Updater.swift:34`), so
   `ditto` failed, and `installUpdate` (`Settings.swift:92-101`) fell back to opening the DMG
   with "Drag EZLM into Applications to replace the old one, then open it again." Two problems:
   the whole DMG was downloaded for nothing, and when the user does drag the new app in, `open`
   activates the still-running translocated instance (same bundle id), so they stay on the old
   version and the update repeats. Fixed (below).
2. **Minor: the translocation check was case-sensitive** (`Updater.swift:105`, `contains("/AppTranslocation/")`).
   The path comes from macOS and is spelled that way today, but a comparison that does not depend
   on case is one less way to miss it.
3. **Minor: `Updater.install` assumed a bundle already existed at the destination** (unconditional
   `moveItem(at: app, to: old)`, old line 44). That only held because the destination was the
   running app. It is now conditional, which the new Applications destination needs.
4. **Minor, frozen file:** `MoveToApplicationsNote` (`Settings.swift:424`) says "Drag it into
   Applications" but only opens Finder; the app could copy itself and reopen from Applications.
   Left alone because `Settings.swift` is frozen; the update path now does exactly that move
   (see fix), which is the case where it matters most.

## What I changed

- `macos/Sources/EZLM/Updater.swift`
  - `AppPlace.isTemporary(path:readOnly:)`: pure, testable classifier. Case-insensitive
    App Translocation match, then the volume's read-only answer. `AppPlace.temporary` is now
    that function on `Bundle.main.bundleURL`.
  - `Updater.destination(for:readOnly:)`: an installed app updates in place; an app running
    from the download updates into `/Applications/EZLM.app`.
  - `Updater.install(dmg:over:)` uses that destination and only moves an old copy aside when one
    exists, so a first update from the DMG installs cleanly. The bundle-id check now also
    compares against the running app when Applications has no copy yet.
  - `Updater.relaunch` reopens the destination, so after updating from the DMG the user lands on
    the new `/Applications/EZLM.app` instead of the translocated old one.
- `macos/Tests/EZLMTests/UpdaterTests.swift` (new, one test file for this agent): 5 tests for
  the classifier and the destination choice. All 27 tests pass.

`cd macos && swift build && swift test` pass.

## Not verified on a device

- The full update flow needs a real DMG and a writable `/Applications`: not exercised here
  (no DMG was built, no engine started).
- A standard (non-admin) user cannot write `/Applications`; the update then still falls back to
  the old drag message. A "Move to Applications" button that copies with authentication would
  close that gap; it belongs in the frozen `MoveToApplicationsNote`.
- Translocation was checked by path shape and by reading the code, not by launching a
  quarantined copy.

## Open questions

- Should the app offer to move itself to Applications at the end of setup (one button) instead of
  asking the user to drag it? The update path now proves the copy works.
- If a user deliberately keeps EZLM on an external writable volume, updates stay in place there.
  That is intentional; a DMG on the same volume is still detected by read-only.
