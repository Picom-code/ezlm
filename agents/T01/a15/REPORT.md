# T01 / a15: qwen3.8-flash-next and qwen3.6-27b

Agent: a15. Branch `agent/T01-a15`. Worktree `~/ezlm-agents/work/subs/T01/a15`.
Models owned: `qwen3.8-flash-next`, `qwen3.6-27b`. Extra assignment: one Go test,
`internal/catalog/consistency_test.go`.

## What I checked

- `scripts/verify_catalog.py --tag b11960 --only qwen3.8-flash-next,qwen3.6-27b`
  (`verify-b11960.txt`, before the fixes; `verify-b11960-after.txt`, after).
- The newest llama.cpp release. `releases/latest` returns `v0.9.0`
  (published 2026-11-04), but the newest release of all is build `b12144`
  (2026-11-06). I ran both: `verify-latest-after.txt` is `b12144`,
  `verify-v0.9.0.txt` is the `releases/latest` tag.
- GGUF metadata by hand, from the first file of every quant set (range requests,
  no downloads): architecture, `block_count`, `nextn_predict_layers`,
  `attention.head_count_kv`, `attention.key_length`, `full_attention_interval`,
  `context_length`, chat template, and the tensor list of the split parts.
- Model cards on Hugging Face (`Qwen/Qwen3.8-Flash-Next`, `Qwen/Qwen3.6-27B`,
  `unsloth/*-GGUF`), the LICENSE file, and the HF API for parameter counts and
  repo timestamps.
- The draft file for `qwen3.8-flash-next` (`ggml-org/Qwen3.8-Flash-Next-GGUF`).
- Step 5 of TASKS T01, for my two models: diff of the published remote catalog
  against the embedded one (see "Remote catalog" below).

## Findings

| # | Where | Finding | Evidence | Severity |
|---|---|---|---|---|
| 1 | `internal/catalog/catalog.json:440` (UD-IQ1_S) | Size was 72,546,461,344; the files now total 66,379,180,698 (-6.17 GB) | HEAD of the 3 parts: 10,946,618 + 37,568,095,648 + 28,800,138,432. The repo was re-uploaded 2026-11-04 ("Better UD 3.1 quants", commit 28361fa7c28a) after the catalog was written | **blocker** (the planner over-reserved 6.2 GB; on a 64 GB machine this decides between fitting and swapping) |
| 2 | `internal/catalog/catalog.json:452` (UD-IQ1_M) | Size was 74,538,755,776; the files now total 68,007,094,938 (-6.53 GB) | HEAD of the 3 parts: 10,946,618 + 39,196,009,888 + 28,800,138,432 | **blocker** (same) |
| 3 | `internal/catalog/catalog.json:406,418,430,442,454` | `lazy_bytes` was `floor(bytes * 0.34)` on all five quants, not the size of the n-gram table | The n-gram table is one tensor, `per_layer_token_embd.weight [160, 320001536]`, IQ4_NL (18/32 B), alone in its own split part: 320,001,536 x 160 x 18/32 = 28,800,138,240 B, part file 28,800,138,432 B. Old values 22.58-25.79 GB were 34% of the (stale) totals | **major** (under-reserved the resident set by up to 6.2 GB, or would have after fix 1) |
| 4 | `internal/catalog/catalog.json:376` | `"qwen-community-1.0 (UNVERIFIED terms)"`; the terms are now read | `Qwen/Qwen3.8-Flash-Next` card: `license: other`, `license_name: qwen-community-1.0`; LICENSE: attribution needed above 100M MAU or US$20M monthly revenue, and a separate licence for Model-as-a-Service | minor |
| 5 | `internal/catalog/catalog.json:390` | `notes` ended mid-sentence: `... merged 2026-08-27 (PR #27742` | PR #27742 "model: add Qwen3.8-Flash-Next (qwen4exp)" is merged 2026-08-27T19:32:31Z; the text just lost its closing paren | minor |

No blocker for `qwen3.6-27b`: every field I could check is correct.

## Corrections (all inside my two model objects, formatting unchanged)

1. `qwen3.8-flash-next` UD-IQ1_S `bytes` 72,546,461,344 -> **66,379,180,698**.
   Sum of the three parts as served now (`Content-Length` of each part; the parts
   are still `-00001-of-00003`..`-00003-of-00003`).
2. `qwen3.8-flash-next` UD-IQ1_M `bytes` 74,538,755,776 -> **68,007,094,938**.
   Same check, same three-part layout.
3. `qwen3.8-flash-next` `lazy_bytes` -> **28,800,138,432** on all five quants.
   Proof: split part 3 of each unsloth quant (part 2 of each ISTA-DASLab quant)
   holds exactly one tensor, `per_layer_token_embd.weight`, dims
   `[160, 320001536]`, type IQ4_NL, and the part file is 28,800,138,432 B. That
   tensor is the 51B n-gram table, the only part that `--lazy-mode` can leave on
   disk, and its size is now identical for every quant (the "mix n match engram"
   upload of 2026-11-04 replaced the per-quant table with one shared IQ4_NL
   table). The old values were 34% of the total and differ per quant.
4. `qwen3.8-flash-next` `license` -> `qwen-community-1.0` (terms read; see
   finding 4). The name matches the model card, so only the stale caveat went.
5. `qwen3.8-flash-next` `notes` -> closing paren after `(PR #27742)`.

## Verified correct, no change needed

