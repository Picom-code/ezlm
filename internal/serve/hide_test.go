package serve

import (
	"os/exec"
	"reflect"
	"runtime"
	"syscall"
	"testing"
)

// hideChild adds "no console window" to a command. It must add, not replace:
// a caller that already set process attributes (a process group, a job object,
// a command line) keeps them.
func TestHideChildKeepsCallerAttributes(t *testing.T) {
	cmd := exec.Command("ezlm-fake")
	attrs := &syscall.SysProcAttr{}
	cmd.SysProcAttr = attrs
	// CREATE_NEW_PROCESS_GROUP, the flag startLogged also sets. The field only
	// exists on Windows; elsewhere the test still covers the pointer below.
	setCreationFlags(attrs, 0x00000200)

	hideChild(cmd)

	if cmd.SysProcAttr != attrs {
		t.Fatalf("hideChild replaced SysProcAttr, so the caller's attributes are gone")
	}
	if got, ok := creationFlags(cmd.SysProcAttr); ok {
		if got&0x00000200 == 0 {
			t.Fatalf("CreationFlags = %#x, want CREATE_NEW_PROCESS_GROUP kept", got)
		}
		if got&0x08000000 == 0 {
			t.Fatalf("CreationFlags = %#x, want CREATE_NO_WINDOW added", got)
		}
	}
}

// A command with no attributes yet gets them: no panic, and on Windows the
// console stays hidden. On macOS and Linux a child has no console window of
// its own to hide, so the no-op leaves the command untouched.
func TestHideChildNilAttributes(t *testing.T) {
	cmd := exec.Command("ezlm-fake")

	hideChild(cmd)

	if runtime.GOOS != "windows" {
		if cmd.SysProcAttr != nil {
			t.Fatalf("SysProcAttr = %+v, want nil off Windows", cmd.SysProcAttr)
		}
		return
	}
	if cmd.SysProcAttr == nil {
		t.Fatalf("SysProcAttr is nil, want HideWindow and CREATE_NO_WINDOW")
	}
	if v := reflect.ValueOf(cmd.SysProcAttr).Elem().FieldByName("HideWindow"); !v.IsValid() || !v.Bool() {
		t.Fatalf("SysProcAttr = %+v, want HideWindow", cmd.SysProcAttr)
	}
	if got, ok := creationFlags(cmd.SysProcAttr); !ok || got&0x08000000 == 0 {
		t.Fatalf("CreationFlags = %#x, want CREATE_NO_WINDOW", got)
	}
}

// setCreationFlags sets SysProcAttr.CreationFlags when the platform has it.
func setCreationFlags(a *syscall.SysProcAttr, flags uint32) {
	v := reflect.ValueOf(a).Elem().FieldByName("CreationFlags")
	if v.IsValid() {
		v.SetUint(uint64(flags))
	}
}

func creationFlags(a *syscall.SysProcAttr) (uint32, bool) {
	v := reflect.ValueOf(a).Elem().FieldByName("CreationFlags")
	if !v.IsValid() {
		return 0, false
	}
	return uint32(v.Uint()), true
}
