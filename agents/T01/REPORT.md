# T01. Catalog truth audit — consolidated report

16 agents (a01–a16) worked this task in parallel, two catalog models each, so all 32 entries in
`internal/catalog/catalog.json` (version 8) were checked against their real GGUF headers.
Consolidated onto `agent/T01-catalog-audit` by the Lead. Every correction below was proved from
GGUF metadata, the Hugging Face tree API or the model card; nothing was changed on a guess.

## Result

- **32/32 models verified**, 0 blockers, 0 dead URLs, 0 missing files.
- **17 models corrected**, 63 field or quant-level changes.
- **2 new pieces of machinery:** a Go consistency test for the embedded catalog
  (`internal/catalog/consistency_test.go`, 6 sub-tests) and `--json` output plus real exit codes
  for `scripts/verify_catalog.py`.
- **Everything verified twice:** against the pinned **b11960** and against the current release.
  Note that llama.cpp moved to semantic tags: `releases/latest` is **v0.9.0** (2026-11-04) while
  the newest build by date is **b12144**; agents checked both where it mattered.
- Gate on the consolidated branch: `gofmt -l .` empty, `go build ./...`, `go vet ./...`,
  `go test -p 2 ./...` all pass, plus `GOOS=windows` amd64/arm64 and `GOOS=linux` builds.

## Corrections

| Model | Field(s) | Was → now | Why |
|---|---|---|---|
| `qwen3.8-flash-next` | `quants[].bytes` (2) | 72,546,461,344 → 66,379,180,698 (UD-IQ1_S); 74,538,755,776 → 68,007,094,938 (UD-IQ1_M) | unsloth re-sharded on 2026-11-04 ("Better UD 3.1 quants"); HEAD of the 3 parts sums to the new numbers. **Blocker severity: the planner over-reserved 6.2–6.5 GB.** |
| `qwen3.8-flash-next` | `quants[].lazy_bytes` (5) | 22.58–25.79 GB → 28,800,138,432 | The n-gram table is one tensor alone in its own split part (28.8 GB). Old values were `floor(bytes*0.34)`, a guess. |
| `qwen3.8-flash-next` | `license`, `notes` | dropped "(UNVERIFIED terms)", closed a dangling "(PR #27742" | Read the LICENSE and the PR. |
| `gemma-4-e4b` / `gemma-4-e2b` | `kv_layers`, `swa_layers` | 7/35 → 4/20; 7/28 → 3/12 | `gemma4.attention.shared_kv_layers` = 18/20: llama.cpp allocates KV only for the first `block_count - shared` layers. Removes a **1.57 GiB / 1.04 GiB** f16 KV overcount at 128k context. |
| `gemma-4-e4b` / `gemma-4-e2b` | `active_params_b` | 4.0 → 4.5; 2.0 → 2.3 | Model card states 4.5B / 2.3B effective. |
| `nemotron-3.5-lightning-30b-a3b` | `ctx_max` | 262,144 → 1,048,576 | `nemotron_h_moe.context_length` = 1048576 in all 7 quant files and the card says "up to 1M". It had the Bolt value. No plan effect today: the fit ladder tops out at 65,536. |
| `qwen3.8-27b` | `quants[].mtp` (9) | absent → `true` | `nextn_predict_layers=1`, `block_count` 65, 866 tensors including `blk.64.nextn.*`. UD-IQ2_XXS and UD-IQ2_S correctly have none. |
| `nemotron-3.5-super-vl-120b-a12b` | `quants[].mtp` (5) | absent → `true` | The unsloth UD-* quants embed the MTP head (89 blocks, 781 tensors vs 763 in ggml-org). The ggml-org Q4_0/NVFP4 keep the separate 2.27 GB draft. |
| `ternary-bonsai-2-4b` | `params_b`, `active_params_b` | 4.66 → 4.54 | 4.66 was the base Qwen3.5-4B total; the shipped tensors are 4.21B LM + 0.33B vision. |
| `bonsai-27b-1bit` | `params_b`, `active_params_b` | 27.78 → 27.36 | 27.78 was the base Qwen3.6-27B count; the tensor table is 26.896B + 0.466B mmproj. |
| `muse-glimmer-30b` | `license`, `params_b`, `active_params_b`, `notes` | "Muse Model License (UNVERIFIED terms)" → Apache-2.0; 30.0 → 29.78 | Card, HF tag and LICENSE; HF safetensors total 29,776,626,688. |
| `lfm3-1.7b` | `license` | "…UNVERIFIED amount" → "free below $10M annual revenue" | LFM3 LICENSE §1 Threshold. |
| `ternary-bonsai-2-27b`, `bonsai-8b-1bit` | `notes` | truncated at 400 chars → full card text | The notes were cut mid-sentence. |
| `qwen3.5-4b`, `qwen3.5-2b` | `notes` | mmproj "~0.67–0.92 GB" → exact 672,423,616 B / 668,227,264 B | 0.92 GB was the 9B projector. |
| `gpt-oss-20b` | `notes` | byte-identical copy of the 120B note (with 120B-only DGX Spark speeds) → 20B facts | Copy-paste. |
| `mistral-small-4-119b` | `notes` | "6.5B UNVERIFIED" → card-confirmed | Card. |
| `nanbeige4.3-33b` | `notes` | "KV ~6x heavier" → 3.3x; "Tool calling UNVERIFIED" → verified | 106,496 vs 32,768 elements/token; template and card §3. |

