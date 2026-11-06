package scan

import "testing"

// Which cards a Windows PC without NVIDIA runs models on: discrete Radeon and
// Arc cards (Vulkan build); built-in graphics, virtual adapters and NVIDIA
// cards (read by nvidia-smi) are left alone.
func TestUsableAdapters(t *testing.T) {
	list := []displayAdapter{
		{Name: "AMD Radeon(TM) Graphics", Provider: "Advanced Micro Devices, Inc.", VRAM: 2 * GB}, // a desktop Ryzen's (seen on the dev desktop)
		{Name: "Intel(R) Iris(R) Xe Graphics", Provider: "Intel Corporation", VRAM: 128 << 20},
		{Name: "AMD Radeon RX 7800 XT", Provider: "Advanced Micro Devices, Inc.", VRAM: 16 * GB},
		{Name: "AMD Radeon 780M Graphics", Provider: "Advanced Micro Devices, Inc.", VRAM: 512 << 20},
		{Name: "AMD Radeon(TM) RX Vega 10 Graphics", Provider: "Advanced Micro Devices, Inc.", VRAM: 2 * GB},
		{Name: "Intel(R) Arc(TM) B580 Graphics", Provider: "Intel Corporation", VRAM: 12 * GB},
		{Name: "Intel(R) Arc(TM) 140V GPU (16GB)", Provider: "Intel Corporation", VRAM: 128 << 20},
		{Name: "NVIDIA GeForce RTX 4070 SUPER", Provider: "NVIDIA", VRAM: 12 * GB},
		{Name: "Microsoft Basic Display Adapter", Provider: "Microsoft", VRAM: 0},
		{Name: "Parsec Virtual Display Adapter", Provider: "Parsec Cloud, Inc.", VRAM: 16 * GB},
	}
	g := usableAdapters(list)
	if len(g) != 2 {
		t.Fatalf("got %d cards: %+v", len(g), g)
	}
	if g[0].Name != "AMD Radeon RX 7800 XT" || g[0].Vendor != "amd" || g[0].MemTotal != 16*GB || g[0].Bandwidth != 624 {
		t.Fatalf("biggest card first, as AMD with its speed: %+v", g[0])
	}
	if g[1].Vendor != "intel" || g[1].Bandwidth != 456 {
		t.Fatalf("Arc B580: %+v", g[1])
	}

	// Built-in graphics with a big share set aside count, but only without a discrete card.
	halo := []displayAdapter{{Name: "AMD Radeon(TM) 8060S Graphics", Provider: "Advanced Micro Devices, Inc.", VRAM: 64 * GB}}
	if h := usableAdapters(halo); len(h) != 1 || h[0].MemTotal != 64*GB || h[0].Bandwidth != 0 {
		t.Fatalf("Ryzen AI Max: %+v", h)
	}
	if h := usableAdapters(append(halo, list[2])); len(h) != 1 || h[0].Name != "AMD Radeon RX 7800 XT" {
		t.Fatalf("a discrete card wins over built-in graphics: %+v", h)
	}
	if len(usableAdapters(list[:2])) != 0 || len(usableAdapters(nil)) != 0 {
		t.Fatal("everyday built-in graphics stay with the processor")
	}

	// Current workstation and AI cards are discrete too.
	for _, name := range []string{"Intel(R) Arc(TM) Pro B60 Graphics", "Intel(R) Arc(TM) Pro B50 Graphics", "Intel(R) Arc(TM) Pro A60 Graphics",
		"AMD Radeon AI PRO R9700", "Radeon RX Vega 64", "Radeon RX Vega 56", "AMD Radeon PRO W7900"} {
		if g := usableAdapters([]displayAdapter{{Name: name, VRAM: 16 * GB}}); len(g) != 1 || g[0].Bandwidth == 0 {
			t.Errorf("%s: %+v", name, g)
		}
	}
	two := usableAdapters([]displayAdapter{{Name: "Intel(R) Arc(TM) Pro B60 Graphics", VRAM: 24 * GB}, {Name: "Intel(R) Arc(TM) Pro B60 Graphics", VRAM: 24 * GB}})
	if len(two) != 2 {
		t.Fatalf("both cards of a two-card rig count: %+v", two)
	}
}

func TestOtherBandwidth(t *testing.T) {
	for name, want := range map[string]float64{
		"AMD Radeon RX 7900 XTX":         960, // not read as the 7900 XT
		"AMD Radeon RX 7900 XT":          800,
		"AMD Radeon RX 9070 XT":          640,
		"Intel(R) Arc(TM) A770 Graphics": 560,
		"AMD Radeon Pro W7900":           864,
		"Some Future GPU":                300,
	} {
		if got := otherBandwidth(name); got != want {
			t.Errorf("%s: got %v, want %v", name, got, want)
		}
	}
}
