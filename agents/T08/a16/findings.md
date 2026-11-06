# T08 updater: severity table (a16)

Status of every updater fault the T08 team was asked to cover, with evidence and the fix each
one needs. Written to stand alone: it names the code, the exact lines, and what was observed.

- Source under review: `internal/update/update.go` (158 lines) and the engine helpers it calls,
  `internal/engine/download.go` and `internal/engine/engine.go` (`VerifyDigest`), at baseline
  commit `5213370` (EZLM source snapshot 2026-11-06).
- Method: read the package, then ran a throwaway characterisation probe (an `httptest` GitHub
  API plus an `httptest` asset server, temp `TMPDIR`, no network) that logs what the current code
  actually does. The probe is not committed, so no other agent's fixes can break it; its full
  output is in `probe-output.txt` next to this file. Evidence rows below say `probe A`..`probe I`
  to point at that output.
- Severity: **blocker** = can brick an install or install something it shouldn't in a normal
  scenario; **major** = installs something it shouldn't in a plausible scenario; **minor** =
  robustness or user-visible defect, no bad install. Status: **open**, **mitigated** (code
  already handles it; keep a test), **fixed** (by a named agent, verified), **by design**.
- Status column reflects this branch at commit time. a01-a15 have not landed yet, so most rows
  are still `open` or `mitigated` here; re-check the rows when their branches merge.

## The table

| ID | Fault (owner) | Status | Severity | Evidence | Fix needed |
|---|---|---|---|---|---|
| F01 | Rate limits: 403, 429, secondary limit (a01) | open | minor | probe F: 403/429/403+`Retry-After` each make exactly one request, no retry, `Info.Error` = `"checking for updates failed (403 Forbidden)"`. `update.go:72-75` ignores `Retry-After`. `internal/app/settings.go:37` re-checks immediately whenever `Error != ""`, so every UI call re-hits GitHub. | Honour `Retry-After`/`X-RateLimit-Reset`, keep the last good `Info`, and suppress re-checks for a minute after a failure. No spin today: `Check` makes one request and gives up. |
| F02 | Redirect chains, cross-host redirects (a02) | mitigated | minor | probe H: a 302 to a *different host* is followed and the release from the second host is accepted. Loops/10-hop cap come from `net/http` defaults; `update.go:66` uses `http.DefaultClient`. | Optionally allowlist hosts (`api.github.com`, `github.com`, `objects.githubusercontent.com`) and assert the final host, so a redirected check cannot hand back an arbitrary release. |
| F03 | Truncated downloads (a03) | mitigated | minor | probe G: body shorter than `Content-Length` -> 5 attempts, then a plain-words error, nothing installed. probe G2: chunked body ending at a chunk boundary -> one attempt, `"came out the wrong size (5 bytes instead of 100)"`, `.part` deleted. `download.go:87-96`. | Keep the tests. Gap: if `Release.Size` is 0 (asset `size` missing in the API), no size check runs at all; only the digest stands between a short file and install (see F04). |
| F04 | Digests: missing, wrong algorithm, mismatch (a04) | **open** | **major** | probe D: digest `""` accepted, digest `"md5:<sha256>"` accepted, both return a path and no error. `engine.go:321-324` returns `nil` unless the string starts with `sha256:`. Mismatch and short-`sha256` are rejected. | Refuse an install with no digest, or with a digest that is not exactly `sha256:`+64 hex. Today a release published without a digest, or with a future algorithm, is installed on size alone. |
| F05 | Replayed old asset at the temp path (a05) | **open** | **major** | probe E2: a file already at `$TMPDIR/EZLM-update/EZLM.dmg`, right size, wrong bytes, no digest -> `Download` returns that path and the app would install it. `download.go:45` short-circuits on size before any hashing; `update.go:120` only hashes what was downloaded. With a digest present the replay is caught (probe E). | Always hash the file that is returned, or delete the cached file when `Digest` is empty. Fixing F04 removes the worst case. |
| F06 | Downgrades, `1.10` vs `1.9` (a05) | mitigated | minor | probe B: `Newer("1.10","1.9")=true` (numeric compare, not text), `Newer("0.1.10-beta","0.1.9-beta")=true`. `update.go:105` only reports `Available` when the release beats the running version. | Keep the test. Gap: `Check` never compares the chosen release to `current` before calling it `Latest`, and `Download`/the install path take whatever `Release` they are handed, so a caller cannot downgrade itself but a stale cached `Info` can still offer an older installer after the app was updated. |
| F07 | Prereleases and drafts (a06) | by design | minor | probe A: list = `0.2.0-beta` (prerelease), `0.1.9` (draft), `0.1.5` -> chosen `0.2.0-beta`, draft skipped. `update.go:81` parses `draft`; there is **no** `prerelease` field in the struct (`update.go:76-88`), and `update.go:95` filters drafts only. | Matches the comment at `update.go:19-22` ("beta builds included"): prereleases are the channel. If a stable-only channel is ever wanted, `prerelease` has to be parsed first; today every user is on the beta channel. |
| F08 | Malformed version tags (a07) | open | minor | probe B/C: no panic for `v`, `v1`, `""`, `1.2.3.4`, `v1.2.3-beta+build`, full-width digits, a 400-digit number. But `1.2.3.4` becomes `Latest.Version="1.2.3.4"` and compares as `1.2.0` (`split` at `update.go:147-157` keeps `3.4`, `Atoi` fails, error discarded). `1.2.3-beta+build` beats `1.2.3-beta` (build metadata is not stripped, `update.go:150`). | Reject tags that are not three numeric components with an optional `-pre`; compare build metadata as equal. `Atoi` errors are swallowed, so any garbage becomes 0. |
| F09 | Huge assets, slow drip, header-stripping proxies (a08) | open | minor | `download.go:21` cuts a transfer after 45 s with no bytes and `download.go:78` gives up after 5 failures with no progress, but `Download` has **no overall deadline**: a server that sends one byte every 40 s never trips the watchdog and never finishes. `download.go:154-157` handles a stripped `Content-Length` (`total=want`); probe G2 covers a short chunked body. No cap on `Size`. | Add a total deadline or a minimum-rate check, and a sanity cap on `Release.Size`. |
| F10 | Disk full partway through the write (a09) | mitigated | minor | `update.go:112-124`: the download goes to `$TMPDIR/EZLM-update`, never over the installed app, so a failed write leaves the old install intact. The write error propagates (`download.go:172-173`); `.part` stays for resume. | Keep the test. Consider removing a `.part` older than the current release so a leftover from an earlier version cannot be resumed. |
| F11 | Wrong platform, right asset name (a10) | **open** | **major** | probe I: `assetFor("windows","EZLM-Setup-linux-amd64.exe")=true`; selection is name prefix/suffix only (`update.go:45-53`). Nothing checks the executable magic, the archive layout, or the arch (arm64 vs amd64 matters: `BETA.md` ships ARM laptops). | Check the PE/Mach-O magic and the arch of the downloaded file before install, and tighten the name match. The digest proves integrity, not platform. |
| F12 | The replace/relaunch step (a14) | open | unknown | Out of this slice; see `agents/T08/a14/replace.md`. Entry points: `macos/Sources/EZLM/Settings.swift:65-107`, `macos/Sources/EZLM/Updater.swift:14`, and the Windows installer. | a14 owns the failure windows; link its table here when it lands. |
| F13 | Trust root for the whole update (a11) | open | minor | The digest, the URL and the version all arrive in one GitHub API response over TLS (`update.go:56-107`). There is no signature and no allowlist, so a compromised API response is trusted end to end. `BETA.md` confirms builds are unsigned and unnotarized. | Threat model in `agents/T08/a11/threat-model.md`. A signature would be the real fix; out of scope for tonight. |
| F14 | Shared temp path across instances (cross-cutting) | open | minor | `update.go:112` uses a fixed `EZLM-update` directory and `filepath.Base(r.Asset)`; two EZLM instances or a retry share the same `.part`. | Per-process temp dir, or a lock file. Low risk because the digest (when present) catches corruption. |
| F15 | Rate-limit error text has no next action (a01) | open | minor | probe F output string, `update.go:73`; user-facing text rule in `README.md`. | "Checking for updates failed. EZLM will try again later." or similar. |

