# T02 proposal schema

This is the contract for every model entry T02 proposes. The schema is defined by
`internal/catalog/catalog.go` (the Go structs) and `internal/catalog/catalog.json`
(the 32 models, `version: 8`, shipped with this snapshot). Proposals must decode
into `catalog.Catalog` without edits to the Go code and without inventing fields.

- Go source: `internal/catalog/catalog.go:25` (`Quant`), `:43` (`Draft`), `:50` (`Model`), `:137` (`Catalog`).
- JSON example: `internal/catalog/catalog.json` (copy an entry and adapt every field).
- Offline checker: `agents/T02/validate_proposal.py` (this directory).
- Online checker: `scripts/verify_catalog.py` (file sizes and GGUF headers).

## File shapes

A tier file (`agents/T02/tiers/tier-<NN>.json`) is:

```json
{"tier": "<label>", "models": [ {model}, {model} ]}
```

A `tier` label is one of `4`, `6`, `8`, `12`, `16`, `24`, `32`, `48`, `64`, `96`,
`128plus`. The file name must be `tier-<label>.json`; a mismatch is a warning.

The assembled file (`agents/T02/catalog-proposed.json`) is the real catalog shape:

```json
{"version": 9, "models": [ {model}, {model} ]}
```

`catalog.Load` only accepts a remote catalog whose `version` is strictly greater
than the embedded one (`catalog.go:174`), so the assembled file needs a version
above the current 8; `version: 1` is ignored at runtime. The validator warns
about a non-increasing version.

Unknown top-level keys are warnings; unknown model, quant and draft keys are
errors unless `--allow-extra` is used.

## Model fields

`R` means required in the default (strict) mode. The T02 brief requires every
field listed there to be present even where the Go tag is `omitempty`; strict
mode follows the brief, `--lenient` follows only what Go decodes.

| field | JSON type | Go type (`catalog.go`) | R | rule |
|---|---|---|---|---|
| `id` | string | `ID string` `:51` | yes | `^[a-z0-9]+(?:[.-][a-z0-9]+)*$`, 1-64 chars; unique in the file and not already in `catalog.json`. Examples: `qwen3.8-27b`, `k2-horizon-7b`. |
| `name` | string | `Name string` `:52` | yes | 1-80 chars, no leading/trailing whitespace. Human display name. |
| `family` | string | `Family string` `:53` | yes | 1-80 chars. Shared lineage, e.g. `Gemma 4`. |
| `vendor` | string | `Vendor string` `:69` | yes | 1-80 chars. Company or lab that trained the base model. |
| `country` | string | `Country string` `:70` | yes | ISO 3166-1 alpha-2, uppercase: `US`, `CN`, `FR`, `AE`. |
| `license` | string | `License string` `:54` | yes | 1-200 chars, SPDX id or the licence name as published (`Apache-2.0`, `Gemma Terms of Use`). Mark `UNVERIFIED` only when the source could not be checked; that produces a warning. |
| `params_b` | number | `ParamsB float64` `:55` | yes | Total parameters in billions, `0 < x <= 10000`. Use the architecture total, not the active count. |
| `active_params_b` | number | `ActiveParamsB float64` `:56` | yes | Parameters used per token, in billions, `0 < x <= params_b`. Dense models normally equal `params_b`; Gemma 4 E2B/E4B are the known exception (MatFormer effective size). For `moe: true` it must be strictly smaller. |
| `moe` | boolean | `MoE bool` `:57` | yes | True only for mixture-of-experts. |
| `score` | number | `Score float64` `:58` | yes | Artificial Analysis Intelligence Index v5.0, same scale as the current catalog (current range 1.0-27.0; 0-100 accepted). Higher is smarter. |
| `score_source` | string | `ScoreSource string` `:59` | yes | 1-2000 chars. Either names the source and a 4-digit year (an `http` link also passes), or starts with `ESTIMATE` and gives at least 40 chars of basis. Never invent a number: use `ESTIMATE` and explain. |
| `layers` | integer | `Layers int` `:60` | yes | Transformer block count, 1-1000. From GGUF `block_count`; `scripts/verify_catalog.py` tolerates the MTP/NextN layer either included or excluded. |
| `kv_heads` | integer | `KVHeads int` `:61` | yes | KV heads on full-attention layers, 1-512. GGUF `attention.head_count_kv` (max across layers when it is an array). |
| `head_dim` | integer | `HeadDim int` `:62` | yes | Key/value head size, 1-4096. GGUF `attention.key_length`; for MLA use `kv_lora_rank + rope.dimension_count`. |
| `kv_layers` | integer | `KVLayers int` `:63` | yes | Layers that own a growing KV cache, 1-`layers`. 0 means "all layers" in Go, but proposals must state the number. For hybrid models count only full-attention layers. |
| `ctx_max` | integer | `CtxMax int` `:64` | yes | Native context in tokens, 2048-16777216; a non-multiple of 1024 is a warning. Do not list a YaRN-extended length as native. |
| `tool_calling` | boolean | `ToolCalling bool` `:65` | yes | True when the chat template and model support tool calls. |
| `repo` | string | `Repo string` `:66` | yes | Hugging Face repo id `org/name` only, 1-200 chars. No scheme, no `huggingface.co/`, no `/resolve/`, no trailing slash, no `.git`, no whitespace. |
| `quants` | array | `Quants []Quant` `:67` | yes | Non-empty; see below. Sorted ascending by `bytes`. |
| `vision` | boolean | `Vision bool` `:71` | yes | True when the repo ships an mmproj / vision tower the app can use. |
| `native_1bit` | boolean | `Native1Bit bool` `:76` | yes | True for models trained at ~1-2 bits (Bonsai, ternary), whose tiny quants are the real model. |
| `notes` | string | `Notes string` `:77` | yes | 1-4000 chars. Release date, base model, quant caveats, benchmark caveats, anything the next reader needs. |
| `no_kv_quant` | boolean | `NoKVQuant bool` `:72` | no | Set true when quantizing the KV cache measurably hurts the model (currently gpt-oss). |
| `draft` | object | `Draft *Draft` `:73` | no | Separate speculative-decoding file; see below. |
| `swa_layers` | integer | `SWALayers int` `:82` | no | Sliding-window attention layers, 1-1000. |
| `swa_kv_heads` | integer | `SWAKVHeads int` `:83` | no | KV heads on those layers, 1-512. |
| `swa_head_dim` | integer | `SWAHeadDim int` `:84` | no | Head size on those layers, 1-4096. |
| `swa_window` | integer | `SWAWindow int` `:85` | no | Window in tokens, 128-`ctx_max`. |

