package scan

// Fuzz targets for the text parsers in this package. Each target seeds the
// documented output shape (and the team's fixtures once they land under
// testdata/) and asserts two things: no panic, and no silent zero-value
// misparse (a parser returning an empty or impossible value for input it
// claimed to understand).

import (
	"strings"
	"testing"
)

func FuzzSplitCSV(f *testing.F) {
	for _, s := range []string{
		"NVIDIA GeForce RTX 4090, 24564, 23901, 580.65",
		"NVIDIA GeForce RTX 3080 Laptop GPU, 16384, 15000, 566.36",
		"NVIDIA NPU, [N/A], [N/A], 32.0.15.5585",
		"NVIDIA H100 80GB HBM3, 81559, 0, 570.86",
		"",
		" , , ",
		"name with, commas inside? no, quotes are not handled",
		"a,b,c",
		"\r\n",
		"名前, 24564, 23901, 580.65",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, line string) {
		got := splitCSV(line)
		if want := strings.Count(line, ",") + 1; len(got) != want {
			t.Fatalf("splitCSV(%q) returned %d fields, want %d", line, len(got), want)
		}
		for i, field := range got {
			if field != strings.TrimSpace(field) {
				t.Fatalf("splitCSV(%q)[%d] = %q is not trimmed", line, i, field)
			}
		}
	})
}

func FuzzParseNvidiaGPUs(f *testing.F) {
	for _, s := range []string{
		"NVIDIA GeForce RTX 4090, 24564, 23901, 580.65\n",
		"NVIDIA GeForce RTX 4090, 24564, 23901, 580.65\nNVIDIA GeForce RTX 4090, 24564, 23800, 580.65",
		"NVIDIA RTX A6000, 49140, 48600, 552.74\r\n",
		"NVIDIA NPU, [N/A], [N/A], 32.0.15.5585\n",
		"Quadro K2200, 4096, 4096, 353.09",
		"Tesla V100-SXM2-16GB, 16384, 16128, 535.104.05",
		"",
		"garbage",
		"NVIDIA GeForce GTX 1080, 8192, 7000",
		" , , ",
		"名前, 8192, 7000, 1.0",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, out string) {
		for _, g := range parseNvidiaGPUs(out) {
			if g.Vendor != "nvidia" {
				t.Fatalf("%q: vendor %q", out, g.Vendor)
			}
			if g.MemTotal == 0 {
				t.Fatalf("%q: a GPU with zero memory was kept", out)
			}
			if g.MemFree > g.MemTotal {
				t.Fatalf("%q: free %d > total %d", out, g.MemFree, g.MemTotal)
			}
			if g.MemTotal%(1<<20) != 0 || g.MemFree%(1<<20) != 0 {
				t.Fatalf("%q: MiB not converted to bytes: %+v", out, g)
			}
		}
		if strings.TrimSpace(out) == "" && len(parseNvidiaGPUs(out)) != 0 {
			t.Fatal("empty input produced a GPU")
		}
	})
}

func FuzzParseNvidiaProcs(f *testing.F) {
	for _, s := range []string{
		"1234, python.exe, 2048",
		"1234, python.exe, 2048\n5678, /usr/bin/ollama, 1024",
		"0, not a process, 100",
		"[N/A], [N/A], [N/A]",
		"1234, name with, comma, 2048",
		"",
		"pid, name, mem",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, out string) {
		for _, p := range parseNvidiaProcs(out) {
			if p.PID <= 0 {
				t.Fatalf("%q: kept PID %d", out, p.PID)
			}
			if p.Mem%(1<<20) != 0 {
				t.Fatalf("%q: MiB not converted to bytes: %+v", out, p)
			}
		}
	})
}

