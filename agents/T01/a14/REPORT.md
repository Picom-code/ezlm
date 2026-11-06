# T01 / a14: granite-4.2-30b and granite-4.2-8b

Agent a14. Models: `granite-4.2-30b`, `granite-4.2-8b` (the only entries I touched or checked in
`internal/catalog/catalog.json`). Branch `agent/T01-a14`.

## Result

No corrections. Every number in both entries matches the GGUF metadata, the Hugging Face model
card, and the repository file listing. All 13 quant file sets exist with exactly the catalog byte
counts, and every file's `general.file_type` matches its catalog quant name.

## What I checked

1. `scripts/verify_catalog.py --only granite-4.2-30b,granite-4.2-8b --jobs 2` against three
   llama.cpp trees, saved in this folder:
   - `verify-b11960.txt` (the pinned release): 13/13 file sets ok, 0 errors.
   - `verify-latest.txt`: `releases/latest` currently returns tag `v0.9.0` (published
     2026-11-04), 13/13 ok.
   - `verify-b12144.txt`: the newest build tag (published 2026-11-06), which is newer than the
     `v0.9.0` release tag. 13/13 ok.
2. GGUF headers read by hand for all 13 files (`gguf-fields.txt`, `gguf-filetypes.txt`). The
   script only warns when the catalog *undercounts* a value, so equality was checked explicitly:
   `block_count`, `attention.head_count`, `attention.head_count_kv`, `rope.dimension_count`,
   `context_length`, `general.license`, `tokenizer.chat_template`, and `general.file_type` are
   identical in all seven 30B files and all six 8B files.
3. `config.json` and `README.md` of both non-GGUF repos (GET only, no weights).
4. Repository file listing for both GGUF repos (`hf-repo-files.txt`), cross-checked against the
   catalog byte counts.
5. The remote catalog at `https://picom-code.github.io/ezlm/internal/catalog/catalog.json` and
   `https://raw.githubusercontent.com/picom-code/ezlm/main/internal/catalog/catalog.json`: both
   are byte-identical to the embedded catalog for these two models (no divergence to report).

## Verification detail

| Field | Catalog | GGUF / card evidence | Verdict |
|---|---|---|---|
| `layers` | 64 / 40 (`catalog.json:2204`, `:2284`) | `granite.block_count` 64 / 40 in every quant | exact |
| `kv_heads` | 8 / 8 (`:2205`, `:2285`) | `granite.attention.head_count_kv` 8 | exact |
| `head_dim` | 128 / 128 (`:2206`, `:2286`) | no `attention.key_length` key, so llama.cpp uses `embedding_length / attention.head_count` = 4096 / 32 = 128; `rope.dimension_count` 128 agrees | exact |
| `kv_layers` | 64 / 40 (`:2211`, `:2291`) | no `attention.sliding_window`, no `shared_kv_layers`, no `full_attention_interval`: every layer keeps a cache | exact |
| `ctx_max` | 131072 / 131072 (`:2207`, `:2287`) | `granite.context_length` 131072; card "Natively Supports 128K (Long-context extension to 512K)"; `max_position_embeddings` 131072, `rope_scaling` null | exact |
| `params_b` | 29.28 / 8.79 (`:2200`, `:2280`) | derived from `config.json`: hidden 4096, 32 heads, 8 KV heads, FFN 32768 / 12800, vocab 100352, 64 / 40 layers, `tie_word_embeddings: false`. 30B = 29,276,766,208; 8B = 8,791,588,864 | exact |
| `active_params_b` | 29.28 / 8.79 (`:2210`, `:2290`) | dense, no `expert_count` key | exact |
| `moe` | false / false (`:2201`, `:2281`) | no MoE keys in the header | exact |
| `vision` | false / false (`:2212`, `:2292`) | text-generation GGUFs, no vision keys | exact |
| `native_1bit` | absent (false) | bf16-trained (`Precision: bfloat16` on the card) | exact |
| `license` | Apache-2.0 (`:2199`, `:2279`) | card front matter `license: apache-2.0`, card table "License Apache 2.0", `general.license = apache-2.0` | exact |
| `tool_calling` | true (`:2208`, `:2288`) | template contains `<tool_call>`, `<function=` and `<parameter=`; b11960 `common/chat.cpp:1412` selects the Qwen3-Coder tool-call parser on exactly those markers | exact |
| `quants` | 7 / 6 entries | `general.file_type` is 10/12/2/15/8/18/7 and 12/2/15/8/18/7, matching Q2_K, Q3_K_M, Q4_0, Q4_K_M, Q5_0, Q6_K, Q8_0 | exact |
| `bytes` | 13 sizes | HEAD `Content-Length` and the repo tree agree with the catalog to the byte | exact |

