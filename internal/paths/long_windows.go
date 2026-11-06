package paths

import "strings"

// longPathThreshold is where Windows paths start to need the \\?\ prefix.
// 260 is MAX_PATH; 248 leaves room for an 8.3 name and for the calls that
// append a suffix of their own.
const longPathThreshold = 248

// Extended returns p in the extended-length form Windows needs to open a
// path of longPathThreshold characters or more (\\?\C:\... for a drive,
// \\?\UNC\server\share\... for a network share).
//
// The os package adds the prefix itself, but os/exec does not: it passes
// the path straight to CreateProcess, which still stops at MAX_PATH unless
// the machine opted into long paths. Programs EZLM starts (llama-server, a
// tool under %USERPROFILE%\.ezlm) therefore need the prefix here.
//
// Paths that are short, relative, already extended or device paths are
// returned unchanged. The result is cleaned by hand, because \\?\ turns off
// the normal . and .. handling, and Windows filepath rules are not available
// on the Mac and Linux builds.
func Extended(p string) string {
	if len(p) < longPathThreshold || hasExtendedPrefix(p) {
		return p
	}
	p = cleanWindows(p)
	if len(p) >= 2 && isSep(p[0]) && isSep(p[1]) {
		// \\server\share\... -> \\?\UNC\server\share\...
		return `\\?\UNC\` + p[2:]
	}
	if len(p) >= 3 && isLetter(p[0]) && p[1] == ':' && isSep(p[2]) {
		return `\\?\` + p
	}
	return p // relative, or rooted without a volume: nothing to extend
}

// hasExtendedPrefix reports whether p already starts with \\?\, \??\ or a
// device path such as \\.\PIPE\ (where adding \\?\ would change the meaning).
func hasExtendedPrefix(p string) bool {
	if len(p) < 4 || !isSep(p[0]) || !isSep(p[1]) {
		return false
	}
	return (p[2] == '?' || p[2] == '.') && isSep(p[3])
}

func isSep(c byte) bool    { return c == '\\' || c == '/' }
func isLetter(c byte) bool { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }

// cleanWindows normalizes a Windows path lexically: forward slashes become
// backslashes, repeated separators and . parts disappear, and .. removes the
// part before it. A leading \\ or \ is kept.
func cleanWindows(p string) string {
	p = strings.ReplaceAll(p, "/", `\`)
	unc := strings.HasPrefix(p, `\\`) && !strings.HasPrefix(p, `\\.`) && !strings.HasPrefix(p, `\\?`)
	rooted := strings.HasPrefix(p, `\`)
	if unc {
		p = p[2:]
	} else if rooted {
		p = p[1:]
	}
	var out []string
	for _, part := range strings.Split(p, `\`) {
		switch part {
		case "", ".":
			continue
		case "..":
			if len(out) > 0 && out[len(out)-1] != ".." && !(len(out) == 1 && isDrive(out[0])) {
				out = out[:len(out)-1]
				continue
			}
			out = append(out, part)
		default:
			out = append(out, part)
		}
	}
	joined := strings.Join(out, `\`)
	if joined == "" {
		joined = `\`
	}
	if len(joined) == 2 && isDrive(joined) {
		joined += `\`
	}
	switch {
	case unc:
		return `\\` + joined
	case rooted:
		return `\` + joined
	}
	return joined
}

func isDrive(p string) bool { return len(p) == 2 && isLetter(p[0]) && p[1] == ':' }
