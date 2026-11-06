package serve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"testing"
)

// /props tells EZLM which reasoning levels the chat template renders, so the
// app writers never send one the template rejects with an HTTP 500.
func TestFetchProps(t *testing.T) {
	// The handler runs on the server's goroutine: guard the captured header so
	// the read below is ordered after the write, not just by the response.
	var mu sync.Mutex
	var auth string
	body := `{"default_generation_settings":{"n_ctx":65536},"total_slots":1,
		"chat_template":"{% if enable_thinking %}<think>{% endif %}",
		"chat_template_caps":{"supports_tools":true,"reasoning_effort_levels":["low","medium","","medium","xhigh",7]},
		"modalities":{"vision":false},"build_info":"b11960-abc"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auth = r.Header.Get("Authorization")
		b := body
		mu.Unlock()
		if r.URL.Path != "/props" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(b))
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	c := &Config{Port: port, Keys: []APIKey{{ID: "k1", Key: "ezlm-propstestkey"}}}
	c.Key = "ezlm-propstestkey"
	p, err := FetchProps(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got := auth
	mu.Unlock()
	if got != "Bearer ezlm-propstestkey" {
		t.Fatalf("auth = %q", got)
	}
	if p.Ctx != 65536 || p.Slots != 1 || !p.Thinking || p.Build != "b11960-abc" || !slices.Equal(p.Levels, []string{"low", "medium", "xhigh"}) {
		t.Fatalf("props = %+v", p)
	}

	// An older server: no levels, no thinking.
	mu.Lock()
	body = `{"default_generation_settings":{"n_ctx":8192},"chat_template":"plain","chat_template_caps":{}}`
	mu.Unlock()
	if p, err = FetchProps(context.Background(), c); err != nil || p.Levels != nil || p.Thinking {
		t.Fatalf("old server: %+v %v", p, err)
	}
	mu.Lock()
	body = `{"chat_template":"x","chat_template_caps":{"supports_thinking":true}}`
	mu.Unlock()
	if p, _ = FetchProps(context.Background(), c); !p.Thinking {
		t.Fatalf("caps thinking: %+v", p)
	}
}
