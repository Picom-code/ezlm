# T07 severity table

Every threat in the T07 brief, with the status of the control in this tree, the
evidence, a severity, and the fix. `main` is the source snapshot; a status of
`mitigated` means the control is present in the code, `open` means it is not.

Severity is judged for the beta threat model: a malicious web page in the user's
browser, and another user on the same LAN. `blocker` = a stranger's PC can be
attacked or its key stolen with no user mistake; `major` = the key or the model
is exposed, or a fix is needed before beta; `minor` = hardening, no practical
attack today.

Evidence is marked `[code]` (read from the source, file:line) or `[test]` (the
owner's test fails without the control). `fixed` means fixed on the owner's
branch at the commit named, not in this a16 worktree: this branch carries only
`agents/T07/a16/`. Line numbers are from the baseline snapshot (`5213370`)
unless the commit is named. A row with no `[test]` is still open to proof by its
owner agent.

| ID | Threat | Status | Evidence | Severity | Fix / residual risk | Owner |
|---|---|---|---|---|---|---|
| T01 | DNS rebinding: a page on `evil.example` resolves to 127.0.0.1 and drives the engine API | fixed | `[test]` `internal/web/host_rebinding_test.go` (`TestHostRebindingReadsAPI`, `TestHostRebindingHosts`, `TestHostCheckRunsBeforeToken`), commit `cc25fa7`. The baseline exact-match check already blocked a foreign `Host` but rejected `localhost`; `loopbackHost` now accepts only `localhost` or a loopback IP literal with the listener's port, and the Host gate still runs before the token gate. | blocker if absent | Fixed. Residual: `127.0.0.2:PORT` passes the gate (still needs the token); a page already at `127.0.0.1:PORT` is a02/a03's problem. | a01 |
| T02 | CSRF / simple-request POST: a page POSTs `text/plain` or form-encoded to the API without a preflight | fixed | `[test]` `internal/web/csrf_test.go` (`TestCSRFSimplePostFromWebPage`, `TestCSRFPostWithSafelistedContentType`, `TestEventSourceKeepsQueryToken`), commit `a91a2b6`. Baseline accepted `?t=` on every method and decoded the body whatever the content type; the fix accepts the query token only on GET and requires `Content-Type: application/json` on non-GET (`isJSON`, 415 otherwise). | major | Fixed. Residual: no `Origin` check on non-GET; safety now depends on the server not answering a permissive preflight, which a03's same-origin-only CORS policy keeps true. A leaked token is still the precondition (T04). | a02 |
| T03 | CORS: a page reads API responses cross-origin | fixed | `[test]` `internal/web/cors_test.go` (`TestCrossOriginReadIsDenied`, `TestSameOriginIsEchoed`, `TestPreflightFailsClosed`, `TestNoWildcardOnSimpleRequest`, `TestAPIResponseIsNotSniffed`), commit `7e14b66`. Baseline had no CORS header at all, so reads were blocked only by omission; the fix adds `cors` middleware (`server.go:258`) that emits `Vary: Origin` always, echoes `Access-Control-Allow-Origin` only for the server's own origin, fails a foreign preflight closed with 204, and adds `X-Content-Type-Options: nosniff`. | minor | None. Residual: CORS is browser-enforced, so it is not a control against a local process; the token remains that control. | a03 |
| T04 | Per-launch token in the URL: leaks via history, `Referer`, logs, screenshots | fixed (blast radius) / open (history) | `[test]` tests named in the a04 report (`TestTokenInQueryOnlyOpensTheEventStream`, `TestResponsesForbidReferer`, `TestHeaderWinsOverQuery`, `TestErrorPagesDoNotEchoTheToken`, `TestTokenBearingRepliesAreNotCached`), commit `95943c6`. The query token now opens only `GET /api/events`; every response carries `Referrer-Policy: no-referrer`; error pages and caching are pinned. | major before, minor after | Residual: the token still sits in the address bar and browser history. The one-line fix (`history.replaceState` in `app.js`) is in a frozen file, so it is a proposal for the static owner. | a04 |
| T05 | LAN mode: what a LAN peer can reach | fixed | `[test]` `internal/web/lan_test.go` (`TestEngineAPIIsLoopbackOnly`, `TestLANModeExposesModelServerOnly`, `TestLANConfigExposure`, `TestEngineAPIRejectsNonLoopbackPeer`), commit `9384fa1`. llama-server binds `0.0.0.0` with every named key only when LAN is on (`internal/serve/config.go:92-97,121`); the setup API stays on `127.0.0.1:0` (`internal/web/server.go:75`), and `hostCheck` now also requires a loopback `RemoteAddr`, so a wider bind still keeps LAN peers out. | major | Fixed. Residual policy gaps: no LAN-only key (a guest gets the same key the local apps use), `lanIP()` can name a VPN interface, and there is no firewall hint when LAN mode is on. | a05 |
| T06 | The API key is visible on the llama-server command line to other local users (`ps`) | fixed | `[test]` `internal/serve/key_cmdline_test.go` (`TestKeyNotOnLlamaServerCommandLine`, `TestKeyFileHoldsEveryKeyOnePerLine`, `TestKeyFileFallbackKeepsServerStarting`), commit `fcaa2d8`. Baseline `internal/serve/config.go:121` passed `--api-key <comma-joined keys>`; the fix writes `~/.ezlm/api-keys.txt` (0600, temp+rename, one key per line) and passes `--api-key-file` instead. | major | Fixed for the normal path. Residual: if the key file cannot be written, `Args()` falls back to `--api-key`, which is visible again; the file is rewritten on every `Args()` so rotation and revocation do take effect at the next start. | a06 |
| T07 | File permissions of `~/.ezlm` (keys, config, logs) | fixed (Unix) / open (Windows ACL) | `[test]` `internal/keys/permissions_test.go` (`TestEZLMHomeOwnerOnly`, `TestServerLogOwnerOnly`, `TestSettingsFileOwnerOnly`, `TestConfigKeyFileOwnerOnly`), commit `0a6ae76`: `~/.ezlm` is created and repaired at 0700, `server.log` 0600, `settings.json` chmodded after write, `config.json.tmp` chmodded before rename. `[code]` Windows: no `SetNamedSecurityInfo` or `icacls` anywhere, so 0600 has no ACL effect and the key is protected only by the profile's inherited ACLs (a07 F5). | major on a shared Windows PC, minor elsewhere | Unix fixed. Windows fix proposed but not applied: a `_windows.go` helper setting a protected DACL for the current user on `~/.ezlm`. Also open: `ui.url` (contains the token) keeps an older file's mode (`cmd/ezlm/main.go:298`, frozen file, a07 F6). | a07 |
| T08 | Redaction: a key or token printed in a log, report or crash dump | mitigated | `[code]` the problem report redacts every configured key, then `integrate.Redact` strips `ezlm-…`, `sk-…` and `Bearer …` (`internal/app/launch.go:230-237`, `internal/integrate/log.go:26-46`); integration logs are written already redacted. The engine API returns `err.Error()` (`server.go:268`), which carries paths, not keys. | minor | Residual: a new log line can bypass `Redact`; the regex misses a token that is not prefixed. Owner should prove with a test that a key in an error string is redacted. | a08 |
| T09 | Timing-safe key comparison | fixed | `[test]` `internal/keys` tests plus `TestSubtleEmptyGotcha`, commit `fed0be6`: `keys.Equal` centralises `subtle.ConstantTimeCompare` and `guard` uses it. a09 measured `==` at a ~1 ns first-vs-last-byte difference (1.5x at 32 bytes) and the constant-time path at 1.0x. | minor | Fixed. Note the latent empty-empty match: `subtle.ConstantTimeCompare("", "")` is 1, so a `Server` built with an empty token would accept a request with no token; `Listen` always sets one. | a09 |
| T10 | Fuzz: malformed request parsing panics or leaks a path | fixed | `[test]` `internal/web/fuzz_api_test.go` plus a seed corpus under `testdata/fuzz/`, commit `8d55ddb`. The fuzzer found that a path that is not valid UTF-8 made the embedded file server answer 500; the static handler now returns 404 for it (`server.go`, `utf8.ValidString` guard). No panic and no internal path in the responses. | minor | Fixed. Keep the seed corpus in the merge; rerun `-fuzz` for 30s after other `server.go` patches land. | a10 |
| T11 | Threat model document | fixed | `[code]` `agents/T07/a11/threat-model.md`, commit `36e0c79`: assets, entry points, attacker capabilities, STRIDE-shaped table with the current control and file:line per threat. | minor | None. This table is its summary; the document is the long form. | a11 |
| T12 | Key storage, generation, rotation, corrupt or empty file | fixed (F1-F4) / open (F5) | `[test]` `internal/keys/storage_test.go` (`TestLoadReplacesEmptyKey`, `TestLoadKeepsKeyMirrorInStep`, `TestSaveFixesLooseTempFileMode`, `TestLoadFixesLooseConfigMode`, `TestKeyReusedAcrossLaunches`, `TestRotationReplacesTheOldKey`, `TestLoadEmptyFile`), commit `1607820`. An empty key value was accepted and passed to llama-server as `--api-key `, which is a working empty credential on a LAN; `repairKeys` now generates a fresh key on load and save, keeps `Key` in step with `Keys[0]`, and tightens a loose `config.json` mode. | major | F1-F4 fixed. F5 open: a corrupt config makes the frozen deploy path (`internal/app/app.go:761` ignores `Load`'s error) silently generate a new key and leave the old server running. F6: `keys.New` panics if the OS random source fails (accepted). | a12 |
| T13 | Shared test harness | fixed | `[code]` `internal/web/authtest/` with a stub backend and request helpers, commit `7526148`; `authtest_test.go` covers token required, Host checked, unauthenticated POST rejected. Use `package web_test` in tests that import it (it imports `internal/web`, so an internal test file would cycle). | minor | None. Note for a01/a02/a14: the harness is the preferred way to build the proofs. | a13 |
| T14 | Handler review: traversal, method/content-type confusion, unbounded bodies, path leaks in errors | fixed | `[test]` `internal/web/handler_review_test.go`, commit `3012670`: `decodeJSON` caps the body with `http.MaxBytesReader` (413), requires `application/json` (415), rejects bad JSON (400), and `redactPaths` rewrites the user's home to `~` in error bodies. Static files stay inside `embed.FS` via `http.FileServerFS`. | minor | Fixed. Merge note: a02 and a14 both add a JSON content-type check to `server.go`; keep one implementation (a14's `decodeJSON`) and a02's query-token-is-GET-only change. | a14 |
| T15 | The apps' engine clients: token in URL vs header, logs, crash reports | fixed (review) | `[code]` `agents/T07/a15/clients.md`, commit `24f300c`: both apps send `X-EZLM-Token` on every call including SSE (`windows/EZLM/Engine.cs:79-80,111,118,145`, `macos/Sources/EZLM/Engine.swift:113,159`), take the token from stdout per launch, never persist it, and redact `ezlm-` keys in problem reports. No app changes were made (outside T07's paths). | minor | Worst finding in the review is minor. Nothing puts the token in a URL, on disk, or in a crash report. | a15 |

