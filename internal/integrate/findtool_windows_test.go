package integrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Registry PATH values hold quotes, blanks and unexpanded %VARS%.
func TestSplitWinPath(t *testing.T) {
	t.Setenv("EZLMTESTDIR", `C:\Tools`)
	got := splitWinPath(`C:\A;;"C:\Program Files\B";%EZLMTESTDIR%\bin; `)
	want := []string{`C:\A`, `C:\Program Files\B`, `C:\Tools\bin`}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q", got)
	}
	if winVars("%A%x%B%") != "${A}x${B}" || winVars("50%") != "50%" {
		t.Fatal(winVars("%A%x%B%"))
	}
}

// An MSIX alias in WindowsApps is found even when it's a zero-byte file
// that os.Stat can't follow, and is reported as an alias.
func TestWindowsAppsAlias(t *testing.T) {
	la := t.TempDir()
	t.Setenv("LOCALAPPDATA", la)
	wa := filepath.Join(la, "Microsoft", "WindowsApps")
	os.MkdirAll(wa, 0o755)
	alias := filepath.Join(wa, "hermes.exe")
	os.WriteFile(alias, nil, 0o755)
	t.Setenv("EZLM_TOOL_PATH", wa)
	if got := FindTool("hermes"); got != alias || !IsAppAlias(got) {
		t.Fatalf("got %q", got)
	}
	found := false
	for _, d := range toolDirs() {
		found = found || d == wa
	}
	if !found {
		t.Fatalf("WindowsApps is not searched: %v", toolDirs())
	}
}

func TestMachineArchKnown(t *testing.T) {
	if a := MachineArch(); a != "amd64" && a != "arm64" {
		t.Fatalf("MachineArch = %q", a)
	}
}

// The Machine PATH in the registry always holds System32.
func TestRegistryPathRead(t *testing.T) {
	found := false
	for _, d := range userPath() {
		found = found || strings.EqualFold(filepath.Base(d), "System32")
	}
	if !found {
		t.Fatalf("userPath = %v", userPath())
	}
}
