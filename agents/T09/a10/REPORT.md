# T09 a10: two downloads of the same file at once

**Fault:** two callers call `engine.Download` with the same destination at the
same time (for example the user starts a deploy twice, or a retry overlaps a
download that is still running).

**Verdict:** reproduced, fixed. Severity: **major**.

## Finding

`internal/engine/download.go:44` (`Download`) had no coordination between
callers. Both callers stat the same `dst+".part"`, both open it (the first
`O_TRUNC`, later ones `O_APPEND`) and both write into it, then both
`os.Rename(part, dst)` (`download.go:97`). Whichever loses the race gets
`rename ...: no such file or directory` even though the file downloaded
correctly, and the file was transferred twice (two full HTTP requests, twice
the bytes and time on a multi-GB model). Because both write the same bytes at
their own file offsets the content usually survives, but two appenders on one
file with independent offsets is not a guarantee.

Evidence, test `TestConcurrentDownloadsOfSameFile` before the fix:

```
--- FAIL: TestConcurrentDownloadsOfSameFile (0.01s)
    download_concurrent_test.go:89: caller 0: rename /tmp/.../model.gguf.part
    /tmp/.../model.gguf: no such file or directory
```

The server had already served the file twice at that point (the test asserts
one request and one copy of the bytes).

## Fix

`internal/engine/download.go`: a per-destination lock, `lockDownload`, keyed by
`filepath.Abs(dst)`, held for the whole `Download` call (including the
resume/retry loop). The second caller waits; when it gets the lock the
existing "file already there and the right size" check at the top of
`Download` returns success and reports final progress. The wait respects the
caller's context, so a cancelled second caller returns `context.Canceled`
without disturbing the transfer in progress. Entries are reference counted and
removed when the last caller leaves, so the map does not grow with model
paths. Only `download.go` changed; no new dependencies.

## Tests (`internal/engine/download_concurrent_test.go`)

- `TestConcurrentDownloadsOfSameFile`: two callers, one 840 KB trickled body.
  Both return nil, both see 100% progress, the file matches, the server sees
  one request and serves the bytes once, no `.part` left.
- `TestConcurrentDownloadsShareFailure`: both callers get the same plain 404
  message and no file is left behind.
- `TestConcurrentDownloadWaiterCancels`: the waiting caller's context
  cancellation returns `context.Canceled` while the first download finishes
  intact.

All three pass after the fix; the first fails before it. `go test -race` on
these tests is clean.

## Residual risk / open questions

- The lock is in-process only. Two EZLM processes (or the CLI and the app)
  downloading the same path would still collide on the `.part` file. A file
  lock (`flock`/`LockFileEx`) would fix that but needs platform-specific code;
  out of scope here.
- Symlinked or differently-spelled paths (`./model.gguf` vs an absolute path)
  normalise through `filepath.Abs`; symlink aliases to the same file still get
  separate locks.
- A waiting caller gets no progress callbacks until it acquires the lock, so a
  second UI progress bar sits at 0 and then jumps to 100. Sharing the active
  download's progress would be a nicer caller experience; it needs a progress
  fan-out and is a follow-up, not part of this fix.
