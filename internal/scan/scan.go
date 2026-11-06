// Package scan inspects the machine: CPU, RAM, disk, GPUs, and what is
// already using GPU memory.
package scan

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/picom-code/ezlm/internal/paths"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
)

// Backend is the llama.cpp build flavor that suits this machine.
type Backend string

const (
	Metal  Backend = "metal"
	CUDA   Backend = "cuda"
	ROCm   Backend = "rocm"
	Vulkan Backend = "vulkan"
	CPU    Backend = "cpu"
)

type GPU struct {
	Name      string  `json:"name"`
	Vendor    string  `json:"vendor"` // apple | nvidia | amd | intel
	MemTotal  uint64  `json:"mem_total"`
	MemFree   uint64  `json:"mem_free"`
	Bandwidth float64 `json:"bandwidth_gbs"` // best-effort estimate, 0 if unknown
	Driver    string  `json:"driver,omitempty"`
}

type Proc struct {
	PID      int32  `json:"pid"`
	Name     string `json:"name"`
	Mem      uint64 `json:"mem"`
	Killable bool   `json:"killable"` // owned by the current user and not EZLM itself
}

type Hardware struct {
	OS        string `json:"os"`
	OSName    string `json:"os_name"` // e.g. "Windows 11 Pro 24H2", "macOS 26.3"
	Arch      string `json:"arch"`
	CPUName   string `json:"cpu_name"`
	Cores     int    `json:"cores"`      // logical (hardware threads)
	PhysCores int    `json:"phys_cores"` // physical cores
	RAMTotal  uint64 `json:"ram_total"`
	RAMAvail  uint64 `json:"ram_avail"`
	DiskFree  uint64 `json:"disk_free"`
	// ModelsDir is where weights should go: EZLM's home, or on Windows the
	// fixed drive with the most free space if that's a different one.
	ModelsDir string `json:"models_dir"`

	GPUs    []GPU   `json:"gpus"`
	Backend Backend `json:"backend"`
	// CUDAMax is the newest CUDA version the NVIDIA driver supports ("" if none).
	CUDAMax string `json:"cuda_max,omitempty"`

	// Unified is true on Apple Silicon: GPU and CPU share RAM.
	Unified bool `json:"unified"`
	// GPUUsable is how much memory the GPU may wire (Apple) or total free VRAM (discrete).
	GPUUsable uint64 `json:"gpu_usable"`
	// CPUBandwidth is an estimate of system RAM bandwidth in GB/s.
	CPUBandwidth float64 `json:"cpu_bandwidth_gbs"`

	// GPUProcs lists processes holding GPU memory (discrete GPUs only).
	GPUProcs []Proc `json:"gpu_procs"`
	// TopMem lists the biggest memory users (shown on unified-memory machines).
	TopMem []Proc `json:"top_mem"`

	Notes []string `json:"notes"`
}

const GB = 1 << 30

// Scan inspects the machine. It never fails outright; missing pieces become Notes.
func Scan(ctx context.Context) *Hardware {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	h := &Hardware{OS: runtime.GOOS, Arch: runtime.GOARCH, Cores: runtime.NumCPU(), CPUBandwidth: 50}

	if n, err := cpu.CountsWithContext(ctx, false); err == nil {
		h.PhysCores = n
	}
	if infos, err := cpu.InfoWithContext(ctx); err == nil && len(infos) > 0 {
		h.CPUName = strings.TrimSpace(infos[0].ModelName)
	}
	if runtime.GOOS == "linux" && (h.CPUName == "" || h.PhysCores == 0) {
		linuxCPUInfo(ctx, h, "/proc/cpuinfo")
	}
	h.OSName = osName(ctx)
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		h.RAMTotal, h.RAMAvail = vm.Total, vm.Available
	}
	_ = paths.Ensure()
	h.ModelsDir = filepath.Join(paths.Home(), "models")
	if du, err := disk.UsageWithContext(ctx, paths.Home()); err == nil {
		h.DiskFree = du.Free
	}
	// Where models go: the user's choice in Settings, else the folder EZLM
	// already uses (it sticks), else the roomiest drive (first setup only).
	st := paths.LoadSettings()
	auto := st.AutoDir
	if auto == "" {
		auto = paths.DeployedModelsDir() // setups from before AutoDir existed
	}
	switch {
	case st.ModelsDir != "":
		h.ModelsDir = st.ModelsDir
		h.DiskFree = FreeAt(ctx, st.ModelsDir)
	case auto != "":
		h.ModelsDir = auto
		h.DiskFree = FreeAt(ctx, auto)
	case runtime.GOOS == "windows" && os.Getenv("EZLM_HOME") == "":
		roomiestDrive(ctx, h) // a test home (EZLM_HOME) keeps its models inside it
	}

	platformScan(ctx, h)
	// EZLM_BACKEND forces cpu/vulkan/cuda/metal: lets a test exercise the
	// builds AMD, Intel and processor-only machines use.
	if b := os.Getenv("EZLM_BACKEND"); b != "" {
		h.Backend = Backend(b)
	}

	if h.Backend == "" {
		h.Backend = backendFor(h.GPUs)
	}
	if !h.Unified {
		for _, g := range h.GPUs {
			h.GPUUsable += g.MemFree
		}
	}
	if len(h.GPUProcs) == 0 || h.Unified {
		h.TopMem = topMem(ctx, 6)
	}
	return h
}

