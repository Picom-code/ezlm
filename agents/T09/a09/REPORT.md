# T09 / a09: unicode and long paths

Fault: non-ASCII model names, paths over 260 characters, spaces in names.

Worktree `~/ezlm-agents/work/subs/T09/a09`, branch `agent/T09-a09`.
Owned code: `internal/engine/download.go`. Test file: `internal/engine/download_paths_test.go`.

## What I did

Wrote `internal/engine/download_paths_test.go` (hermetic: `httptest`, `t.TempDir()`, no real
network, no real home). Every test failed before the fix and passes after.

| Test | Before | After |
|---|---|---|
| `TestDownloadNonASCIINameInNewFolder` | fail: `open .../模型 目录/Qwen2.5 7B 日本語 モデル v1.gguf.part: no such file or directory` after 20.0s | pass, 0.00s |
| `TestDownloadNonASCIIResume` | fail: same `no such file or directory` on the `.part` file | pass |
| `TestDownloadLongPath` (334-character path, mixed non-ASCII and spaces) | fail: `no such file or directory` after 20.0s | pass, 0.00s |
| `TestDownloadUnwritableFolderFailsFast` | pass but took 20.0s (the local folder error was retried five times with backoff) | pass, 0.00s |

## Findings

### 1. `Download` never creates the destination folder (blocker)

`internal/engine/download.go:51` (before the fix) built `part := dst + ".part"` and went straight
to `os.OpenFile` at `internal/engine/download.go:158`. Nothing in `Download` created
`filepath.Dir(dst)`, so the first download into a folder that did not exist yet failed with
`no such file or directory`.

This is exactly the non-ASCII case: the folder is the model id
(`internal/engine/engine.go:455-457`, `filepath.Join(paths.Models(), m.ID, ...)`), and a model that
is new to this computer is a folder that is new to this computer. `DownloadModel` happens to call
`os.MkdirAll` first (`internal/engine/engine.go:463-465`), but `DownloadDraft`
(`internal/engine/engine.go:490-493`) does not, so a draft file for a model whose folder was
removed or never created failed permanently. Every other `Download` caller
(`internal/update/update.go:117`, `internal/integrate/tools.go:176`, `internal/app/app.go:651`)
has to know the same rule, and the doc comment on `Download` says nothing about it.

Evidence: the test output above, plus the same failure for a 334-character path (Windows stops at
260 characters without long-path handling, and a path that long is the one most likely to be built
from scratch, since `MkdirAll` is what every caller has to remember).

Severity: blocker. A model with a non-ASCII name in a fresh install is a first-run download, and
it fails with a raw Go error.

### 2. A local folder problem is retried like a dropped connection (major)

Before the fix, an `os.OpenFile` failure went back through the retry loop in
`internal/engine/download.go:57-86`. Nothing there distinguishes an error from this computer's
filesystem from an error from the network, so a read-only folder, a folder path that is a file, or
a full disk cost five attempts and 2+4+6+8 = 20 seconds of backoff before the user saw anything.
Measured: `TestDownloadUnwritableFolderFailsFast` took 20.01s before, 0.00s after.

Severity: major (20 seconds of nothing, then a raw error, on a failure a retry can never fix).
Note for a08 (disk full): `f.Write` errors at `internal/engine/download.go:172` still take the
retry path, so a full disk keeps the 20-second penalty. I left that alone to stay out of a08's
fault; the same `permanent{...}` wrap applies if a08 wants it.

### 3. Non-ASCII names survive the URL and the filesystem (no bug found)

Checked, and guarded by the tests: `m.FileURL` builds a URL with raw UTF-8, `http.NewRequest`
percent-encodes it, and the server decodes it back to the same name (`TestDownloadNonASCIINameInNewFolder`
asserts `r.URL.Path` is the exact name). A `Range` resume against a non-ASCII URL appends to the
same `.part` file byte for byte (`TestDownloadNonASCIIResume`). Spaces in the destination are fine.
`path.Base` in `statusError` (`internal/engine/download.go:238`) operates on the URL, so a name
with spaces appears in the error message unchanged.

## What I changed and why

`internal/engine/download.go`, two small changes:

1. `Download` now calls `os.MkdirAll(filepath.Dir(dst), 0o755)` before touching the `.part` file,
   and returns that error directly. This makes the folder rule part of `Download` instead of
   something each caller has to remember, and fixes `DownloadDraft` without editing `engine.go`.
   `DownloadModel`'s own `MkdirAll` is now redundant but harmless; removing it would be a change
   in `engine.go`, which T06 owns, so I left it.
2. `fetch` returns `permanent{err}` when `os.OpenFile` fails, so a filesystem problem that a retry
   cannot fix is reported at once instead of after 20 seconds. `permanent` already means "retrying
   can't fix this"; I extended its doc comment to say so.

Nothing else moved: retry counts, stall watchdog, Range handling and error wording are untouched.

## Not finished / residual risk

- Windows long paths are reasoned about, not run: this machine is macOS (PATH_MAX 1024), so the
  334-character test proves nothing truncates our own code, and the actual `\\?\` handling is Go's
  `os` package. Worth one run of `go test ./internal/engine/` on a Windows box before the beta.
- Windows rejects `: * ? " < > |` in names. A model id like `Qwen2.5:7b` would build a path that
  Windows cannot create, and that is decided in `catalog`/`paths`, not in `download.go`. Not tested
  here; noted for whoever owns model ids.
- macOS normalises non-ASCII file names (NFD), so a name that is non-ASCII on disk may not compare
  byte-for-byte with the same string in Go. Nothing in `download.go` compares names, so no action,
  but a checksum-by-name scheme elsewhere could trip on it.

## Open questions

- Should `Download` keep `MkdirAll` or should the rule stay with callers? I chose `Download`
  because `DownloadDraft` proves callers forget. If T06 prefers callers, `DownloadDraft` needs the
  call.
- `permanent{err}` around `os.OpenFile` also covers a full disk. If a08 wants the disk-full wording
  from `plainNetErr`, they should wrap the write error the same way and keep the text.

## Before the last commit

`gofmt -l .` empty, `go build ./...`, `go vet ./...`, `go test -p 2 ./internal/engine/` all pass
(see the commit message for the run).
