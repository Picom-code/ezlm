package engine

import (
	"bufio"
	"strconv"
	"strings"
	"testing"
)

// The names the dashboard parses in internal/app/live.go:metrics. They are
// llama.cpp's, not EZLM's: keep this list in step with that switch. The
// fixtures below are the Prometheus text llama-server prints from
// tools/server/server-task.cpp:to_metrics() at the given tag.
var ezlmMetrics = []string{
	"llamacpp:predicted_tokens_seconds",
	"llamacpp:requests_processing",
	"llamacpp:tokens_predicted_total",
	"llamacpp:requests_deferred",
}

// metricFixtures holds the body of GET /metrics (gauges and counters EZLM
// reads, plus neighbours) at the pinned tag, the newest build and the latest
// release. They were copied from server-task.cpp at each tag on 2026-11-06.
var metricFixtures = map[string]string{
	"b11960": `# HELP llamacpp:prompt_tokens_total Number of prompt tokens processed, excluding cached tokens
# TYPE llamacpp:prompt_tokens_total counter
llamacpp:prompt_tokens_total 0
# HELP llamacpp:tokens_predicted_total Number of generation tokens processed
# TYPE llamacpp:tokens_predicted_total counter
llamacpp:tokens_predicted_total 0
# HELP llamacpp:predicted_tokens_seconds Average generation throughput in tokens/s
# TYPE llamacpp:predicted_tokens_seconds gauge
llamacpp:predicted_tokens_seconds 0
# HELP llamacpp:requests_processing Number of requests processing
# TYPE llamacpp:requests_processing gauge
llamacpp:requests_processing 0
# HELP llamacpp:requests_deferred Number of requests deferred
# TYPE llamacpp:requests_deferred gauge
llamacpp:requests_deferred 0
`,
	"b12144": `# HELP llamacpp:prompt_tokens_total Number of prompt tokens processed, excluding cached tokens
# TYPE llamacpp:prompt_tokens_total counter
llamacpp:prompt_tokens_total 0
# HELP llamacpp:tokens_predicted_total Number of generation tokens processed
# TYPE llamacpp:tokens_predicted_total counter
llamacpp:tokens_predicted_total 0
# HELP llamacpp:predicted_tokens_seconds Average generation throughput in tokens/s
# TYPE llamacpp:predicted_tokens_seconds gauge
llamacpp:predicted_tokens_seconds 0
# HELP llamacpp:requests_processing Number of requests processing
# TYPE llamacpp:requests_processing gauge
llamacpp:requests_processing 0
# HELP llamacpp:requests_deferred Number of requests deferred
# TYPE llamacpp:requests_deferred gauge
llamacpp:requests_deferred 0
`,
	"v0.9.0": `# HELP llamacpp:prompt_tokens_total Number of prompt tokens processed, excluding cached tokens
# TYPE llamacpp:prompt_tokens_total counter
llamacpp:prompt_tokens_total 0
# HELP llamacpp:tokens_predicted_total Number of generation tokens processed
# TYPE llamacpp:tokens_predicted_total counter
llamacpp:tokens_predicted_total 0
# HELP llamacpp:predicted_tokens_seconds Average generation throughput in tokens/s
# TYPE llamacpp:predicted_tokens_seconds gauge
llamacpp:predicted_tokens_seconds 0
# HELP llamacpp:requests_processing Number of requests processing
# TYPE llamacpp:requests_processing gauge
llamacpp:requests_processing 0
# HELP llamacpp:requests_deferred Number of requests deferred
# TYPE llamacpp:requests_deferred gauge
llamacpp:requests_deferred 0
# HELP llamacpp:spec_decode_num_accepted_tokens_per_pos_total Accepted tokens per draft position
# TYPE llamacpp:spec_decode_num_accepted_tokens_per_pos_total counter
llamacpp:spec_decode_num_accepted_tokens_per_pos_total{position="0"} 0
`,
}

// parseMetrics mirrors internal/app/live.go:metrics: skip comment lines,
// require exactly two whitespace-separated fields, parse the value.
func parseMetrics(body string) map[string]float64 {
	out := map[string]float64{}
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		v, _ := strconv.ParseFloat(f[1], 64)
		out[f[0]] = v
	}
	return out
}

// TestMetricsNamesForEveryTag fails when a tag drops a metric EZLM reads, so
// the pin bump cannot silently blank the dashboard's speed, token and queue
// counters.
func TestMetricsNamesForEveryTag(t *testing.T) {
	for tag, body := range metricFixtures {
		got := parseMetrics(body)
		for _, name := range ezlmMetrics {
			if _, ok := got[name]; !ok {
				t.Errorf("%s: EZLM parses %q but the tag does not emit it", tag, name)
			}
		}
		// A labelled series must not be mistaken for one of EZLM's gauges.
		for name := range got {
			if strings.Contains(name, "{") {
				for _, want := range ezlmMetrics {
					if name == want {
						t.Errorf("%s: %q carries a label, EZLM's exact match will miss it", tag, name)
					}
				}
			}
		}
	}
}

// TestMetricsValuesParse checks the values survive the float parse EZLM does,
// including the scientific notation Prometheus uses for large counters.
func TestMetricsValuesParse(t *testing.T) {
	body := strings.Join([]string{
		"llamacpp:predicted_tokens_seconds 42.5",
		"llamacpp:tokens_predicted_total 1.234e+06",
		"llamacpp:requests_processing 2",
		"llamacpp:requests_deferred 1",
	}, "\n")
	got := parseMetrics(body)
	if got["llamacpp:predicted_tokens_seconds"] != 42.5 {
		t.Errorf("tok/s = %v", got["llamacpp:predicted_tokens_seconds"])
	}
	if uint64(got["llamacpp:tokens_predicted_total"]) != 1234000 {
		t.Errorf("tokens = %v", got["llamacpp:tokens_predicted_total"])
	}
}

// TestHealthIsReadiness pins the contract EZLM's Healthy and WaitHealthy rely
// on: /health answers 503 with {"error":{"code":503}} until the model is
// loaded and 200 {"status":"ok"} after, and /health is public while
// /v1/models needs the key. The shape comes from
// tools/server/server-http.cpp (middleware_server_state) at b11960, unchanged
// through v0.9.0.
