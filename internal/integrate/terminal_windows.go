package integrate

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// termOpen opens a new console window running the .cmd launcher: in Windows
// Terminal when it is installed, else (or when it fails to start) in a
// classic console through cmd.exe's start.
func termOpen(spec LaunchSpec, launcher string) error {
	_, err := launchWindowsOpen(termFindWT(), spec.Title, launcher, termStartWT, termStartCmd)
	if err != nil {
		return fmt.Errorf("Windows couldn't open a console window (%s). Run %s to start %s", err, launcher, launchName(spec))
	}
	return nil
}

// termFindWT finds wt.exe on PATH or as the Store app's alias in
// WindowsApps (often missing from the PATH a GUI app inherits). "" when
// there is none.
func termFindWT() string {
	if p, err := exec.LookPath("wt.exe"); err == nil {
		return p
	}
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		p := filepath.Join(la, "Microsoft", "WindowsApps", "wt.exe")
		// The alias is a zero-byte reparse point: Lstat, not Stat.
		if _, err := os.Lstat(p); err == nil {
			return p
		}
	}
	return ""
}

// termStartWT starts Windows Terminal. A stale alias fails to start; one
// that starts but exits with an error within a few seconds counts as failed
// too, so the caller falls back to a classic console.
func termStartWT(wt string, args []string) error {
	cmd := exec.Command(wt, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		return nil
	}
}

// termStartCmd runs line (launchStartLine) in a hidden cmd.exe. start
// returns as soon as the new console is up.
func termStartCmd(line string) error {
	comspec := os.Getenv("ComSpec")
	if comspec == "" {
		comspec = filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	}
	cmd := exec.Command(comspec)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: line, HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(15 * time.Second):
		return errors.New("start didn't return")
	}
}
