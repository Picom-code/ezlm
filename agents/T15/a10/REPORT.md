# T15 / a10 — C# review: the Windows app's panes and screens

Read-only review of `windows/EZLM` (WinUI 3). Slice: the panes and screens — `Shell.cs`
(the window, event pump and poll loop), `Panes.cs` (Overview, Models, System, Connect, Keys,
Logs), `Chat.cs`, `SettingsPane.cs`, `Install.cs` (the install/switch screen), `ModelMode.cs`,
and the shared pieces those call (`Ui.cs`, `SignalWave.cs`, `Engine.cs`, `Taskbar.cs`,
`App.xaml.cs`). Angles: threading, UI updates from background threads, error handling.

There is no Windows machine here, so **every finding below is code-read, not reproduced**. Each
one names the exact path and the user-visible effect, and is written so it can be checked on a
VM in a minute. `windows/**` is frozen for T15, so nothing here is patched — the fixes are
proposals.

## What is already right (so the fixes stay small)

- Every event that reaches the window is marshalled: `engine.ListenAsync(e => Ui(() => OnEvent(e)))`
  (`Shell.cs:154`), `engine.Exited += () => Ui(EngineDied)` (`Shell.cs:89`), and every
  `ContinueWith` either wraps its body in `Ui(...)` or only touches fields.
- `Ui(Action)` (`Shell.cs:140`) catches and logs, so one bad pane cannot kill the pump.
- `ShowSection` wraps the whole pane build in try/catch and renders a "This page hit a problem"
  page with a way out (`Shell.cs:654-676`). Good.
- Job events set their "busy" flag *before* the POST that starts the job (`Install.cs:116`,
  `Panes.cs:649`, `Panes.cs:735`, `SettingsPane.cs:222`), so the first event cannot be dropped.
- The 15/30 Hz animations (`BlockBar`, `PourBar`, `Spinner`, `Logo`, `SignalWave`) are
  `DispatcherTimer`s created on the UI thread and stopped on `Unloaded`. No cross-thread ticks.

## Findings, ranked

| # | Severity | Where | One line |
|---|---|---|---|
| 1 | major | `SettingsPane.cs:78-82` | A failed storage read re-triggers `ShowSection()` forever: the Settings pane rebuilds and re-requests in a tight loop and never leaves the spinner. |
| 2 | major | `Panes.cs:214` | "Stop server" awaits `engine.Get("status")` with no try/catch inside an `async void` handler; on failure the pane is never refreshed and the error is silent. |
| 3 | minor | `Chat.cs:788-808` | The background memory look races `NewChat`: `memFrom` is written from a stale chat, so noticing silently stops in the new chat and a request is sent for a chat that no longer exists. |
| 4 | minor | `Chat.cs:470-483` + `Markdown.cs:23-38` | While an answer streams, the whole answer is re-parsed 15 times a second on the UI thread (quadratic in answer length). |
| 5 | minor | `Shell.cs:189-198` | `Shell.Log` appends from any thread with no lock; a concurrent append throws and the empty `catch` throws the crash entry away. |
| 6 | minor | `Panes.cs:655-681`, `Panes.cs:742` | Integrate and key/lan progress events rebuild the whole pane per event (flicker, lost scroll, and the "New key" text box is wiped while it is being typed in). |
| 7 | minor | `Chat.cs:987-1004`, `Chat.cs:1186` | `MenuFlyout.ShowAt(anchor)` after an `await` on an anchor that may have been rebuilt away; the flyout silently never opens. |
| 8 | minor | `Panes.cs:395-415` | `testing` has no timeout: if the `test` job's `done` event is missed, the button stays "Testing…" and disabled until the window is restarted. |
| 9 | minor | `Install.cs:169-176` | `CancelInstall` leaves `cancelling = true` when the cancel POST fails, so Cancel and Esc are dead for the rest of the job. |
| 10 | minor | `Chat.cs:686`, `Chat.cs:1112-1128` | `SaveChat` serialises the whole chat, including up to 2 MB of attached file text per message, synchronously on the UI thread at the end of every answer. |

---

### 1. major — Settings pane: failed storage read loops forever

`SettingsPane.cs:31-34`

