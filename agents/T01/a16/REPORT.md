# T01 / a16: qwen3.5-122b-a10b and granite-4.2-3b

Worktree `~/ezlm-agents/work/subs/T01/a16`, branch `agent/T01-a16`, based on
`5213370` (Baseline: EZLM source snapshot 2026-11-06).

## Result

**No catalog corrections were needed.** Both model entries match the GGUF
headers, the Hugging Face model cards and configs, and the pinned b11960 build.
They also pass against the current llama.cpp release (`v0.9.0`).

- 13 quant file sets checked (5 for Qwen, 8 for Granite): every file exists,
  every byte size is exact, every split file name follows the
  `<prefix>-0000i-of-0000N.gguf` pattern, and every tensor type and
  architecture is known to both builds.
- `internal/catalog/catalog.json` is unchanged. The only edits on the branch
  are `scripts/verify_catalog.py` (CI stretch, see `verify-ci-notes.md`) and
  the files under `agents/T01/a16/`.

## What I ran

| # | Command | Result |
|---|---|---|
| 1 | `python3 scripts/verify_catalog.py --tag b11960 --jobs 2 --only qwen3.5-122b-a10b,granite-4.2-3b` | 166 architectures, 44 tensor types, 13 file sets, 0 errors, exit 0 (`verify-b11960.txt`) |
| 2 | `python3 scripts/verify_catalog.py --tag v0.9.0 --jobs 2 --only qwen3.5-122b-a10b,granite-4.2-3b` | 170 architectures, 45 tensor types, 13 file sets, 0 errors, exit 0 (`verify-latest.txt`) |
| 3 | GGUF header read of the first file of all 5 Qwen quants | `qwen35moe`, `block_count` 49, `nextn_predict_layers` 1, `split.count` 3 on both split quants |
| 4 | `tokenizer.chat_template` read from one GGUF per model | both templates implement `<tools>` / `<tool_call><function=...>` |
| 5 | Hugging Face API `safetensors.total` for both repos | Qwen 125,086,497,008; Granite 3,659,737,600 |
| 6 | `HEAD` on all 13 files | sizes match the catalog; ETags saved in `file-headers.txt` |
| 7 | Go gate: `gofmt -l .`, `go build ./...`, `go vet ./internal/catalog/`, `go test -p 2 ./internal/catalog/` | clean |

`v0.9.0` is `releases/latest` on 2026-11-06 (published 2026-11-04). The
bNNNN tag scheme has moved to semantic versions; the verifier reads the
sources of either tag the same way.

## qwen3.5-122b-a10b

Catalog object: `internal/catalog/catalog.json:678-749`.

| Field | Catalog | Evidence | Verdict |
|---|---|---|---|
| `license` (`:683`) | `Apache-2.0` | card `README.md:3` `license: apache-2.0`; `license_link` points at the Apache licence | ok |
| `params_b` (`:684`) | 125.09 | HF API `safetensors.total` = 125,086,497,008, which includes the vision tower, same convention as `qwen3.5-9b` (9.65 vs 9,653,104,368) | ok |
| `moe` (`:685`) | true | GGUF `expert_count` 256, `expert_used_count` 8; card "8 Routed + 1 Shared" | ok |
| `layers` (`:688`) | 48 | GGUF `qwen35moe.block_count` 49 minus `nextn_predict_layers` 1; card `:49` "Number of Layers: 48" | ok |
| `kv_heads` (`:689`) | 2 | GGUF `qwen35moe.attention.head_count_kv` 2; card `:55` "32 for Q and 2 for KV" | ok |
| `head_dim` (`:690`) | 256 | GGUF `qwen35moe.attention.key_length` 256 and `value_length` 256; card `:56` | ok |
| `kv_layers` (`:695`) | 12 | GGUF `qwen35moe.full_attention_interval` 4 over 48 layers; `config.json` `layer_types` has `full_attention` at layers 4, 8, ..., 48 | ok |
| `ctx_max` (`:691`) | 262144 | GGUF `qwen35moe.context_length` 262144; card `:64`; `config.json` `max_position_embeddings` 262144 | ok |
| `tool_calling` (`:692`) | true | `tokenizer.chat_template` renders a `<tools>` block and `<tool_call>\n<function=...>`; the same qwen3_coder format the catalog's Granite note cites | ok |
| `vision` (`:696`) | true | card `:5` `pipeline_tag: image-text-to-text`; `config.json` has a `vision_config` | ok (see finding 1) |
| `active_params_b` (`:694`) | 10.0 | card "122B in total and 10B activated" | ok |
| `repo` (`:693`) | `Qwen/Qwen3.5-122B-A10B` | repo exists; `config.json` `architectures` is `Qwen3_5MoeForConditionalGeneration` | ok |
| 5 quants (`:698-748`) | see file | every file present, sizes exact, all 5 carry `nextn_predict_layers` 1 so `mtp: true` is right, both split sets have `split.count` 3 with valid part names | ok |

