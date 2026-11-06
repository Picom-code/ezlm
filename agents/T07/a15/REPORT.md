# T07/a15: the apps' engine clients (report)

**Threat:** the per-launch engine token (`X-EZLM-Token`) and the API key as
they are carried by the two native apps: URL vs header, logs, and crash or
problem reports.

**Method:** code reading only, both apps and the engine side they talk to. No
app files edited (the apps are outside T07's owned paths; the brief says
report-only). Findings with file:line and proposed fixes are in
[`clients.md`](clients.md).

## Result

**Reproduced: n/a (report-only slice). Fixed: no app changes; findings
proposed.** Severity of the worst finding: minor. Nothing in the apps puts the
token in a URL, on disk, or in a crash report today.

What is verified good:

- Both apps carry the token in the `X-EZLM-Token` header on every engine call,
  including the SSE stream (`windows/EZLM/Engine.cs:79-80,111,118,145`,
  `macos/Sources/EZLM/Engine.swift:113,159`).
- The token comes from the child's stdout hello line, is per launch, and is
  never persisted (`Engine.cs:77-80`, `Engine.swift:88-91`).
- The API key goes to llama-server as `Authorization: Bearer`
  (`windows/EZLM/Chat.cs:745`, `macos/Sources/EZLM/ChatModel.swift:246`).
- Problem reports and support pastes redact `ezlm-` keys
  (`windows/EZLM/Shell.cs:214-215`, `macos/Sources/EZLM/AppModel.swift:1040-1050`,
  engine side `internal/integrate/log.go:34-43`,
  `internal/app/launch.go:230-237`).

Findings (details and patches in `clients.md`):

| # | Finding | Severity |
|---|---------|----------|
| F1 | Hello URL is trusted without a loopback check before the token is attached (`Engine.cs:78-80`, `Engine.swift:90-91`) | minor |
| F2 | SSE retry loop keeps sending the stale token to the stale port after the engine dies, and trusts events from whoever holds the port (`Engine.cs:139-161`, `Engine.swift:154-175`) | minor |
| F3 | Engine stderr goes into support bundles, but redaction does not recognise the 64-hex launch token (`Engine.cs:84-96`, `Engine.swift:48-51`, `server.go:81`, `integrate/log.go:34-43`) | minor, latent |
| F4 | Opening llama.cpp's chat copies the API key to the clipboard automatically (`Panes.cs:246-252`, `ReportLaunch.swift:9-16`) | minor |
| F5 | App-side `HideKeys` is narrower than the engine's redaction (no `sk-`/`Bearer` patterns) | info |
| F6 | Engine client uses the system proxy and follows redirects | info |

## Cross-task notes (for the morning merge)

- **F3 is the one worth doing early**, because it is cheap and belongs to T07's
  own files: give the token a recognisable prefix in
  `internal/web/server.go:81` (for example `ezlm-t-` + hex) and add that
  pattern to `integrate.Redact`. That makes the existing "engine stderr goes
  into a public GitHub issue" path safe before anyone adds a debug log line
  that prints the `?t=` URL from `server.go:82`. `guard` reads the header or
  `?t=`, so the token format change needs no app change.
- F1, F2, F4, F5, F6 are app-side and were not edited. They need whoever owns
  `windows/EZLM` and `macos/Sources/EZLM` after the merge.
- a04 should note that the apps do not use the `?t=` URL form at all, so the
  query-string token exposure is browser-only.

## Not finished / open questions

- No runtime test of the apps (no Xcode or Windows run tonight); all app
  findings are code-read, as the brief allows for this slice.
- F2's impact depends on whether a local process can win the port race in the
  gap between the engine exiting and the user pressing retry. Not measured.
- F4 is a product decision (automatic copy is load-bearing for the llama.cpp
  chat flow), so it is proposed, not called a bug.

## Commit

`T07: review the apps' engine token handling` on `agent/T07-a15`.
