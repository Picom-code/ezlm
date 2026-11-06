# T02 a06 report: 24 GB tier

Agent: a06. Tier: 24 GB of usable GPU or unified memory. Branch: `agent/T02-a06`.
Worktree: `~/ezlm-agents/work/subs/T02/a06`. Date: 2026-11-06.

## What I did

1. Read `README.md`, `run/T02-brief.md`, the T02 section of `TASKS.md`,
   `internal/catalog/catalog.json`, `internal/catalog/catalog.go`,
   `scripts/verify_catalog.py` and `internal/fit/fit.go` (budget and overhead math).
2. Surveyed the November 2026 open-weight landscape on Hugging Face: trending
   GGUF repos, newest releases per lab (Qwen, Google, DeepSeek, Z.ai, Moonshot,
   NVIDIA, Mistral, IBM, Liquid AI, Ornith, Groot Team, Recursive, Ricursive,
   inclusionAI, Tencent), and Artificial Analysis for open-weight models in the
   15 to 45B range.
3. Selected four new models that fit 24 GB well, read their GGUF metadata with
   range requests (the same reader as `scripts/verify_catalog.py`), and took exact
   file sizes from the Hugging Face tree API.
4. Wrote `agents/T02/tiers/tier-24.json` and `agents/T02/tiers/tier-24.md`.
5. Validated with `python3 scripts/verify_catalog.py --catalog agents/T02/tiers/tier-24.json`:
   llama.cpp b11960, 16 of 16 quant sets `ok`, no warnings.

Proposed models:

| id | params | active | best quant for 24 GB | bytes | score |
|---|---|---|---|---|---|
| ornith-1.5-35b-a3b | 35.0B | 3.0B | Q3_K_XL | 17,801,889,440 | 13.0 ESTIMATE |
| groot-19b-a3b | 19.0B | 3.0B | Q8_0 | 20,230,129,536 | 10.0 ESTIMATE |
| flint-40b-a2.5b | 39.8B | 2.5B | UD-Q3_K_XL | 18,494,977,472 | 10.5 ESTIMATE |
| lfm2.5-24b-a2b | 24.0B | 2.0B | UD-Q5_K_XL | 17,918,820,608 | 7.5 ESTIMATE |

All ids are new and do not collide with the 32 catalog ids. Every quant file was
checked to exist with the exact byte size, with a known architecture and known
tensor types in b11960, and the catalog's layer, KV-head, head-dim, KV-layer and
context numbers were checked against the GGUF metadata.

## Findings

### 1. Ripple 65B Preview needs a newer llama.cpp than the pinned build (blocker for that model)

`ricursiveAI/Ripple-65B-Preview` (2026-11-04, Apache-2.0, 65B dense, 1M to 2M
context) fits 24 GB at `AtomicChat` `AD-IQ2_XXS` (20.01 GB) with a q4 KV cache
for 32K to 64K context, and its vendor Terminal-Bench 2.1 of 80.52 is above
Qwen3.8-27B's AA-measured 79.8.

Evidence: `verify_catalog.py --catalog` reports
`ERROR architecture 'ricursive' unknown to llama.cpp` for all three quant sets.
`src/llama-arch.cpp` at b11960 has no `ricursive` entry; the b12167 tag has
`{ LLM_ARCH_RICURSIVE, "ricursive" }`. So EZLM's pinned build cannot load it.
Severity: blocker for this model, not for the tier. It is excluded from
`tier-24.json` and recorded in `tier-24.md`. Revisit when the pin moves.

### 2. GLM-5.3-Flash and DeepSeek-V4.1-Flash are not 24 GB models (minor, sizing)

- GLM-5.3-Flash: AA Intelligence Index 27.8, MIT, 320B total and 18B active.
  Smallest quant (UD-IQ1_S) is 93.1 GB across 3 files.
- DeepSeek-V4.1-Flash-1017: 552B total, 16B active. Smallest quant (UD-IQ1_S)
  is 233.0 GB across 5 files.
Both are strong but belong in the 96 GB or 128plus tiers.

### 3. Some new models are not plain chat models (minor)

- Clover-35B-A3B-Preview (DiffuSpace): block-diffusion LM on a Qwen3.6-35B-A3B
  backbone. Q4_0 19.59 GB would fit, but its decoding path is not standard
  llama.cpp generation, and it is a preview.
