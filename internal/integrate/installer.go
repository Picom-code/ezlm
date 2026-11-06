package integrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/picom-code/ezlm/internal/proc"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/process"
)

// ShellCommand runs an installer script in the platform shell: PowerShell
// on Windows, a bash login shell elsewhere, with CI=1 NONINTERACTIVE=1.
// Callers add their own variables to cmd.Env.
func ShellCommand(script string) *exec.Cmd {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	} else {
		cmd = exec.Command("bash", "-lc", script)
	}
	cmd.Env = append(os.Environ(), "CI=1", "NONINTERACTIVE=1")
	return cmd
}

// Install runs an installer: every output line goes to the log and at most
// one every 500 ms to the UI. A timeout (0 = none) or a cancel stops the
// installer's whole process tree; what it deliberately leaves running after
// success is kept. On failure the error names the last telling line and the
// log.
func (r *Run) Install(ctx context.Context, cmd *exec.Cmd, timeout time.Duration) error {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	r.Logf("install: %s%s", strings.Join(cmd.Args, " "), inDir(cmd.Dir))
	lw := &lineWriter{fn: r.Stream}
	start := time.Now()
	left, err := proc.InstallLeft(ctx, cmd, lw)
	lw.flush()
	r.Logf("installer exit after %.0f s: %v", time.Since(start).Seconds(), errOrOK(err))
	if r != nil {
		r.mu.Lock()
		r.left = left
		r.mu.Unlock()
	}
	if err == nil {
		if len(left) > 0 {
			r.Logf("the installer left %d process(es) running: %s", len(left), describePIDs(left))
		}
		return nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("the installer didn't finish within %d minutes. Details are in %s", int(timeout.Minutes()), r.LogPath())
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("the installer stopped with this message: %s. Details are in %s", strings.TrimRight(ErrLine(strings.Join(lw.tail, "\n")), "."), r.LogPath())
}

// LeftRunning lists the PIDs the last Install left running after it ended
// successfully (for example a gateway an installer started), already logged.
func (r *Run) LeftRunning() []int {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.left)
}

// describePIDs is "123 (node), 456 (python3)" for the log.
func describePIDs(pids []int) string {
	var parts []string
	for _, pid := range pids {
		name := "?"
		if p, err := process.NewProcess(int32(pid)); err == nil {
			if n, err := p.Name(); err == nil {
				name = n
			}
		}
		parts = append(parts, fmt.Sprintf("%d (%s)", pid, name))
	}
	return strings.Join(parts, ", ")
}

// lineWriter splits output into lines (on \n or \r, so progress bars don't
// pile up) and keeps the last few for the error message.
type lineWriter struct {
	mu   sync.Mutex
	buf  []byte
	fn   func(string)
	tail []string
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := strings.IndexAny(string(w.buf), "\r\n")
		if i < 0 {
			break
		}
		w.line(string(w.buf[:i]))
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.line(string(w.buf))
		w.buf = nil
	}
}

func (w *lineWriter) line(l string) {
	if strings.TrimSpace(l) == "" {
		return
	}
	w.tail = append(w.tail, l)
	if len(w.tail) > 20 {
		w.tail = w.tail[len(w.tail)-20:]
	}
	if w.fn != nil {
		w.fn(l)
	}
}

// CheckDisk fails with a plain sentence when the drive holding dir has less
// than need bytes free. what names the thing being installed.
func CheckDisk(dir string, need uint64, what string) error {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(d); err == nil {
			u, err := disk.Usage(d)
			if err != nil || u.Free >= need {
				return nil // can't tell: let the installer try
			}
			const gb = 1 << 30
			return fmt.Errorf("installing %s needs about %.1f GB of free disk space and %.1f GB is free. Free up some space, then press Connect again", what, float64(need)/gb, float64(u.Free)/gb)
		}
		if filepath.Dir(d) == d {
			return nil
		}
	}
}
