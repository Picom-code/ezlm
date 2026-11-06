package scan

import (
	"context"
	"os/exec"
	"syscall"
)

// hide keeps helper programs from flashing console windows when EZLM runs as
// a GUI-subsystem app.
func hide(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

// runPS runs a PowerShell command and returns its output as UTF-8. Windows
// PowerShell writes redirected output in the console's OEM code page (cp437,
// cp850, cp932, ...), not UTF-8: a device name or an instance name with é,
// ü or 王 would reach the UI as replacement characters. Ask for UTF-8 first;
// the statement runs before the script. PowerShell 7 is UTF-8 already, so
// this only changes anything on Windows PowerShell 5.1.
func runPS(ctx context.Context, script string) (string, error) {
	const utf8Out = `[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false); `
	return run(ctx, "powershell", "-NoProfile", "-Command", utf8Out+script)
}
