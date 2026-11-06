# T01 / a01: catalog truth audit

Agent: `a01`. Models owned: `ternary-bonsai-2-27b` and `qwen3.8-27b`
(19 quants). Worktree `~/ezlm-agents/work/subs/T01/a01`, branch
`agent/T01-a01`, base `5213370` (main snapshot).

Correction commit: `08f6215` (`T01: mark Qwen3.8-27B unsloth quants that
embed MTP heads as mtp; finish truncated Bonsai 27B notes`).

## What I checked

1. `scripts/verify_catalog.py --tag b11960 --jobs 2 --only
   ternary-bonsai-2-27b,qwen3.8-27b` -> `verify-b11960.txt`.
   20 file sets, 0 errors: every file exists at the listed URL, the byte
   total is exact, every tensor type and the `qwen35` architecture are known
   to b11960, and the layer / KV-head / head-size / context numbers agree
   with each GGUF header.
2. Same with `--tag v0.9.0` (the tag `releases/latest` returns) ->
   `verify-latest.txt`. 20 file sets, 0 errors.
3. Read each GGUF header by range request and compared every field by hand:
   `block_count`, `attention.head_count_kv`, `attention.key_length`,
   `full_attention_interval`, `context_length`, `nextn_predict_layers`,
   and the tensor list (`gguf-metadata.txt`).
4. Read both model cards and the quant repos' cards over HTTPS: licence,
   parameter count, context, layer layout, vision tower, tool calling, and
   which files embed the MTP head. Did not download weights.
5. Checked the separate `draft` file in the qwen entry (existence, exact
   size, GGUF metadata, tensor names).
6. Diffed the embedded catalog against
   `https://picom-code.github.io/ezlm/internal/catalog/catalog.json`.

`gofmt -l .` empty, `go build ./...`, `go vet ./internal/catalog/`,
`go test -p 2 ./internal/catalog/` pass (the package has no test files).

## Findings

| # | Severity | Where | Finding | Evidence |
|---|---|---|---|---|
| 1 | major | `internal/catalog/catalog.json:303,312,321,330,339,348,357,366,375` | Nine unsloth Qwen3.8-27B quants embed the MTP head but were not marked `"mtp": true`, so the planner never selects `draft-mtp` for them. | GGUF `qwen35.nextn_predict_layers = 1`, `block_count = 65` and 866 tensors vs 851 for the 64-block files, with the same `blk.64.nextn.{eh_proj,enorm,hnorm,shared_head_norm}` tensors that the ISTA `-mtp` files carry. `internal/fit/fit.go:505` only enables `draft-mtp` when `q.MTP` is set. |
| 2 | minor | `internal/catalog/catalog.json:25` | The `ternary-bonsai-2-27b` note was cut off mid-sentence at 400 characters ("... RTX PRO 60"). | The string ends inside the speed list; the model card's throughput table has the missing values. |
| 3 | major | `internal/integrate/opencode_config.go:70`, `internal/integrate/dsh_config.go:102` | `vision: true` advertises image input to OpenCode/DSH, but nothing downloads an mmproj file or passes `--mmproj` to llama-server, so a deployed "vision" model cannot actually take an image. | `grep -rn mmproj --include=*.go internal` finds only catalog notes; there is no mmproj field in `catalog.Model` and no `-mmproj` in `internal/serve/config.go`. Both of my models do ship mmproj files in their repos (see below), so this is wiring, not catalog data. Catalog-wide: 23 entries set `vision: true`. Not fixed (frozen area + out of my two-model slice). |
| 4 | minor | `internal/fit/fit.go:407-408` | MTP files have 65 blocks (64 + the nextn block) but the model entry has `layers: 64`; Split mode computes `NGL` from `m.Layers`, so the MTP block can stay on the CPU and the speculative decode runs with a CPU-resident head. Not a correctness bug and pre-existing for the ISTA `-mtp` entries. | `qwen35.block_count = 65` on every file with `nextn_predict_layers = 1`; `fit.go:407` uses `m.Layers` as the layer budget. Model-level field, shared with the 64-block quants, so not changed. |
| 5 | info | `internal/catalog/catalog.json` vs remote | The remote catalog is byte-for-byte the baseline catalog (version 8, all 32 models equal). My corrections only reach clients after the remote copy is republished or a new release embeds them. | `curl https://picom-code.github.io/ezlm/internal/catalog/catalog.json` compared with `git show HEAD:internal/catalog/catalog.json` before the edit: equal. |
| 6 | info | n/a | `releases/latest` returns `v0.9.0` (2026-11-04), not a `bNNNNN` build; nightlies continue past it (newest `b12144`, 2026-11-06). I verified against `v0.9.0` as the brief says. | GitHub API `releases/latest` and `releases?per_page=8`. |
| 7 | minor (open) | `internal/catalog/catalog.json:25` | The note says "QAT on TPU v5" and "Released 2026-09-17"; the 27B model card does not mention QAT or a TPU, and the HF repo was created 2026-09-16T23:40Z. Not changed: plausible but unproven. | `prism-ml/Ternary-Bonsai-2-27B-gguf` README and HF model API `createdAt`. |
| 8 | minor (open) | `internal/catalog/catalog.json:25` | The note's speed numbers are for the PQ2_0/PTQ1_0 packs (custom kernels), while the only catalog quant is the stock-llama.cpp `Q2_0` file, whose throughput is not in the card table. The note now says "PQ2_0", so it is accurate, but a reader may take it for Q2_0 speed. | Card "Cross-Platform Throughput" table header and the `Q2_0` paragraph. |

