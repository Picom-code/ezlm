// Package fakellm is a scripted OpenAI-compatible server shaped like
// llama-server b11960, for tests (httptest.NewServer(fakellm.New(...))) and
// the e2e script (cmd/fakellm). It never runs a model.
//
// Script: when the request offers a tool whose name contains "read" and has
// a path-like parameter, and the last message isn't a tool result, it calls
// that tool on ezlm-check.txt (an absolute path when the prompt names a
// working directory). Without one, a shell tool (a name with shell, bash,
// terminal or exec and a command parameter) gets "cat ezlm-check.txt". When
// the last message is a tool result, it answers with that result. Otherwise
// it answers "Reply with exactly: X" prompts with X, and anything else with a
// fixed sentence.
package fakellm

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// Options shape the fake model.
type Options struct {
	Key      string        // Bearer key every endpoint but /health requires ("" = none)
	Alias    string        // the model id /v1/models lists (default "fake-model")
	Ctx      int           // n_ctx reported by /props (default 65536)
	Levels   []string      // chat_template_caps.reasoning_effort_levels; another reasoning_effort gets the template's HTTP 500
	Thinking bool          // stream reasoning_content first; the template mentions enable_thinking
	NoTools  bool          // never call tools (a model without tool calling)
	Log      io.Writer     // one JSON line per request (no headers, so no keys)
	Delay    time.Duration // wait this long before answering a chat request (a slow prompt read)
}

// Request is what the fake saw in one request.
type Request struct {
	At              time.Time `json:"at"`
	Path            string    `json:"path"`
	Status          int       `json:"status"`
	Auth            string    `json:"auth,omitempty"` // "ok" | "wrong" | "none"; never the key
	Model           string    `json:"model,omitempty"`
	Stream          bool      `json:"stream,omitempty"`
	Tools           []string  `json:"tools,omitempty"`
	Messages        int       `json:"messages,omitempty"`
	ToolResult      string    `json:"tool_result,omitempty"` // the last message, when it is a tool result
	ToolCall        string    `json:"tool_call,omitempty"`   // the tool called, with its arguments
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
	Reply           string    `json:"reply,omitempty"` // "text" | "tool_call" | "error"
	Text            string    `json:"text,omitempty"`  // the text answered
}

// Server is the fake; it is an http.Handler.
type Server struct {
	o    Options
	mu   sync.Mutex
	reqs []Request
	n    int
}

// New makes a fake server.
func New(o Options) *Server {
	if o.Alias == "" {
		o.Alias = "fake-model"
	}
	if o.Ctx == 0 {
		o.Ctx = 65536
	}
	return &Server{o: o}
}

// Requests lists every request so far.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.reqs)
}

func (s *Server) record(r Request) {
	r.At = time.Now()
	s.mu.Lock()
	s.reqs = append(s.reqs, r)
	s.mu.Unlock()
	if s.o.Log != nil {
		b, _ := json.Marshal(r)
		s.mu.Lock()
		s.o.Log.Write(append(b, '\n'))
		s.mu.Unlock()
	}
}

// auth says how a request authenticated, without the key itself.
func (s *Server) auth(r *http.Request) string {
	h := r.Header.Get("Authorization")
	switch {
	case h == "":
		return "none"
	case s.o.Key == "" || h == "Bearer "+s.o.Key:
		return "ok"
	default:
		return "wrong"
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	auth := s.auth(r)
	if r.URL.Path == "/health" || r.URL.Path == "/v1/health" {
		writeJSON(w, 200, map[string]any{"status": "ok"})
		s.record(Request{Path: r.URL.Path, Status: 200, Auth: auth})
		return
	}
	if s.o.Key != "" && auth != "ok" {
		writeJSON(w, 401, map[string]any{"error": map[string]any{"code": 401, "message": "Invalid API Key", "type": "authentication_error"}})
		s.record(Request{Path: r.URL.Path, Status: 401, Auth: auth, Reply: "error"})
		return
	}
	switch r.URL.Path {
	case "/v1/models", "/models":
		writeJSON(w, 200, map[string]any{
			"models": []any{map[string]any{"name": s.o.Alias, "model": s.o.Alias, "type": "model", "capabilities": []string{"completion"}}},
			"object": "list",
			"data": []any{map[string]any{
				"id": s.o.Alias, "object": "model", "created": time.Now().Unix(), "owned_by": "llamacpp",
				"meta": map[string]any{"n_ctx_train": s.o.Ctx, "n_vocab": 151936},
			}},
		})
		s.record(Request{Path: r.URL.Path, Status: 200, Auth: auth})
	case "/props", "/v1/props":
		tmpl := "{{ messages }}"
		if s.o.Thinking {
			tmpl = "{% if enable_thinking %}<think>{% endif %}{{ messages }}"
		}
		levels := s.o.Levels
		if levels == nil {
			levels = []string{}
		}
		writeJSON(w, 200, map[string]any{
			"default_generation_settings": map[string]any{"n_ctx": s.o.Ctx},
			"total_slots":                 1,
			"model_alias":                 s.o.Alias,
			"chat_template":               tmpl,
			"chat_template_caps":          map[string]any{"supports_tools": true, "supports_tool_calls": true, "supports_system_role": true, "reasoning_effort_levels": levels},
			"modalities":                  map[string]any{"vision": false},
			"build_info":                  "b11960-fakellm",
		})
		s.record(Request{Path: r.URL.Path, Status: 200, Auth: auth})
	case "/v1/chat/completions", "/chat/completions":
		s.chat(w, r, auth)
	default:
		writeJSON(w, 404, map[string]any{"error": map[string]any{"code": 404, "message": "File Not Found", "type": "not_found_error"}})
		s.record(Request{Path: r.URL.Path, Status: 404, Auth: auth, Reply: "error"})
	}
}

type message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCallID string          `json:"tool_call_id"`
}

