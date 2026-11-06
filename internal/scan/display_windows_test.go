//go:build windows

package scan

import (
	"context"
	"os"
	"testing"
)

// Reads this PC's graphics cards and their memory in use, as the scan and the
// live dashboard do. Read-only. On a PC known to have a graphics card, set
// EZLM_EXPECT_GPU=1 to require both readings.
func TestReadsThisPCsGraphics(t *testing.T) {
	list := displayAdapters(context.Background())
	for _, a := range list {
		t.Logf("card: %q by %q, %.1f GB", a.Name, a.Provider, float64(a.VRAM)/float64(GB))
	}
	t.Logf("usable for the Vulkan build: %+v", usableAdapters(list))
	used := adapterMemoryUsed()
	for k, v := range used {
		t.Logf("in use: %s %.2f GB", k, float64(v)/float64(GB))
	}
	if os.Getenv("EZLM_EXPECT_GPU") != "" {
		if len(list) == 0 {
			t.Fatal("no graphics cards read")
		}
		if len(used) == 0 {
			t.Fatal("no graphics memory counters read")
		}
	}
}
