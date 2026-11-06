package serve

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/picom-code/ezlm/internal/paths"
	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/windows/registry"
)

// Scheduled tasks used by older EZLM versions; removed (or, if an admin
// created them, disabled) on install.
var taskNames = []string{"EZLM Server", "EZLM Server (" + os.Getenv("USERNAME") + ")"}

// legacyTasks are the old task names this instance may end or remove. Task
// names are per user, not per EZLM folder, so a test instance with its own
// EZLM_HOME never touches them: ending one would stop the real install's
// server (it did, once).
func legacyTasks() []string {
	if os.Getenv("EZLM_HOME") != "" {
		return nil
	}
	return taskNames
}

// testInstance: an isolated install (its own EZLM_HOME, or EZLM_NO_AUTOSTART)
// leaves the real install's login entry alone.
func testInstance() bool {
	return os.Getenv("EZLM_NO_AUTOSTART") != "" || os.Getenv("EZLM_HOME") != ""
}

// activeTaskPath: file left by older versions.
func activeTaskPath() string { return filepath.Join(paths.Home(), "task-name.txt") }

// NoLoginTask is set when the login entry couldn't be written: the server
// runs, but won't come back by itself after a restart.
var NoLoginTask bool

func hidden(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return cmd
}

// The server runs llama-server.exe directly: a plainly named program with
// visible arguments. Earlier designs (a self-copied ezlm.exe, or "conhost
// --headless cmd /c script") looked like malware persistence to Defender's
// behavior models and were blocked. EZLM hands llama-server a handle to its
// log file (startLogged), so no shell is needed.

func taskXMLPath() string { return filepath.Join(paths.Home(), "ezlm-task.xml") }

func Install(c *Config) error {
	if err := StopAndWait(c); err != nil {
		return err
	}
	os.Remove(filepath.Join(paths.Bin(), "ezlm.exe"))          // old self-copy design
	os.Remove(filepath.Join(paths.Home(), "start-server.cmd")) // old script design
	os.Remove(taskXMLPath())
	// Login tasks need admin rights on Windows (a normal user gets "Access is
	// denied"), so EZLM uses the per-user Run key like any desktop app. It
	// launches EZLM's own app with --autostart, which starts the server and
	// exits; it shows up (and can be turned off) in Task Manager > Startup apps.
	// Old task-based installs are removed where we're allowed to.
	if !testInstance() {
		for _, name := range legacyTasks() {
			if hidden(exec.Command("schtasks", "/Delete", "/TN", name, "/F")).Run() != nil {
				hidden(exec.Command("schtasks", "/Change", "/TN", name, "/DISABLE")).Run()
			}
		}
	}
	os.Remove(activeTaskPath())
	NoLoginTask = setRunKey() != nil
	return Start()
}

const runKeyPath, runValue = `Software\Microsoft\Windows\CurrentVersion\Run`, "EZLM"

// setRunKey registers the login entry. The desktop app is preferred (no
// console window); a CLI-only install falls back to the engine itself.
func setRunKey() error {
	if testInstance() {
		return nil // test instance: leave the real login entry alone
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	exe, args := self, loginArgs("")
	if app := filepath.Join(filepath.Dir(filepath.Dir(self)), "EZLM.exe"); fileExists(app) {
		exe, args = app, loginArgs(app)
	}
	cmd := syscall.EscapeArg(exe) + " " + args
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(runValue, cmd)
}

// EnsureAutostart repairs the login entry for an existing setup (installs
// from older versions used a scheduled task, which normal users can't create).
func EnsureAutostart() {
	if _, err := Load(); err != nil || testInstance() {
		return
	}
	if k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE); err == nil {
		v, _, verr := k.GetStringValue(runValue)
		k.Close()
		if verr == nil && v != "" {
			return
		}
	}
	setRunKey()
	// Only disable old login tasks here: the server may be running from one
	// right now, and deleting a running task can end it. Install (which
	// restarts the server anyway) removes them for good.
	for _, name := range legacyTasks() {
		hidden(exec.Command("schtasks", "/Change", "/TN", name, "/DISABLE")).Run()
	}
}

func clearRunKey() {
	if k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE); err == nil {
		k.DeleteValue(runValue)
		k.Close()
	}
}

func fileExists(p string) bool { st, err := os.Stat(p); return err == nil && !st.IsDir() }

// Stop ends EZLM's task and its llama-server. Only processes running from
// EZLM's own folder are touched: other llama-server instances on the machine
// (other apps, the user's own setups) are never killed.
func Stop() error {
	for _, name := range legacyTasks() {
		hidden(exec.Command("schtasks", "/End", "/TN", name)).Run()
	}
	killOwn()
	return nil
}

func killOwn() {
	ctx := context.Background()
	for _, pid := range OwnPIDs(ctx) {
		if p, err := process.NewProcessWithContext(ctx, pid); err == nil {
			p.KillWithContext(ctx)
		}
	}
}

// Start runs the task now. If Task Scheduler doesn't start it (for example
// the task's user isn't logged in at the desktop), EZLM starts the server
// directly so it's available immediately; the task still covers later logins.
func Start() error {
	if len(OwnPIDs(context.Background())) > 0 {
		return nil // already running (e.g. started at login); keep its log
	}
	c, cerr := Load()
	if cerr != nil {
		return cerr
	}
	return startLogged(c.Server, c.Args(), LogPath())
}

// startLogged runs the server detached, with its output going to logPath.
// EZLM opens the log itself and hands the handle over, rather than passing
// --log-file: llama.cpp opens that with plain fopen, which on Windows can't
// open a path with letters outside the ANSI code page (C:\Users\José, Müller,
// 王), so those people would get no log and no explanations when a model
// fails to start. llama.cpp flushes every line and drops colors when its
// output isn't a terminal, so the file reads the same.
func startLogged(server string, args []string, logPath string) error {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	args, env := keylessArgs(args)
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("couldn't open the server log: %v", err)
	}
	defer logf.Close() // the server keeps its own copy of the handle
	// Prefer breaking away from the caller's job so the server outlives the
	// app (or an SSH session); some job objects forbid that, so fall back.
	const base = 0x08000000 | 0x00000008 | 0x00000200 // no window, detached, new process group
	var serr error
	for _, flags := range []uint32{base | 0x01000000, base} {
		cmd := exec.Command(server, args...)
		cmd.Dir = filepath.Dir(server)
		cmd.Env = env
		cmd.Stdout, cmd.Stderr = logf, logf
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags}
		if serr = cmd.Start(); serr == nil {
			break
		}
	}
	if serr != nil {
		return fmt.Errorf("couldn't start the server: %v", serr)
	}
	return nil
}

// keylessArgs moves the API key from the command line into the environment,
// which llama-server reads as LLAMA_API_KEY (verified in llama.cpp b11960).
// On Windows any user can read another user's process command line from Task
// Manager or Win32_Process, so a second account could otherwise use the key.
func keylessArgs(args []string) ([]string, []string) {
	out := make([]string, 0, len(args))
	env := os.Environ()
	for i := 0; i < len(args); i++ {
		if args[i] == "--api-key" && i+1 < len(args) {
			env = append(env, "LLAMA_API_KEY="+args[i+1])
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out, env
}

func Uninstall() error {
	Stop()
	if !testInstance() { // a test instance leaves the real install's login entry alone
		for _, name := range legacyTasks() {
			hidden(exec.Command("schtasks", "/Delete", "/TN", name, "/F")).Run()
		}
		clearRunKey()
	}
	os.Remove(taskXMLPath())
	os.Remove(activeTaskPath())
	return nil
}
