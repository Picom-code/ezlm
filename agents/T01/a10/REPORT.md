# T01 a10 report: gpt-oss-120b and gpt-oss-20b

Agent `a10`, branch `agent/T01-a10`, worktree `~/ezlm-agents/work/subs/T01/a10`.
Models owned: `gpt-oss-120b` (catalog.json:1562-1614), `gpt-oss-20b` (catalog.json:1616-1666).

## Summary

Both entries are numerically correct against the GGUF headers, the upstream
`config.json` and the model cards. One correction: the `gpt-oss-20b` note was
copy-pasted from the 120b entry and carried the 120b speed numbers.

- Corrections: 1 (minor, text only).
- Blockers: none. All six quant file sets exist at the listed sizes on
  Hugging Face, at both b11960 and the newest release (`v0.9.0`).
- Hand-checked fields the verifier only warns about (it flags undercounts, not
  exact values): `kv_heads`, `head_dim`, `kv_layers`, `swa_*`, `params_b`,
  `active_params_b`, `license`, `tool_calling`, `ctx_max`, `vision`, `moe`,
  `native_1bit`.

## What I ran

```sh
python3 scripts/verify_catalog.py --tag b11960 --jobs 2 --only gpt-oss-120b,gpt-oss-20b \
  | tee agents/T01/a10/verify-b11960.txt
# llama.cpp b11960: 166 architectures, 44 tensor types; checking 6 files sets
# all 6 ok, 0 file set(s) with errors

python3 scripts/verify_catalog.py --tag v0.9.0 --jobs 2 --only gpt-oss-120b,gpt-oss-20b \
  | tee agents/T01/a10/verify-latest.txt
# llama.cpp v0.9.0: 170 architectures, 45 tensor types; checking 6 files sets
# all 6 ok, 0 file set(s) with errors
```

`v0.9.0` is what `https://api.github.com/repos/ggml-org/llama.cpp/releases/latest`
returns today. Beyond the script I read each first-part GGUF header with the same
`header()` helper (range requests only, no weights) and compared every field by
hand, and I listed the four Hugging Face repos with the tree API to confirm exact
byte sizes and that no split part is missing.

## Findings

| # | file:line | Finding | Evidence | Severity |
|---|---|---|---|---|
| 1 | `internal/catalog/catalog.json:1636` | `gpt-oss-20b` note was a copy of the 120b note: it claimed the DGX Spark numbers `tg32 ~60 t/s, pp2048 ~1950 (120b)` and called the 20b "the main US-made mid-size MoE". Those speeds are the 120b's, and the sentence is byte-identical to `catalog.json:1582`. | Same string in both entries; the parenthetical says `(120b)`. 20b is 24 layers / 32 experts / 3.6B active (GGUF `gpt-oss.block_count` 24, `gpt-oss.expert_count` 32; card line 26). | minor |

No blocker or major finding. Nothing 404s: all six quant file sets resolve.

## Corrections made

1. `internal/catalog/catalog.json:1636` (`gpt-oss-20b.notes`) rewritten as:

   > Aug 2025 - oldest model kept; the small US-made MoE (21B total, 3.6B active)
   > that runs from a 12 GB MXFP4 file. Half the layers sliding-window(128).
   > Stale vs 2026 Qwen/GLM; keep for license/US-origin/speed reasons.

   Justification: model card `openai/gpt-oss-20b/README.md:26` says "21B
   parameters with 3.6B active parameters"; the MXFP4 file is 12,109,566,624
   bytes (`gpt-oss-20b-MXFP4.gguf`, HF tree API); `gpt-oss.attention.sliding_window`
   is 128 and half the layers use it (see below). The old note's numbers were
   the 120b's. No numeric field changed, field order and 2-space formatting are
   untouched (diff is one line).

## Field-by-field verification

