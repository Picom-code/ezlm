// Package bench runs a quick, real-request check of a deployed model:
// speed, reasoning, tool calling, instruction following, and recall.
package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type Result struct {
	Name   string  `json:"name"`
	Pass   bool    `json:"pass"`
	Detail string  `json:"detail"`
	Value  float64 `json:"value,omitempty"` // tok/s for speed tests
	Secs   float64 `json:"secs"`
}

type Target struct {
	BaseURL string // .../v1
	Key     string
	Model   string
}

type message struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Reasoning string `json:"reasoning_content,omitempty"`
	ToolCalls []struct {
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls,omitempty"`
}

type reply struct {
	Choices []struct {
		Message      message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Timings struct {
		PromptPerSecond    float64 `json:"prompt_per_second"`
		PredictedPerSecond float64 `json:"predicted_per_second"`
		PredictedN         int     `json:"predicted_n"`
		PromptN            int     `json:"prompt_n"`
	} `json:"timings"`
}

func (t Target) chat(ctx context.Context, body map[string]any) (*reply, time.Duration, error) {
	body["model"] = t.Model
	b, _ := json.Marshal(body)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", t.BaseURL+"/chat/completions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.Key)
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, 0, fmt.Errorf("server said %s", resp.Status)
	}
	var r reply
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, 0, err
	}
	if len(r.Choices) == 0 {
		return nil, 0, fmt.Errorf("no answer")
	}
	return &r, time.Since(start), nil
}

func user(s string) []map[string]string { return []map[string]string{{"role": "user", "content": s}} }

// Run executes every test, reporting each result as it finishes.
func Run(ctx context.Context, t Target, report func(Result)) []Result {
	tests := []func(context.Context, Target) Result{speed, reasoning, toolCall, jsonFormat, recall}
	var out []Result
	for _, test := range tests {
		r := test(ctx, t)
		out = append(out, r)
		if report != nil {
			report(r)
		}
	}
	return out
}

func speed(ctx context.Context, t Target) Result {
	r, d, err := t.chat(ctx, map[string]any{
		"messages":   user("Write a 300-word story about a lighthouse keeper. Just the story."),
		"max_tokens": 600,
	})
	res := Result{Name: "Speed", Secs: d.Seconds()}
	if err != nil {
		res.Detail = err.Error()
		return res
	}
	res.Value = r.Timings.PredictedPerSecond
	res.Pass = res.Value >= 5
	res.Detail = fmt.Sprintf("%.1f tokens/s writing (%d tokens)", r.Timings.PredictedPerSecond, r.Timings.PredictedN)
	return res
}

// ask is chat with a second chance: a thinking model that thinks past the
// limit without answering is asked again with thinking off, the same way
// EZLM's chat offers "Answer without thinking".
func (t Target) ask(ctx context.Context, body map[string]any) (*reply, time.Duration, bool, error) {
	r, d, err := t.chat(ctx, body)
	if err != nil || r.Choices[0].Message.Content != "" || r.Choices[0].FinishReason != "length" || r.Choices[0].Message.Reasoning == "" {
		return r, d, false, err
	}
	again := map[string]any{}
	for k, v := range body {
		again[k] = v
	}
	again["chat_template_kwargs"] = map[string]any{"enable_thinking": false}
	again["reasoning_effort"] = "low"
	r2, d2, err := t.chat(ctx, again)
	if err != nil {
		return r, d, false, nil // keep the first answer (and its explanation)
	}
	return r2, d + d2, true, nil
}

const thinkNote = " (with thinking off: it thought past the limit first)"

func reasoning(ctx context.Context, t Target) Result {
	r, d, noThink, err := t.ask(ctx, map[string]any{
		"messages":   user("A bat and a ball cost $1.10 in total. The bat costs $1.00 more than the ball. How many cents does the ball cost? Reply with just the number."),
		"max_tokens": 4096,
	})
	res := Result{Name: "Reasoning", Secs: d.Seconds()}
	if err != nil {
		res.Detail = err.Error()
		return res
	}
	ans := strings.TrimSpace(r.Choices[0].Message.Content)
	if ans == "" && r.Choices[0].FinishReason == "length" && r.Choices[0].Message.Reasoning != "" {
		// Thinking models can think past the limit on a small quant or a slow
		// machine; that's not a wrong answer, but it is worth knowing.
		res.Detail = fmt.Sprintf("kept thinking for %d tokens and never answered (in Chat, \"Answer without thinking\" helps)", r.Timings.PredictedN)
		return res
	}
	res.Pass = isFive(ans)
	res.Detail = "bat-and-ball: answered " + clip(ans, 40) + map[bool]string{true: " (correct: 5)", false: " (expected 5)"}[res.Pass]
	if noThink {
		res.Detail += thinkNote
	}
	return res
}