`notes` claims spot-checked: release date 2026-08-25 and "128K native, extendable to 512K" are on
both cards, the models are dense GQA transformers (`GraniteForCausalLM`), and the 3B comparison
in the note matches the 3B entry (Q4_K_M 2,244,011,552 B, score 4.7).

## Findings

| # | Where | Finding | Severity |
|---|---|---|---|
| 1 | `internal/catalog/catalog.json:2214-2271`, `:2294-2343` | No error in any of the 13 quant file sets: names, sizes and quant types are all correct. | none (verified) |
| 2 | `internal/catalog/catalog.json:2274-2343` | The 8B catalog lists 6 quants and omits the repo's `granite-4.2-8b-Q2_K.gguf` (3,412,312,192 B), while the 3B and 30B entries do list a Q2_K. A machine with about 4 GB of usable memory therefore has no 8B option. Not a truth error, a coverage choice. | minor |
| 3 | `internal/catalog/catalog.json:2274-2343` | Same for the other unlisted IBM quants: Q3_K_S/L, Q4_1, Q4_K_S, Q5_1, Q5_K_M/S, bf16. Q4_K_S (16.72 GB on the 30B) and Q5_K_M (20.78 GB) are the ones a fit planner would plausibly want between existing steps. | minor |
| 4 | `https://huggingface.co/ibm-granite/granite-4.2-30b/README.md:65` | The 30B card says RoPE theta 10,000,000, but `config.json` and the GGUF say `rope.freq_base = 50000000`. The card line looks copied from the 8B (which really is 10M). The catalog does not store this value, so nothing to change; the GGUF is the one llama.cpp uses. | minor (external doc) |
| 5 | `internal/catalog/catalog.json:2213`, `:2293` | The two `notes` strings are identical and both end with a Granite 4.2 3B comparison. Accurate, but the sentence is noise in the 30B entry. | minor (cosmetic) |
| 6 | `scripts/verify_catalog.py:155-203` | The verifier only flags *undercounts* (`<`), so an overcount such as `layers` 128 for a 64-layer file passes as "ok". I checked equality by hand for my 13 files; a full-catalog audit should tighten the comparison or say so in the report. | minor (tool) |
| 7 | both GGUF repos | `lastModified` is 2026-09-02 for both, before the catalog snapshot (2026-11-06), so no file was replaced after the catalog date. Stretch-goal check passes. | none |

No blocker: both repos and all 13 files exist and resolve.

## Corrections

None. `internal/catalog/catalog.json` is unchanged by this agent.

## Open questions

1. Is the missing 8B Q2_K deliberate? If the catalog wants one quant per memory tier, the 3B
   Q4_K_M (2.24 GB) already covers that tier, which would explain the omission. I did not add it
   because the byte count would have been the only change and the choice is curation, not truth.
2. The 30B `score` is an ESTIMATE of 5.2. `https://artificialanalysis.ai/models/granite-4-2-30b`
   currently reports `intelligenceIndex: null`, so the estimate is still the only number
   available, and the 8B's official 5.098 matches the catalog's 5.1. Worth re-checking when AA
   publishes a 30B index, because the 30B then ranks only 0.1 above the 8B while needing about
   three times the memory.

## Next steps

- When someone next edits the 8B entry, consider adding `granite-4.2-8b-Q2_K.gguf`
  (3,412,312,192 B) so the 8B reaches the 4 GB tier.
- The Lead can fold finding 6 into the shared verifier: compare `layers`, `kv_heads`,
  `head_dim`, `kv_layers` and `ctx_max` for equality, not just for undercounts.

## Files

- `verify-b11960.txt`, `verify-latest.txt`, `verify-b12144.txt`: verifier output.
- `gguf-fields.txt`: the catalog-relevant GGUF keys for all 13 files.
- `gguf-filetypes.txt`: `general.file_type` per file against the catalog quant name.
- `hf-repo-files.txt`: complete GGUF file listing and byte sizes for both repos.
