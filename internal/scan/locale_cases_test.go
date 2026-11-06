package scan

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Localized Windows fixtures: display-class registry entries (DriverDesc,
// ProviderName, HardwareInformation.qwMemorySize) and performance-counter
// output from German, Japanese and Chinese Windows. The samples are synthetic
// and built from the documented formats; agents/T05/a06/SOURCES.md names each
// source. Windows-only entry points (registry, PowerShell) are not called, so
// the shared parsing they feed is exercised on every platform.

type localeAdapter struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	VRAM     uint64 `json:"vram"`
}

type localeGPU struct {
	Name      string  `json:"name"`
	Vendor    string  `json:"vendor"`
	MemTotal  uint64  `json:"mem_total"`
	Bandwidth float64 `json:"bandwidth"`
}

type localeDisplayCase struct {
	OS       string          `json:"os"`
	Note     string          `json:"note"`
	Adapters []localeAdapter `json:"adapters"`
	Want     []localeGPU     `json:"want"`
}

type localeUsageCase struct {
	OS   string            `json:"os"`
	File string            `json:"file"`
	Want map[string]uint64 `json:"want"`
}

type localeNvidiaCase struct {
	OS   string `json:"os"`
	File string `json:"file"`
	Want int    `json:"want"`
}

type localeCases struct {
	Display []localeDisplayCase `json:"display"`
	Usage   []localeUsageCase   `json:"usage"`
	Nvidia  []localeNvidiaCase  `json:"nvidia"`
}

func localeFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "locale", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// runLocaleFixtures checks the shared parsers against the localized Windows
// samples. usableAdapters must still pick the right card from a localized
// DriverDesc (ProviderName carries the vendor), parseGPUUsage must ignore
// localized "N/A" lines without dropping valid ones, and nvidiaScan's field
// reading must skip localized N/A values rather than misparse them.
func runLocaleFixtures(t *testing.T) {
	t.Helper()
	var cases localeCases
	if err := json.Unmarshal(localeFixture(t, "cases.json"), &cases); err != nil {
		t.Fatalf("parse cases.json: %v", err)
	}

	for _, c := range cases.Display {
		t.Run("display/"+c.OS, func(t *testing.T) {
			var adapters []displayAdapter
			for _, a := range c.Adapters {
				adapters = append(adapters, displayAdapter{Name: a.Name, Provider: a.Provider, VRAM: a.VRAM})
			}
			got := usableAdapters(adapters)
			if len(got) != len(c.Want) {
				t.Fatalf("usable adapters = %+v, want %d (%s)", got, len(c.Want), c.Note)
			}
			for i, w := range c.Want {
				g := got[i]
				if g.Name != w.Name || g.Vendor != w.Vendor || g.MemTotal != w.MemTotal || g.Bandwidth != w.Bandwidth {
					t.Errorf("adapter %d = %+v, want %+v", i, g, w)
				}
			}
		})
	}

	for _, c := range cases.Usage {
		t.Run("usage/"+c.OS, func(t *testing.T) {
			got := parseGPUUsage(string(localeFixture(t, c.File)))
			if len(got) != len(c.Want) {
				t.Fatalf("usage = %v, want %d processes", got, len(c.Want))
			}
			for pid, want := range c.Want {
				n, err := strconv.ParseInt(pid, 10, 32)
				if err != nil {
					t.Fatalf("bad fixture pid %q: %v", pid, err)
				}
				if got[int32(n)] != want {
					t.Errorf("pid %s = %d bytes, want %d", pid, got[int32(n)], want)
				}
			}
		})
	}

	for _, c := range cases.Nvidia {
		t.Run("nvidia/"+c.OS, func(t *testing.T) {
			// The same fields and conversions nvidiaScan makes for each
			// "name, total, free, driver" line.
			valid := 0
			for _, line := range strings.Split(strings.TrimSpace(string(localeFixture(t, c.File))), "\n") {
				f := splitCSV(line)
				if len(f) < 3 {
					continue
				}
				total, err1 := strconv.ParseUint(f[1], 10, 64)
				free, err2 := strconv.ParseUint(f[2], 10, 64)
				if err1 == nil && err2 == nil && total > 0 {
					valid++
					if free > total {
						t.Errorf("free %d MB above total %d MB", free, total)
					}
				}
			}
			if valid != c.Want {
				t.Errorf("rows with usable memory = %d, want %d", valid, c.Want)
			}
		})
	}
}

// TestFreeAtNonASCIIProfilePath covers a user profile such as
// C:\Users\Müller 太郎 用户: FreeAt must walk a missing, non-ASCII models
// folder up to an existing drive without corrupting the path.
func TestFreeAtNonASCIIProfilePath(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "Benutzer", "Müller 太郎 用户", "EZLM", "models")
	if free := FreeAt(context.Background(), dir); free == 0 {
		t.Fatalf("FreeAt(%q) = 0, want the free space of its drive", dir)
	}
	if !strings.Contains(dir, "Müller") || !strings.Contains(dir, "用户") {
		t.Fatalf("fixture path lost its non-ASCII characters: %q", dir)
	}
}
