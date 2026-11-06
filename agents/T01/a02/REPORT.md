# T01 a02: catalog truth audit

**Agent:** a02
**Models owned:** `ternary-bonsai-2-9b` (1 quant) and `qwen3.6-35b-a3b` (13 quants)
**Worktree:** `~/ezlm-agents/work/subs/T01/a02`, branch `agent/T01-a02`
**Result:** 0 corrections. All 14 file sets, both metadata blocks and both model cards
checked out against their sources. Three minor findings and two open questions are
recorded below; none of them is a wrong catalog value.

## What I checked

1. `scripts/verify_catalog.py --tag b11960 --jobs 2 --only ternary-bonsai-2-9b,qwen3.6-35b-a3b`
   → `agents/T01/a02/verify-b11960.txt` (14/14 `ok`, 0 file sets with errors).
2. Same against the newest release, `v0.9.0` (from the GitHub releases API)
   → `agents/T01/a02/verify-latest.txt` (14/14 `ok`, 0 errors).
3. Every field the verifier is silent or lenient about, read by hand from the GGUF
   header (HTTP range requests, no weights downloaded) and from the model card:
   `params_b`, `active_params_b`, `moe`, `license`, `tool_calling`, `vision`,
   `native_1bit`, `draft`, the `mtp` flag of each quant, and the `notes` claims.
4. Repo existence, file existence, exact byte sizes (HEAD), and repo creation dates
   (HF API `createdAt`) for the release dates quoted in `notes`.
5. Both catalog copies (embedded, `picom-code.github.io`, `raw.githubusercontent`)
   hold byte-identical entries for these two models.

## Evidence per model

### ternary-bonsai-2-9b (`catalog.json:37-73`)

Header of `prism-ml/Ternary-Bonsai-2-9B-gguf/Ternary-Bonsai-2-9B-Q2_0.gguf`:
`general.architecture=qwen35`, `qwen35.block_count=32`,
`attention.head_count_kv=4`, `attention.key_length=256`,
`context_length=262144`, `full_attention_interval=4`, tensor types `{0 F32, 42 Q2_0}`.
Card (`README.md`): "~8.95B language model ... + ~0.46B vision tower", "32 blocks",
"24 linear-attention and 8 full-attention layers", "262,144 tokens", Apache 2.0,
`Q2_0` runs on stock llama.cpp from b11556 on.

| Field | Catalog | Evidence | Verdict |
|---|---|---|---|
| `layers` | 32 | `qwen35.block_count=32` | ok |
| `kv_heads` | 4 | `qwen35.attention.head_count_kv=4` | ok |
| `head_dim` | 256 | `qwen35.attention.key_length=256` | ok |
| `kv_layers` | 8 | `full_attention_interval=4` over 32 blocks = 8 full-attention layers; card says the same | ok |
| `ctx_max` | 262144 | `qwen35.context_length=262144`; card "262,144 tokens" | ok |
| `params_b` | 9.41 | card 8.95B text + 0.46B vision; GGUF text tensors sum to 8,953,803,264 (8.954B) | ok |
| `active_params_b` | 9.41 | dense (no `expert_count` in the header), so active = total | ok |
| `moe` | false | no `expert_count` / `expert_used_count` keys | ok |
| `license` | Apache-2.0 | card front matter `license: apache-2.0` | ok |
| `tool_calling` | true | `tokenizer.chat_template` has the `<tools>` / `<tool_call>` block | ok |
| `vision` | true | `Ternary-Bonsai-2-9B-mmproj-Q8_0.gguf` (624,229,664 B) and `-mmproj-BF16.gguf` in the repo; card "Vision input through a separate mmproj pack" | ok |
| `native_1bit` | true | card "Ternary {−1, 0, +1} with FP16 group-wise scales", tensors are ggml type 42 `Q2_0` | ok |
| `draft.bytes` | 1574233856 | HEAD of `Ternary-Bonsai-2-9B-DFlash2-Q8_0.gguf` = 1,574,233,856 | ok |
| `draft.type` | draft-dflash | draft GGUF `general.architecture=dflash`; b11960 has `LLM_ARCH_DFLASH` and `--spec-type draft-dflash` in `common/arg.cpp` | ok |
| quant `Q2_0` | 2556640128 | HEAD = 2,556,640,128; card "2.56 GB" | ok |
| `notes` | released 2026-09-23 | HF `createdAt` 2026-09-23T06:12:31Z | ok |

