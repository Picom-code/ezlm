package update

import (
	"fmt"
	"strconv"
	"strings"
)

type Version struct {
	Major, Minor, Patch int
	Pre                 []string // prerelease identifiers; empty for a release
}

// ParseVersion reads a release tag such as "v0.1.12-beta.1+build.5".
// It accepts exactly three numeric parts, an optional leading "v", an
// optional prerelease and optional build metadata. Build metadata does not
// affect order, as in SemVer. Anything else is an error, so a malformed tag
// is rejected instead of being read as 0.0.0.
func ParseVersion(tag string) (Version, error) {
	var v Version
	s := strings.TrimSpace(tag)
	if s == "" {
		return v, fmt.Errorf("empty version tag")
	}
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		if err := checkIdentifiers(s[i+1:], true); err != nil {
			return v, fmt.Errorf("version %q: build metadata: %w", tag, err)
		}
		s = s[:i]
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre := s[i+1:]
		s = s[:i]
		if err := checkIdentifiers(pre, false); err != nil {
			return v, fmt.Errorf("version %q: prerelease: %w", tag, err)
		}
		v.Pre = strings.Split(pre, ".")
	}
	parts := strings.Split(s, ".")
	// Real tags are not always three-part ("v1.10"); pad the missing minor
	// and patch with zero. More than three parts is still an error.
	if len(parts) > 3 || len(parts) == 0 {
		return v, fmt.Errorf("version %q: want one to three numeric parts", tag)
	}
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	for i, p := range parts {
		n, err := versionNumber(p)
		if err != nil {
			return v, fmt.Errorf("version %q: %w", tag, err)
		}
		switch i {
		case 0:
			v.Major = n
		case 1:
			v.Minor = n
		case 2:
			v.Patch = n
		}
	}
	return v, nil
}

// versionNumber reads one dotted number, rejecting signs, leading zeros and
// values that do not fit in an int.
func versionNumber(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty number")
	}
	if len(s) > 1 && s[0] == '0' {
		return 0, fmt.Errorf("leading zero in %q", s)
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, fmt.Errorf("%q is not a number", s)
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("number %q is too large", s)
	}
	return n, nil
}

// checkIdentifiers validates a dot-separated prerelease or build list.
func checkIdentifiers(s string, build bool) error {
	if s == "" {
		return fmt.Errorf("empty")
	}
	for _, id := range strings.Split(s, ".") {
		if id == "" {
			return fmt.Errorf("empty identifier")
		}
		for i := 0; i < len(id); i++ {
			c := id[i]
			if c == '-' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				continue
			}
			return fmt.Errorf("bad character %q in %q", c, id)
		}
		if !build && isNumeric(id) && len(id) > 1 && id[0] == '0' {
			return fmt.Errorf("leading zero in %q", id)
		}
	}
	return nil
}

func isNumeric(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// Compare orders two versions: -1 if v is older, +1 if newer, 0 if equal.
func (v Version) Compare(o Version) int {
	if c := compareInt(v.Major, o.Major); c != 0 {
		return c
	}
	if c := compareInt(v.Minor, o.Minor); c != 0 {
		return c
	}
	if c := compareInt(v.Patch, o.Patch); c != 0 {
		return c
	}
	switch {
	case len(v.Pre) == 0 && len(o.Pre) == 0:
		return 0
	case len(v.Pre) == 0:
		return 1 // a release beats its own pre-release
	case len(o.Pre) == 0:
		return -1
	}
	for i := 0; i < len(v.Pre) && i < len(o.Pre); i++ {
		if c := compareIdentifier(v.Pre[i], o.Pre[i]); c != 0 {
			return c
		}
	}
	return compareInt(len(v.Pre), len(o.Pre))
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// compareIdentifier orders prerelease identifiers: numeric ones by value and
// below alphanumeric ones, as SemVer does.
func compareIdentifier(a, b string) int {
	an, bn := isNumeric(a), isNumeric(b)
	switch {
	case an && bn:
		if len(a) != len(b) {
			return compareInt(len(a), len(b))
		}
		return strings.Compare(a, b)
	case an:
		return -1
	case bn:
		return 1
	}
	return strings.Compare(a, b)
}

// Newer reports whether version a is newer than b ("0.1.1" > "0.1.1-beta" >
// "0.1.0"). A tag that does not parse is never newer than anything.
func Newer(a, b string) bool {
	va, err := ParseVersion(a)
	if err != nil {
		return false
	}
	vb, err := ParseVersion(b)
	if err != nil {
		return false
	}
	return va.Compare(vb) > 0
}

// parseVersion and newerThan are the terse forms used by the replay check;
// they keep the single strict parser above as the source of truth.
func parseVersion(tag string) (Version, bool) {
	v, err := ParseVersion(tag)
	return v, err == nil
}

func (v Version) newerThan(o Version) bool { return v.Compare(o) > 0 }

// String renders the canonical tag form.
func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if len(v.Pre) > 0 {
		s += "-" + strings.Join(v.Pre, ".")
	}
	return s
}
