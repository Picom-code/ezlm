# T06/a12: metrics names and health/readiness endpoints

Slice: everything EZLM parses for progress or status, and what changed between
the pinned tag `b11960` and the latest release.

Tags compared: `b11960` (pin), `b12144` (newest build by date at the time of
the brief; `b12167` appeared at 09:45Z today and was not fetched), and
`v0.9.0` (`releases/latest`, 2026-11-04). Sources are raw.githubusercontent
GETs of `tools/server/*.cpp`, `common/arg.cpp` and `common/common.h`.

## What EZLM parses

| What | EZLM usage | Endpoint | Kind |
|---|---|---|---|
| Generation speed, tokens generated, requests processing, requests queued | `internal/app/live.go:77-109` (`metrics`), names at `:99-107` | `GET /metrics` | Prometheus text |
| Model load / readiness | `internal/serve/client.go:26-36` (`Healthy`) | `GET /health` | status code only |
| "This server is ours" | `internal/serve/client.go:40-67` (`Ours`) | `GET /v1/models` | JSON `data[].id` |
| Prompt progress and phase | `internal/app/live.go:124-162` (`slots`) | `GET /slots` | JSON |
| Context, slots, template, build | `internal/serve/props.go:42-50` | `GET /props` | JSON |

The flag that turns the first one on: `--metrics` at `internal/serve/config.go:125`.

## Metrics names

`tools/server/server-task.cpp:1567-1661` (`server_task_result_metrics::to_metrics`)
emits every series as `llamacpp:<name> <value>` with `# HELP` / `# TYPE` lines
in front. The four names EZLM switches on all exist at all three tags:

| Name | b11960 | b12144 | v0.9.0 | EZLM line |
|---|---|---|---|---|
| `llamacpp:predicted_tokens_seconds` (gauge) | yes | yes | yes | `live.go:99` |
| `llamacpp:requests_processing` (gauge) | yes | yes | yes | `live.go:101` |
| `llamacpp:tokens_predicted_total` (counter) | yes | yes | yes | `live.go:103` |
| `llamacpp:requests_deferred` (gauge) | yes | yes | yes | `live.go:105` |

Evidence: `diff b11960:server-task.cpp v0.9.0:server-task.cpp` is empty, and
`b12144` differs from `b11960` in ten lines, all in checkpoint/state handling
(`server-task.cpp:1770`, `:1805-1806`), none in the metric tables.

Additions since b11960 (not read by EZLM, harmless): the labelled series
`llamacpp:spec_decode_num_accepted_tokens_per_pos_total{position="N"}` and the
speculative-decoding counters. EZLM's parser requires exactly two
whitespace-separated fields (`live.go:93-96`), so the labelled series is
skipped rather than mis-read. That is correct today but is the one place where
a future unlabelled rename would silently produce zeros.

`--metrics` still exists with the same meaning at both tags
(`common/arg.cpp:3641` b11960, `:3580` v0.9.0), default off
(`common/common.h:690`), and `/metrics` is still registered at
`tools/server/server.cpp:173` and still requires the API key (it is not in the
public-endpoint set at `tools/server/server-http.cpp:254-257`; EZLM sends the
Bearer key at `live.go:81`).

## Health and readiness

`GET /health` and `/v1/health` are public (`server-http.cpp:56-57`, `:254-257`)
and are handled by `server-context.cpp:5716-5727`: 200 with `{"status":"ok"}`.
While the model is loading, `middleware_server_state` (`server-http.cpp:309-322`)
answers 503 `{"error":{"message":"Loading model","type":"unavailable_error","code":503}}`
for every non-frontend path, including `/health`. So `/health` is a readiness
probe, not a liveness probe.

EZLM depends on exactly that: `Healthy` returns true only on 200 and then calls
`Ours` (`/v1/models` with the key, matching `model_id`). `WaitHealthy`
(`client.go:71-109`) polls it, and a non-200 means the dashboard shows
"starting" (`internal/app/live.go:58-63`). The 200/503 contract, the
`{"status":"ok"}` body and the 503 body shape are identical at b11960, b12144
and v0.9.0 (`server-http.cpp` is byte-identical between b11960 and v0.9.0).

## Slots (prompt progress)

`server-context.cpp:809-840` is the slot object. EZLM reads `is_processing`,
`n_prompt_tokens`, `n_prompt_tokens_processed`, `n_prompt_tokens_cache` and
`next_token[0].n_decoded` (`live.go:134-142`). All five exist with the same
names and the same meaning at all three tags; the block diffed clean between
b11960 and v0.9.0.

`/slots` is on by default (`common/common.h:688`, `endpoint_slots = true`), and
EZLM does not pass `--slots`. It works, but the dashboard's progress bars rest
on a default rather than on an explicit flag: if a release flips that default,
prompt progress disappears with no error (`slots` returns silently).

## Props

`/props` at v0.9.0 adds one key, `voices` (`server-context.cpp:5707-5709`),
for text-to-speech models. Every key EZLM decodes
(`default_generation_settings.n_ctx`, `total_slots`, `chat_template`,
`chat_template_caps`, `modalities.vision`, `build_info`) is still present with
the same name at b11960, b12144 and v0.9.0. Additive only.

## Verdict for the upgrade

Nothing in this slice breaks on the move from b11960 to v0.9.0. The metric
names, the `/health` readiness semantics, the `/slots` schema and the `/props`
keys EZLM reads are unchanged. The three things to watch, in order:

1. `/slots` is used without `--slots`; the default is the only guarantee.
2. The dashboard shows zeros, with no error, if `/metrics` starts failing
   (`metrics` returns early on error, `live.go:82-85`).
3. A rename of a metric would silently zero one field, not fail a build; the
   test `internal/engine/metrics_names_test.go` guards the four names against
   the captured per-tag output.
