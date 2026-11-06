package scan

import (
	"context"
	"os/user"
	"strings"
	"sync"
	"syscall"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/windows"
)

var (
	user32           = windows.NewLazySystemDLL("user32.dll")
	procGetWindow    = user32.NewProc("GetWindow")
	procPostMessageW = user32.NewProc("PostMessageW")
	versionDLL       = windows.NewLazySystemDLL("version.dll")
	procVerSize      = versionDLL.NewProc("GetFileVersionInfoSizeW")
	procVerInfo      = versionDLL.NewProc("GetFileVersionInfoW")
	procVerQuery     = versionDLL.NewProc("VerQueryValueW")
)

const (
	gwOwner = 4
	wmClose = 0x0010
)

// sameUser reports whether two spellings name the same account. Windows
// names one account "MACHINE\name" through one API and "AzureAD\name" or
// "MicrosoftAccount\name" through another, so compare the account part and
// not the prefix.
func sameUser(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return strings.EqualFold(a, b) || strings.EqualFold(accountPart(a), accountPart(b))
}

// accountPart is an account name without its "DOMAIN\" prefix.
func accountPart(s string) string {
	if i := strings.LastIndexByte(s, '\\'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// ownedByMe: the process runs as the current Windows user.
func ownedByMe(ctx context.Context, p *process.Process) bool {
	u, err := p.UsernameWithContext(ctx)
	if err != nil {
		return false
	}
	me, err := user.Current()
	return err == nil && sameUser(u, me.Username)
}

func listProcs(ctx context.Context) []proc {
	me := ""
	if u, err := user.Current(); err == nil {
		me = u.Username
	}
	ps, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil
	}
	out := make([]proc, 0, len(ps))
	for _, p := range ps {
		exe, err := p.ExeWithContext(ctx)
		if err != nil || exe == "" {
			continue // system processes we can't even inspect
		}
		pr := proc{pid: p.Pid, exe: exe}
		if u, err := p.UsernameWithContext(ctx); err == nil && me != "" {
			pr.mine = sameUser(u, me)
		}
		if mi, err := p.MemoryInfoWithContext(ctx); err == nil {
			// Private memory is what closing it gives back (the working set
			// also counts pages shared with other processes).
			pr.ram = mi.RSS
			if mi.VMS > 0 && mi.VMS < pr.ram {
				pr.ram = mi.VMS
			}
		}
		out = append(out, pr)
	}
	return out
}

// gpuUsage reads dedicated graphics memory per process from the "GPU Process
// Memory" performance counters (nvidia-smi doesn't report it under WDDM).
func gpuUsage(ctx context.Context) map[int32]uint64 {
	return windowsGPUUsage(ctx)
}

// Windows allows a limited number of callbacks per process, so the window
// enumerator is created once and its results collected under a lock.
var (
	enumMu  sync.Mutex
	enumOut map[int32][]windows.HWND
	enumCB  = syscall.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		if windows.IsWindowVisible(h) {
			if owner, _, _ := procGetWindow.Call(uintptr(h), gwOwner); owner == 0 {
				var pid uint32
				windows.GetWindowThreadProcessId(h, &pid)
				enumOut[int32(pid)] = append(enumOut[int32(pid)], h)
			}
		}
		return 1 // keep going
	})
)

// visibleTopWindows maps each process to its visible, unowned top-level windows.
func visibleTopWindows() map[int32][]windows.HWND {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumOut = map[int32][]windows.HWND{}
	windows.EnumWindows(enumCB, nil)
	out := enumOut
	enumOut = nil
	return out
}

func windowPIDs() map[int32]bool {
	out := map[int32]bool{}
	for pid := range visibleTopWindows() {
		out[pid] = true
	}
	return out
}

// closeGently asks each of the app's windows to close, like clicking its X.
func closeGently(a *App) {
	wins := visibleTopWindows()
	for _, pid := range a.PIDs {
		for _, h := range wins[pid] {
			procPostMessageW.Call(uintptr(h), wmClose, 0, 0)
		}
	}
}

// Windows can only close an app gently through its windows.
func gracefulWithoutWindow() bool { return false }

// killPID force-closes a process, after checking it's still the same program.
func killPID(pid int32, exe string) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return
	}
	if cur, err := p.Exe(); err != nil || !strings.EqualFold(installRoot(cur, osGetenv), installRoot(exe, osGetenv)) {
		return // the PID was reused by something else
	}
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return
	}
	windows.TerminateProcess(h, 1)
	windows.CloseHandle(h)
}

// fileDescription reads the program's own name from its version resource
// ("Google Chrome", "Discord").
func fileDescription(exe string) string {
	p, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return ""
	}
	size, _, _ := procVerSize.Call(uintptr(unsafe.Pointer(p)), 0)
	if size == 0 || size > 1<<20 {
		return ""
	}
	buf := make([]byte, size)
	if ok, _, _ := procVerInfo.Call(uintptr(unsafe.Pointer(p)), 0, size, uintptr(unsafe.Pointer(&buf[0]))); ok == 0 {
		return ""
	}
	base := uintptr(unsafe.Pointer(&buf[0]))
	// VerQueryValue returns a pointer into buf; work with offsets into buf so
	// nothing outlives or escapes the Go-managed buffer.
	query := func(path string) (off, n int) {
		q, _ := windows.UTF16PtrFromString(path)
		var ptr uintptr
		var cnt uint32
		if ok, _, _ := procVerQuery.Call(base, uintptr(unsafe.Pointer(q)), uintptr(unsafe.Pointer(&ptr)), uintptr(unsafe.Pointer(&cnt))); ok == 0 || cnt == 0 || ptr < base || ptr >= base+uintptr(len(buf)) {
			return -1, 0
		}
		return int(ptr - base), int(cnt)
	}
	u16 := func(off int) uint16 { return uint16(buf[off]) | uint16(buf[off+1])<<8 }
	langs := []string{"040904b0", "040904e4", "000004b0"}
	if off, n := query(`\VarFileInfo\Translation`); off >= 0 && n >= 4 && off+4 <= len(buf) {
		langs = append([]string{hex4(u16(off)) + hex4(u16(off+2))}, langs...)
	}
	for _, l := range langs {
		for _, field := range []string{"FileDescription", "ProductName"} {
			off, n := query(`\StringFileInfo\` + l + `\` + field)
			if off < 0 {
				continue
			}
			var chars []uint16
			for i := 0; i < n && off+2*i+1 < len(buf); i++ {
				c := u16(off + 2*i)
				if c == 0 {
					break
				}
				chars = append(chars, c)
			}
			// Count characters, not bytes: a localized name must not be
			// rejected for being long in UTF-8.
			if s := strings.TrimSpace(string(utf16.Decode(chars))); s != "" && utf8.RuneCountInString(s) < 60 {
				return s
			}
		}
	}
	return ""
}

func hex4(v uint16) string {
	const d = "0123456789abcdef"
	return string([]byte{d[v>>12&15], d[v>>8&15], d[v>>4&15], d[v&15]})
}
