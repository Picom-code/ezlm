# T05/a13: corpus plumbing

Slice: the fixture format and provenance rules, plus a small loader the other
T05 tests can use. No parser changes.

## What I did

- Added `internal/scan/testdata/README.md`: directory layout, group names,
  allowed extensions, exact-bytes rule, size limit, anonymisation rule, the
  provenance format (`SOURCES.md` per group), and how to load a fixture.
- Added `internal/scan/testdata_helpers_test.go` (`package scan`, so every
  test file in the package can call it):
  - `loadFixture(t, "nvidia/a100-mig-on.csv")` returns the file as a string.
  - `loadFixtureBytes` returns the raw bytes, for BOM/CRLF cases.
  - `listFixtures(t, "nvidia")` returns a group's fixture names, sorted, for
    table tests.
  - A missing fixture fails the test and lists the names that do exist.
  - `TestFixtureCorpus` checks the rules above: group directories only, known
    extension, lowercase name, non-empty, at most 64 KB, and every fixture
    named in its group's `SOURCES.md`.
- Added `internal/scan/testdata/example/` as the worked example: a two-GPU
  `nvidia-smi` CSV and its `SOURCES.md`, which `TestFixtureHelper` loads. It
  is not a parser group.

## Findings

1. **No fixture corpus existed in `internal/scan`.** Every test built its
   input as an inline Go string, for example `internal/scan/cuda_test.go:7-8`,
   `internal/scan/gpu_usage_test.go:7-9` and `internal/scan/display_test.go:12-14`.
   Severity: minor. Evidence: `ls internal/scan/testdata` failed before this
   branch; the only `testdata` directory in the repo was
   `internal/integrate/testdata`. Effect: a corpus agent had nowhere to put a
   captured output, and there was no shared rule for what a fixture may
   contain or where its source goes.
2. **The repo already had a testdata convention, in `internal/integrate`.**
   `internal/integrate/launch_golden_test.go:13-17` reads
   `filepath.Join("testdata", ...)` relative to the package directory. I
   matched it instead of inventing an absolute-path lookup. Severity: none,
   consistency note.
3. **Provenance has two homes by design.** The T05 brief asks corpus agents
   for `agents/T05/<agent>/SOURCES.md`; the fixture format needs a source
   next to the fixture too, or the corpus is unreadable to whoever picks it up
   next. The README requires a per-group `SOURCES.md` that names each file;
   the agent's own `SOURCES.md` can summarise. Severity: minor, process.

## What changed and why

Test-only files plus docs, so no product behaviour changes:

- `internal/scan/testdata/README.md` (new): the contract.
- `internal/scan/testdata_helpers_test.go` (new): loader and corpus check.
  Standard library only (`os`, `path/filepath`, `sort`, `strings`, `testing`).
- `internal/scan/testdata/example/` (new): the worked example.

The loader returns bytes unchanged on purpose: `[N/A]`, decimal commas, BOMs,
CRLF and mojibake are the cases the parsers must survive, so the corpus must
not normalise them.

## Verification

```
$ gofmt -l internal/scan/            # empty
$ go vet ./internal/scan/             # clean
$ go test -p 2 ./internal/scan/ -run 'TestFixture' -v
--- PASS: TestFixtureCorpus (0.00s)
--- PASS: TestFixtureHelper (0.00s)
$ go test -p 2 ./internal/scan/       # ok
$ go build ./...                      # ok
```

## Merge dependency

`TestFixtureCorpus` fails the merged tree if a corpus agent adds fixtures
without a `testdata/<group>/SOURCES.md` that names them. I told the Lead the
convention so it can be relayed to a01-a10 and a16 before the merge. If that
is too strict, the fallback is to log provenance gaps with `t.Logf` instead
of `t.Errorf`; the check is one `switch` in `checkFixtureGroup`.

## Open questions

- Should the corpus test run in CI as a hard gate, or is a logged warning
  enough while the corpus is being built in parallel? See above.
- The loader uses relative `testdata` (house style). A test that calls
  `t.Chdir` before loading would break it; none do today.
- Should `listFixtures` include `.md` notes? Currently no, so a table test
  never tries to parse `SOURCES.md`.
