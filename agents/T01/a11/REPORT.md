# T01 a11: Muse Glimmer 30B and Mistral Small 4 119B

Agent `a11`, branch `agent/T01-a11`, worktree `~/ezlm-agents/work/subs/T01/a11`.
Models owned: `muse-glimmer-30b`, `mistral-small-4-119b`.

## Result

Both entries pass the verifier at b11960 and at the two newer tags. All 15 quant
file sets exist at the listed URLs with the exact listed byte sizes, every split
part follows llama.cpp's `-0000i-of-0000N.gguf` naming, and every architecture
and tensor type is known to b11960. No blocker.

Five corrections: one licence, two parameter counts, two notes. One open
question (Mistral context length) is left unchanged on purpose.

## What I checked

- `scripts/verify_catalog.py --tag b11960 --jobs 2 --only muse-glimmer-30b,mistral-small-4-119b`
  -> `verify-b11960.txt`, 0 errors.
- `--tag v0.9.0` (the tag `releases/latest` returns; published 2026-11-04)
  -> `verify-latest.txt`, 0 errors. `v0.9.0` has 170 architectures and 45 tensor
  types, the same as the newest build tag `b12144` (2026-11-06), so I also ran
  that one -> `verify-b12144.txt`, 0 errors.
- GGUF metadata read by range request for both models (`gguf-metadata.txt`),
  plus the draft and mmproj headers (`draft-mmproj-headers.txt`).
- Parameter counts summed from every tensor in the GGUF (`param-counts.txt`):
  Muse Q8_0 has 731 tensors and 27,854,794,240 language-model parameters;
  Mistral UD-Q4_K_XL has 579 tensors and 118,972,826,624 parameters.
- Tensor types of **every** split part, not just part 1 (`tensor-types.txt`):
  all types (Q8_0, F32, Q4_K, Q5_K, Q6_K, MXFP4, IQ2_XXS, IQ2_S, IQ3_XXS,
  IQ3_S, IQ4_XS) are in b11960 and b12144.
- HF repo file listing for both GGUF repos (`repo-files.txt`): byte sizes,
  mmproj files, Muse's DFlash drafter.
- HF API model metadata: `safetensors.total`, licence tag, `createdAt`.
- Model cards: `meta-models/Muse-Glimmer-30B/README.md`,
  `mistralai/Mistral-Small-4-119B-2603/README.md`.
- `tokenizer.chat_template` of both models for tool support (extracted, not
  committed).
- `src/llama-arch.cpp` at b11960 and b12144 for `muse-glimmer`, `mistral4`,
  `dflash`; `tools/mtmd/clip.cpp` at b11960 for `PROJECTOR_TYPE_MUSE_GLIMMER`.

## Findings