Sources: GGUF metadata of `ggml-org/gpt-oss-120b-GGUF/gpt-oss-120b-MXFP4.gguf`
and `ggml-org/gpt-oss-20b-GGUF/gpt-oss-20b-MXFP4.gguf` (headers only, via
`scripts/verify_catalog.py`'s `header()`), `config.json` from the `openai/`
repos, and the model cards.

| Field | 120b catalog | 120b source | 20b catalog | 20b source | Verdict |
|---|---|---|---|---|---|
| `license` | Apache-2.0 | `general.license = apache-2.0`; card `license: apache-2.0` | Apache-2.0 | same | ok |
| `params_b` | 116.83 | sum of GGUF tensor shapes = 116.829156672e9 | 20.91 | sum = 20.914757184e9 | ok |
| `active_params_b` | 5.1 | card line 25 "117B parameters with 5.1B active" | 3.6 | card line 26 "3.6B active" | ok |
| `moe` | true | `gpt-oss.expert_count` 128, `expert_used_count` 4 | true | 32 / 4 | ok |
| `layers` | 36 | `gpt-oss.block_count` 36; config `num_hidden_layers` 36 | 24 | 24 / 24 | ok |
| `kv_heads` | 8 | `gpt-oss.attention.head_count_kv` 8 (of `head_count` 64) | 8 | 8 | ok |
| `head_dim` | 64 | `attention.key_length` 64 and `value_length` 64; config `head_dim` 64 | 64 | 64 / 64 | ok |
| `ctx_max` | 131072 | `gpt-oss.context_length` 131072; config `max_position_embeddings` 131072 | 131072 | same | ok |
| `kv_layers` | 18 | config `layer_types`: 18 `full_attention` of 36; llama.cpp b11960 `src/models/openai-moe.cpp:9` `load_swa_pattern(ml, 2)` marks alternating layers SWA (`src/llama-hparams.cpp:8-20`), so 18 layers get the full-context cache | 12 | 12 of 24 | ok |
| `swa_layers` | 18 | 36 - 18 full | 12 | 24 - 12 | ok |
| `swa_kv_heads` / `swa_head_dim` | 8 / 64 | no `attention.*_swa` keys, so SWA layers reuse `head_count_kv` 8 and `key_length` 64 | 8 / 64 | same | ok |
| `swa_window` | 128 | `gpt-oss.attention.sliding_window` 128 | 128 | 128 | ok |
| `tool_calling` | true | `tokenizer.chat_template` present in every file (16.7-17.8k chars) and renders tools: `render_tool_namespace`, `tool_call` macros, `functions` namespace | true | same | ok |
| `vision` | false | text-only model card, no vision tensors in GGUF | false | same | ok |
| `native_1bit` | absent | not a 1-bit model; native weights are MXFP4 experts | absent | same | ok |
| `repo` | openai/gpt-oss-120b | repo exists, card fetched | openai/gpt-oss-20b | exists | ok |

### Quants

| Entry | Quant | Bytes in catalog | Actual | Check |
|---|---|---|---|---|
| 120b | MXFP4 `ggml-org/gpt-oss-120b-GGUF` | 63387346208 | 63387346208 | exact |
| 120b | UD-Q4_K_XL `unsloth/gpt-oss-120b-GGUF` (2 parts) | 63016311392 | 49866326912 + 13149984480 | exact, part names `00001-of-00002`/`00002-of-00002` match what llama.cpp looks for |
| 120b | F16 `unsloth/gpt-oss-120b-GGUF` | 65369017728 | 65369017728 | exact, 36 blocks, complete |
| 20b | MXFP4 `ggml-org/gpt-oss-20b-GGUF` | 12109566624 | 12109566624 | exact |
| 20b | UD-Q4_K_XL `unsloth/gpt-oss-20b-GGUF` | 11872347328 | 11872347328 | exact, single file |
| 20b | F16 `unsloth/gpt-oss-20b-GGUF` | 13792639168 | 13792639168 | exact, 24 blocks, complete |

Tensor types in every file (0 F32, 1 F16, 6 Q5_0, 8 Q8_0, 12 Q4_K, 14 Q6_K,
39 MXFP4) are all known to b11960 and v0.9.0. The split part 2 of the 120b
UD-Q4_K_XL carries no metadata, which is normal for a non-first part.

### File identity, for future drift checks

The catalog has no per-file ETag baseline, so a later run can compare against
these. From `curl -sI` on the resolve URL (`x-linked-etag`, the LFS sha256):

| File | ETag |
|---|---|
| 120b MXFP4 | `582bd40f6886200101f4c4ed9f25f3fe80cc14c86e9e2b37746cd8904a0c622d` |
| 120b UD-Q4_K_XL part 1 | `c7b59d68c889ada280736d9b82685baeb6152f4d7423a169a369bb3492b3056e` |
| 120b F16 | `2d1f0298ae4b6c874d5a468598c5ce17c1763b3fea99de10b1a07df93cef014f` |
| 20b MXFP4 | `27cd6c432c7672cb812a92f611cf3ba7bbc35928262bb1e1253ff4ee6ae35901` |
| 20b UD-Q4_K_XL | `10fe673de12c20b74b8d670a9fdf0fd36b43b0a86ffc04daeb175c0a2b98c4f9` |
| 20b F16 | `4e4f9cd88d6456e4f389e7262eca4a8d565211e2b22ece9ca7a8556168ff3c66` |

## Observations (checked, no change needed)

- The two `F16` quants are not full F16: `ffn_*_exps` stay at MXFP4 (type 39,
  108 tensors for 120b, 72 for 20b) while attention, embeddings and output are
  F16 (type 1) and norms F32 (type 0). That is exactly the released precision
  of gpt-oss (the original checkpoint keeps experts at MXFP4), so `quantQuality`
  giving F16 1.0 and MXFP4 0.99 is right. File sizes are consistent with it:
  65.37 GB (F16) vs 63.39 GB (MXFP4) for 120b.
- The `ggml-org` MXFP4 files store the non-expert tensors as Q8_0 (type 8),
  not MXFP4. Still "near-native", so 0.99 is a fair quality factor.
- The remote catalog at `https://picom-code.github.io/ezlm/internal/catalog/catalog.json`
  is version 8 and byte-identical to the embedded copy for both of my entries,
  including the old 20b note. So the note fix needs a catalog version bump or a
  republish to reach clients; that is the Lead's call, not mine.

## Open questions

1. `no_kv_quant: true` on both entries is a quality claim I could not prove.
   b11960 supports attention sinks in both paths (`ggml_flash_attn_ext_add_sinks`
   and `ggml_soft_max_add_sinks`, `src/llama-graph.cpp:3015,3074`), so a
   quantized KV cache is technically accepted for gpt-oss; whether it measurably
   hurts quality is not documented in the source I read. I left the flag alone
   because it is the conservative choice and the cost is modest (120b at 64k
   context: 2.4 GB f16 KV vs 1.3 GB q8_0). If someone has quality evidence,
   dropping the flag would save about 1 GB on 120b plans.
2. `score` / `score_source` are estimates from an ArenaIndex fit and cannot be
   verified from the files; left untouched.
3. The 120b note's DGX Spark speeds (`tg32 ~60 t/s, pp2048 ~1950`) are a
   community measurement with no source in the catalog; I could not re-run it
   without loading the model.

## Next steps

- `ggml-org/gpt-oss-20b-GGUF` publishes `eagle3-gpt-oss-20b-BF16.gguf`
  (1,722,588,800 bytes) and `eagle3-gpt-oss-20b-Q8_0.gguf` (921,488,000 bytes),
  and `ggml-org/gpt-oss-120b-GGUF` publishes `eagle3-gpt-oss-120b-BF16.gguf`
  (1,586,786,496 bytes) and `eagle3-gpt-oss-120b-Q8_0.gguf` (849,103,296 bytes).
  `internal/fit/fit.go:501` already accepts `draft-eagle3`, but neither gpt-oss
  entry has a `draft`. Adding one would speed up CUDA plans; it needs a measured
  acceptance rate first, so I did not guess.
- If the Lead bumps the catalog version, carry the 20b note fix into the
  published copy as well.
