# T10/a01: launchd plist (macOS)

Slice: the launchd agent EZLM writes for the model server (`internal/serve/service_darwin.go`).
Tests run on any OS against the generated plist, not against launchd. Nothing was installed or loaded.

## What I reviewed

`Install`/`Start`/`Stop`/`Uninstall` in `internal/serve/service_darwin.go`, `label()`/`testSuffix()`
in `internal/serve/service_name.go`, the plist fields (`ProgramArguments`, `RunAtLoad`, `KeepAlive`,
`ThrottleInterval`, `StandardOutPath`/`StandardErrorPath`), and the argv the plist carries
(`Config.Args()` in `internal/serve/config.go`).

## Changes

- `internal/serve/service_plist.go` (new, no GOOS suffix so it builds and tests everywhere):
  - `plistContent(label, server, args, logPath)` is the plist builder, moved out of the darwin file
    so it can be tested on Linux and Windows. Output is byte-for-byte the old template.
  - `plistEscape` is XML escaping instead of `html.EscapeString`, and replaces XML-invalid control
    characters (finding 1).
  - `writePlistFile` and `ensureLog` create the log's folder and the log file, and return their
    errors (finding 2).
- `internal/serve/service_darwin.go`: `Install` and the `Start` fallback call the helpers; `html`
  import dropped.
- `internal/serve/launchd_plist_test.go` (new): parses the generated plist with `encoding/xml` and
  asserts on the decoded values.

## Tests (all pass on this Mac; the builder is OS-independent)

`go test -p 2 ./internal/serve/`

- `TestLaunchdPlistRoundTripsAwkwardArguments`: paths with spaces, accents, CJK, emoji, `& < > " '`,
  tab, newline, a trailing space, a backslash, `$` and `%` come back from an XML parser as exactly
  the same strings, one array element each. Proves no shell quoting is involved and no escaping loss.
- `TestLaunchdPlistStructure`: Label, `RunAtLoad` true, `KeepAlive.SuccessfulExit` false,
  `ThrottleInterval` 10, both log paths equal the requested log path (with a space in it).
- `TestLaunchdPlistControlCharacters`: an argument with ESC/NUL/BEL still yields a well-formed plist
  with no control characters.
- `TestWritePlistFilePreparesLogFolder`: creates `Library/LaunchAgents`, creates a missing
  `logs/` folder and the log file, and empties an old log.
- `TestWritePlistFileReportsUnwritableLog`: a file where the log folder should be is an error, not
  a silent success.

Cross-checked once by hand with `plutil -lint` on a generated plist with spaces and unicode:
`OK`, and `plutil -p` showed the arguments intact.

## Findings

| # | Finding | file:line | Severity | Status |
|---|---|---|---|---|
| 1 | `html.EscapeString` leaves XML-invalid control characters (ESC, NUL, ...) in the plist. launchd rejects the whole agent, so the server never starts at login and the only symptom is `launchctl bootstrap` failing. A model folder name can contain one on macOS. | `internal/serve/service_darwin.go:52` (old) | minor | **fixed**: `plistEscape` in `service_plist.go:57` replaces them with a space |
| 2 | The log's folder was never created and the `os.Truncate(LogPath(), 0)` error was dropped. launchd creates `StandardOutPath` but not its parent folder; when the folder is missing (fresh `EZLM_HOME`, user deleted `~/.ezlm/logs`, config from an older build) the job fails to spawn and `Install` returns success anyway. | `internal/serve/service_darwin.go:81-85` (old) | major | **fixed**: `writePlistFile`/`ensureLog` (`service_plist.go:86,97`) create it and return the error |
| 3 | On macOS a user's Stop is undone at the next login. `Stop` only bootouts; the plist stays in `~/Library/LaunchAgents`, and launchd loads every plist there at login with `RunAtLoad` true. The "stopped by the user" flag is only consulted by `ezlm start --login`, which only Windows runs (`windows/EZLM/Program.cs:35`). `App.Stop`'s comment (`internal/app/app.go:1379`) promises the opposite. | `internal/serve/service_darwin.go:57-61`, `internal/serve/service_plist.go:41` | major | **not fixed** (see below) |
| 4 | Quoting for spaces and unicode in `ProgramArguments` | `internal/serve/service_plist.go:17` | none | verified correct: it is an array, not a shell string |
| 5 | `KeepAlive {SuccessfulExit:false}` + `RunAtLoad` + `ThrottleInterval 10` | `internal/serve/service_plist.go:41-47` | none | verified correct: crash restarts, clean exit and `bootout` do not |
| 6 | `StandardOutPath` and `StandardErrorPath` are the same file | `internal/serve/service_plist.go:49-52` | minor | no change: both fds are opened append, so lines interleave but are not lost |
| 7 | The agent sets no `EnvironmentVariables` | `internal/serve/service_plist.go` | minor | no change: the server takes every setting from argv and its path is absolute, so launchd's minimal environment (HOME, PATH, TMPDIR) is enough. A future flag that reads an env var needs an `EnvironmentVariables` dict. `EZLM_TEST_GPU_FAIL` is read by the app before the argv is built, so it does not belong in the plist. |

Finding 3 is left for a03 (unload/reload and leftover jobs), because the fix changes install/uninstall
semantics and both agents would edit the same functions. Options, in order of preference:

1. In `Stop`, after `bootout`, rename the plist to `label()+".plist.off"`; `Install` rewrites it and
   `Uninstall` removes both names. The login start then really honours the stopped flag.
2. Make the plist run `ezlm start --login` instead of `llama-server` directly. More faithful to
   Windows, but it needs the app binary path in the plist and a second process.

## Open questions

- Should `Install` fail when the log folder cannot be created, or fall back to a temp log? Today it
  returns the error to the caller, which surfaces it in the UI.
- Is `ThrottleInterval 10` right for a crash loop caused by a taken port? a02/a06 own that.
- Finding 3 assumes launchd auto-loads `~/Library/LaunchAgents` at login, which is documented
  behaviour for user agents; not verified by loading one, per the brief.
