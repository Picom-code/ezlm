//go:build windows

package scan

import "testing"

// TestLocalizedWindows_localewindows runs the localized display and counter fixtures on
// Windows, where the code that reads them actually ships.
func TestLocalizedWindows_localewindows(t *testing.T) {
	if displayClass != `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}` {
		t.Fatalf("display class path changed: %q", displayClass)
	}
	runLocaleFixtures(t)
}
