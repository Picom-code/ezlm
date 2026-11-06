# T01 / a08: qwen3.5-4b and qwen3.5-2b

Scope: the two model objects `qwen3.5-4b` (`internal/catalog/catalog.json:848`) and
`qwen3.5-2b` (`internal/catalog/catalog.json:936`). Nothing else was edited.
Branch `agent/T01-a08`, worktree `~/ezlm-agents/work/subs/T01/a08`.

Result: both entries are numerically correct. One note text was wrong and is fixed.
No blocker, no missing repo, no missing file.

## What I checked, and how

1. `scripts/verify_catalog.py --tag b11960 --jobs 2 --only qwen3.5-4b,qwen3.5-2b`
   (`verify-b11960.txt`): 16 file sets, 0 errors. Each file exists, its
   Content-Length equals the catalog `bytes`, the `qwen35` architecture and every
   tensor type in it are known to b11960, there is one file per quant so split
   completeness is trivial, the block/KV-head/head-size/context numbers match the
   header, and each file carries a chat template.
2. Same against the newest release, `v0.9.0` (`verify-latest.txt`): 170
   architectures, 45 tensor types, 0 errors.
3. Byte sizes re-checked a second way, against the Hugging Face tree API LFS sizes
   (`/api/models/<repo>/tree/main`), all 16 files exact.
4. GGUF metadata read directly by range request (`gguf-meta.txt`), then cross-read
   against the model `config.json` and the model card on the Hub.
5. llama.cpp b11960 source: `src/models/qwen35.cpp` marks the linear-attention
   (Gated DeltaNet) layers recurrent via `qwen35.full_attention_interval` and
   creates Q/K/V cache tensors only for the full-attention layers, so the KV
   cache really is `block_count / 4` layers deep.
6. Licence and tool calling from the model cards and the embedded template.
7. The remote catalog `https://picom-code.github.io/ezlm/internal/catalog/catalog.json`
   (TASKS step 5): identical to the embedded one for both of my models, field by
   field and quant by quant.

## Field-by-field, with evidence

| Field | Catalog | Evidence | Verdict |
|---|---|---|---|
| `layers` 4b / 2b | 32 / 24 | `qwen35.block_count` 32 / 24; card "Number of Layers"; `config.json` `num_hidden_layers` | ok |
| `kv_heads` 4b / 2b | 4 / 2 | `qwen35.attention.head_count_kv` 4 / 2; card "16 for Q and 4 for KV" / "8 for Q and 2 for KV" | ok |
| `head_dim` | 256 / 256 | `qwen35.attention.key_length` 256, `value_length` 256; card "Head Dimension: 256" | ok |
| `kv_layers` 4b / 2b | 8 / 6 | `qwen35.full_attention_interval` 4 and 32 / 24 blocks; `qwen35.cpp:17-22` marks the other layers recurrent; card layout "8 x (3 x DeltaNet + 1 x Gated Attention)" / "6 x (...)" | ok |
| `ctx_max` | 262144 / 262144 | `qwen35.context_length` 262144; card "Context Length: 262,144 natively"; `max_position_embeddings` 262144 | ok |
| `params_b` 4b / 2b | 4.66 / 2.27 | Hub `safetensors.total` 4,659,865,088 / 2,274,069,824 | ok (see note 4) |
| `active_params_b` | 4.66 / 2.27 | dense, no expert keys in the header | ok |
| `moe` | false / false | no `*.expert_count` in the header; card has no MoE section | ok |
| `license` | Apache-2.0 | card front matter `license: apache-2.0`; `general.license` `apache-2.0` | ok |
| `tool_calling` | true / true | `tokenizer.chat_template` (7816 chars, identical in both files) renders `<tools>`, `<tool_call>`, `<tool_response>`; card has a Tool Calling table | ok |
| `vision` | true / true | card "Causal Language Model with Vision Encoder"; GGUF tags `image-text-to-text`; unsloth ships `mmproj-F16/BF16/F32.gguf` (see note 3) | ok |
| `native_1bit` | absent | not a 1-bit model | ok |
| `mtp` on quants | absent | the non-MTP files have no `qwen35.nextn_predict_layers` | ok (see note 2) |
| `bytes`, all 8 quants each | exact | HEAD Content-Length and Hub LFS size, all 16 equal | ok |
| `repo`, `quants[].repo` | `Qwen/Qwen3.5-4B`, `Qwen/Qwen3.5-2B`, `unsloth/Qwen3.5-*B-GGUF` | all resolve, none gated; every listed file present | ok |

