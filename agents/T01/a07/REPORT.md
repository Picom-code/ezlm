# T01 / a07 report: gemma-4-e4b and gemma-4-e2b

Branch `agent/T01-a07`, worktree `~/ezlm-agents/work/subs/T01/a07`.
Owned entries: `gemma-4-e4b` (catalog.json:1350) and `gemma-4-e2b` (catalog.json:1456).
Nothing else in the catalog was touched.

## What was checked

- `scripts/verify_catalog.py --tag b11960 --jobs 2 --only gemma-4-e4b,gemma-4-e2b`
  (18 quant file sets: 9 per model): all files exist, every byte size matches the
  catalog exactly, architecture `gemma4` and all tensor types are known to
  b11960, no split files. Output: `verify-b11960.txt`, 0 errors.
- Newest release: `v0.9.0` (llama.cpp now uses semver tags; the GitHub
  `releases/latest` call returns `v0.9.0`, published 2026-11-04). Same run:
  `verify-latest.txt`, 0 errors.
- GGUF metadata of every quant of both models (`quant-headers.txt`): all 18
  files agree on `block_count`, `head_count_kv`, `key_length`,
  `key_length_swa`, `context_length`, `sliding_window`,
  `shared_kv_layers` and the sliding-window pattern.
- Full metadata dump of the first file of each model: `gguf-metadata.txt`.
- Parameter count from GGUF tensor shapes: `param-count.txt`.
- Shared-KV tensor layout: `shared-kv-tensors.txt`.
- Chat template tool support: `chat-template-tools.txt`.
- Model cards (README.md) and config.json of `google/gemma-4-E4B-it` and
  `google/gemma-4-E2B-it` read over HTTPS.
- MTP draft files: HEAD requests, both exist with the exact catalog sizes
  (98,653,248 and 97,817,664 bytes) and are real draft heads
  (`general.architecture = gemma4-assistant`, 4 `nextn_predict_layers`).
- Remote catalog (`https://picom-code.github.io/ezlm/internal/catalog/catalog.json`,
  same bytes as raw.githubusercontent main): `remote-diff.txt`.

## Findings

| # | file:line | finding | evidence | severity |
|---|---|---|---|---|
| 1 | `internal/catalog/catalog.json:1367`, `:1450`, `:1473`, `:1556` | `kv_layers` and `swa_layers` ignored `gemma4.attention.shared_kv_layers`, so the KV cache was overcounted | GGUF `gemma4.attention.shared_kv_layers` = 18 (E4B) / 20 (E2B); llama.cpp `src/models/gemma4.cpp` sets `n_layer_kv_from_start = n_layer_all - n_kv_shared_layers` (24 / 15) and `src/llama-kv-cache.cpp` skips every layer where `!hparams.has_kv(il)`, so only the first 24 / 15 layers allocate a cache | major (conservative direction: overcount, no OOM, but the plan is too pessimistic) |
| 2 | `internal/catalog/catalog.json:1366`, `:1472` | `active_params_b` did not match the model card | card, "Total Parameters": E2B "2.3B effective (5.1B with embeddings)", E4B "4.5B effective (8B with embeddings)"; catalog said 4.0 / 2.0 | minor (field is unused for dense models today; `internal/fit/fit.go:350` only reads it when `moe` is true) |
| 3 | `internal/catalog/catalog.json:1369`, `:1475` | notes quoted the name ("Effective 4B" / "Effective 2B") instead of the card's effective figure, and did not explain why `kv_layers` is smaller than `layers` | same card table | minor |
| 4 | `internal/integrate/types.go:21` | `vision: true` is capability data only: `catalog.Model.Vision` has no Go consumer, the endpoint's `Vision` comes from llama-server `/props`, and no image projector is ever downloaded ("an image projector is loaded (never, today)"), so image and audio input are advertised by the card but never served | `grep '\.Vision'` finds only `internal/app/connect.go:21` (from `/props`), `internal/integrate/dsh_config.go:102`, `opencode_config.go:70`, `internal/serve/props.go:56` | minor, cross-model, outside my owned paths, not fixed |
| 5 | remote catalog | the published catalog still carries the pre-fix values for both models (version 8, 32 models) | `remote-diff.txt`: 4 differing fields per model, exactly the four fixed here | minor: republish after the merge. `Load` only accepts a remote catalog with `Version >` embedded, so a version-8 republish is ignored by existing builds; the fix reaches users with the next embedded build |

No 404s: every repo and file of both models exists (including
`google/gemma-4-E4B-it-qat-q4_0-gguf`, `google/gemma-4-E2B-it-qat-q4_0-gguf`
and the `MTP/` draft files in the unsloth repos).

## Corrections