## granite-4.2-3b

Catalog object: `internal/catalog/catalog.json:2541-2626`.

| Field | Catalog | Evidence | Verdict |
|---|---|---|---|
| `license` (`:2546`) | `Apache-2.0` | card `:33`; HF API tag `license:apache-2.0` | ok |
| `params_b` / `active_params_b` (`:2547-2548`) | 3.66 / 3.66 | HF API `safetensors.total` = 3,659,737,600; dense, so active equals total | ok |
| `moe` (`:2549`) | false | `GraniteForCausalLM`, dense MLP; card "Decoder-only Dense Transformer" | ok |
| `layers` (`:2552`) | 40 | GGUF `granite.block_count` 40; card `:87`; `config.json` `num_hidden_layers` 40 | ok |
| `kv_heads` (`:2553`) | 8 | GGUF `granite.attention.head_count_kv` 8; card `:105`; `config.json` `num_key_value_heads` 8 | ok |
| `head_dim` (`:2554`) | 64 | GGUF has no `attention.key_length`; `embedding_length` 2560 / `attention.head_count` 40 = 64; card `:93` "Attention head size 64" | ok |
| `kv_layers` (`:2555`) | 40 | dense, all 40 layers have KV; GGUF `head_count_kv` is a scalar | ok |
| `ctx_max` (`:2556`) | 131072 | GGUF `granite.context_length` 131072; `config.json` `max_position_embeddings` 131072; card `:28` "Natively Supports 128K" | ok |
| `tool_calling` (`:2557`) | true | `tokenizer.chat_template` renders `<tools>` and `<tool_call><function=...>` (qwen3_coder style, matching the card's parser note and the catalog `notes`) | ok |
| `vision` (`:2558`) | false | card `pipeline_tag: text-generation`, no vision config | ok |
| `repo` (`:2559`) | `ibm-granite/granite-4.2-3b` | repo exists; HF search shows no `unsloth/granite-4.2-3b-GGUF`, so the note "No unsloth repo exists" holds | ok |
| 8 quants (`:2561-2625`) | see file | every file present, sizes exact; `bartowski/granite-4.2-3b-GGUF` for IQ4_XS, IBM's repo for the rest | ok |

The `notes` on both entries check out: release dates, the 512K long-context
extension, the `qwen3_coder` parser, and b11960 support for the `granite`
architecture.

## Findings

| # | Severity | Location | Finding | Evidence |
|---|---|---|---|---|
| 1 | minor | `internal/catalog/catalog.go:71` | The catalog `vision` field has no reader in the Go code today. Vision in an agent config comes from llama-server `/props` (`internal/serve/props.go:56`), not from the catalog, so `vision: true` on `catalog.json:696` is informational only. No catalog change made; it is correct for the model and affects 23 entries. | `grep -rn '\.Vision\b' internal cmd` shows only `app/connect.go:21`, `integrate/dsh_config.go:102`, `integrate/opencode_config.go:70`, `serve/props.go:56`, none of which read `catalog.Model` |
| 2 | minor | `scripts/verify_catalog.py:184-191` | The layer check subtracts `nextn_predict_layers`, so a quant that lost its MTP heads would still pass while the catalog says `mtp: true`. I checked all 5 Qwen files by hand (all `nextn_predict_layers` 1), but the script should assert it. Not changed here to keep this branch's script diff to the CI stretch. | header reads of all 5 Qwen quants, output in this report |
| 3 | minor | `agents/T01/a16/file-headers.txt` | The catalog stores no ETag or upload date, so "file replaced after the catalog date" cannot be detected from the repo alone. I recorded the current ETags for the 13 files so a later run can compare. | `HEAD` responses, all 13 files |
| 4 | minor | `internal/catalog/catalog.json:684` | `params_b` counts the vision tower (125.09 vs 122B text). Consistent with every other entry, and it errs on the safe side for memory planning, so no change. | HF `safetensors.total` vs card "122B in total" |

No blocker findings. No model repo is gone and no listed file is missing; a
nonexistent HF repo answers 401 rather than 404, which the verifier treats as a
mismatch (exit 1), not a network error.

## Corrections made

None. `git diff main -- internal/catalog/catalog.json` is empty.

## Open questions

1. Should the catalog keep `vision` at all, or should the planner read the
   server's `/props` only? It is dead metadata today.
2. Should `params_b` be text-only parameters (about 122B here) or the whole
   checkpoint (125.09B)? The planner uses it for tier bucketing and for the MoE
   active fraction, so the current convention is conservative.
3. The `(MTP)` suffix in the quant names is display-only. It matches the
   naming used by the other Qwen MTP entry in the catalog.

## Next steps

- Add a `mtp` consistency check to `scripts/verify_catalog.py` (finding 2).
- Store a catalog date or per-file ETag baseline so replaced files can be
  flagged (finding 3).
- Decide whether `vision` should be wired to the catalog or dropped
  (finding 1).
