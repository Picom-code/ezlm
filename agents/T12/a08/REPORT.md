# T12 a08: Swift settings, errors and menus

Report-only. No code or docs changed.

## What I scanned

`macos/Sources/EZLM`, read only, for the settings, error and menu surfaces (the screens and
chat strings belong to a07):

| File | Strings looked at | Notes |
|---|---|---|
| `Settings.swift` | 38 | updates, storage, About, banners (frozen file) |
| `EZLMApp.swift` | 6 | app menu commands, quit alert |
| `Dock.swift` | 1 | progress bar and `!` badge, no prose |
| `Engine.swift` | 6 | engine start failures, HTTP errors |
| `Updater.swift` | 5 | install failures (log and support paste) |
| `ModelMode.swift` | 12 | the failure steps only (`listFailed`, `closeFailed`) |
| `AppModel.swift` | 28 | quit warning, deploy and storage failures, toasts (frozen file) |

About 95 user-facing strings, 24 reported. Files outside the frozen list are owned by **T16**; the
two frozen files are marked `frozen`, so their rewrites land only if the morning rewrite keeps the
lines.

## Worst problems

1. **Raw engine errors become the whole message.** `Engine.swift:143` shows
   `Request failed (500)`, `ModelMode.swift:180` shows the engine's error text with only a Close
   button, and `Settings.swift:297` glues an engine error onto the version line. None names a next
   action. Major.
2. **Four failures end in a dead end.** `Settings.swift:56`, `Settings.swift:198`,
   `Engine.swift:77`, `Engine.swift:90` say what broke and stop. Each needs one "do X" sentence.
   Major.
3. **The quit alert's safe button says `Keep Going`** (`EZLMApp.swift:51`). It is the only way to
   not lose a download, and it does not name what keeps going. Minor.
4. **The quit warning is three sentences** (`AppModel.swift:191`), and its siblings are two each
   (193 to 195). Voice rule: one sentence that names the next action. Minor.
5. **`AppModel.swift:286`** reads as two messages run together, with a capital `Try again` in the
   middle of a sentence. Major, because it is the screen a tester sees after the engine dies.

## Menu findings (no CSV row: no string to quote)

- `EZLMApp.swift:23` replaces the `.newItem` command group with nothing but adds no `Settings…`
  (`Cmd-,`) and no `Check for Updates…` item. Settings is reachable only from the sidebar, and the
  update check only from the Settings pane or the Overview banner. Windows has a Settings pane too,
  so this is a Mac-only gap, not a parity divergence. Worth a T16 fix, not a T12 rewrite.
- The About panel is the stock one; the app's own attribution lives in Settings > About
  (`Settings.swift:246`, kept word for word with `Logos/NOTICE.md` and checked by a test).
- The Dock `!` badge (`Dock.swift:33`) has no text; fine.

## Consistency notes

- Loading lines mix `…` and no punctuation: `Settings.swift:391` (`Looking at your drives`) next
  to `Settings.swift:294` (`Checking for updates…`). Reported as minor.
- `Demo:` as a sentence prefix appears in `Settings.swift:72`; other demo strings use
  `Demo mode`. One wording for one idea.
- Error verbs are split between `Couldn't`, `didn't`, `can't` and `failed`. All are plain; I left
  them alone except where the sentence had no next action.

## Overlap and open questions

- a07 owns the screens and chat, so I left `Screens.swift`, `MainView.swift`, `Components.swift`
  and the `ChatModel` error strings alone, even where they read the same way (`ChatModel.swift:309`
  `Something went wrong: …` is the same dead end as `Engine.swift:143`).
- a10 sweeps two-sentence errors, so the quit-warning rows may be deduped against theirs.
- a12 sweeps messages with no next action, so items 1 and 2 will likely appear twice; the merge
  dedupes on (file, line, current_text).
- Open question: should the app-level failures (`Engine.swift:77`, `:90`) offer a Reinstall or a
  Report a problem button, or is "Try again" enough? I proposed wording for the first case only.
- Open question: is `Keep Going` deliberate to cover four different jobs? If so, `Keep EZLM Open`
  is the neutral label that covers all four.
