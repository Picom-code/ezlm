# EZLM Windows beta: manual test checklist

Manual pass over a clean Windows VM, on x64 and on ARM64, before the public beta.
Everything here is done by hand in the app; the Go tests cover the logic, this covers
the install, the first run, the drivers and the edges a test can't reach.

Status: draft for the 0.1.11 beta. Owner: whoever runs the build.

- App version: 0.1.11 (installer `EZLM-Setup-0.1.11.exe`)
- Engine: bundled in `Engine\ezlm.exe`
- Time: about 75 minutes for a full run on one VM, about 25 minutes for the
  x64 smoke pass (section R) on later builds.

## 1. What this covers

| Area | Sections |
|---|---|
| Install, SmartScreen, shortcuts, per-user install | A |
| First run, scan, model choice, download | B, C |
| URL and key, the OpenAI-compatible API | D |
| Chat, memory, history, files | E |
| Connect OpenCode, Hermes, DeepSeek Harness | F |
| Keys: add, rotate, revoke | G |
| Server start/stop, sign-in start, reboot | H |
| Model mode (free up memory) | I |
| Switching models, model storage on another drive | J |
| Updates, update over a running app | K |
| Report a problem, copy details | L |
| Uninstall, keep or delete models, reinstall | M |
| ARM64 specifics | N |
| Awkward machines: names, locales, no GPU, small disk | O |
| Display scaling, keyboard, accessibility | P |
| Sign-off smoke for later builds | R |

## 2. Builds and VMs

Run the full checklist on the first three. The last row is a stretch pass, done once
if a machine is free.

| ID | Windows | Arch | Why |
|---|---|---|---|
| VM1 | Windows 11 24H2, fresh | x64 | The common case. |
| VM2 | Windows 11 24H2, fresh | ARM64 | Snapdragon / ARM laptop. Most likely to break. |
| VM3 | Windows 10 22H2 (19045) | x64 | Oldest supported family. Installer minimum is 10.0.19041. |
| VM4 | Windows 11, non-English display language | x64 | Optional. Only if a free VM is available. |

For each VM record: Windows edition, version and build (`winver`), architecture
(`$env:PROCESSOR_ARCHITECTURE`), RAM, GPU and driver version, free disk space, and
whether the account is an administrator or a standard user.

### Clean VM setup

1. Start from a checkpoint with no EZLM installed: no `%LOCALAPPDATA%\Programs\EZLM`,
   no `%USERPROFILE%\.ezlm`, no `%LOCALAPPDATA%\EZLM`, and no `EZLM` value under
   `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`.
2. Take a snapshot before installing, so a failed run can be repeated from the same state.
3. Leave Windows Defender and Smart App Control at their defaults. Do not add exclusions:
   the beta has none, so exclusions would hide the real first-run experience.
4. Connect the VM to the internet. Note the download speed, since the model download
   is 2 to 60 GB.
5. Do not install OpenCode, Hermes or DeepSeek Harness first. Section F tests EZLM
   installing them.

## 3. How to record results

Use one row per check: `ID | Pass / Fail / Blocked / Skipped | Notes | Evidence`.

- **Pass**: the expected result happened.
- **Fail**: something else happened. Take a screenshot and attach the logs (section 3.1).
- **Blocked**: a previous check failed and this one depends on it.
- **Skipped**: the machine cannot run it, for example no discrete GPU.

Record the exact wording of any error message, not a paraphrase. Record how long a
step took when it is slow (install, first model load, update).

### 3.1 Evidence to collect on a failure

