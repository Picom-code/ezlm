# T12 / a10: voice-rule sweep

**Scope:** every em dash, emoji, exclamation mark, marketing word and two-sentence error in
user-facing strings across Go, C#, Swift, the Windows installer and `BETA.md`.
**Report-only:** no code or docs changed. Rows: `agents/T12/rows/a10.csv` (60 rows).

## What I scanned

| Area | Files | User-facing strings reviewed |
|---|---|---|
| Go | 168 non-test `.go` files (`internal/**`, `cmd/**`) | every `errors.New` / `fmt.Errorf` / `fmt.Sprintf` literal, plus log/UI strings in `internal/app`, `internal/serve`, `internal/integrate` |
| C# | 21 files in `windows/EZLM/` | every string literal in the WinUI panes, dialogs, chat, install, report, settings and self-test paths |
| Swift | 26 files in `macos/Sources/EZLM/` | every string literal in screens, chat, model mode, settings, updater, wizard, report |
| Installer | `windows/installer/EZLM.iss` | all 6 user-visible strings (task description, run entry, uninstall prompt) |
| Docs | `BETA.md` | all 61 lines |

Method: extracted string literals with a small script (comments stripped), then filtered for
`—`/`–`, `!`, emoji-range code points, a marketing-word list, and a sentence-count check
(`[.!?]\s+[A-Z]`) on error strings. Every row was then read in its source context.

## Counts by kind (rows written)

- two-sentence (or longer) errors/notices: **44**
- marketing words: **10** ("smart", "smartest", "snappy", "One click")
- exclamation marks: **2** (`Sent. Thank you!` in both apps)
- emoji/symbol glyphs inside prose: **2** (`✓ Connected and answering`, `see ✦ memory`)
- em dashes: **0**

## Worst problems

1. **Three-sentence error with the action last** (major, both apps):
   `windows/EZLM/Panes.cs:141` and `macos/Sources/EZLM/MainView.swift:238` explain the
   administrator-rights server, reassure about other apps, then only in the third sentence say
   what to do. Users on the "can't control the server" screen are stuck until they read all three.
2. **Error with no action at all** (major): `windows/EZLM/Engine.cs:76`
   "EZLM's engine stopped while starting. Your antivirus may have blocked it." says why but not
   what to do. `internal/serve/config.go:196` has the same shape but buries the action in a
   subordinate clause after the diagnosis.
3. **The quality/speed presets are named with marketing words** in both apps and in `BETA.md`:
   `Smartest` / `Snappy replies first` / `Smart, at a comfortable speed`
   (`Shell.cs:438,459`, `Models.swift:426,431,433`, `Screens.swift:171`, `BETA.md:22`).
   These are the most-repeated user-facing strings in the product, so they set the voice.
   Note: in Swift the label is `rawValue.capitalized`, and the raw value is persisted in
   settings; rename the display title only, keep `smartest`/`fastest` as the stored value.
4. **`Sent. Thank you! (report #N)`** (`ReportLaunch.cs:199`, `ReportLaunch.swift:238`): the
   only exclamation marks in user-facing text, plus a thank-you instead of a result.

## What is already correct (not rowed)

- **No em dashes** anywhere in user-facing strings. The only `—` hits are in Go/Swift test
  files, and `macos/Sources/EZLM/Models.swift:112` is a doc comment. `BETA.md:23` uses an en
  dash in the range "2–60 GB", which is correct typography, not an em dash.
- The installer is clean: "Put an EZLM icon on the desktop", "Open EZLM", and the uninstall
  prompt ("Choose No to keep them...") are plain, one action, no forbidden characters.
- The `–` (en dash) in `Panes.cs:168,353,356` and `MainView.swift:335,338` is a placeholder for
  "no value yet", not an em dash. It is consistent between the two apps; leave it.
- Demo-model text (`Hello! I'm Qwen...` in `internal/app/demo.go:236`, `SelfTest.cs`) is
  simulated model output, not app chrome, so its exclamation mark is deliberate.
- The app's symbol language (`❯` prompt, `∴ thinking`, `◆ model`, `✦ memory` as a tab icon) is
  UI chrome, not prose; I only rowed glyphs that sit inside a sentence or a status phrase.

## Judgement calls / open questions

- **"Two-sentence error" is applied strictly** to error and status strings that say a thing and
  then add a second sentence. Dialog bodies that legitimately need a second sentence (close
  confirmations, "make room" flow) were left alone.
- `cmd/ezlm` exposes `--smartest` / `--fastest` flags (`cmd/ezlm/main.go:350-353`). They are
  user-facing but renaming them breaks scripts, so I did not row them; worth a decision at the
  merge. `cmd/**` is frozen.
- `internal/fit/fit.go:70-73` uses "smart"/"snappy" only in Go comments; the user-facing labels
  come from the apps, so no row there.
- Windows/Mac wording diverges in places beyond voice (a09's slice); I rowed each platform
  separately so both patches can be applied.

## Next steps

- Apply rows by owner: T15 (C#, 25 rows), T16 (Swift, 25 rows), T13 (`BETA.md`, 2 rows),
  T09 (download errors, 3 rows), T07 (serve error, 1 row), frozen (4 rows, for the merge owner).
- The preset rename is one coordinated change across four files plus `BETA.md`; do it in one pass
  after the merge.