func FuzzParseCudaVersion(f *testing.F) {
	for _, s := range []string{
		"| NVIDIA-SMI 616.92                 KMD Version: 616.92        CUDA UMD Version: 13.4     |",
		"| NVIDIA-SMI 570.124.06   Driver Version: 570.124.06   CUDA Version: 12.8     |",
		"| NVIDIA-SMI 535.104.05             Driver Version: 535.104.05   CUDA Version: 12.2     |",
		"",
		"CUDA Version: ",
		"CUDA Version: ..",
		"CUDA Version: 12.8.1",
		"no version here",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, hdr string) {
		v := parseCudaVersion(hdr)
		if v == "" {
			return
		}
		if !strings.Contains(hdr, v) {
			t.Fatalf("%q: version %q is not in the header", hdr, v)
		}
		for _, part := range strings.Split(v, ".") {
			if part == "" {
				t.Fatalf("%q: malformed version %q", hdr, v)
			}
		}
		if v[0] < '0' || v[0] > '9' {
			t.Fatalf("%q: version %q does not start with a digit", hdr, v)
		}
	})
}

func FuzzParseRocmCards(f *testing.F) {
	for _, s := range []string{
		`{"card0": {"Card Series": "Radeon RX 7900 XTX", "VRAM Total Memory (B)": "25753026560", "VRAM Total Used Memory (B)": "1073741824"}}`,
		`{"card0": {"Card series": "Radeon RX 6800 XT", "VRAM Total Memory (B)": "17163091968", "VRAM Total Used Memory (B)": "0"}}`,
		`{"card0": {"Card Series": "AMD Radeon Graphics", "VRAM Total Memory (B)": "536870912", "VRAM Total Used Memory (B)": "0"}}`,
		`{"card0": {"VRAM Total Memory (B)": "100", "VRAM Total Used Memory (B)": "200"}}`,
		`{"card0": {"Card Series": "no memory"}}`,
		`{"card0": {}}`,
		`{}`,
		`null`,
		`[]`,
		`{"card0": {"Card Series": "x", "VRAM Total Memory (B)": "18446744073709551615", "VRAM Total Used Memory (B)": "1"}}`,
		``,
		`garbage`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, out string) {
		for _, g := range parseRocmCards(out) {
			if g.Vendor != "amd" {
				t.Fatalf("%q: vendor %q", out, g.Vendor)
			}
			if g.MemTotal == 0 {
				t.Fatalf("%q: a card with zero memory was kept", out)
			}
			if g.MemFree > g.MemTotal {
				t.Fatalf("%q: used exceeds total, free wrapped around: %+v", out, g)
			}
		}
	})
}

func FuzzParseNvidiaMem(f *testing.F) {
	for _, s := range []string{
		"24564, 23901",
		"24564, 23901\n24564, 23800",
		"8192, 7000\r\n",
		"[N/A], [N/A]",
		"0, 0",
		"0, 5",
		"",
		"garbage",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, out string) {
		total, free := parseNvidiaMem(out)
		if free > total {
			t.Fatalf("%q: free %d > total %d", out, free, total)
		}
		if total%(1<<20) != 0 || free%(1<<20) != 0 {
			t.Fatalf("%q: MiB not converted to bytes: %d, %d", out, total, free)
		}
		if strings.TrimSpace(out) == "" && (total != 0 || free != 0) {
			t.Fatalf("empty input produced %d, %d", total, free)
		}
	})
}

func FuzzParseGPUUsage(f *testing.F) {
	for _, s := range []string{
		"pid_16640_luid_0x00000000_0x000110FB_phys_0 7687751671808\r\npid_2788_luid_0x00000000_0x00012874_phys_0 10273845248\r\n",
		"pid_2044_luid_0x00000000_0x000110FB_phys_0 251236352\r\npid_2044_luid_0x00000000_0x00012874_phys_0 1048576\r\n",
		"pid_0_luid_0x0_0x0_phys_0 0",
		"pid__luid_0x0_0x0_phys_0 5",
		"_Total 99",
		"pid_1 5 extra",
		"pid_1_luid 9999999999999999999999",
		"",
		"\r\n",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, out string) {
		u := parseGPUUsage(out)
		for pid, b := range u {
			if b >= 1<<40 {
				t.Fatalf("%q: pid %d kept an impossible %d bytes", out, pid, b)
			}
		}
		if strings.TrimSpace(out) == "" && len(u) != 0 {
			t.Fatalf("empty input produced %v", u)
		}
		if !strings.Contains(out, "pid_") && len(u) != 0 {
			t.Fatalf("%q: parsed a map without a pid_ instance: %v", out, u)
		}
	})
}

