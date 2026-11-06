# T01 a09: gemma-4-12b and lfm3-1.7b

Slice: the two catalog entries `gemma-4-12b` and `lfm3-1.7b` in
`internal/catalog/catalog.json`. Everything below was checked against the
pinned release b11960 and the newest release at the time (v0.9.0, published
2026-11-04), by GGUF header reads (HTTP range requests) and by GETs of the
model cards. No model was downloaded or loaded, no server was started.

## What I checked

1. `scripts/verify_catalog.py --tag b11960 --jobs 2 --only gemma-4-12b,lfm3-1.7b`
   -> 12 file sets, 0 errors, 0 warnings. Saved as `verify-b11960.txt`.
2. Same command with `--tag v0.9.0` -> 12 file sets, 0 errors, 0 warnings.
   Saved as `verify-latest.txt`.
3. By hand, from the GGUF headers of the first quant of each model: every
   sizing field (`layers`, `kv_heads`, `head_dim`, `kv_layers`, `swa_*`,
   `ctx_max`, `tool_calling`, `vision`, `moe`, `params_b`,
   `active_params_b`, `native_1bit`), the tensor list, and the `draft` file
   (the script does not check `draft`).
4. Licence text on Hugging Face (README front matter, `general.license`, and
   the licence file itself) and the chat template for tool support.
5. `https://picom-code.github.io/ezlm/internal/catalog/catalog.json` against
   the embedded copy: before this patch both entries were byte-identical to
   the remote copy, so nothing here was already fixed upstream.

## Findings

| # | file:line | finding | evidence | severity |
|---|---|---|---|---|
| 1 | `internal/catalog/catalog.json:2351` | `lfm3-1.7b` licence carried an `UNVERIFIED amount` marker although the amount is stated in the licence file. Corrected (see below). | `LiquidAI/LFM3-1.7B` `LICENSE` section 1: `"Threshold" shall mean annual revenue of 10 million United States dollars ($10,000,000) or more`; section 5(a): commercial use is licensed only while the entity does not exceed the Threshold. Card front matter: `license_name: lfm1.0`; card table row `License` = `LFM Open License v1.0`. | minor |
| 2 | `internal/catalog/catalog.json:2346-2404` | `lfm3-1.7b` sizing is correct: 24 layers, 6 full-attention layers (4 KV heads x 128), 5 sliding-window layers (window 256), 13 short-conv layers. Verified three ways: GGUF keys, tensor shapes, and `config.json` `layer_types`. | GGUF: `lfm3.block_count=24`, `attention.head_count_kv=[0,0,4,0,4,...,4,0]` (11 non-zero), `attention.key_length=128`, `attention.sliding_window=256`, `attention.sliding_window_pattern` true at layers 4,8,12,16,20, `context_length=262144`, `shortconv.l_cache=7`. Tensors: 11 x `attn_{q,k,v}` with `attn_k` shape (512,2048) = 4 heads x 128, 13 x `shortconv.*`. `config.json`: `num_hidden_layers 24`, `layer_types` = 13 `conv` + 5 `sliding_attention` + 6 `full_attention`, `num_key_value_heads 4`, `head_dim 128`, `sliding_window 256`, `max_position_embeddings 262144`. | none (verified) |
| 3 | `internal/catalog/catalog.json:1252-1348` | `gemma-4-12b` sizing is correct: 48 layers, 8 global layers (1 KV head x 512), 40 sliding layers (8 KV heads x 256, window 1024). | GGUF: `gemma4.block_count=48`, `attention.head_count_kv` = 8 on 40 layers and 1 on layers 5,11,17,23,29,35,41,47, `attention.key_length=512`, `attention.key_length_swa=256`, `attention.sliding_window=1024`, `attention.shared_kv_layers=0`, `context_length=262144`. Tensors: 40 x `attn_v (2048,3840)`, 8 x `attn_k (512,3840)`, 40 x `attn_k_norm (256,)`, 8 x `attn_k_norm (512,)`. Card: `Layers 48`, `Sliding Window 1024 tokens`, `context window of up to 256K tokens`. | none (verified) |
| 4 | `internal/catalog/catalog.json:1262-1277` | `gemma-4-12b` `draft` entry is right, and the script never checks it. The file is the matching MTP head, not another model's. | `MTP/mtp-gemma-4-12b-it-Q8_0.gguf` exists in `unsloth/gemma-4-12b-it-GGUF` with `Content-Length: 465109248`, exactly `bytes`. Its header: `general.architecture gemma4-assistant`, `general.finetune 12b-it-assistant`, `block_count 4`, `nextn_predict_layers 4`, `embedding_length_out 3840` (= the 12B `embedding_length`), `size_label 423M` (423M params at ~1.1 byte/param = 465 MB, consistent). `gemma4-assistant` is a known architecture in both b11960 and v0.9.0 (`src/llama-arch.cpp:64-65`). | none (verified) |
| 5 | `internal/catalog/catalog.json:1271` | Notes claim a "Small mmproj (175MB)"; the repo has `mmproj-BF16.gguf` / `mmproj-F16.gguf` at 175115840 bytes and `vision: true` is right. | Repo tree: `mmproj-BF16.gguf 175115840`; card: "Processes Text, Image ... and Audio (featured natively on the E2B, E4B, and 12B models)", "Supported Modalities Text, Image, Audio" for 12B. | none (verified) |
| 6 | `internal/catalog/catalog.json:1266,2360` | `tool_calling: true` is right for both. | `gemma-4-12b-it-Q8_0.gguf` template defines `format_function_declaration` and renders `tools`; `LFM3-1.7B-Q8_0.gguf` template renders `tools` into `<tools>` and emits `<tool_call>` XML; card: "Pass function schemas through `tools=...`". | none (verified) |
| 7 | `internal/catalog/catalog.json:1258` | `params_b 11.96` vs the card's "Total Parameters 11.95B". Left alone: the difference is 0.01B and the catalog's convention is the precise count, not the card's rounding (compare `gemma-4-e4b` 8.0 = card "8B with embeddings"). `ParamsB` only feeds fit tiers (>=150, >=20) and the MoE active fraction, so 0.01B changes nothing. | Card table `Total Parameters ... 11.95B`; my own config-derived text-tower count is ~11.92B, plus ~30-40M of unified vision/audio projections. | minor, no change |
| 8 | `internal/catalog/catalog.json:1263-1264` | The 8 global layers of `gemma-4-12b` ship no `attn_v` tensor (K and V are one tensor, `attention_k_eq_v` in `config.json`), but llama.cpp still allocates a V cache: `n_embd_v_gqa(il) = n_embd_head_v(il) * n_head_kv(il)`. So the catalog's 2 x K x V formula is correct, not an overcount. | `src/llama-hparams.cpp:162` (`n_embd_v_gqa`), tensor list above; `config.json` `"attention_k_eq_v": true`. | minor, no change |