- Screenshot of the window, including the error text.
- `%USERPROFILE%\.ezlm\logs\` (engine logs) and `%LOCALAPPDATA%\EZLM\` (`app.log`,
  `engine.log`, chats, memory).
- Output of the commands in Appendix A.
- The report ID or issue URL if "Report a problem" worked (section L).

## 4. Pass criteria for the beta

A build does not go out if any of these fail on VM1 or VM2:

- The installer completes with no administrator prompt, and EZLM opens.
- A model can be chosen, downloaded, resumed after cancel, and started.
- The URL and key answer a chat completion request.
- Chat works, remembers a fact across a new chat, and history reopens.
- At least one of OpenCode, Hermes or DeepSeek Harness connects and answers.
- EZLM starts the server after signing in to Windows, and stays stopped if the user
  pressed Stop server.
- The in-app update installs over the running app and reopens it with the model kept.
- Uninstall removes the app, shortcuts and the Run entry, and honours the keep-models
  answer.
- Nothing in a report includes the API key or the user's chat text.

## A. Install and first launch

Do this on a clean VM, signed in as the account the user will use.

- **A1 Per-user install.** Run `EZLM-Setup-0.1.11.exe`. Expected: no UAC prompt, no
  administrator password, installs under `%LOCALAPPDATA%\Programs\EZLM`, and the
  wizard opens EZLM at the end. Repeat on a standard (non-admin) account on VM1.
- **A2 SmartScreen.** On first run Windows may show "Windows protected your PC".
  Expected: More info, then Run anyway, then the installer continues. Note the exact
  message and whether the publisher line is "Unknown publisher".
- **A3 Smart App Control.** On a Windows 11 VM with Smart App Control on, expect the
  installer to be blocked with no Run anyway button (documented limit in BETA.md).
  Turn it off, retry, and note the build where it was on.
- **A4 Shortcuts.** Leave the desktop icon task unticked: expect a Start menu shortcut
  only. Reinstall with it ticked: expect a desktop icon that opens EZLM.
- **A5 Install location.** Confirm `%LOCALAPPDATA%\Programs\EZLM\EZLM.exe` exists and
  `Engine\ezlm.exe` sits next to it. Confirm `Assets\` is present.
- **A6 Minimum Windows.** On VM3 (Windows 10 19045) the installer runs. If a 19041 to
  19044 build is available, confirm it also installs; below 19041 the installer should
  refuse with a clear version message.
- **A7 Architecture.** On VM2 (ARM64), confirm `EZLM.exe` and `Engine\ezlm.exe` are
  ARM64 binaries (Task Manager > Details > Platform, or `dumpbin /headers`), not x64
  running under emulation. Note the platform column for both.
- **A8 First window.** EZLM opens maximised or restored, titled EZLM, with the left
  list: Overview, Chat, Models, System, Connect, Keys, Logs, Settings. No blank window,
  no "engine didn't start" error.
- **A9 Second launch.** With EZLM open, launch it again from the Start menu. Expected:
  the existing window comes to the front, no second window, no second engine process
  in Task Manager.
- **A10 Close and reopen.** Close the window, reopen. Expected: the engine process
  exits with the app (it reads the app's stdin) and a new one starts. Confirm in Task
  Manager that no `ezlm.exe` is left behind after 10 seconds. `llama-server.exe`
  keeps running by design: connected apps use it while EZLM is closed.

## B. First run and scan

- **B1 Empty state.** Overview says "Nothing is set up yet" with a "Choose a model"
  button.
- **B2 Scan.** Press "Choose a model". Expected: a short scan, then a model choice.
  No error dialog on a machine with no discrete GPU, or with an unknown GPU.
- **B3 System page.** Open System. Expected: System, Processor, Graphics, Engine and
  Storage lines, all filled in. On a machine with no GPU, Graphics says "None found"
  or "Built into the chip". Engine says one of NVIDIA CUDA, Vulkan, AMD ROCm,
  Processor only. Note the exact values.
- **B4 Memory numbers.** The memory bars update while the window is open (wait 5 to 10
  seconds). Numbers are plausible against Task Manager.
- **B5 Scan again.** Press "Scan again" on Models. Expected: the page refreshes without
  losing the current model or settings.
- **B6 Old or small machine.** On an 8 GB, no-GPU VM (stretch), the scan still finishes
  and offers a model that fits. Note the model and the estimated size.

## C. Choose and download

- **C1 Modes.** Switch Balanced, Smartest, Fastest. Expected: the pick and the
  estimated download size change, and the recommended model is plausible for the
  machine (bigger with more RAM and a GPU).
- **C2 Start.** Start the download. Expected: a progress line with size and speed, and
  the buttons stay responsive.
- **C3 Cancel and resume.** Cancel mid-download, then start again. Expected: the
  download resumes from the partial file, not from zero. Confirm in Task Manager or
  the progress numbers.
- **C4 Close during download.** Close EZLM mid-download, reopen, and start again.
  Expected: it resumes; the partial file is kept.
- **C5 Disk space.** On a VM with little free space (stretch), start a model larger
  than free space. Expected: a clear "not enough space" message before downloading,
  not a failure at the end.
- **C6 Progress honesty.** The percentage, downloaded size and speed move, and the
  speed is in the same range as the connection. No frozen 0% for more than 30 seconds
  without a message.
- **C7 Finish.** The model installs, the engine starts, and Overview shows the model
  with a Live badge. Note the time from start to Live.

## D. URL, key and the API

- **D1 Endpoint.** Overview shows a URL of the form `http://127.0.0.1:8080/v1` (the
  port may differ) and a key. Copy both.