```csharp
async Task RefreshStorage()
{
    try { storage = await engine.Get<StorageInfo>("storage"); } catch (Exception ex) { Log(ex); }
}
```

The catch leaves `storage` at its old value, which is `null` on first open. `SettingsPane.cs:78-82`:

```csharp
if (storage == null)
{
    b.Children.Add(Row(10, new Spinner(), Txt("Looking at your drives", 12, Dim)));
    _ = RefreshStorage().ContinueWith(_ => Ui(() => { if (section == "Settings") ShowSection(); }));
    return;
}
```

Nothing gates the retry on success. Each failed read rebuilds the pane and schedules the next
read, so while the Settings pane is open and `GET /api/storage` fails, the app spins: a full
pane rebuild plus an HTTP request per iteration, with a localhost "connection refused" failing in
about a millisecond. The pane never shows an error and never settles.

The other two panes that load on demand do gate it, which is what makes this look like an
oversight rather than a choice: `Panes.cs:437` and `Panes.cs:526` both re-render only
`if (... && scan != null)`.

When it bites: a drive that cannot be enumerated (a disconnected mapped network drive is the
classic one — `DriveInfo` blocks and then throws), or the engine restarting while Settings is
open.

Fix: re-render only when the read produced something, and give the failure a terminal state.

```csharp
if (storage == null)
{
    b.Children.Add(Row(10, new Spinner(), Txt("Looking at your drives", 12, Dim)));
    if (!storageAsked) { storageAsked = true; _ = RefreshStorage().ContinueWith(_ => Ui(() => { if (section == "Settings" && storage != null) ShowSection(); })); }
    return;
}
```

plus a `storageAsked` flag reset in `OnStorageEvent`, and an error line with a Retry button when
`storageAsked && storage == null`.

### 2. major — Overview "Stop server": a failed status refresh skips the whole update

`Panes.cs:214`

```csharp
st != 0 ? Btn("Stop server", Kind.Normal, async () => {
    try { await engine.Post("stop"); } catch (Exception ex) { note = ex.Message; }
    await Task.Delay(600);
    status = await engine.Get<Status>("status");   // not guarded
    live = null; ShowSection();
})
```

`engine.Get` throws on a non-2xx and on any socket error (`Engine.cs:124-134`). The button's
handler is `async void` (a lambda passed as `Action`, `Ui.cs:61`), so the exception leaves the
handler on the dispatcher: `live = null; ShowSection();` never run. The Overview keeps the old
`live`/`status` and shows "Live" for a server the user just stopped, until the 1-2 s poll notices
(`Shell.cs:707-712`). The exception itself is swallowed by `App.UnhandledException`
(`App.xaml.cs:12`, `e.Handled = true`), so the only trace is `app.log`.

The window for this is small but real: the engine restarts its server on `stop`, so a 5xx or a
dropped connection in that 600 ms window is exactly when it happens.

Fix: guard it like every other call in the file.

```csharp
try { status = await engine.Get<Status>("status"); } catch (Exception ex) { note = ex.Message; }
live = null; ShowSection();
```

### 3. minor — the quiet memory look races `NewChat` and writes a stale `memFrom`

`Chat.cs:784-818`. `NoticeFacts` captures the chat position, waits 8 s, then writes:

```csharp
int upto = chat.IndexOf(reply) + 1;          // line 788
...
await Task.Delay(8000, cts.Token);            // line 793, the chat can change here
...
memFrom = upto;                               // line 805, unconditional
if (reply.Remembered.Count > 0 && section == "Chat") RenderNotes(reply);  // line 808
```

`NewChat()` clears `chat` and resets `memFrom = 0` (`Chat.cs:560-572`) but does not cancel
`memCts`, so the pending look survives. After the delay it sends an extraction request for the
old conversation (`History(upto, ...)`, line 797) and then sets `memFrom` to the *old* chat's
length. In the new chat, `chat.Skip(memFrom)` (line 789) is empty until the new chat grows past
that index, so automatic memory noticing stops without saying so. `RenderNotes(reply)` also
writes to a `StackPanel` belonging to a message that is no longer on screen.

Fix: capture `var id = chatId;` before the delay and return if `chatId != id` (and skip the
`memFrom` write); do the same before `RenderNotes`.

