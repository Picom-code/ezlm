# T06/a12 REPORT: metrics names and health/readiness endpoints

Agent a12 of T06 (llama.cpp upgrade study). Slice: metrics names and
health/readiness endpoints, and anything EZLM parses for progress or status.

Branch `agent/T06-a12`, worktree `~/ezlm-agents/work/subs/T06/a12`.

## What I did

1. Read every place EZLM reads llama.cpp status: `internal/app/live.go`
   (`metrics`, `slots`), `internal/serve/client.go` (`Healthy`, `Ours`,
   `WaitHealthy`), `internal/serve/props.go`, `internal/serve/config.go`.
2. Fetched `tools/server/server.cpp`, `server-http.cpp`, `server-context.cpp`,
   `server-task.cpp`, `common/arg.cpp` and `common/common.h` at `b11960`,
   `b12144` and `v0.9.0` (raw.githubusercontent GET) and diffed them.
3. Wrote `internal/engine/metrics_names_test.go`: the four metric names EZLM
   parses must exist in the `/metrics` output captured for each tag, values
   must survive EZLM's float parse, and `/health` must behave as a readiness
   probe (503 while loading, 200 plus `/v1/models` when ready) for
   `serve.Healthy`.
4. Wrote `agents/T06/a12/metrics.md` with the per-tag evidence tables.

## Findings

| # | Severity | Finding | Evidence |
|---|---|---|---|
| 1 | none (verified) | The four metrics EZLM parses are unchanged at b11960, b12144 and v0.9.0. | `internal/app/live.go:99-107`; `tools/server/server-task.cpp:1567-1661`; whole-file diff b11960 vs v0.9.0 empty; b12144 differs only at `server-task.cpp:1770,1805-1806` (checkpoints, not metrics). |
| 2 | none (verified) | `/health` stays a readiness probe: 503 `Loading model` until ready, then 200 `{"status":"ok"}`; public while `/v1/models` needs the key. `server-http.cpp` is byte-identical b11960 vs v0.9.0. | `tools/server/server-http.cpp:56-57,254-257,309-322`; `tools/server/server-context.cpp:5716-5727`; EZLM `internal/serve/client.go:26-36,40-67`. |
| 3 | none (verified) | `/slots` field names EZLM reads (`is_processing`, `n_prompt_tokens`, `n_prompt_tokens_processed`, `n_prompt_tokens_cache`, `next_token[0].n_decoded`) are unchanged. | `tools/server/server-context.cpp:809-840`; EZLM `internal/app/live.go:134-142`. |
| 4 | minor (risk) | EZLM reads `/slots` without passing `--slots`; it works only because the default is on. A default flip would silently remove prompt progress. | `internal/serve/config.go:114-128` has no `--slots`; `common/common.h:688` `endpoint_slots = true` at both tags. |
| 5 | minor (risk) | `/metrics` failure is silent: `metrics` returns early on any error or on a tag that drops a name, so the dashboard shows zeros rather than an error. | `internal/app/live.go:82-85`, `:93-96`. Guarded now by the new test. |
| 6 | minor (EZLM parser) | `metrics` requires exactly two fields per line and ignores `ParseFloat` errors, so a labelled or malformed series is dropped silently. Correct for the labelled `spec_decode_*_per_pos_total` series, but fragile. | `internal/app/live.go:93-97`. |
| 7 | none (additive) | v0.9.0 adds `voices` to `/props`. Every key EZLM decodes is still present. | `tools/server/server-context.cpp:5707-5709`; `internal/serve/props.go:42-50`. |
| 8 | none (additive) | Speculative-decoding metric counters were added since the pin; EZLM does not read them. | `tools/server/server-task.cpp:1590-1606`. |

No breaking change for the upgrade in this slice. The upgrade target `v0.9.0`
and the newest build `b12144` both keep the surface EZLM uses.

## What I changed

- `internal/engine/metrics_names_test.go` (new, test only). No production
  code changed; `engine.go` and `pinned.go` were not touched, and the pin stays
  `b11960`.

The test is a contract test: it encodes the metric names and the health
semantics EZLM relies on at each tag, so a pin bump that drops a name or a
`/health` status change fails `go test ./internal/engine/` instead of blanking a
dashboard field. It fails if a fixture loses one of the four names, if a name
gains a label (exact-match would miss it), or if `serve.Healthy` stops
treating 503 as "not ready" or stops confirming the server through
`/v1/models`.

## What I could not finish

- `b12167` (published 2026-11-06T09:45Z) appeared after the brief was written;
  I compared `b12144` as instructed. The metric region did not change across
  any of the three tags, so this is unlikely to matter.
- No live llama-server was started (hard rule), so the `/metrics` fixtures are
  the exact lines `to_metrics()` prints, not a capture from a running server.
  The `/health` test uses `httptest` and mirrors the middleware, not the real
  binary.
- I did not audit `internal/app/live.go`'s other status sources (log tailing,
  `StartedAt`) beyond confirming they are not llama.cpp schema.

## Next steps

1. When the pin moves, run `go test ./internal/engine/ -run Metrics` against the
   new tag and add its fixture.
2. Consider passing `--slots` explicitly in `internal/serve/config.go` so
   progress does not depend on a default (owner: T10, not this task).
3. Consider logging once when `/metrics` stops answering (owner: `internal/app`,
   which T06 must not edit).
