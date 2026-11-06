# T07 / a07 — file permissions of `~/.ezlm` (macOS and Windows)

Agent: a07. Threat: another local user on the machine (and, on Windows, any
account that inherits access to the user profile) reads EZLM's API key,
config or logs from `~/.ezlm`.

## What is stored where

| Path | Content | Mode set by code (baseline) |
|---|---|---|
| `~/.ezlm/` | folder | `0755` (`internal/paths/paths.go:40`) |
| `~/.ezlm/config.json` | **the API key(s)**, model path, port, LAN flag | `0600` (`internal/serve/config.go:85`) |
| `~/.ezlm/settings.json` | model folders | `0600` (`internal/paths/settings.go:45`) |
| `~/.ezlm/logs/server.log` | llama-server output | `0644` (`internal/serve/run.go:18`, `internal/serve/service_windows.go:181`) |
| `~/.ezlm/logs/integrate-*.log` | tool logs, keys redacted | `0600` (`internal/integrate/log.go:85`) |
| `~/.ezlm/ui.url` | setup URL **with the per-launch token** (`?t=`) | `0600` (`cmd/ezlm/main.go:298`) |
| `~/.ezlm/integrations.json` | probe cache | `0600` (`internal/integrate/cache.go:58`) |

`internal/keys` only generates keys (`keys.New`); the key is persisted in
`config.json`. Key storage/rotation itself is a12's slice.

## Findings

### F1 (major, fixed) — `~/.ezlm` was world-listable and world-traversable

`internal/paths/paths.go:38-45` created `Home()` with `0o755`. Any local
user could list the folder and traverse into `logs/`. The key file was still
`0600`, so the key itself was not readable on Unix through this alone, but
the directory exposed every filename and made the `0644` server log
readable (F2). `Ensure` also never repaired an existing folder, so an
install made by an older version stayed `0755` forever.

Evidence: `TestEZLMHomeOwnerOnly` failed before the fix with
`new .../.ezlm is 755, want 700` and `existing .../.ezlm stayed 755`.
After: both pass (`0700`, and the chmod repair runs on every `Ensure`).

Fix: create `Home()` at `0700` and `chmod` it to `0700` on each `Ensure`.
Subfolders stay `0755`; they are unreachable by other users once the parent
is `0700`, and `models/` may be a user-chosen external folder.

### F2 (major, fixed) — `server.log` was world-readable

`internal/serve/run.go:18` (foreground) and
`internal/serve/service_windows.go:181` (Windows detached start) created
`~/.ezlm/logs/server.log` at `0644`. Combined with the `0755` folder, any
local user could read the model server's log: startup lines, request lines,
prompts/completions echoed by llama.cpp. The exact key leak depends on what
llama-server prints (not verified without starting it), but the log is user
data and should be owner-only regardless.

Evidence: `TestServerLogOwnerOnly` failed before with `server.log is 644,
want 600`; passes after.

Fix: both open the log at `0600`.

### F3 (minor, fixed) — `settings.json` kept an older file's mode

`internal/paths/settings.go:41-45` used `os.WriteFile(..., 0o600)` on a
file it writes in place. `WriteFile` applies the mode only when it creates
the file, so an existing `0644` `settings.json` (older build, restored
backup, manual copy) stayed `0644`. No key is in this file, but it lists
model folders and is the same class of mistake as F4.

Evidence: `TestSettingsFileOwnerOnly` failed before with `settings.json is
644, want 600`; passes after.

Fix: explicit `os.Chmod(SettingsPath(), 0o600)` after the write, and create
`Home()` at `0700` here too.

### F4 (minor, fixed) — a leftover `config.json.tmp` decided the key file's mode

`internal/serve/config.go:85-89` wrote `config.json.tmp` with
`os.WriteFile(tmp, b, 0o600)` and renamed it over `config.json`. If a temp
file from an earlier crash (or an older build) already existed at `0644`,
`WriteFile` kept `0644` and the rename made the **key-bearing**
`config.json` world-readable. The normal path is safe (`WriteFile` creates
the temp at `0600`); this is a defence-in-depth bug, not an everyday one.

Evidence: `TestConfigKeyFileOwnerOnly` plants a `0644` temp file and failed
before with `config.json after a 0644 temp file is 644, want 600`; passes
after.

Fix: `os.Chmod(tmp, 0o600)` before the rename. A plain `config.json` that
is already `0644` is repaired by the rename today (the test checks that
first).