| # | file:line | Finding | Evidence | Severity | Action |
|---|---|---|---|---|---|
| 1 | `internal/catalog/catalog.json:1674` | Muse licence said `Muse Model License (UNVERIFIED terms)`; the model is Apache-2.0. | Model card front matter `license: apache-2.0`, card body "**License:** [Apache 2.0]", HF tag `license:apache-2.0`, repo has `LICENSE`. | major | Fixed to `Apache-2.0`. |
| 2 | `internal/catalog/catalog.json:1675` | Muse `params_b` 30.0 was an approximation; the real total is 29.78B. | HF API `safetensors.total = 29776626688` (29.78B); card "Total Parameters ~29.6B". The catalog convention is the HF total: qwen3.8-27b 27.78 = 27781427952, qwen3.5-9b 9.65 = 9653104368, gemma-4-31b 31.27 = 31273088876, qwen3.5-122b-a10b 125.09 = 125086497008. | minor | Fixed to `29.78`. |
| 3 | `internal/catalog/catalog.json:1685` | Muse `active_params_b` 30.0 for a dense model; should equal `params_b`. | No `*.expert_count` in the GGUF (dense); every other dense entry has `active_params_b == params_b` (e.g. qwen3.8-27b 27.78/27.78). | minor | Fixed to `29.78`. |
| 4 | `internal/catalog/catalog.json:1688` | Muse note said "Dense ~30B (param count approximate)". | HF total 29.78B; GGUF language-model tensors 27,854,794,240 = 27.85B; mmproj header `general.size_label = 1.9B`. GGUF `general.size_label = 28B` is the LM-only label. | minor | Note now states the split. |
| 5 | `internal/catalog/catalog.json:1786` | Mistral note called the active count "~6.5B UNVERIFIED". | Model card: "119B parameters, with 6.5B activated per token". | minor | Removed UNVERIFIED, cited the card. |
| 6 | `internal/catalog/catalog.json:1780` | Mistral `ctx_max` 1048576 disagrees with the model card's "256k context length". | GGUF `mistral4.context_length = 1048576`; base `config.json` `text_config.max_position_embeddings = 1048576`; `rope.scaling.factor = 128` over `original_context_length = 8192`. Card says 256k twice. | minor (open) | Kept 1048576: it is what the file and the base config declare. See open questions. |
| 7 | `scripts/verify_catalog.py:129-137` | The tensor-type check reads only the file the loop calls `i == 0`; for every split quant here, part 1 is a 7.8 MB header-only file with **zero tensors**, so the check is vacuous. | `tensor-types.txt`: Mistral parts 1+2+3 hold 579 tensors; the MXFP4_MOE set has 35 `MXFP4` tensors that part 1 never shows. | minor | Reported only; `scripts/verify_catalog.py` is shared, and this slice owns only the two model objects. All types were checked by hand instead. |

No 404s, no missing files, no split-name mismatch, no unknown architecture or
tensor type. `dflash-kquant.gguf` (1,631,205,312 bytes) exists and its
`general.architecture = dflash` is known to b11960.

## Field-by-field

### muse-glimmer-30b

| Field | Catalog | File / card | Verdict |
|---|---|---|---|
| `layers` | 52 | `muse-glimmer.block_count = 52`; card "Layers 52" | ok |
| `kv_heads` | 2 | `attention.head_count_kv = 2`; card "32 / 2 (GQA ratio 16:1)" | ok |
| `head_dim` | 128 | `attention.key_length = 128`, `value_length = 128`; card 128 | ok |
| `kv_layers` | 13 | `attention.sliding_window_pattern = 4` over 52 layers -> 13 full, card "[Local, Local, Local, Global] repeating" | ok |
| `swa_layers` / `swa_window` / `swa_kv_heads` / `swa_head_dim` | 39 / 2048 / 2 / 128 | same keys; card "Sliding window size 2048" | ok |
| `ctx_max` | 131072 | `context_length = 131072`; card "131,072+"; base `config.json` 131072 | ok |
| `vision` | true | card "dedicated perception encoder"; repo has `mmproj-kquant.gguf` (1,400,328,928), `mmproj-Q8_0` (2,051,685,088), `mmproj-BF16` (3,849,173,728); `clip.projector_type = muse-glimmer` known to b11960 | ok |
| `moe` | false | no `expert_count` key; 731 dense tensors | ok |
| `tool_calling` | true | template renders `tools`, `tool_calls` and `<atem:function_calls>` | ok |
| `params_b` | 29.78 | HF total 29,776,626,688; GGUF LM 27,854,794,240 + mmproj 1.9B | fixed |
| `active_params_b` | 29.78 | dense | fixed |
| `license` | Apache-2.0 | card, HF tag, `LICENSE` | fixed |
| `draft` | `dflash-kquant.gguf`, 1,631,205,312 | file exists at that exact size; `dflash` arch known to b11960 | ok |
| quants (8) | sizes | all exact: IQ2_XXS 10746373152, IQ2_XS 11513104416, IQ3_XXS 13130658848, Q3_K_XL 13360983072, Q4_K_XL 15878222368, Q5_K_XL 21789618976, Q6_K_XL 26265362976, Q8_0 29612957984 | ok |

