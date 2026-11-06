# T13 a03 report: first run

## What I covered

`agents/T13/sections/a03.md` (about 1,100 words including citations):

- What the first run scans: the five checklist lines on Windows and Mac, the exact wording for graphics on a discrete card, Apple Silicon and a machine with no supported card, and what is read from the machine.
- What the app shows next: the choose screen, the recommended pick, the preference switch, the 14 other options, the memory bars and the three buttons.
- How the first model is picked: ranking by model score adjusted for quantization quality and speed, the Balanced default, the memory margins, and the fact that free disk space does not disqualify a pick.
- How long it takes: the deliberate ~1.8 s checklist animation, the 15 s scan cap, the 4 s catalog fetch, the 30 s engine-start error, and why a first start can be slower.
- What to do when detection looks wrong: rescan after 20 seconds (the engine reuses a hardware scan for 20 s), Free up memory, the System page, the storage folder, driver advice, and the report buttons.

Every behaviour claim carries a `file:line` citation in the Notes section.

## What I checked, and how

- Read `internal/scan/scan.go`, `scan_darwin.go`, `scan_other.go`, `internal/fit/fit.go`, `internal/catalog/catalog.go`, `internal/prereq/prereq.go`, `internal/app/app.go`.
- Read the Windows app (`Shell.cs`, `Panes.cs`, `Engine.cs`, `Models.cs`) and the Mac app (`AppModel.swift`, `Wizard.swift`, `Screens.swift`, `MainView.swift`). The apps are off-limits to edit but readable.
- Ran `go test -run TestPrintDesktop -v ./internal/fit` (no model load, no server) to confirm the ranking behaviour and get a concrete example of a pick. The test passes and prints the plan order for a real machine profile.
- Did not run either app, so all wording is taken from source, not from a screenshot.

## Findings worth passing to the Lead

1. **Minor (docs consistency):** the Mac choose screen has no "Saves to" row or **Change** button; the Windows one does (`windows/EZLM/Shell.cs:395-400` versus `macos/Sources/EZLM/Wizard.swift:159-262`). The section says so, but the final guide should not promise the same storage shortcut on both platforms.
2. **Minor (possible tester confusion):** the engine caches a hardware scan for 20 seconds (`internal/app/app.go:193-195`), so **Scan again** right after closing an app can show identical numbers. Documented as "wait about 20 seconds". This is a real message-behaviour gap, not a bug.
3. **Minor:** hardware `Notes` from the scan (for example "No supported GPU found: models will run on the CPU, which is slower.", `internal/scan/scan_other.go:18`) are in the JSON but never shown in either app; they only travel in a report. If T12 wants a visible "processor only" warning on first run, that text already exists.
4. **Note:** the first model can be picked partly because free memory is low at scan time, and the choose screen offers **Free up memory** right there. That is the intended path for a busy machine, and the section leads with it.

## Could not verify

- Actual first-run wall time on a real Windows or Mac machine. The ~1.8 s figure is the sum of the delays in the two apps plus the scan work, and the engine start is unbounded up to 30 s.
- Whether the 4 s catalog fetch is noticeable on a slow connection in practice; the code caps it, the user experience on a 2G link is untested.
- The exact appearance of the choose screen at small window sizes. Windows clamps the content width (`windows/EZLM/Shell.cs:253`), the Mac screen uses `ViewThatFits`, and neither was rendered.

## Open questions

- Should the guide tell testers that the model list may update from the internet on first run? It is invisible in the UI and only changes rankings when a newer catalog is published. I left it out of the section and mention only that nothing about the computer is sent.
- Is "words per second" the right unit for a non-technical reader? The apps convert tokens to words at 0.75 (`windows/EZLM/Models.cs:70`, `macos/Sources/EZLM/Models.swift:92`). I kept the app's wording.

## Commit

Docs only, no code changes. `gofmt -l .` empty and `go build ./...` passes in the worktree.
