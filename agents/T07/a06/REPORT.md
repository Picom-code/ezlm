# T07 / a06: the API key on the llama-server command line

**Threat:** every local user on the machine can read another user's process
arguments with `ps` (Linux/macOS) or Task Manager / `wmic process get
CommandLine` (Windows). EZLM passed every accepted API key to llama-server as
`--api-key <key>`, so any other logged-in user could read the key and use the
model server (or, in LAN mode, reach it from the network with that key).

**Status:** reproduced and fixed. Severity: **major** on a shared machine
(multi-user desktop, lab or school PC); minor on a single-user laptop, where
the browser page threat is the bigger one.

## Evidence before the fix

`internal/serve/config.go:121` (baseline `5213370`) built the flag list with

```go
"--api-key", c.keyList(),
```

and `c.keyList()` joined every key with commas. All launch paths go through
`Config.Args()`: `internal/serve/run.go:23`, `service_darwin.go:51`,
`service_linux.go:51`, `service_windows.go:167`. On macOS and Linux the
launchd plist / systemd unit also stores the key in plain text, and `ps` shows
it for the life of the process. No `--api-key-file` or `LLAMA_API_KEY` use
existed anywhere in the repo (grep: the only hit was `config.go:121`).

Proof (test written first, run against the baseline):

```
$ go test -p 2 -run 'TestKey' ./internal/serve/
--- FAIL: TestKeyNotOnLlamaServerCommandLine (0.00s)
    key_cmdline_test.go:61: the API key is on the llama-server command line: "REDACTED-KEY"
--- FAIL: TestKeyFileHoldsEveryKeyOnePerLine (0.00s)
    key_cmdline_test.go:107: no --api-key-file in [-m /models/m.gguf --alias  --host 127.0.0.1 --port 0 --api-key REDACTED-KEY-1,REDACTED-KEY-2 -c 0 -ngl 0 --jinja --metrics -np 1]
FAIL
```

## Fix

Only the auth part of `internal/serve/config.go` changed.

- `KeyFile()` is `~/.ezlm/api-keys.txt` (`EZLM_HOME` respected).
- `Args()` calls `writeKeyFile()` and passes `--api-key-file <path>` instead
  of `--api-key`. The file is written to a temp file, `chmod 0600`, then
  renamed, so the server never reads a half-written or wider-mode file.
- One key per line: llama-server's `--api-key-file` parser treats each line
  as a whole key (verified against the pinned engine, llama.cpp `b11960`,
  `common/arg.cpp`: `--api-key-file` reads lines, skips `#` comments, and
  does not split on commas). The old comma-joined form stays only in the
  fallback path via `keyList()`.
- The file is rewritten on every `Args()` call, so a rotated or removed key
  disappears from it.
- Fallback: if the file cannot be written (locked or read-only home), the
  server still starts with `--api-key`, which is visible to `ps`. This is a
  last resort, not the normal path.

`--api-key` was already the only auth mechanism; llama-server accepts either
flag, and `--api-key-file` is present in the pinned b11960 build.

## Tests (`internal/serve/key_cmdline_test.go`)

- `TestKeyNotOnLlamaServerCommandLine`: no argv element contains the key, no
  `--api-key`, `--api-key-file` points under the temp `EZLM_HOME`, the file
  holds the key, and its mode is 0600 (skipped on Windows, where mode bits
  are not meaningful; ACLs are a07's finding).
- `TestKeyFileHoldsEveryKeyOnePerLine`: two keys are written one per line
  with no comma joining, and a rotated key set leaves no stale key behind.
- `TestKeyFileFallbackKeepsServerStarting`: an unwritable home falls back to
  `--api-key` so the server still starts.

All tests set `EZLM_HOME` to `t.TempDir()`, so they never touch the real
`~/.ezlm`.

## Residual risk / open questions

- The key file is only as private as `~/.ezlm` and the user account. On
  Windows, `os.WriteFile(..., 0600)` does not set an ACL; the file inherits
  `%USERPROFILE%\.ezlm`, which is normally readable by that user only. See
  a07 for the ACL review.
- Keys are also stored in plain text in `~/.ezlm/config.json` (0600), and
  macOS launchd/systemd units no longer contain them after this fix, but the
  config does. Out of scope for a06.
- Uninstall does not delete `~/.ezlm/api-keys.txt`; the file dies with the
  home directory. Minor.
- If a key ever contained a newline, `--api-key-file` would split it. Keys
  are generated from base62 by `internal/keys`, so this cannot happen today.

## Commit

`T07: read llama-server API keys from a private file, not the command line`
