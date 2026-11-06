# T16 a09: memory, leaks and retain cycles (app model and chat views)

Worktree `~/ezlm-agents/work/subs/T16/a09`, branch `agent/T16-a09`.
Read: all of `macos/Sources/EZLM` (with the frozen files read only), plus the
existing tests. Changed: `SignalWave.swift` (fix), new
`Tests/EZLMTests/MemoryTests.swift`. Everything below was checked on this Mac
(Swift 6.2, `swift build` and `swift test` both pass; 27 tests, 0 failures).

## Fixed

### a09-1 (minor) The signal wave's timer outlives the wave
`Sources/EZLM/SignalWave.swift:44` (`start()`) scheduled a repeating 30 Hz
`Timer` and held the wave only weakly, so nothing ended the timer except
`stop()`. The run loop owns a scheduled timer, so a `SignalWave` released
without `stop()` (a chat pane torn down without `onDisappear`, a future second
window, a test) leaves a 30 Hz timer running for the life of the app, and that
timer keeps calling `onFrame`, i.e. the chat's typewriter `frame()`
(`ChatModel.swift:156`).

Evidence: `MemoryTests.testWaveTimerStopsWhenTheWaveIsGone` holds the
`Timer`, drops the wave, and asserts the timer is no longer valid. It fails on
the old code (`Optional(true) is not equal to Optional(false)`) and passes
now. `testWaveStartIsIdempotent` also guards the `timer == nil` early return.

Fix (small): the timer block ends itself when the wave is gone
(`guard let self else { t.invalidate(); return }`), and `deinit` invalidates
the timer as well. `timer` is now `private(set)` so the test can read it; the
`isRunning` accessor is new.

## Found, not fixed (frozen file or needs a device)

### a09-2 (minor, AppModel.swift is frozen) The event listener never stops
`AppModel.swift:245`: `if !listening { listening = true; Task { await listen() } }`.
The task is not stored, so it can't be cancelled, and `await listen()` captures
`AppModel` strongly for the `for await` loop in `AppModel.swift:259`, which
only ends when the engine event stream ends (it reconnects forever,
`Engine.swift:154`). The app model therefore can never be released, even if the
engine dies (`engineDied()` cancels `polling` but not this task). Harmless for
the single-window beta; it would leak a whole model per extra window and it
makes "is the model gone?" tests impossible. Fix: keep the listener task next
to `polling` and cancel it in `engineDied()`/on deinit.

### a09-3 (minor) `ChatModel.speak` polls forever-ish and races
`ChatModel.swift:611`: a detached `Task` sleeps 0.5 s in a loop and clears
`speakingID` when the synthesizer reports it is not speaking. It holds the chat
strongly across every sleep, nothing cancels it, and if a second answer is
spoken while the first loop is between sleeps, the old loop can clear
`speakingID` for the new answer (Stop button shows the wrong state). Fix: an
`AVSpeechSynthesizerDelegate` (a small `NSObject` shim) that clears
`speakingID` in `didFinish`/`didCancel`; not done here because it needs a
class hierarchy change in a shared file.

### a09-4 (minor) Saved chats are decoded in full on every save
`ChatData.swift:80` `ChatStore.recent()` reads and JSON-decodes up to 40 whole
conversations; `ChatModel.save()` (`ChatModel.swift:633`) calls it after every
finished answer and every send, and `ChatModel.recent` keeps all 40 decoded
transcripts for the app's life. History shows 15 titles. A user with 40 long
chats holds tens of MB of duplicate text, and each answer re-reads all 40 files
on the main actor. Fix (later): decode a small header for the menu and load
messages only when a chat is restored, or cap by total bytes.

### a09-5 (minor) `ChatModel.session` is never invalidated
`ChatModel.swift:76`: the lazily created `URLSession` is never invalidated.
Apple asks for `invalidateAndCancel`/`finishTasksAndInvalidate`; without it the
session keeps its connection cache and idle connections. App-lifetime object,
so the cost is small; worth a `deinit` or an explicit invalidation on quit.

## Verified clean (tests in `MemoryTests.swift`)

- `testAppModelReleasesWhenIdle`: `AppModel` is released when nothing runs.
- `testChatAndAppModelDoNotKeepEachOtherAlive`: `AppModel.chat` is strong and
  `ChatModel.app` is weak, so there is no cycle between them; a chat is
  released with its app model.
- `testPollingDoesNotKeepTheAppModelAlive`: the `polling` task
  (`AppModel.swift:729`) captures `self` weakly and re-binds it per iteration,
  so it does not keep the model alive after a release. (My first reading
  suspected a cycle here; the test disproved it, so nothing was changed.)

## Open questions

- Does SwiftUI always call `ChatPane.onDisappear` (which stops the wave) when
  the window closes while Chat is open? I could not prove either way without
  running the app; the `deinit`/self-invalidating timer makes the answer not
  matter.
- `AppModel.showToast` (`AppModel.swift:1057`) starts a 4.2 s `Task` that
  captures the model strongly. Harmless for the root model; worth making weak
  if a second window ever gets its own model.