var (
	fiveRx = regexp.MustCompile(`(^|[^\d.])(0?\.05|5)(\D|$)|\bfive\b`)
	tenRx  = regexp.MustCompile(`(^|[^\d.])(0?\.10?|10)(\D|$)|\bten\b`)
)

// isFive: the bat-and-ball answer is 5 cents, however it's written ("5",
// "Five", "$0.05"); the classic wrong answer is 10.
func isFive(ans string) bool {
	low := strings.ToLower(ans)
	return fiveRx.MatchString(low) && !tenRx.MatchString(low)
}

func toolCall(ctx context.Context, t Target) Result {
	r, d, err := t.chat(ctx, map[string]any{
		"messages":   user("What's the weather in Austin right now?"),
		"max_tokens": 2048,
		"tools": []map[string]any{{
			"type": "function",
			"function": map[string]any{
				"name":        "get_weather",
				"description": "Get the current weather for a city",
				"parameters": map[string]any{
					"type":       "object",
					"properties": map[string]any{"city": map[string]string{"type": "string"}},
					"required":   []string{"city"},
				},
			},
		}},
	})
	res := Result{Name: "Tool calling", Secs: d.Seconds()}
	if err != nil {
		res.Detail = err.Error()
		return res
	}
	calls := r.Choices[0].Message.ToolCalls
	if len(calls) == 0 {
		res.Detail = "didn't call the tool: " + clip(r.Choices[0].Message.Content, 60)
		return res
	}
	var args struct{ City string }
	json.Unmarshal([]byte(calls[0].Function.Arguments), &args)
	res.Pass = calls[0].Function.Name == "get_weather" && strings.Contains(strings.ToLower(args.City), "austin")
	res.Detail = fmt.Sprintf("called %s(%s)", calls[0].Function.Name, calls[0].Function.Arguments)
	return res
}

func jsonFormat(ctx context.Context, t Target) Result {
	r, d, noThink, err := t.ask(ctx, map[string]any{
		"messages":   user(`Return ONLY a JSON object with keys "name" (string) and "year" (number) for the first person to walk on the Moon. No other text.`),
		"max_tokens": 2048,
	})
	res := Result{Name: "Following instructions", Secs: d.Seconds()}
	if err != nil {
		res.Detail = err.Error()
		return res
	}
	txt := strings.TrimSpace(r.Choices[0].Message.Content)
	txt = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(txt, "```json"), "```"), "```")
	var v struct {
		Name string
		Year int
	}
	err = json.Unmarshal([]byte(strings.TrimSpace(txt)), &v)
	res.Pass = err == nil && strings.Contains(strings.ToLower(v.Name), "armstrong") && v.Year == 1969
	res.Detail = "strict JSON: " + clip(strings.TrimSpace(txt), 60)
	if noThink {
		res.Detail += thinkNote
	}
	return res
}

func recall(ctx context.Context, t Target) Result {
	// ~6k tokens of filler with one fact buried in the middle.
	var sb strings.Builder
	for i := 0; i < 120; i++ {
		if i == 61 {
			sb.WriteString("Note for later: the vault code is 7342. ")
		}
		sb.WriteString(fmt.Sprintf("Section %d covers routine maintenance of the east wing, including lighting, heating and the inventory of supplies. ", i))
	}
	r, d, err := t.chat(ctx, map[string]any{
		"messages":   user(sb.String() + "\n\nWhat is the vault code? Reply with just the number."),
		"max_tokens": 2048,
	})
	res := Result{Name: "Long-context recall", Secs: d.Seconds()}
	if err != nil {
		res.Detail = err.Error()
		return res
	}
	ans := strings.TrimSpace(r.Choices[0].Message.Content)
	res.Pass = strings.Contains(ans, "7342")
	res.Detail = fmt.Sprintf("found it in %d tokens, read at %.0f tokens/s: %s", r.Timings.PromptN, r.Timings.PromptPerSecond, clip(ans, 30))
	return res
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
