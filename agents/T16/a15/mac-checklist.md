# EZLM Mac beta: manual test checklist

For testers running the public Mac beta. Every item has a number you can quote in a
report (`A3.2`), a thing to do, and what should happen. If what you see differs, that
is a finding: note the number, what happened, and the exact text of any message.

Target matrix (the machines we have tested least are first):

| # | Machine | RAM | macOS | Priority |
|---|---|---|---|---|
| M1 | MacBook Air M1, 8 GB | 8 GB | 14 Sonoma | 1 |
| M2 | MacBook Air M1, 8 GB | 8 GB | 15 Sequoia | 2 |
| M3 | MacBook Pro M1 Pro / M2, 16 GB | 16 GB | 14 | 3 |
| M4 | MacBook Pro M2 Pro / M3, 32 GB | 32 GB | 15 | 4 |
| M5 | Mac Studio / MacBook Pro M3 Max, 64 to 128 GB | 64 to 128 GB | 15 | 5 |
| M6 | Any Apple Silicon, clean install, macOS 15 | 16 GB | 15 | 2 |

Notes for the whole run:

- Use a fresh macOS user account for at least one pass (M6). Most install bugs only
  appear once, on a machine that has never had EZLM.
- Keep a screen recording or screenshots of each failure. The app's own logs are at
  `~/.ezlm/logs/` and the launch agent's output is `~/.ezlm/logs/server.log`.
- Do not test on a Mac with less than 25 GB free. A model download can be 2 to 60 GB.
- The app needs no admin password. If a step asks for one, that is a finding.

## A. Install, first launch, Gatekeeper

**A1. Download and mount.** Download `EZLM.dmg`, double-click it. A Finder window
opens with EZLM and an Applications shortcut. The disk image mounts without an error.
*Fail if:* "no mountable file systems", or the image is empty.

**A2. Drag to Applications.** Drag EZLM into Applications. Copy completes in a few
seconds. Do not launch it from the disk image yet (see A9).

