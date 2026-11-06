# T01 a04: ternary-bonsai-2-2b and nanbeige4.3-33b

Worktree `~/ezlm-agents/work/subs/T01/a04`, branch `agent/T01-a04`. Owned entries:
`internal/catalog/catalog.json` lines 112-142 (`ternary-bonsai-2-2b`) and 2405-2520
(`nanbeige4.3-33b`). No Go test file was assigned, so none was added.

Result: every number in both entries is correct against the GGUF headers and the model
cards. Two sentences in the `nanbeige4.3-33b` note were wrong and are corrected. No
blocker.

## What I ran

```sh
python3 scripts/verify_catalog.py --tag b11960 --jobs 2 --only ternary-bonsai-2-2b,nanbeige4.3-33b
python3 scripts/verify_catalog.py --tag v0.9.0 --jobs 2 --only ternary-bonsai-2-2b,nanbeige4.3-33b
```

Output is in `verify-b11960.txt` and `verify-latest.txt` in this folder: 11 file sets,
0 errors against both builds. The latest release is `v0.9.0` (published 2026-11-04,
166 -> 170 architectures, 44 -> 45 tensor types); llama.cpp tags are semantic now, not
`bNNNNN`.

The script only warns when the catalog *undercounts* (`scripts/verify_catalog.py:160`,
`:177-201`): a `kv_heads`, `head_dim`, `kv_layers` or `ctx_max` that is too large passes
silently, and so does a wrong `vision`, `license`, `params_b`, `moe` or `tool_calling`.
So I re-checked every field in both directions by hand.

## Evidence per field

### ternary-bonsai-2-2b (`internal/catalog/catalog.json:113`)

| Field | Catalog | GGUF / card | Proof |
|---|---|---|---|
| layers | 24 | `qwen35.block_count` 24 | header of `Ternary-Bonsai-2-2B-Q2_0.gguf` |
| kv_heads | 2 | `qwen35.attention.head_count_kv` 2 | same; `attn_k.weight` is [2048, 512] = 2 heads x 256 |
| head_dim | 256 | `attention.key_length`/`value_length` 256 | same; `attn_k_norm.weight` [256] |
| kv_layers | 6 | 6 full-attention blocks (3, 7, 11, 15, 19, 23) | `full_attention_interval` 4 x 24 blocks; exactly six `attn_k.weight` tensors |
| ctx_max | 262144 | `qwen35.context_length` 262144 | same; card "262K-token context" |
| params_b | 2.21 | 1,881,825,088 LM + 331,416,576 vision = 2.213B | summed tensor shapes of the Q2_0 file and of `Ternary-Bonsai-2-2B-mmproj-Q8_0.gguf` |
| active_params_b | 2.21 | dense, no experts | no `expert_count` key, one `ffn_*` set per block |
| moe | false | dense | same |
| vision | true | mmproj has `clip.has_vision_encoder` true, 24 blocks, `qwen3vl_merger`; card "Vision input through a separate mmproj pack" | mmproj header, card |
| native_1bit | true | ternary {-1,0,+1} weights, Q2_0 is the native packing | card "Weight format Ternary {-1, 0, +1}"; matches the other Bonsai 2 entries |
| tool_calling | true | `tokenizer.chat_template` has `tools`, `tool_call`, `tool_calls` handling (Qwen3.5 template) | template in the header |
| license | Apache-2.0 | card `license: apache-2.0`, LICENSE in the repo, base Qwen3.5-2B | HF card + API |
| repo | prism-ml/Ternary-Bonsai-2-2B-gguf | exists, 744k downloads, created 2026-09-23 | HF API; matches the "Released 2026-09-23" note |
| quant Q2_0 | 546689504 B | `Ternary-Bonsai-2-2B-Q2_0.gguf` 546689504 B | HEAD and HF API blob size, exact |

Tensor types in the file: F32 and Q2_0, both known to b11960 and v0.9.0. The card says
the Q2_0 packing needs upstream llama.cpp b11556 or newer for the Hadamard activation
runtime; the pinned build is b11960 (`internal/engine/pinned.go:8`), so this is fine.
The smaller PTQ1_0 (0.43 GB) and PQ2_0 (0.52 GB) packings are correctly left out: they
need the PrismML fork. The card confirms the 2B ships without a draft model, so the
missing `draft` block is right.

### nanbeige4.3-33b (`internal/catalog/catalog.json:2406`)

