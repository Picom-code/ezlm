# T02 a14 report: benchmark sources catalogue

**Job:** build the dated catalogue of independent evaluation sources that the tier agents
(a01 to a11) can cite in `score_source`. **Deliverables:** `agents/T02/BENCHMARKS.md` and
`agents/T02/benchmarks.json`. **Access date for every row: 2026-11-06.**

## What I did

1. Read `README.md`, the T02 brief and the T02 section of `TASKS.md`.
2. Inspected the partial `benchmarks.json` left by the earlier attempt: 15 rows, valid JSON,
   consistent schema, all with access date 2026-11-06. Kept all 15 (see "What I changed").
3. Spot-checked every URL in those 15 rows with GET on 2026-11-06: all returned 200 except
   `arena.ai`, which returns 403 to scripted clients (Cloudflare). That row already records
   the 403 and says the numbers were read from the LMArena Hugging Face snapshot instead.
4. Added the 8 sources the brief names that were missing: MMLU-Pro, GPQA, EvalPlus
   (HumanEval+/MBPP+), BigCodeBench, RULER, Vals AI, and two vendor-independent commentary
   sources (Simon Willison's evals tag, Interconnects). Each was fetched live and the
   `verified` field records what was actually read on the page.
5. Wrote `BENCHMARKS.md` from the same data so the two files cannot drift: an orientation
   section, a summary table, per-source sections (URL, publisher, access date and HTTP status,
   what it measures, coverage, caveats, cadence, cite-as, what was checked), then the gaps.

Final count: **23 sources**, one row each in both files.

## What I found

- **The catalogue the tier agents need already existed in skeleton form and is sound.** The
  partial file's 15 rows are accurate and well-caveated; the earlier attempt died after the
  JSON but before the Markdown and the report. I finished rather than rewrote.
- **Half the named sources were absent.** MMLU-Pro, GPQA, HumanEval+/BigCodeBench,
  RULER/long-context and the blog roundups were all missing, which matters because the
  4-16 GB tiers lean on MMLU-Pro, GPQA and EvalPlus when Artificial Analysis has no row.
- **Two sources are frozen and one is stale.** The Open LLM Leaderboard results dataset was
  last written 2025-03-15; the Aider polyglot board's newest visible run is 2025-10-03.
  Both are still useful, but a 2026 model scored there is a historical comparison. This is
  now rule 3 of the "how to use" section, so agents do not cite them as current.
- **Version drift inside Artificial Analysis.** The catalog cites Intelligence Index v5.0
  (fetched 2026-11-02) while the methodology page described v5.0.1 on 2026-11-06. Index
  versions are not comparable, so `score_source` must carry the version, not just the name.
- **LMArena is not scriptable.** `arena.ai` 403s on GET; `lmarena.ai` redirects there. The
  usable machine-readable route is the `lmarena-ai/leaderboard-dataset` snapshot (CC-BY-4.0,
  lastModified 2026-11-06, text_style_control latest split 11,132 rows / 29 categories,
  publish date 2026-10-28). The row documents this so the next agent does not rediscover it.
- **No source measures what EZLM actually ships.** Nothing here scores a Q4_K_M GGUF, and
  nothing measures local tokens per second. That gap is stated explicitly in the Markdown so
  tier agents do not imply a full-precision benchmark number transfers to a quantised file.

## What I changed

- `agents/T02/benchmarks.json`: kept the 15 existing rows verbatim; appended 8 rows with the
  same key set (`id, name, url, publisher, kind, measures, coverage, caveats,
  open_weight_coverage, cadence, access_date, http_status, verified, cite_as, use_for`).
  Validated: 23 rows, unique ids, no missing keys, every access date 2026-11-06.
- `agents/T02/BENCHMARKS.md`: new. Generated from the JSON, then framed by hand.

No Go files touched, so `gofmt -l .` and `go build ./...` are unaffected; both were run
before the commit anyway.

## What I could not verify

- **GPQA's 448 / 198 question counts** are from the paper, not re-counted: the Hugging Face
  mirror is gated and the repository ships a password-protected zip. The row cites the
  repository and says the numbers come from the paper.
- **Exact per-model rows on several leaderboards** (SWE-bench, Terminal-Bench, BFCL) are
  rendered client-side; I verified the page-level facts (task counts, version, last update)
  rather than scraping individual scores.
- **Vals AI's index weighting** is not published in enough detail to audit; the row says so.

## Next steps

- The tier agents should cite **two** sources per score where possible: a composite index for
  the headline and a task benchmark for the claim that matters to the tier.
- If a future agent needs LMArena numbers, read the HF dataset, not the site.
- If T02 runs again after a new index version, bump the `access_date` and the version string
  in `score_source`; do not mix v5.0 and v5.0.1 numbers in one table.