## Corrections made

1. **`mtp: true` on nine unsloth quants** (finding 1): `UD-Q2_K_XL`,
   `UD-IQ3_XXS`, `UD-IQ3_S`, `UD-Q3_K_XL`, `UD-IQ4_XS`, `UD-Q4_K_XL`,
   `UD-Q5_K_XL`, `UD-Q6_K`, `Q8_0`.
   Proof per file: `qwen35.nextn_predict_layers = 1`, `block_count = 65`,
   866 tensors, and the 15 `blk.64.nextn.*` tensors. `UD-IQ2_XXS` and
   `UD-IQ2_S` were left alone on purpose: they have `block_count = 64`,
   no `nextn_predict_layers` and 851 tensors.
   Why it matters: `internal/fit/fit.go:505-506` maps `q.MTP` to
   `--spec-type draft-mtp`; without the flag those nine files never get
   self-speculative decoding even though the head is in the file, while the
   four ISTA `-mtp` files do.
2. **Finished the truncated `ternary-bonsai-2-27b` note** (finding 2) from
   the card's throughput table: RTX 5090 129.9, RTX PRO 6000 124.8,
   H100 113.9, RTX 6000 Ada 82.8, RTX 4090 81.2, L40S 74.4, A100 73.9,
   L4 29.8, M5 Pro 28.1 (llama-bench tg128, PQ2_0).

No other value changed. Fields verified and left as they were:

- `ternary-bonsai-2-27b`: `layers 64` (`qwen35.block_count`), `kv_heads 4`
  (`attention.head_count_kv`), `head_dim 256` (`attention.key_length`),
  `kv_layers 16` (`full_attention_interval = 4` -> 16 of 64 blocks),
  `ctx_max 262144` (`context_length`), `params_b 27.36` and
  `active_params_b 27.36` (card: 27.36B total, 24.35B backbone + 2.54B
  embedding/head + 0.46B vision tower), `moe false`, `native_1bit true`
  (end-to-end ternary g128 weights), `vision true` (separate Q8_0 mmproj,
  `Ternary-Bonsai-2-27B-mmproj-Q8_0.gguf` in the repo), `tool_calling true`
  (the GGUF chat template defines `tools`), `license Apache-2.0` (card
  frontmatter and `LICENSE`), quant `Q2_0` 7626008928 bytes exact.
- `qwen3.8-27b`: `params_b 27.78` (HF safetensors total 27,781,427,952),
  `active_params_b 27.78` (dense), `layers 64` (config
  `num_hidden_layers`; GGUF 65 minus the nextn layer), `kv_heads 4`,
  `head_dim 256`, `kv_layers 16` (16 x (3 Gated DeltaNet + 1 full
  attention)), `ctx_max 262144`, `moe false`, `vision true`
  (`vision_config` plus mmproj files in the quant repos), `tool_calling
  true` (template defines `tools`), `license Apache-2.0` (card frontmatter;
  `general.license = apache-2.0` in the GGUF), `draft` repo/file/size/type
  (`ggml-org/Qwen3.8-27B-GGUF`, `mtp-Qwen3.8-27B-Q8_0.gguf`,
  3164006816 bytes, 18-tensor MTP draft with `nextn_predict_layers = 1`).
  All 19 quant sizes and URLs match exactly.

## Open questions

- Should `layers` become 65 for MTP quants, or should the plan add one
  layer when `q.MTP` is set? The model-level field cannot express both.
- Should the catalog grow an mmproj field (repo, file, bytes) so `vision`
  is real? For these two models the projectors are
  `prism-ml/Ternary-Bonsai-2-27B-gguf/Ternary-Bonsai-2-27B-mmproj-Q8_0.gguf`
  and `unsloth/Qwen3.8-27B-GGUF/mmproj-F16.gguf`.
- Is "QAT on TPU v5" for Bonsai 2 27B correct? It is not in the model card.
- The `quality` overrides on the GSQ quants (0.94/0.95/0.985/0.995) are
  the catalog author's estimates; the ISTA card reports task averages, not a
  single recovery factor. Left unchanged.

## Next steps

1. Republish the remote catalog so these corrections reach clients that load
   it remotely (finding 5).
2. Decide on the vision/mmproj wiring (finding 3) and the MTP layer count in
   Split plans (finding 4) at the engine level.
3. If the Lead wants a catalog consistency test, the natural file is
   `internal/catalog/catalog_test.go`; none exists yet and none was assigned
   to me.
