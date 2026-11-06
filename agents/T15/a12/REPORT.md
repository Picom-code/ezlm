# T15 a12 — Go path handling on Windows

Slice: `filepath.Join` vs string concat, `os.UserHomeDir`, `%LOCALAPPDATA%` vs
`%APPDATA%`, temp dirs, and `internal/paths` on Windows. No Windows machine
was available: every claim below is either a `GOOS=windows` test that will run
on Windows or a code read, marked as such.

## What I changed

1. `internal/paths/paths.go` — `Home()` no longer falls back to a **relative**
   `./.ezlm` on Windows when `%USERPROFILE%` is missing. It now uses
   `%LOCALAPPDATA%\EZLM` (then `%APPDATA%\EZLM`), so a process started from a
   stripped environment cannot write EZLM's config, models and logs into
   `C:\Windows\System32\.ezlm` (the usual CWD of a Run-key or service launch).
2. `internal/paths/paths.go` — `EZLM_HOME` is passed through `filepath.Clean`,
   so `EZLM_HOME=C:\x\ezlm\` and `EZLM_HOME=C:\x\ezlm` are the same folder for
   every caller that compares `Home()` output as a string (`isOwnExe`,
   `dshDefaultHome`, `legacyTasks` gating). Child paths were already joined
   through `filepath.Join`, so this is the only place the raw value leaked.
3. `internal/paths/paths_windows_test.go` — three Windows-only tests:
   the fallback is absolute and under `%LOCALAPPDATA%`; the normal case is
   still `%USERPROFILE%\.ezlm`; the override is cleaned. They build on macOS
   (`GOOS=windows go vet`) and run on Windows.

Why these are real: on Windows `os.UserHomeDir()` is `%USERPROFILE%` only
(`os/file.go`, Go 1.27), with no `%HOMEDRIVE%`/`%HOMEPATH%` fallback. A
`Process.Start` with `UseShellExecute=false`, a `schtasks` legacy task, or a
Run-key launch can hand the engine an environment without it; the old `"."`
fallback then depends on the CWD, which EZLM never sets. Severity minor
(needs an unusual environment), but the fix is two lines and removes a class
of "my models disappeared" reports.

## Findings (ranked)

### MAJOR (code read; frozen area, not patched)

**1. Report redaction misses Windows spellings of the home folder.**
`internal/app/launch.go:238` replaces only the literal `os.UserHomeDir()`
string before posting a beta report to a public GitHub issue
(`internal/report/report.go:51`). On Windows the same folder appears as
`C:\Users\Name`, `C:/Users/Name` (Git Bash, `Resolve-Path`), `c:\users\name`
(lowercased output), or `\\?\C:\Users\Name` (long-path output). None of those
match `C:\Users\Name`, so the Windows user name (and often a real name) leaks
into a public issue. Proposed fix: on Windows replace, case-insensitively,
both `home` and `filepath.ToSlash(home)`, and strip a leading `\\?\`.
Evidence: code read; a test needs a Windows machine because the leak depends
on what the child tools print.

### MINOR (code read; frozen area, not patched)

**2. `checkFolder` rejects every folder whose path starts with `%SystemRoot%`.**
`internal/app/settings.go:259` compares with `strings.HasPrefix`, so
`C:\WindowsApps` and `C:\Windows.old` are refused as "the Windows folder".
Proposed fix: require the separator after the root (`C:\Windows\`) or compare
whole path components, as `within` does.

**3. Path equality is case-sensitive on Windows.**
`internal/app/settings.go:150`, `internal/app/app.go:905`, `:917`, `:1106`
compare `filepath.Clean(a) == filepath.Clean(b)`. A config written as
`c:\Users\...` and a path returned by `filepath.WalkDir` as `C:\Users\...`
compare unequal, so a running setup is reported as "not running" and a model
file as "elsewhere". `paths.RememberModelsDir` already uses `EqualFold`;
proposed fix: one `samePath` helper (EqualFold on Windows/darwin, `==`
elsewhere) used at those four sites.

**4. Drive-relative PATH entries are joined onto the profile.**
`internal/integrate/findtool.go:92`: `!filepath.IsAbs(d)` is true for `C:tools`
(drive-relative, no separator), so it becomes
`C:\Users\Name\C:tools` — a path that cannot exist. `%PATH%` usually holds
absolute entries, but hand-edited ones are common. Proposed fix: leave entries
with `filepath.VolumeName(d) != ""` alone and resolve them with
`filepath.Abs`.

**5. npm-installed OpenCode under `%LOCALAPPDATA%\npm` is misclassified.**
`internal/integrate/opencode_detect.go:232` matches only
`/appdata/roaming/npm/`. npm's Windows prefix can be `%LOCALAPPDATA%\npm`
(or a custom prefix); those copies fall through to `SourcePath`, which changes
the "your own OpenCode is too old" messaging. Low confidence: the default
prefix is `%APPDATA%\npm`, which matches. Proposed fix: also match
`/appdata/local/npm/` and any `/npm/node_modules/` path.

**6. Update staging folder is never cleaned.**
`internal/update/update.go:112` keeps every downloaded installer in
`%TEMP%\EZLM-update` forever, and runs the new build from `%TEMP%` (Defender
and zone-identifier territory, see a08). Proposed fix: remove older files in
that folder before downloading, and delete the staged file after the update.

### INFO / open questions

**7. Two data roots on Windows.** The C# app keeps logs and chats in
`%LOCALAPPDATA%\EZLM` (`windows/EZLM/Engine.cs:29`) while the Go engine keeps
config, models, keys and logs in `%USERPROFILE%\.ezlm`
(`internal/paths/paths.go:18`). Not a bug, but uninstall must remove both, and
support needs to know which is which. (C#, read only.)

**8. `paths.Models()` does not create the folder it returns.** `Ensure` does,
but `SetModels` can point at a drive that is not mounted; callers that use
`Models()` directly get a path under a missing drive. Windows drive letters
move more often than mount points, so a check on the chosen drive would help.
Not patched: the download path already creates the folder.

**9. OneDrive-redirected profiles.** `%USERPROFILE%` may be under OneDrive,
and `paths.Home()` follows it, so models (tens of GB) can be synced. This is
a02's slice; flagging that the default is not `%LOCALAPPDATA%` on purpose (the
C# app does not know about `.ezlm`), so moving it is a coordinated change.

## What I could not verify

- No Windows machine: the tests build under `GOOS=windows` and must be run
  there. `go test` here runs the macOS build of the `paths` package.
- The redaction leak (finding 1) needs a Windows run with a tool that prints
  its own home path in a different spelling.

## Checks run

`gofmt -l .` (empty), `go build ./...`, `go vet ./...`,
`GOOS=windows GOARCH=amd64 go build ./...`, `GOOS=windows GOARCH=arm64 go build ./...`,
`go test -p 2 ./internal/paths/`.
