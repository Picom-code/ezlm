# T05/a12 locale review: report

Slice: locale review of the Windows scan code and the Windows C# call sites.
Full findings, evidence and severity: [locale-review.md](locale-review.md).

## Result

- Read `internal/scan/*_windows.go` (`display_windows.go`, `apps_windows.go`,
  `exec_windows.go`), the Windows-only paths in `gpu_discrete.go`, and the
  scan-reading C# under `windows/EZLM`.
- Two real parser bugs, both fixed with failing-then-passing tests in
  `internal/scan/locale_review_test.go`:
  - `parseGPUUsage` dropped byte counts formatted for the user's locale
    (German `1.073.741.824`, French no-break-space groups, decimal comma) and
    reported zero graphics memory instead. New `localeInt` helper.
  - `splitCSV` split quoted fields containing a comma (a Windows program
    path), shifting nvidia-smi's columns.
- Not fixed, reported: the `Get-Counter` fallback uses a localized counter
  path, so per-process GPU memory is empty on German and Japanese Windows
  when the WMI query fails, and the card then looks completely free.
- C# side is clean: invariant case folding, `OrdinalIgnoreCase` comparisons,
  `System.Text.Json` number parsing; `Fmt.Gb` is display-only.
- No blockers. `go test -p 2 ./internal/scan/` passes; `gofmt -l .` empty.