### 4. minor — streaming re-parses the whole answer 15 times a second

`Chat.cs:470-484` (`ChatTick`, every other frame of a 30 Hz `DispatcherTimer`,
`SignalWave.cs:37`) grows `m.Shown` and calls `RefreshMsg(m, grew)`; `RefreshMsg`
(`Chat.cs:396-398`) calls `m.Md.Update(text, cursor)`, and `Markdown.Update`
(`Markdown.cs:23-38`) starts with `MdParse.Parse(text)` on the *whole* text every time. Only
changed blocks are rebuilt (good), but the parse is O(answer length) and runs 15x/s on the UI
thread, so a long answer costs quadratic parse work while it streams. On the low-end laptops
this app targets (the code is full of 1366x768 accommodations, `Chat.cs:116-122`), that is where
the typing lag in Chat will come from.

Fix: parse only when the revealed text changed by a block boundary, or cap the re-render rate
for long answers (e.g. every 4th tick once `m.Shown > 4000`), or append to the last block
incrementally.

### 5. minor — `Shell.Log` is not thread-safe, so the log can lose the entry that matters

`Shell.cs:189-198`:

```csharp
public static void Log(Exception ex)
{
    try
    {
        var path = System.IO.Path.Combine(Engine.DataDir, "app.log");
        if (File.Exists(path) && new FileInfo(path).Length > 2_000_000) File.Delete(path);
        File.AppendAllText(path, $"{DateTime.Now:O} {ex}\n\n");
    }
    catch { }
}
```

