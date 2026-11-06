# Unclear messages

Messages a beta tester can see that are too vague to document in TROUBLESHOOTING: a writer cannot say
what the message means or what the tester should do next, because the message itself does not say.
Each entry gives the exact text, where it appears, why it cannot be documented, and a proposed
wording in the README voice. This list feeds T12 (string audit); T12 owns the edits, so nothing here
is a patch.

## How entries were chosen

A message is on the list when all three are true:

1. It reaches a surface a tester sees (app toast, error screen, server panel, self-test row, Settings).
   The evidence column names the display site, not just where the string is built.
2. It leaves out the cause, the next action, or the object it is about, so the TROUBLESHOOTING
   "what it means / what to do" columns would have to be invented.
3. It is the text as shown, not a wrapped or replaced string.

Four failure shapes recur: **no cause**, **no next action**, **jargon**, and **no referent** ("it",
"that key", "that argument"). Two messages that mean different things but read the same also count,
because a tester cannot tell them apart.

## Tier 1: nothing to act on

| # | Text as shown | Where | Tester sees it | Why it cannot be documented | Proposed wording | Owner |
|---|---|---|---|---|---|---|
| 1 | "Something went wrong" | `windows/EZLM/Shell.cs:172`, `macos/Sources/EZLM/Screens.swift:8` | Startup and scan failure screen, above the detail line | Same headline for boot failure, scan failure and a dead background engine. Testers quote this line, not the detail below it. Names no step. | Make the title a parameter: "EZLM couldn't start" for boot, "EZLM couldn't scan this computer" for a scan | T15 (C#), T16 (Swift, `Screens.swift` frozen) |
| 2 | "Cancelled." | `windows/EZLM/ReportLaunch.cs:89`, `macos/Sources/EZLM/ReportLaunch.swift:46` | App tile after cancelling an app launch | Does not say what was cancelled. The same word is used for an install ("Cancelled. What was already downloaded is kept...") and an update ("Update cancelled."). | "Opening OpenCode was cancelled. Press Open again when you're ready." | T15, T16 (both frozen) |
| 3 | "Couldn't open it." | `macos/Sources/EZLM/ReportLaunch.swift:46` | App tile when the engine sends no reason | "It" has no antecedent, there is no cause, and no next action. Windows has a different fallback for the same state. | "Couldn't open OpenCode. Press Open again; if it keeps happening, press Report a problem." | T16 (frozen) |
| 4 | "The update didn't download." | `windows/EZLM/SettingsPane.cs:319` | Settings, update line, when the engine sends no reason | No cause and no action. The line above it in the same slot can say "Update cancelled.", so the tester cannot tell a failure from a cancel. | "The update didn't download. Check your internet connection, then press Update again." | T15 (frozen) |
| 5 | "Couldn't change where models are saved." | `windows/EZLM/SettingsPane.cs:237` | Settings, storage, when the engine sends no reason | No cause and no reassurance that the old folder still works. A tester may assume models were lost. | "Couldn't change where models are saved. Your models are still in the old folder; press Change to try again." | T15 (frozen) |
| 6 | "couldn't reach GitHub to check for updates" | `internal/update/update.go:68` | Settings, update line (macOS shows it as "Couldn't reach GitHub to check for updates.") | Names GitHub, which means nothing to a non-technical tester, and gives no action. | "Couldn't reach the update server. Check your internet connection, then press Check again." | T08 |
| 7 | "couldn't read the release list" | `internal/update/update.go:90` | Same line as #6 | "Release list" is internal. No cause, no action, and it reads like #6 to a tester. | "The update server sent something EZLM didn't understand. Try again in a few minutes." | T08 |
| 8 | "checking for updates failed (%s)" | `internal/update/update.go:73` | Same line as #6 | Raw HTTP status inside the sentence, and no action. Third variant of the same screen. | "The update server is having trouble (503 Service Unavailable). Try again in a few minutes." | T08 |
| 9 | "the model server couldn't start" (no reason appended) | `internal/app/app.go:1469`, empty `serverReason()` at `app.go:1550` | Server panel, "The server couldn't start" plus this detail | When the log has no error line there is no cause at all. "Server" is jargon for the thing that runs the model. | "The model server stopped before it could answer. Press Start server again; if it keeps happening, open Logs and press Report a problem." | internal/app: no task owns it tonight (T07 and T09 read it) |
| 10 | "test request failed: 500 Internal Server Error" | `internal/serve/client.go:185` | Overview, Test model button (`windows/EZLM/Panes.cs:399` shows the raw error) | Names the transport, not the model. No action, and a status code is not a sentence. | "Your model answered with an error (500 Internal Server Error). Press Test again; if it keeps happening, press Report a problem." | T07 |
| 11 | "test request returned no answer" | `internal/serve/client.go:202` | Same as #10 | No cause, no action, and it is nearly identical to #12 though the cause is different. | "Your model's reply had no answer in it. Press Test again; if it happens again, pick a smaller model." | T07 |
| 12 | "the model answered with nothing" | `internal/serve/client.go:208` | Same as #10 | No action, and a tester cannot tell it from #11. | "Your model returned an empty answer. Press Test again, or ask it something in Chat." | T07 |
| 13 | "/props answered 500 Internal Server Error" | `internal/serve/props.go:38` | Model switch and context change | "/props" is an internal endpoint name. No cause, no action. | "EZLM couldn't read your model's settings (500 Internal Server Error). Press Start server to restart it." | T07 |
| 14 | "checksum mismatch for %s" | `internal/engine/engine.go:335` | First run, while installing the engine | "Checksum" is jargon, and it does not say EZLM already deleted the file or what to do. | "The engine download was damaged, so EZLM deleted it. Press Set it up again." | T06 |
| 15 | "could not reach GitHub for llama.cpp: %s" | `internal/engine/engine.go:61` | First run, engine download | Two internal names (GitHub, llama.cpp) and a status code. No action. | "EZLM couldn't download the AI engine (503 Service Unavailable). Check your internet connection, then press Set it up again." | T06 |
| 16 | "llama.cpp release %s has no downloadable builds"; "no llama.cpp build for %s/%s (%s) in release %s" | `internal/engine/engine.go:95`, `engine.go:189` | First run, engine download | Release tags and platform triples. A tester cannot act on either, and the two read the same. | "EZLM couldn't find the AI engine for this computer. Press Report a problem and say which Windows or macOS version you use." | T06 |
| 17 | "unpacking %s: %w" | `internal/engine/engine.go:255` | First run, engine install | Starts mid-sentence with no subject, then appends a raw Go error. No action. | "EZLM couldn't unpack the AI engine. Press Set it up again; details are in Logs." | T06 |
| 18 | "llama-server not found in %s" | `internal/engine/engine.go:261` | First run, engine install | Names the internal program and archive. No action. | "The AI engine download didn't include its server program. Press Set it up again." | T06 |
| 19 | "downloading %s failed: %s" | `internal/engine/download.go:245` | Model download | Status only, no action. It is the catch-all left after the 404 and 403 cases, which do explain themselves. | "The download server refused the request (400 Bad Request). Press Set it up again; if it happens again, pick another model." | T09 |
| 20 | Raw Go transport error as a self-test detail, for example `Post "http://127.0.0.1:8080/v1/chat/completions": dial tcp ... connection refused` | `internal/bench/bench.go:64`, then `Detail = err.Error()` at `bench.go:105`, `:144`, `:193`, `:215`, `:248` | Overview, Test model rows | The detail column is a Go net error. It names a URL and a socket, and no action. "server said 500 Internal Server Error" and "no answer" (`bench.go:70`, `bench.go:77`) are the other variants. | "EZLM couldn't reach your model server. Press Start server, then Test again." | internal/bench: no task owns it tonight |
| 21 | "dsh install: npm wrote no package-lock.json, so the download can't be checked: ..." | `internal/integrate/dsh.go:416`; siblings at `dsh.go:425`, `:429`, `:432` | Connect, DeepSeek Harness | A log prefix ("dsh install:"), npm and lock-file jargon, and no action. The siblings are equally opaque. | "EZLM couldn't check the DeepSeek Harness download. Press Connect to try again." | internal/integrate: no task owns it tonight |
| 22 | "the %s provider would have changed" | `internal/integrate/dsh_config.go:407` | Connect, DeepSeek Harness | Reads like an internal assertion. "Provider" is jargon, and there is no action. | "EZLM stopped before it changed your DeepSeek Harness settings. Press Connect to try again." | internal/integrate |
| 23 | "OpenCode has no %q mode." | `internal/integrate/opencode.go:127` | Connect or Open OpenCode, wrong mode | Internal mode id, trailing full stop on a fragment, no action. | "EZLM can't open OpenCode that way. Press Open OpenCode to use the terminal." | internal/integrate |
| 24 | "there is no program to test" | `internal/integrate/smoke.go:124` | Connect, app check | No subject (which app?) and no action. | "EZLM couldn't find OpenCode to check it. Press Connect to install it again." | internal/integrate |
| 25 | "no version in %q" | `internal/integrate/opencode_detect.go:169` | Connect, OpenCode detection | Quotes the program's output and gives no action. | "EZLM couldn't tell which OpenCode version is installed. Press Connect to let EZLM install its own copy." | internal/integrate |

## Tier 2: understandable only with context

These name something, but the referent is inside EZLM, so the TROUBLESHOOTING entry would have to
explain EZLM's own vocabulary. Lower priority than tier 1.

| # | Text as shown | Where | Why it cannot be documented | Proposed wording |
|---|---|---|---|---|
| 26 | "that key no longer exists" | `internal/app/app.go:1241`, `app.go:1257` | "That key" points at a row the tester may have already forgotten. No action, though the list refreshes. | "That key was already removed. The list is up to date now." |
| 27 | "nothing is running" | `internal/app/app.go:174` | Nothing is what? Cancel with no job is not an error a tester can understand. | "Nothing to cancel right now." |
| 28 | "it disappeared again right after downloading" | `internal/app/app.go:1538` | "It" is the model server program, named only in the surrounding sentence. | "The server program disappeared again right after EZLM downloaded it." |
| 29 | "EZLM can't start %s with that argument" | `internal/integrate/launch_script.go:107`, `:175` | "That argument" is never shown, so a report cannot be matched to a cause. | "EZLM can't start OpenCode with one of its launch arguments. Press Report a problem." |
| 30 | "EZLM can't start %s with the setting %q" | `internal/integrate/launch.go:189`, `launch_script.go:82`, `:152` | The setting name is an environment variable, and no action follows. | "EZLM can't start OpenCode with one of its settings. Press Report a problem." |
| 31 | "no model fits this machine right now" | `cmd/ezlm/main.go:415` | True but not actionable in the terminal. The app version of the same state says to close an app and scan again. | "No model fits in the memory that's free right now. Close an app, then run ezlm scan again." |
| 32 | "unknown key command %q" | `cmd/ezlm/main.go:549` | The quoted word is whatever the tester typed; the valid words are not listed. | "EZLM doesn't know that key command. Try ezlm key list, ezlm key new or ezlm key revoke." |

## Checked and clear (not on the list)

These look short but do carry a cause and a next action, so a09 to a12 can document them as they are:

- "the disk filled up during the download. Free about N GB and press Set it up again" (`internal/app/app.go:744`).
- "the downloaded file came out the wrong size ... so EZLM deleted it. Try again" (`internal/engine/download.go:94`).
- "EZLM's model server program is missing and couldn't be restored ... allow the folder ... then press Start server" (`internal/app/app.go:1541`).
- "the running model server didn't stop. It was probably started with administrator rights ..." (`internal/serve/config.go:196`).
- "moving your models needs N GB and X has M GB free" (`internal/app/settings.go:141`).
- "the download server's security certificate looks expired ... set the clock right" (`internal/engine/download.go:227`).

## Patterns worth one fix each

1. **Raw HTTP status in parentheses.** Nine sites append `resp.Status` to a sentence (`internal/serve/client.go:185`, `internal/serve/props.go:38`, `internal/engine/engine.go:61`, `internal/engine/download.go:245`, `internal/update/update.go:73`, and others). One helper that turns 401, 403, 404, 429 and 5xx into a plain sentence plus an action would fix all of them and keep the status in Logs.
2. **Raw Go error after a colon.** `... : %v` at `internal/integrate/dsh.go:416`, `internal/engine/engine.go:255`, `internal/app/settings.go:196`, `internal/integrate/node_install.go:57`. The tester reads a second, lower-level sentence with no action. Move the detail to Logs and end the visible line with an action.
3. **Missing referents.** "it", "that key", "that argument", "that setting" appear in six messages above. Naming the object is a one-word change in most cases.
4. **Windows and Mac drift.** For the same launch failure Windows says "Couldn't open EZLM." and macOS says "Couldn't open it." (`ReportLaunch.cs:92`, `ReportLaunch.swift:46`). macOS capitalises the Go error before showing it (`Settings.swift:295`), Windows does not. Pick one shape.
5. **Internal names in prose.** npm, package-lock.json, /props, llama-server, llama.cpp, checksum, release list, provider, YAML, port. Each needs either a plain replacement or a one-line glossary in BETA.md. T12 should decide once, not per message.

## Method and limits

Surveyed every Go error and message field under `internal/` and `cmd/ezlm`, every C# string in
`windows/EZLM`, and every Swift string in `macos/Sources/EZLM`, then traced each candidate to the
place a tester would see it. Line numbers are from branch `agent/T13-a14` (the `main` snapshot).
I did not build or run either app, so a string that is built but never displayed could still be on
this list; each entry's display site is cited so a reviewer can check it. The chat, memory and
installer strings were outside this slice.

Open question for the Lead: `internal/app`, `internal/bench` and `internal/integrate` are owned by no
T01 to T16 task tonight, so the tier 1 entries in those files (9 to 13, 20 to 25) have no obvious
owner for the rewrite. Route them with T12 or leave them for the morning.
