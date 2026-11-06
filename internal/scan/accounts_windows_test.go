package scan

import (
	"strings"
	"testing"
)

// Account names keep the case Windows reports: a user called İbrahim must
// still match the processes he owns. listProcs lowercased the current
// user's name and then compared it case-insensitively with the process
// owner's name as Windows wrote it. U+0130 ("İ") lowercases to "i̇"
// (i + combining dot), which no longer folds equal to "İ", so MemoryApps
// listed nothing for him and Kill refused his processes.
func TestSameUserKeepsNonASCII(t *testing.T) {
	names := []string{
		`DESKTOP-1\İbrahim`,
		`DESKTOP-1\José Müller`,
		`DESKTOP-1\王小明`,
		`DESKTOP-1\Łukasz`,
		`DESKTOP-1\Άγγελος`,
		`DESKTOP-1\a b`,
	}
	for _, n := range names {
		if !sameUser(n, n) {
			t.Errorf("sameUser(%q, itself) = false", n)
		}
		// Account names are case-insensitive on Windows.
		if !sameUser(strings.ToUpper(n), n) {
			t.Errorf("sameUser(%q, %q) = false", strings.ToUpper(n), n)
		}
	}
}

// The current user and the process owner come from different Windows APIs
// but name the same account; both stay as written, so the comparison does
// not depend on a lower case matching the original.
func TestSameUserCurrentUserMatchesProcessOwner(t *testing.T) {
	me := `DESKTOP-1\İbrahim`
	owner := `DESKTOP-1\İbrahim`
	if !sameUser(owner, me) {
		t.Fatalf("sameUser(%q, %q) = false", owner, me)
	}
}

func TestSameUserRejectsEmptyAndOthers(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"", "", false},
		{"", `DESKTOP-1\a`, false},
		{`DESKTOP-1\a`, "", false},
		{`DESKTOP-1\a`, `DESKTOP-2\a`, false},
		{`DESKTOP-1\a`, `DESKTOP-1\b`, false},
		{`DESKTOP-1\a`, `desktop-1\A`, true},
	}
	for _, c := range cases {
		if got := sameUser(c.a, c.b); got != c.want {
			t.Errorf("sameUser(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
