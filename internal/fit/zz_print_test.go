package fit

import "testing"

func TestPrintDesktop(t *testing.T) {
	c := cat(t)
	hw := realMachines[0].hw
	for _, pref := range []Pref{Balanced, Smartest, Fastest} {
		ps := PlansFor(hw, c, ComputeBudgets(hw, hw.RAMAvail), pref)
		for i, p := range ps[:4] {
			t.Logf("%s #%d %s %s ctx=%d kv=%s mode=%s ncmoe=%d gpu=%.1fG ram=%.1fG tok=%.1f", pref, i, p.Model.ID, p.Quant.Name, p.Ctx, p.KV, p.Mode, p.NCPUMoE, float64(p.GPUNeed)/gb, float64(p.RAMNeed)/gb, p.TokPerSec)
		}
	}
}