## Current status (updated as owner agents commit)

Landed on their branches so far:

| Agent | Commit | What it changes |
|---|---|---|
| a03 | `7e14b66` | `internal/web/server.go`: CORS middleware, nosniff, preflight fails closed |
| a06 | `fcaa2d8` | `internal/serve/config.go`: `--api-key-file` replaces `--api-key` |
| a07 | `0a6ae76` | `internal/paths/*`, `internal/serve/{config,run,service_windows}.go`: 0700 home, 0600 log |
| a13 | `7526148` | new `internal/web/authtest/` harness (no production file) |
| a02 | `a91a2b6` | `internal/web/server.go`: query token GET-only, JSON content type on non-GET |
| a10 | `8d55ddb` | `internal/web/server.go`: 404 on a non-UTF-8 static path; fuzz test |
| a12 | `1607820` | `internal/serve/config.go`: repair empty key, keep `Key` mirror, tighten modes |
| a14 | `3012670` | `internal/web/server.go`: body cap, 415/400/413, home-path redaction |
| a01 | `cc25fa7` | `internal/web/server.go`: loopback Host allowlist (localhost, 127/8) with tests |
| a04 | `95943c6` | `internal/web/server.go`: query token only for `/api/events`, `Referrer-Policy: no-referrer` |
| a09 | `fed0be6` | `internal/keys/keys.go`: `keys.Equal`; `guard` uses it |
| a11 | `36e0c79` | `agents/T07/a11/threat-model.md` (docs) |
| a15 | `24f300c` | `agents/T07/a15/clients.md` (docs) |
| a05 | `9384fa1` | `internal/web/server.go`: loopback `RemoteAddr` check; LAN tests |

Merge-order warning: seven branches edit `internal/web/server.go` (a01, a02, a03,
a04, a09, a10, a14), and three edit `internal/serve/config.go` (a06, a07, a12); a05 also
edits `hostCheck` in `server.go`, next to a01 and a04's changes to the same
function. The morning merge takes each branch's diff, so
overlapping hunks will conflict. Apply in sequence and re-run the full test set
after each, because one branch's diff does not contain another's lines. In
particular a02 and a14 both add a JSON content-type check; keep one.

Controls already in the baseline: the `Host` check (T01), the `subtle`
comparison (T09) and the report redaction (T08). Their owners should record
"verified, not added" with a regression test, not a fix, so the merge does not
double-apply a patch.

Still open and worth the merge owner's attention: a07's Windows ACL finding
(major on a shared Windows PC), a12's F5 (a corrupt config silently rotates the
key in the frozen deploy path), and T04's token still in browser history. a05
(LAN mode) and a08 (redaction proof) had not committed when this was last
updated; their rows are code-read only.
