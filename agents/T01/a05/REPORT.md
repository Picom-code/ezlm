# T01 / a05: catalog truth audit for `bonsai-8b-1bit` and `gemma-4-26b-a4b`

Worktree `~/ezlm-agents/work/subs/T01/a05`, branch `agent/T01-a05`.
Both model entries were checked against the GGUF headers of every catalog file, the
Hugging Face model cards, and the `config.json` of the base repo. No numeric field
needed changing. One editorial defect was fixed (a note truncated mid-word), and two
planner findings (F6, F7) were proven with a small `go run` probe.

## What I ran

| Command | Result |
|---|---|
| `python3 scripts/verify_catalog.py --tag b11960 --jobs 2 --only bonsai-8b-1bit,gemma-4-26b-a4b` | 11 file sets, 0 errors (`verify-b11960.txt`) |
| same with `--tag v0.9.0` (what `/releases/latest` returns) | 11 file sets, 0 errors (`verify-latest.txt`) |
| same with `--tag b12144` (newest build, 2026-11-06) | 11 file sets, 0 errors (`verify-b12144.txt`) |
| HEAD on every quant file plus the MTP draft | every size matches the catalog byte for byte (`http-headers.txt`) |
| GGUF header + tensor table read by hand | `gguf-metadata.json` |

The script only walks `quants`; it never checks `draft`. I checked the draft by hand:
`unsloth/gemma-4-26B-A4B-it-GGUF/MTP/mtp-gemma-4-26B-A4B-it-Q8_0.gguf` exists,
is exactly 461766816 bytes as catalogued, and its architecture `gemma4-assistant`
is known to b11960, v0.9.0 and b12144.

`https://picom-code.github.io/ezlm/internal/catalog/catalog.json` (version 8) is
identical to the embedded catalog for both of my models, key for key.

## Findings

| # | Where | Finding | Evidence | Severity |
|---|---|---|---|---|
| F1 | `internal/catalog/catalog.json:165` (`bonsai-8b-1bit.notes`) | Note is cut mid-word at exactly 400 chars: `"... Ternary Bonsai 8B Q2_0 2.18GB superseded by Bo"` | the JSON string ends at `Bo`; the sentence has no object | minor (fixed) |
| F2 | `internal/catalog/catalog.json:25` (`ternary-bonsai-2-27b.notes`) | Same 400-char cut: `"... RTX 5090 129.9, RTX PRO 60"` | string ends mid-device-name | minor (not mine, see next steps) |
| F3 | `internal/catalog/catalog.json:390` (`qwen3.8-flash-next.notes`) | Cut at 300 chars: `"... arch merged 2026-08-27 (PR #27742"` | unbalanced `(` | minor (not mine) |
| F4 | `catalog.json:165` (`bonsai-8b-1bit.notes`) | Note claims `M4 Pro 116.8 (PrismML: 131)`; the current Bonsai-8B card's throughput table says M4 Pro 48 GB = 85 tok/s | `prism-ml/Bonsai-8B-gguf` README, "Cross-Platform Throughput" | minor, unproven (open question) |
| F5 | `scripts/verify_catalog.py:220` | The checker never looks at `m["draft"]`, so a dead or resized MTP file passes silently | `jobs = [(m, q) ... for q in m["quants"]]`, no `draft` pass | minor (script gap) |
| F6 | `internal/fit/fit.go:142` (`normQuant`) with `catalog.json:1230,1238` (`QAT-*` quants) | `normQuant` strips `GSQ-RCO-`, `UD-` and `IMATRIX-` but not `QAT-`, so `QAT-Q4_0` and `QAT-UD-Q4_K_XL` fall through to the 0.9 "unknown format" default instead of the 0.985 a Q4 file gets. Below `minQuality` 0.94 (models under 20B params, `fit.go:155-165`) the quant is dropped from planning entirely. | probe A/B: with only those two quants on a 16 GB GPU, `gemma-4-26b-a4b` picks `UD-IQ3_XXS` at 5.197 as-is and `QAT-Q4_0` at 5.417 once the prefix is stripped; `gemma-4-12b` scores the QAT file 2.961 instead of 3.241 (`probe-output.txt`) | major (not my file: one-line fix in `normQuant`) |
| F7 | `catalog.json:1158` (`gemma-4-26b-a4b.draft`) | The model has one draft, the non-QAT `MTP/mtp-gemma-4-26B-A4B-it-Q8_0.gguf`, but 2 of its 10 quants are QAT. Google's QAT card says a QAT target "must also be a QAT checkpoint with the same precision" assistant; unsloth ships a separate QAT drafter (`unsloth/gemma-4-26B-A4B-it-qat-GGUF/MTP/`, and a smaller smart Q4_0 at the repo root). EZLM pairs the draft with every quant (`fit.go:501-514`), so a QAT pick gets the non-QAT drafter. | `google/gemma-4-26B-A4B-it-qat-q4_0-gguf` card, "Assistant Compatibility"; `unsloth/gemma-4-26B-A4B-it-qat-GGUF/MTP/README.md` ("pairs with the QAT model unchanged", acceptance 0.73 against the QAT target) | major (needs a per-quant draft field, not just a catalog edit) |

