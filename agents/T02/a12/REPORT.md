# T02 a12 report: retirement study

## What I did

I read the 32-model catalog at the 2026-11-06 snapshot, the T02 brief, and the
T02 section of `TASKS.md`. I then looked for models that were released or
became current between the catalog's snapshot (2026-11-02) and 2026-11-06,
plus the October 2026 models the catalog had not picked up, and compared them
against every current entry on three axes: Artificial Analysis Intelligence
Index v5.0 (or the catalog's own estimate), the smallest real GGUF quant, and
whether the replacement keeps the niche (vision, audio, EU/US origin,
compression, 128GB fit).

I did not edit `internal/catalog/catalog.json`. Deliverables:

- `agents/T02/retire-proposed.md`: the full 32-row table with per-model
  evidence and reasoning, the 13 retire/replace cases, the 19 keeps, the
  watch list of models that are better but not deployable yet, and the
  sources with access dates.
- `agents/T02/retire-proposed.json`: the 13 models proposed for
  retire/replace, each with a reason and a dated evidence string.

Result: 12 replace, 1 retire, 19 keep.

## Findings

| # | Finding | File:line | Evidence | Severity |
|---|---|---|---|---|
| 1 | Qwen3.6-27B is superseded by Qwen3.8-27B in the same 27.8B class: 20.1 vs 12.0 estimate, 7.27GB vs 9.57GB minimum. The catalog note already calls it a fallback. | `internal/catalog/catalog.json:591` | AA v5.0 Qwen3.8-27B 20.1, https://artificialanalysis.ai/models/qwen3-8-27b (2026-11-06) | major |
| 2 | Qwen3.6-35B-A3B is outclassed at the same 11-12GB footprint by flint (39.8B-A2.5B, SWE 74.2, TB2.1 54.9). | `internal/catalog/catalog.json:460` | https://huggingface.co/RecursiveSI/flint (2026-10-21) | major |
| 3 | Qwen3.5-122B-A10B is dominated in the 48GB tier by Nemotron 3.5 Super VL (13.6 vs 7.5, 46.1GB vs 40.3GB). | `internal/catalog/catalog.json:678` | https://artificialanalysis.ai/models/nemotron-3-5-super-vl (2026-11-06) | major |
| 4 | Qwen3.5-9B is outclassed by MiMo-V2.6-Distill-Qwen-9B, a newer agentic SFT of the same base (SWE 61.1 vs 60.0, SWE Pro 44.6 vs 32.0, AutomationBench 30.3 vs 5.0) at the same quant sizes and with vision kept. | `internal/catalog/catalog.json:751` | https://huggingface.co/XiaomiMiMo/MiMo-V2.6-Distill-Qwen-9B (2026-09-21) | major |
| 5 | Gemma 4 31B and 26B-A4B are outscored by Qwen3.8-27B at a smaller footprint (20.1 vs 8.0/5.5). | `internal/catalog/catalog.json:1024`, `:1138` | https://artificialanalysis.ai/models/qwen3-8-27b (2026-11-06) | major |
| 6 | gpt-oss-120b and gpt-oss-20b are August 2025 models. 120b is replaced by Nemotron 3.5 Super VL (US, 13.6 vs 7.0, smaller); 20b by Qwen3.8-27B (20.1 vs 5.0, smaller). | `internal/catalog/catalog.json:1562`, `:1616` | https://artificialanalysis.ai/models/nemotron-3-5-super-vl, https://artificialanalysis.ai/models/qwen3-8-27b (2026-11-06) | major |
| 7 | Devstral Small 2 24B (December 2025 coding agent) is outclassed by flint at the same 16GB tier. | `internal/catalog/catalog.json:1853` | https://huggingface.co/RecursiveSI/flint (2026-10-21) | major |
| 8 | Nemotron 3.5 Lightning 30B A3B is outclassed by Nemotron 3.5 Bolt 74B A9B in the same family (11.5 vs 5.5, 20.31GB vs 18.90GB). | `internal/catalog/catalog.json:2108` | https://huggingface.co/nvidia/NVIDIA-Nemotron-3.5-Bolt-74B-A9B-BF16 (2026-10-09) | major |
| 9 | Granite 4.2 8B and 3B are outclassed by K2-Horizon-7B and 3.7B, same Apache-2.0 class, much better coding (SWE 72.4 vs 47.7 and 65.6 vs 32.2). | `internal/catalog/catalog.json:2274`, `:2541` | https://huggingface.co/IFM/K2-Horizon-7B, https://huggingface.co/IFM/K2-Horizon-3.7B (2026-09-01) | major |
| 10 | Bonsai 8B 1-bit (2026-03-18) is superseded by Ternary Bonsai 2 4B at the same size (1.21GB vs 1.16GB, same 2.0 estimate, adds vision). Weakest case in the set: I did not benchmark the two. | `internal/catalog/catalog.json:145` | https://huggingface.co/prism-ml/Ternary-Bonsai-2-4B-gguf (2026-09-23) | minor |
| 11 | Muse Glimmer 30B, Mistral Small 4 119B and Granite 4.2 30B are outclassed on score but kept for US-origin, EU-origin and IBM enterprise niches respectively. | `:1669`, `:1767`, `:2194` | See `retire-proposed.md` keeps section | minor (keep) |
| 12 | Kimi K3.1 Pro (2026-11-02, 46.5) and DeepSeek V4.1 Flash 1017 (2026-10-17, 32.6) are the November quality leaders but need 1.56TB FP8 / 233GB minimum GGUF, so they do not fit a current EZLM tier. | n/a (not in catalog) | https://huggingface.co/moonshotai/Kimi-K3.1-Pro, https://huggingface.co/unsloth/DeepSeek-V4.1-Flash-1017-GGUF | minor (watch) |
| 13 | Ripple-65B Preview (2026-11-04, 65B dense, TB2.1 80.5) is a strong local coding model but its `ricursive` architecture is not in llama.cpp v0.9.0, so it is not deployable yet. | n/a (not in catalog) | https://huggingface.co/ricursiveAI/Ripple-65B-Preview; `src/llama-arch.cpp` v0.9.0 | minor (watch) |

The line numbers point at each entry's `"id"` line. The full table in
`retire-proposed.md` is the authority.

## What I changed

Only new files under `agents/T02/`:

- `agents/T02/retire-proposed.md`
- `agents/T02/retire-proposed.json`
- `agents/T02/a12/REPORT.md`

No Go, JSON catalog or other task's files were touched. The catalog is
unchanged, so `go build ./...` and `gofmt -l .` are unaffected; I still ran
them before committing.

## What I could not verify

- **Independent scores for the new small models.** K2-Horizon-7B,
  K2-Horizon-3.7B, flint and MiMo-V2.6-Distill-Qwen-9B are not on the AA
  v5.0 leaderboard on 2026-11-06. Their numbers are vendor-reported on the
  vendor's own harness. The comparisons are same-harness within each card,
  but not independent.
- **Quant degradation.** The benchmark numbers are full precision. The
  smallest GGUF quants (IQ1/IQ2) lose several points, so the score deltas
  overstate the low-bit experience on both sides.
- **Bonsai 8B throughput.** I did not benchmark ternary-bonsai-2-4b against
  the old 1-bit 8B. The retirement is based on release date, size and vision,
  not measured tokens per second.
- **Kimi K3.1 Pro GGUF.** No GGUF was published as of 2026-11-06, so the
  1-bit size is an estimate from the 1.56TB FP8 checkpoint.
- **Ripple-65B llama.cpp support.** The GGUF exists, but the architecture is
  absent from v0.9.0. It may be a small addition in a later build; I did not
  test it.
- **AA estimates.** Several current models have no official v5.0 index
  (qwen3.6-27b, qwen3.6-35b-a3b, qwen3.5-122b-a10b, qwen3.5-9b,
  gemma-4-\*, gpt-oss-\*, devstral, nemotron bolt). The direction of every
  replacement is clear; the exact delta is not.

## Next steps

1. The assembly agent (a16) should merge `retire-proposed.json` into the
   morning catalog review; the catalog itself is not edited here.
2. Re-check Ripple-65B and SenseNova-Loop-9B after the next llama.cpp
   upgrade. If `ricursive` lands, Ripple-65B becomes the 48GB coding pick and
   makes the flint recommendation less important.
3. Re-check Kimi K3.1 Pro and DeepSeek V4.1 Flash 1017 when a 256GB tier is
   added. DeepSeek at UD-IQ1_S (233GB) is the first model that clearly beats
   qwen3.8-flash-next on quality and fits a 256GB machine.
4. If the Lead wants a smaller diff, start with the four major cases that
   need no new GGUF at all: qwen3.6-27b -> qwen3.8-27b, gemma-4-31b ->
   qwen3.8-27b, gemma-4-26b-a4b -> qwen3.8-27b, and gpt-oss-20b ->
   qwen3.8-27b. Those are pure swaps inside the existing catalog.
5. Add a `score_source` note to the new picks (flint, K2-Horizon-7B/3.7B,
   MiMo distill) that says vendor-reported, not AA, so the catalog does not
   imply independent evaluation.
