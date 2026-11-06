package serve

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/shirou/gopsutil/v4/process"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/picom-code/ezlm/internal/integrate"
	"github.com/picom-code/ezlm/internal/paths"
)

func LogPath() string { return filepath.Join(paths.Logs(), "server.log") }

// Healthy reports whether llama-server answers /health with 200.
// It must also be EZLM's own server: another program's llama-server on the
// same port answers /health too.
func Healthy(ctx context.Context, c *Config) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://127.0.0.1:%d/health", c.Port), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200 && Ours(ctx, c)
}

// Ours reports whether the server on c's port is EZLM's: it accepts our key
// and serves our model under its alias.
func Ours(ctx context.Context, c *Config) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://127.0.0.1:%d/v1/models", c.Port), nil)
	req.Header.Set("Authorization", "Bearer "+c.Key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) != nil || len(out.Data) == 0 {
		// Another program on the port can answer 200 and even ignore the key,
		// so a body that lists no model proves nothing. Only a config from
		// before model ids has nothing to compare and goes on the key alone.
		return c.ModelID == ""
	}
	for _, m := range out.Data {
		if m.ID == c.ModelID {
			return true
		}
	}
	return false
}

// WaitHealthy polls until the model is loaded, the service dies, or ctx ends.
// tick is called roughly every second so callers can show liveness.
func WaitHealthy(ctx context.Context, c *Config, timeout time.Duration, tick func(elapsed time.Duration)) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	gone := 0
	for {
		if Healthy(ctx, c) {
			return nil
		}
		if msg := fatalInLog(); msg != "" {
			return errors.New(msg)
		}
		// A server can also just stop (a graphics driver crashing, a bad
		// flag) without writing why: say so in seconds instead of waiting
		// out the whole timeout. It counts as gone only when nothing answers
		// on its port (llama-server listens while it loads) and no process
		// of ours is found, for a while: launchd and systemd restart a
		// crashed server, and a profile reached through a link can hide our
		// processes from OwnPIDs.
		if time.Since(start) > stoppedGrace {
			if !listening(c.Port) && len(OwnPIDs(ctx)) == 0 {
				gone++
			} else {
				gone = 0
			}
			if gone >= stoppedChecks {
				return fmt.Errorf("%s. Last log lines:\n%s", ErrStopped, TailLog(8))
			}
		}
		if tick != nil {
			tick(time.Since(start))
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("the model did not finish loading in %s. Last log lines:\n%s", timeout, TailLog(8))
		case <-time.After(time.Second):
		}
	}
}

// listening reports whether anything accepts connections on the port.
func listening(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// ErrStopped starts the error when the server quit while loading.
const ErrStopped = "the model server stopped while loading"

// How long a just-started server gets before "not running" counts, and how
// many checks (about a second apart) in a row it must be missing.
var stoppedGrace, stoppedChecks = 8 * time.Second, 5

// FatalInLog spots unrecoverable errors so we fail fast instead of timing out.
func FatalInLog() string { return fatalInLog() }

func fatalInLog() string {
	for _, l := range strings.Split(TailLog(30), "\n") {
		ll := strings.ToLower(l)
		for _, k := range []string{"failed to load model", "error loading model", "out of memory", "failed to allocate", "address already in use", "couldn't bind", "exiting due to"} {
			if strings.Contains(ll, k) {
				return strings.TrimSpace(l)
			}
		}
	}
	return ""
}

// TailLog returns the last n lines of the server log, with keys redacted.
// Every caller shows the lines to the user (the log view, a load failure) or
// sends them in a report, so a line the server wrote with a key in it must not
// carry that key any further.
func TailLog(n int) string {
	f, err := os.Open(LogPath())
	if err != nil {
		return ""
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	return integrate.Redact(strings.Join(lines, "\n"))
}

type TestResult struct {
	Reply     string  `json:"reply"`
	TokPerSec float64 `json:"tok_per_sec"`
}

// Test sends one real chat completion through the authenticated API.
func Test(ctx context.Context, c *Config, prompt string) (*TestResult, error) {
	body, _ := json.Marshal(map[string]any{
		"model":      c.ModelID,
		"messages":   []map[string]string{{"role": "user", "content": prompt}},
		"max_tokens": 2048, // reasoning models think before answering
	})
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", c.LocalURL()+"/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("test request failed: %s", resp.Status)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
		Timings struct {
			PredictedPerSecond float64 `json:"predicted_per_second"`
		} `json:"timings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Choices) == 0 {
		return nil, errors.New("test request returned no answer")
	}
	// Show only the final answer, never the model's reasoning scratchpad.
	reply := strings.TrimSpace(out.Choices[0].Message.Content)
	if reply == "" {
		if out.Choices[0].Message.ReasoningContent == "" {
			return nil, errors.New("the model answered with nothing")
		}
		reply = "(Your model is thinking out loud and working. Ask it something in the chat.)"
	}
	return &TestResult{Reply: reply, TokPerSec: out.Timings.PredictedPerSecond}, nil
}

// StartedAt is when the current server log was started (the server truncates
// it on each start), a good-enough proxy for server uptime.
func StartedAt() (time.Time, error) {
	// The server process's own start time; the log file can outlive restarts
	// (it's emptied, not recreated, on some systems).
	var newest int64
	for _, pid := range OwnPIDs(context.Background()) {
		if p, err := process.NewProcess(pid); err == nil {
			if ms, err := p.CreateTime(); err == nil && ms > newest {
				newest = ms
			}
		}
	}
	if newest > 0 {
		return time.UnixMilli(newest), nil
	}
	st, err := os.Stat(LogPath())
	if err != nil {
		return time.Time{}, err
	}
	if t, ok := birthTime(st); ok {
		return t, nil
	}
	return st.ModTime(), nil
}
