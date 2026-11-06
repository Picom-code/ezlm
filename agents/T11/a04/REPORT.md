# T11 a04: internal/prereq tests

Package: `internal/prereq` (tool detection, version parsing, hide helpers).
Branch: `agent/T11-a04`. Test files added (no logic changes):

- `internal/prereq/prereq_a04_test.go`
- `internal/prereq/prereq_a04_windows_test.go` (build tag `windows`; compile-checked with `GOOS=windows go vet`)

## Coverage

| | statements |
|---|---|
| before | 0.0% (no tests existed) |
| after | 95.3% |

Uncovered and why:

- `Run` (75%): the `windows` and `linux` switch arms. Tests run on darwin; the darwin arm and the CUDA nvidia append are covered.
- `vcArch` (66.7%): the `x64` return. Tests run on arm64.
- `FixVCRedist` (87.5%): the `exit 3010` / `1638` branch. Unix truncates process exit codes to 8 bits, so a fake installer cannot produce 3010 or 1638 here; the subtests skip on non-Windows.
- `hide_other.go` `hide` reports 0% because the no-op has zero statements; the profile block count shows it was called.

## What is covered

- `atLeast`: equal, newer, older, numeric (not lexical) compare, missing components, empty and non-numeric input.
- `vcArch` / `VCRedistURL`: arch selection, https/aka.ms link, `.exe` suffix.
- `vcRedist`: current, newer, older than `vcMin`, missing registry key, output without a `v` prefix; Fix gets an Action and every check gets the redist link.
- `macOS`: supported, newer, too old, unreadable `sw_vers`.
- `glibc`: new enough for every build, old for arm64/CUDA only, old for CUDA, unreadable, version only on a later line (first-line parsing).
- `nvidiaDriver`: no GPUs, missing CUDA version, old driver, fine driver, Blackwell needing CUDA 13, Blackwell on 13.0, RTX Spark needing 616.41, Spark satisfied.
- `NeedsCUDA13`: each keyword, case-insensitivity, negatives, multi-GPU.
- `Run`: picks the OS check and appends the nvidia check on CUDA; every returned check has a status.
- `FixVCRedist`: success, installer failure, missing installer, download error (URL is the redist URL, error is propagated).
- `hide`: no panic off Windows; on Windows `HideWindow` and `CREATE_NO_WINDOW` flags.

Fake tools are shell scripts in `t.TempDir()` prepended to `PATH` via `t.Setenv`; no network, no real home. `FixVCRedist` writes its fake installer to `t.TempDir()`.

## Findings (minor, not skipped; no failing test because no proven user impact)

1. **`nvidiaDriver` returns a Check with an empty Status when `GPUs` is empty** (`internal/prereq/prereq.go:141-145`). `Run` appends this check whenever `h.Backend == scan.CUDA` (`prereq.go:47-49`), so a scan that sets CUDA but yields no GPUs produces `{"id":"nvidia","status":""}`. UI code that switches on `ok/fix/warn/fail` has no case for it. Documented by `TestNvidiaDriver/no_GPUs`. Suggested fix for the owner: return `OK` with a "No NVIDIA GPU detected" detail, or skip the check.
2. **`macOS` treats an unreadable `sw_vers` as too old** (`prereq.go:109-118`): `atLeast("", "13.3")` is false, so a failed command yields `fail` with "Update to macOS 13.3". `sw_vers` always exists on macOS, so this is only reachable if the command is blocked or output changes. Documented by `TestMacOS/unreadable`.
3. **`glibc` only reads the first line of `ldd --version`** (`prereq.go:121-123`). Correct for glibc, which prints the version on line 1; musl prints it on line 2 and is reported as `warn` "Couldn't read the glibc version", which is accurate enough. Documented by `TestGlibc/version_only_on_a_later_line`.

No `t.Skip("bug: ...")` tests: none of the above is proven to break a user path.

## Verification

- `gofmt -l .`: empty
- `go build ./...`, `go vet ./...`: pass
- `go test -p 2 ./internal/prereq/`: pass
- `GOOS=windows GOARCH=amd64 go build ./...`, `GOOS=windows GOARCH=arm64 go build ./...`, `GOOS=linux go build ./...`: pass
- `GOOS=windows go vet ./internal/prereq/`: pass (compiles the Windows-only test)

## Open questions

- Should `nvidiaDriver` with no GPUs be `OK` or omitted? The owner (T11/T07 merge or the prereq owner) should decide; a test already documents the current empty status.
