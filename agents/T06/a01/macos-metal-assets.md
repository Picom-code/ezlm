# macOS Metal release assets, b11960 to the latest release

Collected 2026-11-06 with GET requests only. No archive was downloaded.

## Which tag is "the latest release"

| Step | Request | Result |
|---|---|---|
| 1 | `GET /repos/ggml-org/llama.cpp/releases/latest` | `v0.9.0`, published 2026-11-04T13:08:06Z, **1 asset** |
| 2 | that asset: `nightly-tag.txt` (7 bytes) | `b12110` |
| 3 | `GET /repos/ggml-org/llama.cpp/releases/tags/b12110` | 38 assets |

`LatestRelease()` (internal/engine/engine.go:69) follows exactly that chain, so the
build EZLM installs with `EZLM_LLAMA=latest` is **b12110**. The newest build by date
is **b12167** (2026-11-06T09:45:44Z); its macOS names are included below for
comparison. `v0.9.0` itself carries no binaries.

## macOS assets

| Tag | Asset | Size (bytes) | SHA-256 (from the API `digest`) |
|---|---|---|---|
| b11960 | `llama-b11960-bin-macos-arm64.tar.gz` | 16465074 | `58e5a5e1d7c2bf66cedd9c29e0323f3e570660a70a1e2156d32eed08f4cc1e3c` |
| b11960 | `llama-b11960-bin-macos-x64.tar.gz` | 12205764 | `d1e6c9ddcaed9a3b1bb8f3d946bdb3481c7b919357a4029dcb747762397f1da5` |
| b12110 | `llama-b12110-bin-macos-arm64.tar.gz` | 16650691 | `4f5b8de12e0b9516658ca6a5764c6ffb3d7c659c3dd538556b1413c7b4050662` |
| b12110 | `llama-b12110-bin-macos-x64.tar.gz` | 12388601 | `c8094268b6bf62f4998f0e45656fd9f5e2d4afb6b1cd0044d761db64be1dd47d` |
| b12167 | `llama-b12167-bin-macos-arm64.tar.gz` | not read (rate limit) | not read |
| b12167 | `llama-b12167-bin-macos-x64.tar.gz` | not read (rate limit) | not read |

Names are identical apart from the tag. Both architectures still exist at both
tags, and both are still `.tar.gz`. No macOS asset was renamed or removed.

The b11960 rows match `internal/engine/pinned.go`; b12110 was read from
`https://api.github.com/repos/ggml-org/llama.cpp/releases/tags/b12110`.

## Assets new since b11960 that could collide

| b12110 asset | Starts with `llama-<tag>-bin-` | Can `PickAssets` choose it? |
|---|---|---|
| `llama-b12110-ui.tar.gz` | no | no, `hasBinaries`/patterns need `-bin-` |
| `llama-b12110-xcframework.zip` | no | no, the `darwin` pattern is `-bin-macos-<arch>` |
| `llama-b12110-bin-ubuntu-26.04-s390x.tar.gz` | yes | no, needs `-bin-ubuntu-<x64\|arm64>` |
| `llama-b12110-bin-ubuntu-zdnn-s390x.tar.gz` | yes | no, same reason |

The last two are new CPU-ish s390x flavors (not macOS); they belong to a07/a08.

## How EZLM resolves macOS today

`assetPatterns` (internal/engine/engine.go:110) returns `{{"-bin-macos-" + a}}` for
`darwin` with `a` = `arm64` or `x64`, ignoring the backend. `PickAssets` then
requires the `llama-` prefix, the pattern, and a `.zip`/`.tar.gz` suffix, and
returns exactly one asset (there is no CUDA runtime sidecar for macOS).

`flavor()` turns the name into the install folder `llama-<tag>-macos-<arch>`, so
arm64 and x64 never share binaries.

## Rate limit note

`api.github.com` allows 60 requests an hour per address and this address was
exhausted at 12:47 UTC 2026-11-06 (reset ~13:02 UTC). b12167 asset names were
read from the HTML of
`https://github.com/ggml-org/llama.cpp/releases/expanded_assets/b12167`
instead; sizes and digests for b12167 are therefore blank.
