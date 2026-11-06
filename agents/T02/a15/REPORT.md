# a15: GGUF helper

Agent a15, cross-cutting job for T02. Worktree `~/ezlm-agents/work/subs/T02/a15`,
branch `agent/T02-a15`. Everything here is under `agents/T02/`; no Go file,
`catalog.json` or `scripts/verify_catalog.py` was touched.

## What I built

`agents/T02/gguf_info.py` prints the GGUF fields a catalog entry needs for one
Hugging Face repo and file. It reads the header with HTTP range requests and
never downloads weights or writes to disk. Standard library only; tested with
plain `python3` 3.13.7.

```sh
python3 agents/T02/gguf_info.py <repo> <file> [--json] [--parts] \
    [--revision main] [--no-metadata] [--timeout 60]
```

The default output has a `catalog:` block with exactly the fields
`internal/catalog/catalog.go` stores (`layers`, `kv_heads`, `head_dim`,
`kv_layers`, `ctx_max`, and `swa_layers` / `swa_kv_heads` / `swa_head_dim` /
`swa_window` when the model has sliding-window layers), then parameter counts,
the tensor type histogram, hints, and the raw metadata. `--json` emits one
object with the same information for scripts.

How the catalog block is derived, matching `scripts/verify_catalog.py`:

- `layers` = `block_count` minus `nextn_predict_layers`. Several converters
  (qwen3.5, qwen3.6, Nemotron) count the MTP layer in `block_count`, and the
  catalog does not.
- `kv_heads` / `kv_layers` / `swa_*` come from `attention.head_count_kv`
  (scalar or per-layer array) plus `attention.sliding_window_pattern` (bool
  list or every-n-th int), `full_attention_interval`, `shared_kv_layers`,
  and `attention.key_length[_swa]`. Gemma files without a pattern fall back
  to "lower KV-head layers are full attention"; gpt-oss falls back to
  alternating layers. Every guess is printed as a `note:` line.
- `head_dim` = `attention.key_length`; for MLA it is `kv_lora_rank` +
  `rope.dimension_count`; if `key_length` is missing it falls back to
  `embedding_length / head_count`.
- `ctx_max` = `context_length`.
- `params.total_b` is the sum of the tensor shapes in the file. It is not the
  model card number and can be smaller (see finding 4).
- `active_b_est` (MoE only) scales expert tensors by
  `expert_used_count / expert_count` and leaves embedding tables out.

`--parts` reads every part of a split model and merges the tensor tables, so
the type histogram and parameter count cover the whole model. Without it the
script prints a note telling the caller to add `--parts`.

The parser handles GGUF v1-v3 (v1 uses 32-bit counts), skips the tokenizer
arrays instead of decoding them, and guards against a server that ignores
`Range` so a 20 GB file cannot be pulled into memory.

## Proof

`agents/T02/gguf_info-samples.txt` records the full output for four catalog
models:

| model | shape | catalog fields | header read |
|---|---|---|---|
| gpt-oss-20b | single file, MoE, MXFP4 | all match | 1.52 s, 16.8 MB |
| gemma-4-12b | single file, 40 sliding-window layers | all match | 1.69 s, 16.8 MB |
| qwen3.8-27b | single file, hybrid attention | all match | 1.37 s, 16.8 MB |
| qwen3.8-flash-next | split 2 files, `--parts` | all match | 2.37 s, 33.6 MB |

I also ran the script over all 32 models in `internal/catalog/catalog.json`,
one process per model, and compared the derived block with the catalog:
**every `layers`, `kv_heads`, `head_dim`, `kv_layers`, `ctx_max` and `swa_*`
value matched**. The only differences were `params_b` (13 models, expected,
finding 4) and one `ctx_max` (finding 1). The raw log is
`agents/T02/a15/catalog-check.txt`.

## Timing (the question from the brief)

A header read on this Mac, from the a15 worktree, one at a time:

- **Typical single-file model: 1.3 to 2.1 s wall, one range request, 16.8 MB
  fetched.** That includes DNS, TLS, the Hugging Face redirect, and parsing.
  The four recorded samples took 1.37, 1.52, 1.69 and 1.37 s.
- Header sizes in this catalog are 6.0 to 15.8 MB (bonsai-8b is 6.0 MB, the
  Gemma 4 files are 15.8 MB, most others 11 to 13 MB). The script starts with
  a 16 MB chunk, so all of these fit in one request.
- A header larger than 16 MB needs a second request of up to 64 MB; the
  nemotron 120B split part 1 (metadata only, 7.9 MB) is the small case.
- Split model with `--parts`: 2.4 s for two parts, 33.6 MB.
- With four reads in parallel on this shared network each read took 2.5 to
  3.3 s, so 11 tier agents checking five models each are looking at roughly
  10 to 20 s of network per agent, not a bottleneck.

`scripts/verify_catalog.py` reads 4 MB, then 8, then 16 MB and fetched about
29 MB and 3 s per header in my runs. A 16 MB first chunk would roughly halve
that, but that file is not mine to edit.

## Findings

### 1. `nemotron-3.5-lightning-30b-a3b` advertises less context than the file (minor)

