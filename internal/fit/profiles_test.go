package fit

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/picom-code/ezlm/internal/scan"
)

func nv(name string, vramFree, vramTotal, ramAvail, ramTotal uint64, bw float64) *scan.Hardware {
	return &scan.Hardware{Backend: scan.CUDA, RAMTotal: ramTotal * G, RAMAvail: ramAvail * G, GPUUsable: vramFree * G,
		DiskFree: 800 * G, CPUBandwidth: 60,
		GPUs: []scan.GPU{{Name: name, Vendor: "nvidia", MemTotal: vramTotal * G, MemFree: vramFree * G, Bandwidth: bw}}}
}

var realMachines = []struct {
	name string
	hw   *scan.Hardware
}{
	{"Desktop: RTX 4070 SUPER 12GB + 62GB DDR5", nv("RTX 4070 SUPER", 11, 12, 42, 62, 504)},
	{"Laptop: RTX Spark N1X (96GB GPU share, 30GB Windows)", nv("RTX Spark N1X", 90, 96, 20, 30, 273)},
	{"RTX 4090 24GB + 96GB DDR5", nv("RTX 4090", 23, 24, 80, 96, 1008)},
	{"RTX 3060 12GB + 32GB", nv("RTX 3060", 11, 12, 24, 32, 360)},
	{"Mac M3 Pro 18GB (9GB free)", mac(18, 9, 150)},
	{"Mac M4 Max 64GB (50GB free)", mac(64, 50, 546)},
	{"Mac M4 Max 128GB (110GB free)", mac(128, 110, 546)},
	{"CPU only, 16GB", &scan.Hardware{Backend: scan.CPU, RAMTotal: 16 * G, RAMAvail: 12 * G, DiskFree: 500 * G, CPUBandwidth: 50}},
}

// TestShowPicks prints what each preference picks (go test -run ShowPicks -v).
func TestShowPicks(t *testing.T) {
	if os.Getenv("SHOW") == "" {
		t.Skip("set SHOW=1 to print the pick table")
	}
	c := cat(t)
	for _, m := range realMachines {
		fmt.Printf("\n%s\n", m.name)
		for _, pref := range []Pref{Balanced, Smartest, Fastest} {
			plans := PlansFor(m.hw, c, ComputeBudgets(m.hw, m.hw.RAMAvail), pref)
			p, ok := Recommend(plans)
			if !ok {
				fmt.Printf("  %-9s nothing fits\n", pref)
				continue
			}
			fmt.Printf("  %-9s %-28s %-24s %-6s %3dk %-4s ~%5.1f tok/s  smarts %.1f  %s\n", pref, p.Model.ID, p.Quant.Name, p.Mode, p.Ctx/1024, p.KV, p.TokPerSec, p.Effective, p.Spec)
		}
	}
}

func cpuOnly(ramTotal, ramAvail uint64, bw float64, cores int) *scan.Hardware {
	return &scan.Hardware{Backend: scan.CPU, RAMTotal: ramTotal * G, RAMAvail: ramAvail * G, DiskFree: 800 * G, CPUBandwidth: bw, PhysCores: cores}
}

func multiNV(name string, n int, vramEach, ramAvail, ramTotal uint64, bw float64) *scan.Hardware {
	h := &scan.Hardware{Backend: scan.CUDA, RAMTotal: ramTotal * G, RAMAvail: ramAvail * G, DiskFree: 4000 * G, CPUBandwidth: 200}
	for i := 0; i < n; i++ {
		free := vramEach*G - G
		h.GPUs = append(h.GPUs, scan.GPU{Name: name, Vendor: "nvidia", MemTotal: vramEach * G, MemFree: free, Bandwidth: bw})
		h.GPUUsable += free
	}
	return h
}