No numeric field changed without a GGUF key or a card line behind it. Values that disagreed with a
model card but matched the GGUF and the base config were **left alone** and recorded as open
questions (Mistral `ctx_max` 1,048,576 vs card 256k; Devstral 393,216 vs card 256k;
qwen3.6 `active_params_b` 3.0 vs GGUF arithmetic).

## Cross-cutting findings (for the morning merge)

1. **`vision: true` is dead data.** 23 of 32 entries advertise vision, but no code downloads an
   mmproj or passes `--mmproj` (`internal/integrate/types.go:21` says a projector is never loaded,
   and nothing reads `catalog.Model.Vision`). EZLM advertises image input to OpenCode, Hermes and
   DSH that it cannot serve. Needs either a schema field + download path, or `vision: false`.
2. **The verifier hides two error directions** (fixed in part by a16, reported by a04/a13):
   it only warns when the catalog *undercounts* file size, so overcounts pass silently; and
   `scripts/verify_catalog.py:162,194` subtracts `nextn` from the attention-layer count, while
   llama.cpp allocates KV for every layer with `has_kv`, so every MTP model **undercounts** KV by
   one attention layer. a16 added `--json` and exit codes 0/1/2 but did not change the maths.
3. **A future consistency test must not assert `kv_layers + swa_layers == layers`** for shared-KV
   models (gemma-4-e4b/e2b): the sum is the owning layers (24/15), not the block count.
4. **Download keying:** plain and "(MTP)" variants of some unsloth quants share a basename, and
   `internal/engine/engine.go:473` keys downloads by basename. Sizes differ so it re-downloads
   rather than serving the wrong file, but the pair cannot coexist on disk.
5. **The published catalog is stale.** `https://picom-code.github.io/ezlm/internal/catalog/catalog.json`
   is byte-identical to the old embedded copy (version 8, sha256 `4bef13e1…`). `catalog.go:24`
   only accepts the remote copy when its version is greater, so **these fixes do not reach users
   until the catalog is republished with a version bump.**
6. `params_b` includes the vision tower for several models while `active_params_b` is text-only,
   so `fit.go`'s `activeFrac` mixes bases. Left unchanged; worth a schema decision.
7. **`fit.go` `normQuant` does not strip the `QAT-` prefix** (a05, proved with a probe under
   `agents/T01/a05/probe/`): `QAT-Q4_0` and `QAT-UD-Q4_K_XL` score the 0.9 unknown default
   instead of 0.985, so under `minQuality` 0.94 those quants are dropped entirely. Affects five
   gemma entries, not just the ones in this task. One-line fix: add `QAT-` to the prefix list.
8. **A model can need a per-quant draft:** `gemma-4-26b-a4b` has one non-QAT MTP draft but two
   QAT quants, and Google's QAT card says the assistant must be QAT too. The catalog has one
   draft per model, so this needs a per-quant override (a05).

## Files

- `agents/T01/a01..a16/REPORT.md` — one report per agent, with evidence.
- `agents/T01/a*/verify-b11960.txt`, `verify-latest.txt` (and `verify-v0.9.0.txt` / `verify-b12144.txt` where an agent checked both).
- `agents/T01/a*/gguf-metadata*`, `param-count*`, `memory-impact*`, `chat-template-tools*` — raw evidence.
- `agents/T01/a16/remote-vs-embedded.md` — byte-level remote catalog comparison.
- `agents/T01/a16/verify-ci-notes.md` — the new `--json` / exit-code behaviour.
- `internal/catalog/consistency_test.go` — the new Go consistency test.
- `scripts/verify_catalog.py` — `--json` and exit codes, default output unchanged.

## Next steps

1. Republish the catalog with a version bump, otherwise none of this reaches clients.
2. Decide on `vision` (schema field + mmproj download, or drop the flag).
3. Fix the verifier's layer maths for MTP models and make it warn on overcounts too.
4. Decide `ctx_max` precedence: GGUF `context_length` or the model card (Mistral, Devstral).
5. The `mtp` additions assume `internal/app/app.go:750` still downloads the draft; confirm that
   path when the MTP-marked quants are selected.
