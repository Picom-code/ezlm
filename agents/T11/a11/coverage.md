# T11 coverage report (agent a11)

Coverage of the Go engine's `internal/...` tree, measured before and after the T11 test-writing
agents land their branches. This is the number the morning merge looks at.

Method: `go test -p 2 -coverprofile=... -covermode=count ./internal/...` on `agent/T11-a11`
(baseline `5213370`, the source snapshot), then `go tool cover -func` for the total and the
profile for the per-package statement counts. The "after" numbers are measured the same way on a
scratch copy of the tree with the other agents' committed `*_test.go` files added by `git show`
(no branch is merged, no worktree is edited). Reproduce with:

```sh
go test -p 2 -coverprofile=/tmp/cover.out ./internal/... && go tool cover -func=/tmp/cover.out | tail -1
```

## Summary

| | Statements | Covered | Coverage |
|---|---|---|---|
| **Before** (baseline `5213370`) | 7553 | 3973 | **52.6%** |
| **After** (a01-a09, a12-a15 committed; a10 in flight) | 7553 | 4343 | **57.5%** |

The coverage drive added 370 covered statements, a rise of 4.9 points on a tree where 85% of the
remaining uncovered code is outside T11's scope. Across the packages T11 owns (catalog, keys, paths,
prereq, report, version, web, bench, awake, proc: 552 statements) coverage went from 121/552 (21.9%)
to 488/552 (88.4%).

## Per package

`before` and `after` are statement coverage. "Still uncovered" says what is left and why.

| Package | Before | After | Still uncovered and why |
|---|---|---|---|
| internal/app | 361/1890 (19.1%) | 361/1890 (19.1%) | Not in T11's scope, and on the do-not-touch list. Only hand-built fakes reach it. This is the single biggest gap (1529 statements). |
| internal/awake | 13/14 (92.9%) | 13/14 (92.9%) | a15's `pdeath_other.go` neighbour: one OS-specific branch. |
| internal/bench | 54/112 (48.2%) | 111/112 (99.1%) | One branch of `chat` (the error return from a failed request build). a12 covers the parsing and formatting paths with table tests. |
| internal/catalog | 0/56 (0.0%) | 56/56 (100%) | Nothing. a01 covers `Load`, `parse`, `sortModels`, `Find`, the `Model`/`Draft` URL and memory helpers, malformed JSON and missing fields. |
| internal/engine | 131/392 (33.4%) | 131/392 (33.4%) | Not in T11's scope. a13 only hardened the existing download tests against flakes; it added no cases. 261 statements are in the download/verify paths that need a fake HTTP server. |
| internal/fakellm | 164/180 (91.1%) | 164/180 (91.1%) | Not in T11's scope, and on the do-not-touch list. |
| internal/fit | 198/240 (82.5%) | 198/240 (82.5%) | Not in T11's scope. |
| internal/integrate | 2778/3437 (80.8%) | 2778/3437 (80.8%) | Not in T11's scope, and on the do-not-touch list. |
| internal/keys | 0/8 (0.0%) | 7/8 (87.5%) | The `rand.Reader` error branch, which panics. It cannot be reached without replacing `crypto/rand`, and the package takes no reader, so the branch stays untested by design. |
| internal/paths | 0/44 (0.0%) | 44/44 (100%) | Nothing. a03 covers the helpers, the settings file and the temp/`EZLM_HOME` overrides. |
| internal/prereq | 0/85 (0.0%) | 81/85 (95.3%) | `hide` on the non-Windows build, one branch of `vcArch`, `Run`'s Windows-only path and `FixVCRedist`'s download-error path. All need a Windows build or a fake executable on `PATH`; a04 leaves them to the cross-build. |
| internal/proc | 45/53 (84.9%) | 51/53 (96.2%) | `setPdeathsig` (Linux-only) and `release` in `proc_unix.go`, both OS-specific. a15 covers the rest. |
| internal/report | 9/33 (27.3%) | 33/33 (100%) | Nothing. a05 covers `token` redaction, `Send` against `httptest`, and the URL trimming. |
| internal/scan | 91/515 (17.7%) | 91/515 (17.7%) | Not in T11's scope. The hardware probes shell out to `nvidia-smi`, `sysctl` and friends and have no fakes, so 424 statements stay cold. |
| internal/serve | 109/285 (38.2%) | 109/285 (38.2%) | Not in T11's scope. a14 only fixed a handler-state leak in the existing test; no new cases. |
| internal/update | 20/62 (32.3%) | 20/62 (32.3%) | Not in T11's scope. |
| internal/version | 0/0 (no test files) | 0/0 (no statements) | a06 added `version_test.go`. The package is one `const Version` (5 lines, no statements), so `go test -cover` reports "[no statements]" and it contributes nothing to the total. Treat the 0% as "nothing to cover", not as a gap. |
| internal/web | 0/147 (0.0%) | 92/147 (62.6%) | a10 (remaining routes, static files, 404/405) was still in flight at measurement time. Left today: parts of `Serve` (53%), `events` (50%), `Listen` (86%), `hostCheck` (89%), and the routes a10 owns. |

## Where the remaining gap is

Of the 3210 statements still uncovered, T11's own packages account for 64 (web 55, prereq 4, keys 1,
bench 1, proc 2, awake 1). The remaining 3146 are outside T11's scope:

| Area | Uncovered | Why it stays |
|---|---|---|
| `internal/app` | 1529 | Rewrite area; the do-not-touch list. Needs a fake backend to drive the pane logic. |
| `internal/integrate` | 659 | Rewrite area; the do-not-touch list. |
| `internal/scan` | 424 | Hardware probes with no fake executables. |
| `internal/engine` | 261 | Download/verify paths needing a fake HTTP server. |
| `internal/serve` | 176 | Not assigned; a14 was a flakiness review, not a coverage drive. |
| `internal/fit`, `update` | 84 | Not assigned. |
| `internal/fakellm` | 16 | Do-not-touch list. |

A follow-up on `app`, `scan` and `engine` is where the next 10 points are. T11's own packages are
effectively done: only 64 statements remain, and 55 of those belong to a10's routes.

## Caveats

- `-cover` counts statements, not behaviour: a table test that only calls a function can inflate it.
  The per-package "after" column is the honest signal for the morning merge.
- `internal/version` has no statements, so it is excluded from the total rather than counted as 0%.
- Coverage was measured on darwin/arm64. Windows-only files (`internal/prereq/hide_windows.go`,
  `internal/proc/proc_windows.go`) do not appear in a darwin profile, so prereq's denominator here
  is 85 statements and proc's is 53, not the cross-platform figures.
- The "after" snapshot is a scratch-tree reconstruction, not a merge. If the morning merge drops or
  renames a test file, the number moves with it.

## Bugs found while measuring

None from a11: this slice writes no tests. Bug reports, including the `t.Skip("bug: ...")` cases
from a01 to a10, are collected in `agents/T11/a16/findings.md`.
