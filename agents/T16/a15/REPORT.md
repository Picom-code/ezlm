# T16 a15 report: manual Mac beta checklist

## What I did

- Read `README.md`, the T16 brief and `TASKS.md`, then the Mac app's user-facing code
  (`macos/Sources/EZLM/*.swift`), the packaging scripts (`macos/scripts/build-app.sh`,
  `make-dmg.sh`), and the Go side that the checklist touches: `internal/serve`
  (launchd agent, config, ports), `internal/scan/scan_darwin.go` (memory and GPU
  limits), `internal/app/app.go` (Stop, Uninstall), `internal/paths`, and
  `cmd/ezlm/main.go` (`start --login`).
- Wrote `agents/T16/a15/mac-checklist.md`: a numbered, runnable checklist for the
  beta matrix (M1 8 GB on macOS 14 and 15, M-series 16 to 128 GB on 14 and 15, plus
  a clean-account pass), with exact commands, expected results, a severity guide, an
  evidence list, and a results table.
- No app code changed. No test file added: this slice is a document, and the existing
  suite does not cover any of it.

## Findings

### 1. Major: "Stop server" is not remembered across login on macOS

`internal/serve/service_darwin.go:49-80` writes a launchd job whose
`ProgramArguments` are `c.Server` + `c.Args()`, that is `llama-server` itself, with
`RunAtLoad` true and `KeepAlive` on unsuccessful exit. The user's "stop" is recorded
only as the `~/.ezlm/stopped` marker (`internal/app/app.go:1380`,
`internal/serve/config.go:192-194`), and that marker is read only by
`ezlm start --login` (`cmd/ezlm/main.go:163-166`). The macOS job never runs that
command; only the Windows Run key does (`windows/EZLM/Program.cs:35`). So after
Overview > Stop server, the next login or restart starts llama-server again even
though the user stopped it, while the app itself honours the marker
(`macos/Sources/EZLM/AppModel.swift:248`) and shows Stopped.

This contradicts the beta text ("press Stop server" as the way to stop it) and the
Windows behaviour. It is a05's area; I did not change it. Verification is checklist
item D5, which needs a real login (a device pass).

Fix direction (for a05): have the macOS agent run `ezlm start --login` (the binary is
in the bundle's Resources) instead of llama-server directly, or have the app boot out
the agent when it writes the marker.

### 2. Minor: "Remove models and settings" leaves chat history and memory behind

`AppModel.uninstall()` (`macos/Sources/EZLM/AppModel.swift:798-809`) posts the
engine's `uninstall`, which deletes `~/.ezlm` and EZLM-owned model folders
(`internal/app/app.go:1586-1607`). Chat history and memory notes live elsewhere:
`~/Library/Application Support/EZLM` (`macos/Sources/EZLM/ChatData.swift:3-20`).
Nothing calls `ChatStore.deleteAll()` on that path, and the engine does not know about
that folder. After "Remove models and settings" and a reinstall, old chats and memory
notes come back while the model and keys are new. The sheet says "This can't be
undone" and the beta text says the Mac chats live in Application Support, so a user
who wants a clean slate has no in-app way to get one. Checklist item I7 checks this.

Fix direction (small, in an owned file): call `ChatStore.deleteAll()` from
`uninstall()` after the engine call succeeds, or say in the sheet that chats are kept.

### 3. Minor: beta text says Apple Silicon only, the build is universal

`BETA.md` says "Mac with Apple Silicon, M1 or later", but `build-app.sh` builds
`--arch arm64 --arch x86_64` and lipos both engine slices, and
`internal/scan/scan_darwin.go:14-16` has an Intel fallback with a note. Either the
text or the build is wrong; if Intel is unsupported, the app should say so before a
scan. Not testable on the matrix hardware; no code change.

### 4. Note: `iogpu.wired_limit_mb` is 0 on current macOS

On this Mac (macOS 26.3.1, M3 Pro, 18 GB), `sysctl -n iogpu.wired_limit_mb` prints
`0`. `internal/scan/scan_darwin.go:38-48` treats 0 as "unset" and falls back to
0.67 of RAM (0.75 above 36 GB), which gives about 5.4 GB of GPU-usable memory on an
8 GB Mac. That is the number the 8 GB path depends on. The checklist asks testers to
record the value on macOS 14 and 15 and on 8 GB and 64 GB machines, because a
non-zero default (some OS builds report a large value) would change the
recommendation. No evidence of a bug yet; it needs the device matrix.

## What I could not do here

- This machine is macOS 26.3.1 on an M3 Pro with 18 GB, outside the matrix, and has
  no clean account or spare 8 GB / 128 GB hardware. Nothing in the checklist was
  executed, and no model was loaded. Everything above is code reading plus one
  `sysctl` check.
- I did not build the app: no code changed, and `swift build` is slow on this shared
  machine. `swift test` was not run for the same reason. The gate for this slice is
  documentation only; the worktree has no Swift diff.
- The checklist's timing expectations (30 s engine start, 10 s stop, 60 s login
  check) come from the code (`Engine.swift:68`, `internal/serve/config.go`,
  `service_darwin.go`) and should be tightened after the first device pass.

## Open questions

1. On macOS 14 versus 15, does the launch agent get a "background item added" notice,
   and does the Gatekeeper "Open Anyway" path differ enough to need two scripts?
   (Checklist J2 asks testers to record both.)
2. Should "Remove models and settings" also remove chat history and memory? The
   Windows app keeps chats in `%LOCALAPPDATA%\EZLM`, so the two platforms differ
   today. Whoever owns uninstall should decide and make the sheet say it.
3. On an 8 GB M1, is the recommended model still comfortable with macOS 15's higher
   baseline memory use, or does the 0.67 GPU factor over-promise? Checklist B2/B3
   records the inputs; the answer needs the device.

## Commits

- `T16: add manual Mac beta checklist` (checklist and this report).
