// Package prereq checks (and where safe, fixes) what a machine needs before
// llama.cpp can run: runtimes, driver versions, OS versions.
package prereq

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/picom-code/ezlm/internal/scan"
)

type Status string

const (
	OK   Status = "ok"
	Fix  Status = "fix"  // EZLM can fix it automatically
	Warn Status = "warn" // works, but worse than it could (e.g. falls back to Vulkan)
	Fail Status = "fail" // can't run until the user acts
)

type Check struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
	Action string `json:"action,omitempty"` // what EZLM will do, or what the user should do
	Link   string `json:"link,omitempty"`
}

// Run checks everything relevant to this machine.
func Run(ctx context.Context, h *scan.Hardware) []Check {
	var out []Check
	switch runtime.GOOS {
	case "windows":
		out = append(out, vcRedist(ctx))
	case "darwin":
		out = append(out, macOS(ctx))
	case "linux":
		out = append(out, glibc(ctx, h))
	}
	if h.Backend == scan.CUDA {
		out = append(out, nvidiaDriver(h))
	}
	return out
}

func run(ctx context.Context, name string, args ...string) string {
	cmd := exec.CommandContext(ctx, name, args...)
	hide(cmd)
	b, _ := cmd.CombinedOutput()
	return string(b)
}

// version compares dotted versions: a >= b.
func atLeast(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x > y
		}
	}
	return true
}

// llama.cpp's Windows builds link the MSVC runtime dynamically and don't
// ship it (MSVCP140 / VCRUNTIME140 / VCRUNTIME140_1).
const vcMin = "14.51.36247"

func vcArch() string {
	if runtime.GOARCH == "arm64" {
		return "arm64"
	}
	return "x64"
}

// VCRedistURL is Microsoft's permanent link to the latest redistributable.
func VCRedistURL() string { return "https://aka.ms/vc14/vc_redist." + vcArch() + ".exe" }

func vcRedist(ctx context.Context) Check {
	c := Check{ID: "vcredist", Name: "Microsoft Visual C++ runtime"}
	out := run(ctx, "reg", "query", `HKLM\SOFTWARE\Microsoft\VisualStudio\14.0\VC\Runtimes\`+vcArch(), "/v", "Version")
	m := regexp.MustCompile(`v(\d+\.\d+\.\d+)`).FindStringSubmatch(out)
	switch {
	case m == nil:
		c.Status, c.Detail = Fix, "Not installed. The AI engine needs it."
		c.Action = "EZLM will install it from Microsoft (Windows asks for permission once)."
	case !atLeast(m[1], vcMin):
		c.Status, c.Detail = Fix, fmt.Sprintf("Version %s is older than the engine needs (%s).", m[1], vcMin)
		c.Action = "EZLM will update it from Microsoft (Windows asks for permission once)."
	default:
		c.Status, c.Detail = OK, "Version "+m[1]
	}
	c.Link = VCRedistURL()
	return c
}

func macOS(ctx context.Context) Check {
	v := strings.TrimSpace(run(ctx, "sw_vers", "-productVersion"))
	c := Check{ID: "macos", Name: "macOS version", Detail: "macOS " + v}
	if atLeast(v, "13.3") {
		c.Status = OK
	} else {
		c.Status, c.Action = Fail, "Update to macOS 13.3 or newer in System Settings > General > Software Update."
	}
	return c
}

func glibc(ctx context.Context, h *scan.Hardware) Check {
	out := run(ctx, "ldd", "--version")
	m := regexp.MustCompile(`(\d+\.\d+)\s*$`).FindStringSubmatch(strings.SplitN(out, "\n", 2)[0])
	c := Check{ID: "glibc", Name: "System libraries (glibc)"}
	need := "2.34" // CPU and Vulkan x64 builds (Ubuntu 22.04+)
	if h.Backend == scan.CUDA || runtime.GOARCH == "arm64" {
		need = "2.38" // CUDA and arm64 builds (Ubuntu 24.04+)
	}
	if m == nil {
		c.Status, c.Detail = Warn, "Couldn't read the glibc version."
		return c
	}
	c.Detail = "glibc " + m[1]
	if atLeast(m[1], need) {
		c.Status = OK
	} else {
		c.Status, c.Action = Fail, "This Linux is too old for the engine (needs glibc "+need+", e.g. Ubuntu 24.04)."
	}
	return c
}

func nvidiaDriver(h *scan.Hardware) Check {
	c := Check{ID: "nvidia", Name: "NVIDIA driver"}
	if len(h.GPUs) == 0 {
		return c
	}
	g := h.GPUs[0]
	c.Detail = fmt.Sprintf("Driver %s, supports CUDA up to %s", g.Driver, h.CUDAMax)
	switch {
	case h.CUDAMax == "":
		c.Status, c.Detail = Warn, "Couldn't read the driver's CUDA version; EZLM will use the Vulkan engine."
	case !atLeast(h.CUDAMax, "12.4"):
		c.Status, c.Action = Warn, "Your driver is too old for the CUDA engine, so EZLM uses Vulkan (slower). Update your driver for full speed."
	case NeedsCUDA13(h) && !atLeast(h.CUDAMax, "13.0"):
		c.Status, c.Action = Warn, "Your graphics card needs a newer driver (CUDA 13) for full speed; EZLM uses Vulkan until then."
	case strings.Contains(strings.ToLower(g.Name), "n1x") && !atLeast(g.Driver, "616.41"):
		c.Status, c.Action = Warn, "RTX Spark needs driver 616.41 or newer for the CUDA engine."
	default:
		c.Status = OK
	}
	if c.Status != OK {
		c.Link = "https://www.nvidia.com/en-us/drivers/"
	}
	return c
}

// NeedsCUDA13 is true for GPUs whose code only ships in llama.cpp's CUDA 13
// builds (Blackwell: RTX 50-series, RTX Spark / GB10).
func NeedsCUDA13(h *scan.Hardware) bool {
	for _, g := range h.GPUs {
		n := strings.ToLower(g.Name)
		for _, k := range []string{"rtx 50", "rtx pro 6000", "blackwell", "n1x", "gb10", "spark", "b200", "b100"} {
			if strings.Contains(n, k) {
				return true
			}
		}
	}
	return false
}

// FixVCRedist downloads Microsoft's redistributable and installs it silently.
// Windows shows its own permission prompt; exit 3010 (reboot later) and 1638
// (newer already installed) both count as success.
func FixVCRedist(ctx context.Context, download func(url, dst string) error, tmpDir string) error {
	dst := tmpDir + `\vc_redist.` + vcArch() + `.exe`
	if err := download(VCRedistURL(), dst); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, dst, "/install", "/quiet", "/norestart")
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok && (ee.ExitCode() == 3010 || ee.ExitCode() == 1638) {
		return nil
	}
	return err
}
