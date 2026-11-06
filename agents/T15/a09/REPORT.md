# T15 / a09: the installer (per-user install, upgrade over a running app, uninstall, registry, shortcuts)

Scope: `windows/installer/EZLM.iss`, the Go side of the login entry and uninstall
(`internal/serve/service_windows.go`, `internal/update`), and the C# that drives an
in-app update (`windows/EZLM/SettingsPane.cs`, `Program.cs`, `Engine.cs`) — C# read only.

No Windows machine here. Everything below is code-read unless it says "test".

## Changed (Go only)

- `internal/serve/runkey.go` (new, cross-platform), `internal/serve/service_windows.go:93`,
  `internal/serve/runkey_test.go` (new).
  `setRunKey` wrote `"<ezlm.exe>" start` for a CLI-only install (no `EZLM.exe` next to the
  engine). Plain `start` removes the `stopped` flag (`cmd/ezlm/main.go:172-174`), so a user who
  pressed Stop got the model server back at every sign-in, undoing their choice; the app path
  (`EZLM.exe --autostart`) correctly runs `start --login` (`windows/EZLM/Program.cs:30-40`).
  The Run value now uses `start --login` in both cases. Test
  `TestLoginArgsKeepsAStoppedServerStopped` fails before the change (`"start"`) and passes after.

## Findings, by severity

### 1. CLI login entry undoes a Stop — major (Go, fixed, tested)

`internal/serve/service_windows.go:93` (before the fix). Evidence: `app.Stop` writes
`~/.ezlm/stopped` (`internal/app/app.go:1380`); only `start --login` honours it
(`cmd/ezlm/main.go:163-168`). Fix as above.

### 2. A silent update can land in an app that is still holding the single-instance mutex — minor (C#, code-read)

`windows/EZLM/SettingsPane.cs:300-303` starts setup and closes the window after 1.2 s;
`EZLM.iss:50-53` reopens `{app}\EZLM.exe` when setup finishes. `Program.cs:39-45` takes
`Local\EZLM-app` and, if it is taken, only restores a window found by title. If the old
process is slow to exit (its engine pipe, WinUI teardown), the new instance exits silently and
the user sees no window after an update. Proposed fix: retry the mutex for a few seconds in
`Program.cs` before falling back to `FindWindow`. Low probability, high annoyance.

### 3. `IsArm64` decides which payload is installed — verify on ARM64 (Inno)