- **D2 Browser check.** Open the URL in a browser. Expected: a response from the
  server (a JSON error or model list is fine), not a connection refused.
- **D3 Chat completion.** In PowerShell:

  ```powershell
  $u = "http://127.0.0.1:8080/v1"
  $k = "PASTE-KEY"
  Invoke-RestMethod -Uri "$u/chat/completions" -Method Post `
    -Headers @{ Authorization = "Bearer $k" } -ContentType "application/json" `
    -Body '{"model":"MODEL-NAME","messages":[{"role":"user","content":"Say hello in five words."}]}'
  ```

  Use the model name shown on Overview in place of `MODEL-NAME`.
  Expected: a completion. A wrong key returns 401. A missing key returns 401.
- **D4 Model list.** `Invoke-RestMethod -Uri "$u/models" -Headers @{ Authorization = "Bearer $k" }`
  returns the deployed model.
- **D5 Test model.** Press "Test model" on Overview. Expected: the checks pass and
  the results stay on the page. Press "Test again" and confirm it repeats.
- **D6 Port change.** `ezlm port 8090` (Appendix A). Expected: the server moves, the
  URL on Overview updates, the old port stops answering, and connected apps follow.
  Restore the port afterwards.
- **D7 Key in the URL.** Confirm the key is not put in the URL; apps use the Bearer
  header. Check the Endpoint block shows URL and key separately.

## E. Chat and memory

- **E1 First message.** Chat, type a question, send. Expected: the answer streams in
  and formats (bold, list, code block with a copy button).
- **E2 Stop.** While an answer streams, press Stop (or send another message). Expected:
  the stream stops cleanly and the partial answer stays.
- **E3 Remember.** Tell it "My name is Sam and I live in Austin." Then start a New chat
  and ask "What is my name?". Expected: it answers Sam.
- **E4 /remember and /forget.** Type `/remember I am a teacher`, then `/forget teacher`.
  Expected: each confirms and the memory list changes.
- **E5 Memory pane.** Open the memory view. Expected: facts listed, editable, memory
  can be turned off, and a fact added by hand is kept.
- **E6 History.** Reopen an earlier chat. Expected: messages, files and order are
  restored. Delete a chat and confirm it is gone after a restart.
- **E7 Attach a file.** Attach a small text file (or drag one in). Expected: the file
  name is shown and the answer uses it. Try a file with spaces and a non-ASCII name.
- **E8 Retry and Listen.** Retry an answer. Expected: a new answer replaces or follows
  the old one, with no duplicate user message. Listen reads the answer aloud if a
  voice is installed; note if it is greyed out.
