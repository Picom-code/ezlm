package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
)

// Props is what llama-server's /props says about the loaded model. EZLM
// reads it after each deploy and keeps it in Config, so the app writers know
// which reasoning levels the chat template accepts even while the server is
// down.
type Props struct {
	Ctx      int      `json:"ctx"`              // context per slot
	Slots    int      `json:"slots"`            // -np
	Vision   bool     `json:"vision"`           // an image projector is loaded
	Levels   []string `json:"levels,omitempty"` // reasoning_effort values the template renders, lowest first
	Thinking bool     `json:"thinking"`         // the template reads enable_thinking
	Build    string   `json:"build,omitempty"`
}

// FetchProps reads /props from c's server.
func FetchProps(ctx context.Context, c *Config) (*Props, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://127.0.0.1:%d/props", c.Port), nil)
	req.Header.Set("Authorization", "Bearer "+c.Key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("/props answered %s", resp.Status)
	}
	var raw struct {
		Settings struct {
			NCtx int `json:"n_ctx"`
		} `json:"default_generation_settings"`
		Slots        int            `json:"total_slots"`
		ChatTemplate string         `json:"chat_template"`
		Caps         map[string]any `json:"chat_template_caps"`
		Modalities   struct {
			Vision bool `json:"vision"`
		} `json:"modalities"`
		Build string `json:"build_info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	return &Props{
		Ctx: raw.Settings.NCtx, Slots: raw.Slots, Vision: raw.Modalities.Vision, Levels: propLevels(raw.Caps),
		Thinking: propThinking(raw.ChatTemplate, raw.Caps), Build: raw.Build,
	}, nil
}

// propLevels is chat_template_caps.reasoning_effort_levels, cleaned up:
// strings only, no blanks or repeats, in the server's order (lowest first).
// A server too old to report them gives nil, and the writers then leave
// reasoning settings out entirely.
func propLevels(caps map[string]any) []string {
	list, _ := caps["reasoning_effort_levels"].([]any)
	var out []string
	for _, v := range list {
		if s, ok := v.(string); ok {
			if s = strings.TrimSpace(s); s != "" && !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
	}
	return out
}

// propThinking: the template can think. It reads enable_thinking (Qwen,
// DeepSeek style), or its caps say it supports thinking or reasoning.
func propThinking(tmpl string, caps map[string]any) bool {
	if strings.Contains(tmpl, "enable_thinking") {
		return true
	}
	for _, k := range []string{"supports_thinking", "supports_reasoning"} {
		if b, _ := caps[k].(bool); b {
			return true
		}
	}
	return false
}