// ladder spans the machines beta testers might have, from an old 4 GB laptop
// to a GPU server, so every size class gets a sensible pick.
var ladder = []struct {
	name string
	hw   *scan.Hardware
}{
	{"Old laptop, 4GB RAM, processor only", cpuOnly(4, 2, 17, 2)},
	{"Laptop, 8GB RAM, processor only", cpuOnly(8, 4, 34, 4)},
	{"Laptop, 16GB RAM, processor only", cpuOnly(16, 9, 50, 6)},
	{"Mac M1 8GB (4GB free)", mac(8, 4, 68)},
	{"Mac M2 16GB (10GB free)", mac(16, 10, 100)},
	{"Mac M4 Pro 24GB (16GB free)", mac(24, 16, 273)},
	{"GTX 1650 4GB + 16GB", nv("GTX 1650", 3, 4, 10, 16, 128)},
	{"RTX 3050 laptop 6GB + 16GB", nv("RTX 3050", 5, 6, 9, 16, 192)},
	{"RTX 4060 8GB + 16GB", nv("RTX 4060", 7, 8, 9, 16, 272)},
	{"RTX 4060 Ti 16GB + 32GB", nv("RTX 4060 Ti", 15, 16, 22, 32, 288)},
	{"RTX 3060 12GB + 32GB", nv("RTX 3060", 11, 12, 24, 32, 360)},
	{"RTX 4070 SUPER 12GB + 62GB", nv("RTX 4070 SUPER", 11, 12, 42, 62, 504)},
	{"RTX 4080 16GB + 32GB", nv("RTX 4080", 15, 16, 22, 32, 717)},
	{"RTX 3090 24GB + 64GB", nv("RTX 3090", 23, 24, 50, 64, 936)},
	{"RTX 4090 24GB + 96GB", nv("RTX 4090", 23, 24, 80, 96, 1008)},
	{"RTX 5090 32GB + 128GB", nv("RTX 5090", 31, 32, 110, 128, 1792)},
	{"Mac M4 Max 64GB (50GB free)", mac(64, 50, 546)},
	{"Mac M4 Max 128GB (110GB free)", mac(128, 110, 546)},
	{"RTX Spark N1X 96GB share", nv("RTX Spark N1X", 90, 96, 20, 30, 273)},
	{"RTX PRO 6000 96GB + 256GB", nv("RTX PRO 6000", 94, 96, 230, 256, 1792)},
	{"Mac Studio M3 Ultra 256GB (230GB free)", mac(256, 230, 819)},
	{"2x RTX PRO 6000 (192GB) + 512GB", multiNV("RTX PRO 6000", 2, 96, 470, 512, 1792)},
	{"Mac Studio M3 Ultra 512GB (470GB free)", mac(512, 470, 819)},
	{"EPYC server, 1.5TB RAM, processor only", cpuOnly(1536, 1450, 460, 96)},
	{"8x H200 (1.1TB) + 2TB RAM", multiNV("H200", 8, 141, 1900, 2048, 4800)},
}

// TestShowLadder prints the pick per machine size (SHOW=1 go test -run ShowLadder -v).
func TestShowLadder(t *testing.T) {
	if os.Getenv("SHOW") == "" {
		t.Skip("set SHOW=1 to print the ladder")
	}
	c := cat(t)
	for _, m := range ladder {
		fmt.Printf("\n%s\n", m.name)
		for _, pref := range []Pref{Balanced, Smartest, Fastest} {
			plans := PlansFor(m.hw, c, ComputeBudgets(m.hw, m.hw.RAMAvail), pref)
			p, ok := Recommend(plans)
			if !ok {
				fmt.Printf("  %-9s nothing fits\n", pref)
				continue
			}
			fmt.Printf("  %-9s %-30s %-22s %-6s %3dk %-4s ~%6.1f tok/s  smarts %.1f  %.1f GB\n", pref, p.Model.ID, p.Quant.Name, p.Mode, p.Ctx/1024, p.KV, p.TokPerSec, p.Effective, float64(p.Download)/1e9)
		}
	}
}

