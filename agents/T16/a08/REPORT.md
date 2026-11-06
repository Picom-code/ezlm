# T16 / a08: localization and right-to-left text in Chat

Agent a08, branch `agent/T16-a08`, worktree `~/ezlm-agents/work/subs/T16/a08`.
Scope: string handling, layout and truncation for non-English and right-to-left
text in Chat. Reviewed `ChatView.swift`, `ChatModel.swift`, `ChatData.swift`,
`Markdown.swift`, `Theme.swift`, `scripts/build-app.sh`, and the frozen files
read-only (`Models.swift`, `Screens.swift`, `MainWindow`, `Settings.swift`).

## What I found

### 1. No localization at all (major, not fixed)
There is no `Localizable.strings`, no `.lproj`, no `String(localized:)` and no
localization keys in the generated `Info.plist` (`macos/scripts/build-app.sh:53`).
Every user-facing string is an inline English literal, for example
`ChatView.swift:46-49` ("Delete all chats?"), `:129-131` (the welcome text),
`:187-201` (the state lines), `:244-247` (the key hints), and
`ChatData.swift:102-111` (`ago()`: "just now", "min ago", "yesterday").
`Text("literal \(x)")` does build a `LocalizedStringKey`, so a table could be
added later without touching most call sites, but nothing exists to translate.
The app also sets no `CFBundleDevelopmentRegion`/`CFBundleLocalizations`.
A real translation is a separate, larger task; I did not start it.

### 2. `/remember` turned down ordinary Persian and Arabic text (major, fixed)
`Memory.add` rejected any fact containing a bidi control character: the
"unsafe" regex ended with `[\x{200B}-\x{200F}\x{202A}-\x{202E}\x{2066}-\x{2069}]`
(`ChatData.swift:189` before this change). That class includes the zero-width
non-joiner (U+200C) that is part of normal Persian spelling, the zero-width
joiner (U+200D) used in Indic words, and the right-to-left mark (U+200F) that
an RTL keyboard or a copied web page inserts. The person saw
"That looks like an instruction or a secret, so it isn't saved."
Reproduced before the fix (Foundation regex probe):
`"The user\u{200C}lives in Tehran."` -> rejected, `"...cats.\u{200F}"` -> rejected.
The hidden marks were there for Trojan-source style tricks, but rejecting the
fact is the wrong answer: the marks can be dropped instead.

Fix: `Memory.withoutBidiControls` strips the marks that only steer drawing
(U+061C, U+200B, U+200E, U+200F, U+202A-U+202E, U+2066-U+2069) from a fact
before it is checked, and from facts read back from `memory.md`, so older files
behave the same. U+200C and U+200D stay, since they are letters in a word.
The character class is gone from the unsafe regex. `/forget` and the
"Already remembered" check still match, because `norm()` splits on them anyway.

### 3. Markdown tables lean out of their box for CJK and emoji (major, fixed)
`TableBlock` measured a column with `cell.count` and padded with spaces
(`Markdown.swift:254-261` before). In the monospaced font a Chinese character
or an emoji takes two columns, so every table with those scripts had its
`│` separators at different places and the box drawing fell apart; the
truncation `String(cell.prefix(w - 1))` also cut by character, so a clipped
CJK cell could still be up to 48 columns too wide.
Proof (before): a table with `名前`/`日本語` got widths `[4, 3]`, and the rule
line was 14 columns while the data rows were 17.
Fix: `TableLayout` in `Markdown.swift` counts columns the way the font draws
them (`displayWidth`: 2 for wide characters and emoji, 0 for combining marks
and variation selectors, 1 otherwise), pads to that width, and `clip` cuts a
long cell to its column with `…`. Text without wide characters renders
exactly as before.

### 4. Right-to-left text was drawn from the left (minor, fixed; needs a look)
Chat relied on the default leading alignment, which follows the *app's* layout
direction (left to right), not the text's. So an Arabic or Hebrew answer, a
markdown paragraph, the thinking text and the input field all started at the
left margin. Fixed with `readsRightToLeft` (`Sources/EZLM/TextDirection.swift`,
first strong letter decides, digits and punctuation skipped) and
`.environment(\.layoutDirection, …)` on the user message
(`ChatView.swift:277`), the input field (`:219`), thinking text (`:352`), and
markdown headings, list items, quotes and paragraphs (`Markdown.swift`).
English chats are untouched: the direction is left to right whenever the first
strong letter is Latin, Greek, Cyrillic, CJK and so on.
The helper is unit tested; the rendering itself is not, so it needs one visual
check with an Arabic or Hebrew answer on a Mac.

