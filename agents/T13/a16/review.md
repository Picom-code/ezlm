# Cross-check of the T13 beta guide

Reviewed by a16 on 2026-11-06, against branch `agent/T13-a16` (baseline `5213370` plus this
commit). This is a read-through of the sections that existed when it ran, not of the assembled
`BETA.md`.

## What existed when this review ran

`a01` to `a12` were present in the other agents' worktrees (about 15,000 words in total); `a13`,
`a14` and `a15` had not landed yet. Because a16 has its own worktree and does not merge, each
section was read from `~/ezlm-agents/work/subs/T13/<agent>/agents/T13/sections/<agent>.md` at its
committed state. The Lead must re-run the last three checks (section H) after a13, a14 and a15 land.

## How claims were checked

Every quoted message, path, label and minimum version below was compared with the code in this
worktree. The three claims marked "verified" were checked by reading the exact line, not by running
the app: this box is darwin/arm64, so the Windows-only paths are read, not run. The root `BETA.md`
is checked too, because a01, a02 and a08 cite it and because the guide replaces it.

## A. Contradictions to fix before assembly

| # | Severity | Where | What is wrong | Evidence |
|---|---|---|---|---|
| A1 | major | `BETA.md:7` says **EZLM-Setup.exe** | The shipped file is `EZLM-Setup-<version>.exe`, for example `EZLM-Setup-0.1.11.exe`. a01:19 already has this right and a09:396 quotes `EZLM-Setup-<version>.exe`, so `BETA.md` is the outlier. A tester told to look for a file that does not exist will pick the wrong download. | `windows/installer/EZLM.iss:15` (`OutputBaseFilename=EZLM-Setup-{#AppVer}`), `:5` (`AppVer "0.1.11"`) |
| A2 | major | `BETA.md:53` vs a08:33-36 | `BETA.md` says uninstall asks "whether to also delete the downloaded models". The real prompt also offers to delete **settings and keys**, and choosing Yes does delete them. a08 documents this; `BETA.md` does not, so a tester who reads only `BETA.md` loses keys without warning. | `windows/installer/EZLM.iss:94` ("Also delete the AI models EZLM downloaded, and its settings and keys?"), `internal/app/app.go:1586-1605` (removes `paths.Home()`), `macos/Sources/EZLM/MainView.swift:528` |
| A3 | major | `BETA.md:13` "Mac with Apple Silicon, M1 or later" | Intel Macs are supported: the app is built universal (arm64 and x86_64) and the engine ships as a universal binary. a02:10 says Intel works and is slower. Two documents, two answers. | `macos/scripts/build-app.sh:12,29,36` (lipo of arm64 + amd64), `macos/Package.swift:6` |
| A4 | major | `BETA.md:5` "Windows 10 or 11" | Windows 10 **version 2004 (build 19041)** or later, not any Windows 10. a01:3,9 has it right. An older Windows 10 tester gets a setup refusal, and the guide never explains it. | `windows/installer/EZLM.iss:30` (`MinVersion=10.0.19041`) |
| A5 | major | `BETA.md:26` only, and no section | The Mac starts the model at login too, from a launchd agent, and `BETA.md` never says so: it gives a Windows-only instruction (Task Manager > Startup apps) and a Windows-only button. No section tells a Mac tester how to stop EZLM starting at login. | `internal/serve/service_darwin.go:14-18` (plist path), `:44-70` (`RunAtLoad`, `KeepAlive`), `:108-114` (uninstall removes it) |
| A6 | major | `BETA.md:59` and a01:48-59 | The Smart App Control paragraph is an operating-system claim with no source: a01 cites `BETA.md` for it, and `BETA.md` cites nothing. If the wording of the Windows block differs, both documents are wrong together. Either cite Microsoft's documentation or hedge ("Windows may block it with no Run anyway button"). | a01 footnotes [2] and [3]; `BETA.md:59`; no code path can prove it |

## B. Consistency items across sections

1. **Two macOS floors.** a02:7 says macOS 14.0 is the app's minimum, a11 entry 18 says the engine
   needs 13.3. Both are true and they do not contradict, but a tester on macOS 13 gets the OS's
   "requires macOS 14" message, never the 13.3 check. Frame a11 entry 18 as the engine/CLI check, or
   drop it from the app's troubleshooting and keep it in the engine list.
   `macos/Package.swift:6`, `macos/scripts/build-app.sh:65` (14.0) vs `internal/prereq/prereq.go:112-115` (13.3).
