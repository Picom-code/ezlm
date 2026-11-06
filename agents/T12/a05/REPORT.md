# T12 a05: Windows C# panes and screens

Read-only audit of the user-facing text in `windows/EZLM`: the panes and screens
(Overview, Models, System, Connect, Keys, Logs, Chat, Settings, the first-run wizard
and the install/failure panels). No code changed.

## What I scanned

Files read in full or by string sweep: `Panes.cs`, `SettingsPane.cs`, `Shell.cs`,
`Chat.cs`, `Ui.cs`, `Install.cs`, `Memory.cs`, `MemoryFill.cs`, `ModelMode.cs`,
`Engine.cs`, plus `BigType.cs`, `SignalWave.cs`, `Markdown.cs` (no user-facing
sentences in the last three). Roughly 180 user-visible strings; 31 rows written.

Frozen files (README list) are marked `owner_task=frozen`: `Panes.cs` (4 rows) and
`SettingsPane.cs` (4 rows). The rest of `windows/EZLM` is owned by no task tonight;
I used `T15` because T15 reads `windows/EZLM/*.cs` and reports C# findings. If the
Lead prefers a different routing value for unowned C#, rename it in one pass.

## Worst problems

1. `Chat.cs:774` (major): the /help toast literally starts with
   `ChatLogic.Commands:`. A class name is shown to users in the public beta.
2. `Panes.cs:319`, `SettingsPane.cs:66`, `Shell.cs:310`, `Chat.cs:673`,
   `Chat.cs:1052`, `SettingsPane.cs:216` (major/minor): raw `ex.Message` or engine
   error text is pasted into user-facing sentences, sometimes with a period
   appended, which can produce `..`.
3. `Panes.cs:677` (major): a failed app connection shows the bare word `Failed`
   with no next action and no Show details button in that state.
4. `Chat.cs:1049`, `SettingsPane.cs:319`, `Install.cs:225`, `ModelMode.cs:159`
   (major): errors that say what went wrong and stop there.
5. `MemoryFill.cs:56` (minor): "Pouring ... into memory" contradicts
   "Loading the model into memory" in `Panes.cs:313` for the same step.

## Changes

None. Rows only, in `agents/T12/rows/a05.csv`.

## Open questions

- Owner value for non-frozen `windows/EZLM` C# (used `T15`; no task owns it).
- `Panes.cs:319` and `SettingsPane.cs:66` interpolate engine errors; the rewrite
  needs the engine text to lose its trailing period, or the UI must trim it. That
  is a code change outside this task.
- `Chat.cs:538` says "model's memory"; check whether the Mac app says "context"
  (a09 parity) before choosing one word.
- The `↵`, `…`, `·`, `✓` and `◆` glyphs are deliberate UI marks, not emoji; left
  alone. a10 may still flag them.
