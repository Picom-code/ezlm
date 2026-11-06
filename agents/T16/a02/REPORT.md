# T16/a02: SSE reconnect after sleep/wake and engine restarts

Scope: the chat's stream from the local model server (`ChatModel.stream`) and what
happens to it when the Mac sleeps or the engine / model server restarts. Reviewed
`ChatModel.swift`, `ChatView.swift`, `Engine.swift`, `AppModel.swift` (read only),
`ChatData.swift`, `EZLMApp.swift`.

## What I found

### 1. Major: a dropped stream before the first word became an error, with no retry
`ChatModel.swift:251-312`. Every `URLError` of the connection class
(`cannotConnectToHost`, `networkConnectionLost`, `timedOut`) went straight to an
error message, even when the model had written nothing yet. That is exactly the
state after a Mac wakes from sleep (a socket that died while asleep) and after a
model-server restart (switch model, restart from Overview). The person had to
press Retry; before that the chat sat there with a red "Couldn't reach your
model" for an answer that never started.
Fix (below): ask again, up to three times, only while the answer is still empty,
and give a restarting server a moment to come back first.

### 2. Major: a stale socket after wake can spin for up to 15 minutes
`ChatModel.swift:78` sets `timeoutIntervalForRequest = 900` ("a big prompt on a
slow Mac can take minutes before the first word"). URLSession applies that as the
wait for the *next* byte, so a connection that died in sleep and never delivers a
FIN is not reported until 900 s. In that time `streaming` stays true and the
answer bubble shows `reading ⠋` (ChatView.swift, `MessageView.state`), so the
chat looks like it is working when it is waiting on a dead socket. Stop (esc)
still works, so it is recoverable, but it is a real "does it spin: yes" case.
Not fixed here: the reliable fix is to watch `NSWorkspace.didWakeNotification`
and restart a stream that has received nothing, which needs a real sleep/wake to
verify. Recorded as the next step.

### 3. Minor: Retry after a drop throws away the partial answer
`ChatModel.swift:396-403` (`retry`) removes the question and the failed answer
and sends again. After a mid-answer drop the answer already has text on screen,
but Retry deletes it and the question is re-asked from scratch. The partial
answer is not lost from disk (`save()` in `finish` keeps it), only from the chat.
Suggested fix: also offer Continue when a dropped answer has text, i.e. extend
the condition at `ChatView.swift:301` (`msg.cut || msg.stopped`) with
`|| !msg.error.isEmpty`. Not changed here to keep the patch small.

### 4. Minor: a quit in the middle of an answer lost the words that had arrived
`send()` saves the question with an empty answer (`ChatModel.swift:168-190`);
`finish()` is the only save during a stream. Quitting or crashing mid-answer left
the restored chat with a blank answer and no error, so the partial text was gone.
Fixed (below).

### 5. Minor: sending while the server is not live does nothing, silently
`ChatModel.swift:168-176`: the `!streaming, live` guard returns without a word.
The Send button is disabled, but Return in the text field still calls `send()`,
so during a model restart the person presses Return and nothing happens. The
draft is kept, so it is a small annoyance, not data loss.

### 6. Note: the engine's own event stream reconnects on its own
`Engine.swift:152-176` loops with a 0.5 s pause and re-reads `base`/`token` each
time, so it follows an engine restart and does not spin hot (connection refused
on loopback fails immediately). `AppModel.engineDied` (`AppModel.swift:282-290`)
shows its own Try again screen. No change needed; the chat is the part that did
not recover.

## What I changed

- `Sources/EZLM/ChatModel.swift`
  - `stream(...)` takes an internal `tries` count. A connection-class `URLError`
    now goes through `retryable` (line 327): retry only while the answer and the
    thinking are both empty, at most three times, and only for a connection that
    died. Stop / `cancelled`, bad URLs and server errors are not retried.
  - `waitForServer()` (line 340) waits up to 15 s for a server that is
    `loading` (a restart) to be live again, and only 1 s for a stopped server,
    so a stopped model still reports an error quickly instead of spinning.
- `Sources/EZLM/EZLMApp.swift:16-19`: save the chat on
  `NSApplication.willTerminateNotification` before stopping the engine, so a
  quit mid-answer keeps the words that arrived. Shared file: one added line.
- `Tests/EZLMTests/ReconnectTests.swift` (new, mine): the retry policy. Covers
  retry before the first word, no retry after text or thinking has arrived, the
  three-try bound, and that Stop and server errors are not retried. The two
  tests fail to compile before the change (no `retryable`) and pass after.

## Verification

- `cd macos && swift build` passes; `swift test` passes, 24 tests, 0 failures
  (14 ChatTests, 2 ReconnectTests, 1 ReportTests, 7 TargetTests).
- Not verified on a real sleep/wake or a real engine restart: both need the app
  and a model, and no model may be loaded on this machine. The retry path itself
  is exercised only through the policy function, not end to end.

## Next steps

1. Watch `NSWorkspace.didWakeNotification` from `ChatPane` and, when a stream has
   received nothing since before the wake, cancel and start it again (a fresh
   `Task`, because cancellation is sticky). That turns the 900 s wait into a
   second or two.
2. Offer Continue for a dropped answer with text (finding 3).
3. Consider lowering `timeoutIntervalForRequest` to ~180 s and relying on the
   poll's `promptDone/promptTotal` to tell a slow prompt from a dead socket.
