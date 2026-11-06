# T07 / a08: redaction of keys and tokens in logs, reports and errors

**Threat.** A stranger's PC runs EZLM. Keys and the per-launch token must not
reach a log file, a problem report (public GitHub issue), an error string shown
in the UI, or a crash dump. A leaked API key lets anyone who can reach the
port use the model; a leaked web token lets a local page drive the engine API.

**Verdict.** The report path already redacts: `App.Report` runs the whole body
through `integrate.Redact` before filing (`internal/app/launch.go:237`), and
integration logs are redacted when written (`internal/integrate/log.go:99-145`).
Two paths were not redacted, and one pattern gap remains.

## Findings

### F1. Model-server log tail was served and shown unredacted (minor, fixed)

`serve.TailLog` returned the file verbatim (`internal/serve/client.go:148`).
Three consumers pass it on:

- `GET /api/logs` returns it to the page (`internal/web/server.go:222`), where
  it lands in a screenshot or a copy-paste;
- `WaitHealthy` embeds the last 8 lines in the load-failure error
  (`internal/serve/client.go:98,106`), which reaches `Status.StartError` and
  the UI;
- `fatalInLog` returns one of its lines as the user-facing reason
  (`internal/serve/client.go:132`), and `internal/app/app.go:1551` shows it.

The key is on llama-server's command line (`--api-key`, `internal/serve/config.go:121`),
so any verbose build that echoes argv puts it in `~/.ezlm/logs/server.log`.
Today's real log does not (0 matches for `ezlm-` and `--api-key` in
`~/.ezlm/logs/server.log`), so this is a hardening fix, not a live leak: one
server build that logs its args, or one user running llama-server with `-lv 4`,
turns it into a live key leak into the page and into bug reports.

Fix:
- `internal/serve/client.go:163` - `TailLog` now returns
  `integrate.Redact(...)`, so every consumer (log view, load error, report)
  gets shortened keys.
- `internal/web/server.go:234` - new `redactLines` applied to the `/api/logs`
  and `/api/integrate/log` responses (`server.go:177,222`), so the API does
  not rely on the backend having redacted.

Proof (`internal/web/redaction_test.go`): `TestTailLogRedactsKeys`,
`TestFatalInLogRedactsKeys`, `TestAPILogsRedactKeys`,
`TestIntegrationLogRedactsKeys`. All four fail on the baseline:

```
redaction_test.go:100: /api/logs leaked "ezlm-AbCdEfGhIjKlMnOpQrStUvWxYz0123456789ab"
redaction_test.go:141: serve.TailLog leaked "ezlm-AbCdEfGhIjKlMnOpQrStUvWxYz0123456789ab"
redaction_test.go:164: serve.FatalInLog leaked "ezlm-AbCdEfGhIjKlMnOpQrStUvWxYz0123456789ab"
```

and pass after. The test keys look like `REDACTED-KEY` and are generated in
the test; no real key is used or printed.

### F2. The per-launch token is not covered by any redaction pattern (minor, open, report-only)

`integrate.Redact` matches `ezlm-…`, `sk-…` and `Bearer …`
(`internal/integrate/log.go:26-29`). The engine token is 32 lowercase hex
characters (`internal/web/server.go:79-82`, `hex.EncodeToString(16 bytes)`),
so it matches none of them. `TestRedactGapPerLaunchToken` pins this.

Where it can reach a public report: `cmd/ezlm` prints the setup URL with the
token (`cmd/ezlm/main.go:302`), stores it in `~/.ezlm/ui.url` mode 0600
(`main.go:298`), and reprints it on a second launch (`main.go:284`). A tester
who pastes that URL into the report text publishes a working token for the
current launch. The token dies with the process, so exposure is short.

The redactor lives in the frozen `internal/integrate` tree, so this is
report-only. Proposed patch in `internal/integrate/log.go`:

```go
var tokenLike = regexp.MustCompile(`\b[0-9a-f]{32}\b`)

// in Redact, after RedactKeys:
s = tokenLike.ReplaceAllStringFunc(s, func(t string) string { return t[:8] + "…" })
```

Check false positives: a 32-hex model hash would be shortened to 8 chars,
which is harmless in a report. This overlaps a04 (token in the URL).

### F3. The report title is not redacted (minor, open, report-only)

`internal/app/launch.go:244` sends `"Beta report: "+first`, where `first` is
the user's first line, without `integrate.Redact`. The body is redacted
(line 237), the title is not, and the title is the public issue title. A user
who pastes their key or the setup URL in the first line leaks it. Proposed:
redact the title with the same `secrets` slice, or build the title after
redaction.

### F4. Cross-references (not this slice, no change made)

- The API key is in llama-server's argv (`internal/serve/config.go:121`), so
  it is visible in `ps` and in the launchd plist / systemd unit (0600 today)
  and the Windows scheduled task. That is a06/a07.
- `~/.ezlm/logs/server.log` is created mode 0644 (`internal/serve/run.go:18`,
  `internal/serve/service_windows.go:181`), so another local user can read the
  log. It has no key today (see F1). That is a07.
- No crash-dump path exists in the Go engine (no `recover`, no `debug.Stack`
  dump to a file), so a crash cannot write a key to disk through Go's default
  panic output.

## What I changed

| File | Change |
|---|---|
| `internal/serve/client.go` | `TailLog` redacts via `integrate.Redact`; comment says why |
| `internal/web/server.go` | `redactLines` helper; `/api/logs` and `/api/integrate/log` redact backend lines |
| `internal/web/redaction_test.go` | 6 tests: 4 before/after proofs, 1 report-redactor table test, 1 pinned gap (F2) |

No new dependency. `internal/integrate` and `internal/app` untouched (frozen).

## Checks

- `gofmt -l .` empty, `go build ./...`, `go vet ./...` pass.
- `go test -p 2 ./internal/web/ ./internal/keys/ ./internal/serve/` pass.
- Full `go test -p 2 ./...` and the three cross-builds: see the commit message
  / rerun below.

## Residual risk and open questions

- A key split across two log lines is only partly redacted (the first part is
  shortened, the remainder stays). A 40-character key wrapped at 30 still
  leaves 10 characters, not enough to use.
- `Redact` only knows the patterns above. A future key format needs a new
  pattern and a test here.
- Should the setup URL avoid the token in the query string entirely
  (fragment or one-time header)? That is a04's decision; if it changes, F2
  mostly disappears.
- Windows ACLs for the service definition were read by code only (a07 owns
  the test).
