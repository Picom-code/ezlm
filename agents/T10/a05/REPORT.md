# T10 a05: Windows scheduler edge cases

Slice: unicode and spaces in the user profile, OneDrive-redirected paths, and a
per-user login entry for a user who is not logged in.

Worktree `~/ezlm-agents/work/subs/T10/a05`, branch `agent/T10-a05`.
Windows behaviour is code-read and cross-compiled; the tests run on any OS.

## What this snapshot actually generates

The brief (and `TASKS.md` T10 step 1) says "Windows Task Scheduler task" and
"tests on the generated XML". **There is no task XML any more in this
snapshot.** `internal/serve/service_windows.go` installs a per-user
`HKCU\Software\Microsoft\Windows\CurrentVersion\Run` value named `EZLM`
(`runKeyPath`/`runValue`, line 81). The Task Scheduler names survive only as
legacy cleanup: `taskXMLPath()` (line 55) deletes `~/.ezlm/ezlm-task.xml`, and
`legacyTasks()`/`schtasks /Delete|/Change|/End` (lines 24-29, 70-74, 122-124,
140-142, 208-210) disable or remove the old "EZLM Server" tasks. So the
generated artifact to test is the Run value (a command line), not XML. a04's
"tests on the generated XML" needs the same adjustment.

## Tests added

`internal/serve/service_windows_edge_test.go` (one file, named after the
slice). It tests the pure builders in `service_windows_cmd.go`, which the
Windows build compiles unchanged.

- `TestLoginCommandUnicodeProfile`: `C:\Users\José Müller 王\...` splits back
  into exactly one executable argument plus the switch.
- `TestLoginCommandRedirectedProfile`: OneDrive-style
  (`OneDrive - Contoso, Ltd`), UNC (`\\fileserver\home\Sam\...`), `&`, `%`,
  and plain-space profiles all round-trip through a
  `CommandLineToArgvW`-compatible parser.
- `TestLoginCommandPassesLoginSwitch`: both the app and the CLI fallback carry
  the login switch (regression for F1).
- `TestLoginCommandAppNextToEngine`: the sibling `EZLM.exe` is only used when
  it exists; otherwise the engine starts itself.
- `TestLegacyTaskNames`: unicode/parenthesised account names stay one argv
  element and are not hand-quoted.
- `TestWinQuoteArgRoundTrip`: the quoting helper round-trips empty strings,
  quotes, tabs and trailing backslashes.

`go test -p 2 ./...`, `gofmt -l .` (empty), `go build`, `go vet`, and the
windows/amd64, windows/arm64 and linux cross builds all pass.

## Findings

| # | Severity | Status | Finding |
|---|---|---|---|
| F1 | major | fixed | CLI-only login entry used bare `ezlm start`, so a server the user stopped came back at the next login |
| F2 | major | open, code-read | a user who is not logged in gets no server: the login entry is interactive-logon only, and the task XML is gone |
| F3 | major | fixed | `EnsureAutostart` accepted any non-empty Run value, so a moved/redirected install kept a dead path |
| F4 | minor | open, code-read | `NoLoginTask` is set but never read, so a failed login entry is silent |
| F5 | minor | open, code-read | legacy task names freeze `%USERNAME%` at process start; an account rename leaves an orphan task |
| F6 | minor | open, code-read | Run value data over 260 characters is not detected or handled |
| F7 | minor | open, code-read | a UNC/roaming profile puts `~/.ezlm` and the server's working directory on SMB |
| F8 | minor | open, code-read | OneDrive does not move the profile root, so only a user-set `EZLM_HOME`/models dir lands in OneDrive |

### F1 (major, fixed) CLI fallback ignores the user's stop

Evidence: baseline `internal/serve/service_windows.go` built
`syscall.EscapeArg(self) + " start"` for a CLI-only install. `ezlm start`
checks the stop flag only with `--login` (`cmd/ezlm/main.go:163-166`), and
`App.Stop` writes that flag (`internal/app/app.go:1380`). So on a CLI-only
install, Stop then sign-out then sign-in started the server again, while the
desktop-app path (`EZLM.exe --autostart` -> `ezlm start --login`,
`windows/EZLM/Program.cs:30-40`) stayed stopped.

Fix: `internal/serve/service_windows_cmd.go:27` now emits
`<engine> start --login`. Test `TestLoginCommandPassesLoginSwitch` fails on
the old string and passes now.

### F2 (major, open, code-read) no server for a logged-out user

