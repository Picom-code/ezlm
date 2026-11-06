package integrate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// PTY is a program running in a hidden pseudo-terminal (the TUI smoke test
// and cmd/tuiprobe): a ConPTY, with the program in a job object so Close
// stops its whole process tree. Ours only, never anything found by name.
type PTY struct {
	in, out *os.File // our ends: the program's input, the terminal's output
	console windows.Handle
	process windows.Handle
	job     windows.Handle
	pid     int
	conOnce sync.Once
	once    sync.Once
	done    chan struct{}
	code    int
	err     error
}

// UpdateProcThreadAttribute takes the console handle itself as the value,
// which x/sys's wrapper (an unsafe.Pointer) can't express safely.
var ptyUpdateAttr = windows.NewLazySystemDLL("kernel32.dll").NewProc("UpdateProcThreadAttribute")

// StartPTY starts cmd in a cols x rows pseudo-terminal. cmd is only read
// (Path, Args, Env, Dir), never started; a .cmd or .bat program runs through
// cmd.exe.
func StartPTY(cmd *exec.Cmd, cols, rows int) (*PTY, error) {
	if cmd.Err != nil {
		return nil, cmd.Err
	}
	app, line := ptyCommandLine(cmd)
	appPtr, err := windows.UTF16PtrFromString(app)
	if err != nil {
		return nil, err
	}
	linePtr, err := windows.UTF16PtrFromString(line)
	if err != nil {
		return nil, err
	}
	var dirPtr *uint16
	if cmd.Dir != "" {
		if dirPtr, err = windows.UTF16PtrFromString(cmd.Dir); err != nil {
			return nil, err
		}
	}
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	block := ptyEnvBlock(env)

	var ptyIn, inW, outR, ptyOut windows.Handle
	if err := windows.CreatePipe(&ptyIn, &inW, nil, 0); err != nil {
		return nil, err
	}
	if err := windows.CreatePipe(&outR, &ptyOut, nil, 0); err != nil {
		windows.CloseHandle(ptyIn)
		windows.CloseHandle(inW)
		return nil, err
	}
	var con windows.Handle
	err = windows.CreatePseudoConsole(windows.Coord{X: int16(cols), Y: int16(rows)}, ptyIn, ptyOut, 0, &con)
	// The console keeps its own copies of its ends.
	windows.CloseHandle(ptyIn)
	windows.CloseHandle(ptyOut)
	if err != nil {
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return nil, err
	}
	p := &PTY{in: os.NewFile(uintptr(inW), "pty-in"), out: os.NewFile(uintptr(outR), "pty-out"), console: con, done: make(chan struct{})}
	fail := func(err error) (*PTY, error) {
		p.closeConsole()
		p.in.Close()
		p.out.Close()
		return nil, err
	}

	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return fail(err)
	}
	defer attrs.Delete()
	if r, _, e := ptyUpdateAttr.Call(uintptr(unsafe.Pointer(attrs.List())), 0, windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, uintptr(con), unsafe.Sizeof(con), 0, 0); r == 0 {
		return fail(e)
	}
	si := windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.Cb = uint32(unsafe.Sizeof(si))
	// Null standard handles: the program uses the pseudo console, not
	// whatever EZLM's own stdin and stdout are connected to.
	si.Flags = windows.STARTF_USESTDHANDLES
	var pi windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_SUSPENDED)
	if err := windows.CreateProcess(appPtr, linePtr, nil, nil, false, flags, &block[0], dirPtr, &si.StartupInfo, &pi); err != nil {
		return fail(err)
	}
	p.process, p.pid = pi.Process, int(pi.ProcessId)
	// The job is set up before the program runs a single instruction, so
	// every process it starts is in it too.
	if job, err := windows.CreateJobObject(nil, nil); err == nil {
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
		if windows.AssignProcessToJobObject(job, pi.Process) == nil {
			p.job = job
		} else {
			windows.CloseHandle(job)
		}
	}
	windows.ResumeThread(pi.Thread)
	windows.CloseHandle(pi.Thread)
	go func() {
		windows.WaitForSingleObject(p.process, windows.INFINITE)
		var c uint32
		p.err = windows.GetExitCodeProcess(p.process, &c)
		p.code = int(int32(c))
		close(p.done)
		// Like a Unix pty when the program ends: give the console a moment
		// to pass on the last output, then close it so readers see the end.
		time.Sleep(300 * time.Millisecond)
		p.closeConsole()
	}()
	return p, nil
}

// ptyCommandLine is the program and the command line CreateProcess gets. A
// batch file runs as cmd.exe /d /s /c ""<file>" <args>".
func ptyCommandLine(cmd *exec.Cmd) (app, line string) {
	args := append([]string{cmd.Path}, cmd.Args[1:]...)
	if !launchIsBatch(cmd.Path) {
		return cmd.Path, windows.ComposeCommandLine(args)
	}
	comspec := os.Getenv("ComSpec")
	if comspec == "" {
		comspec = filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	}
	words := []string{`"` + cmd.Path + `"`}
	for _, a := range cmd.Args[1:] {
		words = append(words, launchCmdArg(a, 0))
	}
	return comspec, `cmd.exe /d /s /c "` + strings.Join(words, " ") + `"`
}

// ptyEnvBlock is env as a Windows environment block: the last value of each
// name wins (names ignore case), SYSTEMROOT is always there.
func ptyEnvBlock(env []string) []uint16 {
	// A name may start with = (cmd.exe's per-drive folders, "=C:=C:\x").
	name := func(e string) string {
		if i := strings.Index(e[1:], "="); i >= 0 {
			e = e[:i+1]
		}
		return strings.ToUpper(e)
	}
	seen := map[string]int{}
	var kept []string
	for _, e := range env {
		if e == "" || strings.ContainsRune(e, 0) {
			continue
		}
		if i, ok := seen[name(e)]; ok {
			kept[i] = e
			continue
		}
		seen[name(e)] = len(kept)
		kept = append(kept, e)
	}
	if _, ok := seen["SYSTEMROOT"]; !ok {
		if v := os.Getenv("SYSTEMROOT"); v != "" {
			kept = append(kept, "SYSTEMROOT="+v)
		}
	}
	var b []uint16
	for _, e := range kept {
		b = append(b, utf16.Encode([]rune(e))...)
		b = append(b, 0)
	}
	if len(b) == 0 {
		b = append(b, 0)
	}
	return append(b, 0)
}

// closeConsole ends the pseudo console. On older Windows builds that waits
// until its output is read, so it gives up after a few seconds rather than
// hang a caller that stopped reading.
func (p *PTY) closeConsole() {
	p.conOnce.Do(func() {
		done := make(chan struct{})
		go func() {
			windows.ClosePseudoConsole(p.console)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	})
}

func (p *PTY) Read(b []byte) (int, error)  { return p.out.Read(b) }
func (p *PTY) Write(b []byte) (int, error) { return p.in.Write(b) }

// Pid is the program's process id.
func (p *PTY) Pid() int { return p.pid }

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

// Close closes the terminal and kills the program's process tree (the job).
func (p *PTY) Close() error {
	p.once.Do(func() {
		if p.job != 0 {
			windows.TerminateJobObject(p.job, 1)
		} else {
			windows.TerminateProcess(p.process, 1)
		}
		select {
		case <-p.done:
			defer windows.CloseHandle(p.process)
		case <-time.After(5 * time.Second):
		}
		p.closeConsole()
		p.in.Close()
		// A read still in progress ends with the console; don't wait for it.
		go p.out.Close()
		if p.job != 0 {
			windows.CloseHandle(p.job)
		}
	})
	return nil
}
