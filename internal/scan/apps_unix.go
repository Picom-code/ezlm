//go:build !windows

package scan

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/shirou/gopsutil/v4/process"
)

func listProcs(ctx context.Context) []proc {
	ps, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil
	}
	out := make([]proc, 0, len(ps))
	for _, p := range ps {
		exe, err := p.ExeWithContext(ctx)
		if err != nil || exe == "" {
			continue
		}
		pr := proc{pid: p.Pid, exe: exe}
		if uids, err := p.UidsWithContext(ctx); err == nil && len(uids) > 0 {
			pr.mine = int(uids[0]) == os.Getuid()
		}
		if mi, err := p.MemoryInfoWithContext(ctx); err == nil {
			pr.ram = mi.RSS
		}
		out = append(out, pr)
	}
	return out
}

func gpuUsage(context.Context) map[int32]uint64 { return nil }

// ownedByMe: the process runs as the current user.
func ownedByMe(ctx context.Context, p *process.Process) bool {
	uids, err := p.UidsWithContext(ctx)
	return err == nil && len(uids) > 0 && int(uids[0]) == os.Getuid()
}

// On macOS, apps in a .app bundle have a user interface and can be asked to
// quit; elsewhere SIGTERM is the gentle way, which every process supports.
func windowPIDs() map[int32]bool {
	out := map[int32]bool{}
	if runtime.GOOS != "darwin" {
		return out
	}
	for _, p := range listProcs(context.Background()) {
		if strings.Contains(p.exe, ".app/") {
			out[p.pid] = true
		}
	}
	return out
}

func gracefulWithoutWindow() bool { return runtime.GOOS != "darwin" }

func closeGently(a *App) {
	if runtime.GOOS == "darwin" {
		if i := strings.Index(a.Exe, ".app/"); i >= 0 {
			// Ask the app to quit, like Cmd-Q: it can save and restore its windows.
			bundle := filepath.Base(a.Exe[:i+4])
			name := strings.TrimSuffix(bundle, ".app")
			exec.Command("osascript", "-e", `tell application "`+strings.ReplaceAll(name, `"`, ``)+`" to quit`).Run()
			return
		}
	}
	for _, pid := range a.PIDs {
		syscall.Kill(int(pid), syscall.SIGTERM)
	}
}

func killPID(pid int32, exe string) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return
	}
	if cur, err := p.Exe(); err != nil || installRoot(cur, osGetenv) != installRoot(exe, osGetenv) {
		return // the PID was reused by something else
	}
	syscall.Kill(int(pid), syscall.SIGKILL)
}

func fileDescription(string) string { return "" }
