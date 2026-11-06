# T08/a14: the replace step, end to end

Code reading only. Nothing here was executed; every claim cites the tree at
`agent/T08-a14` (snapshot of `main`). The updater's *download* half lives in
`internal/update/update.go`; this note covers what happens **after** the file is
on disk: how the Mac app swaps itself in `/Applications` and how the Windows
installer replaces files, where a crash or a stuck dialog leaves the install.

Both platforms hand off the same way: the Go engine downloads and verifies the
asset, emits a `download` `ok` event whose message is the file path, and the UI
takes over.

- Engine side: `internal/app/settings.go:42` (`DownloadUpdate`), path from
  `internal/update/update.go:111-125` (`Download` verifies the digest at
  `update.go:120`, removes the file on mismatch at `:121`).
- The path is in the per-user temp dir: `update.go:112`
  (`os.TempDir()/EZLM-update`), file name from `filepath.Base(r.Asset)`
  (`update.go:116`).

---

## 1. macOS: the `/Applications` swap

Caller: `macos/Sources/EZLM/Settings.swift:65` (`installUpdate`), reached from
the `download` `ok` event at `Settings.swift:48-51`. It checks the path ends in
`.dmg` and exists (`:66`), then:

1. **Mount** (`Updater.swift:20`): `hdiutil attach -nobrowse -readonly
   -noautoopen -mountpoint <tmp>/EZLM-update-<uuid>`. Temp dir is
   `FileManager.temporaryDirectory` (`:17`); a `defer` removes it (`:19`) and a
   second `defer` detaches (`:21`). Defer order is LIFO, so detach runs first,
   then the temp dir removal, which matches the comment on `:19`.
2. **Validate the payload** (`:23-29`): `EZLM.app` must exist with
   `Contents/MacOS`, and its `CFBundleIdentifier` must equal the running
   bundle's (`bundleID` at `:70-72`). The check only runs when *both* IDs are
   readable; if the running app's plist is unreadable the comparison is skipped
   (`:27`). The new app's ID is read from the *mounted* copy, which is
   read-only and trustworthy enough for this purpose.
3. **Stage beside the target** (`:31-41`): `ditto <mount>/EZLM.app
   <dir>/.EZLM-update-<pid>.app`, where `<dir>` is the folder containing the
   running app (`:32`). Same folder is deliberate: the later move is a rename,
   not a copy (`:31` comment). `ditto` preserves the bundle and its resource
   forks/xattrs. On `ditto` failure the partial staged copy is removed (`:38-41`).
4. **Swap** (`:44-52`), three renames in the same directory:
   - `moveItem(app, old)` where `old` is `.EZLM-old-<pid>.app` (`:44`).
     Rename, atomic on APFS/HFS+.
   - `moveItem(staged, app)` (`:48`). If this fails, `moveItem(old, app)`
     restores the old bundle (`:49`), the staged copy is removed (`:50`), and
     the error propagates to `Settings.swift:93-100`, which opens the dmg in
     Finder and tells the user to drag it over.
   - Remove the old bundle, falling back to the Trash (`:53`).
5. **Relaunch** (`Settings.swift:86-91`): `Updater.relaunch(app)`
   (`Updater.swift:59-68`) spawns `/bin/sh -c 'sleep 1; i=0; while kill -0
   <pid> && [ $i -lt 40 ]; do sleep 0.25; i=$((i+1)); done; open "<app>"'`
   (`:63-64`) and then calls `NSApp.terminate(nil)` (`:66`).

The running process keeps its old inode; the new binary takes effect on the
next launch. That part is sound.

### Failure windows (macOS)

| Crash / failure point | State on disk afterwards | Recovery |
|---|---|---|
| During `ditto` (`Updater.swift:38`) | `EZLM.app` untouched; a partial `.EZLM-update-<pid>.app` is left in `/Applications` (cleanup only happens for `ditto` errors, `:39`, not for a kill) | None automatic. Hidden dot-directory, never scanned for. Leaks one app-sized copy per interrupted run (PIDs differ each time). |
| **Between `:44` and `:48`** (old moved aside, new not yet in) | **`/Applications/EZLM.app` does not exist**; `.EZLM-old-<pid>.app` and `.EZLM-update-<pid>.app` both present | **None in code.** No startup scan for `.EZLM-old-*`. The bundle is gone from Launchpad/Dock/Finder; both leftovers are dot-named, so Finder hides them. The user must press Cmd-Shift-. or reinstall from the dmg. This is the one window that "bricks" the Mac install short of a manual recovery. |
| Between `:48` and `:53` | New `EZLM.app` in place and working; old bundle still present, hidden | Harmless except disk space; a failed remove/trash (`:53`) leaves it forever. |
| After `:53`, before `NSApp.terminate` | New app installed, old process still running the old binary | Next launch runs the new build. The user is told "EZLM is restarting" (`Settings.swift:87`) whether or not it does. |
| `NSApp.terminate` cancelled | Installed, old process still alive | See finding M3. |