2. **Hermes 64K has four wordings, not one.** a12:255 documents the tile text "Hermes needs a 64K
   context, which {model} can't have on this computer...". The connect error is different
   ("... Connect again to give the model the room (it may run slower), or pick a smaller model"),
   and two further tile variants give a speed estimate or say the room is free. A tester quoting any
   of them must be able to find it.
   `internal/integrate/hermes.go:47`, `windows/EZLM/Panes.cs:576`, `macos/Sources/EZLM/AppModel.swift:906,909,911`, `macos/Sources/EZLM/Models.swift:211`.
3. **"Model mode" versus "Free up memory".** The button on both platforms says **Free up memory**;
   the dialog it opens calls itself Model mode. a04:43 tells the tester to press "Model mode" on the
   System page, which does not exist as a button. Use "Free up memory (Model mode)" in every section,
   as `BETA.md:26` already does.
   `windows/EZLM/Panes.cs:217`, `macos/Sources/EZLM/MainView.swift:179`, `windows/EZLM/ModelMode.cs:64,175`.
4. **Reports keep the first 9 characters of a key.** `BETA.md:49` says API keys are "never included".
   The redactor shortens `ezlm-...` keys to their first nine characters on purpose, so a partial key
   does appear. Say "shortened, never the whole key" wherever the report contents are described
   (`BETA.md:49`, a12:394 area, the FAQ once a13 lands).
   `internal/integrate/log.go:31-34`.
5. **Model size range.** `BETA.md:23` and a02:9 both say "often 2 to 60 GB". The catalog's quants run
   from about 0.55 GB to about 76.9 GB. A tester with a 64 GB laptop is inside the real range and
   outside the documented one. Say "from under 1 GB to about 77 GB".
   `internal/catalog/catalog.json` (`quants[].bytes`, 209 entries).
6. **a11 line citations.** Entry 1 cites `internal/web/server.go:234` for the 403; the `http.Error`
   is on 235 (234 is the `if`). Every other a11 citation I spot-checked was correct (see F).
7. **a11 entry 1 "opening the address from another machine"** returns a connection failure, not
   `forbidden`, because the page binds `127.0.0.1:0`. The `forbidden` text is about the Host header.
   `internal/web/server.go:75,234-235`.
8. **a11 entry 4** says `streaming unsupported` means "the page is being served through something
   other than the normal built-in server". The code only fails a `http.Flusher` type assertion, and
   a11's own report calls it probably unreachable. Mark it as unverified rather than explaining it.
   `internal/web/server.go:275-278`.

## C. Windows/Mac drift

| Fact | Windows | Mac | Where it is documented |
|---|---|---|---|
| Starts the model at login | `HKCU\...\Run` value `EZLM` → `EZLM.exe --autostart` | launchd agent `com.ezlm.server.plist`, `RunAtLoad`, restarts on crash | Windows in `BETA.md:26`; Mac only inside a08's uninstall notes (a08:49). **Gap.** |
| How to stop it starting at login | Task Manager > Startup apps | delete or unload the launchd agent; no UI in EZLM | **Gap** on both sides. |
| Uninstall entry point | installer (Settings > Apps), plus in-app **System > Remove models and settings** | in-app **System > Remove models and settings**, then Trash | `BETA.md:53` gives only the installer for Windows and only the button for Mac. a08 covers both. |
| What "Yes"/confirm deletes | models + settings + keys, and model folders on other drives | models + settings + keys, plus `~/.ezlm` | a08 correct; `BETA.md` incomplete (A2). |
| Chat and memory data | `%LOCALAPPDATA%\EZLM` | `~/Library/Application Support/EZLM` | a07:66, a08:35,48 agree with code (`windows/EZLM/Engine.cs:29`, `macos/Sources/EZLM/ChatData.swift:3-15`). |
| Engine and settings data | `%USERPROFILE%\.ezlm` | `~/.ezlm` | a05:17-18, a08:34, a09:76-77 agree (`internal/paths/paths.go:9-36`). |
| Minimum OS | Windows 10 2004 | macOS 14.0 | a01 and a02 correct; `BETA.md` says neither. |

