# T15 a11 — C# review: dialogs, tray/taskbar, self-test, report generation

Read-only review of `windows/EZLM`: `Taskbar.cs`, `SelfTest.cs`, `ReportLaunch.cs` (problem reports and
"Show details"), every `ContentDialog`/picker call site in `Shell.cs`, `Chat.cs`, `Install.cs`,
`ModelMode.cs`, `Panes.cs`, `SettingsPane.cs`, and the report payload the app sends to the engine.

There is no Windows machine here, so every finding is **code-read** (static evidence only). No file
under `windows/**` was edited. `ReportLaunch.cs`, `SelfTest.cs`, `Panes.cs`, `Models.cs` are frozen
per README.md, so the fixes below are proposals for whoever owns them.

## Findings, ranked

### 1. Major — "Report this problem" inside the install dialog does nothing (silent no-op)

`Install.cs:166` puts a **Report this problem** button in the deploy-error panel:

```csharp
Btn("Report this problem", Kind.Normal, () => _ = ReportProblem(msg))));
```

`ShowDeployError` is rendered inside the install `ContentDialog` (`InstallView()` contains
`errorPanel`, and `ShowInstallDialog` puts that view in the dialog, `Install.cs:180-190`). While the
dialog is showing, `dialogOpen` is `true` (`Install.cs:187`). `ReportProblem` starts with
`if (dialogOpen) return;` (`ReportLaunch.cs:169`), and `ShowDialog` itself also returns
`ContentDialogResult.None` when `dialogOpen` (`Shell.cs:511-517`).

So when a model switch/setup fails **from the Overview dialog** — exactly the moment the user wants to
report it — the button is enabled, does nothing, and gives no feedback. In wizard mode
(`screen != Screen.Main`) the same button works because no dialog is open.

Proposed fix: allow the report dialog to open over a non-modal parent by closing/replacing the
install dialog content, or by queueing `ReportProblem` (`if (dialogOpen) { queue; return; }`) and
running it from the install dialog's `Closed` handler. The minimal version: from the install dialog,
`CloseInstallDialog()` first, then `_ = ReportProblem(msg)`.

Note: `ShowAppLog` has the same early guard (`ReportLaunch.cs:126`) and the same silent-drop shape,
though its realistic entry points (Connect tile, app log) are not usually behind another dialog.

### 2. Minor — the taskbar's red error bar can stay forever when the window is already in front

`Install.cs:205` and `SettingsPane.cs:234` call `Taskbar.Failed(Hwnd)` on a failed job. The only
thing that clears it is the window activation handler:

```csharp
Activated += (_, e) => { if (e.WindowActivationState != WindowActivationState.Deactivated && !installRunning && !storageBusy) Taskbar.Clear(Hwnd); };
```
(`Shell.cs:93`)

If the window is already active when the job fails (the normal case for a failed switch or move), no
`Activated` event fires, so the button keeps a red progress bar until the next successful job or the
user switches away and back. Cosmetic but reads as "still broken".

Proposed fix: call `Taskbar.Clear(Hwnd)` from the in-app error/OK path once the user has seen the
failure (e.g. at the end of `ShowDeployError`, or a short timer after `Failed`).

### 3. Minor — the Logs pane never follows new lines; `logScroll` is dead

`Panes.cs:756` declares `ScrollViewer? logScroll;` but nothing ever assigns it. `LogsPane` builds
the viewer inline and throws the reference away (`Panes.cs:773`):

```csharp
b.Children.Add(new Border { ... Child = new ScrollViewer { Content = logView, ... VerticalScrollBarVisibility = ScrollBarVisibility.Disabled } });
```

`RenderLogs` then computes `atBottom` from `logScroll` (`Panes.cs:783`) and calls
`logScroll?.UpdateLayout()` / `ChangeView(...)` (`Panes.cs:791-792`), all of which are no-ops. The
pane refreshes from `Poll()` (`Shell.cs:715`) but never scrolls to the newest line, so the "updated
live" log sits still while the user has to scroll by hand.

Proposed fix: `logScroll = new ScrollViewer { ... }; b.Children.Add(new Border { Child = logScroll });`
and enable the vertical bar (or keep the outer scroll and call `ChangeView` on it).

### 4. Minor — the browser fallback for a report can fail and still claim success

`ReportLaunch.cs:200-204`:

```csharp
await Launcher.LaunchUriAsync(new Uri(r.Url));
Toast("Your report is filled in on GitHub: press Submit there to send it.", Gold);
```

The `bool` result of `LaunchUriAsync` is ignored. With no default browser (or a broken handler) the
report is not opened, yet the user is told it is filled in. Separately, if the engine returns
`Sent == false` with an empty `Url` (or a relative one), `new Uri("")` throws inside the `try`, and
the user sees the misleading `"Couldn't send the report: Invalid URI: ..."` (`ReportLaunch.cs:206`).

Proposed fix: check the return value and toast a "copy the report instead" path on `false`; guard
`Uri.TryCreate(r.Url, UriKind.Absolute, out var uri)` and fall back to `Copy(...)`.

### 5. Minor — an update download shows no taskbar progress while deploy and storage moves do