### 5. The command hint could crash Chat (minor, fixed)
`ChatView.swift:208` (before) did
`cmd.arg.padding(toLength: 20 - cmd.cmd.count, …)`. A command name longer
than 20 characters makes that length negative, and Foundation raises an
Objective-C exception (`__CFStringHandleOutOfMemory`, verified) instead of
returning a string, so the Chat pane would crash. No command is that long
today, so this is a trap for the next one. Fixed with `ChatModel.hint`, which
keeps the argument whole when there is no room, plus a test.

### 6. Smaller findings, not fixed
- **Menu and chip width (minor).** History menu rows are
  `"\(s.title)   ·   \(ChatStore.ago(…))"` with no limit (`ChatView.swift:106`)
  and `FileChip` has no line limit (`:367`), so a long or RTL title widens the
  menu or the chip row. Titles are already cut at 57 characters
  (`ChatModel.save()`), which is why this stays minor.
- **Fact extraction is English-only (minor).** `Memory.extractPrompt` asks for
  `- The user …` lines and `parseFacts` drops any line that does not contain
  "user" (`ChatData.swift:249-269`). A model that answers in the chat's
  language (or a user instruction to answer in Arabic) produces facts that are
  silently dropped.
- **`ChatStore.newID` has no `en_US_POSIX` locale/calendar (minor).** With a
  non-Gregorian system calendar or Arabic-Indic digits the chat file name uses
  those digits; the ID is still unique and is only used as a file name, so the
  effect is cosmetic.
- **Token estimate for vowelised scripts (minor).** `ChatModel.tokens` counts
  non-CJK non-ASCII at half a token per scalar (`ChatModel.swift:518-526`), so
  Arabic harakat and Hebrew niqqud are counted as text. It errs high, which is
  the safe side for `fit`.
- **`BigType.fit` counts characters** (`Markdown.swift:301`), not columns.
  Model names are ASCII today.

## What I changed
- `Sources/EZLM/ChatData.swift`: strip bidi controls from facts on the way in
  and on the way out; drop the character class from the unsafe regex.
- `Sources/EZLM/Markdown.swift`: `TableLayout` and `displayWidth`, used by
  `TableBlock`; layout direction for markdown blocks.
- `Sources/EZLM/TextDirection.swift` (new): `readsRightToLeft`,
  `layoutDirection(of:)`.
- `Sources/EZLM/ChatView.swift`: layout direction for the user message, the
  input field and the thinking text; the command hint uses `ChatModel.hint`.
- `Sources/EZLM/ChatModel.swift`: `ChatModel.hint`.
- `Tests/EZLMTests/LocalizationTests.swift` (new, 8 tests): direction,
  Persian/Hebrew facts, invisible-mark duplicates, display widths, table
  alignment and clipping, the command hint. The table width assertion and the
  fact assertions fail on the old code (widths `[4, 3]` vs `[4, 6]`, row 17
  vs rule 14 columns; both facts rejected).

## Verified
`cd macos && swift build` passes; `swift test` passes (30 tests, 0 failures).
No frozen file was edited. Nothing was launched and no engine was started.

## Needs a device / a person
- One visual check of an Arabic and a Hebrew answer (message text, a list, a
  heading, a table) and of typing Arabic into the box: right alignment, the
  `▌` cursor at the left of the line end, and the model line `◆ …` above.
- A table from a CJK answer in the real font (the width table assumes the
  system monospaced font draws wide characters at exactly two columns).
- A check with a system language set to Arabic/Hebrew, where the whole app
  layout flips, to see whether Chat's own rule bars and paddings still read
  correctly.

## Open questions
- Should an RTL chat mirror the whole transcript (rule bar on the right,
  markers on the right) rather than only the text runs? The current fix keeps
  the terminal frame left to right, which is deliberate but arguable.
- Is a full translation in scope for the beta, or is English-only accepted?
  If translations are wanted, the string table should be added before more
  strings accumulate.
- Should `parseFacts` accept facts in the user's language (match a leading
  dash only, and ask the model for the language of the chat)?
