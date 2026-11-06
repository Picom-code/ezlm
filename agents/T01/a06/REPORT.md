# T01 / a06: `bonsai-27b-1bit` and `qwen3.5-9b`

Agent a06. Branch `agent/T01-a06`. Worktree `~/ezlm-agents/work/subs/T01/a06`.
Owns only the two catalog entries above; no Go test assigned.

## What I checked

1. `scripts/verify_catalog.py` against the pinned release b11960 and against the
   newest release. Saved as `verify-b11960.txt`, `verify-latest.txt` (b12144) and
   `verify-v0.9.0.txt` (see the tag note below). All 10 file sets pass: every
   file exists, every byte size matches the catalog exactly, the architecture and
   every tensor type are known to b11960, and no KV/layer undercount warning fired.
2. Every catalog number read back from the GGUF header with HTTP range requests
   (`dump_header.py`, reusing the verifier's reader).
3. Parameter counts summed from the GGUF tensor table (`gguf_params.py`), so
   `params_b` does not rest on a vendor rounding.
4. Licences against the model cards, tool calling against `tokenizer.chat_template`,
   vision against the mmproj files and the cards.
5. Quant list against the repo file listing, including the MTP sidecar.

## Findings

| # | file:line | Finding | Evidence | Severity |
|---|---|---|---|---|
| 1 | `internal/catalog/catalog.json:2516`, `:2526` | `bonsai-27b-1bit` `params_b` / `active_params_b` were 27.78, the base model's count, not this model's | The Q1_0 tensor table sums to 26,895,998,464 params (26.896B). The shipped vision tower is `Bonsai-27B-mmproj-BF16.gguf`, 931,145,760 bytes, so 465,572,880 params at bf16. 26.896 + 0.466 = 27.36B. The catalog's own sibling entry `ternary-bonsai-2-27b` (line 11) already carries 27.36 for a file with the identical tensor count (26,895,998,464). 27.78 is `Qwen/Qwen3.6-27B`'s safetensors total (55,562,855,904 bytes / 2 = 27.781B), which includes a 1-layer MTP head that the Bonsai GGUF does not ship (`qwen35.block_count` = 64, no `nextn_predict_layers`). | minor |
| 2 | `internal/catalog/catalog.json:2530` | `bonsai-27b-1bit` has no `draft` block although the repo ships two DSpark drafters, and the schema supports `draft-dspark` | `prism-ml/Bonsai-27B-gguf` lists `Bonsai-27B-dspark-Q4_1.gguf` (1,787,468,768 B) and `Bonsai-27B-dspark-bf16.gguf` (7,291,885,792 B); card: "Ships with a DSpark speculative-decoding drafter layer", "1.37x decode speedup on the CUDA serving path". | minor |
| 3 | `internal/catalog/catalog.json:2529` | `bonsai-27b-1bit` is the only entry in the Bonsai family with no `notes` | `ternary-bonsai-2-27b` (`:25`), `ternary-bonsai-2-9b` (`:57`), `ternary-bonsai-2-4b` and `bonsai-8b-1bit` all carry notes with release date, quant caveats and speeds. | minor |
| 4 | `internal/catalog/catalog.json:2528`, `:769` | `vision: true` on both models is factually right but nothing in the Go code reads `Model.Vision` | `grep -rn "Vision" internal/*.go`: only the struct field and `props`/`integrate` runtime plumbing; `internal/integrate/types.go:21` says "an image projector is loaded (never, today)". So the flag has no runtime effect yet, and the deployer loads no mmproj. | minor |
| 5 | `internal/catalog/catalog.json:2519` | Bonsai `tool_calling: true` is right, but tool use degrades much more than the average | Template has a full tools branch (`<tool_call><function=...>` ChatML). Card, by category: agentic 80.00 -> 66.03, BFCL v3 77.10 -> 70.72, tau2-Bench 82.90 -> 61.34. | minor |
| 6 | `internal/catalog/catalog.json:2523` | Bonsai KV math is consistent with the card, worth recording | `kv_layers` 16 (64 blocks / `full_attention_interval` 4), `kv_heads` 4, `head_dim` 256: 2*16*4*256 = 32,768 elems/token, 262,144 tokens at f16 = 17.2 GB, /4 for a q4_0 cache = 4.3 GB, which is the card's "~4.3 GB at the full 262K window". | info |

## Corrections made

`internal/catalog/catalog.json`, `bonsai-27b-1bit` only:

```
-   "params_b": 27.78,
+   "params_b": 27.36,
...
-   "active_params_b": 27.78,
+   "active_params_b": 27.36,
```

Justification: GGUF tensor table 26,895,998,464 params plus the shipped
`Bonsai-27B-mmproj-BF16.gguf` vision tower (931,145,760 bytes at bf16 =
465,572,880 params) gives 27.361B. The catalog's own `ternary-bonsai-2-27b`
entry uses 27.36 for a file with the same 26,895,998,464-param LM. 27.78 is the
base `Qwen3.6-27B` count including an MTP head that this GGUF does not carry.
Formatting, field order and indentation are unchanged; the diff is 2 lines.

Impact: `params_b` feeds only `minQuality` (`internal/fit/fit.go:157`, bucket
`>= 20`) and the MoE active fraction (`fit.go:350`), so no plan changes. This is
a truth fix, not a sizing fix. `q.Bytes` is what the planner sizes from, and
every quant byte count already matched the remote files exactly.

## What matched (no change needed)

`qwen3.5-9b` (catalog line 751). All 9 quant sets verified against b11960 and
b12144: exact byte sizes, files present, `qwen35` arch known, tensor types
{0, 8, 12, 13, 14} all known.

| Field | Catalog | GGUF / card |
|---|---|---|
| layers | 32 | `qwen35.block_count` 32 |
| kv_heads | 4 | `qwen35.attention.head_count_kv` 4 |
| head_dim | 256 | `qwen35.attention.key_length` 256 |
| kv_layers | 8 | `full_attention_interval` 4 on 32 layers, so 8 gated-attention layers (card: "8 x (3 x (Gated DeltaNet -> FFN) -> 1 x (Gated Attention -> FFN))") |
| ctx_max | 262144 | `qwen35.context_length` 262144, card "262,144 natively" |
| params_b / active | 9.65 / 9.65 | safetensors index `total_size` 19,306,216,416 at bf16 = 9.6531B (includes the 0.699B vision tower; the LM-only GGUF is 8.954B) |
| tool_calling | true | template tools branch; card documents `qwen3_coder` tool parser |
| vision | true | card "Causal Language Model with Vision Encoder"; `mmproj-BF16/F16/F32.gguf` in the unsloth repo (0.92 / 0.92 / 1.82 GB) |
| license | Apache-2.0 | card `license: apache-2.0`, GGUF `general.license` `apache-2.0` |
| MTP quant | `UD-Q4_K_XL (MTP)`, `mtp: true` | `unsloth/Qwen3.5-9B-MTP-GGUF` file has `qwen35.block_count` 33 with `nextn_predict_layers` 1 |

`bonsai-27b-1bit` (catalog line 2510). Quant `Q1_0`, 3,803,452,480 bytes, exact
match; `prism-ml/Bonsai-27B-gguf` exists.

| Field | Catalog | GGUF / card |
|---|---|---|
| layers | 64 | `qwen35.block_count` 64 |
| kv_heads | 4 | `qwen35.attention.head_count_kv` 4 |
| head_dim | 256 | `qwen35.attention.key_length` 256 |
| kv_layers | 16 | `full_attention_interval` 4 on 64 layers; card "16 of 64 layers" |
| ctx_max | 262144 | `qwen35.context_length` 262144 |
| native_1bit | true | `general.file_type` 40, tensors are GGML type 41 (`Q1_0`); b11960 has the type, its `type_traits` entry, and CPU, CUDA and Metal kernels (`ggml/src/ggml-metal/kernels/mul_mv.metal:307`) |
| tool_calling | true | template tools branch (ChatML, `<tool_call>`) |
| vision | true | `Bonsai-27B-mmproj-BF16.gguf` / `-Q8_0.gguf` present; card vision category scored |
| license | Apache-2.0 | card `license: apache-2.0`, `LICENSE.txt` is Apache 2.0, NOTICE only asks for attribution |

## Open questions

1. **Which release is "latest"?** The brief's command
   (`releases/latest`) returns `v0.9.0` (published 2026-11-04, not a prerelease),
   but the newest release by date is `b12144` (2026-11-06T03:45Z); the b-tags are
   marked prerelease. I ran both and saved both files. Every check passes on both,
   so the answer does not change for my two models.
2. **Bonsai drafter**: should `bonsai-27b-1bit` carry the Q4_1 DSpark drafter?
   The schema has `draft-dspark` and the model ships one, but enabling it adds a
   1.79 GB download and a `--spec-type` flag that nobody can test tonight without
   loading weights. Recommend a real-weights test before adding.
3. **`vision: true` with no projector**: the deployer never fetches an mmproj, so
   a user asking for images gets a text-only server. The catalog flag is currently
   decorative (see finding 4). Decide whether `vision` should mean "the model can
   see" (today) or "EZLM will load a projector" (not yet).
4. **Bonsai card numbers are rounded**: the card says "~27.3B binary language
   weights", the tensor table says 26.896B. I used the tensor table, which is
   what the catalog's sibling entry already uses.

## Next steps

- Add `draft` (DSpark Q4_1) and a `notes` line to `bonsai-27b-1bit` after a
  real-weights test (findings 2 and 3).
- If `vision` is meant to be actionable, teach the deploy path to fetch the mmproj
  named in the repo and keep `vision: true`; otherwise consider `vision` on the
  Bonsai and Qwen3.5 entries as display-only and say so in the schema comment.
- The Lead's consistency test should assert `params_b` against the sibling
  entries in the same family, since finding 1 was a copy from the base model.

## Reproduce

```sh
python3 scripts/verify_catalog.py --tag b11960 --jobs 2 --only bonsai-27b-1bit,qwen3.5-9b
python3 scripts/verify_catalog.py --tag b12144 --jobs 2 --only bonsai-27b-1bit,qwen3.5-9b
python3 agents/T01/a06/dump_header.py prism-ml/Bonsai-27B-gguf Bonsai-27B-Q1_0.gguf
python3 agents/T01/a06/gguf_params.py prism-ml/Bonsai-27B-gguf Bonsai-27B-Q1_0.gguf
python3 agents/T01/a06/gguf_params.py prism-ml/Ternary-Bonsai-2-27B-gguf Ternary-Bonsai-2-27B-Q2_0.gguf
```
