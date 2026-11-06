# OpenCode fixtures

Keys are `REDACTED-KEY` or test keys; paths, ports, nonces and dates are normalised.

| File | Source |
|---|---|
| `mac-v2-user.json` | the Mac's hand-written `~/.config/opencode/opencode.json` (OpenCode 2.2.5), host name replaced |
| `desktop-ezlm-v1.json` | the desktop's `C:\Users\picom\.config\opencode\opencode.json`, written by EZLM 0.1.x for OpenCode 1.19.5 |
| `commented.jsonc` | hand-written JSONC: comments, trailing commas, one-line objects |
| `db-v1.sqlite.gz` | `opencode.db` created by OpenCode 1.19.5 (`session` table), rows removed |
| `db-v2.sqlite.gz` | `opencode.db` created by OpenCode 2.2.5 (`session_v2` only); 1.19.5 stops with "Database is not empty and has no session table" |
| `db-converted.sqlite.gz` | the v1 database after 2.2.5 opened it: both tables; 1.19.5 still opens it |
| `db-wal.sqlite.gz` + `db-wal.sqlite-wal.gz` | v1 database with `session_v2` created only in its uncheckpointed WAL |
| `run-<version>-<case>.jsonl` | `opencode run [--standalone] --model ezlm/qwen3.5-4b --format json <check prompt>` against `internal/fakellm`, recorded by `TestOpenCodeReal` with `EZLM_OPENCODE_RECORD` |
| `tui-<version>.txt` | the TUI's screen (ANSI stripped) in a 120x40 pty after the check prompt was typed (`cmd/tuiprobe`) |

## `run --format json` events (1.19.5 and 2.2.5)

One JSON object per line, `{"type", "timestamp", "sessionID", ...}`:

- `step_start`: `part.type` `step-start`.
- `tool_use`: `part.type` `tool`, `part.tool` (`read`), `part.state.status` `completed` or `error`,
  `part.state.input` (2.2.5 `{"path"}`, 1.19.5 `{"filePath"}`), `part.state.output`.
- `step_finish`: `part.reason` `tool-calls` or `stop`, `part.tokens`.
- `text`: `part.text`, the answer. The check passes only when the nonce is in a `text` event (the tool's
  output contains it too).
- `error`, exit code 1. 2.2.5: `error.type` + `error.message` (`provider.auth` "Invalid API Key",
  `provider.no-route` "Model unavailable: ezlm/x" or "Variant unavailable for ezlm/x: high",
  `provider.transport` "ECONNREFUSED: Unable to connect…" after about 80 s of retries).
  1.19.5: `error.name` + `error.data.message` (`APIError` "Invalid API Key", `UnknownError`
  "Unexpected server error" for a missing model, whose real text, "Model not found", is only in
  `--print-logs` lines on stderr).

## Behaviour seen with the real binaries

- 2.2.5's TUI has no `--model` ("Unrecognized flag: --model"); the workspace `opencode.json` picks the
  model, and the footer shows "<model name>" and "EZLM (local)" once it is selected (both versions).
- `opencode models` takes no provider on 2.2.5 (`models --standalone` lists all); 1.19.5 takes `models ezlm`.
- Without `--standalone`, 2.2.5 starts or joins the background service on 127.0.0.1:49374 whatever
  `XDG_STATE_HOME` says, so an isolated run collides with the user's own service.
- `opencode run` reads a piped stdin as part of the message and waits for it to close; run it with no stdin.
- OpenCode takes its working folder from `$PWD`, not the process's own; set both.
- Reasoning: with no `variants`, no request carries `reasoning_effort`. With `variants` listing only
  the template's levels, the default request still carries none; 1.19.5's title request uses the first
  variant on its own, and a variant that isn't listed fails with "Variant unavailable".
- 2.2.5 reads `OPENCODE_CONFIG_DIR` instead of `~/.config/opencode`; 1.19.5 reads it on top, after the
  project file, so there it outranks the workspace `opencode.json`.
