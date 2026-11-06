# T15 / a06: ACLs and permissions on Windows

Scope: what EZLM writes, who can read it, and whether a second Windows user can
read the API key. No Windows machine was available, so every claim below is
either (a) proven by a test that compiles under `GOOS=windows` but was not run,
or (b) marked **code-read**. Go behavior on Windows (`os` modes, process
command lines) is documented behavior, not a guess.

## Findings

### F1. The API key is in llama-server's command line, so a second user can read it (major, fixed)

- `internal/serve/config.go:121` puts the key in the server arguments:
  `"--api-key", c.keyList()`.
- `internal/serve/service_windows.go:191` (before the fix) started the server
  with `exec.Command(server, args...)`.
- On Windows the command line of a process is readable by other local users
  through Task Manager's Details tab and `Get-CimInstance Win32_Process`
  (WMI `Win32_Process.CommandLine`). The key is a 40-character base62 string
  (`internal/keys/keys.go`), so once it is seen it cannot be guessed back.
  The server runs for hours or days, so the exposure is not a race.
- This is the direct answer to "can a second user read the API key": yes, for
  any non-admin account on the machine, even though `%USERPROFILE%\.ezlm` is
  private.

Fix: `internal/serve/service_windows.go` now moves the key out of the
arguments into the environment as `LLAMA_API_KEY` before `cmd.Start()`
(`keylessArgs`, lines 207-222). llama-server reads that variable for
`--api-key` (verified in the pinned build: llama.cpp b11960,
`common/arg.cpp` line ~3551, `.set_env("LLAMA_API_KEY")`; the value is still
parsed as a comma-separated key list). A child's environment block is only
readable by its owner (and administrators), so the second-user leak is closed.

Tests: `internal/serve/acl_windows_test.go` (`TestServerCommandLineHidesAPIKey`,
`TestKeylessArgsEdges`) fail against the old `startLogged` and pass with the
fix; they compile under `GOOS=windows`. Not run here.

Residual, not fixed (shared files, out of this slice): `internal/serve/run.go:23`
(`ezlm run-server`, a manual/debug path) still passes `--api-key` in argv. The
same helper should be used there; it needs a non-Windows stub because `run.go`
builds on every OS. On macOS/Linux the same leak exists through
`ps`/`/proc/<pid>/cmdline`, and `service_darwin.go`/`service_linux.go` write
the full command line, key included, into the launchd plist and systemd unit
(both mode 0600, so only the owner can read those files).

### F2. `os.WriteFile(..., 0o600)` is not an ACL on Windows; config.json keeps the folder's ACL (minor, not fixed)

- `internal/serve/config.go:86` writes `config.json` (and its `.tmp`) with
  mode 0600; `internal/paths/paths.go:41` creates `~/.ezlm` with 0755.
- On Windows Go maps a mode to the read-only attribute only. The file reports
  0666 and inherits its DACL from `%USERPROFILE%`, which by default grants the
  user, SYSTEM and Administrators and denies other standard users. So the
  default install is fine, but:
  - `EZLM_HOME` (`internal/paths/paths.go:11`) can point anywhere. A folder on
    a shared or second drive whose ACL grants `Users` read makes the key
    readable by every local user, and mode 0600 will not change that.
  - Rewriting an existing `config.json` (for example one an older build or an
    administrator created with wider inheritance) keeps the old ACL; 0600 does
    not tighten it.
- Test `TestConfigModeIsNotAnACLOnWindows` documents the 0666 result.
- Proposed fix: after `os.Rename`, set a protected DACL on Windows (current
  user, SYSTEM, Administrators) with `SetNamedSecurityInfoW` and
  `PROTECTED_DACL_SECURITY_INFORMATION`, and do the same for the `.tmp` file.
  Needs a `_windows.go`/`_other.go` pair behind `Config.Save`. Not done here
  because it cannot be exercised without a Windows machine and would touch the
  shared `config.go`.

### F3. Keys written into other apps' config files inherit the same weakness (minor, code-read)

