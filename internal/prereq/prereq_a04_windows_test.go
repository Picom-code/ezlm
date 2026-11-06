//go:build windows

package prereq

import (
	"os/exec"
	"testing"
)

func TestHideSetsWindowFlags(t *testing.T) {
	cmd := exec.Command("cmd.exe")
	hide(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("hide() left SysProcAttr nil")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Error("hide() did not set HideWindow")
	}
	if cmd.SysProcAttr.CreationFlags != 0x08000000 {
		t.Errorf("CreationFlags = %#x, want 0x08000000 (CREATE_NO_WINDOW)", cmd.SysProcAttr.CreationFlags)
	}
}