`OnUpdateEvent` (`SettingsPane.cs:285-293`) updates the in-pane bar only; `Taskbar.Progress` is
called for deploys (`Install.cs:150`) and storage moves (`SettingsPane.cs:244`). A multi-GB update is
the longest download in the app, and the one job where the taskbar button stays blank.

Proposed fix: mirror `Taskbar.Progress(Hwnd, (double)d / e.Total.Value)` in the update progress
branch, and `Taskbar.Failed`/`Clear` in its done branch.

### 6. Minor — the report dialog reads up to 2 MB of `app.log` on the UI thread

`ReportLaunch.cs:194-196` does `File.ReadAllText(app.log)` (the file is capped at 2 MB, `Shell.cs:193`)
and then keeps the last 6000 chars, on the UI thread right after the user presses **Send report**. On a
slow disk or a network-redirected `%LOCALAPPDATA%` this is a visible freeze.

Proposed fix: read the tail asynchronously (`await Task.Run(...)`) or read only the last 8 KB with a
`FileStream` seek.

### 7. Minor — `--selftest` crashes instead of returning 1 when it cannot write its results file

`SelfTest.cs:215` `File.WriteAllLines(outPath, lines);` is unguarded. If the caller passes a path in
a folder that does not exist (`EZLM.exe --selftest C:\out\results.txt`), or the file is read-only,
the exception escapes `Run` and `Main` (`Program.cs:24`), so the process exits with the CLR crash code
(`0xE0434352`), not the documented 0/1, and no results are written. The temp folder cleanup at
`SelfTest.cs:212` also swallows every failure, so a locked file leaves `ezlm-selftest-*` folders behind.

Proposed fix: create the parent directory, wrap the write in `try/catch`, and on failure print the
lines to stderr and return 1.

### 8. Minor — `PickFiles` null check is dead code

`Chat.cs:1016-1022`: `PickMultipleFilesAsync()` returns a non-null list (empty on cancel), so
`if (files != null) Attach(...)` always runs; `Attach` then loops over an empty list. Harmless, but
it hides the "cancel" case from a reader. Proposed fix: `if (files.Count > 0) Attach(...)`.

### 9. Nit — `Taskbar.Flash` flags

`Taskbar.cs:72` uses `Flags = 0x2 | 0xC` = `FLASHW_TRAY | FLASHW_TIMER | FLASHW_TIMERNOFG`
(`FLASHW_TIMERNOFG` already includes `FLASHW_TIMER`). Works; the comment only names two of the three
bits. Not a bug.

## What I checked and found sound

- `Taskbar.cs` COM plumbing: the `ITaskbarList3` vtable prefix (ITaskbarList 5 + MarkFullscreenWindow
  + SetProgressValue/State) matches the SDK order, the coclass GUID and `HrInit` are right,
  `SetProgressValue(hwnd, completed, total)` is called in the right order, and the progress flags
  (0, 2, 4) are correct. `FLASHWINFO` layout is correct on x64/ARM64. All call sites are marshalled
  to the UI thread via `Ui(...)` (`Shell.cs:154`), so the STA COM object is used from one apartment.
- Dialog reentrancy is otherwise guarded: `dialogOpen` is set synchronously before `ShowAsync`, and
  reset in `finally`; `OnWindowClosing` (`Shell.cs:528-575`) handles the "close while a dialog is up"
  case with an in-dialog question or `MessageBoxW`, and does not deadlock on the install dialog.
- Report redaction: the engine redacts the literal keys, `ezlm-…`, `sk-…` and Bearer tokens
  (`internal/app/launch.go:237`, `internal/integrate/log.go:39`), and the C# `HideKeys`
  (`ReportLaunch.cs:163`) shortens keys the same way. The `app_log` field name survives the
  `SnakeCaseLower` policy (`Engine.cs:34-39`), so the engine really receives it.
- Self-test isolation: `SelfTest.Run` sets `EZLM_CHAT_DIR` before the first `ChatStore.Base` read and
  `Memory` resolves its paths through `ChatStore.Base` (`Memory.cs:20-21`, `ChatStore.cs:44`), so
  facts, prefs and chats stay in `%TEMP%` and are deleted afterwards. `--selftest` runs before the
  single-instance mutex and before any window/engine start (`Program.cs:20-25`).
- Self-test logic itself: the markdown block kinds, token guesses, trim/continue simulation, target
  decoding (new and old engine shapes) and the `AppText` wording checks match the current sources.

## Could not verify without Windows

- Whether `Launcher.LaunchUriAsync` succeeds on a machine with no default browser, and the practical
  URL-length limit for the pre-filled GitHub link (engine caps the encoded body at ~6000 chars,
  `internal/report/report.go:86`).
- Whether the taskbar COM object is available in every shell (kiosk / Server Core); the code already
  falls back to `broken = true` on failure.
- DPI/multi-monitor behaviour of the taskbar flash (not testable here).

## Open questions

- Is the report button in the install dialog meant to close the dialog first? If yes, the fix in
  finding 1 is a two-line change in `ShowDeployError`.
- Should a failed update also show the red taskbar bar, for consistency with deploy/storage?
- The self-test is not called from any script in this repo (only `Program.cs`), so nothing catches a
  regression in it automatically. Is it wired into the release checklist?
