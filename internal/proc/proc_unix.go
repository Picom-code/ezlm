//go:build !windows

package proc

import (
	"os/exec"
	"syscall"

	"github.com/shirou/gopsutil/v4/process"
)

// tree is the command's process group. The group id is the leader's pid, so
// it names this tree only while the leader is alive: once the group is empty
// the id can be handed to a new process, and a blind kill(-id) would then
// stop something EZLM never started. birth (the leader's start time) tells
// the two apart.
type tree struct {
	pid   int   // process group id: the pid of the command we started
	birth int64 // when that command started, ms since the epoch (0: unknown)
}

func prepare(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	// Linux: die with EZLM even if it's killed hard.
	setPdeathsig(cmd.SysProcAttr)
}

func track(cmd *exec.Cmd) *tree {
	birth, _ := leaderBirth(cmd.Process.Pid)
	return &tree{pid: cmd.Process.Pid, birth: birth}
}

// leaderBirth is when the process with this pid started, in ms since the
// epoch. ok is false when no process has that pid.
func leaderBirth(pid int) (birth int64, ok bool) {
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return 0, false
	}
	b, err := p.CreateTime()
	if err != nil {
		return 0, false
	}
	return b, true
}

// ours reports whether the group id still belongs to the tree we started.
// A new process can only take an id as the leader of a new group, so if the
// leader at that pid started at another time, the id was reused. When the
// leader is gone, the group can still hold its children and no new group can
// take the id while they live, so the group is ours.
func (t *tree) ours() bool {
	if t.birth == 0 {
		return true // start time unknown: keep the old behaviour
	}
	birth, ok := leaderBirth(t.pid)
	return !ok || birth == t.birth
}

func (t *tree) kill() { t.signal(syscall.SIGKILL) }

func (t *tree) close() { t.signal(syscall.SIGKILL) }

func (t *tree) signal(sig syscall.Signal) {
	if t.ours() {
		syscall.Kill(-t.pid, sig)
	}
}

func (t *tree) release() {}

// pids lists the processes still in the command's process group (programs
// that start a daemon properly leave the group, so this is best effort).
func (t *tree) pids() []int {
	if !t.ours() {
		return nil // the id belongs to another process now
	}
	all, err := process.Pids()
	if err != nil {
		return nil
	}
	var out []int
	for _, p := range all {
		if int(p) == t.pid {
			continue
		}
		if g, err := syscall.Getpgid(int(p)); err == nil && g == t.pid {
			out = append(out, int(p))
		}
	}
	return out
}
