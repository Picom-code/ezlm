package integrate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/picom-code/ezlm/internal/paths"
	"github.com/picom-code/ezlm/internal/proc"
)

// LogPath is a tool's log: ~/.ezlm/logs/integrate-<id>.log. Everything EZLM
// installs, writes, checks or launches for the tool goes there, keys removed.
func LogPath(id string) string { return filepath.Join(paths.Logs(), "integrate-"+id+".log") }

// logMax: a log bigger than this at the start of a run moves to .1 first.
const logMax = 2 << 20

var (
	keyLike = regexp.MustCompile(`ezlm-[A-Za-z0-9]{6,}`)
	skLike  = regexp.MustCompile(`\bsk-[A-Za-z0-9_\-]{8,}`)
	bearer  = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=\-]{8,}`)
)

// RedactKeys shortens EZLM keys to their first 9 characters ("ezlm-AbCd…"):
// enough to tell keys apart in a report, not enough to use one.
func RedactKeys(s string) string {
	return keyLike.ReplaceAllStringFunc(s, func(k string) string { return k[:9] + "…" })
}

// Redact removes keys from s: the literal secrets given (replaced whole),
// then any EZLM key, sk-… key or Bearer token.
func Redact(s string, secrets ...string) string {
	for _, k := range secrets {
		if len(k) >= 6 {
			s = strings.ReplaceAll(s, k, "REDACTED-KEY")
		}
	}
	s = RedactKeys(s)
	s = skLike.ReplaceAllString(s, "sk-…")
	return bearer.ReplaceAllString(s, "${1}…")
}

// LogTail returns the last n lines of a tool's log (already redacted).
func LogTail(id string, n int) []string {
	b, err := os.ReadFile(LogPath(id))
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// Run is one job (Setup, Open, a preflight) for one tool: progress lines for
// the UI and the tool's log file, with keys redacted. A nil *Run is valid and
// does nothing, so tool tests can pass nil.
type Run struct {
	ID       string
	progress Log
	secrets  []string
	mu       sync.Mutex
	f        *os.File
	streamAt time.Time
	left     []int // PIDs the last Install left running
}

// NewRun opens id's log for one job. secrets (normally the EZLM key) never
// reach the log or the UI.
func NewRun(id string, progress Log, secrets ...string) *Run {
	r := &Run{ID: id, progress: progress, secrets: secrets}
	path := LogPath(id)
	if os.MkdirAll(filepath.Dir(path), 0o755) == nil {
		if st, err := os.Stat(path); err == nil && st.Size() > logMax {
			os.Rename(path, path+".1")
		}
		r.f, _ = os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	}
	return r
}

// LogPath is where this run writes.
func (r *Run) LogPath() string {
	if r == nil {
		return ""
	}
	return LogPath(r.ID)
}

// Redact removes this run's secrets and anything key-like from s.
func (r *Run) Redact(s string) string {
	if r == nil {
		return Redact(s)
	}
	return Redact(s, r.secrets...)
}

// Step is a progress line: shown in the UI and logged.
func (r *Run) Step(msg string) {
	if r == nil {
		return
	}
	msg = r.Redact(msg)
	r.write(msg)
	if r.progress != nil {
		r.progress(msg)
	}
}

// Stream is one line of an installer's output: every line is logged, and at
// most one every 500 ms reaches the UI.
func (r *Run) Stream(line string) {
	if r == nil {
		return
	}
	line = strings.TrimSpace(r.Redact(line))
	if line == "" {
		return
	}
	r.write("  | " + line)
	r.mu.Lock()
	show := time.Since(r.streamAt) >= 500*time.Millisecond
	if show {
		r.streamAt = time.Now()
	}
	r.mu.Unlock()
	if show && r.progress != nil {
		r.progress(line)
	}
}

// Logf writes to the log only.
func (r *Run) Logf(format string, a ...any) {
	if r == nil {
		return
	}
	r.write(r.Redact(fmt.Sprintf(format, a...)))
}

// Exec runs a check or probe command with a timeout and returns its trimmed
// combined output. The command line, exit and full output go to the log;
// the whole process tree is stopped when the timeout or ctx ends.
func (r *Run) Exec(ctx context.Context, timeout time.Duration, cmd *exec.Cmd) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	r.Logf("run: %s%s", strings.Join(cmd.Args, " "), inDir(cmd.Dir))
	start := time.Now()
	out, err := proc.Output(ctx, cmd)
	s := strings.TrimSpace(string(out))
	r.Logf("exit after %.1f s: %v\n%s", time.Since(start).Seconds(), errOrOK(err), s)
	return s, err
}

// Close ends the run.
func (r *Run) Close() error {
	if r == nil || r.f == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	err := r.f.Close()
	r.f = nil
	return err
}

func (r *Run) write(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	for _, l := range strings.Split(strings.TrimRight(msg, "\n"), "\n") {
		fmt.Fprintf(r.f, "%s %s\n", ts, l)
	}
}

func inDir(d string) string {
	if d == "" {
		return ""
	}
	return " (in " + d + ")"
}

func errOrOK(err error) any {
	if err == nil {
		return "ok"
	}
	return err
}
