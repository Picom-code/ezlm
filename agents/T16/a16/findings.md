# T16 severity table (agent a16)

Every finding from the T16 review, ranked. `file:line` is the baseline snapshot
(`agent/T16-a16`, commit 5213370). "Verified" says what was actually run on this
Mac (macOS 26.3.1, Apple M3 Pro, 18 GB, Swift 6.2) rather than read.

Baseline on this Mac: `cd macos && swift build` passes (11 s) and
`swift test` passes (22 tests, 0 failures). So none of these break the build;
they are behaviour, distribution and robustness findings.

Severity: **blocker** = beta users cannot install or run it correctly;
**major** = a real failure or data loss on ordinary Macs; **minor** = polish,
or a bug behind an unlikely path.

| # | Severity | Area | Finding | Where |
|---|---|---|---|---|
| 1 | blocker | signing | Ad-hoc signature by default, unsigned/un-notarized DMG | `macos/scripts/build-app.sh:73`, `macos/scripts/make-dmg.sh:20` |
| 2 | blocker | signing | TCC grants (Automation, files) do not survive an ad-hoc update | `build-app.sh:73`, `Updater.swift:30-66` |
| 3 | major | updates | Relaunch can start a second EZLM while the first is still asking "Quit?" | `Settings.swift:84-92`, `Updater.swift:73-87`, `EZLMApp.swift:47-55` |
| 4 | major | updates | The downloaded DMG is installed without any signature or version check | `Updater.swift:18-66` |
| 5 | major | chat perf | Whole transcript re-rendered and re-parsed at 15 fps while streaming | `ChatView.swift:155,165,286-288`, `ChatModel.swift:156-164` |
| 6 | major | SSE | Event stream can hang for up to an hour after sleep/wake; 2 Hz retry while dead | `Engine.swift:154-174` |
| 7 | major | memory | Free memory over-reported; model selection is optimistic on small Macs | `internal/scan/scan_darwin.go:18-27,52` |
| 8 | major | launchd | Plist pins the engine path; an app that moves leaves a dead login agent | `internal/serve/service_darwin.go:47-87` |
| 9 | major | translocation | Read-only/translocated app has no in-place update path beyond a Finder drag | `Updater.swift:18-20,102-109`, `Settings.swift:65-102` |
| 10 | minor | chat perf | Every save encodes and writes the whole conversation on the main actor | `ChatData.swift:71-77`, `ChatModel.swift:621-634` |
| 11 | minor | chat perf | First open of Chat reads up to 40 chat files synchronously on the main thread | `ChatData.swift:80-89`, `ChatModel.swift:126-131` |
| 12 | minor | chat | A memory-notes block bigger than the model's room fails every request | `ChatModel.swift:543-569` |
| 13 | minor | i18n | No localization resources at all; every string is English | `macos/Sources/EZLM/*.swift` (no `.xcstrings`/`.lproj`) |
| 14 | minor | animation | Signal wave freezes while a menu or window drag is tracking | `SignalWave.swift:42-47` |
| 15 | minor | scan | Apple Silicon bandwidth table has at least one wrong entry (M3 Ultra) | `internal/scan/scan_darwin.go:72-78` |
| 16 | minor | launchd | stdout and stderr point at the same log file | `service_darwin.go:72-78` |
| 17 | minor | engine | 30 s engine-start timeout is tight on a cold, Gatekeeper-scanned first run | `Engine.swift:69` |
| 18 | minor | data | `nonisolated(unsafe)` prefs cache is reachable from more than one thread | `ChatData.swift:132,138-151` |

---

## 1. Ad-hoc signature and un-notarized DMG (blocker)

`build-app.sh:73` signs with `codesign --force --deep --sign "${SIGN_ID:--}"`,
so a release built without `SIGN_ID` is ad-hoc. `make-dmg.sh:20` packs the app
with `hdiutil create` and never signs, notarizes or staples the image; there is
no `notarytool` or `stapler` anywhere under `macos/scripts/`.

What a beta user sees: the DMG is quarantined, Gatekeeper cannot match an
ad-hoc signature to a Developer ID, and on macOS 15+ the first open is blocked
with "Apple could not verify EZLM is free of malware". The workaround is
right-click > Open, which the app never tells them. `--deep` is also deprecated
and does not sign `Contents/Resources/ezlm-engine` as its own Mach-O; the engine
inherits nothing useful.