// hwProfile is one row of testdata/profiles.json, the shared hardware table.
// Memory fields are in GiB; Hardware converts them to scan.Hardware bytes.
type hwProfile struct {
	Name       string       `json:"name"`
	Family     string       `json:"family"`
	OS         string       `json:"os"`
	Arch       string       `json:"arch"`
	Backend    scan.Backend `json:"backend"`
	Unified    bool         `json:"unified"`
	RAMTotalGB uint64       `json:"ram_total_gb"`
	RAMAvailGB uint64       `json:"ram_avail_gb"`
	DiskFreeGB uint64       `json:"disk_free_gb"`
	CPUBw      float64      `json:"cpu_bandwidth_gbs"`
	PhysCores  int          `json:"phys_cores"`
	GPUUsable  uint64       `json:"gpu_usable_gb"`
	GPUs       []struct {
		Name      string  `json:"name"`
		Vendor    string  `json:"vendor"`
		MemTotal  uint64  `json:"mem_total_gb"`
		MemFree   uint64  `json:"mem_free_gb"`
		Bandwidth float64 `json:"bandwidth_gbs"`
	} `json:"gpus"`
}

// Hardware builds the scan value the planner takes from one table row.
func (p hwProfile) Hardware() *scan.Hardware {
	h := &scan.Hardware{
		OS: p.OS, Arch: p.Arch, Backend: p.Backend, Unified: p.Unified,
		RAMTotal: p.RAMTotalGB * G, RAMAvail: p.RAMAvailGB * G,
		DiskFree: p.DiskFreeGB * G, CPUBandwidth: p.CPUBw,
		PhysCores: p.PhysCores, Cores: 2 * p.PhysCores,
		GPUUsable: p.GPUUsable * G,
	}
	for _, g := range p.GPUs {
		h.GPUs = append(h.GPUs, scan.GPU{Name: g.Name, Vendor: g.Vendor,
			MemTotal: g.MemTotal * G, MemFree: g.MemFree * G, Bandwidth: g.Bandwidth})
	}
	return h
}

// loadHwProfiles reads the shared profile table. It is the one canonical
// source of test machines for the T04 fit tests.
func loadHwProfiles(t *testing.T) []hwProfile {
	t.Helper()
	data, err := os.ReadFile("testdata/profiles.json")
	if err != nil {
		t.Fatalf("read profile table: %v", err)
	}
	var table struct {
		Version  int         `json:"version"`
		Profiles []hwProfile `json:"profiles"`
	}
	if err := json.Unmarshal(data, &table); err != nil {
		t.Fatalf("parse profile table: %v", err)
	}
	if table.Version != 1 {
		t.Fatalf("profile table version %d, want 1", table.Version)
	}
	return table.Profiles
}

// TestProfilesTable checks the shared table itself: 60+ unique machines, the
// families from the T04 brief, and memory values that make sense.
func TestProfilesTable(t *testing.T) {
	profiles := loadHwProfiles(t)
	if len(profiles) < 60 {
		t.Fatalf("profile table has %d machines, want at least 60", len(profiles))
	}
	seen := map[string]bool{}
	families := map[string]int{}
	for _, p := range profiles {
		if p.Name == "" || p.Family == "" {
			t.Fatalf("profile with empty name or family: %+v", p)
		}
		if seen[p.Name] {
			t.Fatalf("duplicate profile name %q", p.Name)
		}
		seen[p.Name] = true
		families[p.Family]++
		if p.RAMAvailGB == 0 || p.RAMAvailGB > p.RAMTotalGB {
			t.Errorf("%s: RAM avail %d GB not within total %d GB", p.Name, p.RAMAvailGB, p.RAMTotalGB)
		}
		switch p.Backend {
		case scan.Metal, scan.CUDA, scan.ROCm, scan.Vulkan, scan.CPU:
		default:
			t.Errorf("%s: unknown backend %q", p.Name, p.Backend)
		}
		if p.Unified && p.Backend != scan.Metal {
			t.Errorf("%s: unified memory with backend %q", p.Name, p.Backend)
		}
		if p.Backend != scan.CPU && p.GPUUsable == 0 {
			t.Errorf("%s: %s backend with no usable GPU memory", p.Name, p.Backend)
		}
		if p.Backend == scan.CPU && len(p.GPUs) > 0 {
			t.Errorf("%s: CPU backend with GPUs", p.Name)
		}
		for _, g := range p.GPUs {
			if g.MemFree > g.MemTotal {
				t.Errorf("%s/%s: free %d GB > total %d GB", p.Name, g.Name, g.MemFree, g.MemTotal)
			}
		}
	}
	for _, want := range []string{"nvidia", "multi-gpu", "vulkan", "apple", "cpu", "edge"} {
		if families[want] == 0 {
			t.Errorf("profile table has no %q profiles", want)
		}
	}
	t.Logf("%d profiles: %v", len(profiles), families)
}