Evidence for the startup row: `internal/serve/service_windows.go:83-110` (Run key, and the comment
that Task Manager lists it), `internal/serve/service_darwin.go:44-70`.

## D. Duplicated advice: who owns it

Each of these appears in three or more sections. Give one section the full text and make the others
one line plus a cross-reference, or the assembled guide will repeat itself.

- **"Not signed yet, here is the warning"** — `BETA.md:59`, a01, a02, a08 (translocation note),
  a12 (`Drag EZLM into Applications...`). Owner: a01 for Windows, a02 for Mac; a12 should point at
  a02 instead of restating the fix.
- **Where models and keys live, and what uninstall leaves behind** — a04 (storage folder), a05
  (`config.json`), a08 (uninstall), and the FAQ once a13 lands. Owner: a08 for removal, a05 for the
  key file, a04 for the folder.
- **The key: where to copy it and where to paste it** — a05, a06, a11 entry 8. Owner: a05 for
  finding and copying, a06 for pasting; a11 should only say "the key is the `ezlm-` string".
- **64K context and Hermes** — a06 (setup), a12 (messages), a13 (FAQ). Owner: a06 explains it once,
  a12 lists the messages, a13 answers "why does Hermes need a bigger context" with a pointer.
- **Free up memory / Model mode** — `BETA.md:26`, a03, a04:43, a12. Owner: a04 or a03, one place.

## E. Claims with no independent citation

- **a01's Smart App Control section** (a01:48-59, footnotes [2], [3]): cites `BETA.md`, which cites
  nothing. See A6.
- **`BETA.md:59`** ("no Run anyway button"): same. This is the one claim in the beta doc that could
  make a tester give up on a machine where the app would have installed.
- **`BETA.md:44`** (gold "Running on the processor" note): real, and a12:154 documents it, but
  `BETA.md` gives no way to act on it. Fine for the beta doc, must be a cross-reference in the guide.
- Sections a04, a05, a08 and a11 cite well. a05 even records a `grep` showing no firewall code
  exists (`a05:144`), which is the standard the other sections should meet for negative claims.

## F. Per-section notes

- **a02** — accurate on the two warning wordings, on translocation and on the universal build. One
  addition: macOS 15's Control-click path is covered; also say what to do when the app is in
  Downloads but **not** on a disk image (the same translocation note covers it, so a cross-reference
  is enough).
- **a04** — sizes, memory reservation, cancel and resume all match the code. The one label problem is
  "Model mode" (B3).
- **a05** — strongest section on evidence. LAN is a saved config flag, the LAN URL is separate, and
  the page and chat stay on loopback; all confirmed (`internal/serve/config.go:33-34,92-108`).
- **a08** — accurate throughout, including "Remove everything" (`macos/Sources/EZLM/MainView.swift:534-537`)
  and the `%LOCALAPPDATA%\EZLM` left-behind claim (`internal/app/app.go:1586-1605` deletes only
  model folders and `paths.Home()`).
- **a11** — 26 entries, three citation problems (B6, B7, B8). The messages I checked against the
  code match word for word: `forbidden`, `this page is out of date: reopen EZLM`,
  `streaming unsupported`, every `internal/prereq` string, `EZLM <version>`.
- **a12** — the messages I sampled are real; the Hermes entry is one wording of four (B2).
- **a01, a03, a07, a09, a10** — spot-checked by grepping each quoted string and each path back to
  the code. No contradictions found beyond A1, A4, A6 and the duplicated advice in D. These five got
  a lighter read than a02, a04, a05, a08, a11 and a12, so treat them as "not contradicted" rather
  than "verified".

## G. Facts the assembled guide must state the same way everywhere

