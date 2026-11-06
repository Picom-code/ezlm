# T11 / a03: `internal/paths` tests

## Scope

New tests only: `internal/paths/paths_test.go` (one file, per the brief). No non-test files were
edited.

## What is covered

Table tests and edge cases for every exported helper in `internal/paths`:

- `Home` (`paths.go:10`): `EZLM_HOME` override returned verbatim (plain, trailing separator,
  spaces, unicode, Windows-style `C:\...`, long path); fallback to `os.UserHomeDir` joined with
  `.ezlm`; fallback to `.` when no home variable is set.
- `Bin`, `Models` default, `Logs`, `Config`, `SettingsPath` (`paths.go:21-36`,
  `settings.go:26`): derived with `filepath.Join`, so expectations follow the host separator and
  stay valid on Windows. A trailing separator on `EZLM_HOME` is cleaned by the join.
- `SetModels`/`Models` (`paths.go:24-34`): default, custom override, Windows-style backslash
  override preserved verbatim on any OS, reset to default.
- `Ensure` (`paths.go:39`): creates the full tree from a missing nested home, creates a custom
  models dir instead of `Home()/models`, creates unicode and long (200-char segment) paths, 0755
  on the bin dir, and returns an error when the home path is a regular file.
- `SaveSettings`/`LoadSettings` (`settings.go:28-46`): round trip with unicode and spaces, file is
  valid JSON, newline-terminated, mode 0600; creates a missing home; error when home is a file;
  missing file, empty file, BOM-prefixed file, corrupt JSON, unknown fields and known dirs all
  load without panic.
- `RememberModelsDir` (`settings.go:49`): empty dir is a no-op that writes no file, paths are
  cleaned, exact and case-insensitive duplicates are not appended twice, other settings fields
  survive.
- `DeployedModelsDir` (`settings.go:65`): missing config, plain value, BOM, corrupt JSON, unknown
  fields and empty value.

All tests are hermetic: `t.TempDir()` plus `t.Setenv("EZLM_HOME", ...)` and a temp `HOME`/
`USERPROFILE`; nothing touches the real home. Package-level state (`modelsDir`) is restored with
`t.Cleanup`, and tests are not parallel.

## Coverage

| Package | Before | After |
|---|---|---|
| `internal/paths` | 0.0% of statements | 100.0% of statements |

`go test -cover ./internal/paths/` and `go tool cover -func` show every function at 100.0%
(`Home`, `Bin`, `Models`, `Logs`, `Config`, `Ensure`, `SettingsPath`, `LoadSettings`,
`SaveSettings`, `RememberModelsDir`, `DeployedModelsDir`).

## Checks run

- `gofmt -l .` empty, `go build ./...`, `go vet ./...`, `go test -p 2 -count=1 ./internal/paths/`
- `GOOS=windows GOARCH=amd64 go build ./...`, `GOOS=windows GOARCH=arm64 go build ./...`,
  `GOOS=linux go build ./...`

## Bugs found

None that could be proven with a deterministic failing test, so there are no `t.Skip("bug: ...")`
tests. Three observations for the owner, none of them a coverage gap:

1. **Lost update under concurrency** (`settings.go:49-61`): `RememberModelsDir` takes the lock
   twice (inside `LoadSettings` and inside `SaveSettings`), so two goroutines adding a folder at
   the same time can overwrite each other's `known_dirs` entry. The mutex does not span the
   read-modify-write. Severity: minor (the app appears to call it from one path at a time). A
   deterministic test would need a hook between load and save; not added because logic changes
   are out of scope.
2. **Case-insensitive dedupe on case-sensitive file systems** (`settings.go:55`): `EqualFold`
   treats `/models` and `/MODELS` as the same folder, which is right on macOS/Windows but wrong on
   Linux, where they are distinct directories. Severity: minor, platform-specific.
3. **Corrupt settings can load partially** (`settings.go:32-34`): the `json.Unmarshal` error is
   ignored, so a file like `{"models_dir":"/m","known_dirs":123}` returns `ModelsDir="/m"` even
   though the document is malformed. Severity: minor; the tolerant behavior may be deliberate.

## Open questions

- Is `EZLM_HOME` expected to be cleaned (e.g. `filepath.Clean`) when returned by `Home`? Today it
  is returned verbatim; derived paths clean it through `filepath.Join`. Tests assert the current
  behavior.
- `Home` on Windows relies on `os.UserHomeDir` (i.e. `USERPROFILE`), so a process with only
  `HOME` set falls back to `.ezlm` there. Not testable on this macOS host; noted for T07.

## Next steps

- If the concurrency observation matters, T07 (or the paths owner) can widen the lock in
  `RememberModelsDir` and unskip a concurrency test.
- No other work is needed for this slice: the package is at 100% statement coverage.
