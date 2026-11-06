# T06. llama.cpp upgrade study — consolidated report

16 agents (a01–a16) worked this task in parallel: eight release-asset groups, four server-surface
groups, the asset test matrix, the upgrade plan, the asset checker script, and the risk matrix.
Consolidated onto `agent/T06-llamacpp-upgrade` by the Lead. The pinned tag is unchanged (b11960).

## Result

- **Upgrade target resolved:** `releases/latest` is **v0.9.0** (2026-11-04) and its only asset is
  `nightly-tag.txt`, which contains **b12110**. So "the latest release" is b12110, not b12144 or
  b12167 (those are newer nightlies). Every agent verified assets at b11960, b12110 and (where it
  mattered) b12167.
- **No renamed, removed or reformatted assets** between b11960 and b12110: 34 → 38 assets, all four
  additions harmless (`llama-<tag>-bin-ubuntu-26.04-s390x.tar.gz`, `-ubuntu-zdnn-s390x.tar.gz`,
  `ui.tar.gz`, `xcframework.zip`). All 38 carry sha256 digests.
- **No breaking server change** in the flags, `/props`, `/slots`, `/v1/messages`, OpenAI endpoints,
  metrics or `/health` that EZLM uses.
- **Gate:** `gofmt -l .` empty, `go build ./...`, `go vet ./...`, `go test -p 2 ./...` pass, plus
  `GOOS=windows` amd64/arm64 and `GOOS=linux` builds.

## Bugs found and fixed in asset selection (`internal/engine/engine.go`)

| # | Bug | Effect |
|---|---|---|
| 1 | The CUDA-13 branch used an OS→prefix map with no `darwin` key (a01) | A darwin + CUDA + cuda13 query produced an empty prefix and selected **`llama-…-bin-android-arm64-snapdragon.tar.gz`**. |
| 2 | The Windows CUDA chain fell back to the generic `-bin-win-cuda-` (a02) | A CUDA-12 driver could get a CUDA-13 build that needs a 13.0 driver. Now falls to Vulkan (arm64, which has no CUDA 12 build, falls to 13.4). |
| 3 | `s390x` was not in the arch map (a07) | Linux/s390x fell through to `-bin-ubuntu-` and picked the **arm64** archive. |
| 4 | An unmapped architecture produced an empty arch substring (a08) | `ppc64le`/`386` matched any archive for the OS; now returns a clear error. |
| 5 | `hasBinaries` counted `cudart-llama-*-bin-*` as a server archive (a08) | A release with only runtime archives would look installable. |

## What the upgrade plan says (a14, a15, a16)

1. Pin **b12110** so the pin matches what `EZLM_LLAMA=latest` resolves to; regenerate the `pinned.go`
   literal (34 → 38 assets, new sizes and digests). There is **no generator** for that literal, which
   is the main mechanical risk (R1).
2. Verify size and digest for the new assets; a wrong size fails every install.
3. Run the asset matrix at both tags (`internal/engine/assets_matrix_test.go` covers 21 rows), the
   Windows/Linux cross builds, and a smoke install.
4. Rollback is a revert of `pinned.go`: old per-tag install directories are untouched, so it is
   instant.
5. `agents/T06/check_assets.py` prints and verifies the asset EZLM would pick for 18 platform rows
   at any tag (proved on b11960, b12110 and b12144).

## Found, not fixed (pre-existing, not caused by the upgrade)

1. **ROCm archives are never selectable:** `assetPatterns` maps `scan.ROCm` to Vulkan on both
   operating systems (`engine.go:126-127,136-137`), so the ROCm entries in `pinned.go` are dead and
   `EZLM_BACKEND=rocm` installs Vulkan. Deliberate per `scan.go:138`, but it should be a decision,
   not an accident. (a06, a13)
2. **`--cache-ram` is never passed**, so every deploy inherits the 8 GiB host prompt-cache cap,
   contradicting the "every memory-relevant flag is explicit" comment in `serve/config.go:113-114`.
   (a09)
3. **`propThinking` looks for `supports_thinking`/`supports_reasoning`**, which `caps::to_map()`
   never emits (the real key is `supports_reasoning_effort`), so the fallback is dead. (a10, a11)
4. **EZLM reads `/slots` without passing `--slots`**; it works only because the default is true.
   (a11, a12)
5. **A CUDA build with a missing cudart installs silently**; the CUDA 12 choice is listing order,
   not version order. (a15)
6. **`internal/app/demo.go:208` hardcodes "(b11960)"** — the app owners need to update it. (a14)

## Files

- `internal/engine/assets_*_test.go` (eight groups), `assets_matrix_test.go`,
  `flags_core_test.go`, `flags_reasoning_test.go`, `metrics_names_test.go`,
  `internal/serve/props_v090_test.go`.
- `agents/T06/a01..a16/REPORT.md`, plus `upgrade-plan.md`, `risks.md`, `flags.md`,
  `reasoning.md`, `http.md`, `metrics.md`, `check_assets.py`, and the release evidence JSON.
- `agents/T06/a13/release-b12110.json` — name, size and digest for all 38 assets.

## Next steps

1. Decide the target: b12110 (latest release) or the newest nightly; the plan assumes b12110.
2. Regenerate `pinned.go` for that tag (a generator is worth writing).
3. Decide the ROCm routing and `--cache-ram` policy.
4. Run a real install and a smoke test before shipping the pin change.
