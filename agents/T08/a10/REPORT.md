# T08 a10: wrong-platform update assets

**Fault:** a release publishes an asset whose *name* matches this platform
(`EZLM-Setup-0.1.11.exe` on Windows, `EZLM-0.1.11.dmg` on macOS) but whose
*contents* are another OS/arch: a Linux ELF, a Mach-O, a Windows PE renamed
`.dmg`, a zip, a DMG without a UDIF trailer, or a PE for the wrong processor.
The updater accepted it and handed the path to the app, which runs or mounts it.

**Status:** reproduced, fixed, tested. Severity: **major**.

## Why it matters

`Check` chooses the asset by name only (`internal/update/update.go:47`, `:58`)
and `Download` checked only the size and GitHub's SHA-256
(`internal/update/update.go:113-130`, digest at `:122`). A digest proves the
bytes are the ones GitHub listed; it does not prove they are an installer for
this computer. A release built or uploaded wrong passes every existing check.

Consumers take the returned path without looking inside:

- Windows runs it: `windows/EZLM/SettingsPane.cs:302` `Process.Start(... /SILENT
  ... /UPDATE=1)`, after `StopEngineHelpers()` has stopped the engine.
- macOS mounts it: `macos/Sources/EZLM/Updater.swift:20` `hdiutil attach`, then
  copies the `EZLM.app` out and swaps it in.

On Windows a wrong-platform PE runs with the user's privileges. A non-PE named
`.exe` fails at `Process.Start`, so the update stops with "Couldn't start the
installer". On macOS a non-DMG fails at `hdiutil attach` and the swap never
starts, so the install stays intact but the error is hdiutil's.

## Reproduction (before)

`internal/update/platform_test.go`, `TestDownloadRejectsWrongPlatform`: an
`httptest` GitHub returns one release whose asset is `EZLM-0.2.0.dmg` (the name
`assetFor("darwin", ...)` accepts), whose body is a PE executable, and whose
metadata digest is the SHA-256 of those PE bytes. Against the baseline:

```
=== RUN   TestDownloadRejectsWrongPlatform
    platform_test.go:137: Download installed "EZLM-0.2.0.dmg" for the wrong platform
        (path ".../EZLM-update/EZLM-0.2.0.dmg")
--- FAIL: TestDownloadRejectsWrongPlatform (0.00s)
=== RUN   TestDownloadAcceptsThisPlatform
--- PASS: TestDownloadAcceptsThisPlatform (0.00s)
```

The wrong file was returned to the caller as a successful download.

## Fix

`internal/update/update.go`:

- `Download` now calls `verifyPlatform(dst, runtime.GOOS, runtime.GOARCH)`
  after the digest check and removes the file when it fails (`:126`). It is in
  the engine, so both apps are covered without touching `windows/` or `macos/`.
- `verifyPlatform` (`:137`) rejects:
  - a Windows asset that is not `.exe`, and a macOS asset that is not `.dmg`;
  - a Windows asset that is not a PE: `MZ` at offset 0, a sane `e_lfanew`, and
    `PE\0\0` at that offset (`peMachine`, `:204`);
  - a PE whose COFF machine cannot run on this processor (`peRunsOn`, `:229`).
    A 32-bit PE is always accepted because EZLM's installer is one Inno Setup
    binary that picks x64 or ARM64 files at install time
    (`windows/installer/EZLM.iss:1-2`); x64 is accepted on ARM64 Windows
    because Windows emulates it;
  - a macOS asset with no UDIF trailer: the last 512 bytes must start with
    `koly` (`isDMG`, `:260`), which is what `hdiutil` writes and mounts.
- Rejection messages name the file type found and the next action, e.g. "the
  update file is not a macOS disk image: its contents look like a Linux
  program, so EZLM deleted it. Try again, and if it happens again, report this
  problem". The file is removed so a retry cannot reuse it.

## Tests (`internal/update/platform_test.go`)

- `TestDownloadRejectsWrongPlatform`: end to end through `Check` + `Download`
  with a matching digest. Asserts an error, no path, a message that names the
  platform, and that the file is gone.
- `TestDownloadAcceptsThisPlatform`: the same flow with a real PE (Windows) or
  a real UDIF image (macOS) still succeeds, so the check is not a blanket
  rejection.
- `TestVerifyPlatform`: table over both hosts: PE amd64/arm64/32-bit,
  arm64-on-amd64, amd64-on-arm64, ELF/Mach-O/DMG/zip/gzip named `.exe`, PE
  named `.dmg`, MZ without a PE header, empty file, 511-byte image with no
  trailer, wrong extension, and a Linux host (not checked). Every rejection is
  asserted on the message as well as on failure.

Tests are hermetic: `httptest` only, `EZLM_UPDATE_URL` points at it,
`TMPDIR`/`TMP`/`TEMP` point into `t.TempDir()`, and the download directory is
removed at the end.

## Before/after

| Case | Before | After |
|---|---|---|
| PE renamed `.dmg`, digest matches | accepted, returned to the app | rejected, deleted |
| ELF/Mach-O/zip named `.exe` | accepted | rejected, deleted |
| PE named `.dmg` | accepted | rejected, deleted |
| 511-byte `.dmg` with no trailer | accepted | rejected, deleted |
| arm64-only PE on amd64 Windows | accepted | rejected |
| 32-bit Inno setup on x64/ARM64 | accepted | accepted |
| correct installer | accepted | accepted |

Full checks after the change: `gofmt -l .` empty, `go build ./...`,
`go vet ./...`, `go test -p 2 ./...`, `GOOS=windows GOARCH=amd64 go build
./...`, `GOOS=windows GOARCH=arm64 go build ./...`, `GOOS=linux go build
./...` all pass.

## Residual risk

- **macOS architecture is not checked.** A DMG that is a valid UDIF image but
  contains an arm64-only `EZLM.app` on an Intel Mac passes `verifyPlatform`.
  The macOS updater then swaps it in and relaunches, so the user can end up
  with an app that will not start. Closing this needs the staged bundle
  inspected before the swap (`lipo -archs` / `file` on
  `Contents/MacOS/EZLM` in `Updater.swift`) or a DMG whose app is universal.
  This is the largest gap left, and it belongs to the macOS side.
- A DMG written by something other than `hdiutil` (raw UDRW, no `koly`) is
  rejected although macOS might mount it. EZLM builds its DMGs with `hdiutil`,
  so this is a false-positive risk, not a hole.
- The Windows check proves "a PE that can run here", not "the EZLM installer".
  Any valid Windows PE with a matching digest passes; the digest remains the
  control for what was published. A PE for a *different* app is out of this
  fault's scope.
- `Check` still trusts names, so the platform is proven only after the whole
  file is downloaded. That wastes a download on a bad release but cannot
  install it. There is also no fallback to another asset: if the only matching
  asset is wrong, the update fails with a clear error instead of trying a
  differently named file.

## Open questions

- Is the shipped `EZLM-*.dmg` universal (arm64 + x86_64)? If not, the macOS
  architecture gap above is reachable and should be fixed in `Updater.swift`.
- Should `Check` also reject asset names that name another OS
  (`EZLM-Setup-linux.exe`) before downloading, as a cheap first pass?

## Commits

- `0d8f289` T08: reject update assets whose contents are for another platform
- `6621d2b` T08: keep the wrong-platform test's downloads in its own temp dir
