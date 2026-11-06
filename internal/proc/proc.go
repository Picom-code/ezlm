// Package proc runs helper commands (installers, app checks) so that the whole
// process tree dies with them: on timeout, on cancel, and when EZLM exits.
// Without this, `opencode.cmd` → cmd.exe → opencode.exe left the real client
// running against the model after EZLM gave up on it.
package proc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"time"
)

// Output runs cmd and returns its combined output. When ctx ends, every
// process the command started is killed, not just the first one.
func Output(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	var buf bytes.Buffer
	err := run(ctx, cmd, true, &buf)
	return buf.Bytes(), err
}

// Install is Output for installers: the tree is killed if ctx ends, but
// anything it deliberately leaves running after success is kept.
func Install(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	var buf bytes.Buffer
	err := run(ctx, cmd, false, &buf)
	return buf.Bytes(), err
}

// InstallTo is Install that hands the combined output to w as it arrives.
func InstallTo(ctx context.Context, cmd *exec.Cmd, w io.Writer) error {
	_, err := InstallLeft(ctx, cmd, w)
	return err
}

// InstallLeft is InstallTo that also returns the PIDs of the processes the
// installer left running in its tree after it ended (a gateway, a
// background service), so the caller can log or report them.
func InstallLeft(ctx context.Context, cmd *exec.Cmd, w io.Writer) ([]int, error) {
	var left []int
	err := runTree(ctx, cmd, false, w, func(t *tree) { left = t.pids() })
	return left, err
}

// waitDelay: after the command exits, how long Wait keeps reading output
// that a leftover process still holds open. Without it, an installer that
// starts a background service would make Wait block forever.
var waitDelay = 5 * time.Second

func run(ctx context.Context, cmd *exec.Cmd, killLeftovers bool, w io.Writer) error {
	return runTree(ctx, cmd, killLeftovers, w, nil)
}

func runTree(ctx context.Context, cmd *exec.Cmd, killLeftovers bool, w io.Writer, after func(*tree)) error {
	cmd.Stdout, cmd.Stderr = w, w
	if cmd.WaitDelay == 0 {
		cmd.WaitDelay = waitDelay
	}
	prepare(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	tree := track(cmd)
	defer func() {
		if after != nil {
			after(tree)
		}
		if killLeftovers {
			tree.close()
		} else {
			tree.release()
		}
	}()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if errors.Is(err, exec.ErrWaitDelay) {
			return nil // it exited fine; a leftover still holds its output
		}
		return err
	case <-ctx.Done():
		tree.kill()
		<-done
		return ctx.Err()
	}
}
