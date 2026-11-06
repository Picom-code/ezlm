# T16 a04: ad-hoc signing consequences (Gatekeeper, quarantine, TCC)

Reviewed the signing and update path in `macos/scripts/build-app.sh`, `macos/scripts/make-dmg.sh`,
`macos/Sources/EZLM/Updater.swift`, `macos/Sources/EZLM/Settings.swift` (frozen, read only) and the
engine's quit path. Verified on this Mac (macOS 26.3.1) with a throwaway ad-hoc signed app in /tmp.

## What the code does today

- `macos/scripts/build-app.sh:73` signs the app ad-hoc by default:
  `codesign --force --deep --sign "${SIGN_ID:--}" "$APP"`. Releases use `SIGN_ID` (Developer ID)
  but nothing in the scripts notarizes or staples (see finding 3).
- The app updates itself in place: `Updater.install` mounts the downloaded DMG, `ditto` copies
  `EZLM.app` next to the running app, swaps the bundles with two renames, then `Updater.relaunch`
  runs `open` after the old process exits (`Updater.swift:14-68`).
- `BETA.md` step 2 tells users to use System Settings > Privacy & Security > Open Anyway on first
  launch, and step 3 says Free up memory needs permission to control other apps.

## Findings

### 1. A quarantined ad-hoc app is blocked by Gatekeeper, and copying it out of a quarantined DMG makes it quarantined. Severity: blocker for the manual install/update path.

Evidence on this Mac, with a minimal app signed the same way (`codesign --force --deep --sign -`):

- `spctl --assess --type execute -vv copied.app` -> `rejected` (status 3), while
  `codesign --verify --deep --strict` -> ok. So the signature is valid but Gatekeeper will not
  accept it; the only reason the app launches at all is that Gatekeeper only checks quarantined
  bundles.
- `codesign -dv` -> `Signature=adhoc`, `TeamIdentifier=not set`.
- Attaching a DMG carrying `com.apple.quarantine` and copying the app out gives the copy
  `com.apple.quarantine: 0281;...` with both `ditto` and `cp -R`, even though the app inside the
  mounted volume has no quarantine attribute. So the quarantine is added on copy, not stored in the
  image.

Consequence: a user who downloads `EZLM.dmg` in a browser and drags the new app over the old one
gets a blocked app on the next launch, with no "the update worked" signal. On macOS 15+ the old
Control-click > Open shortcut is gone, so it is System Settings > Open Anyway again. `BETA.md`
documents this for the first install but not for updates.

What the code can do: `Updater.install` now clears `com.apple.quarantine` on the staged bundle
before the swap (`Updater.swift`), so an update installed from a browser-downloaded DMG still opens.
The manual Finder drag path cannot be fixed from inside the app; the report suggests documenting
"after a manual update, allow EZLM again" in `BETA.md`.

Note: the in-app update downloads the DMG with Go's `http` (`internal/update/update.go:105`), which
does not set quarantine, and the Info.plist has no `LSFileQuarantineEnabled`, so today's one-click
update is not affected. That is accidental, not designed; if `LSFileQuarantineEnabled` is ever
added to `build-app.sh`, every one-click update would install a quarantined app.

### 2. TCC grants do not survive an update. Severity: major (one extra prompt per TCC service per update, plus stale entries).

`codesign -d -r-` on an ad-hoc app prints `designated => cdhash H"d0a5..."`. TCC stores the code
requirement with each grant, and for an ad-hoc signature that requirement is the cdhash. Every
build (and every release) changes it, so after an update the stored requirement no longer matches:
the Automation grant for Free up memory is not applied, the user is asked again, and the old entry
stays in the Privacy list (one more "EZLM" per update). The same applies to any other TCC service
the app ever uses.

The app only needs Automation today: `internal/scan/apps_unix.go:71` runs
`osascript -e 'tell application "X" to quit'`, and the engine is a child of EZLM, so the prompt is
attributed to EZLM (`NSAppleEventsUsageDescription` in `build-app.sh:68` exists for it).

What the code can do, in order of cost:

1. Sign the beta with a stable identity instead of `-`. A self-signed certificate (or Developer ID)
   makes the designated requirement `identifier "com.ezlm.app" and certificate leaf = H"..."`,
   which is stable across builds, so grants survive updates. Still not Gatekeeper-clean without
   notarization, but TCC is fixed. This is a `build-app.sh` default (`SIGN_ID`) plus a
   `scripts/` note; needs a cert on the build machine, so it is a recommendation here.
2. Drop Automation entirely: `NSRunningApplication(processIdentifier:).terminate()` does not send
   an Apple Event from the caller and needs no TCC grant. That is an engine change
   (`internal/scan/apps_unix.go`), outside this slice; a14 owns the darwin Go review.