The HKCU Run value runs only at an interactive logon. `Start()`
(`internal/serve/service_windows.go:168`) starts the server for the current
session only. A user who is not logged in (headless box, fast user switching,
RDP disconnected, reboot before first sign-in) has no EZLM server until they
sign in. The removed Task Scheduler design could cover that only with stored
credentials (`/RU` + `/RP`, or S4U, which runs in session 0 with no desktop and
no network). The stale comment at `Start()` claimed Task Scheduler did it; the
comment is corrected. This is a product decision: if "survives reboots" means
"before anyone signs in", a per-user task is not enough and EZLM needs a
service or a credential-backed task. No Windows machine was available, so this
is code-read.

### F3 (major, fixed) stale login entry after a move or redirect

Baseline `EnsureAutostart` returned as soon as the value was non-empty, so a
Run value left over from an update, a profile rename, or a OneDrive/domain
move of `%LOCALAPPDATA%` kept pointing at a deleted `EZLM.exe` and the server
never returned at login. It now compares the stored value with
`expectedLoginCommand()` (`internal/serve/service_windows.go:114-124`) and
rewrites it on a mismatch. `expectedLoginCommand` and the registry read are
Windows-only; the command it compares is covered by the tests above.

### F4 (minor, open, code-read) failed login entry is silent

`NoLoginTask = setRunKey() != nil` (`internal/serve/service_windows.go:42,77`)
is never read anywhere in the tree. If the Run write is denied (policy,
locked-down HKCU, 260-character value), the user is not told that the server
will not come back by itself.

### F5 (minor, open, code-read) renamed accounts leave orphan tasks

`taskNames` is built once at package init from `os.Getenv("USERNAME")`
(`internal/serve/service_windows.go:18`), and cleanup only runs when
`EZLM_HOME` is empty (`legacyTasks`, lines 24-29). After an account rename the
old `EZLM Server (old-name)` task is never removed. Unicode and spaces in the
name are safe: `exec.Command` passes argv as UTF-16, and the tests assert the
name is one unquoted element.

### F6 (minor, open, code-read) Run value length

Windows documents a 260-character limit for Run/RunOnce value data. A long
account name plus a deep OneDrive-redirected `%LOCALAPPDATA%` can exceed it;
`setRunKey` only checks the registry write and does not check
`len(cmd) > 260`. Suggested fix: fall back to a short (8.3) path or a launcher
in a short folder, and set `NoLoginTask` when even that is too long.

### F7 (minor, open, code-read) UNC profile

On a roaming/redirected profile `%USERPROFILE%` can be `\\server\home\...`.
`paths.Home()` then puts config, logs, pid/birth files and models on SMB, and
`startLogged` sets `cmd.Dir` to a UNC path (`internal/serve/service_windows.go`,
`startLogged`). File locking, rename and oplock semantics differ; not tested
here.

### F8 (minor, open, code-read) OneDrive is not the profile root

OneDrive Known Folder Move redirects Desktop/Documents/Pictures, not
`%USERPROFILE%`, so `~/.ezlm` normally stays at `C:\Users\X\.ezlm`. The
OneDrive risk is a user-set `EZLM_HOME` or a models folder (`paths.Models`)
inside OneDrive, where files can be online-only placeholders and hydrate on
first read. `startLogged` opens the log before starting, which hydrates the
log; model files hydrate later and can make the first start slow.

## What changed

- `internal/serve/service_windows_cmd.go` (new): `legacyTaskNames`,
  `loginCommand`, `winDir`/`winJoin`, `winQuoteArg`. Pure, no build tag, so
  the Windows strings are tested on any OS.
- `internal/serve/service_windows.go`: use `legacyTaskNames`; build the Run
  value with `loginCommand`; add `expectedLoginCommand`; repair a stale value
  in `EnsureAutostart`; pass `--login` in the CLI fallback; correct the
  `Start` comment.
- `internal/serve/service_windows_edge_test.go` (new): the six tests above.

## Open questions

- Does EZLM promise to serve before the first interactive logon? If yes, F2 is
  a design gap, not a bug in the Run key.
- Is the 260-character Run limit still enforced on Windows 11? Worth one
  manual check on a real box.
- Should `NoLoginTask` be surfaced in the UI or app status?

## Next steps

- Run the two command-string tests on a real Windows box (they are OS-neutral,
  but the registry path is not).
- Add a registry abstraction so `EnsureAutostart`'s repair path can be tested
  with a fake key.
- Decide F2 and F6; both are small if the answer is "yes, handle it".
