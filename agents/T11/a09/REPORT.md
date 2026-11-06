# T11 / a09: `internal/web` settings and config routes

Branch `agent/T11-a09`, worktree `~/ezlm-agents/work/subs/T11/a09`.
Test file: `internal/web/settings_test.go` (new file only, no logic changes).

## Scope

The routes that change configuration, in `internal/web/server.go`:

| Route | Backend call | Lines |
|---|---|---|
| `POST /api/lan` | `SetLAN(on)` | 111-117 |
| `GET /api/storage` | `Storage` | 158 |
| `POST /api/storage` | `SetStorage(dir, move)` | 159-166 |
| `POST /api/integrate` | `Integrate(ids, default)` | 118-125 |
| `GET /api/update?force=` | `UpdateInfo(force)` | 154-156 |

Covered: valid bodies, missing fields, extra/unknown fields, unicode values, backend
errors (400 envelope), response headers, and the shared error envelope in `api()`.

## Coverage

`go test -cover ./internal/web`

- before: `0.0% of statements` (no tests existed)
- after: `49.0% of statements`

The remaining statements are mostly the other route closures (owned by a07/a08/a10),
the SSE loop in `events` and `Listen`/`hostCheck` branches.

## Method note

The route table is built inside `Serve`, so the tests drive the real mux over a
loopback `Listen` (ephemeral 127.0.0.1 port, closed in `t.Cleanup`). This needs no app,
model or GPU. One test (`TestSettingsAPIErrorEnvelope`) uses `httptest.NewRecorder`
directly against `s.api`. A fake `Backend` (`settingsFake`, prefixed to avoid colliding
with other T11 test files in this package) records every settings call.

## Findings

### 1. Malformed JSON bodies are silently accepted (major, web)

`internal/web/server.go:94,101,108,115,123,130,137,144,151,164,172,184,195,204,212`
- every handler does `json.NewDecoder(r.Body).Decode(&in)` and ignores the error.

Evidence: `TestSettingsMalformedJSON` (skipped). Body `{"on":` to `POST /api/lan`
returns `200 {"ok":true}` and calls `SetLAN(false)`, i.e. a truncated request quietly
turns LAN off. Same for `/api/storage` and `/api/integrate`. Trailing garbage
(`{"on":true} x`) is also accepted.

Fix direction: return 400 `{"error":"invalid json"}` when `Decode` fails, and reject a
second JSON value.

### 2. Type-mismatched fields are dropped, zero values applied (major, web)

Same decode-error path. `POST /api/lan` with `{"on":"true"}` returns 200 and sets LAN
off; `POST /api/storage` with `{"dir":"/tmp/x","move":"yes"}` returns 200 and applies
the dir but silently ignores `move` (partial application). Evidence:
`TestSettingsWrongFieldType` (skipped).

### 3. `force=0` is treated as true (minor, web)

`internal/web/server.go:155` uses `r.URL.Query().Get("force") != ""`, so any value
including `0`/`false` forces an update check. Evidence: `TestSettingsUpdateForceZero`
(skipped). Fix direction: parse with `strconv.ParseBool` or accept only `1`/`true`.

## Open questions

- Missing body on `POST /api/lan` calls `SetLAN(false)`. That is defensible ("off" is
  the zero value) but means an empty POST from a stale client turns LAN off. Documented
  as current behaviour in `TestSettingsLANPost/missing field`; not filed as a bug.
- `POST /api/storage` with an empty dir is passed to `app.SetStorage`, which rejects it
  with "pick a folder or drive" (400). Handler-level validation would give a clearer
  error; left to the owner.

## Checks

`gofmt -l .` (empty for the new file), `go vet ./internal/web`,
`go test -p 2 ./internal/web` all pass. Skipped tests are the bug deliverables above.
