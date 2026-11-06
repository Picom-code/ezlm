package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/picom-code/ezlm/internal/scan"
	"github.com/picom-code/ezlm/internal/serve"
)

// Live is a cheap snapshot for dashboards, polled every couple of seconds.
type Live struct {
	RAMTotal  uint64  `json:"ram_total"`
	RAMAvail  uint64  `json:"ram_avail"`
	GPUTotal  uint64  `json:"gpu_total"`
	GPUFree   uint64  `json:"gpu_free"`
	Unified   bool    `json:"unified"`
	Healthy   bool    `json:"healthy"`
	Starting  bool    `json:"starting"`    // server process is up but still loading the model
	TokPerSec float64 `json:"tok_per_sec"` // recent generation speed
	Requests  int     `json:"requests"`    // requests being processed now
	Tokens    uint64  `json:"tokens"`      // tokens generated since start
	Uptime    int64   `json:"uptime_s"`
	Queued    int     `json:"queued"` // requests waiting for the current one to finish

	// What the current request is doing, so "working" is never a mystery:
	// reading the prompt (with progress) or writing the answer.
	Phase       string `json:"phase"` // idle, reading, writing
	PromptDone  int    `json:"prompt_done"`
	PromptTotal int    `json:"prompt_total"`
	Written     int    `json:"written"`

	// While starting: how much of the model has been read into memory, and
	// how much of each memory the running setup is planned to take (for the
	// app's memory-filling animation).
	LoadDone  uint64 `json:"load_done"`
	LoadTotal uint64 `json:"load_total"`
	GPUPlan   uint64 `json:"gpu_plan"`
	RAMPlan   uint64 `json:"ram_plan"`
}

func (a *App) Live(ctx context.Context) Live {
	m := scan.QuickMemory(ctx)
	l := Live{RAMTotal: m.RAMTotal, RAMAvail: m.RAMAvail, GPUTotal: m.GPUTotal, GPUFree: m.GPUFree, Unified: m.Unified}
	cfg, err := serve.Load()
	if err != nil {
		return l
	}
	l.Healthy = serve.Healthy(ctx, cfg)
	l.GPUPlan, l.RAMPlan = cfg.GPUNeed, cfg.RAMNeed
	if !l.Healthy {
		l.Starting = a.isWarming() || len(serve.OwnPIDs(ctx)) > 0
		a.mu.Lock()
		l.LoadDone, l.LoadTotal = a.warmDone, a.warmTotal
		a.mu.Unlock()
		return l
	}
	metrics(ctx, cfg, &l)
	l.Phase = "idle"
	if l.Requests > 0 {
		slots(ctx, cfg, &l)
	}
	if st, err := serve.StartedAt(); err == nil {
		l.Uptime = int64(time.Since(st).Seconds())
	}
	return l
}

// metrics reads llama-server's Prometheus endpoint (enabled with --metrics).
func metrics(ctx context.Context, cfg *serve.Config, l *Live) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://127.0.0.1:%d/metrics", cfg.Port), nil)
	req.Header.Set("Authorization", "Bearer "+cfg.Key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
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
		switch f[0] {
		case "llamacpp:predicted_tokens_seconds":
			l.TokPerSec = v
		case "llamacpp:requests_processing":
			l.Requests = int(v)
		case "llamacpp:tokens_predicted_total":
			l.Tokens = uint64(v)
		case "llamacpp:requests_deferred":
			l.Queued = int(v)
		}
	}
}

// ansi matches terminal color codes llama-server writes even into its log file.
var ansi = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// Logs returns the last n lines of the model server's log.
func (a *App) Logs(n int) []string {
	t := ansi.ReplaceAllString(serve.TailLog(n), "")
	if strings.TrimSpace(t) == "" {
		return []string{}
	}
	return strings.Split(strings.TrimRight(t, "\n"), "\n")
}

// slots reads the busy slot's progress from llama-server's /slots.
func slots(ctx context.Context, cfg *serve.Config, l *Live) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://127.0.0.1:%d/slots", cfg.Port), nil)
	req.Header.Set("Authorization", "Bearer "+cfg.Key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var ss []struct {
		Processing bool `json:"is_processing"`
		Prompt     int  `json:"n_prompt_tokens"`
		Processed  int  `json:"n_prompt_tokens_processed"`
		Cached     int  `json:"n_prompt_tokens_cache"`
		Next       []struct {
			Decoded int `json:"n_decoded"`
		} `json:"next_token"`
	}
	if json.NewDecoder(resp.Body).Decode(&ss) != nil {
		return
	}
	for _, s := range ss {
		if !s.Processing {
			continue
		}
		decoded := 0
		if len(s.Next) > 0 {
			decoded = s.Next[0].Decoded
		}
		l.PromptTotal, l.PromptDone, l.Written = s.Prompt, min(s.Prompt, s.Processed+s.Cached), decoded
		if decoded > 0 {
			l.Phase = "writing"
		} else {
			l.Phase = "reading"
		}
		return
	}
}