### mistral-small-4-119b

| Field | Catalog | File / card | Verdict |
|---|---|---|---|
| `layers` | 36 | `mistral4.block_count = 36`; base `text_config.num_hidden_layers = 36` | ok |
| `kv_heads` | 1 | `attention.head_count_kv = 1` | ok |
| `head_dim` | 320 | MLA: `attention.kv_lora_rank = 256` + `rope.dimension_count = 64` = 320; `key_length = 320` | ok |
| `kv_layers` | 36 | every layer full attention (no sliding-window keys) | ok |
| `ctx_max` | 1048576 | GGUF 1048576; base config 1048576; card says 256k | open (see #6) |
| `vision` | true | card "Multimodal input"; repo has `mmproj-F16` (857,078,496), `mmproj-BF16` (866,671,328), `mmproj-F32` (1,713,920,736) | ok |
| `moe` | true | `expert_count = 128`, `expert_used_count = 4` | ok |
| `tool_calling` | true | template renders `[AVAILABLE_TOOLS]` and `tool_calls` | ok |
| `params_b` | 119.40 | HF total 119,401,317,376 = 119.40; GGUF LM 118,972,826,624 (encoder is in mmproj) | ok |
| `active_params_b` | 6.5 | card "6.5B activated per token"; GGUF routed experts 115,964M, so 4/128 gives ~6.63B by weight share | ok (card value) |
| `license` | Apache-2.0 | card, HF tag, GGUF `general.license = apache-2.0` | ok |
| quants (7) | sizes | all exact, including the three-part sets: IQ4_XS 58126815232, MXFP4_MOE 71812829184, Q4_K_XL 74161639424 | ok |

## Open questions

1. **Mistral context: 1M or 256k?** The GGUF (`mistral4.context_length`) and the
   base `config.json` both say 1,048,576, but the model card says 256k twice.
   The 1M value is a YaRN extension (`factor = 128` over `original_context_length
   = 8192`). I kept 1M because that is what llama.cpp will accept; quality past
   256k is unproven. A 1M-token KV cache at f16 is about 23 GB for this model,
   so a user who picks the maximum context pays for it. If the Lead prefers the
   card, the value is 262144.
2. **Mistral release date.** The note says 2026-03, which matches the Unsloth
   GGUF repo (`createdAt 2026-03-17`), but the base model repo is `2026-01-23`.
   I did not change it.
3. **Mistral active parameter count.** 6.5B is the card's number and I kept it.
   Summed from the GGUF, 4 of 128 routed experts plus the non-expert tensors is
   about 6.63B. The difference changes the MoE bandwidth estimate by ~2%.
4. **Muse `vision` is informational today.** `internal/integrate/types.go:21`
   says an image projector is "never, today" loaded, so `vision: true` only
   documents the repo. The mmproj files exist and the projector type is in
   b11960, so the flag is honest.
5. Scores were out of scope and are not verified (`muse` 6.3, `mistral` 4.3).

## Next steps

- `scripts/verify_catalog.py`: read tensor types from the last split part (or all
  parts) instead of part 1, which is header-only for every Mistral quant here.
- `scripts/verify_catalog.py`: it never checks `draft` files or `mmproj` files;
  both exist for these models, but a missing drafter would only show up at run
  time.
- A catalog consistency test (T01 owns `internal/catalog/*_test.go`) should
  assert `active_params_b == params_b` for dense models, `license` non-empty and
  not containing "UNVERIFIED", and the split-name pattern.

## Artifacts

- `verify-b11960.txt`, `verify-latest.txt` (v0.9.0), `verify-b12144.txt`
- `gguf-metadata.txt`, `draft-mmproj-headers.txt`, `param-counts.txt`,
  `tensor-types.txt`, `repo-files.txt`