Why only `Q2_0` is listed: the card says `PTQ1_0` and `PQ2_0` are unknown to upstream
llama.cpp (they need the PrismML fork), while `Q2_0` runs on stock builds from b11556.
b11960 and v0.9.0 both map ggml type 42 to `Q2_0`, so the single listed quant is the
right one for EZLM.

### qwen3.6-35b-a3b (`catalog.json:459-589`)

Header of `unsloth/Qwen3.6-35B-A3B-GGUF/Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf`:
`general.architecture=qwen35moe`, `block_count=40`, `attention.head_count_kv=2`,
`attention.key_length=256`, `context_length=262144`, `expert_count=256`,
`expert_used_count=8`, `full_attention_interval=4`.
Card: 35B total / 3B activated, 40 layers, "16 for Q and 2 for KV", head dim 256,
262,144 native context, 256 experts (8 routed + 1 shared), MTP trained.
`Qwen/Qwen3.6-35B-A3B` safetensors `total_size` 71,903,645,408 → 35.9518B params.

| Field | Catalog | Evidence | Verdict |
|---|---|---|---|
| `layers` | 40 | `block_count=40` on all 10 plain files; the 3 MTP files are `block_count=41` with `nextn_predict_layers=1` | ok |
| `kv_heads` | 2 | `attention.head_count_kv=2`; card "2 for KV" | ok |
| `head_dim` | 256 | `attention.key_length=256`; card "Head Dimension: 256" | ok |
| `kv_layers` | 10 | `full_attention_interval=4` over 40 layers; card "10 × (3 × DeltaNet → 1 × Gated Attention)" | ok |
| `ctx_max` | 262144 | `context_length=262144`; card "262,144 natively" | ok |
| `params_b` | 35.95 | safetensors `total_size`/2 = 35.9518B (GGUF text tensors alone are 34.661B; the 1.29B difference is the vision tower) | ok |
| `active_params_b` | 3.0 | card "3B activated"; GGUF arithmetic gives 3.455B (see open question 1) | ok, see Q1 |
| `moe` | true | `expert_count=256`, `expert_used_count=8` | ok |
| `license` | Apache-2.0 | card `license: apache-2.0`, `general.license=apache-2.0` in the GGUF | ok |
| `tool_calling` | true | chat template has the `<tools>` / `<tool_call>` block | ok |
| `vision` | true | card "Causal Language Model with Vision Encoder"; `mmproj-{BF16,F16,F32}.gguf` in the unsloth repo | ok |
| 13 quants | 13 | each HEAD size matches `bytes` exactly (verifier); all 13 basenames exist in the stated repo | ok |
| `mtp` flags | 3 true | the 3 flagged files have `nextn_predict_layers=1`; the 10 unflagged ones have no `nextn_predict_layers` | ok |
| `notes` | released 2026-04-15 | HF `createdAt` 2026-04-15T05:59:19Z | ok |

## Findings