// TestProfilesPlan is the contract of the shared table: every machine gets a
// plan for every preference, and no plan asks for more memory than the budget
// after the safety margin. A new profile that cannot run anything fails here.
func TestProfilesPlan(t *testing.T) {
	c := cat(t)
	profiles := loadHwProfiles(t)
	prefs := []Pref{Balanced, Smartest, Fastest}
	for _, prof := range profiles {
		hw := prof.Hardware()
		b := ComputeBudgets(hw, hw.RAMAvail)
		for _, pref := range prefs {
			plans := PlansFor(hw, c, b, pref)
			best, ok := Recommend(plans)
			if !ok {
				t.Errorf("%s (%s): no plan for %s", prof.Name, prof.Family, pref)
				continue
			}
			if best.GPUNeed > b.GPU {
				t.Errorf("%s/%s: %s needs %d GPU bytes, budget %d", prof.Name, pref, best.Model.ID, best.GPUNeed, b.GPU)
			}
			if best.RAMNeed > b.RAM {
				t.Errorf("%s/%s: %s needs %d RAM bytes, budget %d", prof.Name, pref, best.Model.ID, best.RAMNeed, b.RAM)
			}
			if best.Quant.Name == "" || best.Mode == "" || best.Ctx <= 0 {
				t.Errorf("%s/%s: incomplete plan %+v", prof.Name, pref, best)
			}
			if best.Ctx > best.Model.CtxMax && best.Model.CtxMax > 0 {
				t.Errorf("%s/%s: context %d above model max %d", prof.Name, pref, best.Ctx, best.Model.CtxMax)
			}
			if best.Mode == GPU || best.Mode == Hybrid {
				if best.NGL != 999 {
					t.Errorf("%s/%s: %s plan leaves -ngl at %d", prof.Name, pref, best.Mode, best.NGL)
				}
			}
		}
	}
}

// TestShowProfiles prints the balanced pick for every profile in the shared
// table (SHOW=1 go test -run ShowProfiles -v), for the review table.
func TestShowProfiles(t *testing.T) {
	if os.Getenv("SHOW") == "" {
		t.Skip("set SHOW=1 to print the profile picks")
	}
	c := cat(t)
	for _, prof := range loadHwProfiles(t) {
		hw := prof.Hardware()
		b := ComputeBudgets(hw, hw.RAMAvail)
		best, ok := Recommend(PlansFor(hw, c, b, Balanced))
		if !ok {
			fmt.Printf("%-52s %-9s nothing fits\n", prof.Name, prof.Family)
			continue
		}
		fmt.Printf("%-52s %-9s %-30s %-20s %-6s %3dk %-4s %-6s ~%5.1f tok/s %.1f GB\n",
			prof.Name, prof.Family, best.Model.ID, best.Quant.Name, best.Mode, best.Ctx/1024, best.KV,
			best.Speed, best.TokPerSec, float64(best.GPUNeed+best.RAMNeed)/float64(G))
	}
}