Power loss at `:44-48` is the worst case: the rename sequence is atomic *per
step* but not *across* the two steps. A single `renameat2(RENAME_EXCHANGE)` or
`ditto` into a fresh bundle then `rename` of the directory over the old one
would still leave the same window; the standard fix is a two-entry fallback:
on launch, if `EZLM.app` is missing and exactly one `.EZLM-old-*.app` exists,
move it back.

### Findings (macOS)

**M1 (major, bricking).** `Updater.swift:44-48`: no recovery for a crash
between "old aside" and "new in". Leaves no `EZLM.app`. Evidence: the two
`moveItem` calls are separate statements; the only restore path is the `catch`
on `:48-51`, which a process kill or power loss never reaches. No code anywhere
reads `.EZLM-old-*` (grep: only `Updater.swift:35,37,49,53` write or delete it).

**M2 (minor).** `Updater.swift:36-37` clears stale staging using the *current
PID*. PIDs are recycled, so a crash leftover from an unrelated run can be
deleted, and a killed run's `.EZLM-update-<pid>.app` is never reclaimed. In
`/Applications` these hidden copies accumulate silently.

**M3 (major, misleading).** `Updater.swift:59-67` + `EZLMApp.swift:46-53`:
`relaunch` calls `NSApp.terminate(nil)` and returns `true` unconditionally
afterwards. `applicationShouldTerminate` can return `.terminateCancel` when
`quitWarning` is set (`AppModel.swift:188-196`: a running model install, a
storage move, or a streaming chat). The user sees a "Quit EZLM?" alert during
the update's own restart; choosing "Keep Going" keeps the old process alive,
yet `Settings.swift:88-91` has already shown "Installed. EZLM is restarting…".
The shell's 10 s wait (`Updater.swift:63`) then `open`s the app, which
activates the still-running old instance. Nothing is corrupted, but the update
appears not to have applied. A `relaunch` that waits for the PID loop before
`terminate`, or a `closeOk`-style flag that skips the alert for update
restarts, would fix it.

**M4 (minor).** `Updater.swift:74-78`: if both `hdiutil detach` attempts fail,
the mount and its temp dir stay (`defer` removal at `:19` fails silently with
`try?`). A dmg stays attached and `/tmp/EZLM-update-*` leaks until reboot.

**M5 (minor, TOCTOU).** The digest is checked at `update.go:120` on a path in
the shared temp dir, and the path is mounted/executed later by the app
(`Settings.swift:84`). Any process running as the user can replace
`$TMPDIR/EZLM-update/<asset>` in between. Low severity for a per-user temp dir,
but it is the hand-off window between "verified" and "used", and nothing
re-verifies at use time.

**M6 (note).** `AppPlace.temporary` (`Updater.swift:102-107`) detects App
Translocation and read-only volumes; `installUpdate` does not consult it
(`Settings.swift:65-84`), so a translocated copy attempts the swap, fails at
`moveItem`, and falls back to the Finder drag (`Settings.swift:93-100`). That is
acceptable behaviour, but the failure message is generic.

---

## 2. Windows: the installer replaces files

The Windows app does not swap anything itself. It starts the Inno Setup
installer and gets out of the way.

App side, `windows/EZLM/SettingsPane.cs`:

- `StartUpdate` (`:254-262`) posts `update/download`; the engine downloads and
  verifies (same `internal/update` path), then emits `download` `ok` with the
  `.exe` path.