No blocker: no 404, no size mismatch, no unknown architecture or tensor type.

### Two planner findings (F6, F7) that need code, not catalog edits

The catalog's `quality` field could paper over F6 for this one model, and pointing
`draft` at the QAT repo would paper over F7 for the two QAT quants, but both fixes
belong at the source:

- F6: add `"QAT-"` to the prefix list in `normQuant` (`fit.go:142`). That fixes all
  five Gemma 4 entries at once (`gemma-4-31b`, `gemma-4-26b-a4b`, `gemma-4-12b`,
  `gemma-4-e4b`, `gemma-4-e2b` all carry `QAT-Q4_0`). I did not add a per-quant
  `quality` override because that number (0.985 vs 0.99) would be my guess, and the
  card only says "preserving similar quality to bfloat16".
- F7: give `Quant` an optional draft override (or key the existing `Draft` on quant
  name) so a QAT quant can pull `unsloth/gemma-4-26B-A4B-it-qat-GGUF`'s drafter
  (Q8_0 = 461785600 bytes, or the root smart Q4_0 = 251939328 bytes). One draft per
  model cannot express this.

`agents/T01/a05/probe/` is a small `go run` program that reproduces F6 against the
real catalog and the real planner; it reads the embedded catalog and writes nothing.

### Why the numbers I did not touch are right

`bonsai-8b-1bit` (file `Bonsai-8B-Q1_0.gguf`, arch `qwen3`):

| Field | Catalog | GGUF / card | Proof |
|---|---|---|---|
| layers | 36 | 36 | `qwen3.block_count`; card "Layers 36" |
| kv_heads | 8 | 8 | `qwen3.attention.head_count_kv = 8` (32 query heads) |
| head_dim | 128 | 128 | `qwen3.attention.key_length = 128`, `value_length = 128` |
| kv_layers | 36 | 36 | every block has its own cache; no `nextn`/shared-KV keys |
| ctx_max | 65536 | 65536 | `qwen3.context_length = 65536`; card "65,536 tokens" (yarn x4 over 16384) |
| params_b / active_params_b | 8.19 / 8.19 | 8.189 | sum of tensor dims = 8,188,548,096; card "8.19B" |
| moe | false | dense | no `*_exps` tensors |
| native_1bit | true | true | all tensors are type 0 (F32) or 41 (`GGML_TYPE_Q1_0`); Q1_0 is a 1-bit type |
| vision | false | false | no mmproj in the repo, arch is text-only `qwen3` |
| tool_calling | true | true | `tokenizer.chat_template` renders `<tools>`/`<tool_call>` |
| license | Apache-2.0 | Apache-2.0 | card front matter `license: apache-2.0`, LICENSE file, table row |
| quants[0] | Q1_0 1158654496 | 1158654496 | HEAD; repo also carries an identical `Bonsai-8B.gguf` |

`gemma-4-26b-a4b` (file `gemma-4-26B-A4B-it-UD-Q4_K_XL.gguf`, arch `gemma4`):

| Field | Catalog | GGUF / config.json | Proof |
|---|---|---|---|
| layers | 30 | 30 | `gemma4.block_count = 30`; `num_hidden_layers: 30` |
| kv_heads | 2 | 2 | global layers have `head_count_kv = 2` (5 layers, indices 5/11/17/23/29) |
| head_dim | 512 | 512 | `gemma4.attention.key_length = 512` |
| kv_layers | 5 | 5 | `sliding_window_pattern` is False on exactly 5 layers |
| swa_layers / swa_kv_heads / swa_head_dim / swa_window | 25 / 8 / 256 / 1024 | same | 25 True layers with `head_count_kv = 8`, `key_length_swa = 256`, `sliding_window = 1024` |
| ctx_max | 262144 | 262144 | `gemma4.context_length`; `max_position_embeddings: 262144` |
| params_b | 25.81 | 25.806 | text 25.233B (tensor dims) + vision encoder 0.573B (mmproj-F16 tensor dims) |
| active_params_b | 3.8 | 3.823 | non-expert 2.395B + 8/128 x 22.838B expert; card "3.8B" |
| moe | true | true | `expert_count = 128`, `expert_used_count = 8`; card "8 active / 128 total" |
| vision | true | true | `mmproj-F16.gguf` in every repo of this family; card vision tables |
| tool_calling | true | true | chat template has `tools`/`tool_call`/`tool_response`; card "Function Calling" |
| license | Apache-2.0 | Apache-2.0 | all four cards say Apache 2.0; `general.license` in the google and unsloth files |
| draft | 461766816 | 461766816 | HEAD; arch `gemma4-assistant` is in b11960 |

