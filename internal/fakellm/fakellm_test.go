package fakellm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func post(t *testing.T, url, key string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", url+"/v1/chat/completions", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestToolTurnThenAnswer(t *testing.T) {
	fs := New(Options{Key: "k", Alias: "m"})
	ts := httptest.NewServer(fs)
	defer ts.Close()
	tools := []any{map[string]any{"type": "function", "function": map[string]any{"name": "read", "parameters": map[string]any{"properties": map[string]any{"filePath": map[string]any{"type": "string"}}}}}}

	resp := post(t, ts.URL, "k", map[string]any{"model": "m", "stream": true, "tools": tools, "messages": []any{
		map[string]any{"role": "system", "content": "Working directory: /tmp/ws"},
		map[string]any{"role": "user", "content": "Use your file-reading tool to open ezlm-check.txt"},
	}})
	var args strings.Builder
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := strings.TrimPrefix(sc.Text(), "data: ")
		if line == "" || line == "[DONE]" {
			continue
		}
		var c struct {
			Choices []struct {
				Delta struct {
					ToolCalls []struct {
						Function struct{ Arguments string }
					} `json:"tool_calls"`
				}
			}
		}
		json.Unmarshal([]byte(line), &c)
		for _, ch := range c.Choices {
			for _, tc := range ch.Delta.ToolCalls {
				args.WriteString(tc.Function.Arguments)
			}
		}
	}
	resp.Body.Close()
	if args.String() != `{"filePath":"/tmp/ws/ezlm-check.txt"}` {
		t.Fatalf("tool call arguments = %s", args.String())
	}

	resp = post(t, ts.URL, "k", map[string]any{"model": "m", "messages": []any{
		map[string]any{"role": "user", "content": "x"},
		map[string]any{"role": "tool", "tool_call_id": "c1", "content": "EZLM-CHECK-ABCDEFGH\n"},
	}})
	var out struct {
		Choices []struct{ Message struct{ Content string } }
	}
	json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	if out.Choices[0].Message.Content != "EZLM-CHECK-ABCDEFGH" {
		t.Fatalf("answer = %+v", out)
	}
	if r := fs.Requests(); len(r) != 2 || r[0].Reply != "tool_call" || r[1].ToolResult != "EZLM-CHECK-ABCDEFGH" {
		t.Fatalf("requests = %+v", r)
	}
}

func TestKeyAndReasoningLevels(t *testing.T) {
	ts := httptest.NewServer(New(Options{Key: "k", Levels: []string{"low", "medium", "xhigh"}}))
	defer ts.Close()
	if resp := post(t, ts.URL, "wrong", map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hi"}}}); resp.StatusCode != 401 {
		t.Fatalf("wrong key: %d", resp.StatusCode)
	}
	resp := post(t, ts.URL, "k", map[string]any{"reasoning_effort": "high", "messages": []any{map[string]any{"role": "user", "content": "hi"}}})
	var e struct{ Error struct{ Message string } }
	json.NewDecoder(resp.Body).Decode(&e)
	if resp.StatusCode != 500 || !strings.Contains(e.Error.Message, "Unexpected reasoning effort high") {
		t.Fatalf("rejected level: %d %+v", resp.StatusCode, e)
	}
	if resp := post(t, ts.URL, "k", map[string]any{"reasoning_effort": "medium", "messages": []any{map[string]any{"role": "user", "content": "Reply with exactly: EZLM OK"}}}); resp.StatusCode != 200 {
		t.Fatalf("accepted level: %d", resp.StatusCode)
	}
}

func TestPromptDir(t *testing.T) {
	for in, want := range map[string]string{
		"Working directory: /x/EZLM Workspace\n":            "/x/EZLM Workspace",
		"You run in.\nWorking directory: \"/x/y z\".\nMore": "/x/y z",
		"cwd = `C:\\Users\\Jörg\\ws`":                       "C:\\Users\\Jörg\\ws",
		"Working directory: somewhere":                      "",
	} {
		if got := promptDir(in); got != want {
			t.Errorf("promptDir(%q) = %q, want %q", in, got, want)
		}
	}
}