`EZLM.iss:17-18,39-40`: the x64 payload is installed when `not IsArm64`. `ArchitecturesAllowed`
already accepts `x64compatible` (which covers ARM64 running x64 code under emulation). If
`IsArm64` reflects the *Setup process* architecture rather than the OS on a machine running the
installer under emulation, an ARM64 machine gets the x64 engine and llama.cpp runs emulated.
Needs one run of the built setup on an ARM64 VM (`EZLM.iss:39-40` + a check of
`{app}\EZLM.exe`'s PE machine type). This overlaps a07's slice; it is the installer half.

### 4. Model removal on uninstall is unchecked — minor (Inno)

`EZLM.iss:103-109`: `Exec(... 'uninstall', ...)` ignores the exit code and does not check that
`{app}\Engine\ezlm.exe` exists. When it is missing or fails (engine already half removed, AV
lock, a 32-bit path oddity), the user answered "delete my models and settings" and both stay in
`%USERPROFILE%\.ezlm` with no message. Proposed fix: check `FileExists` and `Code`, and fall
back to deleting `{userprofile}\.ezlm` (the engine's home, `internal/paths/paths.go:13-20`),
or report it with `MsgBox`. `ewWaitUntilTerminated` with no timeout also means a hung engine
freezes the uninstaller with no progress.

### 5. Version is bumped by hand in three places — minor (release process)

`EZLM.iss:4` (`AppVer 0.1.11`), `windows/EZLM/EZLM.csproj:18` (`0.1.11`),
`internal/version/version.go:5` (`0.1.11-beta`). The uninstall entry and "Apps & features"
show one version, the app and the update check another (`internal/update/update.go:105`
compares the GitHub tag with the Go constant). A forgotten bump ships an update that either
repeats or never appears. Proposed fix: generate `AppVer` from a single file at build time
(`ISCC /DAppVer=...`) or read the csproj.

### 6. The updater picks the first `EZLM-Setup*.exe` — minor (Go, `internal/update/update.go:45-53`)

`assetFor` ignores `runtime.GOARCH` and returns the first matching asset in API order. Today
the one setup carries both payloads, so it works; the day a release publishes
`EZLM-Setup-<ver>-arm64.exe`, ARM64 users get whichever the API lists first (usually x64).
Proposed fix: prefer a name containing the arch, fall back to the generic name, and error when
two generic candidates exist.

### 7. Downloaded installers pile up in `%TEMP%` — minor (Go, `internal/update/update.go:112-116`)

Every update leaves `%TEMP%\EZLM-update\EZLM-Setup-<ver>.exe` (~150 MB, self-contained WinUI +
engine) and nothing ever deletes it: not the installer, not the next update (it only overwrites
the same version's file). Proposed fix: delete other `EZLM-Setup-*.exe` in that folder before
downloading, or have the installer clean the folder in `[InstallDelete]`/`[UninstallDelete]`.

### 8. Desktop shortcut from an earlier install survives an upgrade — minor (Inno)

`EZLM.iss:46-48`: the desktop icon is created under `Tasks: desktopicon` but nothing removes a
previously created one when the task is unticked on an upgrade (Inno only removes icons at
uninstall). Minor UX; a `[InstallDelete]` entry for `{autodesktop}\EZLM.lnk` conditioned on
`not WizardIsTaskSelected('desktopicon')` in `CurStepChanged(ssInstall)` fixes it.

### 9. `EnsureAutostart` never repairs a stale login entry — minor (Go, `internal/serve/service_windows.go:108-126`)

It repairs only when the value is missing or empty. An entry pointing at a deleted
`EZLM.exe` (install folder removed by hand, moved, or a leftover from another user profile) is
kept forever, and the failure is silent at every sign-in. `NoLoginTask` is also never set
from this path. Proposed fix: parse the quoted exe out of the value and repair when it does
not exist.

### 10. Uninstall shortcut and per-user uninstaller entry — minor (Inno)

No `{autoprograms}\Uninstall EZLM` icon (`EZLM.iss:46-48`); users must go through
Settings > Apps. The uninstall entry itself is fine: `PrivilegesRequired=lowest` writes it
under `HKCU\...\Uninstall\{AppId}_is1`, per user, no admin prompt.

## Reviewed and found OK (so nobody re-checks them)

- `AppId={{GUID}` is the correct Inno literal-brace form, one AppId for both architectures.
- Per-user install: `PrivilegesRequired=lowest` + `{localappdata}\Programs\EZLM` +
  `{autoprograms}`/`{autodesktop}` resolve to the user's folders; no UAC prompt.
- The `[Registry]` Run-key delete (`EZLM.iss:57`) matches what Go writes
  (`runKeyPath`/`runValue`, `internal/serve/service_windows.go:81`), and
  `ValueType: none` + `uninsdeletevalue` + `dontcreatekey` is the documented way to delete a
  value the app created without creating it.
- Upgrade over a running app: `CloseApplications=yes` + the app's `/CLOSEAPPLICATIONS` and
  `/SILENT /SUPPRESSMSGBOXES` (`SettingsPane.cs:302`) is the right combination; `AppMutex` is
  deliberately omitted so the self-update cannot be blocked by its own process, and
  `DeinitializeSetup` reopens EZLM when setup stops early. The engine helpers are killed
  before setup starts (`SettingsPane.cs:268-284`).
- `[InstallDelete]` of `*.xbf` and `ignoreversion` on `[Files]` cover the stale-artifact case.
- Uninstall keeps models/settings when the user says No (silent uninstall answers No, so a
  managed silent uninstall never deletes data), and the login entry is still removed.

## Could not verify without a Windows machine

- The `.iss` does not compile here (no ISCC) and nothing in the snapshot builds it: there is
  no `.github` folder at all, so the setup is assembled by hand at release time. A syntax or
  path error in `EZLM.iss` is only found when someone tries to ship.
- Restart Manager closing `{app}\Engine\ezlm.exe` (a console child) and the ARM64 `IsArm64`
  question (#3) need a VM run.
- `schtasks` legacy-task cleanup (`service_windows.go:70-74,123-125`) and the Run key write
  need a real HKCU.

## Open questions

- Should the uninstaller offer to keep models by default when the model folder is on another
  drive (the engine supports that, `paths.SetModels`)? The prompt only mentions "models EZLM
  downloaded".
- Is `EZLM-Setup-<ver>.exe` (one file, two payloads) going to stay, or will releases publish
  per-arch setups? #6 depends on the answer.