## What this means for the goal

"Never brick an install or install something it shouldn't" is mostly held up by three controls:
the size check, the digest, and the fact that downloads land in a temp dir before anything runs.
The weak points are all on the trust side, not the plumbing side:

1. **F04** is the one to fix first: `VerifyDigest` silently passes when the digest is absent or
   not `sha256:` (`engine.go:321-324`). Combined with **F05** (a right-sized file already in the
   temp dir is returned unhashed) and **F11** (name-only platform match), a wrong or stale
   installer can be installed with no error at all.
2. **F11** is the only fault that can leave a user with an app that no longer runs, because the
   installer replaces the working copy.
3. Everything a01-a03 and a09 were asked to break is already handled by `engine.Download`
   (retry, resume, size check, temp dir). Those agents should be writing regression tests, not
   expecting to change code; if they find a case that still fails, it is most likely the
   `Size == 0` path noted in F03.

## Open questions

- Is a missing `digest` a hard failure? GitHub only returns `digest` for releases published after
  the field existed, so making it mandatory could reject a legitimately old release. The
  compromise is to refuse to *install* without one, which is what F04 proposes.
- Is the beta channel the only channel? `update.go:19-22` says prereleases are included on
  purpose, but `BETA.md` describes one build for everyone, so "follow the documented channel
  policy" (a06's brief) may just mean "drafts out, prereleases in".
- Should `Check` compare against `current` before setting `Latest`? Today it reports the newest
  release on GitHub even when it is older than the running build (F06).

## Keeping this current

When a01-a15 land, update the Status/Evidence/Fix cells only, and add a line to the row naming
the commit and the test file that proves it (for example `fixed in a04, internal/update/digest_test.go`).
Re-run the probe by re-creating it from `probe-output.txt`'s shape if a status is in doubt.
