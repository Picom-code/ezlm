package serve

// The Windows login entry is a command line stored in HKCU's Run key. It is
// built here, outside the windows-only file, so its quoting can be tested on
// any OS: a wrong quote turns a profile like C:\Users\José Müller into a
// command that starts nothing at login.

// runKeyCommand is the value written to HKCU\Software\Microsoft\Windows\
// CurrentVersion\Run. The desktop app is preferred (it starts the server
// without a console window); a CLI-only install starts the engine itself.
func runKeyCommand(self, app string, appExists bool) string {
	if appExists {
		return escapeArg(app) + " --autostart"
	}
	return escapeArg(self) + " start"
}

// escapeArg quotes one argument as CommandLineToArgvW reads it. It follows
// the rules of syscall.EscapeArg (Go's copy, BSD licence): quote only when
// the argument has a space or tab, double backslashes that precede a quote,
// and double a run of backslashes at the end of a quoted argument. It lives
// here so a test on macOS can check the exact command Windows will run.
func escapeArg(s string) string {
	if s == "" {
		return `""`
	}
	special := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"', '\\', ' ', '\t':
			special = true
		}
	}
	if !special {
		return s
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

	var b []byte
	if hasSpace {
		b = append(b, '"')
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
			for ; slashes > 0; slashes-- {
				b = append(b, '\\')
			}
			b = append(b, '\\')
		}
		b = append(b, c)
	}
	if hasSpace {
		for ; slashes > 0; slashes-- {
			b = append(b, '\\')
		}
		b = append(b, '"')
	}
	return string(b)
}