Sliding-window fields are all-or-nothing: set all four or none. The planner
uses them in `SWABytes` (`catalog.go:102`), so a partial set silently
undercounts memory.

## Quant fields

Each element of `quants` is a `Quant` (`catalog.go:25`):

| field | JSON type | R | rule |
|---|---|---|---|
| `name` | string | yes | 1-80 chars, unique within the model, the quant label users see (`Q4_K_M`, `UD-IQ2_XXS`, `QAT-Q4_0`). `MTP` in the name implies `mtp: true` (warning). |
| `files` | array of strings | yes | 1+ repo-relative GGUF paths, each `.gguf`; forward slashes only; no absolute path, no `.`/empty component, no `..`, no `?`/`#`, no whitespace, no URL. |
| `bytes` | integer | yes | Exact total size of all files in this quant, 1 B-1 TiB. Below 1 MiB is an error, below 100 MB a warning. Values must strictly ascend with the array index (ties allowed). |
| `repo` | string | no | Override repo when this quant lives outside the model's main repo. Same `org/name` rule. `catalog.go:124` builds the URL from this repo (or the model repo) plus each file. |
| `mtp` | boolean | no | True when the file embeds multi-token-prediction heads. |
| `quality` | number | no | 0 < x <= 1 override for the quality factor; omit when the default bit-width estimate is right. |
| `lazy_bytes` | integer | no | Bytes that can stay on disk with `--lazy-mode`; must be smaller than `bytes`. |
| `args` | array of strings | no | Extra llama-server flags known to help this file; each 1-200 chars. |

### URL rules

`Model.FileURL` (`catalog.go:124`) produces
`https://huggingface.co/<quant.repo or model.repo>/resolve/main/<file>`.
Therefore:

- `repo` is the bare `org/name`, never a URL and never a `resolve/main` path.
- `files` entries are the repo-relative paths, never URLs. A file may live in a
  subdirectory (`Q4/model.gguf`).
