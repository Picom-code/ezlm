# T16 / a12: file access and sandbox

Reviewed what the Mac app writes, where it writes it, and what an in-app update
does to that data. All changes are Swift, in `macos/` (no frozen files edited).

## Where the data lives

- Chats, memory facts and memory switches: `~/Library/Application Support/EZLM/`
  (`chats/<id>.json`, `memory.md`, `memory.json`) — `ChatData.swift:5-15,49,130-131`.
  A test can point this at a throwaway folder with `EZLM_CHAT_DIR`.
- Engine data (models, tools, logs, config, the downloaded update): `~/.ezlm`
  (`internal/paths/paths.go:10-46`). The app spawns the bundled engine with the
  user's normal environment (`Engine.swift:36-63`), so the child writes there too.
- The app bundle itself is read-only data: nothing is written inside `EZLM.app`.

Both data roots are outside the bundle and are keyed to the fixed bundle id
`com.ezlm.app` (`macos/scripts/build-app.sh:59`), so an update that replaces
`EZLM.app` in place keeps every chat and every memory fact. The app is not
sandboxed (no entitlements file, no App Sandbox in `build-app.sh`), so the paths
are the real ones and no container or security-scoped bookmarks are involved.

## Findings

1. **major — an answer in progress was lost on quit (and on an update restart).**
   `ChatModel.save()` runs when a question is sent (`ChatModel.swift:185`), when
   an answer finishes (`:346`), when the chat is switched/restarted (`:375,637`),
   but there was no save on termination. The quit prompt says "what's written so
   far is kept" (`AppModel.swift:195`), which was false: the reply text streamed
   after the question was never written. The in-app update quits and relaunches
   the app (`Updater.relaunch`), so the same loss happened there.
   **Fixed** in `EZLMApp.swift:16-21`: on `willTerminate`, save the chat before
   stopping the engine.

2. **major — an update without the engine was accepted, leaving an app that
   cannot start.** `Updater.install` only checked that the new bundle had
   `Contents/MacOS` (`Updater.swift:23-26` before). A partial or corrupt release
   would replace a working app, and the relaunched app then fails at
   `Engine.binary()` with "EZLM's engine is missing from the app"
   (`Engine.swift:27-37`).
   **Fixed** in `Updater.swift:24,36-42`: `usableApp` requires both
   `Contents/MacOS` and `Contents/Resources/ezlm-engine`. Test:
   `FileAccessTests.testAnUpdateWithoutTheEngineIsTurnedDown`.

3. **major — a cancelled quit could start a second copy.** `Updater.relaunch`
   waits at most ~10 s for the old process to exit, then runs `open` anyway
   (`Updater.swift:78-83` before). If the quit prompt (shown when a chat is
   streaming) is left open or answered "Keep Going", the old app keeps running
   and a second EZLM opens. Two instances write the same `chats/*.json` and
   `memory.*`, so the last writer wins.
   **Fixed** in `Updater.swift:78-83`: wait for the process to actually exit,
   with no cap, then open. If the person keeps going, the new app opens when
   they next quit. Code-read fix; the shell script is not unit tested.

4. **minor — a crash between the two renames leaves no `EZLM.app`.** `swap`
   moves the old bundle aside and then the new one in (`Updater.swift:60-72`).
   Power loss between those two calls leaves only `.EZLM-old-<pid>.app` in
   `/Applications`. Not fixed; a launch-time recovery that renames a lone
   `.EZLM-old-*.app` back would close it.

5. **minor — crash leftovers are hidden and permanent.** `.EZLM-update-<pid>.app`
   and `.EZLM-old-<pid>.app` are dot-files, so Finder hides them; a crash
   before cleanup leaves up to a full app copy on disk. Not fixed.

6. **minor — attachments are copied into every chat save.** Attached files (up
   to 2 MB each, `ChatModel.swift:483-495`) are stored as text inside the chat
   JSON (`ChatData.swift:44-47,75`), and `ChatModel.save()` rewrites that file
   on every answer and re-reads the 40 newest chat files for the History list
   (`ChatData.swift:79-89`). A few large attachments make Application Support
   grow by megabytes and make each save rewrite it. Not fixed; noted for a01.

7. **minor — chat ids can collide.** `ChatStore.newID()` is a millisecond
   timestamp (`ChatData.swift:51-55`); two chats created in the same
   millisecond overwrite one file. Not fixed; hard to hit from the UI.

## Verification

- `cd macos && swift build` — pass.
- `cd macos && swift test` — 25 tests, 0 failures (3 new in
  `Tests/EZLMTests/FileAccessTests.swift`).
- `gofmt -l .` empty, `go build ./...`, `go vet ./...` — pass. No Go changes.
- New test `testSwappingTheAppKeepsChatsAndMemory` builds two throwaway app
  bundles and a sibling `Application Support/EZLM` folder, runs the real swap,
  and checks the chats and memory files are untouched and no `.EZLM-*` staging
  files are left behind.

## Not verified here (needs a real Mac or a clean machine)

- A real DMG update through the UI: mount, swap, relaunch, and that chats and
  memory are still listed afterwards.
- Running from the DMG / translocation (`AppPlace.temporary`), where the update
  falls back to "Drag EZLM into Applications".
- TCC / App Management permission prompts when replacing an app in
  `/Applications` (belongs to a04's signing review).
- Update while a long answer is streaming: with the new termination save, the
  partial answer should appear in History after the restart.

## Open questions

- Should "Delete all chats" also clear `memory.md`? Today it removes only
  `chats/*.json` (`ChatData.swift:95-99`). Likely intentional, but worth a
  product decision.
- Should the updater verify the new bundle's version is newer than the running
  one? It checks the bundle id only (`Updater.swift:27-29`). A re-downloaded
  older DMG would be installed as-is.
