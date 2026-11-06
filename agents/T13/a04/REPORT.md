# a04 report: picking or switching a model

## What I wrote

`agents/T13/sections/a04.md`, about 1,100 words of body text plus 44 code citations. It covers:

- the **Models** page: the "On this computer" library and the "Get another model" list, the 14-option cap, tags, card contents, and Balanced / Smartest / Fastest;
- what the numbers mean: parameter count, mixture-of-experts totals, download size, memory use, context size, smarts and speed, and the quant file line;
- memory guidance: EZLM plans against free memory and keeps a reserve, what to do when a model needs space or does not fit, and the "free about N GB" hint;
- switching: the three switch button labels, keeping the URL, keys and apps, the old model answering during a download, one job at a time, and restoring the previous model on failure;
- downloads: stage order and progress, where files come from, cancel and resume, the 45-second stall retry, the five-failure limit, the size check, disk-full handling, the keep-awake hold, where files are saved, and automatic pruning.

## Findings (docs only)

1. **Minor, wording:** the disk warning says the download "will pause when the disk fills" (`internal/app/app.go:735`), but the actual failure message asks the user to free space and press Set it up again (`internal/app/app.go:743-744`). A pause would suggest it restarts on its own. Proposed wording for T12: "the download will stop when the disk fills; free space and press Set it up again." The section describes the real behaviour.
2. **Minor, discoverability:** both apps list only the first 14 fitting plans (`windows/EZLM/Shell.cs:316`, `macos/Sources/EZLM/AppModel.swift:224`). On a large machine a model that fits can be missing from the list with no "show more" control. The section states the cap.
3. **Minor, platform drift:** the web page shows at most 6 comparison cards (`internal/web/static/app.js:468`), while both native apps show 14. Not a bug, but assembly should not say "all fitting models" for the web page.
4. **Minor, information placement:** on the Models page a card shows the file line but not the download size; the size appears on the selected card's button and on the first-run Choose screen (`windows/EZLM/Panes.cs:457-459`, `windows/EZLM/Shell.cs:389-391`, `macos/Sources/EZLM/AppModel.swift:394-396`, `macos/Sources/EZLM/Wizard.swift:178`). The section tells the reader to select a card to see the button.

No blocker or major finding in this slice.

## What I could not verify

- The real Windows and macOS screens. This slice is code reading only; the wording and layout claims come from the C# and Swift sources, not from running the apps.
- Real download behaviour on a slow or filtered network. The retry, stall, size-check and resume claims are read from `internal/engine/download.go` and `internal/app/app.go`.
- Whether closing the install dialog leaves a download running. The code has `allowDialogClose` and `installDialog.Closing` in `windows/EZLM/Install.cs:185-186`, but I did not trace it far enough to document, so the section does not claim it.
- Actual speed estimates. The section calls them estimates, matching `internal/fit/fit.go:487-494`.

## Open questions

- Should the guide tell users how to reach a fitting model beyond the 14 shown? There is no UI path today.
- Should the disk warning be changed to "stop" instead of "pause"? This feeds T12.
- The model list shows "Slow" below 8 tokens a second, but Smartest still accepts models down to 5 tokens a second on a GPU and 2 on a processor (`internal/fit/fit.go:76-80`, `internal/fit/fit.go:487-494`). The section does not explain that exception; the Lead may want one sentence if Smartest is documented elsewhere.

## Checks

`gofmt -l .` empty, `go build ./...` passed. Docs only, no code changes.
