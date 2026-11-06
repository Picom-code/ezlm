# T07 a12: key storage and rotation

## Scope

`internal/keys/**` holds generation only; the storage, permissions, rotation and reuse of keys
live in `internal/serve/config.go` (`serve.Config` = `~/.ezlm/config.json`) and the rotation
callers in `internal/app/app.go` (frozen, read only). This slice reviews the key parts of both and
puts every test in `internal/keys/storage_test.go` (external `keys_test` package, which imports
`serve`; there is no import cycle because `serve` does not import `keys` before this patch and the
test package is external).

All tests use a temp `EZLM_HOME` and never touch the real home.

## What was already right

- `keys.New()`: `crypto/rand` per character over 62 symbols, 40 characters after `ezlm-`
  (~238 bits). No `math/rand`, no timestamp, no counter. Tested for prefix, length, alphabet and
  uniqueness over 500 keys (`TestNewKeyFormat`).
- A key survives launches: the deploy path copies `old.Key`, `old.Keys`, `old.Port` and `old.LAN`
  into the new config (`internal/app/app.go:771`), and `Save` writes the file atomically
  (`config.json.tmp` + rename) with mode 0600 (`internal/serve/config.go:109`). Tested over three
  save/load cycles (`TestKeyReusedAcrossLaunches`).
- The pre-named-keys format (one `key`, no `keys`) is migrated on load (`config.go:74`). Tested
  (`TestLoadMigratesUnnamedKey`).
- A BOM from Notepad or PowerShell is tolerated (`config.go:69`). Tested (`TestLoadToleratesBOM`).
- Rotation at storage level replaces the value and drops the old one from the next server start:
  `RotateKey` (`app.go:1233`) changes `Keys[i].Key`, `editKeys` saves and restarts. Tested
  (`TestRotationReplacesTheOldKey`).

## Findings

### F1. An empty key in the file is accepted, and llama-server treats it as a working token (major, fixed)

`Load` accepted `{"keys":[{"key":""}]}` unchanged, and `keyList` (`config.go:198`) passed
`--api-key ""` to llama-server. An empty key means the accepted credential is the empty string:
`Authorization: Bearer ` gets in. On a LAN (`Config.LAN`, host `0.0.0.0`) any peer can send that
header, and a browser page can send it too. The value can be emptied by hand-editing (the file is
plain JSON with `"key"` in it), by a partial copy between machines, or by a script.

Evidence: `TestLoadReplacesEmptyKey` failed before the fix with `Key="" Keys=[{k1 Default ...}]`.
Fix: `Config.repairKeys` (`config.go:86`) replaces a blank key value with a fresh `keys.New()` on
both `Load` and `Save`.

### F2. `Config.Key` and `Config.Keys[0].Key` could disagree after load (minor, fixed)

`Keys` is what llama-server is started with (`keyList`); `Key` is the single value EZLM writes
into OpenCode and Hermes (`app.endpointFor`, `internal/app/connect.go:17`). `Save` made `Key`
mirror `Keys[0]`, but `Load` did not, so a file with the two out of step gave connected apps a key
the server rejects. Not a leak, a breakage. Evidence: `TestLoadKeepsKeyMirrorInStep` failed with
the stale value. Fix: the same `repairKeys` sets `c.Key = c.Keys[0].Key` on load.

### F3. A stale `config.json.tmp` could make `config.json` group/world readable (minor, fixed)

`os.WriteFile` does not change the mode of a file that already exists. If a `config.json.tmp`
survived an earlier interrupted save with 0644 (or was copied there), the next save renamed it over
`config.json`, key included, with 0644. Evidence: `TestSaveFixesLooseTempFileMode` failed with
`config.json is 0644`. Fix: `os.Chmod(tmp, 0o600)` before the rename (`config.go:122`).

### F4. A config copied in with loose mode was never tightened (minor, fixed)

Same key, different route: `config.json` copied from another machine or written by an older build
at 0644 stayed readable until the next save. Fix: `tightenConfigMode` (`config.go:100`) chmods a
loose file to 0600 on load (best effort, Unix only; Windows uses ACLs). Evidence:
`TestLoadFixesLooseConfigMode` failed with 0644 before.

### F5. A corrupt or empty config rotates the key silently (minor, open; code is in `internal/app`, frozen)

`Load` reports a damaged file (`config.go:72`, tested by `TestLoadEmptyFile`), but deploy ignores
the error (`internal/app/app.go:761`, `old, _ := serve.Load()`), so `old` is nil: no old key is
kept, no `StopAndWait(old)` runs, and a fresh key is generated. The user's apps are rewritten at
the end of a successful deploy (`rewriteLinked`), so they recover, but the old key is gone and an
old llama-server started with it can be left running on its old port. Proposed fix (app owner):
keep a `key` fallback in a separate file, or treat a damaged config as "stop the old server, then
generate a new key", and say so in the deploy step. Not fixed here because `internal/app` is
frozen for T07.

### F6. `keys.New` panics if the OS random source fails (minor, note)

`keys.go:18` panics on `rand.Int` error. That is acceptable for a credential (better than a weak
key), but it is a panic in library code reached from `Load` now that `repairKeys` calls it. No test
because the error is not injectable without a package-level rand seam; if that is wanted, a
`var read = rand.Reader` seam is the minimal change.

## Not changed, noted for other agents

- `paths.Ensure` creates `~/.ezlm` with 0755 (`internal/paths/paths.go:39`). `config.json` is
  0600, so other users can see the directory and file names but not the key. a07 owns the
  directory/ACL question.
- Keys are still passed to llama-server on the command line (`config.go:157`, `--api-key`), visible
  in `ps`. a06 owns that.
- Rotation has an inherent window: the old server keeps accepting the old key until `editKeys`
  restarts it. That is seconds and only on the local machine; not a finding.

## Tests

`internal/keys/storage_test.go`: `TestNewKeyFormat`, `TestKeyReusedAcrossLaunches`,
`TestLoadKeepsKeyMirrorInStep`, `TestLoadReplacesEmptyKey`, `TestSaveRepairsEmptyKey`,
`TestSaveFixesLooseTempFileMode`, `TestLoadFixesLooseConfigMode`, `TestLoadToleratesBOM`,
`TestLoadMigratesUnnamedKey`, `TestRotationReplacesTheOldKey`, `TestLoadEmptyFile` (table:
empty, whitespace, truncated, not JSON). The five tests for F1-F4 fail on the baseline commit and
pass after the fix.

Commands run: `gofmt -l .` (empty), `go build ./...`, `go vet ./internal/keys/ ./internal/serve/`,
`go test -p 2 ./internal/keys/ ./internal/serve/ ./internal/app/`.

## Open questions

1. Should `Load` also generate a default key for a valid-but-empty `{}` config (today it has no
   keys and `Args` still yields `--api-key ""`; the deploy path regenerates, but `Start server`
   on that file would not)? Left alone because `{}` also has no port and no model, and the deploy
   path already repairs it.
2. Should a blank key name/ID be repaired too? Not security-relevant; skipped.