- Hermes: `internal/integrate/hermes_config.go:164` writes the plaintext
  `api_key` into `~/.hermes/config.yaml` with 0600.
- OpenCode: `internal/integrate/opencode_config.go:106` writes the key file
  with 0600 (its JSON references it as `{file:...}`, so the JSON at 0644 is
  not a leak).
- DeepSeek Harness: `internal/integrate/dsh_config.go:507` writes `.env` with
  0600.
- Same Windows semantics as F2: private only because of the profile's
  inherited ACL, not because of the mode. All of these tools live in the user
  profile by default, so a second standard user is denied; an administrator
  can read them, which is inherent. `dsh` itself refuses a credentials file
  that is "readable beyond its owner" (`internal/integrate/dsh.go:38`); on
  Windows that check sees Go's synthesized mode, so its behavior there is
  worth a Windows check (open question 2).

### F4. Server log is 0644 and does not contain the key (not a finding, checked)

`internal/serve/run.go:18` and `internal/serve/service_windows.go:181` open
`~/.ezlm/logs/server.log` with 0644. llama-server b11960 does not log the API
key (`tools/server/server.cpp`: the only key-related log line warns when no key
is set), so the log is not a cross-user key leak. Its ACL is the profile's,
like F2.

### F5. The web UI token travels on a command line (minor, code-read, frozen file)

`cmd/ezlm/main.go:338` opens the setup page with
`rundll32 url.dll,FileProtocolHandler http://127.0.0.1:PORT/?t=TOKEN`. While
that short-lived process exists, another local user can read the token from its
command line and then call `/api/status`, which returns `*serve.Config`
including every key (`internal/app/app.go:58-62`, guarded only by that token).
The window is short (rundll32 exits after handing off to the browser), so this
is minor. Proposed fix: keep the token out of the URL (a cookie set by a
one-shot endpoint, or the fragment), or accept the token in a header only.
`cmd/**` is frozen, so this is report-only.

## What changed

- `internal/serve/service_windows.go`: `startLogged` strips `--api-key` and its
  value from the child arguments and passes `LLAMA_API_KEY` in the child's
  environment (`keylessArgs`, lines 207-222).
- `internal/serve/acl_windows_test.go`: new tests (Windows-only build).

No C# or installer change. `windows/installer/EZLM.iss` installs per user
into `%LOCALAPPDATA%\Programs\EZLM` with `PrivilegesRequired=lowest`, so the
program folder is per-user and private; nothing in it holds the key. The key
lives in `%USERPROFILE%\.ezlm` and in the connected apps' config files.

## Verification

```
gofmt -l internal/serve                      # empty
go build ./...
go vet ./internal/serve/
go test -p 2 ./internal/serve/                # ok
GOOS=windows GOARCH=amd64 go build ./...      # ok
GOOS=windows GOARCH=arm64 go build ./...      # ok
```

`GOOS=windows go vet ./internal/serve/` also compiles the new test file.

## Could not verify without Windows

- That `LLAMA_API_KEY` actually reaches llama-server and authenticates (the
  argument parser is verified in the pinned b11960 source; the env pass-through
  is Go's standard `exec.Cmd.Env`).
- WMI/Task Manager command-line visibility for a second standard user (Windows
  documented behavior; a clean-VM check belongs in the a16 checklist: log in as
  user B, run `Get-CimInstance Win32_Process -Filter "name='llama-server.exe'"
  | select CommandLine`, and confirm the key is absent after the fix).
- The real ACL of `%USERPROFILE%\.ezlm` on a fresh profile, on a OneDrive
  redirected profile, and on a domain-joined machine.

## Open questions

1. Should EZLM set an explicit protected DACL on `~/.ezlm` (and config.json)
   even though the default profile ACL is already private? It would make
   `EZLM_HOME` on a shared drive safe. Cost: Windows-only code that cannot be
   tested from here.
2. Does `dsh`'s "readable beyond its owner" check pass on Windows? Go reports
   a writable file as 0666, so a check on `Mode().Perm()` would fail on every
   Windows install. Code-read only; a03/a12 territory.