- **E9 Long chat.** Send about 20 messages, or paste a long text. Expected: the chat
  trims old turns instead of failing with a context error, and a context error, if it
  appears, is readable and says what to do.
- **E10 Restart.** Close and reopen EZLM. Expected: chats and memory are still there.

## F. Connect apps

Test with OpenCode, Hermes and DeepSeek Harness, in that order if time is short.

- **F1 Connect.** Connect page, pick one app, press Connect. Expected: EZLM installs
  it if missing, adds itself as a provider, and the app shows as connected with a
  green state.
- **F2 It answers.** Open the app from EZLM's button. Expected: it starts with EZLM's
  model selected and answers a prompt.
- **F3 Already installed.** With the app already installed by the user, Connect again.
  Expected: EZLM edits its config rather than reinstalling, and the app still works.
- **F4 Doctor.** `ezlm doctor apps` (Appendix A) reports each app's state and launch
  preflight. Expected: connected apps pass; a missing app says so without an error.
- **F5 Connect again.** Press "Connect again" on a connected app. Expected: it repairs
  the config and still answers.
- **F6 Report from Connect.** "Report a problem" on an app row opens the report flow
  (section L), not a raw error.
- **F7 Wrong architecture.** On ARM64, confirm the app EZLM installs runs natively or
  under emulation and still answers. Note which.

## G. Keys

- **G1 New key.** Keys page, New key, name it "laptop". Expected: a second key appears.
- **G2 Both keys work.** Call the API with each key. Expected: both answer.
- **G3 Rotate.** Rotate the second key. Expected: the old value stops working, the new
  one works, and the first key still works. Apps EZLM connected are updated.
- **G4 Revoke.** Revoke the second key. Expected: it disappears and stops working; the
  first key still answers. Revoke is not offered for the last key.
- **G5 Key display.** Keys are shown in full when revealed, masked otherwise, and the
  copy button copies the right value. Nothing is logged in plain text in `app.log`
  (search for the key prefix; the full key should not appear).

## H. Server lifecycle and sign-in start

- **H1 Stop.** Press "Stop server". Expected: the badge says Stopped, the URL stops
  answering, and `llama-server` is gone from Task Manager.
- **H2 Start.** Press "Start server". Expected: it loads and goes Live, and the same
  URL and key work.
- **H3 Sign-in start.** With the server running, sign out and sign back in (or reboot).
  Expected: EZLM is not shown, and the model server starts in the background within a
  couple of minutes. The URL answers without opening EZLM.
- **H4 Stay stopped.** Press "Stop server", then sign out and in. Expected: the server
  stays stopped (EZLM records the choice) and does not start at sign-in.
