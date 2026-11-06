//go:build !windows

package scan

import "testing"

// TestLocalizedWindows runs the localized Windows fixtures on the machine the
// corpus is built on. The parsers they feed are shared, so a German, Japanese
// or Chinese sample is checked here and on Windows.
func TestLocalizedWindows(t *testing.T) {
	runLocaleFixtures(t)
}
