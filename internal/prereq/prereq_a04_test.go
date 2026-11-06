package prereq

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/scan"
)

// fakeTool puts an executable shell script named name at the front of PATH.
func fakeTool(t *testing.T, name, body string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestAtLeast(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"13.3", "13.3", true},
		{"13.4", "13.3", true},
		{"13.2", "13.3", false},
		{"13.10", "13.9", true}, // numeric, not lexical
		{"13.3", "13.3.0", true},
		{"13.3.0", "13.3", true},
		{"14.51.36247", "14.51.36247", true},
		{"14.51.36246", "14.51.36247", false},
		{"", "13.3", false},
		{"13.3", "", true},
		{"abc", "1", false},
		{"1", "abc", true},
	}
	for _, c := range cases {
		if got := atLeast(c.a, c.b); got != c.want {
			t.Errorf("atLeast(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestVCArchAndURL(t *testing.T) {
	want := "x64"
	if runtime.GOARCH == "arm64" {
		want = "arm64"
	}
	if got := vcArch(); got != want {
		t.Errorf("vcArch() = %q, want %q", got, want)
	}
	u := VCRedistURL()
	if !strings.HasPrefix(u, "https://aka.ms/vc14/vc_redist.") || !strings.HasSuffix(u, ".exe") {
		t.Errorf("VCRedistURL() = %q, want a Microsoft vc14 .exe link", u)
	}
	if !strings.Contains(u, want) {
		t.Errorf("VCRedistURL() = %q, want arch %q in it", u, want)
	}
}

func TestVCRedist(t *testing.T) {
	cases := []struct {
		name       string
		out        string
		wantStatus Status
		wantDetail string
	}{
		{"current", "HKEY_LOCAL_MACHINE\\...\\Runtimes\\x64\n    Version    REG_SZ    v14.51.36247\n", OK, "Version 14.51.36247"},
		{"newer", "    Version    REG_SZ    v14.60.0\n", OK, "Version 14.60.0"},
		{"older", "    Version    REG_SZ    v14.40.33810\n", Fix, "Version 14.40.33810 is older"},
		{"missing", "ERROR: The system was unable to find the specified registry key or value.\n", Fix, "Not installed"},
		{"no v prefix", "    Version    REG_SZ    14.51.36247\n", Fix, "Not installed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeTool(t, "reg", "printf '%s\\n' '"+c.out+"'\n")
			got := vcRedist(context.Background())
			if got.ID != "vcredist" || got.Name != "Microsoft Visual C++ runtime" {
				t.Errorf("check identity = %+v", got)
			}
			if got.Status != c.wantStatus {
				t.Errorf("Status = %q, want %q (detail %q)", got.Status, c.wantStatus, got.Detail)
			}
			if !strings.Contains(got.Detail, c.wantDetail) {
				t.Errorf("Detail = %q, want it to contain %q", got.Detail, c.wantDetail)
			}
			if got.Status == Fix && got.Action == "" {
				t.Error("Fix check has no Action")
			}
			if got.Link != VCRedistURL() {
				t.Errorf("Link = %q, want %q", got.Link, VCRedistURL())
			}
		})
	}
}

func TestMacOS(t *testing.T) {
	cases := []struct {
		name       string
		out        string
		wantStatus Status
	}{
		{"supported", "13.3\n", OK},
		{"newer", "15.1\n", OK},
		{"too old", "12.7.4\n", Fail},
		{"unreadable", "", Fail},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.out == "" {
				t.Setenv("PATH", t.TempDir()) // no sw_vers
			} else {
				fakeTool(t, "sw_vers", "printf '%s' '"+c.out+"'\n")
			}
			got := macOS(context.Background())
			if got.ID != "macos" {
				t.Errorf("ID = %q, want macos", got.ID)
			}
			if got.Status != c.wantStatus {
				t.Errorf("Status = %q, want %q (detail %q)", got.Status, c.wantStatus, got.Detail)
			}
			if c.wantStatus == Fail && got.Action == "" {
				t.Error("Fail check has no Action")
			}
		})
	}
}

