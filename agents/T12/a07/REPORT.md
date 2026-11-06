# T12 a07: Swift screens and chat (read only)

## Scope

Assignment: Swift in `macos/Sources/EZLM`, screens and chat. Files read in full or in
part and scanned for user-visible strings:

- `Screens.swift` (frozen), `MainView.swift` (frozen), `Components.swift` (frozen)
- `ChatView.swift`, `ChatModel.swift`, `ChatData.swift`, `Markdown.swift`
- `Wizard.swift`, `Pour.swift`

Not mine (a08: settings, errors, menus): `Settings.swift`, `AppModel.swift`, `Updater.swift`,
`Engine.swift`, `Dock.swift`, `EZLMApp.swift`, `ModelMode.swift`, `Models.swift`, `Marks.swift`,
`Theme.swift`, `SignalWave.swift`, `ReportLaunch.swift`. Findings there belong to a08.

`Components.swift` and `Markdown.swift` have almost no user prose: pixel-font glyphs, markdown
syntax and "code"/"Copy"/"Copied" in code blocks. Nothing to report.

Strings reviewed: roughly 170 user-visible literals (Text/Button/Label/help/accessibility plus
toast and error strings in `ChatModel`/`ChatData`). 24 rows written to
`agents/T12/rows/a07.csv`.

Owner column: `frozen` for `Screens.swift` and `MainView.swift` (README frozen list); `T16` for
the other Swift files (T16 owns Swift outside the frozen list).

## Worst problems

1. **ChatModel.swift:702** (major). `Your model answered with an error (\(code)): \(msg)` dumps
   the raw engine message and HTTP status into the chat. Nothing tells the user what to do. This is
   also the string a13 is collecting; it is included here because it lives in my file.
2. **ChatModel.swift:488 and 494** (major). Attachment failures for PDF/Word files and for files
   that exceed the context leave the user stuck: one says "need their text copied in", the other
   only states the token count. Both need an explicit next step.
3. **ChatModel.swift:309** (minor). The catch-all `Something went wrong:
   \(error.localizedDescription)` shows a system string with no next action.

## Consistency findings

- **Speed units.** Model cards and the wizard say `words/s` (`Screens.swift:173`,
  `Wizard.swift:179`), while Overview says `tok/s` (`MainView.swift:53`, `:318`, `:335`). The
  same screen (`MainView`, installed-model list at `:758`) uses both. Both apps share the mix, so
  this is not a Windows/Mac divergence, but it is one app speaking two languages.
- **"your AI" vs "your model".** `MainView.swift:548` and `Wizard.swift:351` are the only
  screens that say AI; everything else says model.
- **"graphics card" vs "graphics chip" vs "GPU".** `Screens.swift:230-232` and `Wizard.swift:313`
  use "graphics card"; `Wizard.swift:313` also says "graphics chip"; `Wizard.swift:315` says
  "GPU".
- **"Report this problem" vs "Report a problem".** Both labels exist for the same action across
  `Screens.swift:355`, `MainView.swift:38/281`, `Screens.swift:420`. Windows has the same split
  (`Shell.cs:174` vs `Panes.cs:602/764`), so a fix should land in both apps together.
- **Server state words.** Chat header says `live`/`loading`/`stopped` (`ChatView.swift:70-71`);
  Overview says `Ready`/`Loading the model into memory`/`The server is stopped`
  (`MainView.swift:300-304`). "Pouring" in `Pour.swift:51` is a third word for loading.

## What I could not finish

- I did not read `Markdown.swift` prose exhaustively (only string literals); its visible strings
  are "code", "Copy" and "Copied" in fenced blocks and are correct.
- Windows/Mac parity for these rows is a09's job. I noted where I happened to check Windows
  (`Report this problem`), but did not audit every row against the C# text.
- The `words/s` vs `tok/s` row proposes converting the live token rate by 0.75 (the same factor
  `Models.swift:92` uses). A safer fix may be to rename the unit only; that is a decision for the
  owner.

## Open questions

- Should user-facing speed be `words/s` everywhere (plain, but approximate) or `tok/s` (exact, but
  jargon)? I proposed `words/s` for consistency with the model cards.
- Is "Overview" a name a beta tester recognises? `ChatModel.swift:306-307` and
  `AppModel.swift:655` send the user to Overview; the sidebar label exists, so I left it.
- `Screens.swift:8` "Something went wrong" is plain but generic. I left it because the body line
  carries the detail; the owner may prefer "EZLM ran into a problem".

## Method

Read-only. No code or docs changed. CSV written with Python's `csv` module so quoting is valid.
