package serve

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/picom-code/ezlm/internal/paths"
	"github.com/shirou/gopsutil/v4/process"
)

// serverPIDPath is where EZLM records a server it started itself (on Windows,
// where the app runs llama-server directly). A server started by launchd or
// systemd belongs to the service manager, so the file can be missing.
func serverPIDPath() string { return filepath.Join(paths.Home(), "server.pid") }

// recordServerPID writes the PID of a server EZLM started. A failed write is
// not fatal: the next start still scans for running servers.
func recordServerPID(pid int) {
	os.MkdirAll(paths.Home(), 0o755)
	os.WriteFile(serverPIDPath(), []byte(strconv.Itoa(pid)+"\n"), 0o600)
}

// forgetServerPID removes the record. Missing is fine.
func forgetServerPID() { os.Remove(serverPIDPath()) }

// serverPID reads the recorded PID, or ok=false when the record can't be
// trusted. A missing file, an empty or garbled one, a PID that no longer
// runs, and a PID now used by another program all read as "no record", and a
// dead record is removed. Callers fall back to scanning for their own server,
// and must never kill a PID this returns without checking it again.
func serverPID() (int32, bool) {
	b, err := os.ReadFile(serverPIDPath())
	if err != nil {
		return 0, false
	}
	pid, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 32)
	if err != nil || pid <= 1 {
		forgetServerPID()
		return 0, false
	}
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		forgetServerPID() // the process already exited
		return 0, false
	}
	name, _ := p.Name()
	exe, _ := p.Exe()
	if !strings.HasPrefix(name, "llama-server") || exe == "" || !isOwnExe(exe, paths.Home()) {
		forgetServerPID() // stale: the PID was reused by another program
		return 0, false
	}
	return int32(pid), true
}