func FuzzParseFreePercentage(f *testing.F) {
	for _, s := range []string{
		"System-wide memory free percentage: 62%",
		"System-wide memory free percentage: 100%",
		"System-wide memory free percentage: 0%",
		"System-wide memory free percentage: 12.5%",
		"free percentage: 62%",
		"free percentage:",
		"free percentage: 62,5%",
		"free percentage: -1%",
		"free percentage: 101%",
		"",
		"nothing",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, out string) {
		pct, ok := parseFreePercentage(out)
		if !ok {
			return
		}
		if pct < 0 || pct > 100 {
			t.Fatalf("%q: percentage %v out of range", out, pct)
		}
	})
}

func FuzzUsableAdapters(f *testing.F) {
	f.Add("AMD Radeon RX 7900 XTX", "Advanced Micro Devices, Inc.", uint64(24*GB))
	f.Add("Intel(R) Arc(TM) B580 Graphics", "Intel Corporation", uint64(12*GB))
	f.Add("AMD Radeon(TM) 8060S Graphics", "Advanced Micro Devices, Inc.", uint64(64*GB))
	f.Add("NVIDIA GeForce RTX 4070 SUPER", "NVIDIA", uint64(12*GB))
	f.Add("Microsoft Basic Display Adapter", "Microsoft", uint64(0))
	f.Add("", "", uint64(0))
	f.Fuzz(func(t *testing.T, name, provider string, vram uint64) {
		if v := vendorOf(name + " " + provider); v != "" && v != "nvidia" && v != "amd" && v != "intel" {
			t.Fatalf("vendorOf(%q) = %q", name+" "+provider, v)
		}
		if bw := otherBandwidth(name); bw <= 0 {
			t.Fatalf("otherBandwidth(%q) = %v", name, bw)
		}
		if bw := nvidiaBandwidth(name); bw <= 0 {
			t.Fatalf("nvidiaBandwidth(%q) = %v", name, bw)
		}
		gpus := usableAdapters([]displayAdapter{{Name: name, Provider: provider, VRAM: vram}})
		for _, g := range gpus {
			if g.MemTotal == 0 || g.MemFree != g.MemTotal {
				t.Fatalf("%q/%q/%d: bad GPU %+v", name, provider, vram, g)
			}
			if g.Vendor != "amd" && g.Vendor != "intel" {
				t.Fatalf("%q/%q: unexpected vendor %q", name, provider, g.Vendor)
			}
		}
		if len(gpus) > 1 {
			t.Fatalf("one adapter produced %d cards", len(gpus))
		}
	})
}

func FuzzAppPaths(f *testing.F) {
	for _, s := range []string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Steam\steamapps\common\Game\game.exe`,
		`C:\Users\me\AppData\Local\Discord\app-1.0.9\Discord.exe`,
		`C:\Users\me\AppData\Local\Microsoft\WindowsApps\wt.exe`,
		`C:\Windows\System32\svchost.exe`,
		`/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`,
		`/Applications/Visual Studio Code.app`,
		`/usr/bin/python3`,
		`/usr/local/bin/node`,
		`/opt/homebrew/bin/ollama`,
		`ollama`,
		``,
		`C:\`,
		`\`,
		`名前.exe`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, exe string) {
		base := baseName(exe)
		if base != strings.ToLower(base) {
			t.Fatalf("baseName(%q) = %q is not lowercased", exe, base)
		}
		root := installRoot(exe, func(string) string { return "" })
		if root == "" {
			t.Fatalf("installRoot(%q) = empty", exe)
		}
		if name := friendlyName(exe); name == "" {
			t.Fatalf("friendlyName(%q) = empty", exe)
		}
	})
}