- **H5 Run entry.** With a model set up, check
  `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, value `EZLM`. Expected: it
  points at `EZLM.exe --autostart`, and it is absent before setup.
- **H6 Startup apps.** Task Manager > Startup apps lists EZLM and disabling it stops
  the sign-in start.
- **H7 Port taken.** With the server stopped, occupy port 8080 with another program,
  then start. Expected: EZLM picks a free port and the URL updates, with no crash.
- **H8 Unmanaged server (stretch).** Start `llama-server` yourself as administrator,
  then open EZLM. Expected: the "EZLM can't control this server" note appears and the
  app still shows the endpoint.
- **H9 Two users.** With EZLM set up for one Windows account, sign in as a second
  account. Expected: the second account does not see the first account's keys or
  chats, and EZLM offers setup. Note whether the second account can read
  `C:\Users\<first>\.ezlm\config.json`.

## I. Model mode (free up memory)

- **I1 Start.** Press "Free up memory" on Overview, or "Start Model mode" on System.
  Expected: a list of apps using memory, biggest first.
- **I2 One at a time.** Choose an app. Expected: EZLM asks before closing it, and only
  that app closes.
- **I3 Nothing closes without OK.** Decline one app. Expected: it stays open and the
  flow continues.
- **I4 Unsaved work.** Try an app with unsaved changes (for example Notepad with text).
  Expected: the prompt warns, or the app is left alone.
- **I5 After.** Memory numbers drop and the model still answers. EZLM itself is never
  in the list to close.

## J. Models and storage

- **J1 Switch.** Models, switch to a smaller model. Expected: download (or reuse),
  restart, and the URL and keys stay the same. Connected apps still answer.
- **J2 Remove.** Remove an installed model. Expected: its files go, the current model
  is untouched, and the list updates.
- **J3 Storage folder.** Settings, Choose a folder on another drive (create
  `D:\EZLM-models`). Expected: EZLM offers to move existing models; after moving, the
  model still runs from the new folder.
- **J4 Storage survives restart.** Restart EZLM. Expected: the chosen folder is still
  the model folder, and System > Storage shows it.
- **J5 Folder with spaces and non-ASCII.** Use `D:\My Models\EZLM ä`. Expected: the
  download and the server work.
- **J6 Remove models and settings.** System, "Remove models and settings". Expected: a
  confirmation, then the server stops, models and `%USERPROFILE%\.ezlm` are gone, and
  Overview returns to "Nothing is set up yet". The app itself stays installed.

## K. Updates

- **K1 Check.** Settings > Check for updates. Expected: either "up to date" or the
  new version with notes. No error on a blocked or slow network; a readable message.
- **K2 Update.** Press "Update now" on Overview (or Settings). Expected: download with
  progress, a verification step, EZLM closes, the installer runs silently, and EZLM
  reopens on the new version with the model still running and the URL unchanged.
- **K3 Update over a running app.** Same as K2 with a chat open and the server Live.
  Expected: no file-in-use error; if the install fails, EZLM reopens on the old
  version so the user is not left without an app.
- **K4 Models kept.** After the update, the existing model is still installed and
  selected; no re-download.
- **K5 Bad download (stretch).** Block the download host mid-update, or use a proxy
  that truncates. Expected: a clear error, no half-installed app, and EZLM still opens.

## L. Report a problem

- **L1 Report.** Press "Report a problem" on an error and from Logs. Expected: the
  report flow opens, shows what will be sent, and either posts to GitHub or opens the
  new-issue page with the body filled in.
- **L2 Redaction.** Open the report body. Expected: no API key, no chat text, no file
  contents, no user name beyond what is needed. Search for the key value and for a
  distinctive word you typed in chat.
- **L3 Copy details.** "Copy details for support" copies the hardware summary and
  recent logs, and pastes into Notepad with readable line breaks.
- **L4 Logs page.** Logs update live, "Copy all" copies the visible lines, and the
  page does not grow without bound during a long session.

## M. Uninstall, upgrade and reinstall

- **M1 Uninstall, keep models.** Settings > Apps > Installed apps > EZLM > Uninstall.
  Answer No to the models question. Expected: the app folder is removed, models and
  settings stay, the Run entry and both shortcuts are gone, and no `ezlm.exe` or
  `llama-server` process is left.
- **M2 Reinstall, keep models.** Reinstall the same version. Expected: EZLM finds the
  existing models and does not re-download.
- **M3 Uninstall, delete models.** Uninstall again and answer Yes. Expected: models,
  settings, keys, chats and memory are gone, including a model folder on another
  drive that EZLM created. Confirm free space returns.
- **M4 Foreign model folder (stretch).** Choose a folder that does not contain "ezlm"
  in its name (for example `D:\Models`), download a model, uninstall with Yes.
  Expected: EZLM deletes only its own files there, or says what it left. Note what
  remains.
- **M5 Install over a running app.** With EZLM open and the server Live, run the
  installer again. Expected: Windows closes EZLM, the install completes, and EZLM
  opens again.
- **M6 Leftovers.** After uninstall, check for `%LOCALAPPDATA%\Programs\EZLM`,
  `%USERPROFILE%\.ezlm`, `%LOCALAPPDATA%\EZLM`, the Run value, the Start menu entry
  and the desktop icon. Record anything left and its size.
- **M7 Uninstall while stopped.** Stop the server, then uninstall. Expected: no error
  about a missing engine, and the same leftovers result.

## N. ARM64 specifics

Run on VM2, and repeat A7, D3 and E1.

- **N1 Native binaries.** Both `EZLM.exe` and `Engine\ezlm.exe` are ARM64 (Task
  Manager Platform column, or `Get-Process | Select-Object Name, Path` plus
  `dumpbin`). No x64 emulation for either.
- **N2 Child processes.** `llama-server.exe` is ARM64. Check Task Manager while Live.
- **N3 No GPU path.** On a Snapdragon VM with no discrete GPU, the scan picks the
  processor path, the model loads, and Overview does not show a false "Running on the
  processor" error for a GPU that is not there.
- **N4 Speed sanity.** Note tokens per second on Overview and whether it is usable for
  the model EZLM picked. Record the numbers; a slow pick is a tuning finding, not
  necessarily a bug.
- **N5 Installer on ARM64.** One installer installs the ARM64 payload, not the x64 one.
  Check the installed `EZLM.exe` platform after A1 on VM2.
- **N6 Update on ARM64.** K2 on VM2: the update downloads the ARM64 asset and the app
  still runs natively afterwards.
- **N7 Emulated helper (stretch).** If any helper (for example a GPU tool) is x64 on an
  ARM64 machine, note it and whether it still works under emulation.

## O. Awkward machines (one pass, if a spare VM is available)

Each of these has caused real Windows bugs in other apps. Do them on x64 unless noted.

- **O1 User name with a space**: `C:\Users\Pat Moorhead`. Full install, download and
  chat.
- **O2 User name with non-ASCII characters**: for example `C:\Users\José`. Full
  install, download, chat and Connect.
- **O3 OneDrive-redirected Desktop, Documents and Pictures.** Install with the desktop
  icon, then check the shortcut points at the redirected Desktop.
- **O4 A path near 260 characters.** Put the model folder deep in a long tree and
  deploy. Expected: no path-too-long error. If it fails, record the full path length.
- **O5 Non-English Windows.** Installer, first run, scan, and one chat on a German or
  Japanese display language. Check that numbers, dates and error text read correctly.
- **O6 No GPU, 8 GB RAM.** Full run with the smallest model. Note the pick, the load
  time and the speed.
- **O7 Defender real-time scanning on.** The default. Watch for a slow first engine
  start or a blocked `Engine\ezlm.exe`; record any Defender notification.
- **O8 Offline.** With no network, open EZLM with a model installed. Expected: it
  starts from disk, chat works, and only the update check shows a network message.
- **O9 Proxy or VPN.** With a system proxy set, start a download and an update check.
  Expected: either it uses the proxy or says clearly that it cannot reach the server.
- **O10 Port 8080 in use.** Covered by H7; also try a port below 1024 in `ezlm port`
  and confirm the message is readable.

## P. Display, input and accessibility

- **P1 Scaling.** 100%, 125% and 150% display scaling. Expected: no clipped text, no
  overlapping buttons, the left list still readable.
- **P2 Small screen.** 1366x768 at 125%. Expected: the main actions are reachable
  without horizontal scrolling, and the window can be resized to a usable minimum.
- **P3 Window controls.** Minimise, maximise, restore, and snap to half screen.
  Expected: layout reflows and nothing is cut off.
- **P4 Keyboard.** Tab through Overview, Chat and the dialogs. Expected: focus is
  visible, Enter activates the primary button, Escape closes dialogs, and Tab order is
  sensible.
- **P5 Screen reader.** With Narrator on, the nav items, the endpoint copy button and
  the dialog buttons have names. Note any "button" with no name.
- **P6 High contrast and light mode.** Switch Windows to light mode, then high
  contrast. Expected: text stays readable; no white-on-white or black-on-black.
- **P7 Long model name.** On a narrow window, a long model name truncates rather than
  pushing the badge off screen.
- **P8 Taskbar and tray.** The taskbar icon is EZLM's and the window title is EZLM.
  Closing the window exits `EZLM.exe` and its `ezlm.exe` engine; `llama-server.exe`
  may keep running, which is expected, and reopening EZLM shows the server still Live.

## R. Sign-off smoke for later builds

About 25 minutes, on VM1 and VM2, after each release candidate.

1. A1, A2, A8: install, SmartScreen, first window.
2. B1, B2, C1, C7: scan, choose, download (reuse a cached model if the VM allows),
   Live.
3. D1, D3, D5: URL, one chat completion, Test model.
4. E1, E3, E10: one chat, remember across a new chat, restart keeps it.
5. F1, F2: connect one app and get an answer.
6. H1, H2, H3: stop, start, sign-in start.
7. K1, K2: update check and update over the running app.
8. M1, M2: uninstall keeping models, reinstall, model still there.

If time allows, add N1 and N6 on ARM64.

## Appendix A: commands

Run in PowerShell. `ezlm` below means
`& "$env:LOCALAPPDATA\Programs\EZLM\Engine\ezlm.exe"`.

```powershell
# Version and status
ezlm version
ezlm status
ezlm url
ezlm port

