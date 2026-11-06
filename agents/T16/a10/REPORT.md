# T16 a10: app state and menus (window restoration, multi-window, menu commands)

Reviewed `macos/Sources/EZLM/EZLMApp.swift` (the whole scene/delegate/menu setup), the window
and quit paths it calls, and the state that survives a relaunch (`AppModel.enterMain`,
`ChatModel.restoreOnce`, `Updater.relaunch`, `Wizard.CloseGuard`). No GUI run: findings are from
the code, and the new helpers are unit-tested.

## Findings

### 1. Boot runs once per window, not once per launch (major)

`EZLMApp.swift:15` (main) attaches `.task { delegate.model = model; await model.boot() }` to the
`WindowGroup` content. `.task` runs every time the view appears, not once per app launch.

Reachable path: the app quits when the last window closes (`EZLMApp.swift:42`,
documented in `Wizard.swift:21-23`). While something is running that is not covered by
`CloseGuard` (`Wizard.swift:17` covers install, storage and update only), for example a streaming
chat answer (`AppModel.swift:195`) or an integrate job, the user presses Cmd-W. The window closes,
`applicationShouldTerminate` asks, and "Keep Going" cancels the quit but leaves the app running
with no window and the job still alive. Clicking the Dock icon opens the window again, the view's
`.task` runs, and `boot()` calls `engine.start()`, which stops the engine first
(`Engine.swift:34`), killing the running job and resetting the UI to `.booting`.

The same happens with a second window (Dock menu, or any future `openWindow`): every window
would boot the shared `AppModel` again.

Fix: `LaunchGate` (`AppState.swift`), taken once in `.task` before `boot()`. Tested.

### 2. An update can be left half-applied by the quit prompt (major, edge)

`Updater.relaunch` (`Updater.swift:59-68`) puts the new app in place and then calls
`NSApp.terminate(nil)`. `applicationShouldTerminate` (main `EZLMApp.swift:46-54`) asked whenever
`quitWarning` was non-nil, and `quitWarning` includes `chat.streaming` (`AppModel.swift:195`).
The update download runs in the background (`Settings.swift:30-60`), and the install starts the
moment it finishes (`Settings.swift:84-90`), so a user answering a chat question can get
"Quit EZLM?" at that moment. "Keep Going" cancels the termination after the bundle has already
been replaced; the relaunch script then only re-opens/activates the old process (same bundle ID),
so the update does not restart the app and the running process is from the replaced bundle until
the next manual launch.

Fix: `applicationShouldTerminate` returns `.terminateNow` while `model.updateInstalling` is true,
because the swap is already committed and the relaunch is waiting for the process to exit.

### 3. Window chrome is only applied to windows that exist at launch (minor)

Main `EZLMApp.swift:35-39` styled `NSApp.windows` in `applicationDidFinishLaunching`. A window
created after launch (the reopened window from finding 1) missed `backgroundColor = .black`,
`titlebarAppearsTransparent` and `isMovableByWindowBackground`: the strip behind the hidden title
bar shows the system gray instead of the app's black, and dragging by the background stops
working.

Fix: the same styling is applied from `NSWindow.didBecomeKeyNotification`, skipping sheets and
panels so alerts and sheets are not restyled.

### 4. No Settings command (minor)

Main `EZLMApp.swift:23` removed New Item but there is no `Settings` scene and no
`CommandGroup(replacing: .appSettings)`, so the app menu has no "Settings…" and Cmd-, does
nothing, even though Settings is a page (`Section.settings`, `AppModel.swift:7`).

Fix: "Settings…" (Cmd-,) selects the Settings page and brings the window forward.

### 5. Engine stop was tied to a view that can be gone (minor)

Main `EZLMApp.swift:16-18` stopped the engine from a `willTerminate` subscription on the window
content. After a cancelled quit with no window (finding 1) the view is gone, so the only thing
stopping the engine is its stdin EOF (`Engine.swift:4-6`). Moved to
`applicationWillTerminate`, which always runs and is idempotent (`Engine.stop`).

### 6. The selected page is not restored (minor)

`AppModel.enterMain` always sets `section = .overview` (`AppModel.swift:703`). Window
restoration brings the window frame back, but the app always reopens on Overview, even when the
user was in Chat or Logs.

Fix: `SectionMemory` (`AppState.swift`) saves the section in `UserDefaults` and the boot task
restores it after `boot()`. A stored value from a section this build no longer has is ignored.

### 7. Quit warning can be skipped very early (minor, not fixed)

`applicationShouldTerminate` reads `model?.quitWarning`, but `delegate.model` is only assigned
inside the view's `.task` (main `EZLMApp.swift:15`). A Cmd-Q in the first moments after launch
gets no warning. The window is a few milliseconds wide; left as is.

## Open questions

- Window frame restoration is left entirely to SwiftUI (`WindowGroup` + `.defaultSize`). It was
  not tested here, including the case of a frame saved on a display that is no longer connected.
  A manual check on the checklist is worthwhile (a10/a15).
- The app is single-window by design (New Item removed) but uses `WindowGroup`. A second window
  shares one `AppModel`, so it shows the same page and chat. With the boot guard it no longer
  restarts the engine, but true multi-window would need `Window("EZLM", id:)` or per-window
  models. Left as an open question, not changed.
- After "Keep Going" on the quit prompt the app is left with no window. Reopening from the Dock
  relies on SwiftUI's default `applicationShouldHandleReopen` handling; not verified here.

## Changes

- `macos/Sources/EZLM/EZLMApp.swift`: boot gate, section restore, Settings… command, window
  styling for later windows, `applicationWillTerminate`, update-install quit bypass.
- `macos/Sources/EZLM/AppState.swift` (new): `LaunchGate`, `SectionMemory`.
- `macos/Tests/EZLMTests/AppStateTests.swift` (new): 3 tests for the gate and section memory.

## Verified

- `cd macos && swift build` passes.
- `cd macos && swift test` passes: 25 tests, 0 failures (22 existing, 3 new).
- `gofmt -l .` empty, `go build ./...`, `go vet ./...` pass.

The boot-once and chrome fixes are SwiftUI/AppKit lifecycle behaviour, so the tests cover the
gate and the stored section; the window behaviour itself needs the manual checklist.