- Split GGUFs must follow llama.cpp's naming: first part
  `<prefix>-00001-of-0000N.gguf`, then `-00002-of-0000N.gguf` ... with the same
  `N`, and `N` equal to the number of listed files. llama.cpp discovers the
  parts by that pattern, so a wrong part name means a broken download.
- `draft.repo` / `draft.file` obey the same rules.

### Draft object

`draft` is a `Draft` (`catalog.go:43`) and requires all four fields:

| field | JSON type | rule |
|---|---|---|
| `repo` | string | bare `org/name` |
| `file` | string | repo-relative `.gguf` |
| `bytes` | integer | 1 B-1 TiB |
| `type` | string | one of `draft-mtp`, `draft-dflash`, `draft-dspark`, `draft-eagle3` (llama.cpp's `fit.specFor` also accepts eagle3). |

## Cross-field rules enforced

- `active_params_b <= params_b`; when `moe` is true it must be strictly smaller.
- `kv_layers <= layers`.
- `swa_layers <= layers`; `swa_layers + kv_layers <= layers`; `swa_window <= ctx_max`.
- Quant `bytes` ascend with the array; quant `name`s are unique; no file is
  listed twice in one model (a warning; cross-model sharing is not checked).
- Split file names agree in part count and sequence.
- `id` unique within the file and absent from the existing catalog. Re-proposing
  a model already in `catalog.json` fails: the proposal is for *new* models.
  The same id may appear in two tier files; the assembler dedupes by id.

## What the offline checker does not verify

`validate_proposal.py` is deliberately offline and fast. It cannot prove:

1. that a file exists or that `bytes` matches the real size (`scripts/verify_catalog.py`
   HEADs every file and compares the sum);
2. that `layers`/`kv_heads`/`head_dim`/`ctx_max` match the GGUF header
   (`scripts/verify_catalog.py`, or `gguf_info.py` from a15);
3. that a benchmark number or licence is true, only that the source string is
   shaped correctly;
4. that the quant is a real llama.cpp quant name or that the tensor types are
   supported by the pinned llama.cpp build;
5. anything about the model actually running (memory fit is `internal/fit`).

Run both checkers on the assembled catalog before the Lead merges:

```sh
python3 agents/T02/validate_proposal.py agents/T02/catalog-proposed.json
python3 scripts/verify_catalog.py --catalog agents/T02/catalog-proposed.json
```

## Using the validator

```sh
python3 agents/T02/validate_proposal.py agents/T02/tiers/tier-08.json
python3 agents/T02/validate_proposal.py agents/T02/tiers/tier-*.json
python3 agents/T02/validate_proposal.py --json agents/T02/catalog-proposed.json
python3 agents/T02/validate_proposal.py --lenient internal/catalog/catalog.json   # audit the shipped catalog
```

| flag | effect |
|---|---|
| `--lenient` | require only the fields Go decodes; the brief-only fields (`vendor`, `country`, `vision`, `native_1bit`, `notes`) become optional. Used to audit the existing catalog. |
| `--allow-extra` | downgrade unknown JSON keys from errors to warnings. |
| `--existing PATH` | catalog whose ids must not be reused (default: `internal/catalog/catalog.json`, found from the current directory or the repo root). |
| `--no-existing` | skip the existing-id collision check (e.g. validating the catalog against itself). |
| `--json` | print one machine-readable report instead of text. |

Exit codes: `0` every entry and file passed (warnings allowed), `1` at least
one entry or file-level check failed, `2` unreadable/unparseable file or bad
arguments. Output is one `[PASS]`/`[FAIL]` line per entry, then every error and
warning indented under it, then per-file and overall totals.

## Known gaps in the shipped catalog (as of this snapshot)

These are findings from running the validator on `internal/catalog/catalog.json`
with `--lenient`, not proposal requirements:

- 13 of 32 models list quants grouped by source repo rather than ascending by
  bytes (e.g. `gemma-4-12b`, `qwen3.8-27b`, `gpt-oss-120b`). The brief requires
  proposals to be sorted; the shipped catalog predates that rule.
- `native_1bit` is omitted on 26 of 32 models and `notes` on
  `bonsai-27b-1bit`; Go defaults them to `false`/`""`. Strict mode requires
  them for proposals.
- `gemma-4-e4b` and `gemma-4-e2b` have `active_params_b` smaller than
  `params_b` despite `moe: false` (MatFormer effective size). This is expected;
  the validator only checks the `<=` bound.
