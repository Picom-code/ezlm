# Engine errors that reach the UI with no plain explanation

Task T12, agent a13 (stretch). Report only; no code changed.

Scope: errors raised in the engine layer (`internal/engine/`, and the
llama-server lifecycle in `internal/serve/client.go` and `service_*.go`) that
are shown to the user verbatim. Each entry gives the evidence that the string
reaches a screen, the problem, and a proposed wording in the README voice
(neutral, plain, one sentence naming the next action, no em dash, no emoji,
no exclamation mark).

How a string reaches the UI, in one line: a failed deploy or start step is
emitted as an event (`internal/app/app.go:594`, `:605`, `:664`, `:842`) and
both apps print `event.msg` unchanged:

- Windows: `windows/EZLM/Panes.cs:677` (`"fail" => (e.Msg ?? "Failed", ...)`) and `:319` (start error).
- Mac: `macos/Sources/EZLM/AppModel.swift:612` (`deployError = e.msg`) and `MainView.swift:270` (start error).

So every string below is the whole message the user gets. Nothing rewrites
it on the way to the screen.

## The pattern

The engine layer is good at explaining network and download failures
(`internal/engine/download.go:210` `plainNetErr`, `:237` `statusError`), and
the app has a second, separate plain-language mapper for start failures
(`internal/app/app.go:1552` `serverReason`). But:

1. The raw engine log line is used as the error itself in three places.
2. `engine.Install` errors have no mapper at all, unlike model downloads.
3. OS errors (`permission denied`, `no space left on device`) are returned
   unwrapped from the engine's own file operations.
4. The two mappers disagree: the same out-of-memory failure is explained on
   the Start path and shown raw on the Deploy path.

## 1. The raw llama.cpp log line is the entire error (worst)

`internal/serve/client.go:131` `fatalInLog` scans the tail of
`server.log` for `failed to load model`, `out of memory`, `failed to
allocate`, `address already in use`, and so on, and returns the matching
line. `WaitHealthy` then returns it as the error at `client.go:81`:

```go
if msg := fatalInLog(); msg != "" {
    return errors.New(msg)
}
```

`internal/app/app.go:817` calls `WaitHealthy`, and `:842` emits
`step("load", "fail", err.Error())`. The user sees, for example:

```
llama_model_load: error loading model: ... failed to allocate ...
```

or

```
ggml_vulkan: device memory allocation of size 1234567890 failed
```

Problems: engine jargon, an internal file/function prefix, a raw byte count,
no statement of what it means for the user, and no next action. It is also
the most likely failure on a beta machine (a model that does not fit).

Proposed wording, keeping the line for support but leading with the meaning:

> The engine stopped while loading the model (%s). Press Start server on Overview to try again, and open Logs for the full detail.

Better still, reuse `serverReason` (`app.go:1552`) on this path: it already
turns out-of-memory, missing model file and port-in-use into plain sentences.
Right now those three cases are only explained when the user presses Start,
not when Setup fails, which is the path a new user takes.

## 2. Log tails pasted after the message

`internal/serve/client.go:97`:

```go
return fmt.Errorf("%s. Last log lines:\n%s", ErrStopped, TailLog(8))
```

`internal/serve/client.go:105`:

```go
return fmt.Errorf("the model did not finish loading in %s. Last log lines:\n%s", timeout, TailLog(8))
```

`ErrStopped` is `the model server stopped while loading` (`client.go:122`).
Neither message names a next action. The second also prints the Go duration
`10m0s`, not "10 minutes". The log tail is useful for support, but it is
currently the only explanation of why, and it can carry ANSI colour codes:
`TailLog` does not strip them, while `serverReason` (`app.go:1553`) does.

Proposed wording:

> The model server stopped while loading. Press Start server on Overview to try again, and open Logs for the full detail.

> The model didn't finish loading in 10 minutes. Press Start server on Overview to try again, and open Logs for the full detail.

Keep the log tail after that sentence, but label it (`Details from the engine
log:`) and strip ANSI first.

## 3. `engine.Install` errors, shown raw

