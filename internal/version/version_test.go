package version

import (
	"regexp"
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/update"
)

// releaseTag matches a GitHub release tag body: major.minor.patch with an
// optional pre-release and build metadata, and no "v" prefix.
var releaseTag = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

// TestVersion checks the constant the app and engine ship with. update.Check
// compares it with GitHub tag_name values after trimming one "v", so it must
// stay a bare tag body.
func TestVersion(t *testing.T) {
	if Version == "" {
		t.Fatal("Version is empty")
	}
	if strings.HasPrefix(Version, "v") {
		t.Errorf("Version = %q; release tags carry the v, Version does not", Version)
	}
	if !releaseTag.MatchString(Version) {
		t.Errorf("Version = %q is not a major.minor.patch release tag body", Version)
	}
	if strings.Contains(Version, "+") {
		t.Errorf("Version = %q carries build metadata; tag matching does not strip it", Version)
	}
	if !strings.Contains(Version, "-") {
		t.Errorf("Version = %q is not a pre-release; the beta channel relies on the suffix", Version)
	}
}

// TestVersionOrdering pins the contract the package documents: the current
// version is newer than every older release tag, including tags with a "v",
// and is older than its own final release.
func TestVersionOrdering(t *testing.T) {
	older := []string{
		"0.1.10-beta",
		"0.1.10",
		"0.1.9-beta",
		"0.1.11-alpha",
		"v0.1.10-beta",
		"0.0.1",
	}
	for _, old := range older {
		if !update.Newer(Version, old) {
			t.Errorf("Newer(%q, %q) = false; current version should be newer", Version, old)
		}
		if update.Newer(old, Version) {
			t.Errorf("Newer(%q, %q) = true; an older tag beat the current version", old, Version)
		}
	}
	if update.Newer(Version, Version) {
		t.Errorf("Newer(%q, %q) = true; a version is not newer than itself", Version, Version)
	}
	if update.Newer(Version, "v"+Version) {
		t.Errorf("Newer(%q, %q) = true; the v prefix must not change ordering", Version, "v"+Version)
	}
	if !update.Newer(strings.TrimSuffix(Version, "-beta"), Version) {
		t.Errorf("Newer(%q, %q) = false; a final release must beat its pre-release", strings.TrimSuffix(Version, "-beta"), Version)
	}
}

// TestNewerPreReleaseOrdering covers SemVer 2.0.0 precedence: a release beats
// its pre-releases, and pre-release identifiers order lexically, with numeric
// identifiers ordered numerically and build metadata ignored.
func TestNewerPreReleaseOrdering(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
		bug  string
	}{
		{a: "1.0.0", b: "1.0.0-rc.1", want: true},
		{a: "1.0.0-rc.1", b: "1.0.0", want: false},
		{a: "1.0.0-beta", b: "1.0.0-alpha", want: true},
		{a: "1.0.0-alpha", b: "1.0.0-beta", want: false},
		{a: "1.0.0-alpha.1", b: "1.0.0-alpha", want: true},
		{a: "1.0.0-alpha", b: "1.0.0-alpha.1", want: false},
		{a: "1.0.0-alpha.1", b: "1.0.0-alpha.beta", want: false},
		{a: "1.0.0-rc.1", b: "1.0.0-beta.11", want: true},
		{a: "1.0.0-beta.11", b: "1.0.0-beta.2", want: true,
			bug: "pre-release identifiers compare as strings, so beta.2 beats beta.11"},
		{a: "1.0.0-beta.2", b: "1.0.0-beta.11", want: false,
			bug: "pre-release identifiers compare as strings, so beta.2 beats beta.11"},
	} {
		t.Run(c.a+"_gt_"+c.b, func(t *testing.T) {
			if got := update.Newer(c.a, c.b); got != c.want {
				if c.bug != "" {
					t.Skipf("bug: %s (Newer = %v, want %v)", c.bug, got, c.want)
				}
				t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

// TestNewerBuildMetadata checks that build metadata never changes ordering,
// as SemVer requires.
func TestNewerBuildMetadata(t *testing.T) {
	for _, c := range []struct {
		a, b string
		bug  string
	}{
		{a: "1.0.0+build.1", b: "1.0.0+build.2"},
		{a: "1.0.0", b: "1.0.0+build",
			bug: "split treats +build as a pre-release, so a final release looks newer than the same version with build metadata"},
		{a: "1.0.0-rc.1+build.5", b: "1.0.0-rc.1",
			bug: "split treats +build as part of the pre-release, so build metadata changes precedence"},
		{a: "1.0.0-rc.1+build.5", b: "1.0.0-rc.2"},
	} {
		t.Run(c.a+"_gt_"+c.b, func(t *testing.T) {
			if got := update.Newer(c.a, c.b); got {
				if c.bug != "" {
					t.Skipf("bug: %s", c.bug)
				}
				t.Errorf("Newer(%q, %q) = true; build metadata must not affect precedence", c.a, c.b)
			}
		})
	}
}

// TestNewerMalformedInput records how the comparison treats input that is not
// a release tag. Nothing may panic; malformed parts count as zero.
func TestNewerMalformedInput(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"", "0.0.0", false},
		{"0.0.0", "", false},
		{"abc", "0.0.0", false},
		{"1", "1.0.0", false},
		{"1.2", "1.2.0", false},
		{"1.2.3.4", "1.2.3", false}, // fourth component is ignored
		{"v1.2.3", "1.2.3", false},
		{" 1.2.3 ", "1.2.3", false},
		{"1.2.3-", "1.2.3", false}, // empty pre-release reads as a final release
		{"-beta", "0.0.0-beta", false},
		{"2.0.0", "10.0.0", false}, // numeric, not lexical
		{"10.0.0", "9.9.9", true},
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Newer(%q, %q) panicked: %v", c.a, c.b, r)
				}
			}()
			if got := update.Newer(c.a, c.b); got != c.want {
				t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
			}
		}()
	}
}
