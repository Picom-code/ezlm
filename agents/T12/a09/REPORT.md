# T12 a09: Windows/Mac parity

## What I scanned

The two apps, side by side, journey by journey: `windows/EZLM/*.cs` (24 files, about 6,700
lines) and `macos/Sources/EZLM/*.swift` (23 files, about 8,000 lines). I extracted every string
literal from both trees (C# and Swift, skipping comments and format-only strings) and aligned the
strings that belong to the same moment:

- boot, scan, choose, install, ready (wizard);
- Overview and server state (stopped, starting, failed);
- Models list and model cards;
- Connect apps (OpenCode, Hermes, DeepSeek Harness) and the key-copy note;
- Keys and LAN;
- Logs;
- Model mode;
- Chat, including the memory sheet;
- Settings, model folder and update;
- Report a problem;
- quit/close warnings.

Most of the journey is already word-for-word identical, which is a good sign. I found 8
divergences, all minor; none is a blocker. No string was already wrong on both sides, so the
proposed text below is one sentence to use in both apps.

## Rows

`agents/T12/rows/a09.csv`, 8 rows. Each row names one app in `file`/`line` and quotes the other
app's text with its `file:line` in `problem`, so both texts travel together without breaking the
merge tooling in a11 (dedupe is on `file,line,current_text`).

| # | Journey | Windows | Mac | Severity |
|---|---|---|---|---|
| 1 | copy key for llama.cpp chat | Panes.cs:250 "paste it in Settings." | ReportLaunch.swift:13 "paste it into the chat's settings." | minor |
| 2 | server failed to start | Panes.cs:319 "use Copy details for support in Logs" | MainView.swift:310 "report the problem so it can be fixed" | minor |
| 3 | delete all chats | Chat.cs:1176 "this computer" | ChatView.swift:49 "this Mac" | minor |
| 4 | thinking toggle | Chat.cs:125 "Thinking: let the model think before answering" | ChatView.swift:87 "Let the model think before answering. Off: straight to the answer." | minor |
| 5 | quit dialog | Shell.cs:551 "Keep going" | EZLMApp.swift:51 "Keep Going" | minor |
| 6 | model folder picker | SettingsPane.cs:194 "Pick the drive with the most room." | Settings.swift:164 "so choose a drive with room." | minor |
| 7 | wizard hint spacing | Shell.cs:358 three spaces before the arrows | Wizard.swift:214 two spaces | minor |
| 8 | clear memory | Chat.cs:922 toast "Memory cleared." | ChatView.swift:441 no confirmation | minor |

The worst two are 1 and 2, because they send the user to a different place or name no control at
all. 2 is the only one where the Mac text is worse than the Windows text; 1 and 6 are the only
ones where the Windows text is worse.

## Divergences I judged justified (not rows)

These differ because the platform differs, not because the wording drifted:

- "Close EZLM?" (Shell.cs:550) vs "Quit EZLM?" (EZLMApp.swift:49), and "If you close it now"
  (Shell.cs:531-533) vs "If you quit now" (AppModel.swift:193-195): Windows closes a window,
  macOS quits an app.
- "Task Manager" vs "Activity Monitor" (Panes.cs:141, MainView.swift:238), "Installed apps in
  Windows Settings" vs "drag EZLM from Applications to the Trash" (Panes.cs:543, MainView.swift:528).
- Windows Settings > Time & language > Speech (Chat.cs:976) has no Mac equivalent.
- Key glyphs and shortcuts: `↵`/`ctrl+n` vs `↩`/`⌘N`, `shift+enter` vs `⌥↩` (Shell.cs:358,
  Chat.cs:274, ChatView.swift:201/244). The one real drift there is spacing, row 7.
- "EZLM asks about each app" on the System card (Panes.cs:523, MainView.swift:548) vs "asks about
  each one" in the Model mode window (ModelMode.cs:64, ModelMode.swift:172). Both apps make the
  same split, so this is an intra-app wording issue, not a Windows/Mac one. Worth a follow-up
  under T12's general sweep.

## Parity gaps that are not wording rows

- The Windows choose card shows a "Saves to <folder> [Change]" row (Shell.cs:397); the Mac choose
  card has no equivalent, so a Mac user cannot see or change where the download lands from the
  wizard. That is a feature gap for T16, not a text edit.
- The Mac update flow has demo-mode copy ("Downloaded. Demo mode doesn't install anything.",
  Settings.swift:71-72); the Windows update flow has no demo branch at all, so in demo mode Windows
  takes the normal install path. Behaviour, for T15/T16.
- Mac "Forget everything" has no confirmation toast (row 8); the Windows one does.

## Owner mapping used

The README's frozen list decides `owner_task` first: Panes.cs, ReportLaunch.cs, Models.cs, Ui.cs,
SettingsPane.cs, SelfTest.cs, Snap.cs (Windows) and Models.swift, Marks.swift, Components.swift,
Screens.swift, MainView.swift, ReportLaunch.swift, AppModel.swift, Settings.swift (Mac) are
`frozen`. For the rest, no TASKS.md Owns list names the C# or Swift files directly, so I used
T15 for non-frozen `windows/EZLM` files (T15 reads every one of them) and T16 for non-frozen
`macos/Sources/EZLM` files (T16 owns Swift outside the frozen list). Consolidation may want to
re-map those two values.

## Open questions

- Should the parity fix live in the Windows text, the Mac text, or both? For rows 1, 2, 4 and 6 the
  proposed sentence is meant for both apps; if only one file is patched, pick the sentence in the
  row and leave the other for a follow-up.
- Row 2 proposes "press Report a problem" for both. Windows currently points at "Copy details for
  support" in Logs, which is also a real button; if support would rather keep the log-copy path,
  use "use Copy details for support in Logs" on both instead.
- Row 7 is cosmetic. It is only worth applying if the merge is already touching that line.

## What I changed

Nothing outside `agents/T12/`. No code, no docs. `gofmt -l .` and `go build ./...` were not run
because this task touched no Go file; the tree is unchanged apart from this folder.
