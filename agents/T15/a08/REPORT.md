# T15 a08 — Defender, SmartScreen and Smart App Control triggers

Scope: unsigned child processes, scripts in temp folders, downloaded executables and zone
identifiers, across `*_windows.go` and the C# app (read only). No Windows machine here, so
everything except the one fix below is code-read; line numbers are from this worktree.

## Findings, ranked

### 1. BLOCKER — nothing EZLM ships is Authenticode-signed, so Smart App Control blocks it

`windows/installer/EZLM.iss` has no `SignTool=` directive and no signing step anywhere in the
build (`EZLM.iss:1-24`), and `windows/EZLM/app.manifest` carries no publisher. The same holds for
the Go binaries (`cmd/ezlm`) and for `llama-server.exe`, which EZLM downloads from the llama.cpp
releases and then runs (`internal/engine/engine.go:192-260`).

Smart App Control is on by default on new Windows 11 consumer installs and blocks every unsigned
or low-reputation executable, including installers it did not see a user approve. On such a
machine EZLM's own installer, `ezlm.exe`, `llama-server.exe`, `node.exe` (when EZLM installs its
own copy) and every update setup are blocked; the user gets "This app can't run because it's not
signed" or nothing at all. This is the single biggest beta risk in this slice.

Fix: sign `EZLM-Setup-*.exe`, `EZLM.exe`, `ezlm.exe` and (if possible) the llama.cpp binary with
an OV/EV certificate; add `SignTool=`/`SignedUninstaller=yes` to the `.iss` and a signing step to
the release build. Code-read.

### 2. MAJOR — Hermes installs by piping a remote script into `powershell -ExecutionPolicy Bypass`

`internal/integrate/hermes.go:36-39`:

```go
"& ([scriptblock]::Create((Invoke-RestMethod https://hermes-agent.nousresearch.com/install.ps1))) -NonInteractive; "
```

run by `internal/integrate/installer.go:27` as
`powershell -NoProfile -ExecutionPolicy Bypass -Command <script>`. Download-and-execute in
memory, with the execution policy explicitly bypassed, is the textbook shape of Defender's ASR
rule "Block execution of potentially obfuscated scripts" (and the audit-mode rule that many
managed machines enforce), and it defeats the usual "the script is on disk, let me look at it"
story. The script then starts unsigned `uv`/Python/Hermes shims under `%USERPROFILE%\.hermes`,
which Smart App Control blocks. The user sees only
"the installer stopped with this message: <last line>" (`installer.go:69`).

Fix: fetch `install.ps1` with Go to `%LOCALAPPDATA%\EZLM\hermes-install.ps1` (or `paths.Home()`),
check a pinned SHA-256, run `powershell -NoProfile -ExecutionPolicy Bypass -File <path>` so the
file is visible in the log and to the user, and keep the file on failure. When the exit code is
`ERROR_ACCESS_DISABLED_BY_POLICY` (0x800704EC) or `0x800704E8`, say "Windows security software
blocked Hermes's installer" instead of echoing a Python traceback line. Code-read.

### 3. MAJOR — the updater runs an unsigned setup from `%TEMP%` and closes EZLM 1.2 s later

`internal/update/update.go:112` put the installer in `%TEMP%\EZLM-update`; the window starts it
with `Process.Start(... UseShellExecute = true)` and then closes itself
(`windows/EZLM/SettingsPane.cs:295-308`). `Process.Start` does not throw when SmartScreen or
Smart App Control blocks the target: the prompt appears (or the process is denied) and EZLM still
shows "Installing the update" and closes. The user is left with no window and no update, and the
next launch runs the old version. A temp folder is also the wrong place to keep a 120 MB file that
Disk Cleanup or Storage Sense may delete while the window is closing.

Fix (partly done here): the installer now waits in `EZLM_HOME\update` (`DownloadDir`,
`internal/update/update.go`). On the C# side, keep the `Process` returned by `Process.Start`, wait
up to a few seconds and only close EZLM if it is still running; otherwise show the installer's
path in a sentence the user can act on. Code-read for the C# part.

### 4. MAJOR — no publisher check, and no zone identifier either way

`Download` verifies only the SHA-256 that came in the same GitHub JSON response
(`internal/update/update.go:117-123`, `internal/engine/engine.go:318-338`). Nothing checks the
Authenticode signature of the setup before running it, so a compromised or substituted asset
signed by anyone is accepted. Conversely, Go's `net/http` writes no `Zone.Identifier`, so
SmartScreen's app-reputation check never sees the file: the unsigned setup runs silently, and if
the user later opens it from Explorer it looks locally built.

