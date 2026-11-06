# T02 a10: 96 GB tier

## What I did

Researched the 96 GB usable-memory tier and wrote `agents/T02/tiers/tier-96.json` (5 models, 9 verified quant file sets) and `agents/T02/tiers/tier-96.md` (ranked table, reasoning, drops).

Candidates considered: Qwen3.8 Flash Next, GLM-5.3 Flash, GLM-5.3, Nemotron 3.5 Super VL 120B A12B, Step 3.7 Flash, Qwen3 Coder Next, Mistral Medium 3.5 128B, Mistral Small 4 119B, Qwen3.5 122B A10B, Kimi K3/K3.1 Pro, DeepSeek V4.1 Flash 1017, Ornith 1.5 397B.

Method: real file sizes from the Hugging Face tree API (`/api/models/<repo>/tree/main?recursive=true`), real GGUF metadata with `agents/T02/gguf_info.py` (range requests, from the a15 worktree) and the catalog's own header-derived values, then `python3 scripts/verify_catalog.py --catalog agents/T02/tiers/tier-96.json`.

## Result

`verify_catalog.py` reports **0 file set(s) with errors** for all 9 sets on llama.cpp b11960. Two issues were found and fixed during validation: the guessed byte count for Mistral Medium UD-Q4_K_XL (correct 75,686,323,648) and Qwen3 Coder Next `kv_layers` (the file has 48 attention layers, not 12).

## Findings

- The 96 GB tier is the first tier where a 180B-class model fits at 3-bit with full context: Qwen3.8 Flash Next IQ3_S 83.62 GB plus 6.3 GB of 256K KV. It is the top pick (estimated AA v5 27.0).
- GLM-5.3 Flash (AA 28) misses the tier by about 3 GB: its smallest quant is 93.09 GB. Worth revisiting if a Q1_0-style file appears or if the fit margins shrink.
- Step 3.7 Flash and Qwen3.8 Flash Next scores are ESTIMATEs from AA component fits; both are documented in `score_source`.
- Qwen3 Coder Next at Q8_0 (84.81 GB) is the only lossless-weight option in the tier and is kept as the coding pick.

## Sources (all fetched 2026-11-06)

- https://artificialanalysis.ai/models/qwen3-8-flash-next
- https://artificialanalysis.ai/models/nemotron-3-5-super-vl
- https://artificialanalysis.ai/models/step-3-7-flash
- https://artificialanalysis.ai/models/qwen3-coder-next
- https://artificialanalysis.ai/models/mistral-medium-3-5-128b
- Hugging Face tree API for each repo named in the JSON.

## Next steps

- Re-score Flash Next and Step 3.7 Flash when AA publishes official v5 indices.
- Re-check GLM-5.3 Flash for a smaller quant; if one lands under 90 GB it takes rank 1.
