# Independent evaluation sources for T02 model scouting

Compiled by agent **a14** for task T02, November 2026. Access date for every row: **2026-11-06**.
The machine-readable twin of this file is [`benchmarks.json`](benchmarks.json); the two are kept in step, one row per source.

## How to use this catalogue

The tier agents cite these sources in `score_source`. Three rules keep the citations honest:

1. **Name the source, the version or index, and the date.** `Artificial Analysis Intelligence Index v5.0.1, 2026-11-06` is a citation; `Artificial Analysis` is not. Index versions are not comparable, so the version is part of the number.
2. **Prefer a source that ran the model itself.** A vendor model card, a Hugging Face model card or a launch blog is a *claim*, not an independent result. If the only number for a model comes from its maker, write `ESTIMATE` in `score_source` and say what the estimate is based on.
3. **Say when a source is stale or frozen.** The Aider board's newest visible run is 2025-10-03 and the Open LLM Leaderboard's results dataset was last written 2025-03-15. A 2026 model scored on either is a historical comparison, not a current measurement.

Two habits that save time: cite the **component** you actually used (SWE-bench Verified, not "SWE-bench"; GPQA Diamond, not "GPQA"), and record the **harness** for agentic results, because the scaffold is part of the score.

## Summary

| Source | Kind | What it answers | Open-weight coverage | Access |
|---|---|---|---|---|
| [Artificial Analysis Intelligence Index](https://artificialanalysis.ai/leaderboards/models) | composite-index | Composite index (Intelligence Index v5.0.1, roughly 0-100) over nine evaluations: AA-Briefcase v1.1, AutomationBench-AA, GDP.pdf, FrontierCode 2.0, Terminal-Bench 4.0, Terminal-Bench-Science 0.1, CritPt v2 (hard subset), AA-Omniscience v1.1 and HLE-Diamond | high | 2026-11-06 |
| [LMArena (text, vision, webdev, agent)](https://arena.ai/leaderboard/text) | human-preference | Pairwise human preference votes turned into a Bradley-Terry/Elo rating with confidence intervals | high | 2026-11-06 |
| [LiveBench](https://livebench.ai) | benchmark-leaderboard | Objective, auto-scored accuracy across 18 tasks in 6 categories (reasoning, math, coding, language, data analysis, instruction following) per the repository README, with monthly question refreshes and an agentic coding task run with Mini-SWE-Agent. | medium | 2026-11-06 |
| [Hugging Face Open LLM Leaderboard v2](https://huggingface.co/spaces/open-llm-leaderboard/open_llm_leaderboard) | benchmark-leaderboard | Six fixed benchmarks for open-weight models: IFEval, BBH, MATH-Hard, GPQA, MuSR and MMLU-Pro, run with fixed few-shot settings by the leaderboard harness. | high | 2026-11-06 |
| [SWE-bench](https://www.swebench.com) | coding-benchmark | Percentage of real GitHub issues resolved, judged by each repository's own tests | high | 2026-11-06 |
| [SWE-rebench v2](https://swe-rebench.com) | coding-benchmark | SWE-style issue resolution on tasks mined continuously from recent pull requests, with a time-window filter so a model can be scored only on tasks created after its training cutoff. | medium | 2026-11-06 |
| [Aider polyglot leaderboard](https://aider.chat/docs/leaderboards/) | coding-benchmark | Percentage of 225 Exercism exercises solved across C++, Go, Java, JavaScript, Python and Rust, with a second attempt allowed (pass rate 2 is the headline), plus cost per run and edit-format compliance. | high | 2026-11-06 |
| [LiveCodeBench](https://livecodebench.github.io) | coding-benchmark | Contamination-free code evaluation on problems from LeetCode, AtCoder and Codeforces, annotated with release dates | medium | 2026-11-06 |
| [EvalPlus (HumanEval+ / MBPP+)](https://evalplus.github.io/leaderboard.html) | coding-benchmark | pass@1 on HumanEval+ (164 problems with 80x more test cases than HumanEval) and MBPP+ (378 tasks with 35x more tests), plus EvalPerf for the runtime efficiency of generated code | high | 2026-11-06 |
| [BigCodeBench](https://bigcode-bench.github.io/) | coding-benchmark | Calibrated pass@1 on 1,140 practical programming tasks that require calling diverse libraries, with a Hard subset of about 150 user-facing tasks | high | 2026-11-06 |
| [Terminal-Bench 4.0](https://www.tbench.ai) | agent-benchmark | Resolution rate on 66 end-to-end terminal tasks, run through the Harbor harness with a named agent scaffold | medium | 2026-11-06 |
| [Berkeley Function Calling Leaderboard V4](https://gorilla.cs.berkeley.edu/leaderboard.html) | tool-use-benchmark | Function-call accuracy scored by AST matching, plus multi-turn (v3) and agentic (v4) categories, irrelevance and relevance detection, REST API, SQL, Java and JavaScript | medium | 2026-11-06 |
| [tau-bench 2.0](https://taubench.com) | tool-use-benchmark | Task success for a tool-using agent talking to a simulated user across airline, retail, telecom and banking_knowledge domains, plus the newer intake (tau-Entity) domain with 200+ tasks | medium | 2026-11-06 |
| [MMLU-Pro leaderboard](https://huggingface.co/spaces/TIGER-Lab/MMLU-Pro) | knowledge-benchmark | Accuracy on 12,032 test questions (plus 70 validation) across 14 disciplines, each with 10 answer options instead of MMLU's 4, so guessing is harder | high | 2026-11-06 |
| [GPQA / GPQA Diamond](https://github.com/idavidrein/gpqa) | knowledge-benchmark | Accuracy on 448 graduate-level, 'Google-proof' multiple-choice questions in physics, chemistry and biology, written by domain PhDs | medium | 2026-11-06 |
| [RULER long-context suite](https://github.com/NVIDIA/RULER) | long-context-benchmark | Synthetic long-context tasks in 4 categories (retrieval, multi-hop tracing, aggregation, question answering) across 13 task types at lengths from 4K to 128K | high | 2026-11-06 |
| [MMMU](https://mmmu-benchmark.github.io) | vision-benchmark | Accuracy on 11.5K college-level multimodal questions across six disciplines and 30 subjects, using 30 image types such as charts, diagrams, maps, tables, chemical structures and music sheets | medium | 2026-11-06 |
| [Epoch AI Benchmarking Hub](https://epoch.ai/benchmarks) | meta-aggregator | Database of benchmark results with the Epoch Capabilities Index (ECI), a single latent capability scale combining many benchmarks | medium | 2026-11-06 |
| [OpenCompass CompassRank](https://rank.opencompass.org.cn/home) | meta-aggregator | Aggregated scores across 100+ datasets covering knowledge, reasoning, coding, science, language, long-context and safety | high | 2026-11-06 |
| [HELM (Holistic Evaluation of Language Models)](https://crfm.stanford.edu/helm/) | benchmark-leaderboard | Standardized, reproducible evaluation across scenario families with multiple metrics: accuracy, calibration, robustness, fairness, bias, toxicity and efficiency. | medium | 2026-11-06 |
| [Vals AI benchmarks](https://vals.ai/) | independent-eval-lab | A composite Vals Index plus domain suites (finance, legal, software engineering, cybersecurity, immigration, mental health) run in-house on economically valuable tasks, reported with cost, latency and token usage alongside accuracy. | medium | 2026-11-06 |
| [Simon Willison, evals tag](https://simonwillison.net/tags/evals/) | blog-roundup | Hands-on write-ups of eval methodology, harnesses (smevals), prompt-level comparisons and specific model releases, usually with the prompts and raw outputs attached. | low | 2026-11-06 |
| [Interconnects (Nathan Lambert)](https://www.interconnects.ai/) | blog-roundup | Analysis of open-weight model releases, benchmark claims and the open-model ecosystem, written by a researcher who trains models; useful for judging whether a vendor benchmark number is plausible and how a release compares with its predecessor. | medium | 2026-11-06 |

`open_weight_coverage` is a judgement: **high** means most current open-weight releases appear, **medium** means the big ones appear, **low** means the source rarely covers them.

## General capability, preference and cost

### Artificial Analysis Intelligence Index

- **URL:** <https://artificialanalysis.ai/leaderboards/models>
- **Publisher:** Artificial Analysis
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** Composite index (Intelligence Index v5.0.1, roughly 0-100) over nine evaluations: AA-Briefcase v1.1, AutomationBench-AA, GDP.pdf, FrontierCode 2.0, Terminal-Bench 4.0, Terminal-Bench-Science 0.1, CritPt v2 (hard subset), AA-Omniscience v1.1 and HLE-Diamond. Four categories are weighted 25% each: Knowledge Work, Coding, Science and General.

**Coverage.** 250+ models on the public LLM leaderboard, proprietary and open weights, with a separate open-weights view (https://artificialanalysis.ai/models/open-source). Per-model pages break the index into its components and also report cost per task, time per task, output speed and an Openness Index.

**Caveats.**
- Index versions are not comparable. The catalog cites v5.0 fetched 2026-11-02 while the methodology page described v5.0.1 on 2026-11-06; record the exact version and fetch date in score_source.
- The category weights are an editorial choice; a model can rank differently on a single component such as FrontierCode 2.0 or HLE-Diamond.
- Reasoning-effort variants are scored separately (medium, high, xhigh), so a score without its effort setting is ambiguous.
- Small (under 4B), 1-bit/ternary and brand-new models are often missing or only partly scored; catalog ESTIMATE entries exist for this reason.
- The index is primarily English text; image inputs appear only inside some evaluations, and multilingual, speech and visual reasoning are reported separately.

**Cadence.** Leaderboard updated continuously; index version bumps every few months (v5.0 to v5.0.1 in late 2026).

**Cite as.** `Artificial Analysis, Intelligence Index v5.0.1 (artificialanalysis.ai, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. Methodology page states Intelligence Index v5.0.1 with nine evaluations, 25% per category and per-evaluation weights. Leaderboard page states 250+ models.

### LMArena (text, vision, webdev, agent)

- **URL:** <https://arena.ai/leaderboard/text>
- **Publisher:** LMArena
- **Access date:** 2026-11-06 (HTTP 403 on that date)

**What it measures.** Pairwise human preference votes turned into a Bradley-Terry/Elo rating with confidence intervals. The text board has 29 categories including overall, coding, math, creative writing, hard prompts, multi-turn, expert, nine languages and industry verticals, plus separate vision, webdev, document, search and agent boards.

**Coverage.** The latest text style-control snapshot (published 2026-10-28) has 11,132 model-by-category rows; 5,325 of those rows carry a proprietary licence, so open-weight models are roughly half the table. The full text_style_control history has 1,093,242 rows. Vote counts range from about 4,000 to 45,000 per model, so interval widths vary a lot.

**Caveats.**
- Crowdsourced prompts favour chat style and English; use the style-control board when comparing model quality.
- Cloudflare returns 403 to scripted GETs. Read numbers from the CC-BY-4.0 mirror https://huggingface.co/datasets/lmarena-ai/leaderboard-dataset (lastModified 2026-11-06).
- New models enter with few votes and wide intervals; do not rank on point estimates alone.
- Ratings are not comparable across boards (text, vision, webdev) or between raw and style-control variants.

**Cadence.** Republished periodically; the latest text snapshot is dated 2026-10-28, with full history in the Hugging Face dataset.

**Cite as.** `LMArena, text leaderboard, style control, published 2026-10-28 (accessed 2026-11-06)`

**Checked on 2026-11-06.** arena.ai returned 403 (Cloudflare) to scripted GET on 2026-11-06; lmarena.ai redirects there. Numbers read from lmarena-ai/leaderboard-dataset: text_style_control latest split 11,132 rows, 29 categories, publish date 2026-10-28, top rating 1527.5.

### LiveBench

- **URL:** <https://livebench.ai>
- **Publisher:** LiveBench team
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** Objective, auto-scored accuracy across 18 tasks in 6 categories (reasoning, math, coding, language, data analysis, instruction following) per the repository README, with monthly question refreshes and an agentic coding task run with Mini-SWE-Agent.

**Coverage.** Leaderboard covers API-served models and local models served on an OpenAI-compatible endpoint, with per-category and per-task tables. The changelog adds tasks after the README count (new reasoning, math and data-analysis tasks between 2025-12 and 2026-01), so check the live board for the current task set.

**Caveats.**
- The README's '18 tasks' lags the changelog; cite the task-set date, not just the total.
- Local-model inference is unmaintained in the repository; serve with vLLM behind an OpenAI-compatible API.
- Self-run agentic coding needs task-specific Docker images that can total about 150GB.
- Scores are per-task averages; a strong category total can hide regressions elsewhere.

**Cadence.** Monthly question refresh; leaderboard updates when the maintainers run new models.

**Cite as.** `LiveBench, <category>, task set <date> (livebench.ai, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. Repository README (pushed 2026-10-18) states 18 tasks across 6 categories; changelog entries run through 2026-01-08.

### Hugging Face Open LLM Leaderboard v2

- **URL:** <https://huggingface.co/spaces/open-llm-leaderboard/open_llm_leaderboard>
- **Publisher:** Hugging Face
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** Six fixed benchmarks for open-weight models: IFEval, BBH, MATH-Hard, GPQA, MuSR and MMLU-Pro, run with fixed few-shot settings by the leaderboard harness.

**Coverage.** Open-weight models only, which matches the EZLM catalogue. Per-model result JSON is public.

**Caveats.**
- Frozen: the results dataset (open-llm-leaderboard/results) was last modified 2025-03-15, so nothing from 2026 is scored.
- Fixed prompts and few-shot counts reward prompt shape over real use; several benchmarks are saturated.
- No tool calling, agentic or long-context evaluation.

**Cadence.** Frozen since March 2025.

**Cite as.** `Open LLM Leaderboard v2, <benchmark> (huggingface.co, accessed 2026-11-06; frozen 2025-03-15)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. Results dataset lastModified 2025-03-15; a result JSON contains leaderboard_ifeval, leaderboard_bbh, leaderboard_math_hard, leaderboard_gpqa, leaderboard_musr and leaderboard_mmlu_pro tasks.

## Coding

### SWE-bench

- **URL:** <https://www.swebench.com>
- **Publisher:** SWE-bench team
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** Percentage of real GitHub issues resolved, judged by each repository's own tests. Boards cover Verified (500 instances), Lite (300), the original set (2,294 instances from 12 Python repositories), Multilingual and Multimodal.

**Coverage.** Has an open-weights quick select and marks runs 'performed or directly checked by the SWE-bench team' versus self-reported entries. The Bash Only view puts every model in the same mini-SWE-agent environment.

**Caveats.**
- Verified is Python-only; the Multilingual and Multimodal boards differ in size and harness.
- The scaffold and tool access dominate results; compare models within one view, for example Bash Only.
- Contamination: issues predate most training cutoffs; use a time-controlled source for brand-new models.
- Self-reported entries may use different prompts, retries and cost limits.

**Cadence.** Updated as new runs arrive; each entry carries its run date.

**Cite as.** `SWE-bench Verified, bash-only view (swebench.com, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. Page lists 2,294 original, 500 Verified bash-only and 300 Lite instances, and a team-checked marker.

### SWE-rebench v2

- **URL:** <https://swe-rebench.com>
- **Publisher:** SWE-rebench (Nebius)
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** SWE-style issue resolution on tasks mined continuously from recent pull requests, with a time-window filter so a model can be scored only on tasks created after its training cutoff.

**Coverage.** Public leaderboard plus paper and data. The SWE-rebench/SWE-rebench-V2 repository (pushed 2026-10-27) builds tasks and Docker images from pull requests and evaluates them with per-task log parsers.

**Caveats.**
- v2 tasks are not comparable with v1 or with SWE-bench; name the version and the time window.
- Task construction is automated (LLM-written PR descriptions, generated Dockerfiles), so per-task quality varies.
- Fewer models than SWE-bench, and open-weight coverage is thinner.

**Cadence.** Continuous task mining; leaderboard refreshed as runs are submitted.

**Cite as.** `SWE-rebench v2, window <start>-<end> (swe-rebench.com, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. Site is labelled SWE-rebench v2 and exposes a time-window control (15/05/2026-01/07/2026 shown); repository pushed 2026-10-27.

### Aider polyglot leaderboard

- **URL:** <https://aider.chat/docs/leaderboards/>
- **Publisher:** Aider
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** Percentage of 225 Exercism exercises solved across C++, Go, Java, JavaScript, Python and Rust, with a second attempt allowed (pass rate 2 is the headline), plus cost per run and edit-format compliance.

**Coverage.** Includes open-weight entries such as DeepSeek-V3.2, Kimi K2, gpt-oss-120b and Qwen3. Each run records model, edit format, commit hash and reasoning effort.

**Caveats.**
- The newest run dates visible on the page top out at 2025-10-03, so the board is roughly a year behind November 2026 releases; check the run date before citing.
- Only 225 tasks with two attempts; differences of a few points are noise.
- Results depend on edit format (diff versus whole) and harness, so compare within one format.

**Cadence.** Irregular, maintainer-run.

**Cite as.** `Aider polyglot leaderboard, run dated <date> (aider.chat, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. Page states 225 exercises across six languages; newest visible run date is 2025-10-03.

### LiveCodeBench

- **URL:** <https://livecodebench.github.io>
- **Publisher:** LiveCodeBench team (UC Berkeley, MIT, Cornell)
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** Contamination-free code evaluation on problems from LeetCode, AtCoder and Codeforces, annotated with release dates. Covers code generation, self-repair, code execution and test-output prediction.

**Coverage.** 300+ problems released May 2023 to February 2024 at publication, with rolling updates. Release dates let you score a model only on problems published after its cutoff.

**Caveats.**
- Competitive-programming style and Python-heavy; it does not measure repository-scale work.
- Easy subsets are saturated; check which version and time window a number comes from.
- Leaderboard maintenance varies; prefer the released data and code for reproducibility.

**Cadence.** Rolling problem collection; leaderboard updated irregularly.

**Cite as.** `LiveCodeBench, <version and window> (livecodebench.github.io, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. Page describes 300+ problems from May 2023 to February 2024 with release-date annotations.

### EvalPlus (HumanEval+ / MBPP+)

- **URL:** <https://evalplus.github.io/leaderboard.html>
- **Publisher:** EvalPlus team (UIUC, HKUST, Salesforce)
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** pass@1 on HumanEval+ (164 problems with 80x more test cases than HumanEval) and MBPP+ (378 tasks with 35x more tests), plus EvalPerf for the runtime efficiency of generated code. The stricter tests catch solutions that pass the original weak tests but are wrong.

**Coverage.** Leaderboard of open and API models. The datasets are used as the code benchmark in Meta Llama, Qwen-Coder and DeepSeek-Coder reports, which makes cross-checking vendor claims easy.

**Caveats.**
- Function-level, Python-only problems; it does not measure repository-scale or multi-file work.
- Saturated: strong models exceed 90, so it mainly separates small models.
- HumanEval+ scores are lower than HumanEval scores; never compare the two directly.
- Test suites are still finite, and a model can be tuned to the public tests.
- The board updates irregularly (package v0.3.1 was 2024-10-20), so check the run date.

**Cadence.** Dataset versions occasionally (v0.3.1 on 2024-10-20); leaderboard runs irregular.

**Cite as.** `EvalPlus HumanEval+, pass@1, <date> (evalplus.github.io, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06 for the leaderboard. README states 80x more tests than HumanEval, 35x more than MBPP, MBPP+ trimmed to 378 tasks, and v0.3.1 dated 2024-10-20.

### BigCodeBench

- **URL:** <https://bigcode-bench.github.io/>
- **Publisher:** BigCode project (Hugging Face, ServiceNow)
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** Calibrated pass@1 on 1,140 practical programming tasks that require calling diverse libraries, with a Hard subset of about 150 user-facing tasks. Two modes: Complete (code completion from a structured docstring) and Instruct (generation from a brief natural-language instruction).

**Coverage.** Hugging Face leaderboard with open-weight flags: open weights and open data, or open weights with a closed base. Includes small and mid-size open models, which is useful for the lower tiers.

**Caveats.**
- Results depend on installed library versions; the harness pins them, but entries can drift.
- Complete and Instruct scores are not comparable; state which mode you mean.
- Calibrated pass@1 is a rescaled number, not raw pass@1.
- Tasks were written by humans and are public, so contamination is possible for newer models.
- It rewards breadth of library knowledge, not algorithmic depth.

**Cadence.** Leaderboard updated as models are submitted; dataset is versioned.

**Cite as.** `BigCodeBench, <mode>, calibrated pass@1, <date> (bigcode-bench.github.io, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. Page states 1,140 tasks, a Hard subset of about 150, calibrated pass@1 with greedy decoding, and the Complete vs Instruct split.

## Agentic and tool use

### Terminal-Bench 4.0

- **URL:** <https://www.tbench.ai>
- **Publisher:** Stanford, Laude Institute and Harbor
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** Resolution rate on 66 end-to-end terminal tasks, run through the Harbor harness with a named agent scaffold. The leaderboard reports resolution rate with a 95% confidence interval, cost and tokens per run.

**Coverage.** Model-by-agent matrix rather than a model-only board. Tasks include GPU workloads; the dataset is versioned as terminal-bench/terminal-bench@4.0.0 and Artificial Analysis uses the same 66-task Terminal-Bench 4.0 at 10% of its index.

**Caveats.**
- The agent scaffold is part of the result; a model's rank changes with the harness (Claude Code, mini-SWE-agent and others).
- 66 tasks give wide confidence intervals; do not treat small gaps as real.
- Expensive and GPU-dependent; the sandbox choice (Modal, Daytona) affects reproducibility.
- Version 4.0 results are not comparable with 2.x results.

**Cadence.** Versioned releases (2.0, 2.1, 3.0, 4.0); leaderboard updated as runs are uploaded.

**Cite as.** `Terminal-Bench 4.0, agent <name> (tbench.ai, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. Page titles Terminal-Bench 4.0 and states 95% CI; Hugging Face dataset harborframework/terminal-bench has 66 rows; docs reference terminal-bench@4.0.0.

### Berkeley Function Calling Leaderboard V4

- **URL:** <https://gorilla.cs.berkeley.edu/leaderboard.html>
- **Publisher:** Gorilla LLM team, UC Berkeley
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** Function-call accuracy scored by AST matching, plus multi-turn (v3) and agentic (v4) categories, irrelevance and relevance detection, REST API, SQL, Java and JavaScript. Overall accuracy is the unweighted average of the sub-categories, reported with cost and latency.

**Coverage.** Separates native function-calling models (FC) from prompt-based ones and runs a format-sensitivity test for prompt models. Model responses and evaluation code are published (bfcl-eval==2025.12.17, commit f7cf735).

**Caveats.**
- The unweighted average hides a model that fails one category badly, for example irrelevance or multi-turn.
- Last updated 2026-04-12, so late-2026 releases may be missing.
- English-centric, and the agentic categories depend on the harness.

**Cadence.** Periodic; last update 2026-04-12.

**Cite as.** `BFCL V4, overall accuracy, updated 2026-04-12 (gorilla.cs.berkeley.edu, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. Page shows BFCL V4, last updated 2026-04-12, commit f7cf735 and evaluation package 2025.12.17.

### tau-bench 2.0

- **URL:** <https://taubench.com>
- **Publisher:** Sierra
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** Task success for a tool-using agent talking to a simulated user across airline, retail, telecom and banking_knowledge domains, plus the newer intake (tau-Entity) domain with 200+ tasks. tau-bench 2.0 adds six languages (English, Spanish, Portuguese, Hindi, Chinese, Korean) and full-duplex voice.

**Coverage.** Text and voice leaderboards. The original paper introduced pass^k to measure reliability across repeated trials.

**Caveats.**
- The simulated user (persona, model, language) is part of the harness; results are not model-only.
- Voice runs require realtime APIs and are limited to providers that offer them.
- Domain-specific: a strong tau-bench score does not imply good coding or general chat.

**Cadence.** Irregular; v2 leaderboard.

**Cite as.** `tau-bench 2.0, <domain>, <language> (taubench.com, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. tau2-bench README describes six languages, voice, the intake domain (200+ tasks) and banking_knowledge; taubench.com is the linked live leaderboard.

## Knowledge and science

### MMLU-Pro leaderboard

- **URL:** <https://huggingface.co/spaces/TIGER-Lab/MMLU-Pro>
- **Publisher:** TIGER-Lab (University of Waterloo)
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** Accuracy on 12,032 test questions (plus 70 validation) across 14 disciplines, each with 10 answer options instead of MMLU's 4, so guessing is harder. The reference protocol is 5-shot chain-of-thought and answer-letter extraction; the leaderboard reports overall and per-category accuracy.

**Coverage.** Open-weight and API models, with per-category columns (math, physics, chemistry, law, engineering, health, psychology, business, biology, philosophy, computer science, history, economics, other). The dataset card lists 12,032 test and 70 validation rows, MIT licence.

**Caveats.**
- Most rows are submitter-run; prompting and answer extraction differ between entries, so a 1-2 point gap is not meaningful.
- The card's 2026-01-18 note fixed a leading-space bug in answer options for chemistry and physics that could be exploited as a shortcut, so pre-2026 scores are not strictly comparable.
- Scores are saturating near 90 for frontier models; use it to separate small and mid-size models, not to rank the top few.
- It measures knowledge recall more than reasoning; pair it with GPQA or LiveBench for reasoning claims.
- Per-category scores vary by more than 20 points for the same model, so an overall number hides weak domains.

**Cadence.** Updated when the maintainers add models; the dataset card records changes (latest 2026-03-11).

**Cite as.** `MMLU-Pro, 5-shot CoT, <score> (huggingface.co/spaces/TIGER-Lab/MMLU-Pro, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06 for the space and the dataset card. Card states 12,032 test / 70 validation questions, 14 categories, MIT licence, and the 2026-01-18 answer-option fix.

### GPQA / GPQA Diamond

- **URL:** <https://github.com/idavidrein/gpqa>
- **Publisher:** Rein et al. (NYU, Anthropic, Cohere)
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** Accuracy on 448 graduate-level, 'Google-proof' multiple-choice questions in physics, chemistry and biology, written by domain PhDs. The Diamond subset (198 questions) keeps only items that experts answer correctly and non-experts with unrestricted web access mostly miss.

**Coverage.** No central leaderboard. The repository ships the data, a canary string and baseline scripts; results are reported in papers and vendor model cards, and aggregators such as Artificial Analysis and Epoch AI re-run it.

**Caveats.**
- There is no canonical harness, so numbers come from whoever ran it; prompting (0-shot vs 5-shot CoT) moves scores by several points.
- Diamond is only 198 questions, so the sampling error is roughly +/-3 points; do not read small gaps as real.
- The dataset is deliberately hard to access (password-protected zip, canary string) but is still widely trained on, so contamination is possible.
- Usually cited as GPQA Diamond only; say which subset and shot count you mean.
- It is a science-reasoning test, not a coding or agentic test.

**Cadence.** Static dataset (2023, arXiv 2311.12022); new results arrive through papers and aggregators.

**Cite as.** `GPQA Diamond, <n>-shot, as reported by <runner> (github.com/idavidrein/gpqa, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. README describes the password-protected dataset zip, the canary string, and baseline scripts for 0-shot, 5-shot, CoT and retrieval modes. The Hugging Face mirror is gated.

## Long context

### RULER long-context suite

- **URL:** <https://github.com/NVIDIA/RULER>
- **Publisher:** NVIDIA
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** Synthetic long-context tasks in 4 categories (retrieval, multi-hop tracing, aggregation, question answering) across 13 task types at lengths from 4K to 128K. It reports an 'effective context length': the longest length at which a model still beats Llama-2-7B's 4K score, which is far more informative than the advertised window.

**Coverage.** The paper benchmarks 17 open-source models; the code can be run on any model and the results table is public. It is the standard way to check whether a GGUF's claimed context is usable.

**Caveats.**
- Synthetic tasks, so it understates or overstates real document work; pair it with a long-document QA set.
- The effective-length threshold is a qualitative choice (Llama-2-7B at 4K); cite the threshold when you quote a number.
- The published table is from 2024-2025 models; newer models need a fresh run, and many catalog claims are extrapolations.
- Scores fall sharply past 32K for most models, which matters because EZLM ships long advertised windows.
- It does not cover long-context coding or agentic memory.

**Cadence.** Code and paper are stable; the results table is updated occasionally (Qwen3 rows added from the Qwen3 report).

**Cite as.** `RULER, effective context length, <model> (github.com/NVIDIA/RULER, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. README states 4 task categories, 13 tasks, 17 open-source models, and defines effective length against the Llama-2-7B 4K threshold; the table covers 4K-128K.

## Vision

### MMMU

- **URL:** <https://mmmu-benchmark.github.io>
- **Publisher:** MMMU team
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** Accuracy on 11.5K college-level multimodal questions across six disciplines and 30 subjects, using 30 image types such as charts, diagrams, maps, tables, chemical structures and music sheets. Tests perception, subject knowledge and reasoning.

**Coverage.** Leaderboard of vision-language models, with MMMU-Pro as a harder variant; open-weight VLMs are included.

**Caveats.**
- Leaderboard entries are often vendor-reported with different prompting; there is no single fixed harness.
- Multiple-choice format invites answer-pattern effects.
- Vision capability only; it says nothing about text-only quality or tool calling.

**Cadence.** Periodic; MMMU-Pro added later.

**Cite as.** `MMMU, <split> (mmmu-benchmark.github.io, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. Page states 11.5K questions, six disciplines, 30 subjects and 30 image types.

## Aggregators and trackers

### Epoch AI Benchmarking Hub

- **URL:** <https://epoch.ai/benchmarks>
- **Publisher:** Epoch AI
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** Database of benchmark results with the Epoch Capabilities Index (ECI), a single latent capability scale combining many benchmarks. Tracks 97 benchmarks and 425 models with pricing, architecture and training data.

**Coverage.** Mixes Epoch-run evaluations with externally collected results, and publishes benchmark reviews that rate each benchmark (for example Verified or Flawed). The ECI moved to a Bayesian model on 2026-11-04.

**Caveats.**
- ECI is a latent scale, not a percentage; do not mix it with benchmark accuracy.
- External entries may be self-reported; check the source flag on each result.
- Frontier-oriented: fewer sub-10B and quantized local models than the other sources.
- Epoch's own reviews flag several popular benchmarks as flawed.

**Cadence.** Continuous; page updated 2026-11-06.

**Cite as.** `Epoch AI Benchmarking Hub, <benchmark> (epoch.ai, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. Page states 425 models tracked, 97 benchmarks tracked, ECI top 169 (GPT-6 Astra), updated Nov 6 2026.

### OpenCompass CompassRank

- **URL:** <https://rank.opencompass.org.cn/home>
- **Publisher:** Shanghai AI Laboratory
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** Aggregated scores across 100+ datasets covering knowledge, reasoning, coding, science, language, long-context and safety. CompassRank normalizes per-dataset results into a comparable ranking.

**Coverage.** Supports OpenAI, Anthropic, Gemini, Qwen, GLM and DeepSeek models, with strong coverage of Chinese open-weight models that Western boards miss. Repository pushed 2026-11-06.

**Caveats.**
- CompassRank is a normalized aggregate; its numbers are not raw benchmark accuracies.
- Many entries are vendor-submitted; check whether a result is official, reproduced or self-reported.
- Mixed English and Chinese evaluation; prompt language affects scores.

**Cadence.** Continuous.

**Cite as.** `OpenCompass CompassRank, <dataset> (rank.opencompass.org.cn, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. Repository description lists 100+ datasets and the supported model families; repository pushed 2026-11-06.

### HELM (Holistic Evaluation of Language Models)

- **URL:** <https://crfm.stanford.edu/helm/>
- **Publisher:** Stanford CRFM
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** Standardized, reproducible evaluation across scenario families with multiple metrics: accuracy, calibration, robustness, fairness, bias, toxicity and efficiency.

**Coverage.** Open framework and public leaderboards; runs are documented with prompts and harness versions, and open-weight models are included.

**Caveats.**
- Leaderboards update slowly relative to vendor releases; check the run date.
- Compute-heavy, and coverage of sub-10B models is thin.
- Scenario versions change, so numbers from different HELM releases are not always comparable.

**Cadence.** Periodic releases.

**Cite as.** `HELM, <leaderboard>, run dated <date> (crfm.stanford.edu/helm, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. Repository description confirms the CRFM framework; repository pushed 2026-09-01.

### Vals AI benchmarks

- **URL:** <https://vals.ai/>
- **Publisher:** Vals AI
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** A composite Vals Index plus domain suites (finance, legal, software engineering, cybersecurity, immigration, mental health) run in-house on economically valuable tasks, reported with cost, latency and token usage alongside accuracy.

**Coverage.** Frontier proprietary models and a growing set of open-weight ones (Kimi, DeepSeek, GLM, Qwen, MiniMax, Nemotron appear in the November 2026 index).

**Caveats.**
- Several suites are built in-house with limited external replication, so a single number is hard to audit.
- The Vals Index weighting is editorial.
- Coverage of small open-weight models (under 30B) is thin.
- Some reports are produced with vendor involvement; check who commissioned the run.
- Good for domain-shaped questions, not a substitute for a public reproducible benchmark.

**Cadence.** Continuous; model evaluations posted most days (latest 2026-11-05).

**Cite as.** `Vals AI, Vals Index v<version>, <date> (vals.ai, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. Page shows a Vals Index dated 2026-11-05 with named models, and states that Vals runs its own evaluations and builds many benchmarks in-house.

## Commentary and methodology

### Simon Willison, evals tag

- **URL:** <https://simonwillison.net/tags/evals/>
- **Publisher:** Simon Willison
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** Hands-on write-ups of eval methodology, harnesses (smevals), prompt-level comparisons and specific model releases, usually with the prompts and raw outputs attached.

**Coverage.** 46 posts tagged 'evals' as of 2026-11-06, latest 2026-09-21; strong on practical pitfalls such as prompt sensitivity and benchmark contamination.

**Caveats.**
- Single-author commentary, not a leaderboard; there are no comparable scores across models.
- Coverage follows the author's interests, so many small open models never appear.
- Use it to sanity-check a claim or to borrow a methodology, not as a score source.
- Posts age; a 2024 comparison may not reflect a 2026 model refresh.

**Cadence.** Irregular, several posts a month.

**Cite as.** `Simon Willison, '<post title>' (simonwillison.net, <date>, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. Tag page lists 46 posts, most recent 2026-09-21, and describes the tag as evaluations and benchmarks of AI systems.

### Interconnects (Nathan Lambert)

- **URL:** <https://www.interconnects.ai/>
- **Publisher:** Nathan Lambert
- **Access date:** 2026-11-06 (HTTP 200 on that date)

**What it measures.** Analysis of open-weight model releases, benchmark claims and the open-model ecosystem, written by a researcher who trains models; useful for judging whether a vendor benchmark number is plausible and how a release compares with its predecessor.

**Coverage.** Open-weight and frontier releases, with post-mortems on evals and on the gap between benchmark scores and real use. About 87,000 subscribers.

**Caveats.**
- Commentary and analysis, not reproducible measurement; numbers are quoted from elsewhere.
- Publication is irregular and some posts are paywalled.
- Author has ties to the open-model community; read alongside a primary source.
- Not a place to get a score for a specific small GGUF model.

**Cadence.** Irregular, a few posts a month.

**Cite as.** `Nathan Lambert, '<post title>', Interconnects (interconnects.ai, <date>, accessed 2026-11-06)`

**Checked on 2026-11-06.** GET 200 on 2026-11-06. Page describes the newsletter and states 87,000+ subscribers.

## Gaps this catalogue does not fill

- **Small models (under 4B) are thinly covered.** Artificial Analysis, LMArena and LiveBench all concentrate on models people chat with. For a 4 GB tier pick, BigCodeBench, EvalPlus and MMLU-Pro per-category rows are often the only independent numbers, and sometimes there is none, which is what `ESTIMATE` is for.
- **Quantisation quality is not measured anywhere.** Every source here scores full-precision or vendor-quantised weights. A Q4_K_M of a 30B model is not the model that was benchmarked, and no public source quantifies the gap per quant level. Say so in `notes` rather than implying the benchmark number transfers.
- **Local inference speed is not measured.** Tokens per second on the user's GPU is a property of the machine, not the model. Artificial Analysis reports speed for hosted endpoints only.
- **No source covers every model in one place.** Cross-check at least two: a composite index for the headline, a task benchmark for the claim that matters to the tier, and a vendor-independent comment for plausibility.

## Provenance

Rows were checked by GET on 2026-11-06 and the `verified` field records what was actually read. `arena.ai` returned 403 to a scripted GET (Cloudflare), so the LMArena numbers were read from the published leaderboard snapshot instead; that is noted in the row. Everything else returned 200.
