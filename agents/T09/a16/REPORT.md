# T09 a16 report: severity table and report support

## What I did

- Read `internal/engine/download.go` in full and the three existing download
  tests, plus `DownloadModel` and `verifyDigest` in `internal/engine/engine.go`
  and the call sites in `internal/app` and `internal/update`.
- Checked every T09 fault against the baseline code and wrote the standalone
  severity table in `agents/T09/a16/findings.md`.
- Ran two throwaway probes (deleted before commit) to turn two suspicions into
  evidence: ETag change on resume (a03) and two concurrent downloads of the
  same file (a10).
- Ran `go test -p 2 ./internal/engine/` on the baseline: passes in 7.5 s.
- No production code changed. This slice is a report, as the brief asks.

## Findings

Full table with evidence and fixes: `agents/T09/a16/findings.md`.

Highest severity, in the order the Lead should care:

1. **a03, ETag change on resume - blocker.** No `If-Range` and no stored
   validator. Probe: a 64-byte file replaced between attempts (same size, new
   `ETag`) produced `A x 32 + B x 32`, `Download` returned nil, and the size
   check passed. Silent corruption. Fix: persist `ETag`/`Last-Modified` next
   to `.part`, send `If-Range`, restart on 200.
2. **a07, checksums - blocker.** `verifyDigest` (`engine.go:317`) runs only
   for llama.cpp release assets, never for model files, and on mismatch it
   leaves the bad file in place. The next run's `Download` sees `dst` exists
   with the right size (`download.go:45`) and returns nil without fetching, so
   the install fails on every retry forever. A digest with no `sha256:` prefix
   is silently skipped.
3. **a10, two downloads at once - major.** No per-destination lock. Probe with
   a half-written `.part` and two concurrent `Download` calls: one caller got
   a spurious `the downloaded file came out the wrong size ... EZLM deleted
   it`, and both fetched the whole file. The failing caller can delete the
   part file under the winning caller.
4. **a06, split GGUF - major.** `DownloadModel` passes `want = 0` whenever
   `len(q.Files) > 1` (`engine.go:465-467`), so no part is size-checked. A
   truncated part installs and then fails inside llama.cpp.
5. **a08, disk full - major.** The write error is retried five times, the
   `.part` is left behind, and the user sees Go's raw `no space left on
   device` instead of plain words. The old file does survive, as required.

a01, a02, a04, a05, a11, a14 are handled in code but not all are tested; the
table says which. a12, a13, a15 are other agents' slices, listed so the table
is complete.

## State of the other agents at the time of writing

`git log --oneline -1` in every `work/subs/T09/a*/` worktree showed all of
them still on `5213370`, so the table records the status of the baseline code,
not of a patch that landed later. a03 had an untracked
`download_etag_test.go`, which matches the a03 hole I found.

## What I could not finish

- a08 (disk full) has no reproduction: faking ENOSPC needs a small filesystem,
  and `hdiutil` was too heavy for a 25 minute box. The row cites code paths.
- a09 (long paths on Windows) was read, not run; this machine is macOS.
- a06 and a07 fixes live in `engine.go`, which the brief gives to T06. I only
  recorded them; the Lead needs to decide who patches `engine.go`.

## Next steps

1. a03: validator file, `If-Range`, restart on 200. This also closes most of
   a11's residual risk.
2. a07: delete the file on mismatch and validate the digest length, so a retry
   recovers.
3. a10: per-destination singleflight.
4. a06: per-part size from the catalog, then `want` is no longer 0.
5. Re-run this table once the fault branches land: flip status to fixed only
   when the test is on the branch, and note the commit sha in the row.

## Open questions

- Does T09 own `engine.go` changes for a06/a07? The brief says no; the holes
  are there.
- Is a same-size replacement behind the same URL realistic for the HF CDN? If
  not, a03 is major rather than blocker, but the corrupt-file path still needs
  the test.
