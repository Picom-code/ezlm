# T08 a12: fuzz version tags, comparison and channel selection

Agent a12 (cross-cutting, stretch). Branch `agent/T08-a12`, worktree
`~/ezlm-agents/work/subs/T08/a12`.

## Slice

`internal/update/fuzz_version_test.go` with `testing.F` over version tag
parsing, comparison and channel selection. It asserts no panic, total
ordering, and a clear reject for malformed input. Both fuzz targets ran for
30 seconds.

## What the fuzzer found (all reproduced before the fix)

The baseline `internal/update/update.go` read tags with `split` at line 147,
which threw away `strconv.Atoi` errors. Anything that was not a number became
zero, and anything out of range became `math.MaxInt64`. Evidence from a probe
test against the baseline (recorded in the run log below):

| input | baseline result | consequence |
|---|---|---|
| `9.9.9.9` | `[9 9 0]`, `Newer` true | `Check` returned `Available=true` with `Latest.Version="9.9.9.9"`; the app offers and installs a release with a malformed tag. **Major.** |
| `1.2.3.4` | `[1 2 0]` | same class: a four-part tag is read as a different, lower version. |
| `v1` | `[1 0 0]` | a one-part tag is accepted as a full version. |
| 400 nines | `[9223372036854775807 0 0]` | an overflowing number clamps to MaxInt64 and beats every real version. |
| `1.2.3+build` | `pre="build"` | build metadata is read as a prerelease, so `1.2.3+build` sorts *below* `1.2.3` (SemVer ignores build metadata). **Minor.** |
| `1.2.3-beta.10` vs `1.2.3-beta.2` | lexical compare | `beta.10` sorts below `beta.2`; `Check` keeps `beta.2` as the newest, so a newer prerelease can be missed or an older one offered. **Minor.** |
| `β1.2.3` | `[0 2 3]` | unicode tag silently read as `0.2.3`. |

No input panicked: `split` always returned a three-element array, so the
existing `Newer` was a total preorder even on garbage. The defect was that
malformed tags were *silently coerced* rather than rejected, which let `Check`
select and report them as installable releases.

## What changed

`internal/update/update.go` (minimal, contained in the file this package
owns):

- `ParseVersion(tag) (Version, error)` (line 139) reads exactly three
  numeric parts, an optional leading `v`, an optional SemVer prerelease and
  optional build metadata. Signs, leading zeros, empty identifiers, unicode,
  more than three parts and numbers that overflow `int` are errors.
- `Version.Compare` (line 235) implements SemVer ordering: numbers first,
  a release beats its prerelease, prerelease identifiers compare numerically
  and numeric identifiers sort below alphanumeric ones, and build metadata is
  ignored.
- `Newer` (line 289) is now defined through `ParseVersion`; a tag that does
  not parse is never newer than anything.
- `Check` (line 93) skips a release whose tag does not parse, before the
  draft check, so a malformed tag is rejected at the channel-selection step
  and can never be reported as available or downloaded.
- The old `split` helper is gone; `ParseVersion` replaces it.

No other file changed. `internal/update/update_test.go` still passes
unchanged.

## The fuzz test

`internal/update/fuzz_version_test.go`:

- `TestParseVersion` is a deterministic table for the tags the task lists
  (`v`, `v1`, `1.2.3.4`, `v1.2.3-beta+build`, unicode, 400-digit, empty),
  so the reject is documented even when `-fuzz` is not running.
- `FuzzVersion(a, b, c string)` asserts: nothing panics, `Newer(x, x)` is
  false, `Newer(a, b)` and `Newer(b, a)` are never both true, `Newer` agrees
  with `Compare` when both tags parse, ordering is transitive, and a tag that
  fails `ParseVersion` never compares as newer than anything.
- `FuzzCheckChannel(tagA, tagB string, draftA, draftB bool)` drives a fake
  GitHub (`httptest`, no real network, no real GitHub) with two releases and
  asserts: no error, drafts are never selected, malformed tags are never
  selected, the picked release is the newest well-formed non-draft one, and
  `Available` is exactly `picked > running version`. This is the channel
  selection half of the slice.

Both targets seed from the same `fuzzTags` corpus so the first seed run
already covers the a07 tag list.

## Fuzz run (30s each)

```
$ go test -run '^$' -fuzz '^FuzzVersion$' -fuzztime=30s ./internal/update/
fuzz: elapsed: 30s, execs: 2848786 (115080/sec), new interesting: 283 (total: 859)
PASS
ok  github.com/picom-code/ezlm/internal/update  30.886s

$ go test -run '^$' -fuzz '^FuzzCheckChannel$' -fuzztime=30s ./internal/update/
fuzz: elapsed: 31s, execs: 286766, new interesting: 123 (total: 1851)
PASS
ok  github.com/picom-code/ezlm/internal/update  31.283s
```

No input failed either target: 2.85M comparisons and 287k channel selections
with no panic, no ordering violation and no malformed tag selected. The
generated corpus lives in Go's fuzz cache, not in the tree, so no
`testdata/fuzz` files were added.

## Residual risk

- `Newer` returns false when either side is unparsable, so an unparsable
  *running* version would report "up to date" forever. The running version is
  a compile-time constant and is covered by `TestParseVersion`'s parse of
  `version.Version` inside the fuzz body; a build-time check would be
  stronger.
- `Check` still trusts the release list order for equal versions (first
  wins). That is deterministic, and the fuzz target asserts the maximum, not
  the identity, of the pick.
- This patch overlaps a05 (downgrades) and a07 (malformed tags) in
  `update.go`; if their branches also change `Check`/`Newer`, the morning
  merge needs a look. `Newer`'s signature is unchanged, and
  `update_test.go` passes as-is.

## Open questions

- Should a leading `V` be accepted? Real tags use lowercase `v`; `V1.2.3`
  is rejected today.
- SemVer forbids leading zeros in numeric parts and prerelease identifiers;
  this patch follows SemVer and rejects `01.2.3`. No real tag uses one.

## Before/after evidence

Before (baseline probe, `internal/update/update.go` line 147 `split`):

```
"1.2.3.4"      -> [1 2 0] pre="" newer_than_current=true
"v1"           -> [1 0 0] pre="" newer_than_current=true
"1.2.3+build"  -> [1 2 3] pre="build" newer_than_current=true
Newer(1.2.3.4, 1.2.2)=false Newer(1.2.3.4, 1.2.3)=false
Newer(1.2.3+build, 1.2.3)=false Newer(1.2.3, 1.2.3+build)=true
Newer(1.2.3-beta.10, 1.2.3-beta.2)=false
Check -> available=true latest=&{Version:9.9.9.9 ...}
```

After: `TestParseVersion` rejects `1.2.3.4`, `v1`, `1.2.3-`, unicode and the
overflowing tag; `Newer("1.2.3+build", "1.2.3")` is false both ways;
`Newer("1.2.3-beta.10", "1.2.3-beta.2")` is true; `Check` with a `9.9.9.9`
release picks nothing.