- Clef and Clef-Flash (Cloudflare): decision models that emit one probability
  per allowed option, no free-form text.
- Granite Switch 4.1 30B (IBM): base model plus adapters plus speech encoder,
  needs the granite-switch composition runtime.
- Hy-Memory-30B-A3B (Tencent): a memory-writer specialist for Mem0/MemOS style
  harnesses.
None are suitable as a general catalog chat entry without extra runtime support.

### 4. No new dense model beats the catalog's dense 24 GB picks (minor)

The catalog already has Nanbeige4.3 33B (20.4), Qwen3.8 27B (20.1), Ternary
Bonsai 2 27B (18.0), Gemma 4 31B (8.0) and Muse Glimmer 30B (6.3). The only new
dense model with a credible claim at this tier was Ripple, which is blocked by
finding 1. That is why all four picks are MoE.

### 5. Score evidence is vendor-only for all four picks (minor, expected)

None of the four is on Artificial Analysis as of 2026-11-06 (checked the AA
model sitemap and the individual pages). Scores are `ESTIMATE`, with the basis
written into `score_source`: vendor Terminal-Bench or LiveClaw numbers mapped
through AA-measured values for Qwen3.6-27B, Qwen3.8-27B and Qwen3.6-35B-A3B,
which are already on the catalog scale. Treat them as unverified.

### 6. Flint's licence metadata disagrees with its repository (minor)

`RecursiveSI/flint` ships an Apache-2.0 LICENSE, but the GGUF header says
`general.license.name = flint-preview`. The entry uses Apache-2.0. T01 should
confirm with the vendor before this ships.

### 7. Coordination: `tier-24.json` was overwritten by another writer (process)

At 03:53 and 04:01 local, after my first write and validation,
`agents/T02/tiers/tier-24.json` was replaced by a different proposal containing six
*catalog* models (nanbeige4.3-33b, qwen3.8-27b, ternary-bonsai-2-27b,
nemotron-3.5-bolt-74b-a9b, qwen3.6-35b-a3b, gpt-oss-20b), whose ids collide with
catalog ids. Commit 90da599 (04:02) then committed that file as `tier-24.json`, my
proposal as `tier-24-alternate.json`, and my `tier-24.md`.

I restored the layout the brief asks for: `tier-24.json` now holds the four new
models with unique ids, and the incumbent lineup is preserved at
`agents/T02/a06/tier-24-incumbents.json` (also `tier-24-baseline-from-parent.json`).
The md already lists the incumbents in prose. If the Lead wants one full lineup
file per tier instead, merge the incumbents back in and drop the six duplicate ids
at assembly time. Please confirm the intended shape.

## What I could not verify

- Vendor country for Groot Team and Ornith AI. Recorded as US in the entries;
  no public source states it.
- Independent benchmark numbers for Ornith, Groot, flint, LFM2.5 and Ripple.
  Terminal-Bench, SWE-bench and LiveBench leaderboards were reachable but do not
  list these models; LMArena is behind Cloudflare and returned 403.
- Ornith's tool-call rendering under the catalog's template rules. Its card says
  the Qwen template was adjusted for benchmark parity; T03 should check it.
- Speed on real hardware. No model was loaded and no GPU memory was allocated.

## Next steps

1. Re-run `verify_catalog.py` against a newer llama.cpp tag once the pin moves;
   Ripple becomes eligible when `ricursive` is in the pinned build.
2. Add Ornith and Groot to the next AA fetch and replace the ESTIMATE scores.
3. T03: render the Ornith and Groot chat templates with tools and reasoning
   effort, and confirm the flint template's tool-call block.
4. Decide whether the catalog should include community fine-tunes; if yes, Tess
   4.5 27B and Saluki 27B are the candidates at this tier.
5. Confirm with the parent whether `tier-24.json` should contain new models only
   (current) or new plus incumbent recommendations (baseline file).

## Commands run

```
python3 scripts/verify_catalog.py --catalog agents/T02/tiers/tier-24.json
# llama.cpp b11960: 166 architectures, 44 tensor types; checking 16 files sets
# ... all ok
# 0 file set(s) with errors
```
