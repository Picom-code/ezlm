package engine_test

// T06 a10: --reasoning-format, --reasoning-budget and the thinking controls.
// llama.cpp's flag surface for these is unchanged between b11960 and v0.9.0
// (b12110), so EZLM's exposure is what it passes and what it reads back from
// /props. These tests pin that contract. internal/serve is read-only for T06,
// so the checks live here with the rest of the upgrade study.

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/serve"
)

// TestArgsPassNoReasoningFlags: EZLM leaves every reasoning flag at the
// llama.cpp default. The defaults EZLM relies on are reasoning-format
// deepseek (thoughts in message.reasoning_content), reasoning-budget -1
// (unrestricted) and --reasoning auto, all the same at b11960 and b12110.
// If EZLM starts passing one, it must be a deliberate change with a test here.
func TestArgsPassNoReasoningFlags(t *testing.T) {
	c := &serve.Config{
		ModelPath: "/models/m.gguf",
		ModelID:   "m",
		Key:       "REDACTED-KEY",
		Port:      8080,
		Ctx:       8192,
		NGL:       99,
	}
	args := c.Args()

	for _, want := range [][]string{{"--jinja"}, {"--metrics"}, {"-np", "1"}} {
		if !hasSeq(args, want) {
			t.Fatalf("Args() = %v, missing %v", args, want)
		}
	}
	// Flags whose presence would silently change how thoughts are returned.
	reasoningFlags := []string{
		"--reasoning-format", "--reasoning-budget", "--reasoning-budget-message",
		"--reasoning", "-rea", "--reasoning-effort", "--reasoning-preserve",
		"--no-reasoning-preserve", "--chat-template-kwargs",
	}
	for _, a := range args {
		for _, f := range reasoningFlags {
			if a == f || strings.HasPrefix(a, f+"=") {
				t.Fatalf("Args() passes %q, so EZLM no longer runs on llama.cpp's reasoning defaults: %v", a, args)
			}
		}
	}
}

// TestArgsReasoningFlagsFromRecipeComeLast: recipe flags are appended after
// EZLM's own, so a recipe can set --reasoning-budget or --reasoning-format
// and win. That ordering is what makes the defaults above safe.
func TestArgsReasoningFlagsFromRecipeComeLast(t *testing.T) {
	extra := []string{"--reasoning-budget", "0", "--reasoning-format", "none"}
	c := &serve.Config{ModelPath: "/models/m.gguf", ModelID: "m", Key: "k", Port: 8080, Ctx: 8192, ExtraArgs: extra}
	args := c.Args()
	if len(args) < len(extra) {
		t.Fatalf("Args() = %v, want it to end with %v", args, extra)
	}
	tail := args[len(args)-len(extra):]
	for i := range extra {
		if tail[i] != extra[i] {
			t.Fatalf("Args() ends %v, want %v", tail, extra)
		}
	}
}

// propsJSON is the shape of GET /props at b12110 (v0.9.0): chat_template_caps
// still carries reasoning_effort_levels, and the release adds a "voices" key
// next to it. Unknown keys must not disturb EZLM's parse.
func propsJSON(template string, levels string) string {
	return fmt.Sprintf(`{
		"default_generation_settings": {"n_ctx": 8192},
		"total_slots": 1,
		"chat_template": %q,
		"chat_template_caps": {
			"supports_tools": true,
			"supports_tool_calls": true,
			"supports_system_role": true,
			"supports_preserve_reasoning": false,
			"supports_reasoning_effort": true,
			"reasoning_effort_levels": %s
		},
		"modalities": {"vision": false},
		"build_info": "b12110-000000000",
		"voices": ["alloy"]
	}`, template, levels)
}

func propsServer(t *testing.T, body string) *serve.Config {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/props" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer REDACTED-KEY" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	var p int
	if _, err := fmt.Sscanf(port, "%d", &p); err != nil {
		t.Fatal(err)
	}
	return &serve.Config{Port: p, Key: "REDACTED-KEY"}
}

// TestFetchPropsReasoningAtLatestRelease: the fields EZLM's app writers need
// (reasoning levels, thinking) are still readable from /props at v0.9.0, with
// duplicates and blanks dropped and the order kept.
func TestFetchPropsReasoningAtLatestRelease(t *testing.T) {
	c := propsServer(t, propsJSON("...{% if enable_thinking %}...{% endif %}...",
		`["low", "medium", "", "medium", "xhigh", 7]`))
	p, err := serve.FetchProps(context.Background(), c)
	if err != nil {
		t.Fatalf("FetchProps: %v", err)
	}
	if want := []string{"low", "medium", "xhigh"}; !equal(p.Levels, want) {
		t.Fatalf("Levels = %v, want %v", p.Levels, want)
	}
	if !p.Thinking {
		t.Fatal("Thinking = false, want true for a template that reads enable_thinking")
	}
	if p.Ctx != 8192 || p.Slots != 1 || p.Build != "b12110-000000000" {
		t.Fatalf("Props = %+v", p)
	}
}

// TestFetchPropsThinkingFallsBackToCapsKey: /props caps from b11960 through
// b12110 report supports_reasoning_effort, never supports_thinking or
// supports_reasoning. A template that thinks without the literal
// "enable_thinking" therefore reads as Thinking=false, and the writers leave
// the thinking toggle out. Pinned here so a caps change at the next upgrade
// shows up as a test failure.
func TestFetchPropsThinkingFallsBackToCapsKey(t *testing.T) {
	c := propsServer(t, propsJSON("...{% if thinking %}...{% endif %}...", `["low", "high"]`))
	p, err := serve.FetchProps(context.Background(), c)
	if err != nil {
		t.Fatalf("FetchProps: %v", err)
	}
	if p.Thinking {
		t.Fatal("Thinking = true, want false: the caps have no supports_thinking key")
	}
	if want := []string{"low", "high"}; !equal(p.Levels, want) {
		t.Fatalf("Levels = %v, want %v", p.Levels, want)
	}
}

func hasSeq(args, seq []string) bool {
	for i := 0; i+len(seq) <= len(args); i++ {
		ok := true
		for j := range seq {
			if args[i+j] != seq[j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
