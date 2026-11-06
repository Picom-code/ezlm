# T01 / a12: catalog truth audit for `devstral-small-2-24b` and `nemotron-3.5-super-vl-120b-a12b`

Agent a12, branch `agent/T01-a12`. Two models only: `devstral-small-2-24b`
(catalog lines 1852-1923) and `nemotron-3.5-super-vl-120b-a12b` (catalog lines
1924-2018). Baseline commit `5213370` (EZLM source snapshot 2026-11-06).

## What I checked and how

- `scripts/verify_catalog.py --tag b11960 --jobs 2 --only <both ids>`:
  13 quant sets, 0 errors, 0 warnings (`verify-b11960.txt`).
- Same against the tag from `/releases/latest` (`v0.9.0`, 2026-11-04):
  0 errors (`verify-latest.txt`). The newest release by publish date is
  `b12144` (2026-11-06T03:45Z); that run is also clean (`verify-b12144.txt`).
  `/releases/latest` returns `v0.9.0`, not `b12144`, so both are recorded.
- Hand-read every field the script does not check, from the GGUF headers of
  the first file of each quant (`agents/T01/a12/dump_gguf.py`), from
  `config.json` / model cards on Hugging Face (GET only), and from the
  llama.cpp `b11960` source where the question is what llama.cpp allocates.
- Exact byte sizes and split-file completeness: HEAD on every file; totals
  match the catalog for all 13 sets, including all split parts.
- ETag snapshot for every file of both models (`etags.txt`) so a later run
  can tell whether a file was replaced after the catalog was written.

## Verified correct (no change)

| Field | `devstral-small-2-24b` | `nemotron-3.5-super-vl-120b-a12b` |
|---|---|---|
| `layers` | 40 = GGUF `mistral3.block_count` | 88 = GGUF `nemotron_h_moe.block_count` minus 1 MTP layer (config `num_hidden_layers` 88) |
| `kv_heads` | 8 = `mistral3.attention.head_count_kv` | 2 = `attention.head_count_kv` on the 8 attention layers |
| `head_dim` | 128 = `attention.key_length` | 128 = `attention.key_length` |
| `kv_layers` | 40, all layers full attention (config `sliding_window: null`) | 8, at block indices 7,16,25,36,47,58,69,78 |
| `ctx_max` | 393216 = `mistral3.context_length` and config `max_position_embeddings` | 1048576 = `context_length` and config `max_position_embeddings` |
| `params_b` | 24.0, HF safetensors total 24,011,361,280 | 124.45, HF safetensors total 124,453,395,976 |
| `active_params_b` | 24.0, dense | 12.0, card "120B total / 12B active" |
| `moe` | false | true, 40 MoE + 40 Mamba-2 + 8 attention layers |
| `vision` | true, `Mistral3ForConditionalGeneration` + pixtral `vision_config` | true, C-RADIO encoder, `image-text-to-text` |
| `tool_calling` | true, template has `[AVAILABLE_TOOLS]` / `[TOOL_CALLS]` / `[ARGS]` / `[TOOL_RESULTS]` | true, template has `<tools>` / `<tool_call>` / `<function=...>` (Qwen3-Coder format) |
| `license` | Apache-2.0 (card and GGUF `general.license`) | OpenMDW-1.1 (card, `general.license.name`) |
| `draft` | none | `mtp-...-Q4_0.gguf` exists, 2,270,017,280 bytes = catalog |
| quants | all 6 files exist, sizes exact, arch and tensor types known to b11960 | all 7 sets exist, sizes exact, split parts complete and correctly named |
| notes | "40 layers x 8 x 128" matches the GGUF | "40 Mamba + 40 MoE (512 experts, top-22) + 8 attention" matches the GGUF arrays exactly |

`general.size_label`, `general.license`, `split.count` and the per-layer
`feed_forward_length` / `attention.head_count_kv` arrays all agree with the
catalog. No missing file, no 404, no unknown architecture or tensor type at
`b11960` or `v0.9.0` / `b12144`.

## Findings

### F1 `catalog.json:1953-2004` (major, fixed): five Nemotron quants embed the MTP head but had no `mtp` flag

Evidence: the unsloth split files of `UD-IQ1_M`, `UD-IQ2_XXS`, `UD-Q2_K_XL`,
`UD-IQ3_XXS` and `UD-Q3_K_XL` all report `nemotron_h_moe.block_count = 89`
and `nemotron_h_moe.nextn_predict_layers = 1`, and their
`attention.head_count_kv` array has 89 entries with 2 heads at index 88 (the
MTP block). Their shards hold 781 tensors (`split.tensors.count`), against
763 in the single-file `Q4_0` from ggml-org, which reports `block_count = 88`
and no `nextn_predict_layers` key. So the MTP head is inside the unsloth
files and outside the ggml-org files.

Effect: `internal/fit/fit.go:505` uses `q.MTP` before `m.Draft`. Without the
flag, a CUDA plan for those five quants downloads and loads the separate
2,270,017,280-byte draft file although the head is already in the model file.

Fix: added `"mtp": true` to those five quant objects (after `repo`, the order
used at `catalog.json:568`). The ggml-org `Q4_0` and `NVFP4` quants keep the
model-level draft, because their files really do not carry the head.

### F2 `catalog.json:1942` (minor, not fixed: schema limit): `kv_layers` undercounts the MTP layer for the five 89-block quants