3. If 1 and 2 are not taken, tell the user: after an update the first "Free up memory" will ask
   again. `AEDeterminePermissionToAutomateTarget(..., askUserIfNeeded: false)` can detect the
   missing grant without prompting and show a one-line note. Not implemented.

### 3. Release signing has no hardened runtime and no notarization/staple step. Severity: major for the release, not for tonight's beta.

`build-app.sh:73` uses `--deep` and no `--options runtime`; `make-dmg.sh` only creates the image.
`--deep` is discouraged for Developer ID (sign the nested `ezlm-engine` explicitly), notarization
needs the hardened runtime, and `make-dmg.sh` has no `notarytool submit` + `stapler staple`. The
comment on line 72 says releases are "signed with a Developer ID and notarized", but the scripts do
not do it. If the release pipeline is outside the repo, this is only a documentation gap.

### 4. The updater swaps first and verifies never. Severity: minor.

`Updater.install` checks only that `Contents/MacOS` exists and the bundle ID matches
(`Updater.swift:24-29`), then swaps and deletes the old app (`Updater.swift:44-53`). A truncated or
modified copy is moved into place and the old bundle is removed before the new one is ever launched,
so a bad update leaves no app. It now runs `codesign --verify --deep --strict` on the staged bundle
before the swap and keeps the running app if it fails. There is still no rollback after a failed
relaunch; `Updater.relaunch` returns false only if `Process.run` fails, not if `open` fails.

### 5. Apple Silicon needs the signature to stay valid. Severity: minor / note.

On arm64 an unsigned or wrongly signed executable is killed at launch. `--deep` currently covers
the nested `ezlm-engine`; if the engine is ever copied in after signing, or a build skips
`codesign`, the app will not start on M-series Macs. The verify step added above also catches this
before an update is installed.

## Changed

- `macos/Sources/EZLM/Updater.swift`: after `ditto`, clear `com.apple.quarantine` on the staged
  bundle and verify its signature with `codesign --verify --deep --strict`; both before the swap,
  so a failure leaves the running app untouched.
- `macos/Tests/EZLMTests/SigningTests.swift`: new test file (one per agent). Covers (a) an update
  installed from a quarantined DMG has no quarantine on the installed app, (b) a broken signature
  in the update is rejected and the old app stays in place.

## Verified on this Mac (macOS 26.3.1)

- `spctl --assess --type execute -vv` -> `rejected` (status 3) for an ad-hoc app, while
  `codesign --verify --deep --strict` -> ok; `codesign -dv` -> `Signature=adhoc`,
  `TeamIdentifier=not set`.
- `codesign -d -r-` -> `designated => cdhash H"d0a5..."` for ad-hoc.
- `ditto` and `cp -R` from a mounted DMG that carries `com.apple.quarantine` add
  `com.apple.quarantine: 0281;...` to the copy; the app inside the mounted volume has none.
- `xattr -dr com.apple.quarantine` removes it.
- `SigningTests`, both tests fail without the `Updater.swift` change:
  `XCTAssertNil failed: "0281;00000000;;"` and `did not throw an error` / marker `"new"` instead
  of `"old"`. Both pass with it.
- `gofmt -l .` empty, `go build ./...` and `go vet ./...` ok.
- `cd macos && swift build && swift test` -> 24 tests, 0 failures. The two new tests take about
  30 s because each mounts a disk image; the whole suite is about 21 s of test time.

## Next steps

1. Before the beta: decide the signing story. Either ship the beta signed with a stable self-signed
   certificate (fixes TCC resets, still needs Open Anyway) or accept that every update asks for
   Automation again and say so in `BETA.md`.
2. `BETA.md`: add one line under Updates: after a manual update from a downloaded disk image, macOS
   may ask again to allow EZLM and to control your apps.
3. If a14 touches `internal/scan/apps_unix.go`, replace the `osascript` quit with
   `NSRunningApplication.terminate()` in the engine or a small Swift helper, and the Automation
   prompt disappears for good.
4. Release pipeline: sign the nested engine explicitly, add `--options runtime`, notarize and
   staple in `make-dmg.sh`.

## Open questions

- Does replacing its own bundle in /Applications trigger the App Management TCC prompt (macOS 14+)
  for an ad-hoc app with no Team ID? Needs a clean Mac and a real update; not testable here without
  installing over /Applications/EZLM.app.
- Are Automation grants attributed to EZLM (the responsible process) or to `ezlm-engine` when the
  engine runs from the LaunchAgent rather than as a child of the app? Needs a clean Mac.
- Is the release signed/notarized outside these scripts? If so, finding 3 is only a comment fix.
