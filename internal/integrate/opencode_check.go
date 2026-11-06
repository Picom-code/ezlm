package integrate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// The model-and-tool check and the launch preflight (PLAN 4.5).

// openCodeCheckTimeout: a first agent turn on a slow computer reads 10-20K
// prompt tokens before the first answer, then a tool round trip.
const openCodeCheckTimeout = 8 * time.Minute

// openCodeRun is what `opencode run --format json` printed: one JSON event
// per line (shapes in testdata/opencode/*.jsonl). Other lines (1.x's
// --print-logs on stderr) are ignored here and only matched by Explain.
type openCodeRun struct {
	Texts  []string // "text" events
	Tools  []string // "tool_use" events: the tool's name
	Errors []string // "error" events: the message
}

type openCodeEvent struct {
	Type string `json:"type"`
	Part struct {
		Type string `json:"type"`
		Text string `json:"text"`
		Tool string `json:"tool"`
	} `json:"part"`
	Error struct {
		Type    string `json:"type"` // 2.x: "provider.auth", "provider.no-route", …
		Message string `json:"message"`
		Name    string `json:"name"` // 1.x: "APIError", "UnknownError"
		Data    struct {
			Message string `json:"message"`
		} `json:"data"`
	} `json:"error"`
}

// parseOpenCodeRun reads the JSON lines of `opencode run --format json`.
// 2.x and 1.x print the same event types: step_start, tool_use, step_finish,
// text and error.
func parseOpenCodeRun(out string) openCodeRun {
	var r openCodeRun
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var e openCodeEvent
		if json.Unmarshal([]byte(line), &e) != nil {
			continue
		}
		switch e.Type {
		case "text":
			r.Texts = append(r.Texts, e.Part.Text)
		case "tool_use":
			r.Tools = append(r.Tools, e.Part.Tool)
		case "error":
			msg := firstNonEmpty(e.Error.Message, e.Error.Data.Message, e.Error.Name, e.Error.Type)
			r.Errors = append(r.Errors, msg)
		}
	}
	return r
}

