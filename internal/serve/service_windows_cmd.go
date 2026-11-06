package serve

import (
	"strings"
)

// This file holds the Windows login-entry strings as pure functions so they can
// be tested on any OS (the Windows build still compiles them, unchanged).

// legacyTaskNames are the Task Scheduler names older versions created. They
// embed the account name, which may hold spaces or letters outside the ANSI
// code page; schtasks receives them as separate argv strings, so no extra
// quoting is needed.
func legacyTaskNames(user string) []string {
	return []string{"EZLM Server", "EZLM Server (" + user + ")"}
}

// loginCommand is the HKCU Run value. It prefers the desktop app when it sits
// next to the engine (the app starts the engine with --login and exits), and
// falls back to the engine itself for a CLI-only install. Both paths carry the
// login switch, so a server the user stopped stays stopped until they start it.
func loginCommand(self string, appExists func(string) bool) string {
	app := winJoin(winDir(winDir(self)), "EZLM.exe")
	if appExists(app) {
		return winQuoteArg(app) + " --autostart"
	}
	return winQuoteArg(self) + " start --login"
}

// winDir and winJoin are filepath.Dir and filepath.Join with Windows
// separators, so a Windows path can be exercised on any OS. On Windows they
// give the same answer for the absolute paths EZLM installs under.
func winDir(p string) string {
	p = strings.TrimRight(p, `\/`)
	i := strings.LastIndexAny(p, `\/`)
	if i < 0 {
		return "."
	}
	dir := p[:i]
	if len(dir) == 2 && dir[1] == ':' {
		return dir + `\`
	}
	if dir == "" {
		return p[:1]
	}
	return dir
}

func winJoin(dir, name string) string {
	if dir == "" || dir == "." {
		return name
	}
	if strings.HasSuffix(dir, `\`) || strings.HasSuffix(dir, "/") {
		return dir + name
	}
	return dir + `\` + name
}

// winQuoteArg quotes one argument the way CommandLineToArgvW parses it: wrap
// in quotes when it holds a space or tab, and escape backslashes that precede
// a quote. Unicode passes through unchanged. It mirrors syscall.EscapeArg,
// which only compiles on Windows; keeping the same code here lets the login
// command be tested on any OS.
func winQuoteArg(s string) string {
	if s == "" {
		return `""`
	}
	needsBackslash, hasSpace := false, false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"', '\\':
			needsBackslash = true
		case ' ', '\t':
			hasSpace = true
		}
	}
	if !needsBackslash && !hasSpace {
		return s
	}
	var b strings.Builder
	if hasSpace {
		b.WriteByte('"')
	}
	slashes := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		default:
			slashes = 0
		case '\\':
			slashes++
		case '"':
			b.WriteString(strings.Repeat(`\`, slashes+1))
		}
		b.WriteByte(c)
	}
	if hasSpace {
		b.WriteString(strings.Repeat(`\`, slashes))
		b.WriteByte('"')
	}
	return b.String()
}
