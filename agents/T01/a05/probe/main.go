// Command probe shows how the fit planner scores the QAT-* quants of the
// Gemma 4 entries. normQuant in internal/fit does not strip the "QAT-"
// prefix, so those quants fall through to the 0.9 "unknown format" default.
// The probe keeps only two quants per model (the QAT file and a smaller,
// lower-quality IQ3 file) so the comparison is not masked by Q6/Q8 fits.
// It reads the embedded catalog and writes nothing.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/picom-code/ezlm/internal/catalog"
	"github.com/picom-code/ezlm/internal/fit"
	"github.com/picom-code/ezlm/internal/scan"
)

func load() catalog.Catalog {
	b, err := os.ReadFile("internal/catalog/catalog.json")
	if err != nil {
		panic(err)
	}
	var c catalog.Catalog
	if err := json.Unmarshal(b, &c); err != nil {
		panic(err)
	}
	return c
}

func hw(vram uint64) *scan.Hardware {
	return &scan.Hardware{
		OS: "linux", Arch: "amd64", Cores: 16, PhysCores: 8,
		RAMTotal: 64 * scan.GB, RAMAvail: 60 * scan.GB, DiskFree: 2000 * scan.GB,
		GPUs:    []scan.GPU{{Name: "RTX", Vendor: "nvidia", MemTotal: vram, MemFree: vram, Bandwidth: 504}},
		Backend: scan.CUDA, GPUUsable: vram, CPUBandwidth: 60,
	}
}

func pick(m catalog.Model, h *scan.Hardware) string {
	plans := fit.PlansFor(h, &catalog.Catalog{Models: []catalog.Model{m}}, fit.ComputeBudgets(h, h.RAMAvail), fit.Balanced)
	if len(plans) == 0 {
		return "<no plan>"
	}
	p := plans[0]
	return fmt.Sprintf("%s (eff %.3f, %s)", p.Quant.Name, p.Effective, p.Mode)
}

func quants(c catalog.Catalog, id string, keep ...string) catalog.Model {
	for _, m := range c.Models {
		if m.ID != id {
			continue
		}
		var out []catalog.Quant
		for _, q := range m.Quants {
			for _, k := range keep {
				if q.Name == k {
					out = append(out, q)
				}
			}
		}
		m.Quants = out
		return m
	}
	panic("model not found: " + id)
}

func main() {
	cases := []struct {
		id   string
		vram uint64
		keep []string
	}{
		{"gemma-4-12b", 10 * scan.GB, []string{"QAT-Q4_0"}},
		{"gemma-4-12b", 10 * scan.GB, []string{"QAT-Q4_0", "UD-IQ3_XXS"}},
		{"gemma-4-26b-a4b", 16 * scan.GB, []string{"QAT-Q4_0", "UD-IQ3_XXS"}},
		{"gemma-4-26b-a4b", 18 * scan.GB, []string{"QAT-UD-Q4_K_XL", "UD-IQ3_XXS"}},
	}
	for _, tc := range cases {
		fmt.Printf("\n== %s, %.0f GB, quants %v\n", tc.id, float64(tc.vram)/float64(scan.GB), tc.keep)

		base := load()
		m := quants(base, tc.id, tc.keep...)
		fmt.Printf("  as-is                       : %s\n", pick(m, hw(tc.vram)))

		ren := load()
		mr := quants(ren, tc.id, tc.keep...)
		for i := range mr.Quants {
			if mr.Quants[i].Name == "QAT-Q4_0" {
				mr.Quants[i].Name = "Q4_0"
			}
			if mr.Quants[i].Name == "QAT-UD-Q4_K_XL" {
				mr.Quants[i].Name = "UD-Q4_K_XL"
			}
		}
		fmt.Printf("  QAT- prefix stripped        : %s\n", pick(mr, hw(tc.vram)))

		q := load()
		mq := quants(q, tc.id, tc.keep...)
		for i := range mq.Quants {
			if mq.Quants[i].Name == "QAT-Q4_0" || mq.Quants[i].Name == "QAT-UD-Q4_K_XL" {
				mq.Quants[i].Quality = 0.985
			}
		}
		fmt.Printf("  quality override 0.985      : %s\n", pick(mq, hw(tc.vram)))
	}
}