**A3. First launch, unsigned app.** Open EZLM from Applications.
*Expected:* macOS says it cannot verify the app ("Apple could not verify EZLM is free
of malware"). Click **Done**, open **System Settings > Privacy & Security**, scroll
down, click **Open Anyway**, confirm with password or Touch ID, then **Open**.
*Record:* the exact wording. macOS 14 and 15 differ here; both must be covered.
*Fail if:* no Open Anyway button appears, or the button does nothing.

**A4. Second launch.** Quit EZLM, open it again. It opens with no warning.

**A5. Quarantine after update.** After section H installs an update, open the app once
more. The Gatekeeper prompt may return because the new copy is quarantined again.
*Expected:* at most one more Open Anyway, never a crash loop.

**A6. Wrong architecture.** On an Intel Mac (not in the matrix, note it if you have
one), EZLM should still open and the scan should say it runs on the processor.
*Context:* the app is universal, the beta text says Apple Silicon only. A mismatch
here is a documentation finding, not a blocker.

**A7. First-run window.** EZLM opens one window, dark, at least 900 by 620 points,
with the scan running. No blank white window, no window behind other apps, no dock
icon missing.

**A8. Close and reopen.** Close the window with the red button.
*Expected:* the app quits (this is a single-window app; Cmd-W and the red button
both end it). Reopen: state is remembered, not rescanned from zero.
*Fail if:* the app keeps running invisibly, or two windows can be opened with Cmd-N
(Cmd-N is deliberately disabled).

**A9. Run from the DMG (translocation).** Eject nothing; instead open EZLM directly
from the mounted disk image.
*Expected:* the app works for a look around, but Settings shows "EZLM is running from
the download, so it won't be here after a restart. Drag it into Applications."
*Then:* press Set it up, let it scan, and quit before a long download if you like.
*Fail if:* the app writes into the read-only image, crashes, or forgets its setup on
relaunch from the image. After dragging to Applications, the note disappears.

**A10. Spaces and permissions.** Move EZLM.app to a folder with a space and an accent
in its name (for example `~/Desktop/Test Apps/ÉZLM.app`), open it, complete a scan.
*Expected:* works; the engine is found inside the bundle.

## B. Scan and model choice

**B1. Hardware lines.** After the scan, the wizard shows the chip, memory and free
disk. Compare with **Apple menu > About This Mac** and `sysctl -n hw.memsize`
(divide by 1073741824 for GB).
*Expected:* same chip name, memory within 1 GB, disk within a few GB.

**B2. Graphics line.** System > Graphics should name the Apple GPU and show usable
graphics memory. On 8 GB it should be about 5.4 GB, on 16 GB about 10.7 GB, on 32 GB
about 21.5 GB, on 64 GB about 48 GB (macOS reserves part of RAM for the GPU).
*Check:* `sysctl -n iogpu.wired_limit_mb`. `0` means macOS default and the numbers
above apply. A non-zero value means EZLM must use that many MB instead.
*Record:* the value, on macOS 14 and 15, on 8 GB and 64 GB.

**B3. Recommendation on 8 GB (M1/M2).** The wizard recommends a small model (a few
GB) and says how many words per second it expects. It must not recommend something
that cannot fit. Balanced / Smartest / Fastest change the pick, and the memory
estimate changes with it.
*Fail if:* the recommended model needs more than the free memory shown, or the
wizard offers nothing at all with no explanation.

**B4. Nothing fits (8 GB, busy machine).** Open Safari with many tabs, Xcode, Slack
and Photos, then scan. The wizard should say no model fits in the memory free right
now, offer **Free up memory**, and still allow a smaller model.
*Expected:* the message names the next action. No dead end.

**B5. Free up memory.** Press it. A list of apps with memory use appears. Close one
with **Quit**, then **Keep it** for another. Nothing closes without confirmation.
*Expected:* the first run asks for permission to control other apps (Automation).
Allow it; then quitting an app works. Deny it once and confirm the app explains that
it cannot close apps and still lets you continue.

**B6. Scan again.** Change what is open, press **Scan again**. The numbers update
without restarting the app.

**B7. Long model names.** On a 128 GB machine the recommended model name may be
long. It must not overlap the speed/quality columns or truncate to something
unreadable at the default window size and at 900 points wide.

## C. Set up (download and install)

**C1. Start setup.** Press **Set it up**. The download panel shows engine and model
steps with sizes, a progress bar and a cancel control.
*Expected:* progress moves within 30 seconds on a normal connection.

**C2. Cancel and resume.** Let it reach about 20 percent, press cancel, confirm.
Reopen setup and start again. It resumes rather than starting from zero (watch the
downloaded size and the network graph).

**C3. Quit during download.** Press Cmd-Q mid-download. A dialog says "Quit EZLM?"
and names what is in progress, with **Keep Going** and **Quit**. Keep Going returns
to the download; Quit exits cleanly. Reopen and resume.

**C4. Disk full.** Start a download on a volume with little space. When it fills, the
error says the disk filled and that it resumes after freeing space. Free space, press
setup again: it resumes. No partial file is left as a 0-byte "model" in Models.

**C5. Sleep during download.** Close the lid for two minutes mid-download, wake,
unlock. The download continues or resumes by itself within a minute. No "stuck at
0 bytes/s" forever state; the progress number does not go backwards.

**C6. Network drop.** Turn Wi-Fi off for 30 seconds, turn it on. The download resumes.
*Fail if:* it reports failure and throws away progress.

**C7. Slow connection.** On a slow link, the "time remaining" (if shown) should not
show absurd values or negative numbers.

**C8. Done screen.** When setup finishes, a screen shows "Your AI is live", the URL
(`http://127.0.0.1:8080/v1`, port may differ) and a key starting `ezlm-`. **Copy**
puts it on the clipboard; **Copy** changes to **Copied** briefly.
*Check:* `curl` the URL with the key (section D1).

## D. Server, URL and key

**D1. API answers.** In Terminal:
`curl -s http://127.0.0.1:8080/v1/models -H "Authorization: Bearer <key>"` returns
JSON with the model id shown in the app. Without the key it returns 401.
*If the port differs:* Keys shows the real port; use that.

**D2. OpenAI-compatible chat.** `curl` a short chat completion (or use any OpenAI
client) with the key. An answer comes back. This proves the URL works for other apps.

**D3. Wrong key.** A key with one character changed returns 401, and the app still
shows Live.

**D4. Stop and start.** Overview > **Stop server**. The state changes to Stopped, the
port stops answering within about 10 seconds, and the app does not spin.
Press **Start server**: it comes back, and the same key still works.

**D5. Stop survives login (M1/M2, macOS 14 and 15).** With the server Stopped, log
out and back in (or restart). **Do not open EZLM first.** Wait 60 seconds, then:
`lsof -nP -iTCP:8080 -sTCP:LISTEN` and `launchctl print gui/$(id -u)/com.ezlm.server`.
*Expected:* the server stays stopped until you press Start server.
*Known suspicion (verify):* on macOS the launch agent runs llama-server directly at
login (`RunAtLoad`), and nothing checks the `stopped` marker on that path, so the
server may come back at login. If it does, that is a major finding for a05: "Stop
server" is not remembered across login.

**D6. Server survives app quit.** With the server running, quit EZLM. The URL keeps
answering for other apps (the login agent owns it now). Reopen EZLM: it reconnects
and shows Live without redeploying.

**D7. Crash recovery.** Force quit llama-server in Activity Monitor (the process is
`llama-server`). The app notices, shows a restart path within about 30 seconds, and
the server comes back. Chat recovers (section F8).

**D8. Port taken.** With EZLM stopped, run `python3 -m http.server 8080`. Start EZLM.
It moves to the next free port and Keys shows the new URL. Stop the other server;
EZLM keeps working.

## E. Keys and LAN

**E1. New key.** Keys > **New key**, name it "Phone". A second key appears, marked
"used by connected apps" only for the default one.
**E2. Second key works.** `curl` with the new key returns 200. **Revoke** it, and the
same request returns 401 while the first key still works.
**E3. Rotate.** **Rotate** on the default key: a confirmation warns that apps using
the old value stop working. After rotating, EZLM updates the apps it connected. The
new key answers; the old one does not.
**E4. LAN.** Turn on "Let other devices on my network use it". The Keys pane shows a
`http://<lan-ip>:<port>/v1` URL. From another device on the same network, `curl` it
with the key. Then turn LAN off: the LAN URL stops answering, localhost still works.
**E5. Firewall.** With LAN on, macOS may ask whether to allow incoming connections.
Allow it; note whether the prompt appears once per launch (it should not keep asking).
**E6. No key shown in logs.** `grep ezlm- ~/.ezlm/logs/*` shows no full keys.

## F. Chat

**F1. First answer.** Chat > type "hello" > send. Tokens stream in (not one dump at
the end). The answer is formatted (bold, code blocks, lists) and can be selected and
copied.

**F2. Thinking model.** With a thinking model, the thinking appears, then folds
into "thought for Ns". Expand and collapse it. **Answer without thinking** appears
where offered and produces a shorter wait.

**F3. Memory.** Say "My name is <name>". New chat. Ask "What is my name?" The answer
knows it. `/remember I take my coffee black`, then `/memory` shows the note; edit it
and confirm the edit persists across relaunch. `/forget coffee` removes it.
*Check:* memory is stored in `~/Library/Application Support/EZLM` (not `~/.ezlm`).

**F4. History.** History lists the previous chat with a relative time. Reopen it: the
messages are there, including formatting. Delete one chat, then Delete all chats.

**F5. File attach.** **+ file** attaches a small text file; the answer can quote it.
Drag a file onto the chat also attaches. A large binary is refused with a reason, not
a hang.

**F6. Retry and Listen.** Retry on an answer replaces it (or adds a new one) without
duplicating the question. **Listen** speaks the answer; it stops on a second press.
*Note:* Listen needs a voice installed; a clear message is fine if none is.

**F7. Model switch.** Change the model at the top of Chat. A notice says the server
is restarting; the chat survives and the next answer comes from the new model.
Switching back does not download twice.

**F8. Sleep and wake (M1 and M3).** With Chat open and a long answer streaming,
close the lid for five minutes. Wake, unlock. Within about 30 seconds either the
answer completes or a clear "connection lost, Retry" appears. No infinite spinner, no
duplicated messages, no lost history.

**F9. Long chat.** Paste or send enough text to pass 20 messages, then scroll. The
window stays responsive (typing does not lag by seconds), scrolling is smooth, memory
in Activity Monitor does not grow without bound while idle.
*Record:* Activity Monitor memory for EZLM before and after 20 messages.

**F10. Long answer.** Ask for a 2000-word answer. It streams, the box grows and
scrolls to the newest text, and the app stays usable while streaming.

**F11. Copy and export.** Copy an answer, Copy this chat, and Copy all put sensible
text on the clipboard. Nothing crashes with an empty chat.

**F12. Right-to-left and accents (stretch).** Send Arabic or Hebrew text with English
code in the same message. Lines do not overlap, code stays left-to-right, and the
input box keeps the caret where you type. Send emoji and an accent-heavy name; no
tofu or clipping.

**F13. Window sizes.** Resize to the minimum (900 by 620) and to full screen in each
section. No controls are cut off, no text overlaps, the send button stays reachable.

**F14. Menu commands.** Cmd-C / Cmd-V / Cmd-A work in the message box. Cmd-Q asks
before quitting only when work is in progress. There is no Cmd-N new window.

## G. Models, storage, updates

**G1. Models list.** Models shows the current model marked Running, with size and
quantization. A second model can be downloaded while the first keeps answering.
**G2. Switch model.** Click a model, press Switch. Progress appears, then the model
is Running. URL, keys and connected apps are unchanged (verify with D1).
**G3. Remove a model.** Remove a non-running model: confirmation names the GB freed.
The folder disappears from disk.
**G4. Storage location.** Settings > Storage shows the models folder, free space and
how much EZLM uses. Pick another folder on another volume: it offers to move existing
models and does so, then downloads go there. Restart EZLM; the setting persists.
*Check:* `~/.ezlm/config.json` has `models_dir` set, and models are not duplicated
in both places.
**G5. Read-only target.** Choose a folder you cannot write to. A clear error appears;
the old location stays in use.
**G6. Update check.** Settings > Updates shows the current version and, when one
exists, the new version and download size. Compare with the DMG's
`CFBundleShortVersionString`.
**G7. Update install.** Install an update. EZLM downloads, verifies, replaces itself
and reopens. The model keeps running; chat history and keys survive; the version in
Settings is the new one.
**G8. Update from Applications only.** Run EZLM from the DMG, then update. It must
either refuse with a clear message or update the copy in Applications, never leave a
half-replaced app. Reopen after updating and confirm it launches.
**G9. Failed update leaves the old app.** Simulate by disconnecting the network at
90 percent. The old app still opens and still runs the model.

## H. Apps and problem reports

**H1. Connect OpenCode.** Connect > OpenCode > Connect. It installs if needed, adds
EZLM as a provider, and the check gets an answer. The first answer can take a minute;
a note says so.
**H2. Open OpenCode.** The button opens a terminal with OpenCode already using the
model. Ask it something; the answer comes from the local model.
**H3. Hermes.** Same two checks for Hermes.
**H4. DeepSeek Harness.** Same for DSH where offered.
**H5. Default model.** With "Make it the default model in those apps" on, a connected
app uses EZLM without extra setup. Turn it off and confirm the app still works with
its previous provider.
**H6. Connect before start.** With the server stopped, Connect explains "Start your
server first" and does not pretend to succeed.
**H7. Report a problem.** Press it, type a sentence, send. Either a GitHub issue
opens with the details filled in, or a browser page opens with them. **Copy details
for support** puts the same text on the clipboard.
*Check:* the text includes hardware and recent logs, and no `ezlm-` key.
**H8. Report on a failed setup.** The same form is reachable from the error state and
from Logs.

## I. System, memory, removal

**I1. System page.** It shows chip, memory, GPU, disk and what is using memory. The
numbers update live (watch for about 10 seconds with an app opening and closing).
**I2. 8 GB pressure.** With memory pressure high (open many apps), System shows the
pressure and suggests Free up memory. The suggestion is honest: it names what would
free the most.
**I3. Model mode from Overview.** Start Model mode, close one app, stop there. The
"Freed so far" number is plausible (compare Activity Monitor).
**I4. Model mode apply tune.** After freeing enough memory, it offers "Apply the
faster setup". Applying restarts the server with a bigger context or more GPU layers;
the URL and key stay the same.
**I5. Automation permission.** The first Model mode close asks for permission. Deny
it once: the app explains and does not loop. Grant it in System Settings > Privacy &
Security > Automation and retry; closing works.
**I6. Logs.** Logs shows live server output and scrolls. No key text. Copy all works.
**I7. Remove models and settings.** System > **Remove models and settings**: it warns
that connected apps stop working and that it cannot be undone. Confirm. The server
stops, the login item goes away (`launchctl print gui/$(id -u)/com.ezlm.server`
fails), `~/.ezlm` is gone, and the app returns to the first-run state.
*Check:* `~/Library/Application Support/EZLM` (chat history) and
`~/Library/LaunchAgents/com.ezlm.server.plist` are also gone, or the app says what it
kept. If chat history survives a "remove everything", that is a finding.
**I8. Drag to Trash.** Drag EZLM from Applications to the Trash. The app is gone and
the trash empties. No leftover `com.ezlm.server` job that keeps failing at login.
**I9. Reinstall after removal.** Install again, set up. A fresh scan, a fresh key, no
stale state from the previous install.

## J. macOS 14 versus 15, and other system states

**J1. Minimum version.** On macOS 14.0 (not 14.6), the app opens. On 13.x it should
refuse to open rather than misbehave; note what macOS says.
**J2. Gatekeeper wording.** Record the exact prompt on 14 and 15; both paths (Done >
Privacy & Security > Open Anyway) must work.
**J3. Launch agent across login.** On both versions, the model starts at login only
when the user has not stopped it (see D5).
**J4. Low Power Mode.** On battery with Low Power Mode, the model still answers;
speed is lower and the app does not warn incorrectly.
**J5. Multiple displays.** Move the window between a Retina and a non-Retina display;
text stays sharp, layout does not jump.
**J6. External keyboard and full keyboard access.** Tab reaches the main controls;
Return activates the primary button; Escape cancels a sheet.
**J7. Screen lock.** Lock the screen during a download and during a chat answer.
Unlock: both continue or offer Retry, and the app is not logged out.
**J8. Time zone and locale.** Switch the Mac to 24-hour time and another region.
Timestamps in History and Logs stay sensible; no layout break in German or French
(long words).
**J9. Second user account.** Install EZLM for one user, then log in as another. The
second user sees no EZLM setup (each user has their own `~/.ezlm`), and EZLM does not
crash on the first user's data.

## K. Evidence to send

For every failure, include:

1. The item number (`D5`), the matrix row (`M1`, macOS 14.6, 8 GB).
2. What you did, what you saw, and the exact text of the message.
3. A screenshot or short recording.
4. The output of: `sw_vers`, `sysctl -n hw.memsize`,
   `sysctl -n iogpu.wired_limit_mb`, `launchctl print gui/$(id -u)/com.ezlm.server`
   (first 40 lines), `lsof -nP -iTCP -sTCP:LISTEN | grep -i llama`.
5. The last 100 lines of `~/.ezlm/logs/server.log`, with keys removed.
6. Whether it reproduces after a restart, and on a second machine.

Severity guide: **blocker** = cannot install, cannot set up, cannot get an answer;
**major** = a main flow is wrong or data is lost, but there is a way around it;
**minor** = wording, layout, or an edge case.

## Results table

Copy one row per machine and fill it in.

| Item | M1 8 GB / 14 | M2 8 GB / 15 | M3 16 GB / 14 | M4 32 GB / 15 | M5 64-128 GB / 15 | M6 clean / 15 |
|---|---|---|---|---|---|---|
| A3 Gatekeeper |  |  |  |  |  |  |
| A9 from DMG |  |  |  |  |  |  |
| B3 8 GB pick |  |  |  |  |  |  |
| C2 cancel/resume |  |  |  |  |  |  |
| C4 disk full |  |  |  |  |  |  |
| D5 stop across login |  |  |  |  |  |  |
| E4 LAN |  |  |  |  |  |  |
| F3 memory |  |  |  |  |  |  |
| F8 sleep/wake |  |  |  |  |  |  |
| G7 update |  |  |  |  |  |  |
| H1 OpenCode |  |  |  |  |  |  |
| I7 remove everything |  |  |  |  |  |  |
