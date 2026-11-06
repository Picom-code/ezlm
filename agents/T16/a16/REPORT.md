# T16 / a16 report: severity table and report support

**Agent:** a16. **Branch:** `agent/T16-a16`, worktree
`~/ezlm-agents/work/subs/T16/a16`. **Baseline:** 5213370.

## What I reviewed

I did not own a code area; I owned the ranked list. I read the whole Mac app
under `macos/` (21 Swift files, about 8,000 lines), the build and packaging
scripts, `Package.swift`, and the three Darwin Go files the task lets T16 read:
`internal/scan/scan_darwin.go`, `internal/serve/service_darwin.go`,
`internal/serve/birth_darwin.go`. Frozen files were read only, never edited.

I then produced `agents/T16/a16/findings.md`: 18 findings, ranked, each with
file:line, severity, the fix, and whether it was verified by running something
on this Mac or only by reading code.

## Findings by severity

- **Blocker (2):** ad-hoc signature and unsigned, un-notarized DMG
  (`build-app.sh:73`, `make-dmg.sh:20`); TCC grants do not survive an ad-hoc
  update.
- **Major (7):** update relaunch can start a second instance
  (`Settings.swift:88` + `EZLMApp.swift:47`); update install trusts the
  downloaded DMG without a signature or version check (`Updater.swift:31-66`);
  the whole chat transcript re-renders and re-parses at 15 fps
  (`ChatView.swift:155`, `ChatModel.swift:156`); the SSE loop can hang for an
  hour after sleep/wake and retries at 2 Hz while the engine is dead
  (`Engine.swift:154-174`); free memory is over-reported by about 2x on this Mac
  (`scan_darwin.go:18-27`); the launchd plist pins the engine path
  (`service_darwin.go:49-87`); the translocated-app update path only works by
  falling back to a Finder drag.
- **Minor (9):** main-actor chat saves, first-open chat file reads, notes larger
  than the context room, no localization resources, wave timer not in common
  run-loop mode, M3 Ultra bandwidth entry, shared launchd log file, 30 s engine
  timeout, unsynchronized prefs cache.

## What I changed

Only `agents/T16/a16/findings.md` and this report. No Swift or Go source
changes: every finding either belongs to another agent's area or is a report-only
item (signing, packaging, update flow), and the brief says fixes in shared files
must be minimal and attributed. The one-line fixes are spelled out in the table
so the owning agent or the morning merge can apply them.

## Verified on this Mac

- `cd macos && swift build`: passes (11 s).
- `cd macos && swift test`: passes, 22 tests, 0 failures.
- `go build ./cmd/ezlm` and `./ezlm scan`: run, no model loaded. This
  reproduced finding 7: the engine reports "18.0 GB total, 7.9 GB free" while
  `top -l 1` reports 198 MB unused and 6.2 GB in the compressor, and `vm_stat`
  gives about 4.3 GB free + inactive + speculative.
- Environment: macOS 26.3.1, Apple M3 Pro, 18 GB, Swift 6.2.

## What needs a device or a clean Mac

Gatekeeper and TCC on a quarantined DMG; a real update install and relaunch; an
engine restart and sleep/wake recovery; Instruments numbers for the chat
rendering; login-agent behaviour after moving the app. All are listed at the end
of `findings.md` under "what was verified, and what was not".

## Open questions

1. Does the release job set `SIGN_ID` and notarize outside `macos/scripts`? If
   not, the public beta cannot be opened by a normal user without right-click >
   Open.
2. Is `memory_pressure`'s free percentage the intended source of truth for
   `RAMAvail`, given how far it diverges from `vm_stat` under load?
3. Sibling agents had not committed findings to their worktrees when this table
   was written, so it is built from my own review. It is the merge point: the
   lead should append a01 to a15's findings rather than replace the file.

## Next steps

- Fix 1, 2 and 4 in the release path first: they decide whether a beta user can
  open and update the app at all.
- Fix 3 with a `relaunching` flag plus `LSMultipleInstancesProhibited`.
- Fix 5 and 6 next: they are the two things users will feel.
- Fold the other agents' findings into the table, keeping the severity order and
  the "verified" column honest.