func TestGlibc(t *testing.T) {
	arm := runtime.GOARCH == "arm64"
	cases := []struct {
		name    string
		out     string
		backend scan.Backend
		want    Status
	}{
		{"new enough for every build", "ldd (Ubuntu GLIBC 2.39-0ubuntu8) 2.39\n", scan.CPU, OK},
		{"new enough for CPU, old for CUDA", "ldd (Ubuntu GLIBC 2.35-0ubuntu3) 2.35\n", scan.CPU, pick(arm, Fail, OK)},
		{"old for CUDA", "ldd (Ubuntu GLIBC 2.35-0ubuntu3) 2.35\n", scan.CUDA, Fail},
		{"new enough for CUDA", "ldd (Ubuntu GLIBC 2.38-1ubuntu6) 2.38\n", scan.CUDA, OK},
		{"unreadable", "ldd: not found\n", scan.CPU, Warn},
		{"version only on a later line", "garbage\n2.39\n", scan.CPU, Warn},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeTool(t, "ldd", "printf '%s\\n' '"+c.out+"'\n")
			got := glibc(context.Background(), &scan.Hardware{Backend: c.backend})
			if got.ID != "glibc" {
				t.Errorf("ID = %q, want glibc", got.ID)
			}
			if got.Status != c.want {
				t.Errorf("Status = %q, want %q (detail %q)", got.Status, c.want, got.Detail)
			}
			if c.want == OK && !strings.Contains(got.Detail, "glibc 2.") {
				t.Errorf("Detail = %q, want the glibc version", got.Detail)
			}
			if c.want == Fail && got.Action == "" {
				t.Error("Fail check has no Action")
			}
		})
	}
}

func pick(cond bool, a, b Status) Status {
	if cond {
		return a
	}
	return b
}

func TestNvidiaDriver(t *testing.T) {
	cases := []struct {
		name       string
		hw         scan.Hardware
		wantStatus Status
		wantDetail string
		wantAction string
	}{
		{"no GPUs", scan.Hardware{}, "", "", ""},
		{"no CUDA version", scan.Hardware{GPUs: []scan.GPU{{Name: "RTX 4090", Driver: "560.1"}}}, Warn, "Couldn't read the driver's CUDA version", ""},
		{"old driver", scan.Hardware{GPUs: []scan.GPU{{Name: "RTX 4090", Driver: "550.1"}}, CUDAMax: "12.3"}, Warn, "supports CUDA up to 12.3", "too old"},
		{"driver fine", scan.Hardware{GPUs: []scan.GPU{{Name: "RTX 4090", Driver: "560.1"}}, CUDAMax: "12.4"}, OK, "supports CUDA up to 12.4", ""},
		{"blackwell needs CUDA 13", scan.Hardware{GPUs: []scan.GPU{{Name: "NVIDIA GeForce RTX 5090", Driver: "570.1"}}, CUDAMax: "12.8"}, Warn, "", "CUDA 13"},
		{"blackwell on CUDA 13", scan.Hardware{GPUs: []scan.GPU{{Name: "NVIDIA GeForce RTX 5090", Driver: "580.1"}}, CUDAMax: "13.0"}, OK, "", ""},
		{"spark needs new driver", scan.Hardware{GPUs: []scan.GPU{{Name: "NVIDIA RTX Spark N1X", Driver: "616.0"}}, CUDAMax: "13.0"}, Warn, "", "616.41"},
		{"spark driver fine", scan.Hardware{GPUs: []scan.GPU{{Name: "NVIDIA RTX Spark N1X", Driver: "616.41"}}, CUDAMax: "13.0"}, OK, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := nvidiaDriver(&c.hw)
			if got.ID != "nvidia" || got.Name != "NVIDIA driver" {
				t.Errorf("check identity = %+v", got)
			}
			if got.Status != c.wantStatus {
				t.Errorf("Status = %q, want %q (detail %q)", got.Status, c.wantStatus, got.Detail)
			}
			if c.wantDetail != "" && !strings.Contains(got.Detail, c.wantDetail) {
				t.Errorf("Detail = %q, want it to contain %q", got.Detail, c.wantDetail)
			}
			if c.wantAction != "" && !strings.Contains(got.Action, c.wantAction) {
				t.Errorf("Action = %q, want it to contain %q", got.Action, c.wantAction)
			}
			if c.wantStatus != OK && c.wantStatus != "" && got.Link == "" {
				t.Error("non-OK check has no Link")
			}
			if c.wantStatus == OK && got.Link != "" {
				t.Errorf("OK check has Link %q", got.Link)
			}
		})
	}
}

