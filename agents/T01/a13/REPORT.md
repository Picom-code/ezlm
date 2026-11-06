# T01 / a13: Nemotron 3.5 Bolt 74B A9B, Nemotron 3.5 Lightning 30B A3B

Agent a13. Branch `agent/T01-a13`. Worktree `~/ezlm-agents/work/subs/T01/a13`.
Models owned: `nemotron-3.5-bolt-74b-a9b`, `nemotron-3.5-lightning-30b-a3b` in `internal/catalog/catalog.json`.
No extra assignment (no Go test file added).

## What I did

1. `scripts/verify_catalog.py --tag b11960 --jobs 2 --only <both ids>`: 14 file sets, 0 errors
   (`verify-b11960.txt`). All 14 quant sets exist at the listed URL with the exact byte size,
   split names and `split.count` are complete, and every tensor type and the architecture
   (`nemotron_h_moe`) are known to b11960.
2. Same run against the newest release tag from the GitHub API: `v0.9.0` (2026-11-04,
   `releases/latest`; the nightly build tags around it are `b12136`..`b12144`) -> 0 errors
   (`verify-latest.txt`). I also ran the newest nightly tag `b12144` -> 0 errors
   (`verify-b12144.txt`), because the pin is a build number and the semver release lags the nightlies.
3. Dumped the full GGUF header of one file per model and derived the planner numbers from the
   header plus the tensor table (range requests only, no weights): `header-*.txt`, `derive-*.txt`.
   Helpers: `dump_header.py`, `derive.py` (they import `scripts/verify_catalog.py` for the reader).
4. Cross-checked the numbers against the Hugging Face model cards and the HF API
   (`safetensors.total`), and against the other conversions of the same models
   (ggml-org, nvidia 24GB) to be sure the metadata is not a one-off.
5. Read both `tokenizer.chat_template` values from the GGUF headers (`template-*.jinja`) and
   checked the tool-calling path by hand.
6. Checked the `draft` files (HEAD size) and every quant file's `nextn_predict_layers`.

Evidence files in this folder: `verify-b11960.txt`, `verify-latest.txt`, `verify-b12144.txt`,
`header-bolt-UD-IQ1_M.txt`, `header-lightning-UD-IQ3_XXS.txt`, `derive-bolt.txt`,
`derive-lightning.txt`, `template-bolt.jinja`, `template-lightning.jinja`.

## Findings

Severity: blocker (wrong number that can crash or freeze a machine), major (wrong user-visible
capability or plan), minor (data or text wrong, no plan effect today).

| # | file:line | finding | evidence | severity |
|---|---|---|---|---|
| 1 | `internal/catalog/catalog.json:2121` | `nemotron-3.5-lightning-30b-a3b` `ctx_max` was `262144`; the model supports 1048576 | GGUF `nemotron_h_moe.context_length = 1048576` in all 7 catalog quant files (unsloth and ggml-org conversions); model card: "Context Length \| Up to 1M tokens (for single H100 deployment, we use 256K)"; the same-family `nemotron-3.5-super-vl-120b-a12b` (GGUF also 1048576) is already recorded as 1048576 | minor |
| 2 | `internal/catalog/catalog.json:2033` | bolt `ctx_max 262144` is correct, not a copy of super-vl's 1M | GGUF `context_length = 262144` in all 7 quant files (unsloth and the nvidia 24GB conversion); card "Context Length \| Up to 256K tokens" | ok |
| 3 | `internal/catalog/catalog.json:2030-2032`, `2037-2038` | bolt `layers 88`, `kv_heads 2`, `head_dim 128`, `kv_layers 8`, `vision true` all correct | `block_count 88`; `attention.head_count_kv` is a 88-entry list, 8 entries are 2 and the rest 0 (indices 7,16,25,36,47,58,69,78); `attention.key_length`/`value_length 128`; card: VLM with a C-RADIO v4-H vision encoder, `pipeline_tag image-text-to-text`; `mmproj-BF16.gguf` exists in the unsloth repo | ok |
| 4 | `internal/catalog/catalog.json:2118-2120`, `2125-2126` | lightning `layers 52`, `kv_heads 2`, `head_dim 128`, `kv_layers 6`, `vision false` all correct | unsloth files: `block_count 53` + `nextn_predict_layers 1`, and llama.cpp's `hparams.n_layer()` is `n_layer_all - n_layer_nextn` (`src/llama-hparams.cpp:349`), so the trunk is 52; ggml-org `Q4_0` has `block_count 52` with no nextn; 6 trunk layers carry `head_count_kv 2` (indices 5,12,19,26,33,42) and the nextn block 52 has its own 2-head attention in a separate MTP context (`src/llama-model.cpp:3127`); `key_length`/`value_length 128`; card `pipeline_tag text-generation`, no mmproj in the repo | ok |
| 5 | `internal/catalog/catalog.json:2026`, `2114` | `params_b 74.89` and `31.58` are exact | HF API `safetensors.total`: bolt 74,885,081,096 = 74.885B; lightning 31,577,937,344 = 31.578B. The bolt GGUF tensor table sums to 74.04B because the text-only GGUF leaves out the vision tower (the BF16 repo total includes it) | ok |
| 6 | `internal/catalog/catalog.json:2036`, `2124` | `active_params_b 9.0` / `3.0`, `moe true` | cards: "74B (9B active)", "30B (3B active)"; GGUF `expert_used_count` 9 (bolt) and 6 (lightning), `expert_count` 496 / 128 | ok |
| 7 | `internal/catalog/catalog.json:2034`, `2122` | `tool_calling true` for both | both templates take a `tools` list, render `<tools><function>...` and emit `<tool_call>` with `tool_calls` in assistant history (see `template-*.jinja`); the bolt card's vLLM recipe also sets `--enable-auto-tool-choice --tool-call-parser qwen3_coder` | ok |
| 8 | `internal/catalog/catalog.json:2025`, `2113` | `license "OpenMDW-1.1"` correct | GGUF `general.license.name = 'openmdw-1.1'`, `general.license.link = https://openmdw.ai/license/1-1/`; both cards `license: other, license_name: openmdw-1.1`; all four GGUF repos report the same | ok |
| 9 | `internal/catalog/catalog.json:2040-2045`, `2128-2133` | `draft` files exist with the listed sizes | HEAD `dflash-NVIDIA-Nemotron-3.5-Bolt-74B-A9B-NVFP4.gguf` = 998242560; `mtp-NVIDIA-Nemotron-3.5-Lightning-30B-A3B-Q4_0.gguf` = 1155907520 | ok |
| 10 | `internal/catalog/catalog.json:2134-2196` (lightning quants) | the 6 unsloth lightning files embed the MTP head (`nextn_predict_layers 1`, `blk.52.nextn.{eh_proj,enorm,hnorm,shared_head_norm}`) but are not marked `"mtp": true`; only the ggml-org `Q4_0` (52 blocks, no nextn) needs the separate `draft` file | tensor dump of `blk.52.*`; `internal/catalog/catalog.go:29-32` documents `mtp` as "the file embeds multi-token-prediction heads"; `internal/fit/fit.go:505-507` then skips the draft download. Marking them would NOT save the download today: `internal/app/app.go:750` fetches the draft whenever `p.Spec == p.Model.Draft.Type`, and lightning's draft type is already `draft-mtp`, so `p.Download` would just understate what is downloaded by 1.16GB. Needs an `internal/app` change (frozen area), so I did not touch the catalog for this | minor |
| 11 | `internal/catalog/catalog.json:2127` | lightning note says "Released 2026-08-01" | HF `createdAt 2026-08-01`, but the card's Model Summary says "Release Date \| August 11, 2026". Ambiguous, left as is | minor |
| 12 | `internal/catalog/catalog.json:2038`, `2126` | `vision true` for bolt while the catalog has no mmproj path, and `internal/integrate/types.go:21` says an image projector is "never, today" loaded | the value is a true model property, but `internal/app/connect.go:21` copies it into the client config, so EZLM will advertise image input it cannot serve. Cross-model issue, not mine to fix | minor |

