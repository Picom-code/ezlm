# T16 a01: main-thread blocking and SwiftUI performance with long chats

Worktree `~/ezlm-agents/work/subs/T16/a01`, branch `agent/T16-a01`. Reviewed `ChatModel.swift`,
`ChatView.swift`, `ChatData.swift`, `Markdown.swift` (all in my ownership), plus read-only
`AppModel.swift`, `MainView.swift`, `Marks.swift` for how Chat is driven. One fix made, in
`ChatModel.save()`; the rest are measured findings with a proposed change and no patch, to keep the
diff small.

## How this was measured

New test file `macos/Tests/EZLMTests/ChatPerfTests.swift` (mine, a01). It runs against a throwaway
`EZLM_CHAT_DIR` and prints `MEASURE` lines; run `cd macos && swift test --filter ChatPerfTests`.
Numbers below are from this Mac (arm64, macOS 26, Swift 6.2, **debug build**), median of 2 to 5
repetitions, so a release build will be somewhat faster and a slow 8 GB Mac somewhat slower.

## Findings

### F1 (major, fixed): every chat save re-read the whole chat store on the main actor

`ChatModel.save()` (`macos/Sources/EZLM/ChatModel.swift:621`) wrote the chat and then did
`recent = ChatStore.recent()`. `ChatStore.recent()` (`macos/Sources/EZLM/ChatData.swift:80-88`)
lists the chat folder, stats up to 40 files, reads and JSON-decodes **all 40**, then sorts them. That
is 40 file reads and decodes on the main thread, for a History menu that is usually closed, and it is
called on every turn: `send()` (`ChatModel.swift:185`) and `finish()` (`ChatModel.swift:346`), and
also from `restore()` (`:643`) and `newChat()` (`:375`) when a turn is in flight.

Cost grows with the size of the *store*, not of the chat being saved, so a long chat that has been
saved once is then slow to save again on every later turn.

Evidence (before the fix):

```
MEASURE recent() with 40 chats (42 turns each, ~85 KB): 16.1-25.2 ms
MEASURE ChatModel.save() of a 42-turn chat:             18.0 ms   (nearly all of it the re-read)
```

Fix: build the `ChatStore.Saved` once, write it, then put it at the top of `recent` in memory
(dedup by id, cap at the 40 the store keeps). Three lines, no new types, no change to the file format
or to `ChatStore`. After the fix:

```
MEASURE ChatModel.save() of a 42-turn chat: 1.1 ms
```

The test asserts both the behaviour (saved chat is first, exactly once, list capped at 40) and the
cost (`save` under half of `recent()`), so the timing assertion failed before the fix (18.0 vs 10.1)
and passes after. `ChatStore.recent()` is still used where it belongs: once when Chat opens
(`ChatModel.restoreOnce()`, ~20 ms once) and after deleting chats.

### F2 (minor): the typewriter re-parses the whole growing answer 15 times a second

`MarkdownView.body` (`macos/Sources/EZLM/Markdown.swift:12`) calls `MdBlock.parse(text)` on every
body evaluation, and `MdBlock.parse` (`Markdown.swift:42`) splits every line and runs up to four
`NSRegularExpression.firstMatch` calls per line. During streaming, `ChatModel.frame()`
(`ChatModel.swift:156`) advances the typewriter 15 times a second, `msg.shown` grows, and the view's
`text` changes, so the entire answer written so far is re-parsed on the main thread every frame.

Evidence:

```
MEASURE MdBlock.parse(16076 chars):            1.0-1.2 ms
MEASURE parsing one answer over 60 frames:     24-32 ms   (~0.4-0.5 ms/frame)
```

So a typical long answer costs a few percent of a core, and a very long one (50 KB and up, tables and
code) costs 3 to 4 ms per frame, which is enough to drop frames on an 8 GB Mac. Proposed change (not
made, F1 was the smallest fix): only the final block of a growing answer can change, so parse from
the start of the last block and keep the rest; failing that, gate the four regexes on the first
character of the line (`#`, `-*+0-9`, `-*_`, `>`) before calling them, which removes most of the
`firstMatch` calls with no change in output.