On `params_b` = 25.81: the catalog's convention includes the vision encoder, which is
why `gemma-4-31b` is 31.27 (30.7 text + 0.55 vision). 25.233 + 0.573 = 25.806, so
25.81 follows the same rule and I left it alone.

On the KV math: the 5 global layers have no `attn_v` tensor (unified K/V,
`attention_k_eq_v: true`), but llama.cpp still allocates a separate V cache buffer
for them: `src/models/gemma4.cpp:84` creates `attn_v` as optional and `:253` uses
`Kcur` as `Vcur`, while the cache sizes both from `n_embd_k_gqa`/`n_embd_v_gqa`.
So `KVBytesPerToken` counting K and V for those 5 layers matches what llama.cpp
allocates, and `SWABytes` matches the 25 window layers. No change.

## Correction made

**C1, `catalog.json:165`, `bonsai-8b-1bit.notes`:** replaced the trailing
`"... 2.18GB superseded by Bo"` with `"... 2.18GB superseded by Bonsai 2."`
Why: the note was cut at 400 characters in the middle of a word. `Bonsai 2`
(`ternary-bonsai-2-{2,4,9,27}b`, released 2026-09-16..23, catalog scores 1.0-18.0)
is the generation that replaced the April-2026 Ternary Bonsai 8B
(`prism-ml/Ternary-Bonsai-8B-gguf`, created 2026-04-18, Q2_0 = 2,182,184,672 bytes
= 2.18 GB, 75.5 avg). The rest of the note (1.16 GB file, sibling sizes
0.57 GB / 0.25 GB) checks out against the repos. No numbers changed.

Commit: `T01: complete truncated bonsai-8b-1bit note`.

## Open questions

1. The exact original wording of the truncated note is lost. If the author meant a
   specific model (for example `Bonsai 2 9B`, the closest size), that is a better
   sentence than the family-level one I committed.
2. `bonsai-8b-1bit.notes` says M4 Pro 116.8 tok/s (PrismML 131); the card in the
   repo today says 85 for M4 Pro 48 GB. The other numbers in that note (RTX 3090,
   3080, GB10, Strix Halo, 1080 Ti, A2000) are not on the card at all, so they come
   from the whitepaper or the demo repo and I could not check them.
3. `bonsai-8b-1bit.score = 2.0` is an ESTIMATE. The card's own suite puts it at
   70.5 avg against Qwen 3 8B at 79.3, and the catalog scores Qwen3.5-9B at 5.0.
   A v5-equivalent of ~2-3 is plausible, but nothing official exists, so I left it.
4. `gemma-4-26b-a4b` counts the 0.573B vision encoder in `params_b` but not in
   `active_params_b`. That shifts the MoE active fraction by ~3% (0.147 vs 0.152),
   which only affects the speed estimate. Consistent with `gemma-4-31b`; no change.

## Next steps for the Lead

- F2 and F3 are the same truncation bug in two entries I do not own
  (`ternary-bonsai-2-27b` at 400 chars, `qwen3.8-flash-next` at 300 chars). Worth a
  sweep of every `notes` string for a mid-word tail; `granite-4.2-3b` (518 chars) is
  a complete sentence, so the cut is not a uniform field limit.
- Teach `scripts/verify_catalog.py` to check `m["draft"]` (existence, size,
  architecture) the same way it checks quants; the MTP draft is downloaded too, so a
  wrong byte count there costs the user disk and a failed speculative load.
- `/releases/latest` returns `v0.9.0` (2026-11-04) while `b12144` (2026-11-06) is the
  newest build. Both were checked; the brief's "newest release" step should say which
  one it means.
- F6 is a one-line fix in `internal/fit/fit.go` (`normQuant`: add `"QAT-"`). It changes
  no number in the catalog, so it is not mine to make; it is worth doing before the
  merge because five entries ship `QAT-*` quants.
- F7 needs a schema decision (per-quant draft). Until then, `gemma-4-26b-a4b` on CUDA
  will offer `--spec-type draft-mtp` with the non-QAT drafter for its two QAT quants.
