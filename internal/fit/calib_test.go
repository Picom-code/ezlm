package fit

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/picom-code/ezlm/internal/catalog"
	"github.com/picom-code/ezlm/internal/scan"
)

// Measured speeds on real hardware (see catalog research + the user's runs).
var measured = []struct {
	name       string
	hwIdx      int // index into realMachines
	model, qnt string
	tokS       float64
	noSpec     bool
}{
	{"4070S Qwen3.6 35B UD-Q4_K_M (EZLM e2e test)", 0, "qwen3.6-35b-a3b", "UD-Q4_K_XL", 69.1, true},
	{"4070S Flash Next GSQ Q2_0 (user's run-fn.bat)", 0, "qwen3.8-flash-next", "GSQ-RCO-Q2_0", 30.8, true},
	{"4090+96GB Flash Next UD-IQ3_XXS (research)", 2, "qwen3.8-flash-next", "UD-IQ3_XXS", 30, true},
	{"M3 Max 128GB Flash Next UD-IQ4_XS (PR #27836)", -1, "qwen3.8-flash-next", "UD-IQ4_XS", 27.4, true},
	{"M5 Max 128GB Qwen3.6 35B Q8_0 (stared/benching)", -2, "qwen3.6-35b-a3b", "Q8_0", 93, true},
}

func calibHW(i int) *scan.Hardware {
	switch i {
	case -1:
		return mac(128, 110, 400) // M3 Max
	case -2:
		return mac(128, 110, 614) // M5 Max
	}
	return realMachines[i].hw
}

func TestCalibration(t *testing.T) {
	c := cat(t)
	for _, mm := range measured {
		hw := calibHW(mm.hwIdx)
		var m catalog.Model
		for _, x := range c.Models {
			if x.ID == mm.model {
				m = x
			}
		}
		if mm.noSpec {
			m.Draft = nil
		}
		var q catalog.Quant
		for _, x := range append(m.Quants, hiddenQuants(t)[mm.model]...) { // measured files may be hidden for the beta
			if x.Name == mm.qnt {
				q = x
				q.MTP = false
			}
		}
		p, ok := try(hw, m, q, 32768, "f16", ComputeBudgets(hw, hw.RAMAvail))
		if !ok {
			t.Errorf("%s: doesn't fit", mm.name)
			continue
		}
		ratio := p.TokPerSec / mm.tokS
		t.Logf("%-46s est %5.1f  measured %5.1f  ratio %.2f  (%s, ncpumoe %d)", mm.name, p.TokPerSec, mm.tokS, ratio, p.Mode, p.NCPUMoE)
		if ratio < 0.6 || ratio > 1.6 {
			t.Errorf("%s: estimate off by more than 1.6x", mm.name)
		}
	}
}

// GSQ IQ3_XXS on the same desktop ran 5.6 tok/s in EZLM's own test because it
// pages from SSD: it must not be offered there.
func TestNoHeavyPaging(t *testing.T) {
	c := cat(t)
	hw := realMachines[0].hw
	for _, m := range c.Models {
		if m.ID != "qwen3.8-flash-next" {
			continue
		}
		for _, q := range m.Quants {
			if q.Name == "GSQ-RCO-IQ3_XXS" {
				if p, ok := try(hw, m, q, 32768, "f16", ComputeBudgets(hw, hw.RAMAvail)); ok {
					t.Errorf("IQ3_XXS should not fit (would page heavily), got %.1f tok/s", p.TokPerSec)
				}
			}
		}
	}
}

// hiddenQuants are files kept out of the beta catalog (too big to test with
// real weights here); their speed measurements still calibrate the model.
func hiddenQuants(t *testing.T) map[string][]catalog.Quant {
	var h struct {
		Quants map[string][]catalog.Quant `json:"quants"`
	}
	b, err := os.ReadFile("../catalog/hidden-for-beta.json")
	if err != nil {
		return nil
	}
	if err := json.Unmarshal(b, &h); err != nil {
		t.Fatal(err)
	}
	return h.Quants
}
