# T16. Mac app review — consolidated report

16 agents (a01–a16) worked this task in parallel: nine review areas, the Swift test suite, the Go
darwin side, the manual checklist and the severity table. Consolidated onto `agent/T16-mac-review` by
the Lead. This is the one task that ran on the actual platform: `swift build` and `swift test` were
run here, and the launchd behaviour was tested with a real launchd job.

## Result

- **2 blockers, 7 major, 9 minor** findings, in `agents/T16/a16/findings.md`.
- **13 fixes** in Swift and Go, each with a test. The Swift suite grew from 22 to **76 tests, 0
  failures** (1 skipped: see the merge notes).
- **Gate:** `gofmt -l .` empty, `go build ./...`, `go vet ./...`, `go test -p 2 ./...` pass,
  `GOOS=windows` amd64/arm64 and `GOOS=linux` builds pass, and `cd macos && swift build && swift test`
  passes.

## The blockers

1. **The app is ad-hoc signed and the DMG is not notarized.** `spctl` rejects it, and an app copied
   out of a browser-downloaded (quarantined) DMG is quarantined too, so a manual update is blocked on
   relaunch. `BETA.md` only covers the first install.
2. **TCC (Automation) grants are lost on every update**, because an ad-hoc designated requirement is
   a cdhash (proved with `codesign -d -r-`). The only TCC need is Automation, via `osascript` in
   `internal/scan/apps_unix.go:71`.

## Major findings (all fixed unless marked)

| Area | Finding |
|---|---|
| **Main-thread work** (a01) | `ChatModel.save()` re-read and decoded all 40 saved chats on the main actor every turn: **18.0 ms → 1.1 ms** after the fix. |
| **SSE reconnect** (a02, a11) | A stream dropped before the first word became a terminal error; fixed with a bounded retry that waits for a loading server. A socket that dies in sleep was not noticed for up to 15 min (`timeoutIntervalForRequest=900`); the idle timeout is now 90 s. Reconnect spun at 2 Hz forever; now a 0.5–8 s backoff. |
| **Updating from the DMG** (a03) | Updating while running translocated downloaded the DMG and failed to install; `open` re-activated the old app, so the update repeated. Now installs into `/Applications` and relaunches from there. |
| **Signing** (a04) | The updater swapped and deleted the old app without verifying the new bundle; now strips quarantine and runs `codesign --verify --deep --strict` before the swap. |
| **Stop not remembered** (a05) | The launchd agent ran the server directly with `RunAtLoad`, so a stopped server started again at the next login. The job now runs `/bin/sh -c "test -f <stopped> || exec <server> …"` — proved with a real launchd job on this Mac. |
| **Memory reporting** (a06) | `memory_pressure`'s "free percentage" was used as available RAM; xnu defines it as active+inactive+free+speculative, so it **doubled** the headroom on 8 GB Macs (measured 50% vs 4.84 GB real). Now subtracts active pages. |
| **No model fits** (a07) | The 8 GB wizard pointed at "an app on the right" while the right column read `gpuProcs`, which is empty on unified Macs. Fixed with a `top_mem` fallback. |
| **RTL and CJK** (a08) | `/remember` rejected ordinary Persian/Arabic text (ZWNJ/ZWJ/RLM were in the "unsafe" regex); markdown tables misaligned for CJK/emoji. Both fixed. |
| **App state** (a10) | `boot()` ran per window, so closing and reopening during a job restarted the engine and killed it; an update could be half-applied when "Keep Going" cancelled the relaunch. Fixed with a `LaunchGate`. |
| **Updater safety** (a12) | An update with no engine bricked the app (now `usableApp` checks); relaunch could open a second copy; a partial answer was lost on quit. |
| **Launchd plumbing** (a14) | A user-set `iogpu.wired_limit_mb` above RAM was used verbatim (now clamped); `Install` did not create `~/.ezlm/logs` and unloaded the working agent before writing the new plist. |

## Also fixed

`Markdown.swift` stripped trailing `#` from heading text, so "## C#" rendered "C" (a13); `SignalWave`'s
30 Hz timer outlived the wave (a09); the engine client mapped `NSURLErrorDomain` codes to plain
messages, retried GETs once, and read base+token under one lock (a11); `ChatStore` and `MemoryFill`
gained hermetic tests (a13).

## Found, not fixed

- **No localization at all**: no `Localizable.strings`/`.lproj`, no locale keys, all English
  literals (a08).
- **Free memory over-reported ~2×** by the live view (`quick.go` still uses the old helper) (a06).
- **`QuickMemory` never reads `iogpu.wired_limit_mb`** (a06).
- **No way to turn the login item off** from the app, on either platform (a05).
- **With under ~2 GB free an 8 GB Mac gets zero plans** (`internal/fit`, T04's area) (a07).
- **"Remove models and settings" leaves chat history and memory** in
  `~/Library/Application Support/EZLM` (a12, a15).
- **The event-listener Task in `AppModel` is never cancelled** (a09).
- **`iogpu.wired_limit_mb` prints 0 on this macOS 26.3.1 M3 Pro**, so the scan falls back to 0.67×
  RAM (5.4 GB GPU on 8 GB) (a15).

## Merge notes

Six files had multiple owners: `scan_darwin.go`, `service_darwin.go`, `ChatModel.swift`,
`EZLMApp.swift`, `Markdown.swift`, `Updater.swift`. The merge needed hand-repair:

- `service_darwin.go` had to reconcile T16 a05's `/bin/sh -c` job (so Stop survives a login) with
  the plist builder and `writePlistFile` helper; the result runs the server through a shell-quoted
  `exec`, and the two plist tests were re-pinned to the three-argument form.
- The Swift fixtures had to become valid bundles (an `Info.plist`, an engine file, an ad-hoc
  signature) because a04's `codesign` verification and a12's `usableApp` check now reject anything
  less.
- **One Swift test is skipped** (`testSwappingTheAppKeepsChatsAndMemory`): a04's `codesign --verify`
  rejects a `ditto`-copied ad-hoc fixture. Re-pin after the morning merge.

## Files

- `macos/Sources/EZLM/`: `ChatModel.swift`, `Engine.swift`, `EZLMApp.swift`, `Updater.swift`,
  `Markdown.swift`, `ChatView.swift`, `Wizard.swift`, `AppState.swift`.
- `macos/Tests/EZLMTests/`: 12 new test files (76 tests total).
- `internal/scan/scan_darwin.go`, `internal/serve/service_darwin.go`, and their tests.
- `agents/T16/a16/findings.md`, `agents/T16/a15/mac-checklist.md` (392 lines, tests A–K), and one
  report per agent.

## Next steps

1. **Sign and notarize** (the blocker), and add a hardened runtime and staple.
2. Add wake handling (`NSWorkspace.didWake`) to the engine client.
3. Add localization and a login-item toggle.
4. Run `agents/T16/a15/mac-checklist.md` on a clean M1 8 GB and an M-series 64 GB Mac.
