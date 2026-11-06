# T08 / a16: severity table

## What I did

Read `internal/update/` end to end (`update.go`, plus the engine helpers it depends on:
`internal/engine/download.go` and `VerifyDigest` in `internal/engine/engine.go`), and the call
sites that matter (`internal/app/settings.go:33-60`, `internal/web/server.go:154-157`,
`macos/Sources/EZLM/Settings.swift:18-107`). Then ran a throwaway `httptest` probe against the
real updater to replace guesses with observations: a fake GitHub release list, a fake asset
server, temp `TMPDIR`, no network. Its output is committed as `probe-output.txt`; the probe
itself is not committed so it cannot go stale or collide with a01-a10's test files.

Deliverable: `agents/T08/a16/findings.md`, a 15-row table (F01-F15) with status, severity,
file:line and probe evidence, and the fix for each. It is written to stand alone.

## What I found

No code changes. The plumbing (retry, resume, size check, temp-dir download) is in decent
shape; the gaps are on the trust side. In severity order:

1. **F04, major, open**: `engine.go:321-324` returns `nil` when the digest is missing or is not
   `sha256:`, so a release with no digest, or with e.g. `md5:`, installs on size alone.
   Probe D. Owner a04.
2. **F05, major, open**: `download.go:45` returns an existing file at the temp path if its size
   matches, and `update.go:120` then hashes it; with no digest (F04) arbitrary same-size bytes
   in `$TMPDIR/EZLM-update` are installed. Probe E2. Owner a05/a04.
3. **F11, major, open**: `assetFor` (`update.go:45-53`) matches by name prefix/suffix only, so
   `EZLM-Setup-linux-amd64.exe` is a Windows asset; contents and arch are never checked.
   Probe I. Owner a10.
4. **Minor, open**: rate-limit errors bypass the one-hour cache (`internal/app/settings.go:37`),
   no `Retry-After` (F01); malformed tags are accepted and compare as 0 (F08); no overall
   download deadline, so a 1-byte-every-40s drip never trips the 45 s watchdog (F09); shared
   temp dir across instances (F14); rate-limit error text has no next action (F15).
5. **Mitigated, keep the tests**: truncation (F03, probe G/G2), redirects incl. cross-host
   (F02, probe H), `1.10` vs `1.9` (F06, probe B), disk-full isolation (F10).
6. **By design**: prereleases are offered to everyone and drafts are skipped (F07, probe A);
   `update.go:19-22` says so. There is no `prerelease` field in the JSON struct, so a stable-only
   channel cannot be implemented without adding one.

## What I changed and why

Nothing in `internal/update/`. My slice is the table; the fixes belong to a01-a10 so each lands
with its own failing-then-passing test. Adding a fix here would collide with their patches.

## What I could not finish

- F12 (replace/relaunch) is a14's slice; the table links it rather than duplicating it.
- F13 (trust root) is a11's threat model.
- The probe covers macOS asset names because that is the host; `assetFor` is pure and its
  Windows branch is identical, but nobody has run the probe on Windows.
- F09's slow-drip case is code reading plus a 20 s truncation run, not a live 45 s stall test.

## Next steps

- a04: make a missing or non-`sha256` digest a hard failure; a05: hash whatever file `Download`
  returns; a10: check the file magic/arch before install.
- When a01-a15 land, update the Status cells in `findings.md` with the commit and test file.
- Consider one allowlist of asset hosts (F02) and one total download deadline (F09) as follow-ups.

## Open questions

- Must a release have a digest to be installable? (See `findings.md`, Open questions.)
- Is the beta channel the only channel, or should `prerelease` become a real field?
- Should `Check` clamp `Latest` to something newer than `current` before reporting it?
