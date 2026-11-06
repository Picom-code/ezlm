package serve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"testing"
)

// propsV090 is a /props body shaped like llama.cpp v0.9.0 (the current
// release), copied from get_res_props in tools/server/server-context.cpp and
// common/jinja/caps.cpp to_map(). The upgrade adds "voices" for audio models
// and nothing else EZLM reads, so the parser must keep working unchanged.
const propsV090 = `{
  "default_generation_settings": {"params": {"temperature": 0.8}, "n_ctx": 32768},
  "total_slots": 1,
  "model_alias": "qwen3-8b",
  "model_ftype": "Q8_0",
  "model_path": "/models/qwen3-8b.gguf",
  "modalities": {"vision": true, "video": false, "audio": false},
  "media_marker": "<__media_abc__>",
  "endpoint_slots": true,
  "endpoint_props": false,
  "endpoint_metrics": true,
  "ui": true,
  "ui_settings": {},
  "chat_template": "{% if enable_thinking %} think {% endif %}{{ messages }}",
  "chat_template_caps": {
    "supports_string_content": true,
    "supports_typed_content": false,
    "supports_tools": true,
    "supports_tool_calls": true,
    "supports_parallel_tool_calls": true,
    "supports_system_role": true,
    "supports_preserve_reasoning": false,
    "supports_reasoning_effort": true,
    "supports_object_arguments": false,
    "reasoning_effort_levels": ["low", "medium", "high"]
  },
  "bos_token": "<s>",
  "eos_token": "</s>",
  "build_info": "b12144-abc1234",
  "is_sleeping": false,
  "cors_proxy_enabled": false,
  "voices": ["female", "male"]
}`

func propsServer(t *testing.T, body string) *Config {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/props" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	return &Config{Port: port, Key: "REDACTED-KEY"}
}

// The v0.9.0 /props schema still carries every field EZLM reads, so the
// upgrade does not break reasoning levels, vision or the build stamp.
func TestFetchPropsV090(t *testing.T) {
	c := propsServer(t, propsV090)
	p, err := FetchProps(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if p.Ctx != 32768 || p.Slots != 1 || !p.Vision || p.Build != "b12144-abc1234" {
		t.Fatalf("props = %+v", p)
	}
	if !slices.Equal(p.Levels, []string{"low", "medium", "high"}) {
		t.Fatalf("levels = %v", p.Levels)
	}
	if !p.Thinking {
		t.Fatal("thinking = false, the template reads enable_thinking")
	}
}

// chat_template_caps has no supports_thinking or supports_reasoning key in
// b11960 or v0.9.0: caps::to_map() reports supports_reasoning_effort instead.
// EZLM's caps fallback therefore never fires and a template without the
// enable_thinking text reads as no-thinking. Flip this assertion when
// props.go learns the real key.
func TestFetchPropsThinkingCapsGap(t *testing.T) {
	c := propsServer(t, `{"chat_template":"plain","chat_template_caps":{
		"supports_reasoning_effort": true, "supports_preserve_reasoning": true}}`)
	p, err := FetchProps(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if p.Thinking {
		t.Fatal("thinking = true: props.go now knows supports_reasoning_effort, update this test")
	}
}
