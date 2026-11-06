# T11. Coverage drive for untested packages — consolidated report

16 agents (a01–a16) worked this task in parallel: one per untested package or web-route group, plus a
coverage reporter, two flakiness reviews, and the findings table. Consolidated onto
`agent/T11-coverage` by the Lead.

Goal: real tests for the packages that had none, and a written list of the bugs the tests expose.

## Result

- **Test-only changes** (no logic changes, as the task requires); bugs are recorded as
  `t.Skip("bug: ...")` tests with the owner task named.
- **Coverage of the T11-owned packages: 21.9% → 88.4%** of statements.
- **Gate:** `gofmt -l .` empty, `go build ./...`, `go vet ./...`, `go test -p 2 ./...` pass, plus
  `GOOS=windows` amd64/arm64 and `GOOS=linux` builds.

## Coverage after

| Package | Before | After |
|---|---|---|
| `internal/catalog` | 0.0% | **100.0%** |
| `internal/paths` | 0.0% | **100.0%** |
| `internal/report` | 27.3% | **100.0%** |
| `internal/bench` | 48.2% | **99.1%** |
| `internal/proc` | 84.9% | **96.2%** |
| `internal/prereq` | 0.0% | **95.3%** |
| `internal/web` | 0.0% | **94.6%** |
| `internal/awake` | 92.9% | 92.9% (the one gap is the `caffeinate` start-failure branch) |
| `internal/keys` | 0.0% | **87.5%** (the gap is the `crypto/rand` panic path) |
| `internal/version` | no tests | covered; the package is one constant |

Whole-tree coverage moved from 52.6% to 57.5%; what still holds it down is outside T11
(`internal/app` 19.1%, `internal/scan` 17.7%, `internal/engine` 33.4%, `internal/serve` 38.2%).

## Bugs found (15 confirmed, 5 major — full table in `agents/T11/a16/findings.md`)

| # | Bug | Owner |
|---|---|---|
| 1 | `catalog.parse` accepts a quant with no files; `app.Installed` then panics on `q.Files[0]` (shipped catalog is clean). | T01 |
| 2 | Every `web` POST handler ignores a malformed JSON body and applies zero values: `POST /api/lan {"on":` returns 200 and turns LAN off. | T07 |
| 3 | `web` drops type-mismatched fields and applies zero values: `{"on":"true"}` turns LAN off. | T07 |
| 4 | `serve.Config.Save` persists an empty API key (fixed on `agent/T07-api-security`). | T07 |
| 5 | A crashed `.tmp` can leave `config.json` world-readable (fixed on `agent/T07-api-security`). | T07 |
| 6 | `catalog.parse` accepts duplicate model ids; `Find`/`Use` silently pick the first. | T01 |
| 7 | `/api/update` treats `force=0` as true. | T07 |
| 8 | GET on a POST-only route returns the static 404; POST to an unknown path returns 405 instead of 404. | T07 |
| 9 | `update` treats build metadata as a prerelease (`1.0.0+build` sorts below `1.0.0`). | T08 |
| 10 | `update` compares prerelease strings, so `beta.2 > beta.11`. | T08 |
| 11 | `bench.clip` cuts at a byte offset and can split a multibyte rune (invalid UTF-8). | unowned |
| 12 | `bench.jsonFormat` strips only a lowercase ` ```json ` fence, so an uppercase fence fails the answer. | unowned |

Two flakiness reviews found and fixed test-only races: a global `stallTimeout` read by a watchdog
goroutine in `internal/engine` (2/20 `-race` failures on the baseline), and unsynchronised
handler/test sharing in `internal/serve/props_test.go`. After the fixes,
`-race -count=20 -shuffle=on ./internal/engine` is clean (148.9 s), and `internal/serve` is clean at
20× and under 4 concurrent binaries.

## Merge notes

Test files were mostly disjoint, so this was the cleanest merge so far: no source conflicts, only one
duplicate helper (`fakeBackend`) between two `internal/web` test files, renamed. Two agents changed
existing test files (`download_stall_test.go`, `props_test.go`) to remove races; both are inside
their slices.

## Files

- New `*_test.go` files in `internal/{catalog,keys,paths,prereq,report,version,web,bench,awake,proc}`.
- `agents/T11/a11/coverage.md` — the per-package before/after table.
- `agents/T11/a16/findings.md` — 15 confirmed bugs with file:line, failing test and owner.

## Next steps

1. Fix the 5 major bugs (catalog quant-without-files, the three `web` JSON handling issues, the
   config key issues).
2. Give `internal/bench` an owner; its two bugs are real and currently unassigned.
3. Re-run the coverage table after the other task branches merge, since several T11 skip-tests become
   passing tests once T07/T08 land.
