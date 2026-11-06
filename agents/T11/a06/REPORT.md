# T11 / a06: `internal/version` tests

## What the package is

`internal/version/version.go` holds one constant, `Version = "0.1.11-beta"`, and a
package comment saying it is compared against GitHub release tags
(`v0.1.1-beta` → `0.1.1-beta`). There is no parsing, comparison or build-info code
in the package; the comparison the brief asks about lives in `internal/update`
(`Newer` at `internal/update/update.go:128`, `split` at
`internal/update/update.go:147`).

So the test file has two halves:

1. invariants of the constant itself, which is what the rest of the engine and
   app display and compare (`TestVersion`, `TestVersionOrdering`);
2. the version-comparison contract the constant depends on, exercised through
   the exported `update.Newer` (`TestNewerPreReleaseOrdering`,
   `TestNewerBuildMetadata`, `TestNewerMalformedInput`). These stay in
   `internal/version` because that is the only package this slice owns.

No logic changes. The only new file is `internal/version/version_test.go`.

## Coverage

| Package | Before | After |
|---|---|---|
| `internal/version` | `[no test files]` | `coverage: [no statements]` |
| `internal/update` (cross-check, `-coverpkg=./internal/update`) | n/a | 29.0% of statements exercised by the new tests |
| `internal/update` (its own suite) | 32.3% | 32.3% (unchanged; not this slice) |

`internal/version` has no executable statements, so its statement percentage
cannot move. The meaningful before/after is that the package had no tests at all
and now has 5 test functions (19 cases/subtests, 4 of them skipped as bug
evidence).

## Tests added

- `TestVersion`: `Version` is non-empty, has no `v` prefix, no build metadata,
  matches a `major.minor.patch[-pre]` release-tag body, and carries a
  pre-release suffix (the beta channel).
- `TestVersionOrdering`: the current version is newer than a table of older tags
  (including a `v`-prefixed one and `0.1.11-alpha`), is not newer than itself or
  `v` + itself, and its final release `0.1.11` beats it.
- `TestNewerPreReleaseOrdering`: SemVer 2.0.0 precedence for release vs
  pre-release and lexical/numeric pre-release identifiers.
- `TestNewerBuildMetadata`: build metadata must not affect precedence.
- `TestNewerMalformedInput`: empty, non-numeric, short, long, whitespace,
  `v`-prefixed, trailing-dash and leading-dash inputs; no panics and documented
  lenient results.

## Bugs found (left as skipped tests)

Both are in `internal/update`, which is outside T11's owned packages, so they are
recorded here and skipped rather than fixed.

### 1. Build metadata is parsed as a pre-release (minor)

`internal/update/update.go:150` splits on `IndexAny(v, "-+")`, so the part
after `+` lands in `pre` and changes ordering. SemVer says build metadata is
ignored.

Evidence: `Newer("1.0.0", "1.0.0+build")` returns `true`; a final release must
not be newer than the same version with build metadata.
Skipped tests:
`TestNewerBuildMetadata/1.0.0_gt_1.0.0+build` and
`TestNewerBuildMetadata/1.0.0-rc.1+build.5_gt_1.0.0-rc.1`.

Impact: if a GitHub tag ever carries `+build`, the update check reports a
spurious update or misses one. Current tags do not, so this is minor.

### 2. Numeric pre-release identifiers compare as strings (minor)

`internal/update/update.go:144` compares the whole pre-release string, so
`beta.2` sorts above `beta.11`. SemVer compares dot-separated identifiers and
orders all-numeric ones numerically.

Evidence: `Newer("1.0.0-beta.2", "1.0.0-beta.11")` returns `true`; SemVer says
`beta.11` is newer.
Skipped tests: `TestNewerPreReleaseOrdering/1.0.0-beta.2_gt_1.0.0-beta.11` and
`TestNewerPreReleaseOrdering/1.0.0-beta.11_gt_1.0.0-beta.2`.

Impact: only matters for numbered pre-release tags; today the project ships
bare `-beta` tags, so this is minor.

## Documented malformed-input behaviour (not filed as bugs)

`split` is deliberately lenient: non-numeric components read as 0 (`"abc"` is
`0.0.0`), a fourth component is ignored (`"1.2.3.4"` is `1.2.3`), surrounding
spaces and one leading `v` are stripped, and `"1.2.3-"` reads as the final
release. No input panics. Whether `Newer` should reject malformed input instead
of coercing it is an open question, not a defect the tests pin down.

## Open questions

- Should `Version` be checked against the release tag at build time (for
  example `go:embed` or `-ldflags`)? Today the constant is the only source of
  truth and the tests can only check its shape.
- Should `Newer` move into `internal/version`? The package comment promises
  comparison but the code is in `internal/update`; a move would make this
  slice's tests land in the same package as the logic.

## Commands run

```sh
go test -p 2 -cover ./internal/version
go test -p 2 -coverpkg=./internal/update -cover ./internal/version
gofmt -l .
go build ./...
go vet ./...
```
