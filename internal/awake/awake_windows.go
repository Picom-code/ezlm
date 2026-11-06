package awake

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32  = windows.NewLazySystemDLL("kernel32.dll")
	createReq = kernel32.NewProc("PowerCreateRequest")
	setReq    = kernel32.NewProc("PowerSetRequest")
	clearReq  = kernel32.NewProc("PowerClearRequest")
)

// REASON_CONTEXT with a simple reason string; the padding covers the union's
// larger (detailed) form.
type reasonContext struct {
	version uint32
	flags   uint32
	reason  *uint16
	_       [2]uintptr
}

const (
	simpleString   = 0x1 // POWER_REQUEST_CONTEXT_SIMPLE_STRING
	systemRequired = 1   // PowerRequestSystemRequired
)

// A power request (unlike SetThreadExecutionState) isn't tied to one thread,
// shows up with its reason in "powercfg /requests", and Windows drops it when
// EZLM exits.
func hold(why string) func() {
	s, err := windows.UTF16PtrFromString(why)
	if err != nil || createReq.Find() != nil {
		return func() {}
	}
	rc := reasonContext{flags: simpleString, reason: s}
	h, _, _ := createReq.Call(uintptr(unsafe.Pointer(&rc)))
	if h == 0 || windows.Handle(h) == windows.InvalidHandle {
		return func() {}
	}
	if ok, _, _ := setReq.Call(h, systemRequired); ok == 0 {
		windows.CloseHandle(windows.Handle(h))
		return func() {}
	}
	return func() {
		clearReq.Call(h, systemRequired)
		windows.CloseHandle(windows.Handle(h))
	}
}