| Field | Catalog | GGUF / card | Proof |
|---|---|---|---|
| layers | 52 | `llama.block_count` 52 | header of `Nanbeige_Nanbeige4.3-33B-Q4_K_M.gguf`; 52 `attn_*` tensor sets |
| kv_heads | 8 | `llama.attention.head_count_kv` 8 | `attn_k.weight` [7168, 1024] = 8 x 128 |
| head_dim | 128 | `attention.key_length`/`value_length` 128 | same; `attn_q.weight` [7168, 7168] = 56 x 128 |
| kv_layers | 52 | all 52 blocks carry K and V | 52 `attn_k.weight`, no sliding window key, no `full_attention_interval` |
| ctx_max | 524288 | `llama.context_length` 524288 | same; card "up to 524,288 tokens (512K)" |
| params_b | 33.12 | 33,118,977,024 | GGUF tensor sum and the base repo's safetensors total, identical |
| active_params_b | 33.12 | dense | one `ffn_*` set per block, no expert keys |
| moe | false | dense | same |
| vision | false | llama arch, text-generation, no vision tensors | card tags; template mentions `image` only in a content-item branch |
| tool_calling | true | `tools` branch renders `<tool_call>` in xml or json | template; card section 3 "Passing `tools` enables the tool-use template" |
| license | Apache-2.0 | card `license: apache-2.0`, GGUF `general.license` apache-2.0 | both |
| repo | Nanbeige/Nanbeige4.3-33B | exists, created 2026-10-24 | HF API; matches the score_source date |
| 10 quants | sizes | all ten files exist, every `bytes` exact | HEAD and HF API blob sizes |

`Q6_K_S` is a real bartowski file (27,750,272,128 B) and uses only F32/Q8_0/Q6_K, so it
is not a new tensor type. Quant order is ascending by size. All ten files have the same
block count, KV heads, key length and context, so one plan fits every quant.

## Findings

| # | Where | Evidence | Severity |
|---|---|---|---|
| F1 | `internal/catalog/catalog.json:2425` | note said "KV ~6x heavier per token than Qwen3.8-27B". 52 x 8 x 128 x 2 = 106,496 elements/token here; Qwen3.8-27B is 16 x 4 x 256 x 2 = 32,768 (its GGUF `qwen35.attention.key_length` is 256, `full_attention_interval` 4 of 64 blocks). Ratio 3.25, not 6. | minor |
| F2 | `internal/catalog/catalog.json:2425` | note said "Tool calling UNVERIFIED" while the flag is `true` and both the GGUF template and the card document tool calls. | minor |
| F3 | `scripts/verify_catalog.py:160,177-201` | the verifier only warns on undercounts; an overcounted `kv_heads`, `head_dim`, `kv_layers` or `ctx_max` (or a `ctx_max` up to 4x the file value) passes as "ok". Overcounts do not risk an OOM, but they can reject a machine that would fit. Script is not mine, not changed. | minor |
| F4 | `internal/catalog/catalog.json:131` (and 22 other entries) | `vision: true` is not deliverable today: no mmproj file is in any catalog entry, no Go code mentions mmproj, `internal/integrate/types.go:21` says "an image projector is loaded (never, today)", and the catalog field is never read (only llama-server `/props` sets `Vision`). For this model the flag is accurate about the model, not about what EZLM can do. Not changed: the model really is multimodal. | minor |
| F5 | `internal/catalog/catalog.json:113-142` | the entry is otherwise fully correct: no numeric correction was needed for ternary-bonsai-2-2b. | info |

## Corrections made

1. `nanbeige4.3-33b` note: "KV ~6x heavier per token than Qwen3.8-27B" -> "KV ~3.3x
   heavier per token than Qwen3.8-27B: 106,496 vs 32,768 elements". Proof: the numbers
   above, from both GGUF headers.
2. `nanbeige4.3-33b` note: "Tool calling UNVERIFIED." -> "Tool calling verified: the GGUF
   chat template renders tools as `<tool_call>` (xml or json)." Proof: template branch on
   `tools` in `tokenizer.chat_template`, and model card section 3.

No other field changed. Formatting, field order and 2-space indent are untouched.

## Remote catalog

Both published copies (raw.githubusercontent.com/picom-code/ezlm/main and
picom-code.github.io) are version 8 with 32 models and are byte-identical to the embedded
copy for these two ids; no differences to report.

## Open questions

- The nanbeige note's benchmark numbers (AutomationBench 53% vs 48%, TB4 30% vs 6%)
  disagree with the model card's own table (54.2 vs 38.3 and 23.8 vs 6.3). They are
  probably AA v5 numbers from a different harness, so I left them; someone with the AA
  export should confirm.
- `score` 1.0 for ternary-bonsai-2-2b is an ESTIMATE with no AA data, as its
  `score_source` says. Nothing to check against.
- The ternary Q2_0 file was last changed 2026-10-14 and the README 2026-11-04; the
  catalog bytes match the current file. If the catalog was written before 2026-10-14,
  its size would already be stale, but it matches today, so nothing to fix.

## Next steps

- Tighten `scripts/verify_catalog.py` to compare `kv_heads`, `head_dim`, `kv_layers` and
  `ctx_max` for equality and to print the observed values, so overcounts stop passing.
- Decide whether EZLM ships mmproj files. The ternary 2B needs
  `Ternary-Bonsai-2-2B-mmproj-Q8_0.gguf` (0.36 GB) before `vision: true` means anything;
  the same applies to the other 22 vision entries.
- The card's examples pass `-fa on`. llama.cpp defaults flash attention to auto, so no
  catalog change is needed, but a smoke test of the Q2_0 file on the pinned b11960 would
  confirm the Hadamard runtime path.
