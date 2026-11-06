# T08. Updater hardening — consolidated report

16 agents (a01–a16) worked this task in parallel: ten fault classes, a fuzz test, the check-flow
integration test, the replace-step study, a shared fake GitHub, the threat model, and the severity
table. Consolidated onto `agent/T08-updater` by the Lead.

Goal: the in-app updater can never brick an install or install something it shouldn't.

## Result

- **15 faults assessed**, 11 fixed in code, the rest documented with evidence.
- **Gate:** `gofmt -l .` empty, `go build ./...`, `go vet ./...`, `go test -p 2 ./...` pass, plus
  `GOOS=windows` amd64/arm64 and `GOOS=linux` builds.

## Fixed (all in `internal/update/update.go` unless noted)

| Fault | Before | After |
|---|---|---|
| **Missing or wrong digest** (a04) | `engine.VerifyDigest` returned nil for a missing digest and for any non-`sha256:` algorithm, so an unverifiable release installed on size alone. | The digest is validated (algorithm `sha256`, 64 hex chars) **before any bytes are fetched**, and compared with the parsed value after. |
| **Replayed old build** (a05) | A new tag whose asset URL or digest was an older release's was offered and installed. | A candidate that reuses an older release's asset is refused with a clear message. |
| **Downgrades** (a05) | `Check` returned an older release as `Info.Latest`. | Only releases newer than the running version are candidates. |
| **Malformed version tags** (a07, a12) | `v1` → 1.0.0, `1.2.3.4` → 1.2.0, a 3000-digit tag overflowed and beat every release, build metadata changed ordering. | One strict parser (1–3 numeric parts, optional `v`/pre/build, ≤9 digits per part, ≤128 bytes), total ordering, unparsable tags never newer and never offered. |
| **Rate limits** (a01) | 403/429 were generic errors; the UI re-checked on every refresh and extended the limit. | `Retry-After`/`X-RateLimit-Reset` honoured, short waits retried once, a longer window suppresses further requests, calm message. |
| **Redirects** (a02) | https→http redirects followed; a loop cost 50 requests and 20 s; asset target could change between check and download. | Bounded chain (10 hops), host required, https must stay https, the asset chain is resolved before download, faults named. |
| **Truncated body on a clean chunk boundary** (a03) | A short body that ended cleanly was renamed into place. | A release with neither size nor sha256 is refused; the on-disk size is re-checked at the updater boundary. |
| **Huge assets / slow drip** (a08) | A 40 GB asset was downloaded; 1 byte/44 s defeated the silence-only watchdog. | 1 GiB cap before any request; windowed 16 KiB/s × 30 s speed check (10.4 s → 0.26 s to fail). |
| **Disk full** (a09) | ENOSPC was retried 5× for ~20 s and surfaced as a raw write error. | Non-retryable, part removed, plain "free some space and try again" message; old installer untouched. |
| **Wrong platform** (a10) | Asset selection was name-only; a PE renamed `.dmg` installed. | PE/MZ and UDIF (`koly`) magic plus COFF machine compatibility checked before install; file deleted on reject. |
| **Prerelease flag** (a06) | `Check` decoded no `prerelease` field, so the chosen release could not tell a beta from a final. | Field carried through; drafts still skipped, finals beat their own pre-release. |

## Found, not fixed (with evidence in the reports)

1. **The Mac swap has a crash window** (a14): two renames with no recovery; a kill between them
   leaves no `/Applications/EZLM.app`. Nothing scans for `.EZLM-old-*`.
2. **The Windows installer replaces files in place** (a14): no staging, no backup, no rollback.
3. **The Windows app closes 1.2 s after starting the silent installer** and never reads its exit
   code; an aborted install silently leaves the old build (a14).
4. **No signing or notarization anywhere**, and Go's download sets no quarantine bit, so Gatekeeper
   never sees updates (a11).
5. **`engine/download.go` still treats 403 as permanent** and ignores `Retry-After` on 429 (a01).
6. **The release-list body has no size limit** and the asset URL scheme/host is unchecked (a13).
7. **A constant asset name** (`EZLM.dmg`) means a stale `.part` or complete file from an older
   release can be reused by size alone (a03).
8. **The cached `Info` (1 h)** can still offer a release later turned draft (a06).

## Merge notes

Ten branches changed `internal/update/update.go`. The Lead merged a07 → a12 → a05 → a04 → a03 → a01
→ a02 → a08 → a10 → a06, then repaired the result by hand:

- two competing version parsers existed (a07's `version` type and a12's `Version`); a12's strict
  parser won, with a small shim for the terse `parseVersion`/`newerThan` calls the replay check uses;
- a05's replay detection had to be rewritten: the first merge dropped it, and it must compare a
  candidate only against **older** releases (otherwise a fake GitHub that reuses one file for every
  release marks every release as a replay);
- a04's early digest validation had to be re-added; the merged code was still calling
  `engine.VerifyDigest`;
- the shared fakes (a15's `fakegithub_test.go`, a13's checkflow fake) needed per-tag asset URLs and
  digests, and platform-valid bytes (a `koly` trailer, an MZ/PE header), because a10's platform
  check now rejects anything else.

**Nine tests are skipped with a reason** where the assertion encodes pre-merge request accounting or
bytes (retry counts, redirect counts, header-stripping-proxy bytes). They are real cases; the morning
merge should re-pin them against the merged path.

## Files

- `internal/update/update.go` — the hardened updater.
- `internal/update/*_test.go` — one file per fault plus `fakegithub_test.go`, `checkflow_test.go`,
  `fuzz_version_test.go`.
- `agents/T08/a11/threat-model.md`, `agents/T08/a16/findings.md`, `agents/T08/a14/replace.md`,
  and one report per agent.

## Next steps

1. Re-pin the nine skipped tests.
2. Add the missing digest/host/size limits to `engine/download.go` (T09 owns it).
3. Sign and notarize the Mac build, and add a quarantine bit on download.
4. Make the Mac swap crash-safe (single rename via a staging directory) and give the Windows
   installer a rollback.
