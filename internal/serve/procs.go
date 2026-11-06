package serve

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/picom-code/ezlm/internal/paths"
	"github.com/shirou/gopsutil/v4/process"
)

// OwnPIDs finds llama-server processes started from EZLM's install directory.
func OwnPIDs(ctx context.Context) []int32 {
	ps, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil
	}
	homes := ownHomes()
	var out []int32
	for _, p := range ps {
		name, _ := p.NameWithContext(ctx)
		if !strings.HasPrefix(name, "llama-server") {
			continue
		}
		if exe, err := p.ExeWithContext(ctx); err == nil && ownExe(exe, homes) {
			out = append(out, p.Pid)
		}
	}
	return out
}

// ownHomes is EZLM's home and the path its links point at. The kernel reports
// a process's real path, so a server started through a linked folder (~/.ezlm
// moved to another drive, /tmp on macOS) would otherwise look like someone
// else's: a crashed engine's server would never be found again, and a restart
// would start a second one beside it.
func ownHomes() []string {
	home := filepath.Clean(paths.Home())
	homes := []string{home}
	if real, err := filepath.EvalSymlinks(home); err == nil && filepath.Clean(real) != home {
		homes = append(homes, filepath.Clean(real))
	}
	return homes
}

// ownExe reports whether exe is one of EZLM's programs, resolving the links
// in its own path too.
func ownExe(exe string, homes []string) bool {
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	for _, home := range homes {
		if isOwnExe(exe, home) {
			return true
		}
	}
	return false
}

// isOwnExe reports whether exe lives inside EZLM's home folder. This is the
// only test EZLM uses before stopping a process: servers started by other
// apps or by the user live elsewhere and are never touched.
func isOwnExe(exe, home string) bool {
	exe, home = realPath(exe), realPath(home)
	if runtime.GOOS == "windows" {
		exe, home = strings.ToLower(exe), strings.ToLower(home)
	}
	return strings.HasPrefix(exe, home+string(filepath.Separator))
}

// realPath resolves a home reached through a link (~/.ezlm on another drive,
// /var on macOS) to the path the kernel reports for a process, so EZLM still
// finds the server it started. A path that is gone is resolved as far as it
// exists. Falls back to the cleaned path when nothing can be resolved.
func realPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	cur, rest := p, ""
	for {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(resolved, rest)
		}
		dir, name := filepath.Split(cur)
		dir = filepath.Clean(dir)
		if dir == cur || dir == "." {
			return filepath.Clean(p)
		}
		rest = filepath.Join(name, rest)
		cur = dir
	}
}

// OwnRSS sums resident memory of the given processes.
func OwnRSS(ctx context.Context, pids []int32) uint64 {
	var total uint64
	for _, pid := range pids {
		if p, err := process.NewProcessWithContext(ctx, pid); err == nil {
			if mi, err := p.MemoryInfoWithContext(ctx); err == nil {
				total += mi.RSS
			}
		}
	}
	return total
}
