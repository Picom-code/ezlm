package scan

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Real nvidia-smi --query-gpu=name,memory.total,memory.free,driver_version
// --format=csv,noheader,nounits output: laptop parts, two cards where one is
// busy, a MIG-enabled A100, and [N/A] memory. See agents/T05/a01/SOURCES.md.
func TestNVIDIAGPUsCSV(t *testing.T) {
	const ramTotal, ramAvail = 128 * GB, 100 * GB
	cases := []struct {
		file     string
		ramTotal uint64
		ramAvail uint64
		want     []GPU
	}{
		{
			// Two cards; the first is in use, so it has less free memory.
			file: "query_gpu_dual_4090.csv",
			want: []GPU{
				{Name: "NVIDIA GeForce RTX 4090", Vendor: "nvidia", MemTotal: 24564 << 20, MemFree: 23000 << 20, Bandwidth: 1008, Driver: "550.54.14"},
				{Name: "NVIDIA GeForce RTX 4090", Vendor: "nvidia", MemTotal: 24564 << 20, MemFree: 24564 << 20, Bandwidth: 1008, Driver: "550.54.14"},
			},
		},
		{
			// Laptop GPU; the fixture has Windows CRLF line endings.
			file: "query_gpu_laptop_4060.csv",
			want: []GPU{
				{Name: "NVIDIA GeForce RTX 4060 Laptop GPU", Vendor: "nvidia", MemTotal: 8188 << 20, MemFree: 7600 << 20, Bandwidth: 272, Driver: "566.36"},
			},
		},
		{
			// MIG is enabled: nvidia-smi still reports the parent card and
			// its whole memory, not one MIG slice.
			file: "query_gpu_a100_mig.csv",
			want: []GPU{
				{Name: "NVIDIA A100-SXM4-80GB", Vendor: "nvidia", MemTotal: 81920 << 20, MemFree: 81920 << 20, Bandwidth: 1935, Driver: "535.104.05"},
			},
		},
		{
			// GB10 (DGX Spark) shares system RAM and reports [N/A] for
			// memory; the NPU row beside it is not a CUDA device.
			file:     "query_gpu_gb10_na.csv",
			ramTotal: ramTotal,
			ramAvail: ramAvail,
			want: []GPU{
				{Name: "NVIDIA GB10", Vendor: "nvidia", MemTotal: ramTotal, MemFree: ramAvail, Bandwidth: 273, Driver: "580.95.05"},
			},
		},
		{
			// A WDDM driver that cannot report memory: the card is dropped
			// today (the a02 slice covers keeping it). The second line ends
			// in a NUL byte, as one real Windows report did.
			file: "query_gpu_wddm_na.csv",
			want: nil,
		},
		{
			// Free memory unknown but the total is known: keep the card.
			file: "query_gpu_free_na.csv",
			want: []GPU{
				{Name: "NVIDIA RTX A5000", Vendor: "nvidia", MemTotal: 24564 << 20, MemFree: 24564 << 20, Bandwidth: 400, Driver: "552.22"},
			},
		},
		{
			// GB10 without RAM figures cannot be sized: no card.
			file: "query_gpu_gb10_na.csv",
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			out := readNVIDIAFixture(t, tc.file)
			got := parseNVIDIAGPUs(out, tc.ramTotal, tc.ramAvail)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

// The header row from --format=csv (no noheader) must not become a GPU, and
// empty output must not panic.
func TestNVIDIAGPUsCSVHeader(t *testing.T) {
	header := "name, memory.total [MiB], memory.free [MiB], driver_version\n"
	if got := parseNVIDIAGPUs(header, 0, 0); got != nil {
		t.Fatalf("header parsed as a GPU: %+v", got)
	}
	if got := parseNVIDIAGPUs("", 0, 0); got != nil {
		t.Fatalf("empty output parsed as a GPU: %+v", got)
	}
}

// A process using the card keeps its place even when its memory is unknown.
func TestNVIDIAComputeAppsCSV(t *testing.T) {
	want := []Proc{
		{PID: 12345, Name: "python3", Mem: 6144 << 20},
		{PID: 23456, Name: "/usr/bin/Xorg"},
	}
	got := parseNVIDIAComputeApps(readNVIDIAFixture(t, "compute_apps_busy.csv"))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if got := parseNVIDIAComputeApps(""); got != nil {
		t.Fatalf("empty output parsed as processes: %+v", got)
	}
}

func readNVIDIAFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "nvidia", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