type chatReq struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Stream   bool      `json:"stream"`
	Tools    []struct {
		Function struct {
			Name       string `json:"name"`
			Parameters struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"parameters"`
		} `json:"function"`
	} `json:"tools"`
	ReasoningEffort string `json:"reasoning_effort"`
	StreamOptions   struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options"`
}

// text is a message's content as plain text (a string or text parts).
func (m message) text() string {
	var s string
	if json.Unmarshal(m.Content, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	json.Unmarshal(m.Content, &parts)
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.Text)
	}
	return b.String()
}

var (
	pathParam  = regexp.MustCompile(`(?i)^(path|file|filepath|file_path|filename|target_file|absolute_path)$`)
	shellTool  = regexp.MustCompile(`(?i)shell|bash|terminal|exec`)
	shellParam = regexp.MustCompile(`(?i)^(command|cmd)$`)
	workDir    = regexp.MustCompile(`(?im)(?:working directory|cwd|current directory)\s*(?:is)?\s*[:=]?\s*(.+)$`)
	replyWith  = regexp.MustCompile(`(?i)reply with exactly:\s*(.+)`)
)

// promptDir is the working folder a system prompt names ("Working
// directory: /x/y", or dsh's `Working directory: "/x/y".`), or "".
func promptDir(all string) string {
	for _, m := range workDir.FindAllStringSubmatch(all, -1) {
		d := strings.Trim(strings.TrimSpace(m[1]), "`'\"<>.,;")
		if strings.HasPrefix(d, "/") || (len(d) > 2 && d[1] == ':') {
			return d
		}
	}
	return ""
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request, auth string) {
	var req chatReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Messages) == 0 {
		writeJSON(w, 400, map[string]any{"error": map[string]any{"code": 400, "message": "Invalid request", "type": "invalid_request_error"}})
		s.record(Request{Path: r.URL.Path, Status: 400, Auth: auth, Reply: "error"})
		return
	}
	rec := Request{Path: r.URL.Path, Auth: auth, Model: req.Model, Stream: req.Stream, Messages: len(req.Messages), ReasoningEffort: req.ReasoningEffort}
	for _, t := range req.Tools {
		rec.Tools = append(rec.Tools, t.Function.Name)
	}
	if e := req.ReasoningEffort; e != "" && e != "none" && len(s.o.Levels) > 0 && !slices.Contains(s.o.Levels, e) {
		writeJSON(w, 500, map[string]any{"error": map[string]any{"code": 500, "message": "Jinja Exception: Unexpected reasoning effort " + e, "type": "server_error"}})
		rec.Status, rec.Reply = 500, "error"
		s.record(rec)
		return
	}
	last := req.Messages[len(req.Messages)-1]
	var all strings.Builder
	for _, m := range req.Messages {
		all.WriteString(m.text() + "\n")
	}
	var text, tool, args string
	switch {
	case last.Role == "tool":
		rec.ToolResult = strings.TrimSpace(last.text())
		text = rec.ToolResult
	case !s.o.NoTools:
		tool, args = pickTool(req, promptDir(all.String()))
	}
	if tool == "" && text == "" {
		text = "Hello from the fake model."
		if m := replyWith.FindStringSubmatch(last.text()); m != nil {
			text = strings.TrimSpace(m[1])
		}
	}
	rec.Status = 200
	if tool != "" {
		rec.Reply, rec.ToolCall = "tool_call", tool+" "+args
	} else {
		rec.Reply, rec.Text = "text", text
	}
	s.record(rec)
	s.mu.Lock()
	s.n++
	id := fmt.Sprintf("chatcmpl-fake%d", s.n)
	s.mu.Unlock()
	if s.o.Delay > 0 {
		select {
		case <-time.After(s.o.Delay):
		case <-r.Context().Done():
			return
		}
	}
	if req.Stream {
		s.stream(w, id, text, tool, args, req.StreamOptions.IncludeUsage)
		return
	}
	msg := map[string]any{"role": "assistant", "content": text}
	finish := "stop"
	if s.o.Thinking {
		msg["reasoning_content"] = "Thinking about it."
	}
	if tool != "" {
		msg["content"] = nil
		msg["tool_calls"] = []any{map[string]any{"id": "call_" + id, "type": "function", "function": map[string]any{"name": tool, "arguments": args}}}
		finish = "tool_calls"
	}
	writeJSON(w, 200, map[string]any{
		"id": id, "object": "chat.completion", "created": time.Now().Unix(), "model": s.o.Alias, "system_fingerprint": "b11960-fakellm",
		"choices": []any{map[string]any{"index": 0, "finish_reason": finish, "message": msg}},
		"usage":   usage(), "timings": timings(),
	})
}

