# T15 a16 report: manual Windows beta checklist

## What I did

- Read the beta user docs (`BETA.md`), the Windows installer (`windows/installer/EZLM.iss`),
  the Windows app's entry point and panes (`windows/EZLM/Program.cs`, `Panes.cs`,
  `SettingsPane.cs`, `Engine.cs`, `ChatStore.cs`), and the Go side that the app drives
  (`cmd/ezlm/main.go`, `internal/paths`, `internal/serve`, `internal/app`, `internal/update`).
- Wrote `agents/T15/a16/windows-checklist.md`: a manual checklist for a clean Windows VM,
  run on x64 (Windows 11 24H2), ARM64 (Windows 11 24H2) and Windows 10 22H2, with an
  optional non-English pass. Sections A to P cover install, SmartScreen/Smart App Control,
  scan and model choice, download and resume, URL/key/API, chat and memory, connecting
  OpenCode/Hermes/DeepSeek Harness, keys, sign-in start and server lifecycle, Model mode,
  model storage on another drive, updates over a running app, reports and redaction,
  uninstall/upgrade/reinstall, ARM64, awkward machines (spaces, non-ASCII, OneDrive,
  long paths, no GPU, offline, proxy, port conflict), and display/accessibility.
  Section R is a 25-minute sign-off smoke for later builds; appendices hold commands,
  data locations, expected warnings and a failure record template.
- No Go code was changed, so there is no test file for this slice. Every step is
  grounded in a real control or CLI command in the tree, so a tester will not hit a
  button that does not exist.

## Findings

Ranked by severity. All are code-read; there is no Windows machine here.

### 1. Major: "Remove everything" leaves chats and memory behind

- Where: `internal/app/app.go:1586-1605` (`Uninstall`), `windows/EZLM/Panes.cs:544-546`
  (dialog says "Remove everything"), `windows/EZLM/ChatStore.cs:42` and
  `windows/EZLM/Engine.cs:25-31` (chats and memory live in `%LOCALAPPDATA%\EZLM`).
- Evidence: `App.Uninstall` removes `paths.Home()` (`%USERPROFILE%\.ezlm`) and model
  folders only. Chats (`ChatStore.Base` = `Engine.DataDir`) and `memory.md` are under
  `%LOCALAPPDATA%\EZLM`, which neither `ezlm uninstall` nor the installer's uninstall
  step touches.
- Effect: a user who presses "Remove models and settings" and confirms "Remove
  everything", or uninstalls and answers Yes, keeps their chat history and the facts
  EZLM remembered about them ("my name is Sam"). The dialog promises the opposite, and
  memory facts are the most personal data EZLM holds.
- Proposed fix: in `App.Uninstall`, remove the app's data dir too (`%LOCALAPPDATA%\EZLM`
  on Windows, the same place `Engine.DataDir` points at), or move chats/memory under
  `paths.Home()` so one delete covers everything. Add a test with a temp `LOCALAPPDATA`
  and a temp `EZLM_HOME` that asserts both are gone.

### 2. Minor: the update picker is architecture-blind

- Where: `internal/update/update.go:44-53` (`assetFor`), used at `:99-100`.
- Evidence: for Windows it accepts any asset whose name starts with `EZLM-Setup` and
  ends with `.exe`, and takes the first match in the release. It never looks at
  `runtime.GOARCH`.
- Effect: safe today, because `windows/installer/EZLM.iss` builds one installer with
  both payloads (`ArchitecturesAllowed=x64compatible or arm64`). If the beta ever ships
  separate x64 and ARM64 installers, an ARM64 machine can download and run the x64 one;
  the app would then run under emulation, and `Engine\ezlm.exe` too.
- Proposed fix: either keep one installer and assert that in the release script, or
  name assets with the architecture and match on `runtime.GOARCH`.

### 3. Minor: the uninstaller ignores whether the engine removed the data

- Where: `windows/installer/EZLM.iss:99-110`.
- Evidence: both `Exec({app}\Engine\ezlm.exe, 'uninstall', ...)` and
  `stop --for-uninstall` are called with the exit code stored in `Code` and never
  checked. If `Engine\ezlm.exe` is missing (a partial install, antivirus quarantine)
  or fails, the wizard still says nothing and the app is removed with models, settings
  and keys left on disk.
