# T01 a03: catalog truth audit for `ternary-bonsai-2-4b` and `gemma-4-31b`

Worktree `~/ezlm-agents/work/subs/T01/a03`, branch `agent/T01-a03`.
Models: `ternary-bonsai-2-4b`, `gemma-4-31b`. Catalog version 8.

## What I checked

- `scripts/verify_catalog.py` against the pinned release **b11960** (`verify-b11960.txt`) and the
  newest release at audit time, **v0.9.0** (published 2026-11-04, commit `cd959bae`;
  `verify-latest.txt`). Both runs: 11 quant file sets, 0 errors. Every file exists, every
  `bytes` value matches `Content-Length`, architecture and all tensor types are known to both
  builds.
- GGUF header metadata for every quant of both models (`gguf-metadata.txt`). All gemma quants
  carry identical architecture metadata; the Bonsai repo has one quant.
- The separate draft files, which `verify_catalog.py` does not inspect: existence, size, GGUF
  architecture, tensor types, and that the architecture is in b11960/v0.9.0.
- Licence tags against the Hugging Face model cards; `tool_calling` against the
  `tokenizer.chat_template` in the GGUF (both templates implement a `tools` block).
- `vision` against the mmproj files published in the same repos.
- `params_b` / `active_params_b` by summing GGUF tensor element counts (language model plus the
  vision tower, the convention the other Bonsai entries already follow; `params.txt`).

Tensor type ids confirmed in b11960 and v0.9.0: 0 F32, 8 Q8_0, 30 BF16, 42 Q2_0.

## Findings

| # | file:line | Finding | Evidence | Severity |
|---|---|---|---|---|
| 1 | `internal/catalog/catalog.json:81` and `:91` | `ternary-bonsai-2-4b` `params_b` and `active_params_b` were 4.66, the total of the **base** Qwen3.5-4B, not of this model. Correct total is 4.54. | GGUF tensor counts: `Ternary-Bonsai-2-4B-Q2_0.gguf` 426 tensors = 4,205,751,296 params; `Ternary-Bonsai-2-4B-mmproj-Q8_0.gguf` 298 tensors = 333,514,240 params; sum 4,539,265,536 = 4.54B. Model card line 45: "~4.21B language model (32 blocks, hidden 2560, tied embedding/LM head) + ~0.33B vision tower (24 blocks)". Sibling entries use the same LM+vision convention: 9B 8.9538B + 0.4560B = 9.41, 27B 26.8960B + 0.4607B = 27.36. The old 4.66 is exactly the base `Qwen/Qwen3.5-4B` safetensors total (4,659,865,088), i.e. the wrong basis. | minor |

Everything else matched:

- `ternary-bonsai-2-4b`: `layers` 32 = `qwen35.block_count` 32; `kv_heads` 4 =
  `qwen35.attention.head_count_kv` 4; `head_dim` 256 = `attention.key_length` 256;
  `kv_layers` 8 = 32 layers / `full_attention_interval` 4; `ctx_max` 262144 =
  `qwen35.context_length`; `moe` false; `tool_calling` true (template has `<tools>` and
  `tool_calls`); `native_1bit` true (ternary weights, card "Ternary {−1, 0, +1}");
  `vision` true (mmproj Q8_0 0.37 GB, card); `license` Apache-2.0 (card frontmatter and
  `LICENSE`); `repo` exists; `Q2_0` 1212103520 bytes matches.
- `ternary-bonsai-2-4b` `draft`: `Ternary-Bonsai-2-4B-DFlash2-BF16.gguf` exists,
  1575278208 bytes, arch `dflash` (known to b11960 and v0.9.0), types F32/BF16,
  `target_layers` [2,6,10,14,18,22,26,30] = the 8 full-attention layers; `draft-dflash` is a
  type `internal/fit` handles.
- `gemma-4-31b`: `layers` 60 = `gemma4.block_count`; `kv_heads` 4 = the 4-entry in
  `attention.head_count_kv` (10 layers); `kv_layers` 10 = count of `sliding_window_pattern`
  false entries; `head_dim` 512 = `attention.key_length`; `swa_layers` 50 / `swa_kv_heads` 16
  / `swa_head_dim` 256 / `swa_window` 1024 = pattern true count, `head_count_kv` 16,
  `key_length_swa`, `sliding_window`; `ctx_max` 262144 = `gemma4.context_length`;
  `moe` false (card "31B Dense"); `tool_calling` true (template has `<|tool>` blocks; card
  "native function-calling support"); `vision` true (mmproj files in all three quant repos,
  card "Vision Encoder Parameters ~550M"); `license` Apache-2.0 (card frontmatter, body, and
  the `license_link` page is titled "Apache License 2.0"); `params_b` 31.27 = 30.6973B
  language model + 0.5757B mmproj = 31.273B; all 10 quants and their repos exist with exact
  sizes; all tensor types known to b11960 and v0.9.0.
- `gemma-4-31b` `draft`: `MTP/mtp-gemma-4-31B-it-Q8_0.gguf` exists, 514687104 bytes,
  arch `gemma4-assistant` (known to b11960 and v0.9.0), types F32/Q8_0, Q8_0 is the
  recommended precision in `MTP/README.md`, and the README says the drafter "pairs with any
  quant of the 31B".

## Corrections

1. `ternary-bonsai-2-4b`: `params_b` 4.66 -> 4.54 and `active_params_b` 4.66 -> 4.54.
   Justification: GGUF tensor counts (4.21B LM + 0.33B vision = 4.54B) and model card line 45;
   the old value was the base Qwen3.5-4B total. No other field changed.

The embedded and remote (`https://picom-code.github.io/ezlm/internal/catalog/catalog.json`)
catalogs are identical for both of my models (both version 8), so the same correction is needed
in the published copy.

## Open questions

- `gemma-4-31b` global layers use unified K and V (card: "global layers feature unified Keys
  and Values"). If llama.cpp stores one tensor per layer instead of K and V, `KVBytesPerToken`
  overcounts the global-layer cache by 2x. Overcounting is conservative for the planner, and I
  could not prove the cache layout from the header, so I left `head_dim` 512 (=
  `attention.key_length`) as is.
- `vision: true` on both models is a capability claim: the catalog has no mmproj repo/file/size
  field, and `internal/integrate/types.go` says an image projector is "never, today" loaded.
  The mmproj files exist (Bonsai 4B Q8_0 0.37 GB, Gemma 4 31B mmproj 0.55B params), so the flag
  is accurate about the model, but nothing downloads the projector.
- `ternary-bonsai-2-4b` `Q2_0` needs llama.cpp b11556 or newer (card). The pinned b11960 and
  v0.9.0 both work, but the catalog has no minimum-build field to express this.
- The `notes` release dates ("2026-09-23", "2026-03-11") are not stated on the model cards and
  I could not verify them.
- `gemma-4-31b` `score_source` is an estimate, not audited here.

## Next steps

- The Lead should apply the same 4.54 value to the remote/published catalog copy.
- If `params_b` is meant to drive the quality floor (`internal/fit/fit.go:159`), both 4.54 and
  4.66 stay under the 20B threshold, so the fix has no ranking effect today; it matters for
  display and for any future threshold.
