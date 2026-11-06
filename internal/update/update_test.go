package update

import "testing"

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.1.1-beta", "0.1.0-beta", true},
		{"0.1.1", "0.1.1-beta", true},
		{"0.1.1-beta", "0.1.1", false},
		{"0.1.0-beta", "0.1.0-beta", false},
		{"0.2.0-alpha", "0.1.9", true},
		{"v1.0.0", "0.9.9", true},
		{"0.1.0", "0.1.1-beta", false},
		{"0.1.10-beta", "0.1.9-beta", true}, // numbers, not text: 10 > 9
		{"0.1.9-beta", "0.1.10-beta", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestAssetFor(t *testing.T) {
	if !assetFor("windows", "EZLM-Setup.exe") || !assetFor("windows", "EZLM-Setup-0.1.1.exe") || assetFor("windows", "EZLM.dmg") {
		t.Error("windows asset choice")
	}
	if !assetFor("darwin", "EZLM.dmg") || !assetFor("darwin", "EZLM-0.1.1.dmg") || assetFor("darwin", "EZLM-Setup.exe") {
		t.Error("mac asset choice")
	}
}