- Effect: silent leftovers, and the user has no idea why disk space did not come back.
- Proposed fix: check `Code` and show one message when it is non-zero ("EZLM could not
  remove its models. Delete %USERPROFILE%\.ezlm by hand."). Keep the app removal.

### 4. Minor: uninstall decides what to delete from the folder's name

- Where: `internal/app/app.go:154-163` (`isEZLMDir`), used at `:1598-1603`.
- Evidence: a user-chosen model folder is deleted only when its path contains "ezlm"
  (case-insensitive). `D:\Models` is kept, and a folder the user made called
  `D:\ezlm-backup` would be deleted.
- Effect: either leftover models after "delete models", or unrelated files deleted.
  The current folder picker lets the user choose any folder.
- Proposed fix: record the folders EZLM actually created or downloaded into in
  `settings.json`, delete only EZLM's files there, and say in the uninstall result
  what was left.

### 5. Minor: BETA.md points at the wrong folder for keys and settings

- Where: `BETA.md:34` versus `internal/paths/paths.go:10-36`, `internal/app/app.go:1586-1605`.
- Evidence: the doc says "Chats and memory stay on your computer:
  `%LOCALAPPDATA%\EZLM` on Windows". Chats and memory are there, but keys, settings,
  config and models are in `%USERPROFILE%\.ezlm`; the app log is in
  `%LOCALAPPDATA%\EZLM`.
- Effect: support and testers look in the wrong place, and the ACL question (a06) is
  answered against the wrong path. A second Windows account's access to
  `C:\Users\<first>\.ezlm\config.json` is the thing to test.
- Proposed fix: say both paths in BETA.md, and add the table from Appendix B of the
  checklist to the support notes.

### 6. Minor: the download name in BETA.md does not match the built asset

- Where: `BETA.md:7` ("Download **EZLM-Setup.exe**") versus
  `windows/installer/EZLM.iss:19` (`OutputBaseFilename=EZLM-Setup-{#AppVer}`, so
  `EZLM-Setup-0.1.11.exe`).
- Effect: testers who search for the exact file name do not find it; the update check
  only works because it matches the prefix.
- Proposed fix: name the file in BETA.md the way it is built, or keep a stable
  `EZLM-Setup.exe` name on the release page.

### 7. Minor: the Windows version floor is not in the docs

- Where: `windows/installer/EZLM.iss:33` (`MinVersion=10.0.19041`) versus
  `BETA.md:5` ("Windows 10 or 11").
- Effect: a user on Windows 10 1809 or 1909 gets an installer refusal with no
  explanation in the docs. The checklist covers 19045 and asks for a 19041 to 19044
  check.
- Proposed fix: "Windows 10 version 2004 (build 19041) or later, or Windows 11" in
  BETA.md.

### 8. Info: the self-test is a good per-VM gate

- Where: `windows/EZLM/Program.cs:21-26`, `windows/EZLM/SelfTest.cs`.
- `EZLM.exe --selftest <file>` runs Chat, memory, markdown and config logic in a temp
  folder with no window and no engine, and exits with a code. The checklist runs it on
  every VM (Appendix A); it catches a broken build before a tester spends an hour on
  the UI.

## What I could not verify without a Windows machine

- Every runtime step: SmartScreen wording, Smart App Control blocking, per-user install
  with no UAC, ARM64 native binaries, sign-in start, `CloseApplications` during an
  upgrade, Defender behaviour, and the installer's uninstall messages. All of these are
  code-read against `EZLM.iss` and the C# entry points.
- The exact report flow (posting to GitHub versus opening the new-issue page) is in
  `ReportLaunch.cs`, which is frozen; the checklist asks the tester to record which
  happened instead of asserting one.
- Whether `%LOCALAPPDATA%\EZLM` is readable by a second Windows account depends on the
  default profile ACLs; the checklist records the answer rather than assuming it.

## Open questions

- Is the beta one installer for both architectures, or two? Finding 2 only matters if
  it is two.
- Should "Remove models and settings" delete chats and memory as well? The dialog says
  "Remove everything", so I assumed yes (finding 1). If the intent is to keep chats,
  the dialog text should say so.
- Is `%USERPROFILE%\.ezlm` intentional, or should all per-user data move under
  `%LOCALAPPDATA%\EZLM`? One folder is easier to document, back up and remove, and it
  matches what BETA.md already tells users.

## Next steps

- Run the checklist on VM1 and VM2 as soon as a Windows machine is available; file
  each failure with its check ID (Appendix D).
- Fold findings 1 and 3 into T15 a15's severity table; both are small Go or Inno
  changes.
- Re-run section R on every release candidate, and add a check when a new pane or
  button appears.