- `OnUpdateEvent` (`:285`), on that event:
  - `StopEngineHelpers()` (`:268-281`): kills only `ezlm.exe` processes whose
    `MainModule.FileName` equals *this* install's `Engine\ezlm.exe` and whose
    PID is not the window's own engine (`:270-278`). Never by name alone.
  - `Process.Start(installer, "/SILENT /SUPPRESSMSGBOXES /NORESTART /SP-
    /CLOSEAPPLICATIONS /UPDATE=1")` with `UseShellExecute = true` (`:302`).
  - Toast "Installing the update. EZLM will open again in a moment."
    (`:303-304`), then `Close()` after 1.2 s (`:307`).
- `Closed += (_, _) => { quit.Cancel(); engine.Dispose(); }`
  (`windows/EZLM/Shell.cs:90`), and `Engine.Dispose` kills the engine after
  1.5 s (`windows/EZLM/Engine.cs:104`). So the window's engine goes away with
  the window.

Installer, `windows/installer/EZLM.iss`:

- Per-user, no elevation: `DefaultDirName={localappdata}\Programs\EZLM`
  (`:12`), `PrivilegesRequired=lowest` (`:16`), directory page disabled
  (`:13`).
- Replaces every shipped file in place: `Source: {#SrcX64}\*` /
  `{#SrcArm64}\*` with `Flags: ignoreversion recursesubdirs
  createallsubdirs` (`:39-40`). `ignoreversion` means always overwrite; there
  is no staging directory and no directory-level rename.
- Deletes only `{app}\*.xbf` from older builds (`:42-44`).
- `CloseApplications=yes`, `RestartApplications=no` (`:28-29`). No `AppMutex`
  on purpose (`:30-32`) so the silent self-update is not aborted mid-exit;
  in-use files are left to Restart Manager.
- Reopens the app twice: after a normal install via the postinstall entry
  (`:51`), and for `/UPDATE=1` via `[Run] ... Check: IsUpdate` (`:53`).
- `DeinitializeSetup` (`:78-89`): if this was an update (`IsUpdate`,
  `:64-67`) and the install never reached `ssPostInstall` (`:69-73`), it
  reopens `{app}\EZLM.exe` if it exists, so an aborted update does not leave
  the user with no window.
- `MinVersion=10.0.19041` (`:33`), `ArchitecturesAllowed=x64compatible or
  arm64` (`:17`).

### Failure windows (Windows)

