# T06 a16 report: risk matrix and report support

Branch: `agent/T06-a16`. Worktree: `~/ezlm-agents/work/subs/T06/a16`.
Deliverables: `agents/T06/a16/risks.md` (ranked findings and the test
matrix), `agents/T06/a16/picks.txt` (probe output), and this report.

## What I did

1. Read `internal/engine/engine.go` and `internal/engine/pinned.go` (selection
   and release following), `internal/serve/config.go`, `props.go`, `run.go`,
   `client.go` (what EZLM passes and reads), and the scan/prereq code that
   feeds backend and CUDA-version decisions.
2. Ran a throwaway probe test (deleted before commit) calling
   `engine.PickAssets` for 19 platform/backend cases at both `b11960` (the
   pinned release) and `b12110` (the build the current stable release pins).
   The 38 output lines are committed as `picks.txt`; `risks.md` section 4
   turns them into the platform x tag x asset x status matrix.
3. Compared the server surface at both tags with read-only GETs to
   raw.githubusercontent.com: `common/arg.cpp`, `common/chat.cpp`,
   `tools/server/server.cpp`, `tools/server/server-context.cpp`. No model was
   loaded and no archive was downloaded.
4. Cached the b12110 release JSON (72 KB, from a02's fetch) as
   `latest-build.json` so the matrix can be reproduced offline.

## Findings

Full detail, evidence and ranking in `agents/T06/a16/risks.md`. Summary:

| id | severity | finding |
|---|---|---|
| R1 | major | `pinned.go` is a hand-shaped b11960 snapshot (34 assets); the pin move needs a full regeneration of URL, size and digest for the 38 b12110 assets, or `verifyDigest` fails after a large download (`engine.go:320-338`). |
| R2 | major | `releases/latest` is `v0.9.0` with only `nightly-tag.txt` = `b12110`; the newest builds by date are `b12144`/`b12167`, which are not release-pinned. The upgrade target should be stated as `b12110` (`engine.go:69-96`). |
| R3 | minor | Unknown `GOARCH` degenerates to an empty arch pattern (`engine.go:111`), so `linux/s390x` picks `ubuntu-arm64` at b11960 and `ubuntu-26.04-s390x` at b12110 only by asset-name order. |
| R4 | minor, pre-existing | `scan.ROCm` maps to Vulkan (`engine.go:125-126,134-135`); both releases ship ROCm archives that EZLM never selects. |
| R5 | minor | Four assets appeared between b11960 and b11988 and persist at b12110: `ui.tar.gz`, `xcframework.zip`, `ubuntu-26.04-s390x`, `ubuntu-zdnn-s390x`. Harmless today (selection requires `-bin-`), a trap for a rewritten matcher. |
| R6 | minor | The `nightly-tag.txt` follow path does not check the HTTP status and has no timeout (`engine.go:81-88`). |
| R7 | minor | An upgrade leaves the previous engine folder on disk; CUDA flavors are 160-290 MB plus a cudart archive up to 594 MB (`engine.go:226-266`). |
| R8 | info | Every flag `Config.Args` passes still exists at b12110 with the same definition; `server.cpp` and `chat.cpp` are byte-identical at both tags, and the `/props` keys EZLM reads are still emitted. No flag or schema break found in this slice. |
| R9 | info | The CUDA 12.4 floor (`engine.go:215`) still matches the shipped 12.4/12.8 archives; CUDA 12.x minor-version compatibility covers the difference. |

No blocker found. The upgrade looks asset-safe and flag-safe; the risk is in
regenerating the pin and naming the right target tag.

## What I changed

Nothing in Go code. This slice is evidence and assembly support only, so no
test file or `engine.go`/`pinned.go` edit was needed and none was made. The
only files added are under `agents/T06/a16/`.

## What I could not finish

- Sibling findings were not committed when I wrote `risks.md`, so section 5 is
  a placeholder table (a01-a15) and section 3 has no entries from the
  platform, flag or schema agents. The Lead must merge them at assembly time.
- The flag/schema check (R8) is a source-level comparison at the two tags, not
  a running server: no model may be loaded on this machine, so runtime
  behaviour differences inside unchanged code are out of scope.
- I did not verify the b12110 digests by downloading archives (only HEAD/GET
  metadata); R1 assumes a regeneration from the API listing, as intended.
- The s390x row (R3) is out of scope for the beta build matrix; it is recorded
  because it is the only failing matrix row.

## Next steps

1. a14's upgrade plan should use `b12110` as the target, script the
   `pinned.go` regeneration, and include the old-folder cleanup decision.
2. a13's matrix test should cover the 19 rows in `risks.md` section 4, with
   the s390x guard from R3 if a08 adds one.
3. At report assembly, merge a01-a12 findings into section 3 and re-rank;
   a09-a12 should reconcile with R8, which says the flag and `/props` surface
   is unchanged across this range.
