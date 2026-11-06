# T11 a02: `internal/keys` tests

Branch `agent/T11-a02`. Test-only change: `internal/keys/keys_a02_test.go` (`package keys_test`). No logic changes.

## What I covered

`internal/keys` itself only generates values (`keys.New`). The key store is
`serve.Config` (`~/.ezlm/config.json`), so the lifecycle tests live in the same
test file and exercise the store through its public API. The `a02` prefix keeps
every helper and test name distinct from the key tests T07 adds in this package.

- **Generation:** `ezlm-` prefix, 40 base62 characters, 1000 distinct values,
  every base62 character reachable.
- **Storage:** save/load round trip of two keys with ID, name, value and
  `Created`; `Config.Key` mirrors `Keys[0]`; migration of a pre-named-keys
  config (`"key"` only) into the default key.
- **Permissions:** `config.json` is 0600 (Windows skipped, mode bits are ACLs);
  `~/.ezlm` should be 0700 (bug, below).
- **Rotation:** replacing one key's value updates the mirror, keeps the other
  key and its metadata, removes the old value from the file and writes the new
  one.
- **Corrupt/empty file:** missing file (`os.ErrNotExist`), empty, whitespace,
  truncated, non-JSON, `keys` wrong type, `port` wrong type. Each error names
  the file and says it is damaged. UTF-8 BOM is tolerated.
- **Unicode:** names and IDs in French, Japanese, Russian, emoji and combining
  marks; a hand-edited non-ASCII key value round-trips.
- **Error path:** `Save` fails when `EZLM_HOME` points at a file.

## Coverage

| package | before | after |
|---|---|---|
| `internal/keys` | 0.0% (no test files) | 87.5% |

The one uncovered statement is the `panic` on `crypto/rand` failure in
`keys.New` (`internal/keys/keys.go:14-15`); it cannot be reached without
changing the logic (no injectable reader). `internal/serve` coverage is
unchanged by this file (38.2% before and after) because coverage is measured
per test binary and this test file lives in `internal/keys`.

## Bugs found

All three are in `internal/serve/config.go` / `internal/paths/paths.go`, which
T07 owns. Each test asserts the correct behaviour and skips with
`t.Skipf("bug: ...")` on the baseline; after T07's fixes land the same test
runs and passes instead of skipping.

1. **Empty key is persisted** (`major`). `serve.Config.Save`
   (`internal/serve/config.go:77-81`) only copies `Keys[0].Key`; an empty
   value stays empty. `llama-server` accepts `Bearer ` for an empty
   `--api-key`, so a config built or hand-edited with an empty key leaves the
   local API open, including on a LAN. Test: `TestA02SaveReplacesEmptyKey`.
   Fixed on `agent/T07-api-security` by `repairKeys`.
2. **Leftover `config.json.tmp` decides the final mode** (`major` for a
   shared machine). `os.WriteFile` keeps the mode of an existing file
   (`internal/serve/config.go:86`), so a 0644 `.tmp` left by a crash or a copy
   is renamed into place world-readable. Test:
   `TestA02SaveTightensTempFilePermissions`. Fixed on `agent/T07-api-security`
   with an explicit `chmod` before the rename.
3. **`~/.ezlm` is created 0755** (`minor`). `paths.Ensure`
   (`internal/paths/paths.go:39-46`) lets other local users list and traverse
   the folder. The key file itself is 0600, so the key is not readable, but
   the folder should be owner-only. Test: `TestA02HomePermissions`. Fixed on
   `agent/T07-api-security` (0700).

## Open questions

- The brief lists storage, permissions and rotation under `internal/keys`, but
  that package has no storage code in this snapshot. I kept the file inside the
  owned path (`internal/keys/keys_a02_test.go`) and imported `serve` there
  rather than adding a test file in `internal/serve`, which the merge would
  drop as outside T11's owned paths.
- T07's branch adds `internal/keys/storage_test.go` and
  `internal/keys/permissions_test.go` with overlapping coverage. All my names
  are `a02`-prefixed so the two branches merge without duplicate declarations.

## Checks

`gofmt -l .` empty, `go build ./...`, `go vet ./...`,
`go test -p 2 ./internal/keys`, plus the Windows and Linux cross builds.
