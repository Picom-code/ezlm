//go:build !windows

package integrate

import (
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

// PTY is a program running in a hidden pseudo-terminal (the TUI smoke test
// and cmd/tuiprobe). Close stops the program's whole process group: ours
// only, never anything found by name.
type PTY struct {
	f    *os.File
	cmd  *exec.Cmd
	once sync.Once
	done chan struct{}
	code int
	err  error
}

// StartPTY starts cmd in a cols x rows pseudo-terminal. A program started
// from the app has no TERM, and terminal UIs refuse to draw without one, so
// TERM defaults to xterm-256color.
func StartPTY(cmd *exec.Cmd, cols, rows int) (*PTY, error) {
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	if !slices.ContainsFunc(cmd.Env, func(e string) bool { return strings.HasPrefix(e, "TERM=") }) {
		cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	}
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return nil, err
	}
	p := &PTY{f: f, cmd: cmd, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			p.code, err = ee.ExitCode(), nil
		}
		p.err = err
		close(p.done)
	}()
	return p, nil
}

func (p *PTY) Read(b []byte) (int, error)  { return p.f.Read(b) }
func (p *PTY) Write(b []byte) (int, error) { return p.f.Write(b) }

// Pid is the program's process id.
func (p *PTY) Pid() int { return p.cmd.Process.Pid }

// Exited reports whether the program has ended.
func (p *PTY) Exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

// Wait waits for the program to end and returns its exit code.
func (p *PTY) Wait() (int, error) {
	<-p.done
	return p.code, p.err
}

// Close closes the terminal and kills the program's process group.
func (p *PTY) Close() error {
	p.once.Do(func() {
		// pty.Start makes the program a session leader, so its pid is its
		// process group id.
		syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
		p.f.Close()
		<-p.done
	})
	return nil
}