`internal/catalog/catalog.json:2122` says `"ctx_max": 262144`, but
`nemotron_h_moe.context_length` in
`unsloth/NVIDIA-Nemotron-3.5-Lightning-30B-A3B-GGUF`
(`NVIDIA-Nemotron-3.5-Lightning-30B-A3B-UD-IQ3_XXS.gguf`) is **1048576**.
Evidence: `python3 agents/T02/gguf_info.py ... --json` prints
`"ctx_max": 1048576`; the raw key is in the same output. Understating is safe
for the memory plan (`scripts/verify_catalog.py` only warns when the catalog
value is more than 4x the file's), but the catalog sells the model short by
4x. A tier agent should confirm the vendor number and raise it, or record why
the catalog deliberately caps it.

### 2. Split part 1 can contain no tensors at all (major for the T02 workflow)

For `nemotron-3.5-super-vl-120b-a12b`, part 1 of 2 is metadata only: 0 tensor
infos, 7.9 MB. Part 2 has all 781 tensors and 46.1 GB. A helper that reads
only the first file reports `tensor_count: 0`, an empty type histogram and
`params: 0.0`, which looks like a broken file rather than a split. Evidence:
with `--parts` the same command reports 781 tensors and 123.61B parameters.

`qwen3.8-flash-next` is the other shape: part 1 has 1223 of 1224 tensors and
part 2 holds the single 51.2B `per_layer_token_embd.weight` lookup table.
Reading part 1 alone gives 125.74B parameters; `--parts` gives 176.94B, which
is close to the catalog's 180.0. Tier agents must pass `--parts` for any file
named `-NNNNN-of-NNNNN.gguf`, and a16 should not merge a proposal whose split
count disagrees with the number of files.

### 3. `expert_used_count` can be a per-layer array (minor, but it bites)

`nemotron_h_moe.expert_used_count` in
`nvidia/NVIDIA-Nemotron-3.5-Bolt-74B-A9B-24GB-GGUF` is an array of 88 int32
values, not a scalar. My first version computed `expert * used / experts`,
where `expert` is an int and `used` a list, and Python expanded a
74-billion-element list: the process grew past 1.8 GB and took 90 s. Fixed by
taking the maximum and printing a note. Anything else that does arithmetic on
`expert_used_count` needs the same guard. Same for `feed_forward_length` and
`expert_feed_forward_length`, which are also per-layer arrays in that file.

### 4. `params_b` in the catalog is a card number, not the file's tensor count (minor)

On 13 of 32 catalog models the sum of the GGUF tensor shapes differs from
`params_b` by more than 2%. Examples: `qwen3.8-flash-next` 176.94B vs 180.0
(part 2 only), `qwen3.8-27b` 26.90B vs 27.78, `gemma-4-e4b` 7.52B vs 8.0,
`muse-glimmer-30b` 27.85B vs 30.0. The differences are consistent with the
card counting tensors that are not in the main GGUF (vision towers, MTP heads,
n-gram tables) or rounding differently. The helper labels its number
`params.total_b` ("from tensor shapes") so nobody copies it into `params_b`
by mistake. Tier agents should keep taking `params_b` from the card.

### 5. Tensor type ids are hardcoded to llama.cpp b11960 (minor)

`TYPES` in `gguf_info.py` covers ids 0-42 from
`ggml/include/ggml.h` at tag b11960, including the removed ids and the 2026
additions `Q1_0` / `Q2_0` / `MXFP4` / `NVFP4`. A file that uses a later id
prints it as `TYPE_<n>` and adds a note, so it is visible rather than silent.
The catalog's GSQ-RCO quants only use standard ids, so this does not affect
T02 today.

## What I could not finish

- `vision` and `tool_calling` are hints only. The main GGUF often has no
  vision tensors (Gemma 4 and Nemotron VL keep the vision tower in a separate
  `mmproj` file), so `hints.vision_tensors: no` does not mean the model is
  text-only. `hints.chat_template_tools` is a substring check on the chat
  template, not a capability test.
- `--parts` needs a `-NNNNN-of-NNNNN.gguf` file name. A split file with a
  different naming scheme, or a full-URL invocation, reads only the file given.
- The GGUF v1 path is implemented but I found no v1 file to test it on; every
  catalog file is v3.
- `params.active_b_est` is a sanity check, not a source. It is close on
  gpt-oss (3.61 vs 3.6) and qwen3.8-flash-next (6.67 vs 6.0) but a model with
  a large lookup table can still make it drift.

## Next steps

1. Tier agents: use `--json` and copy the `catalog:` block; add `--parts` for
   split files. For `params_b`, `score`, `license` and `vision`, keep the
   card/benchmark source.
2. a16: reject a tier entry whose quant `files` length disagrees with
   `split.count`, and prefer the helper's tensor types when checking that a
   quant is loadable by the pinned llama.cpp tag.
3. Someone who owns `scripts/verify_catalog.py` could start the reader at
   16 MB (about 1.5 s instead of 3 s per header) and reuse this script's
   `derive()` so the two cannot drift.