Fix: after the digest check, verify the signature (`WinVerifyTrust` via `golang.org/x/sys/windows`,
or `signtool verify /pa`) and name the publisher in the log; decide deliberately whether the
update file should carry a zone identifier. Code-read.

### 5. MAJOR (engine, report only) — downloaded executables run from the profile

`llama-server.exe` (and its DLLs) are unpacked into `~/.ezlm` and started on every deploy
(`internal/engine/engine.go:192-260`, `:349-410`); EZLM's own `node.exe`
(`internal/integrate/node_install.go:28-90`) and `opencode.exe` (`internal/integrate/tools.go:176`)
are downloaded the same way. These are freshly written, unsigned or third-party binaries, which is
exactly what Smart App Control and "block unsigned" ASR rules target. Signing (finding 1) is the
only real fix; until then, map the Windows error to a plain sentence rather than the installer's
last output line.

### 6. MINOR — check workspaces live in `%TEMP%` and the agent CLI runs with that cwd

`internal/integrate/check.go:33`, `internal/integrate/dsh_check.go:51` and
`internal/integrate/opencode_check.go:80` create `%TEMP%\ezlm-check-*` / `ezlm-dsh-check-*` /
`ezlm-opencode-check-*` and run third-party CLIs with the working folder inside them. Some EDR
products flag child processes whose working folder is a temp folder; the check itself passes, so
this is a noise risk, not a failure.

Fix: put the check folders under `EZLM_HOME` (the package already honours `EZLM_HOME`, so tests
stay hermetic) and remove them on the same path as today.

### 7. MINOR — stale `%TEMP%\EZLM-update` copies are never removed

Before this change every update left the previous setup in `%TEMP%\EZLM-update`. Existing users
keep that folder after upgrading; it is harmless but large. A one-time cleanup on the next
download would be cheap. Not done here to keep the change small.

### 8. NOTE — the `.cmd` launcher is unsigned but is not a SmartScreen target

`internal/integrate/launch.go:97` rewrites `~/.ezlm/launch/<id>.cmd` on every launch and
`internal/integrate/terminal_windows.go:63-80` starts it through `cmd.exe`. Smart App Control
evaluates PE images, not batch files, and cmd.exe is signed, so this is low risk. No change
proposed; recorded so the next reviewer does not re-derive it.

## What I changed

- `internal/update/update.go`: new `DownloadDir()` (`EZLM_HOME\update`) and `Download` uses it
  instead of `os.TempDir()/EZLM-update`. Reason: finding 3 — do not run an unsigned installer
  from a temp folder, and do not let Disk Cleanup remove it before the user runs it.
- `internal/update/update_windows_test.go` (new): `TestDownloadDirIsNotTemp` and
  `TestDownloadWritesIntoDownloadDir`. Both failed against the old `os.TempDir()` code (the first
  with the temp path, the second with a path outside `EZLM_HOME`) and pass after. Validated by
  running them here under a non-Windows name; they are in a `_windows_test.go` file because the
  temp-folder behaviour they pin is Windows behaviour.

Checks run in this worktree: `gofmt -l .` (empty for my files), `go build ./internal/update`,
`GOOS=windows GOARCH=amd64 go build ./...`, `GOOS=windows GOARCH=arm64 go build ./...`,
`go vet ./internal/update`.

## Could not verify without a Windows machine

- Whether Smart App Control actually blocks EZLM in its shipped configuration (needs a clean Win11
  VM with SAC on; check `HKLM\SYSTEM\CurrentControlSet\Control\CI\Policy`).
- The exact SmartScreen behaviour of `Process.Start` with `UseShellExecute` on a file with no
  `Zone.Identifier` (finding 3/4).
- Defender ASR audit events for the Hermes install (needs `Get-MpPreference`/event 1121-1124).
- The `_windows_test.go` file compiles under `GOOS=windows` (`go vet`), but has not been run on
  Windows.

## Open questions

- Is the update installer meant to be signed this release? If not, SAC machines cannot self-update
  at all, and the window should say so instead of closing.
- Should EZLM set a `Zone.Identifier` on downloaded installers and models, or keep them "local"?
  The current behaviour is neither documented nor asserted by a test.
- Do we want a single "Windows security software blocked this" sentence shared by the updater,
  Hermes and llama-server, instead of three different failure paths?