| Crash / failure point | State on disk afterwards | Recovery |
|---|---|---|
| App starts the installer, then crashes before `Close()` (`SettingsPane.cs:302-307`) | Nothing replaced yet. Installer still runs as its own process. | Installer proceeds; Restart Manager closes the (already dead) app; `[Run]` opens it (`iss:53`). Fine. |
| Setup aborts before `ssPostInstall` (file in use, `MinVersion`, arch) | Old version intact; user got the "will open again" toast (`SettingsPane.cs:303`) but setup showed nothing (`/SILENT /SUPPRESSMSGBOXES`) | `DeinitializeSetup` (`iss:82-88`) reopens the old `EZLM.exe`. The user sees the old build and no error. |
| **Power loss / kill during the file copy** (`iss:39-40`) | Mixed-version install: some files new, some old, possibly a truncated `EZLM.exe`/DLL. No backup copy of the previous build is kept for EZLM to restore (Inno's rollback only runs inside a live setup). | **None.** No repair path: the next launch runs whatever `EZLM.exe` is there; if it is truncated, nothing starts and `DeinitializeSetup` is not involved (it is part of the same, now-dead, setup). This is the Windows bricking window. |
| Files copied, before `[Run]` (`iss:51-53`) | New build on disk, no process | Nothing opens it; the user reopens from the Start menu and gets the new build. Acceptable. |
| **Old app still running when `[Run]` opens the new one** (`iss:53`) | New build on disk; the new process hits the single-instance mutex (`windows/EZLM/Program.cs:44-49`) and exits after activating the old window | The user keeps using the **old** process while disk holds the new build; the version only changes at the next real quit/start. See W2. |
| Installer succeeds but EZLM.exe won't start | New build on disk, no window | User retries; if it crashes on boot there is no rollback to the previous build. |

### Findings (Windows)

**W1 (major, bricking).** `iss:39-40`: in-place replacement with
`ignoreversion`, no staged directory, no atomic swap, no previous-version
backup. A crash or power loss mid-copy leaves a partially replaced `{app}` and
no way back. The macOS swap at least gets a rename; Windows gets neither
atomicity nor rollback.

**W2 (major, silent no-op).** `SettingsPane.cs:307` calls `Close()` while
`updateBusy` is still `true` (it is only cleared on error, `:308-312`, or on
the `done` event, `:316-321`). `Close()` goes through
`AppWindow.Closing` (`Shell.cs:91`), i.e. `OnWindowClosing` (`:526`), which
cancels the close when `what != null` (`:535`) and shows a "Close EZLM?" dialog
(`:539-577`) whose text says EZLM is *downloading* its update. So the window
does not actually close; the app keeps its `Engine\ezlm.exe` open
(`Engine.cs:104` runs only on `Closed`, `Shell.cs:90`) while the installer is
already running. Restart Manager's WM_CLOSE re-enters the same handler and can
cancel again. In `/SILENT` mode the setup may then find files in use; if it
aborts, `DeinitializeSetup` reopens the old build (`iss:82-88`) and the user,
who was told the update is installing, is silently back on the old version.
Fix shape: set `closeOk = true` (or `updateBusy = false`) before the
update-path `Close()`.

**W3 (major, state skew).** `iss:53` opens `EZLM.exe` while the old process may
still be alive; the single-instance mutex (`Program.cs:44-49`) makes the new
process exit and foreground the old window. Disk and process disagree about the
version until the user quits. `RestartApplications=no` (`iss:29`) is part of
why: the setup does not wait for or restart the app itself.

**W4 (minor).** `iss:42-44` deletes only `*.xbf`; any other file dropped or
renamed between builds stays in `{app}` next to the new ones (old native DLLs,
old `Engine\` files). Version skew that only shows up at runtime.

**W5 (minor).** `DisableDirPage=yes` (`iss:13`) plus `AppId` (`:7`) means an
older install in a different location (e.g. a per-machine `Program Files`
build) is not moved or removed: setup installs a second copy under
`{localappdata}`, and the old shortcut can keep launching the old build.

**W6 (minor, TOCTOU).** Same hand-off as M5: the digest is checked in the
engine (`update.go:120`), the path sits in `%TEMP%\EZLM-update`, and the app
executes it later (`SettingsPane.cs:302`) with no signature check. Anything
that can write the user's temp dir between verify and exec wins.

**W7 (note).** `iss:30-32` deliberately omits `AppMutex`, which is right for a
silent self-update, but it also means the setup cannot tell whether the app it
is about to replace is still running; it relies entirely on Restart Manager.

---

## 3. Cross-platform summary

| | macOS | Windows |
|---|---|---|
| Who replaces | the app (`Updater.install`) | the installer (`EZLM.iss`) |
| Mechanism | `ditto` to a staged bundle, then two renames in the same directory (`Updater.swift:38-52`) | Inno copies files into `{app}` in place (`iss:39-40`) |
| Atomic? | per rename; **not** across the pair | no |
| Previous version kept? | yes, as `.EZLM-old-<pid>.app`, deleted after the swap (`:53`) | no |
| Worst leftover | no `EZLM.app` at all, hidden old copy (M1) | half-replaced install (W1) |
| Restart | shell waits for the PID, then `open` (`Updater.swift:63-64`) | `[Run]` + `DeinitializeSetup` (`iss:51-53, 78-89`) |
| Blocks a stuck app? | no | yes: W2's close dialog |

Invariants worth asserting in the threat model (a11):

1. After a successful `install`, exactly one `EZLM.app`/`{app}\EZLM.exe`
   exists and it is the new one.
2. After any failure, the old version is still launchable. **macOS M1 and
   Windows W1 both violate this.**
3. The version on disk and the version in the running process are the same
   after a restart. **Windows W3 violates this.**
4. Nothing outside the app directory is touched by the replace step. Holds on
   both (the dmg mount and temp file aside).

## Open questions

- Does the macOS swap need to `xattr -d com.apple.quarantine` or re-verify the
  code signature of the staged bundle? `ditto` preserves xattrs from the
  mounted image; a quarantined inner bundle would prompt on next launch. Not
  reproduced (code reading only).
- Is the 1.2 s delay (`SettingsPane.cs:307`) enough for Inno to unpack before
  the app exits? If not, W2's dialog is what actually keeps the app alive, and
  the timing is accidental rather than designed.
- Should the Windows installer keep the previous build under `{app}\.old` for
  one run, so W1 has a rollback? Inno can do this with a `[Files]` backup or a
  post-install copy; it is a build-script change, not an updater change.