## Findings

| # | Severity | file:line | Evidence | Action |
|---|---|---|---|---|
| 1 | minor | `internal/catalog/catalog.json:867`, `:955` | Note claimed "mmproj ~0.67-0.92GB". The 0.92GB figure is the 9B's projector: `unsloth/Qwen3.5-9B-GGUF/mmproj-F16.gguf` is 918,166,080 B. For these two the F16 projector is 672,423,616 B (4B) and 668,227,264 B (2B), BF16 675,569,344 / 671,372,992 B, and F32 1,334,075,072 / 1,325,684,416 B. | Fixed: note now says "mmproj F16 ~0.67GB (F32 ~1.33GB)". |
| 2 | minor | `internal/catalog/catalog.json:868-933`, `:956-1021` | The 9B entry carries an MTP quant (`:836-844`, `mtp: true`, `unsloth/Qwen3.5-9B-MTP-GGUF`). The same repos exist for these two sizes and their `UD-Q4_K_XL` files really do carry the extra NextN block: `Qwen3.5-4B-UD-Q4_K_XL.gguf` 2,990,664,000 B, `block_count` 33, `nextn_predict_layers` 1; `Qwen3.5-2B-UD-Q4_K_XL.gguf` 1,384,902,048 B, `block_count` 25, `nextn_predict_layers` 1. | Not changed: it is an omission, not a wrong value, and adding a quant changes what EZLM downloads. Open question for the Lead. |
| 3 | minor | `internal/catalog/catalog.json:866`, `:954` | `vision: true` is defensible (the checkpoint has a vision encoder) but EZLM never loads a projector: no entry in the catalog lists an `mmproj` file, and `internal/integrate/types.go:21` says the projector is loaded "never, today". The field also has no reader in Go (`grep Vision` finds only `catalog.go:71` and `internal/serve/props.go`, which is fed by llama.cpp, not by the catalog). | Not changed: the flag is inert today and consistent with the other 21 entries. |
| 4 | minor | `internal/catalog/catalog.json:854`, `:942` | `params_b` follows the Hub total, which includes the vision tower. The text-only weights in the deployed GGUF are 4,205,751,296 params (4B, 426 tensors) and 1,881,825,088 (2B, 320 tensors), so 0.45B / 0.39B of the listed figure never lands in the file. | Not changed: `internal/fit/fit.go:157-163` only uses `params_b` for the quality tiers, and the sibling 9B entry uses the same convention (9.65). |

No correction was needed to `layers`, `kv_heads`, `head_dim`, `kv_layers`,
`ctx_max`, `params_b`, `active_params_b`, `moe`, `license`, `tool_calling`,
`vision` or any `bytes` value.

## Open questions

- **Recurrent state is not in the memory plan.** For a hybrid DeltaNet model the
  KV cache is only 8 (4B) / 6 (2B) layers, but llama.cpp also allocates a
  per-sequence Gated DeltaNet state: `ssm.inner_size * ssm.state_size` floats per
  linear layer (`4096*128*4 B = 2 MiB` for the 4B, `2048*128*4 B = 1 MiB` for the
  2B) plus the conv state, so roughly 50 MiB (4B) and 19 MiB (2B) per slot that
  `catalog.KVBytesPerToken` cannot express. Small, but it is real and the catalog
  has no field for it. Not mine to fix; flagging for whoever owns `internal/fit/`.
- **MTP quants** for 4B/2B (finding 2). Sizes are in the table if the Lead wants
  them added.
- **Scores** (`2.0`, `1.0`) are estimates per `score_source` and cannot be checked
  from a GGUF. They are at least monotone with the 9B's 5.0 and in line with the
  other 2B/4B-class entries (`ternary-bonsai-2-4b` 2.0, `ternary-bonsai-2-2b` 1.0).

## Next steps

- Decide on the MTP quants; if yes, they are a three-line addition per model plus
  a `--only` verifier run (the files above already pass the b11960 arch check).
- Consider a catalog field or a fit-side constant for hybrid recurrent state, or
  document that it is inside `overhead()`.
- Nothing else in this slice needs follow-up.

## Gate run

`gofmt -l .` empty, `go build ./...`, `go vet ./internal/catalog/`,
`go test -p 2 ./internal/catalog/`: all clean. No Go files were changed, so the
gate is unchanged by this slice.