| # | file:line | Finding | Severity |
|---|---|---|---|
| 1 | `internal/catalog/catalog.json:538-541` vs `:580-583` (also `:498`/`:561`, `:522`/`:570`) | The plain and MTP variants of `UD-IQ3_XXS`, `UD-IQ4_XS` and `UD-Q4_K_XL` have **the same basename** (`Qwen3.6-35B-A3B-UD-*.gguf`) but different bytes and different repos. `internal/engine/engine.go:473` saves by `filepath.Base(file)` under `paths.Models()/<model id>`, so the two variants share one local path: switching between them forces a full re-download and both files can never coexist. Sizes differ (13211155424 vs 14069266720, 17730509792 vs 18209036576, 22360456160 vs 22853663008), so `Download` re-fetches instead of silently loading the wrong file (`internal/engine/download.go:44-48`). Same pattern exists for `qwen3.5-9b` (`catalog.json:837`), so it is not unique to this slice. | minor |
| 2 | `internal/catalog/catalog.json:55` and `:478` | `vision: true` is correct for both models (both repos ship mmproj files), but the field is **dead data**: `grep '\.Vision'` finds only `internal/app/connect.go:21`, `internal/integrate/dsh_config.go:102`, `internal/integrate/opencode_config.go:70` and `internal/serve/props.go:56`, all of which read the *runtime* props, not the catalog. EZLM never downloads or passes an `--mmproj` (no `mmproj` reference anywhere in the Go, C# or Swift tree), and `internal/integrate/types.go:21` says "an image projector is loaded (never, today)". The Bonsai card is explicit that images need `--mmproj Ternary-Bonsai-2-9B-mmproj-Q8_0.gguf`. | minor (catalog cannot express the mmproj; no wrong value) |
| 3 | `internal/catalog/catalog.json:43,53,466,476` | `params_b` counts the vision tower (9.41 = 8.95 + 0.46; 35.95 = 34.66 + 1.29) while `active_params_b` is text only. `internal/fit/fit.go:350-357` computes `activeFrac = active_params_b / (params_b * resident)`, so the two sides of the ratio have different bases. For qwen that is 3.0/35.95 = 8.34%, whereas the text-only active fraction is 3.455/34.661 = 9.97%. The value was left alone: 3.0 is the vendor's published "3B activated", and `internal/fit/fit.go:358-362` documents the sparsity curve as "calibrated on Qwen3.6 35B A3B (8% active, 69 tok/s measured)", i.e. the planner was tuned with 3.0/35.95. See open question 1. | minor |
| 4 | `internal/catalog/catalog.json:57` | `native_1bit` + `Q2_0` is only correct because EZLM's pinned llama.cpp carries the PrismML Hadamard runtime. The card warns that builds older than b11556 "give wrong output without any warning". b11960 and v0.9.0 both have it (`grep -c hadamard src/llama-model.cpp` = 62 and 44) and both know ggml type 42 `Q2_0`, so the pin is safe. Worth a regression check if the pin ever moves back. | info |

## Corrections

None. Every value in both model objects matched the GGUF header, the model card or the
base repo's `model.safetensors.index.json`. `internal/catalog/catalog.json` is unchanged
by this slice, so the branch carries only `agents/T01/a02/`.

## Open questions

1. **`active_params_b` for qwen3.6-35b-a3b: 3.0 or 3.46?** The vendor says "3B
   activated" and the planner's calibration comment assumes 8% (3.0/35.95). The GGUF
   tensors give 3.455B including the padded embedding and LM head (34.661B total), or
   2.44B excluding them. I did not change the number, because any change moves every
   estimated tok/s for this model and the three candidates (2.44 / 3.0 / 3.46) are
   different definitions rather than a proven error. If the Lead wants the planner to use
   the measured arithmetic, the change belongs together with a re-run of
   `internal/fit/calib_test.go`, not in a truth pass.
2. **Should `params_b` include the vision tower?** It does for both of my models (and
   for every other `vision: true` entry I compared), but the GGUF EZLM actually loads is
   the text-only file. The tier cut-offs in `internal/fit/fit.go:157-164` (>=20, >=150)
   are far from both values, so nothing flips today. Worth stating as a rule in the
   catalog README so the next audit does not "fix" one entry to the text-only number.
3. **Bonsai 2 9B: `draft` pairing is unproven.** The DFlash2 drafter's header is
   `general.architecture=dflash` with `target_layers=[2,6,...,30]`, `block_size=16`,
   the same qwen35 tokenizer and hidden size 4096, and upstream b11960 accepts
   `--spec-type draft-dflash`. But the card only says the drafter is "optional" and never
   states that it was trained against the `Q2_0` packing rather than the recommended
   `PQ2_0`. A real-weights acceptance-rate test is the only proof; that needs a GPU, so
   it is out of scope tonight.
4. **Bonsai release notes drift.** The repo was modified 2026-11-04 (after the 09-23
   creation) while all catalog sizes still match. If a file is ever replaced in place,
   the byte-size check alone will not notice; that is the ETag check in T01's stretch
   goal, not something I could prove here.

## Next steps

- Lead: decide open question 1 and, if 3.46 is wanted, land it with a calibration re-run.
- Lead: consider a `mmproj` field (repo + file + bytes) on `catalog.Model` so `vision`
  stops being dead data; today every `vision: true` model is advertised to OpenCode and
  DSH as image-capable by the runtime props, which are always false.
- Lead: the duplicate-basename pattern (`plain` vs `(MTP)` of the same quant) affects
  `qwen3.6-35b-a3b` and `qwen3.5-9b`. Either key the download path by quant name or give
  the MTP files a distinguishing subdirectory in the catalog.
- Nobody needs to touch either model object tonight.

## Commands used

```sh
python3 scripts/verify_catalog.py --tag b11960 --jobs 2 \
  --only ternary-bonsai-2-9b,qwen3.6-35b-a3b | tee agents/T01/a02/verify-b11960.txt
curl -s https://api.github.com/repos/ggml-org/llama.cpp/releases/latest   # v0.9.0
python3 scripts/verify_catalog.py --tag v0.9.0 --jobs 2 \
  --only ternary-bonsai-2-9b,qwen3.6-35b-a3b | tee agents/T01/a02/verify-latest.txt
# header dumps and per-tensor parameter sums: import scripts/verify_catalog.py's
# Reader/header and walk the tensor-info table (no weights downloaded).
```
