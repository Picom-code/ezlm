# T09/a08: disk full partway through a download

## Fault

The disk fills up after the download has already written part of the model. The
write fails, the download should stop with a plain message, the half-written
`.part` file should be cleaned up, and the previous model file must survive.

## Reproduction

Test file: `internal/engine/download_diskfull_test.go` (new, this agent's file).

`TestDownloadDiskFullPartway` uses a real `t.TempDir()`, a `httptest` server
that honours `Range`, and a fake part-file writer (`openPart` seam) whose free
space runs out after 1 MiB and stays out for every later open. The destination
already holds an older model file, and `want` is the new size so the early
"already complete" return does not hide the download.

Before the fix (seam only, no behaviour change):

```
=== RUN   TestDownloadDiskFullPartway
    download_diskfull_test.go:100: error doesn't explain the full disk: write /…/model.gguf.part: no space left on device
    download_diskfull_test.go:106: the partial file was left behind: <nil>
    download_diskfull_test.go:109: a full disk was retried 5 times
--- FAIL: TestDownloadDiskFullPartway (0.03s)
```

Three real defects, all in `internal/engine/download.go`:

1. **Raw Go error shown to the caller.** A failed write was returned unwrapped
   from `fetch` (`download.go:172` before the fix) and then fell through
   `plainNetErr`, which does not recognise disk errors, so the user saw
   `write …: no space left on device` instead of an explanation and next action.
2. **Partial file left behind.** `Download`'s retry loop only removed `.part`
   on a wrong final size (`download.go:93`), never on a failed write. A model
   download that died at 40 GB left a 40 GB `.part` file on the already-full
   disk, so the next attempt starts with less space than the failed one.
3. **Five pointless retries.** A full disk was treated like a flaky network:
   `Download` retried five times with backoff (`download.go:78`). Each retry
   reopened the file and failed immediately. A disk-full condition does not
   heal in two seconds.

`TestDownloadDiskFullAtOpen` covers the same fault when the disk is too full to
create the `.part` file at all; the old code also retried five times and
returned the raw `open …: no space left on device`.

The old model file was not damaged in either case before the fix: writes only
go to `.part` and `dst` is replaced by a rename after a complete transfer.

## Fix

`internal/engine/download.go`, minimal and local to the download path:

- `openPart` seam (lines 37-46): `fetch` now opens the part file through a
  package variable returning a small `partFile` interface (`io.Writer` +
  `io.Closer`). The default is `os.OpenFile`, so production behaviour is
  unchanged. This is what lets a test inject `ENOSPC` partway through a write.
- `diskErr` (lines 227-231): open and write failures are wrapped in this
  type, so `Download` can tell them apart from network errors. It unwraps to
  the original `*os.PathError`.
- `Download` (lines 81-87): a `diskErr` is terminal. It removes `part` and
  returns `saveErr(err)`; no retry and no backoff.
- `saveErr` (lines 233-240): plain message. `ENOSPC` becomes "there isn't
  enough free space on this computer to save the model. Free up some space and
  try again"; any other save failure says to check the folder is writable. The
  original error stays wrapped, so `errors.Is(err, syscall.ENOSPC)` still works.

## After

```
=== RUN   TestDownloadDiskFullPartway
--- PASS: TestDownloadDiskFullPartway (0.01s)
=== RUN   TestDownloadDiskFullAtOpen
--- PASS: TestDownloadDiskFullAtOpen (0.00s)
```

The tests assert: one request (no retry), `.part` gone, the old destination
file byte-for-byte unchanged, a message containing "free space", and
`errors.Is(err, syscall.ENOSPC)`.

Checks run in the worktree: `gofmt -l .` (empty), `go build ./...`,
`go vet ./...`, `go test -p 2 ./internal/engine/` (all pass),
`GOOS=windows GOARCH=amd64 go build ./...`,
`GOOS=windows GOARCH=arm64 go build ./...`, `GOOS=linux go build ./...`
(all pass; `syscall.ENOSPC` exists on all three).

## Residual risk

- **Resume state is discarded.** On a disk-full failure the `.part` file is
  removed, including bytes that were valid from earlier attempts, so a later
  retry starts from zero. This is deliberate (it frees space on a full disk)
  but costs a re-download when the user frees space later. If resume after a
  disk-full failure matters more than space, the alternative is to keep `.part`
  and only report the failure; that decision belongs to the task owner.
- **Other disk errors are now terminal too** (permission denied, read-only
  folder, I/O error). Retrying those was also pointless, but the message for
  them is generic ("check that the folder is writable").
- **Not covered:** a disk that fills up between the last write and the final
  `os.Rename` (rename usually needs no data blocks, so this is rare), and a
  quota that is hit during `os.Rename` or `os.Stat`. Those still surface as
  raw errors.
- **Not covered:** the size-check path (`want > 0`, wrong final size) already
  removes `.part` and returns a plain message, but that is a different fault
  (a03/a07 territory).
- **Interaction with other agents:** the `openPart` seam is package-level and
  mutable, like the existing `retryStep` and `stallTimeout` seams. Tests that
  swap it must not run in parallel with other download tests (none do today).
  The fake writer is local to `download_diskfull_test.go`; helper names are
  prefixed `diskFull` to reduce merge collisions.

## Open questions

- Should a disk-full failure keep the partial file when the `.part` is smaller
  than the model (to resume later), and delete it only when free space is
  critical? Current fix always deletes it, per the fault description.
- Should the app surface free-space guidance (required vs available bytes) in
  the UI? `internal/app` was read-only for this task; the error text is the
  only place that guidance exists today.