// openCodeCheck runs the check prompt through `opencode run` in the check
// workspace. It reads the real config folder; the data, state and cache
// folders are throwaway ones, so the check adds no session to the user's
// history and works whatever database another OpenCode left there.
func openCodeCheck(ctx context.Context, f Found, ep Endpoint, w *CheckWorkspace, r *Run) (bool, error) {
	tmp, err := os.MkdirTemp("", "ezlm-opencode-check-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(tmp)
	fl := openCodeFlagsOf(f)
	args := []string{"run"}
	if fl.standalone {
		args = append(args, "--standalone")
	} else {
		// 1.x reports a missing model only in its log ("Model not found").
		args = append(args, "--print-logs", "--log-level", "ERROR")
	}
	args = append(args, "--model", openCodeProvider+"/"+ep.ModelID, "--format", "json", w.Prompt())
	extra := []string{
		"XDG_DATA_HOME=" + filepath.Join(tmp, "data"),
		"XDG_STATE_HOME=" + filepath.Join(tmp, "state"),
		"XDG_CACHE_HOME=" + filepath.Join(tmp, "cache"),
		"OPENCODE_DISABLE_MODELS_FETCH=1",
	}
	// The config folder (OPENCODE_CONFIG_DIR, XDG_CONFIG_HOME or
	// ~/.config) is inherited unchanged, so this tests the user's config.
	out, err := r.Exec(ctx, openCodeCheckTimeout, openCodeCmd(f, w.Dir, extra, args...))
	res := parseOpenCodeRun(out)
	r.Logf("check: %d text, tools %v, errors %q", len(res.Texts), res.Tools, res.Errors)
	for _, t := range res.Texts {
		if w.Passed(t) {
			return len(res.Tools) > 0, nil
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		return len(res.Tools) > 0, fmt.Errorf("OpenCode didn't answer within %d minutes. Details are in %s.", int(openCodeCheckTimeout.Minutes()), r.LogPath())
	}
	return len(res.Tools) > 0, openCodeCheckErr(out, res, r)
}

// openCodeCheckErr is the sentence for a failed check.
func openCodeCheckErr(out string, res openCodeRun, r *Run) error {
	if e, ok := Explain("opencode", out); ok {
		return errors.New(e.Say)
	}
	switch {
	case len(res.Errors) > 0:
		return fmt.Errorf("OpenCode stopped with %q. Details are in %s.", r.Redact(res.Errors[len(res.Errors)-1]), r.LogPath())
	case len(res.Tools) == 0 && len(res.Texts) > 0:
		return errors.New("The model answered OpenCode without using a tool, so OpenCode can't edit files with it. Pick a model marked Tools, or use Chat.")
	case len(res.Tools) > 0:
		return errors.New("The model read the file but didn't repeat what was in it, so EZLM couldn't confirm it works with OpenCode. Press Connect to try again, or pick a larger model.")
	}
	return fmt.Errorf("OpenCode didn't answer: %s. Details are in %s.", r.Redact(openCodeCause(out, "")), r.LogPath())
}

// openCodePreflight checks what Open will run, with the launch's program,
// environment, config and data folders, without starting a session: the
// key file, the workspace's model, the chat history's format, and that
// OpenCode lists EZLM's model.
func openCodePreflight(ctx context.Context, f Found, ep Endpoint, r *Run) error {
	dir := openCodeConfigDir()
	if err := openCodeKeyMatches(dir, ep.Key); err != nil {
		return err
	}
	if w, err := readOpenCodeConfig(filepath.Join(Workspace(), "opencode.json")); err != nil || w.Model != openCodeProvider+"/"+ep.ModelID {
		return errors.New("OpenCode would open with another model. Press Connect to update it.")
	}
	data := openCodeDBFormat(openCodeDataDir())
	if !openCodeAtLeast(f.Version, "2.0.0") && data == openCodeDataV2 {
		return errors.New("OpenCode 1.x can't open the chat history a newer OpenCode wrote; press Connect and EZLM will use OpenCode 2.2.5.")
	}
	fl := openCodeFlagsOf(f)
	args := []string{"models", openCodeProvider}
	if fl.standalone {
		args = []string{"models", "--standalone"} // 2.x lists every provider
	}
	out, err := r.Exec(ctx, time.Minute, openCodeCmd(f, Workspace(), openCodeLaunchEnv(f), args...))
	want := openCodeProvider + "/" + ep.ModelID
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == want {
			return nil
		}
	}
	if e, ok := Explain("opencode", out); ok {
		return errors.New(e.Say)
	}
	if err != nil {
		return openCodeStartErr(out, r)
	}
	return errors.New("OpenCode doesn't list EZLM's model. Press Connect to update it.")
}

// openCodeStartErr is the sentence for an OpenCode that exited at once. Its
// own words are often only "Unexpected error" (1.x, with the cause on the
// next line) or "Standalone server exited before reporting readiness" (2.x,
// with the cause only in its log), so the cause is looked for there, and a
// history it can't open is named as such.
func openCodeStartErr(out string, r *Run) error {
	cause := openCodeCause(out, openCodeDataDir())
	r.Logf("opencode's own reason: %s", cause)
	if openCodeDBError.MatchString(cause) {
		return fmt.Errorf("OpenCode can't open its chat history in %s (%s). Details are in %s.", openCodeDataDir(), r.Redact(cause), r.LogPath())
	}
	return fmt.Errorf("OpenCode didn't start: %s. Details are in %s.", r.Redact(cause), r.LogPath())
}

// openCodeDBError: SQLite's words for a database OpenCode couldn't open or
// migrate.
var openCodeDBError = regexp.MustCompile(`(?i)sqlite|already exists|no such (table|column)|database (disk image )?is (malformed|locked)|no session table`)

// openCodeCauseRe finds the cause in a 2.x log line:
// message="cli process failed" cause="Cause([Die(SQLiteError: table `project` already exists)])".
var openCodeCauseRe = regexp.MustCompile(`message="cli process failed" cause="Cause\(\[(?:Die|Fail)\((.*?)\)\]\)"`)

// openCodeCause is the most telling reason in an OpenCode run's output: the
// line after 1.x's "Unexpected error", else the last failure a 2.x server
// wrote to its log in dataDir (when the output only says it exited), else
// ErrLine.
func openCodeCause(out, dataDir string) string {
	lines := strings.Split(StripANSI(out), "\n")
	for i, l := range lines {
		if strings.Contains(l, "Unexpected error") {
			for _, n := range lines[i+1:] {
				if n = strings.TrimSpace(n); n != "" {
					return n
				}
			}
		}
	}
	if strings.Contains(out, "exited before reporting readiness") {
		if c := openCodeLogCause(dataDir); c != "" {
			return c
		}
	}
	return ErrLine(out)
}

// openCodeLogCause is the cause of the last failure in the newest log file
// in dataDir/log ("" when there is none). Only the file's last 256 KB are
// read.
func openCodeLogCause(dataDir string) string {
	logs, _ := filepath.Glob(filepath.Join(dataDir, "log", "*.log"))
	var newest string
	var at time.Time
	for _, l := range logs {
		if st, err := os.Stat(l); err == nil && st.ModTime().After(at) {
			newest, at = l, st.ModTime()
		}
	}
	if newest == "" || time.Since(at) > 10*time.Minute {
		return ""
	}
	f, err := os.Open(newest)
	if err != nil {
		return ""
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > 256<<10 {
		f.Seek(st.Size()-256<<10, io.SeekStart)
	}
	b, _ := io.ReadAll(f)
	cause := ""
	for _, l := range strings.Split(string(b), "\n") {
		// The CLI's own "Standalone server exited" line comes after the
		// server's real reason; keep the first cause of the last burst.
		if m := openCodeCauseRe.FindStringSubmatch(l); m != nil && !strings.Contains(m[1], "exited before reporting readiness") {
			cause = m[1]
		}
	}
	return cause
}
