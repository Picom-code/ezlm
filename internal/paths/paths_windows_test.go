//go:build windows

package paths

import (
	"os"
	"path/filepath"
	"testing"
)

// A process started without USERPROFILE (a service, or an environment a
// launcher stripped) must not keep EZLM's data in whatever folder it happened
// to start from: on Windows that can be C:\Windows\System32.
func TestHomeFallsBackToLocalAppData(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EZLM_HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("LOCALAPPDATA", dir)
	got := Home()
	if want := filepath.Join(dir, "EZLM"); got != want {
		t.Fatalf("Home() = %q, want %q", got, want)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("Home() = %q is relative", got)
	}
}

// The normal case still lands under the profile folder.
func TestHomeUsesUserProfile(t *testing.T) {
	profile := t.TempDir()
	t.Setenv("EZLM_HOME", "")
	t.Setenv("USERPROFILE", profile)
	if got, want := Home(), filepath.Join(profile, ".ezlm"); got != want {
		t.Fatalf("Home() = %q, want %q", got, want)
	}
}

// A test's EZLM_HOME is used as given, but a trailing separator must not make
// two spellings of the same folder compare unequal (isOwnExe and friends
// compare paths as strings).
func TestHomeCleansOverride(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ezlm-home")
	t.Setenv("EZLM_HOME", dir+string(os.PathSeparator))
	if got := Home(); got != dir {
		t.Fatalf("Home() = %q, want %q", got, dir)
	}
}