### F3 (minor): a dropped text file is read and tokenized on the main thread

`ChatModel.attach(_:)` (`ChatModel.swift:478-495`) does `Data(contentsOf:)` (up to the 2 MB limit),
`decodeText`, and `Self.tokens(text)` inline, all on the main actor. `pickFiles()`
(`ChatModel.swift:469`) also calls `NSOpenPanel.runModal()`, a nested run loop while the panel is
open.

Evidence:

```
MEASURE ChatModel.tokens(2 MB):     24.0 ms
MEASURE ChatModel.decodeText(2 MB):  2.0 ms
```

Plus the file read itself: about 30 ms or more of frozen UI per dropped file, worse from a network
home or an external disk. Proposed change: read, decode and tokenize in a `Task.detached`, then
append the `ChatFile` on the main actor. Small, but it changes the order of the "too big" toasts, so
it wants its own commit.

### F4 (minor, by design): the transcript builds every message, and the page body re-runs 15 times a second

`ChatPane.transcript` (`ChatView.swift:148-171`) uses a non-lazy `VStack` over all messages; the
comment at `:152` explains why (a lazy list stopped building the newest turn after a very long
answer). The cost is that a long chat builds and lays out every turn. On top of that, `ChatPane.body`
reads `c.tick` in `.onChange(of: c.tick)` (`ChatView.swift:165`), so the whole page (status line,
menus, every `MessageView`) re-evaluates at 15 Hz while an answer streams, not just the growing
message. Two cheap follow-ups: move the follow-the-bottom logic into a small view that observes
`tick` so the page body stays still, and note that `MessageView` (`ChatView.swift:282`) creates one
`TimelineView(.periodic(by: 3600))` per finished answer, i.e. one timer per message in a long chat.
`:286` also copies the whole answer text (`String(msg.text.prefix(...))`) on each body evaluation of
a finished message.

### F5 (minor, frozen file, reported not fixed)

`macos/Sources/EZLM/Marks.swift:277` reads logo images with `NSImage(contentsOf:)` synchronously
inside the view body, cached afterwards. Small files, so a one-off cost per mark; noted for whoever
owns `Marks.swift`.

## What I changed

- `macos/Sources/EZLM/ChatModel.swift` (`save()`): one record built and written, then the in-memory
  `recent` list updated instead of re-read from disk. Comment says why.
- `macos/Tests/EZLMTests/ChatPerfTests.swift` (new, mine): measures the store read, a save, markdown
  parsing of a growing answer, and tokenizing a 2 MB file; asserts the save is cheap and that History
  still lists the saved chat first, once.

## Verified

- `cd macos && swift build` passes.
- `cd macos && swift test` passes: 24 tests, 0 failures (14 ChatTests, 7 TargetTests, 1 ReportTests,
  2 ChatPerfTests).
- The timing assertion in `ChatPerfTests` fails on the old `save()` and passes on the new one.
- No Go files touched, so the Go build is unaffected; `gofmt -l .` / `go build ./...` / `go vet ./...`
  were not re-run here.

## What I could not finish / needs a device

- All numbers are from a debug build in a test process, not from the running app with a real model
  streaming. The scrolling and redraw costs (F2, F4) want a look on a real 8 GB Mac with a 50 KB
  answer and a 200-turn chat; the test can only time the parsing, not the view diffing.
- F2 and F3 are measured but not fixed (time-box); both are small and described above.

## Open questions

- After the fix, `recent` reflects what was written, not what is on disk. If `ChatStore.save` fails
  (disk full, permissions), History would list a chat that is not there until the next launch.
  Before, the re-read hid that. Acceptable, but a "save failed" toast would be better than both.
- Only this app writes chats, so an in-memory list cannot go stale today. If a second window or a
  second instance is ever allowed, `recent` needs a refresh when History opens.
- The store keeps the newest 40 files by modification date, so a person with 500 saved chats only
  ever sees 40 in History. Not my slice, but it looks unintentional.
