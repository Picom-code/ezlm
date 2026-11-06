# T12 / a15: BETA.md and user-facing docs strings

Report-only. No code and no docs were changed; the only files added are under `agents/T12/`.

## What I scanned

- `BETA.md`, all 61 lines, read line by line against the voice rules and checked for factual accuracy against the code:
  - installer per-user install (`windows/installer/EZLM.iss`, `PrivilegesRequired=lowest`);
  - startup registration (`internal/serve/service_windows.go`, Task Manager > Startup apps);
  - download resume (`internal/app/app.go` `.part` files, "EZLM resumes where it stopped");
  - update keeps the model running (`windows/EZLM/SettingsPane.cs:61`);
  - chat and memory storage (`windows/EZLM/ChatStore.cs:5`, `macos/Sources/EZLM/ChatData.swift:3`);
  - every button and page name the guide asks the user to press: Free up memory, Report a problem, Copy details for support, Remove models and settings, Stop server, Start server, Running on the processor, Open OpenCode, Open Hermes, New chat, History, Retry, Listen, Settings > Updates, System > Graphics/Engine, Connect.
- In-app documentation strings that reach users in both apps: the chat welcome hints, the `/` command list and descriptions, the `/help` output, the memory pane hints, and the shortcut hint strip. Command descriptions and shortcut hints match across Windows and Mac; the `/help` output does not (see below).

## Rows

`agents/T12/rows/a15.csv`: 13 rows. 10 on `BETA.md` (owner T13), 2 on `windows/EZLM/Chat.cs` (owner T15), 1 on `macos/Sources/EZLM/ChatView.swift` (owner T16). 4 major, 9 minor, 0 blocker.

## Worst findings

1. **`windows/EZLM/Chat.cs:774` (major, T15).** The `/help` toast shows the literal text `ChatLogic.Commands:` because the string has no `$` interpolation. Every Windows user who types `/help` sees a code identifier. Proposed: `Toast("Commands: /remember <fact>   /forget <words>   /memory   /new", Ink);`. The Mac toast is already correct (`ChatModel.swift:415`).
2. **`BETA.md:26` (major, T13).** "or press **Stop server**" reads as a way to stop EZLM starting at sign-in. It only stops the server now; the Run key stays, so the model starts again at the next sign-in. Users will think autostart is off when it is not.
3. **`BETA.md:24` and `BETA.md:45` (major, T13).** DeepSeek Harness is a shipped integration (`internal/integrate`, the Connect page, "Open DeepSeek Harness (browser)") but `BETA.md` never names it. Beta testers of that integration get no instructions at all.
4. **`BETA.md:49` (minor, T13).** The guide says "on any error" press **Report a problem**, but a Windows error card's button is labelled **Report this problem** (`windows/EZLM/Shell.cs:174`). The same report form is reachable from Logs as **Report a problem** (`windows/EZLM/Panes.cs:764`).

## Voice findings in BETA.md

- One en dash (`2–60 GB`, line 23); the rule says no em dashes, so the en dash should be "2 to 60".
- Marketing or flavour wording: "**It remembers you.**" (line 32), "**Connect** wires them up" (line 24), "then fold it away" (line 30).
- The star glyph `✦` is used as part of the memory button name in prose (line 32).
- Slash lists used for option names (lines 22 and 24) where the rest of the guide uses bold button names.

## What I could not do

- I could not verify the macOS Gatekeeper flow end to end (no Mac UI run tonight); the wording matches the macOS 14/15 dialogs as described in the code comments.
- `BETA.md` has no listed owner in `TASKS.md`. I used **T13** because T13 reads `BETA.md` and may propose a replacement guide. If the merge wants the current file patched instead, the owner is whoever applies T13's proposal.
- The star glyph and the "It remembers you" wording also fall in a10's voice sweep; rows may be deduped by (file,line,current_text), which they are.

## Open questions

- Does an en dash count as an em dash for the voice rule? I treated it as a minor violation.
- Should `BETA.md` document DeepSeek Harness (and the browser launch), or is the guide deliberately OpenCode/Hermes only? If deliberate, line 24 still needs to say why.
- The `2 to 60 GB` range is a guess about catalog sizes; T01/T13 should confirm the real spread before publishing.

## Next steps

- T13: apply the ten `BETA.md` rows in one pass while writing the new guide.
- T15: fix the `/help` toast string; it is a one-character fix and does not touch a frozen file.
- a16: the four major rows above are the ones worth grouping into the post-merge one-pass patch.