### F5 (major, open, code-read only) — Windows has no ACL protection at all

Nothing in the Go engine or either app sets a Windows DACL: there is no
`SetNamedSecurityInfo`, `icacls`, `FileSecurity` or `DirectorySecurity`
anywhere (grep over `internal/`, `cmd/`, `windows/EZLM`, `macos/`). Go's
`os.FileMode` on Windows only maps the write bit to
`FILE_ATTRIBUTE_READONLY`; it does not restrict who may read. So the
`0o600` on `config.json`, `settings.json`, `ui.url` and the logs has **no
access-control effect on Windows**. The key is protected only by the ACLs
inherited from `%USERPROFILE%\.ezlm`, which are whatever the profile
inherits (a default profile typically lets `BUILTIN\Users` read and
traverse; administrators and backup operators can always read). On a
shared Windows PC, another local user may therefore read the API key from
`config.json`, and with LAN mode on, use it against the model server.

Severity major for a shared Windows PC; minor for the common single-user
laptop. This is code-read, not reproduced: no Windows host was available.

Proposed fix (not applied; `golang.org/x/sys` is already a dependency):
a `_windows.go` helper that sets a protected DACL on `~/.ezlm` granting
`FILE_ALL_ACCESS` to the current user SID only (owner rights, no
inheritance from the profile), called from `paths.Ensure` after the
`MkdirAll`, then `SetNamedSecurityInfo` with
`PROTECTED_DACL_SECURITY_INFORMATION`. Test on Windows with `icacls`.

### F6 (minor, open, frozen file) — `ui.url` mode is not repaired

`cmd/ezlm/main.go:298` writes `~/.ezlm/ui.url` (which contains the
per-launch token in `?t=`) with `os.WriteFile(..., 0o600)`. Like F3/F4, an
existing `0644` file keeps its mode. `cmd/**` is frozen for T07, so this is
a finding for T10/a04: `chmod 0600` after the write, or write through a
temp file. (a04 owns token leakage; a07 only notes the mode.)

## What changed

- `internal/paths/paths.go` — `Ensure` creates `Home()` at `0700` and
  repairs an existing folder with `chmod`.
- `internal/paths/settings.go` — `SaveSettings` creates `Home()` at `0700`
  and chmods `settings.json` after writing.
- `internal/serve/config.go` — `Save` chmods `config.json.tmp` to `0600`
  before renaming.
- `internal/serve/run.go`, `internal/serve/service_windows.go` — server log
  opened at `0600` instead of `0644`.
- `internal/keys/permissions_test.go` — four tests covering F1-F4, all
  hermetic (`EZLM_HOME` in `t.TempDir()`).

**Ownership note for the merge:** T07's stated owned paths are
`internal/web/server.go`, `internal/keys/**` and the auth/LAN/key parts of
`internal/serve/**`; `internal/paths/**` is not on that list, but the
`~/.ezlm` folder mode is set there. If the merge drops paths outside the
owned list, the `internal/paths` hunks must be kept (or applied by the
owner of `internal/paths`), otherwise F1/F3 stay open.

## Verification

- `go test -p 2 -count=1 ./internal/keys/ ./internal/paths/ ./internal/serve/`
  — pass (before the fix the four new tests failed as quoted above).
- `gofmt -l .` — empty.
- `go build ./...`, `GOOS=windows GOARCH=amd64 go build ./...`,
  `GOOS=windows GOARCH=arm64 go build ./...` — pass.
- `go vet ./internal/keys/ ./internal/paths/ ./internal/serve/` — pass.
- Not run: full `go test -p 2 ./...` (other packages are being rewritten
  tonight; the slice's packages are green).

## Residual risk and open questions

- Root/administrator, backups (Time Machine), and cloud sync of the home
  folder can still read the key; file modes cannot fix that.
- The Windows default ACL on `C:\Users\<user>` was not checked on a real
  machine. If the default profile is not readable by `Users` on current
  Windows builds, F5 drops to minor; the absence of an explicit DACL is
  still true and worth fixing for LAN mode.
- Does llama-server print its `--api-key` value at startup? Not verified
  (no model/server start allowed). If it does, F2 was a direct key leak on
  Unix before the fix.
- `logs/` and `bin/` stay `0755`; if `Home()` is ever made group-accessible
  again, revisit.
- Stretch not done: a Windows ACL test (needs a Windows runner), and the
  same mode-repair for `cmd/ezlm`'s `ui.url` (frozen).