// pickTool chooses the scripted turn-1 call: a read tool with a path-like
// parameter on ezlm-check.txt (absolute when dir is known), else a shell
// tool running cat (type on Windows). Map order is random, so parameters are
// looked at sorted.
func pickTool(req chatReq, dir string) (string, string) {
	file := "ezlm-check.txt"
	if dir != "" {
		sep := "/"
		if strings.Contains(dir, `\`) {
			sep = `\`
		}
		file = strings.TrimRight(dir, `/\`) + sep + file
	}
	params := func(i int) []string {
		var ps []string
		for p := range req.Tools[i].Function.Parameters.Properties {
			ps = append(ps, p)
		}
		slices.Sort(ps)
		return ps
	}
	for i, t := range req.Tools {
		if !strings.Contains(strings.ToLower(t.Function.Name), "read") {
			continue
		}
		for _, p := range params(i) {
			if pathParam.MatchString(p) {
				b, _ := json.Marshal(map[string]string{p: file})
				return t.Function.Name, string(b)
			}
		}
	}
	for i, t := range req.Tools {
		if !shellTool.MatchString(t.Function.Name) {
			continue
		}
		for _, p := range params(i) {
			if shellParam.MatchString(p) {
				cat := "cat "
				if strings.Contains(file, `\`) {
					cat = "type "
				}
				b, _ := json.Marshal(map[string]string{p: cat + `"` + file + `"`})
				return t.Function.Name, string(b)
			}
		}
	}
	return "", ""
}

// stream sends llama-server's chunk sequence: content null first, then
// reasoning, then the tool call or the text, a finish chunk with timings,
// an optional usage chunk, and [DONE].
func (s *Server) stream(w http.ResponseWriter, id, text, tool, args string, withUsage bool) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)
	fl, _ := w.(http.Flusher)
	send := func(choices []any, extra map[string]any) {
		c := map[string]any{"id": id, "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": s.o.Alias, "system_fingerprint": "b11960-fakellm", "choices": choices}
		for k, v := range extra {
			c[k] = v
		}
		b, _ := json.Marshal(c)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if fl != nil {
			fl.Flush()
		}
	}
	delta := func(d map[string]any) []any {
		return []any{map[string]any{"index": 0, "finish_reason": nil, "delta": d}}
	}
	send(delta(map[string]any{"role": "assistant", "content": nil}), nil)
	if s.o.Thinking {
		send(delta(map[string]any{"reasoning_content": "Thinking "}), nil)
		send(delta(map[string]any{"reasoning_content": "about it."}), nil)
	}
	finish := "stop"
	if tool != "" {
		half := len(args) / 2
		send(delta(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "call_" + id, "type": "function", "function": map[string]any{"name": tool, "arguments": args[:half]}}}}), nil)
		send(delta(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "function": map[string]any{"arguments": args[half:]}}}}), nil)
		finish = "tool_calls"
	} else {
		for _, part := range splitWords(text) {
			send(delta(map[string]any{"content": part}), nil)
		}
	}
	send([]any{map[string]any{"index": 0, "finish_reason": finish, "delta": map[string]any{}}}, map[string]any{"timings": timings()})
	if withUsage {
		send([]any{}, map[string]any{"usage": usage()})
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
	if fl != nil {
		fl.Flush()
	}
}

func splitWords(s string) []string {
	var out []string
	for i, w := range strings.SplitAfter(s, " ") {
		if w != "" || i == 0 {
			out = append(out, w)
		}
	}
	return out
}

func usage() map[string]any {
	return map[string]any{"prompt_tokens": 1200, "completion_tokens": 12, "total_tokens": 1212, "prompt_tokens_details": map[string]any{"cached_tokens": 0}}
}

func timings() map[string]any {
	return map[string]any{
		"cache_n": 0, "prompt_n": 1200, "prompt_ms": 400.0, "prompt_per_token_ms": 0.333, "prompt_per_second": 3000.0,
		"predicted_n": 12, "predicted_ms": 300.0, "predicted_per_token_ms": 25.0, "predicted_per_second": 40.0,
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
