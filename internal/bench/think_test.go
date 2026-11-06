package bench

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// A model that thinks past the limit without answering gets one more try with
// thinking off; the result says so.
func TestOverthinkingGetsASecondTry(t *testing.T) {
	var calls atomic.Int32
	var secondOff atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if calls.Add(1) == 1 {
			w.Write([]byte(`{"choices":[{"finish_reason":"length","message":{"role":"assistant","content":"","reasoning_content":"hmm, let me think again..."}}],"timings":{"predicted_n":4096}}`))
			return
		}
		kw, _ := body["chat_template_kwargs"].(map[string]any)
		secondOff.Store(kw["enable_thinking"] == false && body["reasoning_effort"] == "low")
		w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"5"}}]}`))
	}))
	defer srv.Close()
	res := reasoning(context.Background(), Target{BaseURL: srv.URL, Key: "k", Model: "m"})
	if !res.Pass || calls.Load() != 2 || !secondOff.Load() {
		t.Fatalf("pass=%v calls=%d secondOff=%v detail=%q", res.Pass, calls.Load(), secondOff.Load(), res.Detail)
	}
	if !strings.Contains(res.Detail, "thinking off") {
		t.Fatalf("detail should say thinking was off: %q", res.Detail)
	}
}

// An ordinary answer is never asked twice.
func TestNormalAnswerAskedOnce(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{\"name\": \"Neil Armstrong\", \"year\": 1969}"}}]}`))
	}))
	defer srv.Close()
	res := jsonFormat(context.Background(), Target{BaseURL: srv.URL, Key: "k", Model: "m"})
	if !res.Pass || calls.Load() != 1 || strings.Contains(res.Detail, "thinking off") {
		t.Fatalf("pass=%v calls=%d detail=%q", res.Pass, calls.Load(), res.Detail)
	}
}
