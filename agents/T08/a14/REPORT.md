# T08/a14: the replace step (code reading)

**Agent:** a14, cross-cutting, stretch.
**Slice:** how the Mac app swaps itself in `/Applications`, how the Windows
installer replaces files, the failure windows, and what a crash at each point
leaves behind.
**Deliverable:** `agents/T08/a14/replace.md`.
**Code reading only:** no code changes, no tests, nothing executed.

## What I did

Read the whole replace path on both platforms:

- macOS: `macos/Sources/EZLM/Updater.swift` (mount, stage, swap, relaunch),
  the caller in `macos/Sources/EZLM/Settings.swift:44-100`, the quit hook in
  `macos/Sources/EZLM/EZLMApp.swift:46-53`, `quitWarning` in
  `macos/Sources/EZLM/AppModel.swift:188-196`, and the packaging in
  `macos/scripts/build-app.sh` and `macos/scripts/make-dmg.sh`.
- Windows: the app-side hand-off in `windows/EZLM/SettingsPane.cs:254-321`
  (start, `StopEngineHelpers`, installer launch, delayed `Close`), window close
  in `windows/EZLM/Shell.cs:90-91, 526-577`, single-instance startup in
  `windows/EZLM/Program.cs:44-49`, engine lifetime in
  `windows/EZLM/Engine.cs:104`, and the installer in
  `windows/installer/EZLM.iss` (all 111 lines).
- The hand-off from the engine: `internal/app/settings.go:42-62`,
  `internal/update/update.go:111-125`.

`replace.md` has the step-by-step sequence with `file:line`, a per-platform
table of crash points and the resulting on-disk state, and findings with
severity.

## Findings

| # | Severity | Finding |
|---|---|---|
| M1 | major | macOS `Updater.swift:44-48`: two separate renames with no crash recovery. A kill or power loss between them leaves **no `/Applications/EZLM.app`**, plus a hidden `.EZLM-old-<pid>.app` that nothing ever scans for. The only restore path is the `catch` at `:48-51`, which a crash never reaches. |
| M2 | minor | `Updater.swift:36-37` clears stale staging by current PID only; killed runs leak hidden `.EZLM-update-<pid>.app` copies in `/Applications` forever. |
| M3 | major | `Updater.swift:59-67` + `EZLMApp.swift:46-53`: `relaunch` returns `true` even when `applicationShouldTerminate` returns `.terminateCancel` (quitWarning set by a model install, storage move or streaming chat). The user is told "EZLM is restarting" (`Settings.swift:87-91`) and it does not; the shell then `open`s and reactivates the old process. |
| M4 | minor | `Updater.swift:74-78`: failed `hdiutil detach` leaves the image attached and the temp mount dir behind (the `defer` cleanup at `:19` fails silently). |
| M5 | minor | Digest checked at `update.go:120`, path in the shared per-user temp dir, executed/mounted later by the UI. TOCTOU between verify and use; no re-check. |
| M6 | note | `AppPlace.temporary` is not consulted by `installUpdate`; a translocated app tries the swap, fails at `moveItem`, and falls back to the Finder drag. Acceptable, generic message. |
| W1 | major | `EZLM.iss:39-40`: in-place file replacement with `ignoreversion`, no staging, no atomic swap, no backup of the previous build. A power loss mid-copy leaves a half-replaced `{app}` with no rollback. This is the Windows bricking window. |
| W2 | major | `SettingsPane.cs:307` calls `Close()` while `updateBusy` is still `true`; `Shell.cs:526-535` therefore cancels the close and shows a "Close EZLM? ... still downloading" dialog (`:539-577`) after the installer has already started. The app keeps `Engine\ezlm.exe` open (`Engine.cs:104` only on `Closed`), so Restart Manager can hit files in use; a silent setup that then aborts is undone by `DeinitializeSetup` (`iss:82-88`) and the user is silently back on the old build after being told the update is installing. |
| W3 | major | `EZLM.iss:53` reopens `EZLM.exe` while the old process may still run; the single-instance mutex (`Program.cs:44-49`) makes the new process exit and foreground the old window. Disk and process disagree on the version until the next quit. `RestartApplications=no` (`iss:29`) is part of why. |
| W4 | minor | `EZLM.iss:42-44` deletes only `*.xbf`; any other file dropped between builds lingers in `{app}` (old native DLLs, old `Engine\` files). |
| W5 | minor | `DisableDirPage=yes` (`iss:13`) + fixed `AppId` (`:7`): an older install in another location is not moved or removed, so a second copy appears under `{localappdata}`. |
| W6 | minor | Same TOCTOU as M5, Windows side: `%TEMP%\EZLM-update\<asset>` is executed by `SettingsPane.cs:302` after the engine verified it; no signature check anywhere. |
| W7 | note | No `AppMutex` by design (`iss:30-32`); the setup cannot tell whether the app is still running and relies entirely on Restart Manager. |

## Invariants that the replace step should hold

1. After success, exactly one app exists and it is the new one. Holds.
2. After any failure or crash, the old version is still launchable. **M1 and
   W1 violate this.**
3. After the restart, the running version equals the version on disk. **W3
   violates this.**
4. The replace step touches nothing outside the app directory. Holds.

## What I changed

Nothing. This slice is code reading only and owns no code paths; `windows/**`
and `macos/**` were off limits and `internal/update/**` was not needed.

## Residual risk and open questions

- M1's severity depends on how often the process is killed in a window that is
  milliseconds wide on a normal update. It is still the only failure that needs
  a reinstall, so it is worth a startup fallback: if `EZLM.app` is missing and
  exactly one `.EZLM-old-*.app` is present in the same folder, move it back.
- Not reproduced: whether the staged bundle carries `com.apple.quarantine`
  through `ditto` from the mounted dmg, and whether a codesign re-check is
  needed before the swap. Code reading only.
- The 1.2 s delay before `Close()` (`SettingsPane.cs:307`) looks like it exists
  to let the installer start; W2 suggests the window usually does not close
  anyway. Whether Inno's Restart Manager then succeeds silently is unknown
  without a Windows machine.
- A previous-build backup under `{app}\.old` would give W1 a rollback; that is
  a build-script change, not an updater change, so it belongs to whoever owns
  `windows/installer`.
