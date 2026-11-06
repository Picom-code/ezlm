package scan

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fakeRegistry is one display driver entry as readAdapter sees it on Windows:
// the registry values, without a registry. A missing value returns an error,
// as a registry key that isn't there does.
type fakeRegistry struct {
	strs map[string]string
	ints map[string]uint64
	bins map[string][]byte
}

func (f fakeRegistry) GetStringValue(name string) (string, uint32, error) {
	if v, ok := f.strs[name]; ok {
		return v, 1, nil
	}
	return "", 0, os.ErrNotExist
}

func (f fakeRegistry) GetIntegerValue(name string) (uint64, uint32, error) {
	if v, ok := f.ints[name]; ok {
		return v, 4, nil
	}
	return 0, 0, os.ErrNotExist
}

func (f fakeRegistry) GetBinaryValue(name string) ([]byte, uint32, error) {
	if v, ok := f.bins[name]; ok {
		return v, 3, nil
	}
	return nil, 0, os.ErrNotExist
}

// loadDisplayEntry reads a fixture: one registry value per line, "Name TYPE
// data", where TYPE is REG_SZ, REG_QWORD, REG_DWORD or REG_BINARY (hex bytes).
// Blank lines and lines starting with # are provenance notes.
func loadDisplayEntry(t *testing.T, name string) fakeRegistry {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	f := fakeRegistry{strs: map[string]string{}, ints: map[string]uint64{}, bins: map[string][]byte{}}
	for i, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, " ", 3)
		if len(parts) != 3 {
			t.Fatalf("%s:%d: want \"Name TYPE data\", got %q", name, i+1, line)
		}
		data := strings.TrimSpace(parts[2])
		switch parts[1] {
		case "REG_SZ":
			f.strs[parts[0]] = strings.Trim(data, `"`)
		case "REG_QWORD", "REG_DWORD":
			n, err := strconv.ParseUint(data, 0, 64)
			if err != nil {
				t.Fatalf("%s:%d: %v", name, i+1, err)
			}
			f.ints[parts[0]] = n
		case "REG_BINARY":
			for _, h := range strings.Fields(data) {
				n, err := strconv.ParseUint(h, 16, 8)
				if err != nil {
					t.Fatalf("%s:%d: %v", name, i+1, err)
				}
				f.bins[parts[0]] = append(f.bins[parts[0]], byte(n))
			}
		default:
			t.Fatalf("%s:%d: unknown type %q", name, i+1, parts[1])
		}
	}
	return f
}

// What the scan makes of a Windows display driver entry: the card's name and
// memory, and whether it is one models can run on. Fixtures are real registry
// values where a public report has them, and say so in SOURCES.md.
func TestDisplayRegistryValues(t *testing.T) {
	cases := []struct {
		fixture string
		want    displayAdapter
		ok      bool
		usable  *GPU
	}{
		{
			// A 16 GB Radeon: the 64-bit qwMemorySize, not the 32-bit AdapterRAM.
			fixture: "display_amd_rx9070.txt",
			want:    displayAdapter{Name: "AMD Radeon RX 9070", Provider: "Advanced Micro Devices, Inc.", VRAM: 17095983104},
			ok:      true,
			usable:  &GPU{Name: "AMD Radeon RX 9070", Vendor: "amd", MemTotal: 17095983104, MemFree: 17095983104, Bandwidth: 640},
		},
		{
			// Built-in graphics with 512 MB set aside are named, but models stay on the processor.
			fixture: "display_amd_igpu.txt",
			want:    displayAdapter{Name: "AMD Radeon(TM) Graphics", Provider: "Advanced Micro Devices, Inc.", VRAM: 536870912},
			ok:      true,
		},
		{
			// Intel's own name for the card, and qwMemorySize wins over the saturated MemorySize.
			fixture: "display_intel_arc_a770.txt",
			want:    displayAdapter{Name: "Intel(R) Arc(TM) A770 Graphics", Provider: "Intel Corporation", VRAM: 16106127360},
			ok:      true,
			usable:  &GPU{Name: "Intel(R) Arc(TM) A770 Graphics", Vendor: "intel", MemTotal: 16106127360, MemFree: 16106127360, Bandwidth: 560},
		},
		{
			// A driver that writes MemorySize as an 8-byte value: reading 4 bytes
			// would turn 15 GB into 3 GB.
			fixture: "display_intel_arc_memorysize_qword.txt",
			want:    displayAdapter{Name: "Intel(R) Arc(TM) A770 Graphics", Provider: "Intel Corporation", VRAM: 16106127360},
			ok:      true,
			usable:  &GPU{Name: "Intel(R) Arc(TM) A770 Graphics", Vendor: "intel", MemTotal: 16106127360, MemFree: 16106127360, Bandwidth: 560},
		},
		{
			// The 4-byte sentinel Windows writes for cards over 2 GB, with no
			// qwMemorySize to correct it: 0x7FFFF000 is 4 KB under 2 GiB, so the
			// card falls below the discrete threshold and is dropped. Known Windows
			// limitation; the fixture records the value, not a fix.
			fixture: "display_intel_arc_memorysize_4byte.txt",
			want:    displayAdapter{Name: "Intel(R) Arc(TM) A770 Graphics", Provider: "Intel Corporation", VRAM: 2147479552},
			ok:      true,
		},
		{
			// A zero qwMemorySize must not hide the size the driver also wrote.
			fixture: "display_intel_uhd_770.txt",
			want:    displayAdapter{Name: "Intel(R) UHD Graphics 770", Provider: "Intel Corporation", VRAM: 1073741824},
			ok:      true,
		},
		{
			// No driver entry at all: skip it rather than report a nameless 0 GB card.
			fixture: "display_missing_key.txt",
			ok:      false,
		},
		{
			// A stale entry for a removed card: named, but with no memory it is unusable.
			fixture: "display_ghost_entry.txt",
			want:    displayAdapter{Name: "AMD Radeon RX 580 Series", Provider: "Advanced Micro Devices, Inc."},
			ok:      true,
		},
	}
	for _, c := range cases {
		t.Run(strings.TrimSuffix(c.fixture, ".txt"), func(t *testing.T) {
			a, ok := adapterFrom(loadDisplayEntry(t, c.fixture))
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v (adapter %+v)", ok, c.ok, a)
			}
			if !ok {
				return
			}
			if a != c.want {
				t.Errorf("adapter = %+v, want %+v", a, c.want)
			}
			gpus := usableAdapters([]displayAdapter{a})
			if c.usable == nil {
				if len(gpus) != 0 {
					t.Fatalf("usable = %+v, want no card", gpus)
				}
				return
			}
			if len(gpus) != 1 {
				t.Fatalf("usable = %+v, want one card", gpus)
			}
			if gpus[0] != *c.usable {
				t.Errorf("usable = %+v, want %+v", gpus[0], *c.usable)
			}
		})
	}
}
