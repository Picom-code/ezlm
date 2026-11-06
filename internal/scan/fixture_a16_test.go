package scan

import "context"

// AdapterFixture is one graphics adapter as the test fixture describes it,
// without exposing displayAdapter outside the package.
type AdapterFixture struct {
	Name, Provider string
	VRAM           uint64
}

// GPUFixtureForTest runs the same GPU detection order Scan uses, against the
// fake nvidia-smi / rocm-smi the test put on PATH, and finishes the Hardware
// the way Scan does. It exists for the scan-to-fit end-to-end test.
func GPUFixtureForTest(ctx context.Context, osName string, ramTotal, ramAvail uint64, cores int) *Hardware {
	h := baseFixture(osName, ramTotal, ramAvail, cores)
	if !nvidiaScan(ctx, h) && !amdScan(ctx, h) {
		h.Backend = CPU
		h.Notes = append(h.Notes, "No supported GPU found: models will run on the CPU, which is slower.")
	}
	if !h.Unified {
		for _, g := range h.GPUs {
			h.GPUUsable += g.MemFree
		}
	}
	return h
}

// AdaptersFixtureForTest feeds Windows display-class adapters through
// usableAdapters, the same choice displayScan makes on a machine without
// nvidia-smi.
func AdaptersFixtureForTest(osName string, ramTotal, ramAvail uint64, cores int, adapters []AdapterFixture) *Hardware {
	h := baseFixture(osName, ramTotal, ramAvail, cores)
	list := make([]displayAdapter, 0, len(adapters))
	for _, a := range adapters {
		list = append(list, displayAdapter{Name: a.Name, Provider: a.Provider, VRAM: a.VRAM})
	}
	if gpus := usableAdapters(list); len(gpus) > 0 {
		h.GPUs = gpus
		h.Backend = Vulkan
		for _, g := range gpus {
			h.GPUUsable += g.MemFree
		}
		return h
	}
	h.Backend = CPU
	h.Notes = append(h.Notes, "No supported GPU found: models will run on the CPU, which is slower.")
	return h
}

func baseFixture(osName string, ramTotal, ramAvail uint64, cores int) *Hardware {
	return &Hardware{
		OS: osName, Arch: "amd64", Cores: cores, PhysCores: cores,
		RAMTotal: ramTotal, RAMAvail: ramAvail, DiskFree: 500 * GB, CPUBandwidth: 60,
	}
}
