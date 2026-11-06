# a12 report: the apps' own messages

## What this covers

`agents/T13/sections/a12.md` documents the messages the two desktop apps show themselves, with the
exact on-screen text, what each one means, and the next action. It covers:

- startup and engine failures (`windows/EZLM/Engine.cs`, `windows/EZLM/Shell.cs`,
  `macos/Sources/EZLM/Engine.swift`, `macos/Sources/EZLM/AppModel.swift`)
- setup and model-switch failures and their four hint sentences (`windows/EZLM/Install.cs`,
  `macos/Sources/EZLM/AppModel.swift`)
- server state on Overview: stopped, could not start, unmanaged, processor fallback
  (`windows/EZLM/Panes.cs`, `macos/Sources/EZLM/MainView.swift`)
- every Chat error sentence and every attachment rejection (`windows/EZLM/Chat.cs`,
  `windows/EZLM/ChatStore.cs`, `macos/Sources/EZLM/ChatModel.swift`)
- connecting and launching apps, including the Hermes 64K trade-off and tool-calling block
  (`windows/EZLM/Panes.cs`, `windows/EZLM/Models.cs`, `macos/Sources/EZLM/AppModel.swift`,
  `macos/Sources/EZLM/Models.swift`)
- Model mode on macOS (`macos/Sources/EZLM/ModelMode.swift`)
- updates on both platforms (`windows/EZLM/SettingsPane.cs`, `macos/Sources/EZLM/Settings.swift`,
  `macos/Sources/EZLM/Updater.swift`)
- model storage and problem reports.

41 entries, about 3,400 words. Messages that come from the Go engine and are displayed unchanged
are deliberately left to a09 to a11; the section says so and points there.

## Method

Read the C# and Swift sources and collected every string that reaches the screen: `Txt(...)`,
`Toast(...)`, `Dialog(...)`, `showToast(...)`, `Text(...)`, `updateNote`, `note`, `launchNote`,
`deployHint`, plus the exception messages thrown by `Engine.cs` / `Engine.swift`. Each entry cites
the file and line that produces the text. Variable parts are shown as `{braces}` and explained.

Not verified: no Windows or macOS build was run, so the visual placement of each message (banner,
dialog, status card) is read from the code, not from a screenshot. The toast lifetime (about four
seconds) is read from the delay in `Shell.cs:106` and `AppModel.swift:1058`.

## Findings

### Windows and macOS word the same message differently (minor)

Six messages drift. The section gives both versions; if TROUBLESHOOTING must be one file, use the
Windows text as the heading and add the macOS sentence underneath.

1. Engine start timeout. Windows: `EZLM's engine didn't start within 30 seconds. Your antivirus may
   be scanning it; try again in a moment.` (`Engine.cs:75`). macOS drops the antivirus clause
   (`Engine.swift:69`). macOS has Gatekeeper instead, so the Mac text could name it.
2. Engine died while starting. Windows adds `Your antivirus may have blocked it.` (`Engine.cs:76`);
   macOS says only `EZLM's engine stopped while starting.` (`Engine.swift:76`).
3. Server start failure hint. Windows ends `use Copy details for support in Logs`
   (`Panes.cs:319`); macOS ends `report the problem so it can be fixed` (`MainView.swift:310`).
4. Unreadable attachment. Windows shows the exception: `Couldn't read {name}: {details}`
   (`Chat.cs:1052`); macOS shows only `Couldn't read {name}.` (`ChatModel.swift:485`).
5. Launch failure. Windows can name the app: `Couldn't open {app}.` (`ReportLaunch.cs:92`); macOS
   always says `Couldn't open it.` (`ReportLaunch.swift:46`).
6. Closing apps to free memory. macOS has a whole Model mode with per-app steps and a force-close
   choice (`ModelMode.swift:180-257`); Windows only asks `Quit {app}?` in a dialog
   (`Shell.cs:494`). The two cannot share one troubleshooting entry.

### `Setup failed.` does not say which step failed (minor, feeds a14)

`Install.cs:225` and `AppModel.swift:612` use this sentence only when the engine's `done` event
carries no message. The steps are on screen at the time, so the app could name the last running
step: `Setup failed while loading the model.` As written the tester cannot tell a download failure
from a load failure.

### macOS update failures collapse to one sentence (minor, feeds a14)

`Settings.swift:96-101` catches every `Updater.install` error and shows
`Drag EZLM into Applications to replace the old one, then open it again.` The real reason
(`The update doesn't contain EZLM.app`, `The update is a different app`, `hdiutil failed: ...`,
`Updater.swift:21-58`) goes only to the log. A tester whose download is corrupt is told to drag the
app, which cannot work. The catch could show the reason when it is short.

### The failure screen truncates the reason at the first newline (minor)

`Shell.cs:164` uses `ex.Message.Split('\n')[0].Trim()`. Multi-line engine errors lose everything
after the first line on the Windows failure screen. `Copy details for support` keeps the full text,
so support still has it.

### Raw fallbacks reach the screen (minor, feeds a14)

`Engine.cs:129` and `Engine.swift:143` produce `Request failed ({status})` when the engine's error
body is not JSON. Several screens put that string straight into `note` (for example
`Panes.cs:509`, `AppModel.swift:781`). A tester sees a bare status code with no sentence around
it. A prefix such as `EZLM's engine refused the request (500).` would read better.

## Open questions

- Should TROUBLESHOOTING be split per platform, or one entry per message with a Windows line and a
  macOS line? The section is written so either works, but the assembler has to choose.
- The section quotes Windows text with straight quotes around `llama-server` as the code does
  (`Panes.cs:141`). The Mac text uses escaped quotes in the same place (`MainView.swift:238`).
  Normalise both to one style during assembly if the guide does that elsewhere.
- Demo-mode text (`Demo: nothing is installed...`, `Shell.cs:66`; `Wizard.swift:101`) is not a
  troubleshooting message. It is listed nowhere in the section; add it to BETA if the demo build
  goes to testers.

## Deliverables

- `agents/T13/sections/a12.md`
- `agents/T13/a12/REPORT.md` (this file)