Callers are on different threads by design: `App.xaml.cs:12` (`UnhandledException`, UI thread),
`App.xaml.cs:15` (`TaskScheduler.UnobservedTaskException`, thread pool), `App.xaml.cs:17`
(`AppDomain.UnhandledException`, the crashing thread), `Shell.cs:140` (`Ui`'s catch, UI thread),
`Taskbar` (UI thread), `Engine`/`ChatStore`/`Memory` (UI thread). `File.AppendAllText` opens
with `FileShare.Read`, so two appends that overlap throw `IOException`, and the empty `catch`
discards it — possibly the crash record the "Copy details for support" flow is meant to send
(`Shell.cs:209-213`). The 2 MB delete has the same race.

Fix: `static readonly object logLock = new();` and wrap the body in `lock (logLock)`, or queue
entries and write from one thread. A `FileShare.ReadWrite` append would also do.

### 6. minor — integrate/key events rebuild the whole pane per event

`Panes.cs:680` (`OnIntegrateEvent`, on every step) and `Panes.cs:669` call `ShowSection()`,
which tears down and rebuilds every tile, re-runs `Marks.AppLogo` (a decode per tile) and resets
the scroll position — for each app, for each of its steps. `Shell.cs:14-18` explicitly says
"rebuilding whole screens on every event is what made earlier versions feel slow and flickery";
this path still does it. `OnRestartJobEvent` (`Panes.cs:742`) has the same shape, and because
`KeysPane` is rebuilt, the "New key" `TextBox` (`Panes.cs:715`) is emptied while a rotation is
running — a name typed during a rotate is lost.

Fix: update the affected tile's texts in place (as `StepView.Update` does for installs), or
debounce `ShowSection` for these job types.

### 7. minor — flyouts shown after an `await` on a possibly detached anchor

`Chat.cs:987` `await RefreshInstalled();` then `Chat.cs:1004` `menu.ShowAt(anchor);` (same shape
at `Chat.cs:1186`). `RefreshInstalled` is an HTTP round-trip; if the pane is rebuilt while it
runs — a poll sees the server state change (`Shell.cs:711`) or the user clicks a sidebar item —
`anchor` is no longer in the visual tree and `ShowAt` throws. The method is `async Task` called
as `_ = ShowModels(models)` (`Chat.cs:130`), so the exception is unobserved: the "Model ▾"
button just does nothing. `Chat.cs:1104` guards the same kind of hazard for `RetryLast`, so the
pattern is known here.

Fix: `if (!anchor.IsLoaded || anchor.XamlRoot == null) return;` before `ShowAt`, and wrap the
`ShowAt` in try/catch with `Log`.

### 8. minor — the model test has no timeout

`Panes.cs:395-400`: `testing = true` before the POST, and only the `done` event
(`Panes.cs:809`) clears it. If the event stream drops and the engine does not replay `done` for
this window (`OnEvent` only acts `if (testing)`, `Panes.cs:808`), the button is stuck on
"Testing…" and disabled (`Panes.cs:201-202`) and the spinner (`Panes.cs:411`) never stops.
Fix: record the start time and clear `testing` after, say, 5 minutes, with `note` saying so.

### 9. minor — a failed cancel leaves Cancel and Esc dead

`Install.cs:169-176`:

```csharp
cancelling = true;
if (overallStatus != null) overallStatus.Text = "Cancelling…";
RenderSteps();                                  // cancelBtn.IsEnabled = installRunning && !cancelling
try { await engine.Post("cancel"); } catch (Exception ex) { note = ex.Message; }
```

On failure `cancelling` stays true, so `cancelBtn` is disabled (`Install.cs:151`) and the
dialog's Esc handler (`Install.cs:186`) calls `CancelInstall`, which returns at its first line
(`if (!installRunning || cancelling) return;`). The user can no longer cancel until the job ends
on its own. Fix: `catch { cancelling = false; ...; RenderSteps(); }`.

### 10. minor — `SaveChat` writes the whole conversation on the UI thread

`Chat.cs:686` calls `SaveChat()` from `SendChat`'s `finally`; `Chat.cs:1112-1128` serialises
every message including `Files[].Text`, and each attachment can be up to 2 MB
(`Chat.cs:1037`). `ChatStore.Save` (`ChatStore.cs:48-58`) then writes it synchronously. The
JSON of a chat with two attachments is tens of megabytes built and written on the UI thread at
the moment the answer finishes — a visible hitch. Fix: write on a background thread (the
`.tmp` + `File.Move` pattern already makes it atomic) or store attachments once by hash.

## Not verified without a Windows machine

- All ten. In particular #1 needs a `GET /api/storage` failure to prove the loop, and #2 needs a
  failing `GET /api/status` in the 600 ms after `POST /api/stop`.
- Whether `MenuFlyout.ShowAt` throws or silently no-ops on a detached anchor in WinUI 3 (#7).
- Whether disposing the `SpeechSynthesizer` at the end of `Speak` (`Chat.cs:964-969`) truncates
  playback of the stream it just handed to `MediaPlayer`; the `using` scope ends right after
  `Play()`. Worth an on-hardware listen.
- Whether the 30 Hz `SignalWave` timer plus the 15 Hz chat render really is what makes Chat
  feel slow on a 4-core laptop (#4) — needs a profile.

## Open questions

- `App.UnhandledException` sets `e.Handled = true` for every UI exception (`App.xaml.cs:12`).
  That keeps a beta build alive after a UI bug, but it also means a half-updated screen is the
  user's only symptom and the failure is invisible unless they use "Copy details for support".
  Is that the intended trade for the beta, or should only known-recoverable exceptions be
  handled?
- `Toast` is `async void` (`Shell.cs:102`) and keeps running after `Close()`; it is harmless
  today because it only sets `Opacity`/`Visibility` on a detached border, but it is the one
  `async void` in the app that is not an event handler. Worth converting to a
  `CancellationTokenSource`-driven task for consistency?
- `ModelMode` (out of my slice, a11's dialogs) sets `dialogOpen = true` directly instead of
  going through `ShowDialog` (`ModelMode.cs:42`). It is correct today, but it means the
  "only one dialog" invariant has two owners.

## Next steps

1. Fix #1 and #2 first: both are small, both are in the error path, and #1 can pin a CPU on a
   beta tester's machine.
2. Then #3 (a stale index that silently disables a feature) and #9 (a dead Cancel button).
3. #4 and #10 need a profile before touching them; they are the only two that change how the UI
   behaves rather than what it says.
4. A VM check for each: kill `ezlm.exe` while Settings is open (#1); press Stop server with the
   engine paused in a debugger (#2); start a chat, send, then Ctrl+N within 8 s and watch
   `memory.md` (#3); cancel an install with the engine paused (#9).
