package scan

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// readWMIFixture loads a captured Win32_VideoController output.
func readWMIFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The list displayAdapters works from: one PNPDeviceID per adapter Windows
// reports, blank lines for adapters that report none, and no entry at all
// for a card that was taken out (it only keeps its driver entry).
func TestVideoControllerIDs(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		want    []string
	}{
		{"display_wmi_desktop_amd.txt", []string{`PCI\VEN_1002&DEV_747E&SUBSYS_E1B61002&REV_C8\6&1B2C3D4E&0&00000019`}},
		{"display_wmi_two_adapters.txt", []string{
			`PCI\VEN_8086&DEV_E20B&SUBSYS_10A019DA&REV_00\6&1F2E3D4C&0&00000010`,
			`PCI\VEN_1002&DEV_164E&SUBSYS_88771043&REV_C1\4&2E9B0F1A&0&0041`,
		}},
		{"display_wmi_ghost_egpu.txt", []string{`PCI\VEN_1002&DEV_164E&SUBSYS_88771043&REV_C1\4&2E9B0F1A&0&0041`}},
		{"display_wmi_vm_virtio.txt", []string{`PCI\VEN_1AF4&DEV_1110&SUBSYS_11001AF4&REV_02\3&267A616A&0&08`}},
		{"display_wmi_null_id.txt", []string{`PCI\VEN_10DE&DEV_2684&SUBSYS_88A21043&REV_A1\4&2F3C1A5E&0&0008`}},
		{"display_wmi_none.txt", nil},
	} {
		got := videoControllerIDs(readWMIFixture(t, tc.fixture))
		if len(got) != len(tc.want) {
			t.Fatalf("%s: got %q, want %q", tc.fixture, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%s: adapter %d: got %q, want %q", tc.fixture, i, got[i], tc.want[i])
			}
		}
	}

	// PowerShell ends its lines with CRLF, and may repeat or pad a line.
	inline := "  PCI\\VEN_10DE&DEV_2684&SUBSYS_88A21043&REV_A1\\4&2F3C1A5E&0&0008  \r\n" +
		"PCI\\VEN_10DE&DEV_2684&SUBSYS_88A21043&REV_A1\\4&2F3C1A5E&0&0008\r\n" +
		"\r\n"
	if got := videoControllerIDs(inline); len(got) != 1 || got[0] != `PCI\VEN_10DE&DEV_2684&SUBSYS_88A21043&REV_A1\4&2F3C1A5E&0&0008` {
		t.Fatalf("CRLF, padding and a repeat should give one trimmed ID: %q", got)
	}
}

// Only the adapters Windows reports get read. A card that was taken out keeps
// its driver entry ("0001" here), and it must not come back as a card with
// its old memory: the entries are reached from the adapters, never listed.
func TestAdapterKeys(t *testing.T) {
	igpu := `PCI\VEN_1002&DEV_164E&SUBSYS_88771043&REV_C1\4&2E9B0F1A&0&0041`
	egpu := `PCI\VEN_10DE&DEV_2684&SUBSYS_88A21043&REV_A1\4&2F3C1A5E&0&0008`
	class := `{4d36e968-e325-11ce-bfc1-08002be10318}`
	entries := map[string]string{
		igpu: class + `\0000`, // the built-in graphics
		egpu: class + `\0001`, // the card that was unplugged, entry still there
	}
	lookup := func(id string) (string, bool) {
		v, ok := entries[id]
		return v, ok
	}

	ghost := videoControllerIDs(readWMIFixture(t, "display_wmi_ghost_egpu.txt"))
	if got := adapterKeys(ghost, lookup); len(got) != 1 || got[0] != "0000" {
		t.Fatalf("the unplugged card's entry must stay unused: %q", got)
	}

	// Re-plugged: Windows lists both, and both entries are read.
	if got := adapterKeys(append(ghost, egpu), lookup); len(got) != 2 || got[0] != "0000" || got[1] != "0001" {
		t.Fatalf("both cards of a two-card machine, in Windows' order: %q", got)
	}

	// A device Windows lists whose instance key is gone is skipped, not fatal.
	if got := adapterKeys([]string{`PCI\VEN_FFFF&DEV_0000`, igpu}, lookup); len(got) != 1 || got[0] != "0000" {
		t.Fatalf("a card with no instance key: %q", got)
	}
	if got := adapterKeys(nil, lookup); len(got) != 0 {
		t.Fatalf("no adapters: %q", got)
	}
}

func TestAdapterKeyFromDriver(t *testing.T) {
	class := `{4d36e968-e325-11ce-bfc1-08002be10318}`
	for _, tc := range []struct {
		drv  string
		want string
		ok   bool
	}{
		{class + `\0000`, "0000", true},
		{class + `\0012`, "0012", true},
		{"  " + class + `\0003` + "\r\n", "0003", true},
		{"", "", false},
		{class, "", false},
		{class + `\`, "", false},
	} {
		got, ok := adapterKeyFromDriver(tc.drv)
		if got != tc.want || ok != tc.ok {
			t.Errorf("%q: got %q, %v, want %q, %v", tc.drv, got, ok, tc.want, tc.ok)
		}
	}
}

// Win32_VideoController.AdapterRAM is a uint32 of bytes, so a card with more
// than 4 GB reports about 4 GB (the "4 GB clamp"): 4293918720 here for a
// 24 GB RTX 4090. That is why displayAdapters takes the size from the
// driver entry's qwMemorySize instead.
func TestAdapterRAMClampsAt4GB(t *testing.T) {
	var ram uint64
	for _, line := range strings.Split(readWMIFixture(t, "display_wmi_adapterram_4gb.txt"), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(k) != "AdapterRAM" {
			continue
		}
		n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 32)
		if err != nil {
			t.Fatal(err)
		}
		ram = n
	}
	if ram != 4293918720 {
		t.Fatalf("AdapterRAM: got %d", ram)
	}
	if ram >= 4*GB {
		t.Fatalf("a clamped AdapterRAM must stay under 4 GB, got %d", ram)
	}
	// Using it as the card's size would leave 20 of the 24 GB unseen.
	if got := float64(ram) / float64(GB); got < 3.9 || got >= 4 {
		t.Fatalf("clamped size: %.2f GB", got)
	}
}

// Documents a gap rather than a fix: Win32_VideoController also lists an
// adapter whose driver failed (ConfigManagerErrorCode 43), and displayAdapters
// keeps it, so its memory still counts as usable. See REPORT.md.
func TestVideoControllerListsBrokenAdapter(t *testing.T) {
	dump := readWMIFixture(t, "display_wmi_config_error_43.txt")
	var code, id string
	for _, line := range strings.Split(dump, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "ConfigManagerErrorCode":
			code = strings.TrimSpace(v)
		case "PNPDeviceID":
			id = strings.TrimSpace(v)
		}
	}
	if code != "43" || id == "" {
		t.Fatalf("fixture: code %q, id %q", code, id)
	}
	if got := videoControllerIDs(id); len(got) != 1 {
		t.Fatalf("the broken adapter is listed all the same: %q", got)
	}
}