No blockers: every file exists at the listed URL with the exact size for both models, on b11960
and on the newest tags.

## Corrections made

1. `internal/catalog/catalog.json:2121` (`nemotron-3.5-lightning-30b-a3b`):
   `"ctx_max": 262144` -> `"ctx_max": 1048576`.
   Proof: GGUF `nemotron_h_moe.context_length = 1048576` in all 7 quant files (checked each
   file, not just the first), and the card says "Up to 1M tokens". The entry had the bolt value.
   No plan changes today: `internal/fit/fit.go:92` tops the context ladder out at 65536 and
   `fit.go:309` only uses `ctx_max` as an upper bound, so this is a data-truth fix that matters
   once the ladder or a UI shows the real maximum.

Nothing else changed. Field order, indentation (1 space per level, as the file has it) and the
quant order are untouched.

## Open questions

- Bolt `vision: true` but no mmproj path in the catalog, and `internal/integrate/types.go:21`
  says a projector is never loaded. Either the catalog should name the mmproj file
  (`mmproj-BF16.gguf` in the unsloth repo) or `vision` should mean "the model is multimodal"
  only. Owner: whoever owns `internal/integrate` / the apps.
- Lightning's unsloth quants embed MTP heads (finding 10). Enabling `"mtp": true` on them is
  correct data, but it only pays off together with a change to `internal/app/app.go:750`
  (skip the separate draft when `q.MTP`). Left to the owner of `internal/app`.
- `score` for bolt is an explicit ESTIMATE (11.5) and for lightning an AA v5 lookup (5.5).
  I could not re-check AA's leaderboard from here, so both are unverified.
- `verify_catalog.py` never compares `kv_layers` for the nextn block and never warns when the
  catalog `ctx_max` is *below* the file's `context_length` (only when it is more than 4x above).
  That is why the lightning error in finding 1 went unnoticed. Suggested script change for the
  T01 Lead: also report `ctx_max < context_length`, and report `nextn_predict_layers > 0` as an
  `mtp` hint.

## Next steps

- Fold finding 10 into the `internal/app` draft-download change, then mark the 6 unsloth
  lightning quants `"mtp": true`.
- Decide the `vision` semantics (finding 12) across all multimodal entries; 23 of the 32
  entries have `vision: true` and none names an mmproj.
- Add the two `verify_catalog.py` checks above so the next audit catches under-reported
  `ctx_max` and embedded MTP heads.

## Verification run

```
python3 scripts/verify_catalog.py --tag b11960 --jobs 2 --only nemotron-3.5-bolt-74b-a9b,nemotron-3.5-lightning-30b-a3b
python3 scripts/verify_catalog.py --tag v0.9.0  --jobs 2 --only ...
python3 scripts/verify_catalog.py --tag b12144  --jobs 2 --only ...
```
All three: `0 file set(s) with errors`.
