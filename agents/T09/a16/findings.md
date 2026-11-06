# T09 download faults: severity table (a16)

Status of every T09 fault against the baseline snapshot (`5213370`) of
`internal/engine/download.go`. This table is meant to stand alone: each row
names the code path, the evidence (file:line or a run probe), the severity and
the fix. Agent columns are current as of this writing: all fault worktrees were
still on the baseline commit, so status here is the status of the code, not of
another agent's patch.

Severity:

- **blocker** - silent data corruption, or a failed install that never
  recovers, on a path users hit.
- **major** - the download fails or wastes a lot of work in a realistic
  condition, or the user gets a confusing error.
- **minor** - rough edge, no data loss.

Status:

- **fixed** - code handles it and a test in this tree proves it.
- **mitigated** - code handles it, but no test proves it, or the handling is
  incidental (for example Go's HTTP client doing the work).
- **open** - the code does not handle it.

| # | Fault | Status | Evidence | Severity | Fix |
|---|---|---|---|---|---|
| a01 | 302 to CDN, and a redirect target that changes on resume | mitigated | `download.go:132` uses `http.Client.Do` with the default redirect policy (10 hops). Go copies `Range` to the redirect target for a different host (`Range` is not in the sensitive-header list), so a CDN resume works. A changed target that ignores `Range` falls into the 200 path at `download.go:145-147`, which truncates and restarts. No test exercises a redirect. | minor (untested) | Add `httptest` 302 -> CDN test with a Range-aware second hop, and a test where the second hop changes to a different file. The latter overlaps a03. |
| a02 | server ignores `Range`, sends 200 with the whole body | mitigated | `download.go:145-147`: `StatusOK` sets `have = 0` and `O_TRUNC`, so the part file is rewritten from byte 0. No test sends 200 to a Range request. | minor (untested, wastes one full download) | Test: first attempt breaks at half, resume gets 200 + full body; assert the final file is correct and only one extra full body was read. |
| a03 | ETag changes mid-download; resume must detect and restart | **open** | `download.go:129-131` sends `Range` but never `If-Range`; no validator is stored next to `.part`. `download.go:143-144` appends any 206 to the old bytes. Probe run by a16 (not committed): first attempt returns 32 of 64 bytes with `ETag: "v1"` then drops; second returns 206 with `ETag: "v2"` and 32 bytes of different content. `Download` returned nil and wrote `32 x "A" + 32 x "B"` - a corrupt file whose size matched `want`, so the size check at `download.go:87-96` passed. | **blocker** (silent corruption of a multi-GB file; also poisons a later checksum, a07) | Store the first response's `ETag` (and `Last-Modified` fallback) in `<part>.etag`; send `If-Range: <etag>` on resume; when the server answers 200, truncate and restart (already the 200 path); delete the stored validator when the part file is removed. Add `download_etag_test.go`. |
| a04 | stalls and slow drips; watchdog must fire but not on a slow-but-alive link | mitigated | Watchdog at `download.go:105-123`: a ticker at `stallTimeout/9` cancels the request when `last` (updated on every read with `n > 0`, `download.go:171`) is older than `stallTimeout` (45 s, `download.go:21`). A slow drip keeps `last` fresh, so it does not fire - correct by design, but untested. `download_stall_test.go:19` and `:66` prove the stall case over HTTP/1.1 and HTTP/2. The watchdog is armed before `Do`, so it also covers a hanging connect/header phase, though `ResponseHeaderTimeout` (60 s) usually wins. | minor (slow-drip boundary untested) | Add a slow-drip test (1 byte every `stallTimeout/3`) asserting no stall, plus a boundary test at just over `stallTimeout`. a14 owns the threshold review. |
| a05 | connection resets at offset 0, mid-header, mid-body, at the last byte | mitigated | A read error becomes `dropped` (`download.go:188-192`) and is retried by the loop at `download.go:57-86`, resuming from the part file's size. Mid-header/mid-connect errors come back from `Do` at `download.go:132-138` and are retried the same way. `download_neterr_test.go:55` proves 12 consecutive mid-body drops still finish with the exact bytes. Offset 0 and mid-header are not tested separately; a clean EOF at the last byte returns success (`download.go:182-186`). | minor (offset-0 and mid-header cases untested) | Add one test that hijacks before writing headers, and one that drops at byte 0; both should finish. |
| a06 | split GGUF: all parts present, missing part, wrong-size part, out of order | **open** | `DownloadModel` (`engine.go:461-484`) passes `want = 0` for every file when `len(q.Files) > 1` (`engine.go:465-467`), so the size check at `download.go:87-96` is skipped for split models. A part with the wrong size is accepted, renamed and later fails inside llama.cpp. Parts are fetched sequentially in `q.Files` order; out-of-order is not exercised. A missing part is a 404 -> `permanent` (`download.go:240-241`) and fails cleanly. Note the fix lives in `engine.go`, which T06 owns; a06 can only add tests unless that is coordinated. | **major** (truncated split model installs and then fails to load) | Give each part a known size (catalog or HEAD) and pass it as `want`, or verify the total after the loop; add `download_parts_test.go` for missing/wrong-size/order. |
| a07 | checksum failures: wrong SHA, missing SHA, wrong-length SHA | **open** | `download.go` has no digest parameter at all; the only checksum is `verifyDigest` (`engine.go:317-339`) applied to llama.cpp release assets in `Install` (`engine.go:248-255`), never to model files. On mismatch the bad archive is left on disk; the next run's `Download` sees `dst` exists with the right size (`download.go:45-50`) and returns nil without fetching, so `verifyDigest` fails again forever - a poisoned install. A digest without the `sha256:` prefix silently returns nil (`engine.go:321-325`), so a missing SHA passes. Wrong-length hex is compared as a string and reported as a mismatch, with no length validation. | **blocker** (checksum mismatch is unrecoverable without deleting the file by hand; missing SHA is silent) | Delete the file on mismatch (or rename to `.bad`) before returning, so the retry re-downloads; validate the hex length and return a clear error; add `download_checksum_test.go`. Also consider a digest in `Download` for model files. |
| a08 | disk full partway: write fails, partial cleaned up, old file survives | **open** | `download.go:172-174` returns the `Write` error as a plain error, so `Download` retries 5 times (`download.go:57-86`), each time reopening and appending, before giving up - pointless work on ENOSPC. The `.part` file is left behind (never removed), and the error is Go's raw `write ...: no space left on device`, because `plainNetErr` (`download.go:210-234`) matches only net/x509 errors. The old `dst` does survive, because the rename happens only on success (`download.go:97`). | **major** (confusing error, five retries, leftover part file; the next run resumes into the same full disk) | Wrap write/close errors as `permanent` (retrying cannot fix ENOSPC), remove the `.part` on a write failure or leave a resumable part and explain it, and add a plain-words ENOSPC branch. Test by injecting a writer or using a small filesystem. |
| a09 | unicode and long paths, spaces | **open** | `Download` passes the path straight to `os.Stat`, `os.OpenFile` and `os.Rename` (`download.go:45-55`, `158`, `97`); there is no Windows long-path (`\\?\`) handling, so a model directory over 260 characters fails on Windows. Non-ASCII and spaces are fine on macOS and on Windows with the default UTF-16 APIs, but untested. `path.Base(strings.SplitN(url, "?", 2)[0])` (`download.go:238`) is only used for messages. | **minor** on macOS, **major** on Windows for deep model paths | Add a `longPath` helper for Windows and a test using `t.TempDir()` with a long, non-ASCII name (skip the >260 case off Windows or test the helper directly). |
| a10 | two downloads of the same file at once: no corruption, no duplicate work, clear result for both | **open** | `Download` has no per-destination lock. Two callers share `dst + ".part"` and both write it. Probe run by a16 (not committed): a half-written `.part` plus two concurrent `Download` calls produced one `nil` result and one spurious error - `the downloaded file came out the wrong size (32768 bytes instead of 65536), so EZLM deleted it` - because the resuming caller checked the size after the other caller had truncated and renamed the part file. Both callers also fetched the whole file, so the work is duplicated. Corruption is timing dependent but the spurious failure and duplicate bandwidth are reproducible. | **major** (a user clicking twice gets a false failure; the losing caller can delete the shared part file under the winner) | Add a per-destination singleflight/lock (`sync.Map` of path -> `*sync.Mutex`, or a channel) so the second caller waits and shares the result; add `download_concurrent_test.go` asserting both get the same result and one body was fetched. |
| a11 | resume after the app restarts; stale state file must not corrupt | mitigated | The part file is the only state; `Download` stats it (`download.go:51-55`) and sends `Range` from its size. A restart therefore resumes correctly, and a stale part with a different file behind the URL is exactly the a03 hole. There is no state/validator file, so nothing else can go stale. A stale `.part` larger than `want` triggers a 416 -> `return nil` (`download.go:148-149`), then the size check deletes it and reports the wrong-size error; a stale `.part` smaller than the real file appends silently. | **major** until a03 lands (the a03 validator file is the fix) | Land a03's validator; on 416 with `have != want`, delete the part and restart once. Test: write a part file, new `Download` call, correct final bytes. |
| a12 | shared fake HF/CDN server | pending | `internal/engine/fakehuggingface_test.go` did not exist in any worktree at the time of writing. The existing tests each hand-roll their own handler (`download_neterr_test.go:59`, `download_stall_test.go:26`). | minor (tooling) | a12 to land the fake; a16 to re-check that each fault test uses it rather than a fifth copy. |
| a13 | progress and observability: bytes, speed, ETA, error vs stall | **open** | `Progress(done, total, bytesPerSec)` (`download.go:39-40`) reports at most every 200 ms (`download.go:177-180`) and once at EOF (`download.go:183-186`). For a split model `DownloadModel` passes `want = 0` (`engine.go:465-467`), so `total` is 0 until a `Content-Length` arrives, and `q.Bytes` is used as the total across parts (`engine.go:475`) while per-file `want` is unknown. A stall and a drop are distinguishable in the final error (`errStalled` -> "keeps stalling", `dropped` -> "keeps dropping", `download.go:222-231`), but not during the retry backoff, which reports nothing. | minor | Document `total == 0` as unknown; emit a progress event when a retry starts so the UI can say "reconnecting". Tests belong to a13. |
| a14 | watchdog review: thresholds, false positives, retry interaction | mitigated | 45 s `stallTimeout` with a 5 s ticker (`download.go:21`, `109`); `last` updates on every read. A retry resets the watchdog because each `fetch` builds a new one. The retry policy gives up after 5 failures with no progress and resets the count whenever the part file grows (`download.go:71-80`), so a flapping link that keeps delivering never exhausts retries. Tests mutate the package-level `stallTimeout` (`download_stall_test.go:20-22`), which would race if any download test ran in parallel; none do today. | minor | Consider a test hook instead of the global; add a boundary test at `stallTimeout` +/- one tick. a14 owns the numbers. |
| a15 | memory and CPU on a 20 GB fake download | pending | Not measured yet. Code path allocates one 1 MiB buffer per active download (`download.go:164`) and never buffers the file, so memory should be flat. | minor | a15 to land the benchmark and numbers. |
| a16 | severity table and report support | fixed | This file and `REPORT.md`. | - | Keep current as fault agents commit. |

## Cross-cutting findings not owned by a single fault

1. **416 is treated as success** (`download.go:148-149`). If `have` is not the
   final size (stale or truncated part, or a server that answers 416 for an
   empty file), `fetch` returns nil, and `Download` either renames a
   wrong-size part over `dst` when `want == 0`, or reports the wrong-size
   error after deleting the part. Fix: on 416, compare `have` with `want`; if
   they differ, truncate the part and fetch again once.
2. **A `dst` that already exists is trusted when `want == 0`**
   (`download.go:45-50`). Combined with the missing checksum (a07), a
   truncated model file is never re-fetched. Fix: for model files, verify the
   size from the catalog (a06) or a digest.
3. **A short body with no `Content-Length` is success** (`download.go:182-186`).
   A server that closes cleanly after half the file, without a length, produces
   a short file with no error when `want == 0`. Fix: same as 2.
4. **Global knobs** (`stallTimeout`, `retryStep`) are package variables
   (`download.go:21`, `37`) mutated by tests. Fine today, but they block
   `t.Parallel` and hide intent. a12's fake could expose them per test.
5. **Redirect target changes are invisible**: the code never records the
   final URL or validator, so a CDN switch and a file replacement look the
   same. a03's validator file also fixes this.

## What a16 verified, and how

- Read `internal/engine/download.go` in full and the three existing tests
  (`download_neterr_test.go`, `download_stall_test.go`,
  `download_status_test.go`), plus `DownloadModel`/`verifyDigest` in
  `engine.go` and the call sites in `internal/app` and `internal/update`.
- Ran `go test -p 2 ./internal/engine/` on the baseline: passes in 7.5 s.
- Ran two throwaway probes (not committed) for a03 and a10; their exact
  output is quoted in the rows above.
- `git log --oneline -1` in every `work/subs/T09/a*/` worktree: all on
  `5213370`, so no fault agent had committed a fix yet.

## Open questions

- Does T09 own the fix for a06/a07, which live in `engine.go` (T06's file)?
  The brief says T09 owns `download.go` only, but `DownloadModel` and
  `verifyDigest` are where the multi-part and checksum holes are. The Lead
  should decide before a06/a07 patch `engine.go`.
- Should `Download` grow a digest parameter, or should callers verify and
  delete? The latter needs the "delete on mismatch" fix in `engine.go` and
  leaves the window where a killed process keeps a bad file.
- Is a same-size file replacement behind the same URL realistic for the HF
  CDN? If not, a03 drops to major; the corrupt-file path still needs a test.