| Fact | Value | Proof |
|---|---|---|
| App version | `0.1.11-beta` | `internal/version/version.go:5` |
| Windows installer file | `EZLM-Setup-0.1.11.exe` | `windows/installer/EZLM.iss:5,15` |
| Windows minimum | 10.0.19041 (2004) | `windows/installer/EZLM.iss:30` |
| Windows install folder | `%LOCALAPPDATA%\Programs\EZLM` | `windows/installer/EZLM.iss:12` |
| Mac minimum | macOS 14.0, universal | `macos/Package.swift:6`, `macos/scripts/build-app.sh:65` |
| Engine and settings folder | `~/.ezlm` (`%USERPROFILE%\.ezlm`) | `internal/paths/paths.go:9-36` |
| Chat data folder | `%LOCALAPPDATA%\EZLM`, `~/Library/Application Support/EZLM` | `windows/EZLM/Engine.cs:29`, `macos/Sources/EZLM/ChatData.swift:3-15` |
| Base URL | `http://127.0.0.1:<port>/v1` | `internal/serve/config.go:100` |
| LAN URL | `http://<lan-ip>:<port>/v1`, empty when LAN is off | `internal/serve/config.go:103-108` |
| Key shape | `ezlm-` + 40 base62 characters | `internal/keys/keys.go:11-22` |
| Key file | `~/.ezlm/config.json` | `internal/paths/paths.go:36`, `internal/serve/config.go:27-34` |
| Hermes context floor | 64,000 | `internal/integrate/hermes.go:21,62` |
| Report destination | `picom-code/ezlm` issues, label `beta-report` | `internal/report/report.go:16,60-62` |

## H. For the Lead, after a13, a14 and a15 land

1. Run a15's script, then check the assembled `BETA.md` against A1 to A5; those five are the ones a
   tester acts on.
2. Grep the assembled files for `EZLM-Setup.exe`, `M1 or later`, `Task Manager > Startup apps` and
   `Model mode`; each should appear once, in the right place.
3. Check that every TROUBLESHOOTING entry's Message line can be found verbatim in the code with
   `grep -F`. a11 and a12 pass on the samples I tried; a09 and a10 were not sampled entry by entry.
4. Confirm the FAQ answers "where are the models" with `~/.ezlm` and "what does uninstall leave"
   with the three folders (models, `~/.ezlm`, `%LOCALAPPDATA%\EZLM`).

## I. Addendum: a13, a14 and a15 landed after the first pass

Added after the first commit. a13 (FAQ) and a14 (unclear messages) were checked the same way; a15's
`assemble.py` was read but not run, because it merges other agents' files and the Lead owns that.

1. **a13 contradicts itself on model size, three times.** a13:18 says "between about 0.5 GB and 68
   GB", a13:62 says "Plan for 2 to 60 GB", and a13:121-122 says "0.55 GB ... 66 GB for Qwen3.8 Flash
   Next". The largest quant in the catalog's visible set is 76.9 GB
   (`nemotron-3.5-super-vl-120b-a12b`, NVFP4), and Qwen3.8 Flash Next's largest is 75.8 GB
   (GSQ-RCO-IQ3_XXS), not 66 GB. Pick one sentence: "from about 0.5 GB to about 77 GB", and use it
   in a02, a13 and `BETA.md` (B5). The 66 GB figure also appears to be a stale catalog snapshot.
2. **a13:61 repeats the Apple Silicon only claim** ("a Mac with Apple Silicon (M1 or later)"), so A3
   now has two documents against a02's Intel statement. Fix all three at once.
3. **a13:88 repeats the uncited Smart App Control claim** (A6). Same fix: cite Microsoft or hedge.
4. **a13:49 "keys and files are never included"** repeats `BETA.md:49`. The Windows report dialog
   says the same thing in its own checkbox text ("Keys and your files are never included",
   `windows/EZLM/ReportLaunch.cs:174`), and the code then shortens keys to nine characters
   (`ReportLaunch.cs:163`, `internal/integrate/log.go:31-34`). So the guide should not contradict
   the dialog, but it should explain: "your keys appear shortened, never whole." Add it once, in the
   FAQ, and have a12 point at it.
5. **a13's LAN answer is correct** and matches a05: plain HTTP on the local network, same key, turn
   it off when done (`internal/serve/config.go:102-111`). No change.
6. **a14 overlaps my a11 notes.** a14 already lists `forbidden`, `streaming unsupported`,
   `context deadline exceeded` and the silent `awake` failure, which are the same four I flagged in
   B6 to B8. Keep a14's wording and fix the a11 entries to match, so T12 gets one list.
7. **a15's script** reads `sections/*.md` and checks the three parts of each TROUBLESHOOTING entry. It
   cannot catch the contradictions above, which are between files, so run section H by hand after
   assembly.
