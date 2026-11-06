# T13 a02 report: Install on macOS

**Slice:** the DMG, Gatekeeper, the "unidentified developer" path, App Translocation, and how to verify the install.
**Deliverable:** `agents/T13/sections/a02.md` (about 1,250 words including footnotes; 17 code citations).
**Status:** complete, docs only. No code changed.

## What the section covers

1. Requirements: macOS 14 or later, Apple Silicon recommended, Intel works CPU-only, disk space, engine bundled in the app.
2. Drag-install from `EZLM.dmg` into Applications, then eject the image.
3. First open and Gatekeeper: Done, System Settings > Privacy & Security > Open Anyway, password or Touch ID, Open. Covers both the macOS 14/15 "Apple could not verify" wording and the older "unidentified developer" wording, and notes that macOS 15 removed the Control-click bypass.
4. App Translocation: why EZLM runs from a hidden read-only copy when opened from the DMG or Downloads, the exact gold note the app shows, why in-place update fails there, and how to move the app properly.
5. Verification: app in Applications, gold note gone, System page rows and their expected values on Apple Silicon and Intel, data folders, and an optional `codesign` check.
6. The macOS "EZLM is damaged" alert and the standard recovery (re-download, then remove the quarantine attribute).

## Findings

### F1 (major): release signing/notarization is described two ways

`macos/scripts/build-app.sh:72` says "releases are signed with a Developer ID and notarized", but the user-facing `BETA.md:16` says the beta "isn't notarized by Apple yet". Nothing in the repo notarizes or staples a build: `make-dmg.sh` only runs `hdiutil create` (`macos/scripts/make-dmg.sh:14-21`), and there is no `notarytool` or `stapler` call anywhere in the tree (`grep -rniE "notar|stapl"`). So either releases are notarized outside this repo, or the `build-app.sh` comment is stale.

Impact: if the shipped beta is notarized, section 2 of my file describes a dialog testers will not see. I wrote it conditionally ("may not be notarized", "if your copy is notarized ... skip to section 4") so it is true either way, but the Lead should make it definite once the answer is known. If the beta is notarized, `BETA.md:16` is also wrong and should be fixed in the root file (not mine to edit).

### F2 (minor): BETA.md says Apple Silicon only, but the app supports Intel

`BETA.md:13` says "Mac with Apple Silicon, M1 or later". The app is built universal (`macos/scripts/build-app.sh:12-13` builds both Swift arches; `:37-38` lipos the engine for arm64 and amd64), the minimum is macOS 14 (`:65`), and the scanner has an explicit Intel path that sets the CPU backend and adds "Intel Mac detected: models will run on the CPU." (`internal/scan/scan_darwin.go:10-13`). So Intel Macs on macOS 14 are supported, just slower. My section says so. `BETA.md` should probably be widened, or the team should state that Intel is unsupported despite the code.

### F3 (minor): the gold translocation note is the only in-app signal

`AppPlace.temporary` (`macos/Sources/EZLM/Updater.swift:99-107`) drives a note shown in the wizard (`Wizard.swift:100`) and the main window (`MainView.swift:146`), with text and an "Open Applications" button at `Settings.swift:424-437`. The note says "it won't be here after a restart", which matches the temporary nature of the translocated copy. No code removes the note or offers a one-click move; the user must drag the app. My section tells them exactly that. Worth considering for a future build: a button that moves the app and relaunches.

### F4 (minor): update failure from a translocated copy is handled, but the message assumes Applications

`Updater.install` stages the new app next to the running one (`Updater.swift:31-35`), which cannot work on the read-only translocated volume. `Settings.swift:94-101` catches the failure, opens the DMG in Finder, and says "Drag EZLM into Applications to replace the old one, then open it again." That is correct for the common case, but a user who keeps EZLM in a non-standard folder (say `~/Apps`) is told to use Applications. This is a wording edge case, not a blocker. Documented in the section as "drag over the old one" without repeating the Applications assumption.

### F5 (minor): "damaged" alert has no in-app guidance

Nothing in `macos/` mentions the macOS "EZLM is damaged and can't be opened" alert. It is a macOS Gatekeeper outcome, not an EZLM string, so it cannot be cited to EZLM code. I documented the standard recovery (re-download, then `xattr -dr com.apple.quarantine`) as a last resort and labelled it as macOS behaviour. The exact wording varies by macOS version, so testers may report variants.

## What I could not verify

- **No GUI run.** All claims come from source. I did not launch the app, mount a DMG, or reproduce Gatekeeper on this machine. The dialogs' exact wording is Apple's and varies by macOS version; the section says so.
- **Release notarization state** (F1). The repo does not contain the release pipeline.
- **Intel behaviour.** Code supports it (`scan_darwin.go:10-13`), but I did not run on an Intel Mac, and no test covers the darwin non-arm64 path.
- **Whether the translocated copy is removed at quit.** The code comment at `Updater.swift:99-101` says so and it matches Apple's documented "randomized, read-only locations" (Apple Platform Security, Gatekeeper), but I did not observe it.
- **macOS 15 Control-click behaviour.** Widely documented as removed; not tested here, and not in the repo.

## Open questions for the Lead

1. Is the beta release Developer ID signed and notarized? This decides whether section 2 is the main path or a fallback, and whether `BETA.md:16` needs a correction. (F1)
2. Is Intel Mac support intended for the beta? If not, the code path in `scan_darwin.go` and the universal build should be called out as untested rather than supported. (F2)
3. Should the section include the `xattr` command? It is the reliable fix for "damaged", but it is Terminal advice for a non-technical audience. I kept it as step 2 of a last resort. (F5)

## Checks before commit

- `gofmt -l .` (expected empty; no Go files touched).
- `go build ./...` (sanity; no code touched).

## Next steps

- Lead: resolve F1 and F2, then tighten section 2 and the requirements list.
- a08 (uninstall) should cross-reference the "Remove models and settings" button on the System page (`MainView.swift:530`) and note that dragging the app to the Trash does not remove `~/.ezlm` or `~/Library/Application Support/EZLM`.
- a16 (review) should check that my "Open Anyway" steps match a01's Windows SmartScreen steps in tone and numbering.
