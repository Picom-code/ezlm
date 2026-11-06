package fakellm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// chunk is one streamed chat.completion.chunk, as a client reads it.
type chunk struct {
	Object  string `json:"object"`
	Choices []struct {
		FinishReason *string `json:"finish_reason"`
		Delta        struct {
			Role             string  `json:"role"`
			Content          *string `json:"content"`
			ReasoningContent string  `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Timings map[string]float64 `json:"timings"`
	Usage   map[string]any     `json:"usage"`
}

// readStream reads an SSE body into its chunks and checks it ends in [DONE].
func readStream(t *testing.T, body io.Reader) []chunk {
	t.Helper()
	var out []chunk
	done := false
	sc := bufio.NewScanner(body)
	for sc.Scan() {
		line, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		if line == "[DONE]" {
			done = true
			continue
		}
		var c chunk
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatalf("chunk %s: %v", line, err)
		}
		out = append(out, c)
	}
	if !done {
		t.Fatal("the stream didn't end with [DONE]")
	}
	return out
}

// TestAgentLoop is a tiny agent against the fake, the way OpenCode, Hermes
// and dsh drive llama-server: offer a read tool, run the call it makes on a
// real file in a folder with spaces, send the result back, and get the
// file's text as the answer.
func TestAgentLoop(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "ezlm check ws")
	os.MkdirAll(ws, 0o755)
	os.WriteFile(filepath.Join(ws, "ezlm-check.txt"), []byte("EZLM-CHECK-ABCD2345\n"), 0o644)
	var log bytes.Buffer
	fs := New(Options{Key: "ezlm-secretkey123456", Alias: "qwen3.5-4b", Thinking: true, Log: &log})
	ts := httptest.NewServer(fs)
	defer ts.Close()

	tools := []any{
		map[string]any{"type": "function", "function": map[string]any{"name": "bash", "parameters": map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}}}},
		map[string]any{"type": "function", "function": map[string]any{"name": "read_file", "parameters": map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}, "offset": map[string]any{"type": "integer"}}}}},
	}
	msgs := []any{
		map[string]any{"role": "system", "content": "You are an agent.\n<env>\n  Working directory: " + ws + "\n  Is directory a git repo: no\n</env>"},
		map[string]any{"role": "user", "content": "Use your file-reading tool to open ezlm-check.txt in the current folder. Reply with exactly the text inside it and nothing else."},
	}
	for turn := 1; turn <= 2; turn++ {
		resp := post(t, ts.URL, "ezlm-secretkey123456", map[string]any{"model": "qwen3.5-4b", "stream": true, "stream_options": map[string]any{"include_usage": true}, "tools": tools, "messages": msgs})
		if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
			t.Fatalf("turn %d: %s %s", turn, resp.Status, resp.Header.Get("Content-Type"))
		}
		chunks := readStream(t, resp.Body)
		resp.Body.Close()
		first := chunks[0].Choices[0].Delta
		if first.Role != "assistant" || first.Content != nil {
			t.Errorf("turn %d: first chunk %+v, want role and content null", turn, first)
		}
		var text, reasoning, name, args, id, finish strings.Builder
		var timings map[string]float64
		var usage map[string]any
		for _, c := range chunks {
			if c.Object != "chat.completion.chunk" {
				t.Errorf("object %q", c.Object)
			}
			if c.Timings != nil {
				timings = c.Timings
			}
			if c.Usage != nil {
				usage = c.Usage
			}
			for _, ch := range c.Choices {
				if ch.Delta.Content != nil {
					text.WriteString(*ch.Delta.Content)
				}
				reasoning.WriteString(ch.Delta.ReasoningContent)
				for _, tc := range ch.Delta.ToolCalls {
					name.WriteString(tc.Function.Name)
					args.WriteString(tc.Function.Arguments)
					id.WriteString(tc.ID)
				}
				if ch.FinishReason != nil {
					finish.WriteString(*ch.FinishReason)
				}
			}
		}
		if reasoning.String() == "" {
			t.Errorf("turn %d: no reasoning_content with Thinking", turn)
		}
		if timings["prompt_n"] == 0 || timings["predicted_per_second"] == 0 {
			t.Errorf("turn %d: timings %v", turn, timings)
		}
		if usage == nil {
			t.Errorf("turn %d: include_usage gave no usage chunk", turn)
		}
		if turn == 1 {
			if finish.String() != "tool_calls" || name.String() != "read_file" {
				t.Fatalf("turn 1: finish %q, tool %q", finish.String(), name.String())
			}
			var a struct{ Path string }
			if err := json.Unmarshal([]byte(args.String()), &a); err != nil {
				t.Fatalf("arguments %s: %v", args.String(), err)
			}
			if a.Path != filepath.Join(ws, "ezlm-check.txt") {
				t.Fatalf("path %q", a.Path)
			}
			b, err := os.ReadFile(a.Path)
			if err != nil {
				t.Fatal(err)
			}
			msgs = append(msgs,
				map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{"id": id.String(), "type": "function", "function": map[string]any{"name": "read_file", "arguments": args.String()}}}},
				map[string]any{"role": "tool", "tool_call_id": id.String(), "content": string(b)})
			continue
		}
		if finish.String() != "stop" || strings.TrimSpace(text.String()) != "EZLM-CHECK-ABCD2345" {
			t.Fatalf("turn 2: finish %q, text %q", finish.String(), text.String())
		}
	}
	reqs := fs.Requests()
	if len(reqs) != 2 || reqs[0].ToolCall == "" || reqs[1].ToolResult != "EZLM-CHECK-ABCD2345" || reqs[0].Auth != "ok" {
		t.Fatalf("requests %+v", reqs)
	}
	if strings.Contains(log.String(), "secretkey") {
		t.Fatal("the request log holds the key")
	}
	if n := strings.Count(log.String(), "\n"); n != 2 {
		t.Fatalf("log has %d lines:\n%s", n, log.String())
	}
}

func TestShellToolFallback(t *testing.T) {
	ts := httptest.NewServer(New(Options{}))
	defer ts.Close()
	resp := post(t, ts.URL, "", map[string]any{"messages": []any{
		map[string]any{"role": "system", "content": "Current working directory: `C:\\Users\\Jörg\\ezlm-check-ab12`"},
		map[string]any{"role": "user", "content": "open it"},
	}, "tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "persistent_shell", "parameters": map[string]any{"properties": map[string]any{"command": map[string]any{}}}}}}})
	var out struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content   *string `json:"content"`
				ToolCalls []struct {
					Function struct{ Name, Arguments string }
				} `json:"tool_calls"`
			}
		}
	}
	json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	c := out.Choices[0]
	if c.FinishReason != "tool_calls" || c.Message.Content != nil || len(c.Message.ToolCalls) != 1 {
		t.Fatalf("%+v", out)
	}
	if got := c.Message.ToolCalls[0].Function.Arguments; got != `{"command":"type \"C:\\Users\\Jörg\\ezlm-check-ab12\\ezlm-check.txt\""}` {
		t.Fatalf("arguments %s", got)
	}
}

func TestModelsPropsAndAuth(t *testing.T) {
	fs := New(Options{Key: "k", Alias: "qwen3.8-27b", Ctx: 32768, Levels: []string{"low", "high"}, Thinking: true})
	ts := httptest.NewServer(fs)
	defer ts.Close()
	get := func(path, key string) (*http.Response, map[string]any) {
		req, _ := http.NewRequest("GET", ts.URL+path, nil)
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var m map[string]any
		json.NewDecoder(resp.Body).Decode(&m)
		return resp, m
	}
	if resp, _ := get("/health", ""); resp.StatusCode != 200 {
		t.Errorf("/health: %d", resp.StatusCode)
	}
	if resp, _ := get("/v1/models", ""); resp.StatusCode != 401 {
		t.Errorf("/v1/models without a key: %d", resp.StatusCode)
	}
	if resp, _ := get("/v1/models", "nope"); resp.StatusCode != 401 {
		t.Errorf("/v1/models with a wrong key: %d", resp.StatusCode)
	}
	resp, m := get("/v1/models", "k")
	data, _ := m["data"].([]any)
	if resp.StatusCode != 200 || len(data) != 1 || data[0].(map[string]any)["id"] != "qwen3.8-27b" || m["models"] == nil {
		t.Errorf("/v1/models: %d %v", resp.StatusCode, m)
	}
	_, p := get("/props", "k")
	caps, _ := p["chat_template_caps"].(map[string]any)
	if p["default_generation_settings"].(map[string]any)["n_ctx"] != float64(32768) || len(caps["reasoning_effort_levels"].([]any)) != 2 ||
		!strings.Contains(p["chat_template"].(string), "enable_thinking") {
		t.Errorf("/props: %v", p)
	}
	auths := map[string]int{}
	for _, r := range fs.Requests() {
		auths[r.Auth]++
	}
	if auths["none"] != 2 || auths["wrong"] != 1 || auths["ok"] != 2 {
		t.Errorf("auth record %v", auths)
	}
}
