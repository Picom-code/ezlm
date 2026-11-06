# T11 a16: findings table (report support)

## What I did

Read `README.md`, the T11 brief and the T11 section of `TASKS.md`, then read the source of every
package in T11's scope (`internal/catalog`, `keys`, `paths`, `prereq`, `report`, `version` and the
`internal/web` handlers) and the test files the other coverage agents are landing.

Compiled `agents/T11/a16/findings.md`: every bug the coverage agents found, with file:line, the
failing or skipped test name, severity and the owning task. It also carries a separate table of
agent observations that have no deterministic failing test, so a reader can tell a proven bug from
a hunch.

## Sources

- a01 catalog (skipped tests on `agent/T11-a01`)
- a03 paths, a05 report, a06 version/update, a07 and a08 web, a09 web settings, a11 coverage,
  a12 bench, a13 engine flake: their `agents/T11/aXX/REPORT.md` and/or their skipped tests
- a16 direct review: prereq `nvidiaDriver` zero status, catalog quant ordering
- a13's fix of the `stallTimeout` test race is recorded as fixed, not skipped

All fifteen coverage agents (a01-a15) had reported or committed their skipped tests by 10:25, so
the table covers every agent. No logic was changed and no other agent's worktree was touched.

## Findings summary

- 15 confirmed bugs (5 major, 10 minor). The majors: `internal/catalog.parse` accepts a quant
  with no files and the app then panics on `q.Files[0]` (T01); every `internal/web` POST handler
  silently swallows malformed JSON and succeeds with zero values, and drops type-mismatched fields
  (T07); `serve.Config.Save` persists an empty API key and can leave `config.json` world-readable
  after a crashed `.tmp` (T07, a02 reports both fixed on `agent/T07-api-security`).
- 21 observations without a failing test, all minor or info.
- Fixed in T11 branches: the `internal/engine` `stallTimeout` race (a13) and the
  `internal/serve/props_test.go` race (a14).
- No bugs in `internal/report` (a05), the web status/live handlers (a07), or `internal/awake` /
  `internal/proc` (a15).

## What I could not finish

- The table is a snapshot at 10:25. If an agent amends its branch after that, the morning merge
  should re-run the same collection (grep `t.Skip("bug:` on each `agent/T11-aXX` branch, read each
  `agents/T11/aXX/REPORT.md`) and append anything new.
- Rows 6-8 are fixed on `agent/T07-api-security` but still skipped on the T11 branches, so the
  merged tree will carry skips until T07's branch lands.

## Next steps

1. After the merge, diff each agent's `REPORT.md` "Bugs found" section against `findings.md` and
   add anything missing.
2. T01 and T07 own the two majors; T08 owns the two `internal/update` ordering bugs.
3. `internal/bench`, `internal/paths` and `internal/prereq` have no owning task in `TASKS.md`;
   the observations about them need an owner before they can be fixed.
