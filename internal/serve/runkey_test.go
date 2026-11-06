package serve

import (
	"strings"
	"testing"
)

// The login entry must not undo a stop. The app's --autostart runs
// "ezlm start --login" internally, and plain "start" clears the stopped flag
// (cmd/ezlm/main.go), so the CLI fallback has to say --login too.
func TestLoginArgsKeepsAStoppedServerStopped(t *testing.T) {
	if got := loginArgs("C:\\Program Files\\EZLM\\EZLM.exe"); got != "--autostart" {
		t.Fatalf("app login args = %q, want --autostart", got)
	}
	got := loginArgs("")
	if got != "start --login" {
		t.Fatalf("engine login args = %q, want \"start --login\"", got)
	}
	if !strings.Contains(got, "--login") {
		t.Fatalf("engine login args %q would restart a server the user stopped", got)
	}
}