## Corrections made

1. `internal/catalog/catalog.json:2351`, `lfm3-1.7b` licence:
   - before: `LFM Open License v1.0 (free below a revenue threshold; UNVERIFIED amount)`
   - after: `LFM Open License v1.0 (free below $10M annual revenue)`
   - proof: `LICENSE` section 1 defines `Threshold` as "annual revenue of 10
     million United States dollars ($10,000,000) or more", and section 5(a)
     conditions commercial use on not exceeding it.

No other value in either entry changed. Both entries still match the published
remote catalog apart from this one string.

## Open questions

1. **LFM3 partial rotation may be lost in llama.cpp (not a catalog field).**
   The reference implementation rotates only half of each full-attention head:
   `config.json` sets `partial_rotary_factor 0.5` for `full_attention`, and
   `modeling_lfm3.py:99-110` says "zero-tail partial rotation: the first
   `partial_rotary_factor * head_dim // 2` frequencies ... the rest are zero,
   leaving those dimensions unrotated". The GGUF encodes exactly that in
   `rope_freqs.weight` (64 entries: 32 x 1.0, then 32 x 1e30), but
   `src/models/lfm2.cpp:173,175` (b11960) and the same lines in v0.9.0 call
   `ggml_rope_ext(..., nullptr, n_rot, ...)` with no frequency factors, and
   `lfm3.rope.dimension_count` is 128, so the build rotates all 64 pairs and
   the table is unused. For comparison, `gemma-4-12b` encodes the same idea in
   `rope_freqs.weight` (64 x 1.0 then 192 x 1e30) and `src/models/gemma4.cpp:94-97`
   loads it. Effect on output quality is unproven here because no model was
   loaded; it needs an eval on the b11960 build before it is claimed as a bug.
   The card's "LFM3-compatible llama.cpp builds" wording fits an upstream gap.
2. `lfm3-1.7b` notes mention the `LFM3-VL-2B` variant and a `d1-2B` diffusion
   model; both exist (`LiquidAI/LFM3-VL-2B-GGUF` has the same file sizes plus a
   203031872-byte mmproj, and `LiquidAI/d1-2B` is published). Left as written.
3. The catalog has no field for the LFM3 short-conv state. llama.cpp sizes it
   as `n_embd * (shortconv.l_cache - 1)` per layer (`src/llama-hparams.cpp:214-216`),
   so 13 layers x 2048 x 6 = about 160K values, well under 1 MB. It does not
   change a memory plan, so nothing was added.

## Next steps

- The Lead's full gate is the only remaining check for this slice.
- If the LFM3 rotation question matters, run one perplexity or long-context
  comparison on a b11960 build with `rope_freqs` wired in, or report it
  upstream (read-only, no issue creation from here).
- `scripts/verify_catalog.py` still does not check `draft` files (size, header
  architecture, or that the draft matches the model's hidden size) or licence
  text; both were done by hand here. A follow-up could add the `draft` checks
  to the script: they are the same range reads the script already does.

## Reproduction

```sh
cd ~/ezlm-agents/work/subs/T01/a09
python3 scripts/verify_catalog.py --tag b11960 --jobs 2 --only gemma-4-12b,lfm3-1.7b
python3 scripts/verify_catalog.py --tag v0.9.0 --jobs 2 --only gemma-4-12b,lfm3-1.7b
```