Fix: Developer ID Application certificate, `--options runtime` (hardened
runtime), sign the engine binary explicitly before the bundle, then
`xcrun notarytool submit --wait` and `xcrun stapler staple` on the DMG. Keep
`CFBundleIdentifier` `com.ezlm.app` stable.

Verified: code-read on this Mac; `grep -n "codesign\|notarytool\|stapler"` over
`macos/scripts` returns only the one ad-hoc `codesign` line. Not verified on a
clean Mac (no second machine here).

## 2. TCC grants do not carry across ad-hoc updates (blocker)

An ad-hoc signature has no stable designated requirement: the identity is the
CDHash, and every build changes it. macOS keys Automation (Apple Events, which
Model mode's force-quit needs) and Files-and-Folders grants to the code
identity, so after each update the user is re-prompted, and a denied prompt
from an earlier build can leave Automation permanently off in System Settings.

Fix: same as #1. A Developer ID identity with a stable Team ID keeps the
requirement stable and the grants stick.

Verified: code-read. `NSAppleEventsUsageDescription` is present
(`build-app.sh:68`), so the prompt itself is correctly declared.

## 3. Update relaunch can leave two copies running (major)

`Settings.swift:88` calls `Updater.relaunch(app)`, which spawns
`/bin/sh -c 'sleep 1; ... wait up to 10 s ...; open "$2"'` and then
`NSApp.terminate(nil)` (`Updater.swift:73-87`). But terminate goes through
`AppDelegate.applicationShouldTerminate` (`EZLMApp.swift:47-55`), which shows a
modal alert whenever `quitWarning` is non-nil. `quitWarning` still returns the
Chat warning while an answer is streaming (`AppModel.swift:195`), and
`updateInstalling` only suppresses the update warning. If the user is chatting
during an update install, the alert appears, the shell's 10 s wait expires, and
`open` starts a second EZLM next to the first. Two instances mean two engines,
two chat writers on the same files, and two login agents.

Fix: set a flag that makes `applicationShouldTerminate` return `.terminateNow`
during the updater's relaunch, and add `LSMultipleInstancesProhibited` to the
Info.plist as a backstop.

Verified: code-read of the three call sites. Not reproduced by launching an
update (no release DMG on this Mac).

## 4. Update install trusts whatever DMG the engine downloaded (major)

`Updater.install` checks only that the image contains `EZLM.app` and that its
`CFBundleIdentifier` matches (`Updater.swift:31-40`). It does not verify the
code signature, the Team ID, the notarization ticket, or that the version is
newer. A truncated or substituted download is copied over the running app and
launched.

Fix: `SecStaticCodeCheckValidity` with `kSecCSCheckAllArchitectures` on the
staged copy, plus `CFBundleShortVersionString` greater than the running one,
before the rename swap.

Verified: code-read.

## 5. Long chats re-render the whole transcript at 15 fps (major)

`ChatModel.frame()` (`ChatModel.swift:156-164`) advances `shown` 15 times a
second while an answer streams, and `shown` is observed. `ChatView` renders the
transcript in a plain `VStack` (`ChatView.swift:155`) with
`ForEach(c.messages)` (156), and every `MessageView` re-parses its markdown on
each pass (`ChatView.swift:286-288` -> `MarkdownView`). `onChange(of: c.tick)`
(165) also scrolls to the end on every tick. With a few hundred messages this
is a full re-layout plus a markdown parse of every visible answer, 15 times a
second, on the main thread.

Fix: `LazyVStack`, and pass the typewriter progress only to the last message
(for example a small `StreamingText` subview that owns `shown`), so finished
messages are not re-evaluated. Caching the parsed blocks per message is the
second step.

Verified: code-read. `swift build` and `swift test` pass, so this is a
performance claim, not a build failure; no Instruments run inside the time box.

## 6. Event stream reconnect (major)

`Engine.events()` (`Engine.swift:154-174`) loops forever: request with
`timeoutInterval = 3600`, then on any failure sleep 500 ms and retry. Two
problems. After sleep/wake, a half-open connection is not guaranteed to raise
an error, so the app can sit up to an hour before it reconnects to a restarted
engine. And while the engine is dead, the loop issues a request every 0.5 s
with no backoff, for as long as the failed screen is up.

Fix: cap the per-attempt timeout at ~30 s for the SSE request (the stream is
`data:` lines, so a short idle timeout is safe), and use a backoff from 0.5 s to
~10 s. `AppModel.engineDied` (`AppModel.swift:273-290`) could also restart the
listener instead of leaving it spinning.

Verified: code-read. Not exercised against a real engine restart (the brief
forbids starting llama-server).

## 7. Free memory is over-reported (major)

`scan_darwin.go:18-27` takes `memory_pressure`'s "free percentage" and does
`h.RAMAvail = max(h.RAMAvail, ...)`, so the kernel estimate can only raise the
figure, never lower it. Measured on this Mac under load:

- `memory_pressure`: 43% free -> the engine's `scan` prints "18.0 GB total,
  7.9 GB free" and offers models up to ~4.7 GB as fitting.
- `vm_stat`: free 3851 + inactive 272055 + speculative 791 pages (16 KiB) =
  about 4.3 GB reclaimable; `top -l 1` reports "198M unused" and 6.2 GB in the
  compressor, with 342M swapins.

So the app advertises roughly twice the memory it can actually get, which is
exactly the 8 to 16 GB case the beta cares about: it picks a model that then
swaps. `GPUUsable` (`:39-48`) inherits the same number through
`MemFree: min(usable, h.RAMAvail)` (`:52`).

Fix: use the smaller of the two estimates (`min`, not `max`), or compute
available as free + inactive + speculative from `vm_stat` and treat
`memory_pressure` only as a pressure flag. Consider leaving ~1.5 GB for macOS
on machines at or below 16 GB.

Verified: measured on this Mac with `memory_pressure`, `vm_stat`, `top -l 1`,
and a real `./ezlm scan` run (built with `go build ./cmd/ezlm`, no model
loaded).

## 8. launchd plist pins the engine path (major)

`service_darwin.go:49-87` writes `ProgramArguments` with the absolute
`c.Server` path of the engine that installed it. If the app is moved (DMG drag,
translocation, a rename, an update that changes the path), the agent still
points at the old path, fails at login, and `KeepAlive` keeps retrying it every
10 s.

Fix: resolve the engine from the app bundle at login (a small wrapper script in
`~/Library/Application Support/EZLM/`), or re-install the agent when the app
path changes at launch.

Verified: code-read; the plist template is literal (`:51-52,60-62`).

## 9. Translocated app and the update path (major)

`AppPlace.temporary` (`Updater.swift:102-109`) detects
`/AppTranslocation/` and a read-only volume, and the UI shows a note
(`Wizard.swift:100`, `MainView.swift:146`). But `installUpdate`
(`Settings.swift:65-102`) still tries `Updater.install` first, which copies next
to the app and renames inside the read-only DMG mount, so it always throws and
falls back to opening the DMG in Finder. That is a working path, but it means a
first-run user who launches from the DMG gets an update prompt they cannot
complete in place, and the fallback message ("Drag EZLM into Applications")
appears only after the failed attempt.

Fix: skip `Updater.install` when `AppPlace.temporary` and go straight to the
drag message, or move the app to `/Applications` on first launch.

Verified: code-read of both branches; `AppPlace.temporary` is false for a
normal build here, so not exercised.

## 10. Saving a chat blocks the main actor (minor)

`ChatModel.save()` (`:621-634`) maps every message, and `ChatStore.save`
(`ChatData.swift:71-77`) JSON-encodes the whole conversation and writes it
atomically, on the main actor, at the end of every answer and on every restore
or new chat. `Data.write(options: .atomic)` also rewrites the file. On a long
chat this is a visible hitch after each answer.

Fix: encode off the main actor and write through a serial background queue,
coalescing writes.

Verified: code-read; `swift test` covers `save`/`restore` correctness
(`ChatTests.testChatsSaveAndComeBack`), not latency.

## 11. First Chat open reads 40 files on the main thread (minor)

`restoreOnce` (`ChatModel.swift:126-131`) calls `ChatStore.recent()`, which
lists the directory, sorts, and decodes up to 40 JSON files with
`Data(contentsOf:)` (`ChatData.swift:80-89`), synchronously on the main actor
during `onAppear`.

Fix: load recent chats in a background task and publish them, as the History
list already tolerates an empty first frame.

Verified: code-read.

## 12. Notes larger than the model's room fail every request (minor)

`fit` (`ChatModel.swift:543-569`) subtracts the memory notes from the room.
If `notes >= room(ctx)` the room goes to zero or negative, the newest turn is
kept, and the notes are still inserted (`:216-221`), so every request is over
budget. The refusal loop squeezes three times (`:229-233`) and then shows
"more than the model can read at once". The notes are capped at 1500 chars, so
this needs a small-context model plus a full memory, which is plausible on
8 GB.

Fix: drop or trim the notes to fit before the first request, and report that
in the UI.

Verified: code-read. `ChatTests.testLongChatsFit` covers the normal trimming
path only.

## 13. No localization resources (minor)

There is no `.xcstrings`, `.strings` or `.lproj` under `macos/`; all UI text is
English literals in the Swift sources. For an English-only beta this is
acceptable, but the brief calls for localization review, and adding strings
later means touching every view. Layout is mostly direction-agnostic (SwiftUI
`.leading`/`.trailing` are semantic and flip under RTL), but the monospaced
terminal aesthetic, the braille wave and code blocks assume a left-to-right
run.

Fix: extract to a String Catalog before beta; add a pseudo-RTL pass over Chat.

Verified: `find macos -name "*.xcstrings" -o -name "*.lproj"` returns nothing.

## 14. Wave animation stops during menu tracking (minor)

`SignalWave.start()` (`:42-47`) uses `Timer.scheduledTimer`, which adds the
timer to the default run-loop mode only. While a menu is open or a window is
being dragged, the mode is `.eventTracking` and the wave freezes; the typewriter
also stops, because `onFrame` is driven by the same timer.

Fix: `RunLoop.main.add(timer, forMode: .common)`.

Verified: code-read.

## 15. Apple Silicon bandwidth table (minor)

`appleBandwidth` (`scan_darwin.go:56-83`) returns 819 GB/s for M3 Ultra; Apple
rates it at 800. M5 entries cannot be checked here. The fallback for an unknown
chip is 100 GB/s, which mislabels anything newer than M5.

Fix: correct the M3 Ultra entry and fall back to the base tier rather than 100.

Verified: code-read; the M3 Pro path returns 150, which matches Apple's spec.

## 16. launchd stdout and stderr share one file (minor)

`service_darwin.go:72-78` sets both `StandardOutPath` and `StandardErrorPath`
to `LogPath()`. launchd opens both descriptors without `O_APPEND` in some
versions, so interleaved writes can overwrite each other and the log reads as
garbage after a crash.

Fix: give stderr its own file, or wrap the server in a shell that redirects with
`>>`.

Verified: code-read.

## 17. 30 s engine start timeout (minor)

`Engine.swift:69` fails the boot if the engine has not printed its hello line
within 30 s. On a cold first launch from a quarantined DMG, Gatekeeper scans
the universal engine binary before it runs; that plus a slow 8 GB Mac can eat
into the budget.

Fix: 60 s, with a "still starting" note after 20 s.

Verified: code-read. On this Mac the engine starts in well under a second.

## 18. Unsynchronized prefs cache (minor)

`Memory.prefsCache` is `nonisolated(unsafe)` (`ChatData.swift:132`) and read or
written from `Memory` statics that are called both from `@MainActor` chat code
and from tests. Today every production caller is main-actor, so this is latent.

Fix: mark `Memory` `@MainActor`, or make the cache a lock-protected box.

Verified: code-read; `swift test` runs the same code single-threaded.

---

## What was verified on this Mac, and what was not

Verified by running:

- `cd macos && swift build` (passes, 11 s) and `swift test` (22 tests, 0
  failures) at baseline commit 5213370.
- `go build ./cmd/ezlm` and `./ezlm scan` (no model load): reproduces finding
  #7's numbers.
- `memory_pressure`, `vm_stat`, `top -l 1`, `sw_vers` (macOS 26.3.1, M3 Pro,
  18 GB) for #7.
- `find`/`grep` evidence for #1, #2, #13.

Not verified here (needs a clean Mac, a second machine, or a real release):

- Gatekeeper and TCC behaviour on a quarantined DMG (#1, #2): code-read only.
- An actual update install and relaunch (#3, #4, #9): code-read only.
- Engine restart and sleep/wake recovery (#6): code-read only; the brief forbids
  starting llama-server.
- Instruments measurements for #5, #10, #11: code-read only.
- Login-agent behaviour after moving the app (#8, #16): code-read only.

## Open questions

- Is the beta shipping ad-hoc builds on purpose (#1)? If so, the first-run text
  needs to say "right-click, Open".
- Is `memory_pressure`'s free percentage the intended source of truth (#7)? The
  comment says it is "what macOS acts on", but it disagrees with `vm_stat` by
  about 2x on this Mac under load.
- Does the release job set `SIGN_ID` and run notarization outside these scripts?
  Nothing in `macos/scripts` does.
