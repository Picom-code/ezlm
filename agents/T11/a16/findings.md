# T11 findings: bugs found by the coverage agents

Snapshot: 2026-11-06 10:20 CST, agent a16. Sources are the `agents/T11/aXX/REPORT.md` files and
the skipped tests on each `agent/T11-aXX` branch (the branch is the patch). Where a report had not
been committed yet, the skipped test on the branch is the source and the row says so. "Test" is
the test that fails or is skipped with `t.Skip("bug: ...")`.

Severity: **blocker** (crash or wrong data on a user's machine), **major** (visible wrong
behaviour on a common path), **minor** (edge case or cosmetic), **info** (observation, no failing
test).

## Confirmed bugs

| # | Where (file:line) | Bug | Test | Severity | Owner |
|---|---|---|---|---|---|
| 1 | `internal/catalog/catalog.go:190` (`parse`) | A quant with an empty `files` list is accepted. `app.Installed` then indexes `q.Files[0]` and panics (`internal/app/app.go:1103`). | `TestParseAcceptsQuantWithoutFiles` (a01, `internal/catalog/catalog_test.go:638`, skipped) | major | T01 (catalog) |
| 2 | `internal/catalog/catalog.go:190` (`parse`) | Duplicate model ids are accepted. `Find` and the app's `Use` silently pick the first entry, so a catalog edit can shadow a model without any error. | `TestParseRejectsDuplicateIDs` (a01, `internal/catalog/catalog_test.go:654`, skipped) | minor | T01 (catalog) |
| 3 | `internal/web/server.go:94` and every POST handler (`:101,108,115,123,130,137,144,151,164,172,184,195,204,212`) | A malformed JSON body is silently decoded as zero values. `POST /api/lan` with `{"on":` returns `200 {"ok":true}` and turns LAN off; trailing garbage is accepted too. | `TestSettingsMalformedJSON` (a09, `internal/web/settings_test.go:343`, skipped); `TestMalformedJSONBody` (a10, `internal/web/server_routes_test.go:531`, skipped) | major | T07 (web) |
| 4 | `internal/web/server.go:94` (same decode-error path) | A type-mismatched field is silently dropped, so the request succeeds with the zero value: `{"on":"true"}` turns LAN off, `{"dir":"/tmp/x","move":"yes"}` applies the dir and ignores `move`. | `TestSettingsWrongFieldType` (a09, `internal/web/settings_test.go:364`, skipped) | major | T07 (web) |
| 5 | `internal/web/server.go:155` | `GET /api/update` treats any non-empty `force` value as true, so `?force=0` and `?force=false` still force a check. | `TestSettingsUpdateForceZero` (a09, `internal/web/settings_test.go:323`, skipped) | minor | T07 (web) |
| 6 | `internal/serve/config.go:77-81` (`Config.Save`) | Only `Keys[0].Key` is copied to the legacy `Key` field; an empty key stays empty. `llama-server` accepts `Bearer ` for an empty `--api-key`, so a hand-edited config leaves the local API open, including on a LAN. | `TestA02SaveReplacesEmptyKey` (a02, `internal/keys/keys_a02_test.go:289`, skipped; a02 reports it fixed on `agent/T07-api-security` with `repairKeys`) | major | T07 (serve keys) |
| 7 | `internal/serve/config.go:85-89` | `os.WriteFile` keeps the mode of an existing file, so a leftover 0644 `config.json.tmp` is renamed into place world-readable and the API key becomes readable by other local users. | `TestA02SaveTightensTempFilePermissions` (a02, `internal/keys/keys_a02_test.go:266`, skipped; fixed on `agent/T07-api-security` with an explicit chmod) | major | T07 (serve keys) |
| 8 | `internal/paths/paths.go:39-46` (`Ensure`) | `~/.ezlm` is created 0755, so other local users can list the folder that holds the key file (the file itself is 0600). | `TestA02HomePermissions` (a02, `internal/keys/keys_a02_test.go:250`, skipped; fixed on `agent/T07-api-security`, 0700) | minor | T07 (a02 reports it as T07's; `TASKS.md` does not list `internal/paths`) |
| 9 | `internal/web/server.go:88` (`GET /` static handler) | Because the static pattern catches every GET, `GET` on a POST-only route such as `/api/deploy` is served by the file server and returns 404 instead of 405 with `Allow: POST`. | `TestGetOnPostOnlyRoute` (a10, `internal/web/server_routes_test.go:381`, skipped) | minor | T07 (web) |
| 10 | `internal/web/server.go:88` | `POST` to an unknown path returns 405 (from the mux) instead of 404; only GET has a catch-all handler. | `TestPostToUnknownPath` (a10, `internal/web/server_routes_test.go:369`, skipped) | minor | T07 (web) |
| 11 | `internal/update/update.go:150` (`split`) | Build metadata is parsed as a pre-release: `split` cuts on `-+`, so `Newer("1.0.0", "1.0.0+build")` is true. SemVer ignores `+` metadata. | `TestNewerBuildMetadata/1.0.0_gt_1.0.0+build`, `TestNewerBuildMetadata/1.0.0-rc.1+build.5_gt_1.0.0-rc.1` (a06, `internal/version/version_test.go:117`, skipped) | minor | T08 (update) |
| 12 | `internal/update/update.go:144` (`Newer`) | Numeric pre-release identifiers compare as strings, so `beta.2` sorts above `beta.11` (SemVer compares dot-separated numeric identifiers numerically). | `TestNewerPreReleaseOrdering/1.0.0-beta.2_gt_1.0.0-beta.11` (a06, `internal/version/version_test.go:92`, skipped) | minor | T08 (update) |
| 13 | `internal/bench/bench.go:257` (`clip`) | `clip` cuts at a byte offset, so it can split a multibyte character and return invalid UTF-8. | `TestClipKeepsValidUTF8` (a12, `internal/bench/parse_test.go:61`, skipped) | minor | unassigned (no task owns `internal/bench`; a12 tests it) |
| 14 | `internal/bench/bench.go:208` (`jsonFormat`) | Only a lowercase ```` ```json ```` fence is stripped, so a correct answer fenced with ```` ```JSON ```` is left wrapped. | `TestJSONFormatUppercaseFence` (a12, `internal/bench/parse_test.go:263`, skipped) | minor | unassigned (no task owns `internal/bench`; a12 tests it) |
| 15 | `internal/engine/download_stall_test.go:21` and `:69` (baseline) | Test-only data race: both stall tests write the package global `stallTimeout` while a watchdog goroutine from an earlier download reads it (`internal/engine/download.go:109`). Found with `-count=20 -race` (2 failures in 20). | `TestDownloadResumesAfterStall`, `TestDownloadResumesAfterStallHTTP2` (a13; **fixed** in the test by setting it once in `TestMain`, verified `-race -count=20 -shuffle=1793981251028295000`) | minor | T11 a13 (fix is in the test; production code unchanged) |

Rows 1-5 and 9-14 are skipped tests on the agent branches (9-10 were read from a10's worktree
before its report landed; re-check `agent/T11-a10` for the committed test names). Rows 6-8 are
fixed on `agent/T07-api-security` but remain skipped on the T11 branches. Row 15 is fixed in a13.

## Observations without a failing test

Reported by the coverage agents (or found by a16 while reading the sources), with no deterministic
failing test, so listed separately rather than as confirmed bugs.

| # | Where (file:line) | Observation | Source | Severity | Owner |
|---|---|---|---|---|---|
| 16 | `internal/paths/settings.go:49-61` | `RememberModelsDir` locks inside `LoadSettings` and again inside `SaveSettings`, so the read-modify-write is not atomic; two concurrent calls can lose one `known_dirs` entry. | a03 | minor | unassigned (`internal/paths`) |
| 17 | `internal/paths/settings.go:55` | `EqualFold` dedupe treats `/models` and `/MODELS` as the same folder. Correct on macOS and Windows, wrong on a case-sensitive Linux filesystem. | a03 | minor | unassigned (`internal/paths`) |
| 18 | `internal/paths/settings.go:32-34` | The `json.Unmarshal` error is ignored, so a malformed settings file can load partially (`{"models_dir":"/m","known_dirs":123}` keeps `ModelsDir`). May be deliberate tolerance. | a03 | minor | unassigned (`internal/paths`) |
| 19 | `internal/web/server.go:250-253` | The token is accepted from the query string on every route, though the page only needs that for `/api/events`; the token therefore appears in browser history and same-origin `Referer` headers. | a08 | minor | T07 (web) |
| 20 | `internal/web/server.go:79-81` | `Listen` ignores the `rand.Read` error; if the entropy source failed, the token would be 32 zero characters. No seam to test it. | a08 | minor | T07 (web) |
| 21 | `internal/web/server.go:255` | 401 replies are plain text, not JSON; the page currently falls back to a status message, so this only breaks if the page starts requiring JSON errors. | a08 | info | T07 (web) |
| 22 | `internal/web/server.go:250` | An empty `X-EZLM-Token` header is treated as absent and falls back to the query token. Harmless today (no caller sends the header). | a08 | info | T07 (web) |
| 23 | `internal/prereq/prereq.go:141-145` | `nvidiaDriver` returns a `Check` with the zero `Status` (`""`) when the backend is CUDA but `GPUs` is empty; UI code that switches on ok/fix/warn/fail has no case for it. Only reachable from a hand-built `Hardware`. | a04 | minor | unassigned (`internal/prereq`) |
| 24 | `internal/prereq/prereq.go:109-118` | An unreadable `sw_vers` is treated as too old: `atLeast("", "13.3")` is false, so the check fails with "Update to macOS 13.3". Only reachable if the command is blocked or its output changes. | a04 | minor | unassigned (`internal/prereq`) |
| 25 | `internal/prereq/prereq.go:121-123` | `glibc` reads only the first line of `ldd --version`. Correct for glibc; musl prints the version on line 2 and is reported as warn, which is accurate enough. | a04 | info | unassigned (`internal/prereq`) |
| 26 | `internal/proc/proc.go:56` (`runTree`) | Starts `cmd` even when the caller's `ctx` is already done (checked only after `Start`), unlike `exec.CommandContext`; the process is killed immediately, so the cost is a wasted fork/exec. | a15 | minor | T10 (proc) |
| 27 | `internal/catalog/catalog.json` (`gemma-4-31b`) | `QAT-Q4_0` (17,651,001,568 B) is listed before the smaller `QAT-UD-Q4_K_XL` (17,287,670,048 B); 5 of 32 models are not in ascending quant-size order (the others group by quant family). Selection scans all quants and ranks by effective quality, so the order is cosmetic, but T01's planned "quants sorted" consistency test will flag it. | a16 direct review | minor | T01 (catalog data) |
| 28 | `internal/serve/props_test.go:16,19` | `TestFetchProps` read the handler's `auth`/`body` variables without synchronisation. Never failed, but a memory-model data race. **Fixed** in the test with a mutex (a14). | a14 | minor | T11 a14 (test) |
| 29 | `internal/serve/stopped_test.go:28-29,51-52` | `stoppedGrace`/`stoppedChecks` package globals are saved and restored by two tests; correct only while nothing calls `t.Parallel()`. A trap for a future edit, not a present flake. | a14 | minor | T11 a14 (test) |
| 30 | `internal/serve/stopped_test.go:22-36` | `TestWaitHealthyNoticesAStoppedServer` reserves a port then assumes nothing binds it; if something does, the test waits out the one-minute timeout instead of failing fast. Window is tiny. | a14 | minor | T11 a14 (test) |
| 31 | `internal/serve/port_test.go:20-30` | `TestPortFreeSeesListener` requires a specific ephemeral port to stay free after `Close()`; a concurrent binder on the host makes it fail. Not observed in 100 runs. | a14 | minor | T11 a14 (test) |
| 32 | `internal/serve/startlog_windows_test.go:26` | Windows-only test returns before `cmd.exe` exits, so the child can still hold the log open when `t.TempDir()` cleanup runs; also `t.Fatal`s when `ComSpec` is unset. Unverifiable on darwin. | a14 | info | T11 a14 (test) |
| 33 | `internal/catalog/catalog.go:156-180` (`Load`) | `cacheMu` is held across the 4 s remote fetch, so concurrent `Load` calls serialize behind a slow network. Latency only. | a01 | info | T01 (catalog) |
| 34 | `internal/catalog/catalog.go:174` vs `:192` | The remote path decodes with `json.Decoder.Decode` (accepts trailing bytes) while the local and embedded paths use `json.Unmarshal` (rejects them). Lenient, not a bug today. | a01 | info | T01 (catalog) |
| 35 | `internal/web/server.go:94` (all POST handlers) | Request bodies have no size limit; a very large body is read into memory. | a10 | info | T07 (web) |
| 36 | `internal/web/server.go:275-303` (`events`) | No cap on concurrent `/api/events` streams; a browser refresh leaves the old stream until its context is cancelled. | a10 | info | T07 (web) |


## Agents with no bugs

- a05 (`internal/report`): none. `Send`/`NewIssueURL` behaved as documented; open questions only
  (trimming 10% per step is coarser than needed; a 201 with an unreadable body reports the browser
  fallback, so a report could be filed twice).
- a07 (`internal/web` status/live): none. The only note is that the 403 from `hostCheck` carries
  no `Cache-Control`/`X-Frame-Options` (not a bug).
- a11 (coverage): none; baseline coverage only.
- a15 (`internal/awake`, `internal/proc`): none; only the `runTree` observation above.

## Coverage context

a11's `agents/T11/a11/coverage.md` has the before/after numbers: the engine starts at 52.6%
statements; T11's seven packages hold 340 statements, and the total is expected to move to
roughly 57% once a01-a10 land.
