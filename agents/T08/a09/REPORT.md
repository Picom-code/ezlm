# T08 a09: disk full

**Fault:** the update disk fills in the middle of the download.
**Reproduced:** yes. **Fixed:** yes. **Severity:** major (bad failure mode, no data loss).

## What happens now

`Download` stages the installer at `os.TempDir()/EZLM-update/<asset>` and
`engine.Download` writes `<asset>.part`, renames it to `<asset>` only after the
size matches, and `update.Download` deletes it if the digest is wrong. A full
disk therefore cannot touch the installed app. But when `f.Write` returns
`ENOSPC`, `engine.fetch` returns a plain error and `engine.Download` treats it
like a dropped connection: it sleeps 2+4+6+8 seconds (20 seconds total), retries
five times against a disk that is still full, then returns the raw syscall text
(`internal/engine/download.go:57-86`). That text is shown as the update step's
failure message (`internal/app/settings.go:52-58`), and the half-written
`.part` file is left in temp.

Before, against a 1 MiB disk image with a 2 MiB asset:

```
=== RUN   TestDownloadDiskFull
    diskfull_test.go:98: Download spent 20.021035625s retrying a full disk
    diskfull_test.go:101: error does not name the next action: write .../EZLM.dmg.part: no space left on device
    diskfull_test.go:113: a partial download was left behind at .../EZLM.dmg.part
--- FAIL: TestDownloadDiskFull (32.04s)
```

After:

```
--- PASS: TestDownloadDiskFull (1.42s)
```

The old installer file stayed byte-for-byte identical in both runs, so the
"leave the old install intact" invariant already held; the test now pins it.

## Fix

`internal/engine/download.go` (+18/-2, shared file, kept minimal):

- new `writeErr` type wraps failures from `os.OpenFile` and `f.Write` of the
  part file;
- `Download` handles `writeErr` before the network retry loop: it removes the
  partial file and returns at once, because retrying with the same free space
  cannot help;
- an `ENOSPC` failure gets a plain message that names the next action:
  "this computer ran out of space while saving the update, so EZLM deleted the
  partial download; free some space and try again". Other local write errors
  get a generic disk message.

Tradeoff: a local write failure deletes the resumable part. Network failures
still resume as before; a disk-full part is no use until space is freed, and
deleting it keeps the staging directory clean.

## Test

`internal/update/diskfull_test.go` mounts a 1 MiB HFS+ disk image with
`hdiutil` (user level, detached in `t.Cleanup`), points `TMPDIR` at it, serves
a 2 MiB asset from `httptest` (with `Content-Length` and Range support), and
calls `update.Download` with a real SHA-256. It asserts:

- the download fails and returns no path;
- it fails in under 5 seconds (before the fix: 20 seconds);
- the message names the next action ("free some space");
- the previous `EZLM.dmg` is unchanged, byte for byte;
- no `.part` file is left behind.

The test skips on non-darwin hosts (`hdiutil`), and every path it writes is a
temp directory or the temp disk image.

## Residual risk and open questions

- Windows reports a full disk as `ERROR_DISK_FULL` (112), not `ENOSPC`, so a
  Windows user gets the generic "check the disk" message. Still immediate and
  clean, but not covered by a test here (no Windows host).
- `f.Close()` errors are still ignored. The part file is unbuffered, so a full
  disk surfaces at `Write`; only network filesystems could fail at close.
- Should the partial file be kept for resume after the user frees space? The
  fix deletes it, matching the "clean up" requirement. A resume after freeing
  space would work too, but a full-disk part is usually small and restarting
  is simpler.
- `os.Rename(part, dst)` failing (for example on a read-only volume) still
  leaves the part behind; out of this fault's scope.
