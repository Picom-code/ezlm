# a13 report: FAQ

**Slice:** FAQ, the 15 questions a beta tester actually asks.
**Deliverable:** `agents/T13/sections/a13.md`.
**Branch:** `agent/T13-a13`. Docs only, no code touched.

## What I covered

Fifteen questions, each answered in two or three sentences with footnote citations to the code:

1. Does EZLM work without internet?
2. Where are the models?
3. Can I use my graphics card?
4. Why is it slow?
5. Why did it pick a smaller model than I expected?
6. Is it safe? Does it send anything?
7. Can I delete it, and what is left behind?
8. What do I need to run it?
9. Is it free? Do I need an account?
10. Can I use it from other apps?
11. Can other devices on my network use it?
12. Why does Windows or macOS warn about the app?
13. Where are my chats and memory, and how do I delete them?
14. Can I switch models or update without downloading everything again?
15. Does it keep running in the background?

The FAQ is written for the Lead to drop into `FAQ.md` as is; the numbered headings are stable and
the notes are self-contained.

## How I checked the answers

Every behavioural claim was read from the code, not from `BETA.md` alone:

- Offline use: `internal/serve/config.go:99-100` (local URL), `internal/catalog/catalog.go:148-185`
  (embedded catalogue fallback), `internal/update/update.go:56-70` (GitHub needed for update check).
- Model locations and size: `internal/paths/paths.go:21-34`, `internal/scan/scan.go:108-124` and
  `154-175`, `internal/paths/settings.go:12-22`, `internal/catalog/catalog.json` (0.55 GB to 66 GB).
- Graphics cards: `internal/scan/scan.go:133-142`, `internal/scan/scan_darwin.go:35`,
  `internal/scan/gpu_discrete.go:196-217`, `internal/app/app.go:814-836`.
- Speed: `internal/fit/fit.go:74-86`, `480-495`, `internal/app/app.go:804-817`.
- Model choice: `internal/fit/fit.go:171-198`, `249-259`, `72-79`.
- Safety and reports: `internal/serve/config.go:92-127`, `161-171`, `internal/keys/keys.go:11-22`,
  `internal/report/report.go:42-71`, `windows/EZLM/ReportLaunch.cs:162-198`.
- Delete and leftovers: `internal/app/app.go:1584-1604`, `windows/EZLM/Panes.cs:542-548`,
  `BETA.md:53-55`.
- Chat and memory storage: `windows/EZLM/ChatStore.cs:5`, `windows/EZLM/Memory.cs:7`,
  `macos/Sources/EZLM/ChatData.swift:3`, `macos/Sources/EZLM/ChatView.swift:46`.
- Model reuse and updates: `internal/app/app.go:680-719`, `BETA.md:36-38`, `BETA.md:53`.
- Startup: `internal/serve/service_windows.go:67-105`, `internal/serve/service_darwin.go:47`,
  `internal/app/app.go:1378`, `internal/serve/config.go:192-193`.

I also checked that the only outbound hosts in non-test code are GitHub (releases, update check,
reports), Hugging Face (model files), the raw catalogue on GitHub, and the app installer sources used
by the Connect page. Nothing in Chat or the OpenAI endpoint calls out.

## Could not verify

- **Real speed numbers.** The FAQ describes the thresholds and causes, not measured tokens per
  second, because the estimates in `internal/fit` are hardware-dependent and I did not load a model.
- **What the Windows uninstaller removes besides models.** The app's own "Remove models and settings"
  calls `internal/app/app.go:1584-1604`, which deletes `~/.ezlm` and EZLM-created model folders. It
  does not delete `%LOCALAPPDATA%\EZLM`, where chats and memory live. Whether the MSI/EXE uninstaller
  also removes that folder is installer behaviour and not visible in this tree, so the FAQ says the
  chat folder stays until you delete it, which matches the code path it cites.
- **Whether LAN mode uses TLS.** `internal/serve/config.go:102-111` builds a plain `http://` LAN URL
  and the server is started without a certificate, so the FAQ says "plain HTTP". I did not run a
  second device to confirm the firewall prompt on Windows.
- **Price of EZLM itself.** Nothing in the repo states it, so question 9 only claims that no account
  or service key is needed, which the setup flow supports.
- **macOS behaviour of the app on Intel Macs.** `BETA.md:13` says Apple Silicon M1 or later; the
  engine builds also include `macos-x64` (`internal/engine/engine_test.go`), so an Intel Mac may
  work but is not promised.

## Open questions for the Lead

- Question 9 could mention the model licences (some Hugging Face models have terms). No licence
  field is exposed in `internal/catalog/catalog.json`, so I left it out.
- If the Lead wants a shorter FAQ, questions 5, 8 and 14 are the safest to cut; 1, 2, 3, 4, 6 and 7
  are the ones testers ask first.
- The footnote markers assume the Lead keeps the notes with the section. If `assemble.py` strips
  footnotes, the citations should become inline parentheticals instead.

## Not done

- No changes outside `agents/T13/`.
- No tests were added or run beyond `gofmt -l .` and `go build ./...`, since this slice is docs only.