`qwen3.8-flash-next`: `layers` 48 (`qwen4exp.block_count` 48, no nextn layer),
`kv_heads` 2 (`attention.head_count_kv` 2), `head_dim` 256
(`attention.key_length` 256), `kv_layers` 12 (`full_attention_interval` 4 ->
layers 4, 8, ... 48 are full attention), `ctx_max` 262144
(`context_length` 262144; the card also says 262,144 native, 1M with YaRN),
`params_b` 180.0 (HF safetensors 179,999,981,459 = 125B + 51B n-gram + 4B MTP),
`active_params_b` 6.0 (card: "125B with 6B activated"), `moe` true (512 experts,
top-10 + 1 shared), `vision` true (card: vision encoder; repo has
`mmproj-BF16/F16.gguf`), `tool_calling` true (chat template has `<tools>`, `<tool_call>` and
`<tool_response>`), draft
`mtp-Qwen3.8-Flash-Next-Q4_0.gguf` 2,199,652,672 B exists in
`ggml-org/Qwen3.8-Flash-Next-GGUF` with exactly that size, `native_1bit` absent.
The three ISTA-DASLab GSQ-RCO quants match their part sizes exactly.

`qwen3.6-27b`: `layers` 64 (`qwen35.block_count` 65 minus
`nextn_predict_layers` 1), `kv_heads` 4, `head_dim` 256, `kv_layers` 16
(`full_attention_interval` 4 over 64 layers), `ctx_max` 262144, `params_b`
27.78 (HF safetensors 27,781,427,952), `active_params_b` 27.78 (dense),
`moe` false (no `expert_count`, dense `feed_forward_length` 17408), `vision`
true (card: vision encoder; repo has three `mmproj-*.gguf`), `tool_calling`
true (template and card), `license` Apache-2.0 (card `license: apache-2.0`,
GGUF `general.license = apache-2.0`), all seven quants exist in
`unsloth/Qwen3.6-27B-MTP-GGUF` at the listed size, and every one of them is an
MTP file (`block_count` 65 + `nextn_predict_layers` 1), so `mtp: true` is right
for all seven.

## Go test: `internal/catalog/consistency_test.go`

Loads the embedded catalog with the package's own `parse(embedded)` (no network,
no files, no temp dirs) and checks:

- model ids non-empty and unique;
- ranks: the loaded order is the rank order (score non-increasing) and ranks are
  unique and contiguous 1..len. Ties in `score` are allowed; `sortModels` is
  stable, so the file order breaks them. There is no `rank` field in the
  catalog, so a rank is the model's position in the ranked list that `Load`
  returns;
- quants: `bytes` > 0, at least one file, and sizes strictly increasing inside
  each repo group (the catalog lists quants cheapest first per repo; a strict
  order is what makes "the next smaller quant" unambiguous);
- split files: a one-file quant has no `-NNNNN-of-NNNNN` suffix; a multi-file
  quant has every part named that way, numbered 1..n in order, with the same
  prefix and a total that matches the list length;
- URLs: every `FileURL` and `Draft.URL` is `https` on `huggingface.co`, and
  every repo is `owner/name` (not empty, no `://`);
- planner fields: `ctx_max` > 0, `layers` > 0, `kv_heads` > 0, `head_dim` > 0.

It passes on this branch. I mutation-tested each check by breaking the catalog
on purpose (duplicate size, wrong split number, wrong part count, empty id,
duplicate id, `ctx_max: 0`, `layers: 0`, repo as a URL) and each one made the
test fail.

## Remote catalog (TASKS step 5)

`https://picom-code.github.io/ezlm/internal/catalog/catalog.json` is byte-equal
to the embedded copy before my fixes: same 32 ids, same order, version 8, and
the only differences now are the five lines I changed. So the published copy
still carries the two stale sizes and the 34% `lazy_bytes`, and because `Load`
only takes a remote catalog with `Version > embedded.Version`, the stale copy
will win as soon as someone bumps the version. Republish it together with the
merge, or the fix does not reach users.

## Open questions

- `qwen3.6-27b` notes say "Released 2026-04-15"; the HF repo was created
  2026-04-21. The 15th may be the announcement date. I did not change it.
- `qwen3.8-flash-next` has more quants in the repo than the catalog lists
  (UD-IQ2_XS, UD-IQ3_XXS, UD-IQ4_XS, UD-Q2_0, UD-Q2_K_XL, UD-Q3_K_XL,
  UD-Q4_K_XL, UD-Q5_K_XL, UD-Q6_K_XL, Q8_0). Adding quants is not a correction
  and the brief says corrections only, so I left them out.
- `attention.compress_ratios` (QSA, 4 for the full-attention layers) is not
  modelled. The catalog's `kv_heads`/`head_dim` therefore over-count the QSA
  cache, which is the safe direction.
- The `qwen3.6-27b` speed note ("RTX 3090 + MTP ~60 t/s code (club-3090)") is
  not something I can verify without a GPU.

## Next steps

- Republish the remote catalog after the merge (see above).
- If the Lead wants it, add the missing flash-next quants from the unsloth repo.
- Run `verify_catalog.py` for flash-next again after any future unsloth upload:
  this repo was re-sharded twice in October/November, so its sizes move.

## Gate

`gofmt -l .` empty, `go build ./...`, `go vet ./internal/catalog/`,
`go test -p 2 ./internal/catalog/ ./internal/fit/` pass.