All of these reach the UI through `internal/app/app.go:662` to `:664`
(`step("engine", "fail", err.Error())`), which is also what
`internal/app/launch.go` shows as the setup failure.

| File:line | Current text | Problem | Proposed wording |
|---|---|---|---|
| `internal/engine/engine.go:61` | `could not reach GitHub for llama.cpp: %s` | Raw HTTP status; "GitHub for llama.cpp" is jargon; no next action | EZLM couldn't reach GitHub to download the engine (%s). Check the internet connection and press Set it up again. |
| `internal/engine/engine.go:95` | `llama.cpp release %s has no downloadable builds` | Names an internal release tag; no next action | The engine release EZLM needs (%s) has no files for any computer, which is a problem at llama.cpp's end. Press Set it up again in a few hours, or report this problem. |
| `internal/engine/engine.go:189` | `no llama.cpp build for %s/%s (%s) in release %s` | Raw `goos/arch` and backend tokens (`darwin/arm64`, `Metal`); no next action | The engine release %s has no build for this computer (%s, %s). Press Set it up again in a few hours, or report this problem. |
| `internal/engine/engine.go:255` | `unpacking %s: %w` | Wraps a raw archive error (`zip: not a valid zip file`); no next action | The engine download arrived damaged and couldn't be unpacked (%s: %s). Press Set it up again, and if it keeps happening, report this problem. |
| `internal/engine/engine.go:261` | `llama-server not found in %s` | File name only; no next action | The engine download didn't contain llama-server (%s), so EZLM can't start it. Press Set it up again, and if it keeps happening, report this problem. |
| `internal/engine/engine.go:335` | `checksum mismatch for %s` | "Checksum" is jargon; no next action | The engine download was damaged on the way here (checksum mismatch in %s). Press Set it up again: EZLM downloads it fresh. |
| `internal/engine/engine.go:344` | `unsafe path in archive: %s` | Reads like a crash; no next action | The engine download contains an unsafe file path (%s), so EZLM stopped unpacking it. Press Set it up again, and if it keeps happening, report this problem. |

## 4. Raw OS errors from the engine's own file work

These are `*os.PathError` values returned unwrapped, so the user sees a
filesystem path and a syscall message.

| File:line | What the user can see | Proposed wording |
|---|---|---|
| `internal/engine/engine.go:463` | `mkdir /Users/x/.ezlm/models: permission denied` | EZLM can't create its engine folder (%s). Allow EZLM in your security app or reinstall it, then press Set it up again. |
| `internal/engine/engine.go:264` | `rename /Users/x/.ezlm/bin/llama-....tmp /Users/x/.ezlm/bin/llama-...: permission denied` | EZLM couldn't move the engine into place (%s). Close any app that may be using EZLM's folder and press Set it up again. |
| `internal/engine/download.go:90` | `stat /Users/x/.ezlm/bin/foo.zip.part: no such file or directory` | EZLM lost track of a partial download (%s). Press Set it up again: it starts that file over. |
| `internal/engine/download.go:97` | `rename /Users/x/.ezlm/bin/foo.zip.part /Users/x/.ezlm/bin/foo.zip: no space left on device` | EZLM couldn't finish saving the download (%s). Free some disk space and press Set it up again. |
| `internal/engine/download.go:160` | `open /Users/x/.ezlm/bin/foo.zip.part: permission denied` | EZLM can't write its download to disk (%s). Check the disk has free space and that EZLM is allowed to write there, then press Set it up again. |

### Disk full during the engine install (major)