# Apps and their launch preflight
ezlm doctor apps
ezlm apps

# Keys
ezlm key list
ezlm key new "laptop"
ezlm key rotate <id>
ezlm key revoke <id>

# Storage
ezlm storage
ezlm storage "D:\EZLM-models" --move

# Server
ezlm start
ezlm stop
ezlm test

# Updates
ezlm update
ezlm update --download

# Uninstall without the installer (stops the server, deletes models and settings)
ezlm uninstall
```

Developer self-test, no window and no engine:

```powershell
& "$env:LOCALAPPDATA\Programs\EZLM\EZLM.exe" --selftest "$env:TEMP\ezlm-selftest.txt"
Get-Content "$env:TEMP\ezlm-selftest.txt"
```

The self-test is a developer check, not a user check. Record it on each VM; a failure
there is a blocker even if the UI looks fine.

## Appendix B: where EZLM keeps things

| What | Path |
|---|---|
| App and engine | `%LOCALAPPDATA%\Programs\EZLM` |
| Models (default) | `%USERPROFILE%\.ezlm\models`, or the folder chosen in Settings |
| Settings, config, keys | `%USERPROFILE%\.ezlm` (`config.json`, `settings.json`) |
| Engine logs | `%USERPROFILE%\.ezlm\logs` |
| App log, chats, memory | `%LOCALAPPDATA%\EZLM` |
| Startup entry | `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, value `EZLM` |
| Uninstall entry | Settings > Apps > Installed apps, and
  `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall` |

BETA.md says "Chats and memory stay on your computer: `%LOCALAPPDATA%\EZLM` on
Windows". That is true for chats and memory only; keys and settings are in
`%USERPROFILE%\.ezlm`. Confirm both on each VM and correct the text before the beta.

## Appendix C: expected warnings (not bugs)

- SmartScreen "Windows protected your PC" on every unsigned build.
- Smart App Control blocks the installer outright on Windows 11 with SAC on.
- macOS-style "unidentified developer" wording does not appear on Windows; ignore.
- A slow first engine start on a machine with Defender real-time scanning.
- "Running on the processor" when the GPU driver cannot load the model; use "Try the
  graphics card again" and note the driver.

## Appendix D: failure record

```
Build:              (version, installer name, sha256 of the installer)
VM:                 (ID, Windows edition, build, arch, RAM, GPU, driver)
Account:            (admin or standard, user name, locale)
Check ID:
What happened:
Expected:
Screenshot:         (file name)
Logs:               (copied from %USERPROFILE%\.ezlm\logs and %LOCALAPPDATA%\EZLM)
Self-test:          (pass or the failing line)
Time (local):
```

File it as a GitHub issue with the "Report a problem" body when possible, and paste the
check ID at the top so it can be matched to this list.
