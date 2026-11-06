# T08 / a11: the updater threat model

**Agent:** a11 (cross-cutting, documentation only)
**Branch:** `agent/T08-a11`
**Slice:** `agents/T08/a11/threat-model.md` plus this report.
**Code changed:** none. T08's other agents own the fixes and the tests.

## What I did

Read the baseline updater end to end — `internal/update/update.go`,
`internal/engine/download.go`, `internal/engine/engine.go:318-337`, the app
call site `internal/app/settings.go:31-62`, and both install paths
(`macos/Sources/EZLM/Updater.swift`,
`windows/EZLM/SettingsPane.cs:250-315`) — and wrote
`agents/T08/a11/threat-model.md`. It contains:

- what the updater trusts, as a table of 13 inputs with the line each enters
  and what breaks if it lies;
- the failure windows, grouped by pipeline stage and by OS, including what a
  crash at each one leaves behind;
- eight invariants that make "never worse than before" testable, each marked
  enforced / partly / not;
- the controls that exist today, with citations;
- the gaps, mapped to a01-a10 and a14, plus two nobody owns tonight;
- residual risk and five open questions.

Commit: `056bc12`.

## Findings

Severity is my judgement from code reading; the fault agents own the failing
tests that prove each one. No claim here rests on a reproduction of mine.

| # | Finding | Evidence | Severity |
|---|---------|----------|----------|
| F1 | A release with no digest, or a digest of any algorithm other than `sha256:`, is installed with no content check. `verifyDigest` returns `nil` on anything without the prefix, so `sha512:...` is silently ignored rather than refused | `internal/engine/engine.go:321-324`; call at `internal/update/update.go:120` | major |
| F2 | A release with `size: 0` and no digest gets **no** integrity check at all: the size check is skipped when `want == 0` | `internal/engine/download.go:45`, `:87-96` | major |
| F3 | No signature or notarization check anywhere, and because the download uses Go's HTTP client the `com.apple.quarantine` bit is never set, so macOS Gatekeeper is not consulted for updates | `grep -rn "codesign\|notariz\|spctl\|quarantine"` finds nothing under `internal/`, `macos/Sources/EZLM`, `windows/EZLM`; Mac install `Updater.swift:14-54` | major |
| F4 | The macOS swap has a window between two renames where no `EZLM.app` exists at the install path; a crash there leaves only hidden `.EZLM-old-<pid>.app` / `.EZLM-update-<pid>.app`, and the restore on the second-rename failure is best effort (`try?`) | `macos/Sources/EZLM/Updater.swift:44-52` | major |
| F5 | Windows starts the installer, then closes the window 1.2 s later without reading the installer's exit code. `/SILENT /SUPPRESSMSGBOXES` means a failed or UAC-declined install looks like EZLM simply quitting | `windows/EZLM/SettingsPane.cs:295-306` | major |
| F6 | A check error defeats the one-hour cache (`updCache.Error != ""` forces a refresh), so a rate-limited or failing API is re-hit on every UI refresh instead of backing off | `internal/app/settings.go:36-38` | major (a01) |
| F7 | `prerelease` is never decoded; only `draft` is filtered. Stable users receive betas. This matches the comment at `update.go:19-21`, so it is intended, but it is policy by comment rather than by code | `internal/update/update.go:76-95` | minor (a06) |
| F8 | `Newer` is not a total order on malformed tags: `split` ignores `Atoi` errors and drops components past the third, so `1.2.3.4` compares equal to `1.2.3` and `v` compares equal to `0.0.0` | `internal/update/update.go:128-158` | minor (a05, a07) |
| F9 | The install path is a string from an engine event; it is protected only by the per-launch API token, not by any check that the path is under `EZLM-update` | `internal/app/settings.go:52-58`; `internal/web/server.go:247-254` | minor |
| F10 | The download directory is predictable, `0o755`, never cleaned, and a pre-existing file of the right size is reused without a fetch, so a local account that creates `/tmp/EZLM-update` first can pre-seed an installer; with F1 that installer is installed | `internal/update/update.go:112-116`; `internal/engine/download.go:45-50` | minor |
| F11 | Redirects are followed to any host, for the API response and for the asset, with no allow-list | `internal/update/update.go:66`; `internal/engine/download.go:25-32` | minor (a02) |

The through-line: **the digest is the only content control, and it is
optional.** T1 through T9 in the threat model all arrive in one HTTPS
response, so the digest catches corruption and CDN mixups but not a
compromised release. That is worth stating plainly before a public beta.

## What I changed and why

Nothing outside `agents/T08/a11/`. My slice is documentation; edits to
`internal/update/**` would collide with a01-a10, and edits to `windows/` or
`macos/` are forbidden tonight.

## What I could not finish

- No failing tests. Proving F1, F2 and F10 needs the fault agents' fakes
  (a04 and a09/a10 respectively); I deliberately did not add a test file,
  since one test file per agent is the rule and a11 owns none.
- The Windows installer's own behaviour (whether Inno leaves the old files on
  a failed replace) is not in this tree. a14 should say.

## Next steps

1. a04 should decide whether a missing digest is fatal, and refuse (not skip)
   a digest of the wrong algorithm. That closes F1 and, with a size floor,
   F2.
2. a14 should confirm or correct the W16/W17 analysis against the real
   installer, and say whether an exit code is observable on Windows.
3. Before the public beta: a signing key held off GitHub, verified in both
   apps. That is the only thing that closes F3.
4. Someone should own temp-directory hygiene (F10): a per-user, per-run
   directory under `EZLM_HOME`, created `0700`, cleaned after install.

## Open questions

See section 8 of `threat-model.md`: channel policy for prereleases, whether
a missing digest is fatal, who cleans the temp directory, whether `Download`
re-validates the version at install time, and whether the Windows installer
reports failure.
