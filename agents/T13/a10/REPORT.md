# a10 report: TROUBLESHOOTING for `internal/scan`, `internal/fit`, `internal/catalog`

Section: `agents/T13/sections/a10.md` (about 2,100 words).
Format: one entry per real message, each with the exact text, what it means, and
what to do. Where a failure has no message at all, the entry says so and
describes what the tester sees instead, because most failures in these three
packages are silent by design.

## What I covered

`internal/scan`: the two notes the scan writes, plus every silent fallback that
changes what the user sees (card skipped by the adapter rules, `nvidia-smi`
missing or failing, memory and disk readings at zero, live graphics memory at
zero before the first scan), and the two errors a close request can return.

`internal/fit`: the four "doesn't fit" sentences the app builds from an empty
plan, the low-disk path, and the slow-plan label. The fit package has no
message strings of its own, so each entry cites both the fit condition and the
place the sentence is written.

`internal/catalog`: the one error string, the startup panic, and the three
silent fallbacks (bad `EZLM_CATALOG` file, failed remote fetch, remote catalog
with an equal or older version).

## Findings

1. **Major, docs.** The two scan notes never reach a tester using an app. Both
   apps declare the field (`windows/EZLM/Models.cs:28`,
   `macos/Sources/EZLM/Models.swift:39`) and neither renders it; only
   `cmd/ezlm/main.go:376` prints it. "No supported GPU found" is therefore
   invisible in the Windows and Mac apps, which is exactly the case a
   non-technical tester needs explained. Either the apps should show the notes,
   or the guide should describe the symptom (slow replies, no card listed)
   instead of the text.
2. **Major, silent failure.** Every catalog failure is silent
   (`internal/catalog/catalog.go:148` to `181`). A tester who is offline, or
   whose firewall blocks `raw.githubusercontent.com`, gets the built-in list and
   no hint that a newer one exists. The message "catalog has no models"
   (`internal/catalog/catalog.go:196`) is effectively unreachable: a bad
   `EZLM_CATALOG` file is swallowed, and the embedded copy is checked at
   startup, where a failure is a panic.
3. **Major, wrong reassurance.** Unknown disk space reads as "enough room".
   `internal/fit/fit.go:321` treats `DiskFree == 0` as fine, and
   `internal/scan/scan.go:280` to `289` returns zero when no folder on the path
   can be read. So a failed disk reading removes the low-disk warning and the
   download can fill the drive. Worth a proposed wording for a14.
4. **Minor.** The fit context ladder stops at 65536 (`internal/fit/fit.go:92`),
   but two call sites set the floor to the running config's context unchanged
   (`internal/app/app.go:534`, `internal/app/app.go:1142`). A hand-edited
   config with a larger context makes every candidate fail and the app reports
   "doesn't fit this machine right now". No UI path sets a context above 65536,
   so this is a hand-edited-config case only.
5. **Minor.** Close errors are raw: "process does not exist" comes from the
   process library, and "operation not permitted" is `os.ErrPermission`
   (`internal/scan/scan.go:260` to `266`). Both are shown as the note in a
   dialog (`windows/EZLM/Shell.cs:495`, `macos/Sources/EZLM/AppModel.swift:339`)
   with no advice attached. The guide supplies the advice, but a plain "It was
   already closed" would be better.
6. **Minor.** On Windows, an AMD or Intel card is dropped without a message
   when its name does not match the discrete pattern or it reports under 2 GB
   (`internal/scan/gpu_discrete.go:203` to `224`). The common case, built-in
   graphics with a small share, is deliberate, but the tester only sees "no GPU
   found" plus a processor-speed model.

## What I could not verify

- No GPU hardware was available, so the scan branches were read, not run. The
  tests in `internal/scan` (`TestUsableAdapters`, `TestReadsThisPCsGraphics`)
  cover the adapter rules and were not extended, since this task is docs only.
- The exact on-screen rendering of the close errors belongs to a12. I cite the
  dialog note assignment but did not confirm how the note is laid out.
- Whether any release ever ships a catalog with a version lower than the
  embedded one (the "stale remote" branch) is unknowable from here.

## Open questions

- Should the apps display `Hardware.notes`? If yes, this section's first two
  entries become visible text and a12 should own the layout.
- The four fit sentences interpolate the model name and file name, so the guide
  cannot quote them literally for every model. I kept the shape and used one
  example; the lead may prefer a generic form.

## Checks

Docs only, no Go changes. `gofmt -l .` empty and `go build ./...` passes in
the worktree.