// roomiestDrive moves model storage to the fixed drive with the most space
// when it beats the home drive by a useful margin (models are tens of GB).
func roomiestDrive(ctx context.Context, h *Hardware) {
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return
	}
	home := strings.ToUpper(filepath.VolumeName(paths.Home()))
	for _, p := range parts {
		if strings.ToUpper(filepath.VolumeName(p.Mountpoint)) == home {
			continue
		}
		du, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil || du.Free < h.DiskFree+10*GB {
			continue
		}
		h.DiskFree = du.Free
		// "D:" alone is drive-relative on Windows; the root needs its separator.
		root := strings.TrimRight(p.Mountpoint, `\/`) + string(filepath.Separator)
		h.ModelsDir = filepath.Join(root, "EZLM", "models")
	}
}

// osName is a human-readable OS name and version.
func osName(ctx context.Context) string {
	info, err := host.InfoWithContext(ctx)
	if err != nil {
		return runtime.GOOS
	}
	switch info.OS {
	case "darwin":
		return "macOS " + info.PlatformVersion
	case "windows":
		// Platform is e.g. "Microsoft Windows 11 Pro".
		return strings.TrimSpace(strings.TrimPrefix(info.Platform, "Microsoft ") + " " + info.PlatformVersion)
	default:
		name := info.Platform
		if name != "" {
			name = upperFirst(name)
		}
		return strings.TrimSpace(name + " " + info.PlatformVersion)
	}
}

// backendFor picks the llama.cpp build for the cards found. Vulkan builds are
// the most reliable path on consumer AMD and Intel Arc, and on the shared-memory
// GPUs of Windows on Arm.
func backendFor(gpus []GPU) Backend {
	switch {
	case hasVendor(gpus, "nvidia"):
		return CUDA
	case hasVendor(gpus, "amd"), hasVendor(gpus, "intel"), hasVendor(gpus, "qualcomm"):
		return Vulkan
	}
	return CPU
}

func hasVendor(gpus []GPU, v string) bool {
	for _, g := range gpus {
		if g.Vendor == v {
			return true
		}
	}
	return false
}

// topMem returns the n processes using the most resident memory.
func topMem(ctx context.Context, n int) []Proc {
	ps, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil
	}
	var out []Proc
	for _, p := range ps {
		mi, err := p.MemoryInfoWithContext(ctx)
		if err != nil || mi.RSS < 200<<20 {
			continue
		}
		name, _ := p.NameWithContext(ctx)
		out = append(out, Proc{PID: p.Pid, Name: name, Mem: mi.RSS, Killable: killable(ctx, p)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mem > out[j].Mem })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// killable applies Model mode's safety rules to one process: the user's own,
// not part of the operating system, remote access or EZLM.
func killable(ctx context.Context, p *process.Process) bool {
	exe, err := p.ExeWithContext(ctx)
	if err != nil {
		return false
	}
	return closableProc(proc{pid: p.Pid, exe: exe, mine: ownedByMe(ctx, p)})
}

func procFor(ctx context.Context, pid int32, name string, memBytes uint64) Proc {
	pr := Proc{PID: pid, Name: name, Mem: memBytes}
	if p, err := process.NewProcessWithContext(ctx, pid); err == nil {
		pr.Killable = killable(ctx, p)
		if n, err := p.NameWithContext(ctx); err == nil && n != "" {
			pr.Name = n
		}
	}
	return pr
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	hide(cmd)
	out, err := cmd.Output()
	return string(out), err
}

// Kill closes one process that passes the safety rules: gently first (its
// windows are asked to close), then for real if it's still running.
func Kill(ctx context.Context, pid int32) error {
	p, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		return err
	}
	if !killable(ctx, p) {
		return os.ErrPermission
	}
	exe, _ := p.ExeWithContext(ctx)
	closeGently(&App{PIDs: []int32{pid}, Exe: exe})
	for i := 0; i < 20; i++ {
		if ok, _ := p.IsRunningWithContext(ctx); !ok {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	killPID(pid, exe)
	return nil
}

// FreeAt is the free space on the drive holding dir (which may not exist yet).
func FreeAt(ctx context.Context, dir string) uint64 {
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		if du, err := disk.UsageWithContext(ctx, d); err == nil {
			return du.Free
		}
		if filepath.Dir(d) == d {
			return 0
		}
	}
}