1. `gemma-4-e4b` `kv_layers` 7 -> 4, `swa_layers` 35 -> 20.
   GGUF `gemma4.attention.shared_kv_layers = 18`, `block_count = 42`: layers
   0..23 own a cache (`n_layer_kv_from_start = 42 - 18 = 24` in
   `src/models/gemma4.cpp`; `src/llama-kv-cache.cpp` skips the rest), and the
   sliding-window pattern marks 4 of those 24 as full attention and 20 as
   sliding window. At 131072 context this removes a 1.57 GiB f16 / 0.84 GiB
   q8_0 / 0.44 GiB q4_0 overcount (`memory-impact.txt`).
2. `gemma-4-e2b` `kv_layers` 7 -> 3, `swa_layers` 28 -> 12.
   Same keys with `shared_kv_layers = 20`, `block_count = 35` (15 owning
   layers: 3 full, 12 sliding window). Removes a 1.04 GiB f16 / 0.55 GiB q8_0
   / 0.29 GiB q4_0 overcount.
3. `gemma-4-e4b` / `gemma-4-e2b` `active_params_b` 4.0 -> 4.5 and 2.0 -> 2.3.
   Model card: "4.5B effective" and "2.3B effective".
4. Notes: "Effective 4B" -> "Effective 4.5B", "Effective 2B" -> "Effective
   2.3B", plus "Last 18 layers share KV caches." / "Last 20 layers share KV
   caches." so the next auditor does not "fix" `kv_layers` back.

Not changed, verified correct:

- `layers` 42 / 35 = `gemma4.block_count`.
- `kv_heads` 2 / 1 = `gemma4.attention.head_count_kv` (HF
  `num_key_value_heads`); `head_dim` 512 = `gemma4.attention.key_length`
  (HF `global_head_dim`).
- `swa_kv_heads` 2 / 1, `swa_head_dim` 256 = `gemma4.attention.key_length_swa`
  (HF `head_dim`), `swa_window` 512 = `gemma4.attention.sliding_window`.
- `ctx_max` 131072 = `gemma4.context_length`; card says 128K.
- `tool_calling: true`: the chat template defines
  `format_function_declaration` and `format_tool_response_block` (67
  tool/function lines); the card lists "Function Calling".
- `license: "Apache-2.0"`: card front matter `license: apache-2.0`, GGUF
  `general.license = apache-2.0`, card header "License: Apache 2.0".
- `params_b` 8.0 / 5.12: the text GGUF sums to 7.5181B / 4.6475B
  (`param-count.txt`) and the card counts the ~150M vision and ~300M audio
  encoders on top ("8B with embeddings" / "5.1B with embeddings"). Left as is.
- `moe: false`: no `ffn_gate_inp` or expert tensors, HF `enable_moe_block`
  false. E2B's `feed_forward_length` is a per-layer array (6144 for layers
  0..14, 12288 for 15..34): a double-wide MLP in the second half, not MoE.
- `native_1bit` absent: correct, these are not 1-bit trained.
- `vision: true`: correct capability (image and audio input), see finding 4.

## Open questions

- E4B effective parameters: the card says 4.5B, but the GGUF text tensors
  minus the PLE table are 7.5181B - 2.8186B = 4.6995B. E2B matches exactly
  (4.6475B - 2.3488B = 2.2987B vs card 2.3B). I used the card for E4B; if
  measured values are preferred, it should be 4.70.
- A future catalog consistency test must not assert
  `kv_layers + swa_layers == layers`: for shared-KV models the sum is the
  number of layers that own a cache (24 / 15 here), not the block count.
- `scripts/verify_catalog.py` already models sharing (`own = len(heads) -
  shared`) but only warns on undercounts, so it stayed silent on this. A
  symmetric "overcounted by more than one layer" warning would have caught it.
- The same check should run for every other catalog entry with
  `shared_kv_layers > 0`. I checked the other three Gemma 4 entries
  (`gemma-4-12b`, `gemma-4-31b`, `gemma-4-26b-a4b`): their GGUFs have
  `shared_kv_layers = 0`, so their counts are exact. They are not my slice.

## What I could not finish

- The published remote catalog (finding 5): it is built from `main`, so it can
  only be refreshed after the morning merge. Nothing else needs doing here.
- Vision wiring (finding 4): the consumers live in `internal/integrate` and
  `internal/serve`, frozen tonight.
- The E4B effective-parameter figure (open question 1): the card and the
  measured tensors disagree by 0.2B; the catalog owner should pick one.

## Next steps

- Republish the remote catalog after the merge (finding 5).
- Decide the E4B effective-parameter figure (open question 1).
- Wire `vision` end to end or stop advertising image input in the apps
  (finding 4, frozen paths).