Evidence: llama.cpp allocates the KV cache for every layer below
`n_layer_all` whose `has_kv` is true (`src/llama-kv-cache.cpp:164-165`, and
`has_kv` returns true by default, `src/llama-hparams.cpp:321-332`).
`n_layer_all` is `block_count` (89), so the MTP layer at index 88 gets a K and
a V cache of 2 heads x 128 dims each. That is 1024 bytes per token more than
the catalog's 8 layers (8192 -> 9216 bytes/token): 128 MiB at 128K context,
1 GiB at the full 1M. The two ggml-org quants (88 blocks) are exact.

`kv_layers` is a model-level field and the seven quants differ, so a fix
needs either per-quant KV fields or a decision to set 9 and overcount the two
ggml-org files. `scripts/verify_catalog.py:162,194` subtracts `nextn` from
the attention-layer count, which hides this for every MTP model in the
catalog, not just mine.

### F3 `catalog.json:1866` (minor, open question): Devstral `ctx_max` is 384K in the file, 256K on the card

Evidence: `mistral3.context_length = 393216` and config
`max_position_embeddings = 393216` (yarn factor 48 x 8192) agree with the
catalog, so I did not change it. The model card says "A 256k context window"
(README.md of `mistralai/Devstral-Small-2-24B-Instruct-2512`). Either the
card is stale or the yarn extension is not backed by training. Low impact:
this entry needs 163,840 bytes of KV per token, so 384K context already
costs 60 GiB of f16 cache and the planner rarely reaches it.

### F4 `catalog.json:1871,1943` (major, cross-cutting, outside my patch): `vision: true` while no projector is ever loaded

Evidence: the Devstral GGUF is text only (363 tensors, no `v.*` / `mm.*`
tensors); the Nemotron projector is a separate 1,733,696,768-byte
`mmproj-...-BF16.gguf`. The catalog has no mmproj field, and
`internal/integrate/types.go:21` says "an image projector is loaded (never,
today)". So `/props` reports vision for both models while llama-server has no
projector, and an image request will fail. Affects every `vision: true` entry,
not just mine.

### F5 (observation, no action): unsloth splits put all tensors in parts 2..N

Part 1 of each unsloth split is metadata only (7,874,592 bytes for
`UD-IQ1_M`, `n_tensors = 0` in the header; `split.tensors.count = 781`).
llama.cpp accepts this because it collects tensors from every shard and then
checks the total against `split.tensors.count`
(`src/llama-model-loader.cpp:660-675`), and the byte totals in the catalog are
exact. Worth knowing for any tool that assumes part 1 holds tensors: the
verifier's tensor-type check reads zero types from part 1 but does check
parts 2..N, so nothing was missed here.

### F6 (verified, no action): `notes` details

"LiLiCorr NVFP4 correction file (1.35GB)" is 1,353,710,368 bytes in
ggml-org (1,290,795,104 in unsloth). "Released 2026-10-12" matches the HF
repo creation time (`createdAt 2026-10-12T15:32:06Z`). Devstral "Older
(2025-12)" matches repo creation 2025-11-28 and `general.version = 2512`.

## Corrections made

1. `internal/catalog/catalog.json`: `"mtp": true` added to the five unsloth
   Nemotron quants. Justification per quant: GGUF
   `nemotron_h_moe.nextn_predict_layers = 1` and `block_count = 89` (the
   head is in the file). Formatting untouched otherwise; field placed after
   `repo`, matching `catalog.json:568`.

No other value changed. Both models were otherwise exact against the GGUF
headers, `config.json`, the model cards and llama.cpp `b11960`.

## Open questions

1. Devstral `ctx_max`: 393216 (GGUF/config) or 262144 (card)? I kept the
   file value, per the task rule that `ctx_max` follows the GGUF.
2. Should `kv_layers` include the MTP layer? For these five quants llama.cpp
   allocates one more attention layer than the catalog says (F2). This affects
   every MTP model in the catalog and the verifier's `- nextn` logic.
3. Why is the Nemotron draft `Q4_0` (2.27GB) when the other `draft-mtp`
   entries use `Q8_0`? ggml-org publishes BF16, Q8_0 and Q4_0 MTP heads. A
   Q4_0 draft head may lower acceptance; I did not change it because it is a
   tuning choice, not a wrong number.
4. `score` / `score_source` were not re-checked: `devstral` 4.0 is a hand
   estimate and `nemotron` 13.6 cites the AA v5 JSON. I had no reliable way
   to fetch that JSON (GET only), so both are unverified by me.

## Next steps

- Add an mmproj field (repo, file, bytes) to the schema and use it when
  `vision` is true; otherwise stop advertising vision (F4).
- Decide the MTP KV question centrally (F2) and fix
  `scripts/verify_catalog.py:162,194` if the MTP layer really allocates.
- Quant coverage gaps, not errors: Devstral has no 2-bit entry although the
  repo has `UD-IQ1_S`, `UD-IQ1_M`, `UD-IQ2_XXS`, `UD-IQ2_M`, `UD-Q2_K_XL`
  (an 8-10GB card has nothing to load); Nemotron has nothing below 46GB and
  skips `UD-IQ2_M`, `UD-Q4_K_XL`, `UD-Q5_K_XL`, `UD-Q6_K_XL`, `Q8_0`.
- Re-run `etags.txt` before the next catalog bump; any changed ETag means the
  bytes need re-verification.

## Files

- `verify-b11960.txt`, `verify-latest.txt` (v0.9.0), `verify-b12144.txt`
- `etags.txt`: per-file size and ETag snapshot, 2026-11-06
- `dump_gguf.py`: prints GGUF header key/values (and is the tool used above)
- `devstral-chat-template.jinja`, `nemotron-chat-template.jinja`: templates
  read from the GGUF headers, the evidence for `tool_calling: true`