func TestNeedsCUDA13(t *testing.T) {
	yes := []string{"NVIDIA GeForce RTX 5090", "RTX PRO 6000", "Blackwell", "NVIDIA GB10", "RTX Spark", "B200", "B100", "rtx 5070 Ti"}
	for _, name := range yes {
		if !NeedsCUDA13(&scan.Hardware{GPUs: []scan.GPU{{Name: name}}}) {
			t.Errorf("NeedsCUDA13(%q) = false, want true", name)
		}
	}
	no := []string{"RTX 4090", "RTX 3090", "AMD Radeon RX 7800 XT", "Apple M3 Max", ""}
	for _, name := range no {
		if NeedsCUDA13(&scan.Hardware{GPUs: []scan.GPU{{Name: name}}}) {
			t.Errorf("NeedsCUDA13(%q) = true, want false", name)
		}
	}
	if NeedsCUDA13(&scan.Hardware{}) {
		t.Error("NeedsCUDA13(no GPUs) = true")
	}
	multi := &scan.Hardware{GPUs: []scan.GPU{{Name: "RTX 4090"}, {Name: "RTX 5090"}}}
	if !NeedsCUDA13(multi) {
		t.Error("NeedsCUDA13(multi-GPU with one Blackwell) = false")
	}
}

func TestRunPicksChecksForThisOS(t *testing.T) {
	wantID := ""
	switch runtime.GOOS {
	case "windows":
		fakeTool(t, "reg", "printf '%s\\n' '    Version    REG_SZ    v14.51.36247'\n")
		wantID = "vcredist"
	case "darwin":
		fakeTool(t, "sw_vers", "printf '%s\\n' '14.5'\n")
		wantID = "macos"
	case "linux":
		fakeTool(t, "ldd", "printf '%s\\n' 'ldd (Ubuntu GLIBC 2.39-0ubuntu8) 2.39'\n")
		wantID = "glibc"
	}
	checks := Run(context.Background(), &scan.Hardware{Backend: scan.CPU})
	if len(checks) == 0 || checks[0].ID != wantID {
		t.Fatalf("Run() checks = %+v, want first ID %q", checks, wantID)
	}
	for _, c := range checks {
		if c.Status == "" {
			t.Errorf("check %q has an empty status", c.ID)
		}
	}
}

func TestRunAddsNvidiaCheckOnCUDA(t *testing.T) {
	switch runtime.GOOS {
	case "windows":
		fakeTool(t, "reg", "printf '%s\\n' '    Version    REG_SZ    v14.51.36247'\n")
	case "darwin":
		fakeTool(t, "sw_vers", "printf '%s\\n' '14.5'\n")
	case "linux":
		fakeTool(t, "ldd", "printf '%s\\n' 'ldd (Ubuntu GLIBC 2.39-0ubuntu8) 2.39'\n")
	}
	hw := &scan.Hardware{
		Backend: scan.CUDA,
		GPUs:    []scan.GPU{{Name: "NVIDIA GeForce RTX 4090", Driver: "560.1"}},
		CUDAMax: "12.8",
	}
	checks := Run(context.Background(), hw)
	last := checks[len(checks)-1]
	if last.ID != "nvidia" || last.Status != OK {
		t.Fatalf("last check = %+v, want an OK nvidia check", last)
	}
}

func TestHideDoesNotPanic(t *testing.T) {
	cmd := exec.Command("true")
	hide(cmd) // no-op off Windows; sets window flags on Windows
}

func TestFixVCRedist(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake redistributable is a shell script")
	}
	cases := []struct {
		name    string
		exit    int
		dlErr   error
		wantErr bool
	}{
		{"success", 0, nil, false},
		{"reboot later counts as success", 3010, nil, false},
		{"already installed counts as success", 1638, nil, false},
		{"installer failure", 1, nil, true},
		{"installer missing", -1, nil, true},
		{"download failure", 0, errors.New("no network"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if (c.exit == 3010 || c.exit == 1638) && runtime.GOOS != "windows" {
				t.Skip("exit codes above 255 are truncated off Windows")
			}
			var gotURL string
			download := func(url, dst string) error {
				gotURL = url
				if c.dlErr != nil {
					return c.dlErr
				}
				if c.exit < 0 {
					return nil // download claims success but leaves no file
				}
				body := "#!/bin/sh\nexit " + strconv.Itoa(c.exit) + "\n"
				return os.WriteFile(dst, []byte(body), 0o755)
			}
			err := FixVCRedist(context.Background(), download, t.TempDir())
			if gotURL != VCRedistURL() {
				t.Errorf("download URL = %q, want %q", gotURL, VCRedistURL())
			}
			if (err != nil) != c.wantErr {
				t.Errorf("FixVCRedist error = %v, want error %v", err, c.wantErr)
			}
			if c.dlErr != nil && !errors.Is(err, c.dlErr) {
				t.Errorf("FixVCRedist error = %v, want it to wrap %v", err, c.dlErr)
			}
		})
	}
}