`internal/app/app.go:743` rewrites a full disk into a plain sentence, but
only for model downloads. The engine install at `:662` has no such
rewrite, and `engine.Install` returns `Download`'s raw `no space left on
device` at `engine.go:248`. On a small machine the very first setup step can
therefore fail with a syscall message. The model path already says the right
thing:

> the disk filled up during the download. Free about %.0f GB and press Set it up again: EZLM resumes where it stopped

Proposed for the engine path (`internal/engine/engine.go:248`):

> There isn't enough free disk space for the engine (about 1 GB). Free some space and press Set it up again.

### The unmapped network fallback

`internal/engine/download.go:233` is the default arm of `plainNetErr`: any
network error that does not match the cases above is returned raw (for
example a proxy demanding authentication, or a TLS handshake failure that is
not a certificate error). It reaches the UI at `app.go:746`. Proposed:

> The download failed (%s). Check the internet connection and press Set it up again: EZLM picks up where it stopped.

## 5. llama-server process and test errors, shown raw

| File:line | Current text | Problem | Proposed wording |
|---|---|---|---|
| `internal/serve/client.go:185` | `test request failed: %s` | Raw HTTP status; no next action | Your model didn't answer EZLM's test question (%s). Press Start server on Overview to try again. |
| `internal/serve/client.go:202` | `test request returned no answer` | No next action | Your model answered EZLM's test question with nothing. Press Start server on Overview to try again. |
| `internal/serve/client.go:208` | `the model answered with nothing` | No next action | Your model answered with nothing. Ask it something in the chat, or press Start server on Overview to try again. |
| `internal/serve/service_darwin.go:44` | `launchctl bootstrap: %s` | Raw command and its output; no next action | EZLM couldn't register the model server with the system (launchctl: %s). Restart your computer and press Start server; if it keeps happening, report this problem. |
| `internal/serve/service_linux.go:20` | `systemctl %s: %s` | Raw command and its output; no next action | EZLM couldn't register the model server with the system (systemctl %s: %s). Restart your computer and press Start server; if it keeps happening, report this problem. |
| `internal/serve/service_windows.go:183` | `couldn't open the server log: %v` | Raw `*PathError`; no next action | EZLM couldn't open its server log (%v). Allow EZLM in your security app, then press Start server. |
| `internal/serve/service_windows.go:200` | `couldn't start the server: %v` | Raw `exec` error, e.g. `Access is denied`; no next action | EZLM couldn't start the model server (%v). Press Start server on Overview to try again; if it keeps happening, use Copy details for support in Logs. |

These reach the user through `StartSync` to `Start` (`internal/app/app.go:1497`)
to `startError`, which both apps show without a mapper:
`windows/EZLM/Panes.cs:319` appends "Try Start server again..." to it, and
`macos/Sources/EZLM/MainView.swift:270` shows it alone. That Windows suffix
is also a symptom of this gap: the app adds the next action because the
engine error has none.

## 6. The app's own raw fallback (twin of the above)

`internal/app/app.go:1570` (frozen file, reported only) ends
`serverReason` with the last log line containing "error" or "failed":

```go
return ": " + t
```

so the message becomes `the model server couldn't start: <raw engine line>`.
This is the same failure mode as section 1, reached from the Start button
instead of Setup. Proposed: after the known cases, fall back to a plain
sentence and keep the line as detail:

> The model server stopped while starting. Press Start server on Overview to try again, and open Logs for the full detail.

## 7. Plain explanation that exists but is thrown away (minor)

`internal/serve/config.go:69` says `EZLM's settings file (%s) is damaged:
%v`, with the full path and the raw JSON error. It never reaches a screen:
every UI caller replaces it (`app.go:1389` `Start` and `:31` `Launch` turn
it into "nothing is set up yet" / "set up a model first"; `Status` at `:470`
returns an empty status). The user whose settings file is damaged sees an
unconfigured app and no reason, which looks like data loss. Proposed: surface
it on Overview with the next action, for example:

> EZLM's settings file is damaged, so your model setup looks empty (%s). Press Set it up again to recreate it, or report this problem with the file.

`internal/serve/props.go:38` (`/props answered %s`) is the same shape but
does not reach the UI today: `refreshProps` ignores the error. It needs no
wording until it is surfaced.

## Open questions

- Is `internal/serve/client.go` owned by T07 (the brief lists `client.go`
  under T07) even though `WaitHealthy` is lifecycle code that T10 might
  touch? Rows use T07 as the owner.
- `TailLog` output can contain ANSI escapes on Windows when the log is a
  console; no test covers this. Worth stripping in one place before display.
- The engine error strings are also printed by the CLI (`cmd/ezlm`), so any
  rewrite changes terminal output too. The proposed sentences read fine in a
  terminal, but a CLI-specific prefix may be wanted.
