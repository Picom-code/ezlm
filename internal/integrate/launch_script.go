package integrate

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// The launchers, built here for every OS so the golden tests cover all three
// from any machine (testdata/launch). terminal_*.go only opens them.

// launchFile is the launcher for goos: its path in LaunchDir and its bytes.
// exit is the exit file it writes.
func launchFile(goos string, spec LaunchSpec, exit string) (string, []byte, error) {
	switch goos {
	case "windows":
		s, err := launchCmdScript(spec, exit)
		return filepath.Join(LaunchDir(), spec.ID+".cmd"), []byte(s), err
	case "darwin":
		s, err := launchShScript(spec, exit, true)
		return filepath.Join(LaunchDir(), spec.ID+".command"), []byte(s), err
	default:
		s, err := launchShScript(spec, exit, false)
		return filepath.Join(LaunchDir(), spec.ID+".sh"), []byte(s), err
	}
}

// launchBare: arguments made only of these characters (flags, subcommands)
// are written without quotes; everything else is quoted.
var launchBare = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// launchStopped is what the window says when the app ends with an error.
const launchStopped = "EZLM shows what went wrong."

// ---- Mac (.command, zsh) and Linux (.sh, sh) ----

// launchShQuote quotes s for a POSIX shell in single quotes; a quote inside
// closes them, adds an escaped quote and opens them again.
func launchShQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// launchShWord quotes an argument unless it is a plain flag or word.
func launchShWord(s string) string {
	if launchBare.MatchString(s) {
		return s
	}
	return launchShQuote(s)
}

// launchShDQ escapes s for the inside of "…".
func launchShDQ(s string) string {
	return strings.NewReplacer(`\`, `\\`, `$`, `\$`, `"`, `\"`, "`", "\\`").Replace(s)
}

// launchShScript is the Mac .command (zsh) or Linux .sh (sh) launcher:
//
//	#!/bin/zsh
//	{
//	printf '\033]0;%s\007' 'OpenCode (EZLM)'
//	cd '/Users/sam/EZLM Workspace' || { …exit file…; exit 1; }
//	'/Users/sam/.ezlm/tools/opencode/2.2.5/opencode' --standalone --model 'ezlm/qwen3.5-4b'
//	code=$?
//	print -r -- "$code $(date +%s)" > '/Users/sam/.ezlm/launch/opencode.exit'
//	if (( code != 0 )); then …message…; read -sk1; fi
//	exit $code
//	}
//
// The braces make the shell read the whole script before running the app.
func launchShScript(spec LaunchSpec, exit string, zsh bool) (string, error) {
	var b strings.Builder
	line := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	if zsh {
		line("#!/bin/zsh")
	} else {
		line("#!/bin/sh")
	}
	line("{")
	line(`printf '\033]0;%%s\007' %s`, launchShQuote(launchOneLine(spec.Title)))
	for _, e := range spec.Env {
		name, parts := launchSplitEnv(e)
		if strings.ContainsAny(e, "\r\n\x00") {
			return "", fmt.Errorf("EZLM can't start %s with the setting %s", launchName(spec), name)
		}
		var v strings.Builder
		for _, p := range parts {
			if p.Ref != "" {
				v.WriteString(`"${` + p.Ref + `}"`)
			} else {
				v.WriteString(launchShQuote(p.Text))
			}
		}
		if len(parts) == 0 {
			v.WriteString("''")
		}
		line("export %s=%s", name, v.String())
	}
	record := func(code string) string {
		if zsh {
			return fmt.Sprintf(`print -r -- "%s $(date +%%s)" > %s`, code, launchShQuote(exit))
		}
		return fmt.Sprintf(`printf '%%s %%s\n' "%s" "$(date +%%s)" > %s`, code, launchShQuote(exit))
	}
	line("cd %s || { %s; exit 1; }", launchShQuote(spec.Dir), record("1"))
	cmd := []string{launchShQuote(spec.Bin)}
	for _, a := range spec.Args {
		if strings.ContainsAny(a, "\x00") {
			return "", fmt.Errorf("EZLM can't start %s with that argument", launchName(spec))
		}
		cmd = append(cmd, launchShWord(a))
	}
	line("%s", strings.Join(cmd, " "))
	line("code=$?")
	line("%s", record("$code"))
	msg := launchShDQ(launchName(spec)) + " stopped (exit $code). " + launchStopped
	if zsh {
		line(`if (( code != 0 )); then print; print -r -- "%s"; print "Press any key to close this window."; read -sk1; fi`, msg)
	} else {
		line(`if [ "$code" -ne 0 ]; then echo; printf '%%s\n' "%s"; echo "Press Enter to close this window."; read ezlm_key; fi`, msg)
	}
	line("exit $code")
	line("}")
	return b.String(), nil
}

// launchOneLine keeps a title on one line.
func launchOneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// ---- Windows (.cmd) ----

// launchCmdScript is the Windows launcher, UTF-8 without a BOM and CRLF line
// ends. cmd.exe reads a batch file in the console's code page, so the first
// two lines are ASCII and switch it to UTF-8 (65001) before any path appears.
// % is doubled everywhere it is meant literally.
//
// cmd.exe reopens a running batch file by name for every line it reads, so
// everything from the app's start to the end sits in one parenthesised
// block: it is read once, before the app starts, and a launcher rewritten
// while this window is open can't change what it does afterwards. Inside
// the block the app's exit code is read with call (a second expansion when
// the line runs) rather than %ERRORLEVEL%, which a block expands up front.
func launchCmdScript(spec LaunchSpec, exit string) (string, error) {
	var b strings.Builder
	line := func(s string) { b.WriteString(s + "\r\n") }
	line("@echo off")
	line("chcp 65001 >nul")
	line("title " + launchCmdText(launchOneLine(spec.Title)))
	for _, e := range spec.Env {
		name, parts := launchSplitEnv(e)
		if strings.ContainsAny(e, "\"\r\n\x00") {
			return "", fmt.Errorf("EZLM can't start %s with the setting %s", launchName(spec), name)
		}
		var v strings.Builder
		for _, p := range parts {
			if p.Ref != "" {
				v.WriteString("%" + p.Ref + "%")
			} else {
				v.WriteString(launchCmdPct(p.Text, 1))
			}
		}
		line(`set "` + name + "=" + v.String() + `"`)
	}
	ex := `"` + launchCmdPct(exit, 1) + `"`
	line(`cd /d "` + launchCmdPct(spec.Dir, 1) + `" || (>` + ex + ` echo 1& exit /b 1)`)
	call := launchIsBatch(spec.Bin)
	pct := 1
	if call {
		// call expands % once more: %%%% reaches the program as %.
		pct = 2
	}
	words := []string{`"` + launchCmdPct(spec.Bin, pct) + `"`}
	for _, a := range spec.Args {
		if strings.ContainsAny(a, "\r\n\x00") {
			return "", fmt.Errorf("EZLM can't start %s with that argument", launchName(spec))
		}
		words = append(words, launchCmdArg(a, pct))
	}
	run := strings.Join(words, " ")
	if call {
		run = "call " + run
	}
	msg := launchCmdText(launchName(spec) + " stopped. " + launchStopped)
	failed := []string{
		`(call echo %%ERRORLEVEL%%)>` + ex,
		"echo.",
		"echo " + msg,
		"pause",
	}
	line("(")
	line(run)
	line("if errorlevel 1 (")
	for _, l := range failed {
		line(l)
	}
	line(") else if not errorlevel 0 (")
	for _, l := range failed {
		line(l)
	}
	line(") else (")
	line(">" + ex + " echo 0")
	line(")")
	line("exit /b")
	line(")")
	return b.String(), nil
}

// launchIsBatch: a .cmd or .bat program (an npm shim) has to be started with
// call, or the launcher would end when it does.
func launchIsBatch(bin string) bool {
	_, base := launchWinSplit(bin)
	ext := strings.ToLower(filepath.Ext(base))
	return ext == ".cmd" || ext == ".bat"
}

// launchWinSplit splits a Windows path into its folder and file name, on
// any OS (the golden tests build Windows launchers on a Mac).
func launchWinSplit(p string) (dir, base string) {
	i := strings.LastIndexAny(p, `\/`)
	if i < 0 {
		return ".", p
	}
	dir = p[:i]
	if dir == "" || strings.HasSuffix(dir, ":") {
		dir += p[i : i+1]
	}
	return dir, p[i+1:]
}

// launchCmdPct doubles % for cmd.exe's expansion, n times over.
func launchCmdPct(s string, n int) string {
	for range n {
		s = strings.ReplaceAll(s, "%", "%%")
	}
	return s
}

// launchCmdText escapes text that cmd.exe reads outside quotes (title, echo).
func launchCmdText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&', '|', '<', '>', '^', '(', ')':
			b.WriteRune('^')
			b.WriteRune(r)
		case '%':
			b.WriteString("%%")
		case '"':
			b.WriteRune('\'')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// launchCmdArg writes one argument for a program started from a batch file.
// The program splits its command line the way the C runtime does (a " inside
// an argument is \", backslashes before it doubled), and cmd.exe must see
// the same quotes: where an escaped quote leaves cmd.exe outside quotes, its
// special characters get a caret. % is doubled pct times.
//
// A ^ inside quotes reaches a .cmd program doubled (call's own rule); no
// escape avoids that, and nothing EZLM launches passes one.
func launchCmdArg(a string, pct int) string {
	if launchBare.MatchString(a) {
		return a
	}
	crt := launchCRTQuote(a)
	var b strings.Builder
	inQuote := false
	for i := 0; i < len(crt); i++ {
		c := crt[i]
		switch {
		case c == '"':
			// \" is a literal quote for the program but still toggles
			// cmd.exe's view.
			inQuote = !inQuote
			b.WriteByte(c)
		case c == '%':
			b.WriteString(launchCmdPct("%", pct))
		case !inQuote && strings.IndexByte("&|<>^()", c) >= 0:
			b.WriteByte('^')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// launchCRTQuote quotes s for CommandLineToArgvW and the C runtime: always in
// quotes, backslashes doubled before a quote and at the end, " as \".
func launchCRTQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			slashes++
		case '"':
			b.WriteString(strings.Repeat(`\`, slashes*2+1))
			slashes = 0
		default:
			if slashes > 0 {
				b.WriteString(strings.Repeat(`\`, slashes))
				slashes = 0
			}
		}
		if c != '\\' {
			b.WriteByte(c)
		}
	}
	b.WriteString(strings.Repeat(`\`, slashes*2))
	b.WriteByte('"')
	return b.String()
}

// ---- how the launcher is opened ----

// launchMacArgv opens a .command in a new Terminal window. open needs no
// AppleEvents permission, and its exit status says whether Terminal took it.
func launchMacArgv(launcher string) []string {
	return []string{"open", "-a", "Terminal", launcher}
}

// launchLinuxTerminals are tried in order; each runs the .sh as its command.
var launchLinuxTerminals = [][]string{
	{"x-terminal-emulator", "-e"},
	{"gnome-terminal", "--"},
	{"konsole", "-e"},
	{"xterm", "-e"},
}

// launchLinuxArgv is the command for the first terminal look finds.
func launchLinuxArgv(launcher string, look func(string) (string, error)) ([]string, error) {
	for _, t := range launchLinuxTerminals {
		if p, err := look(t[0]); err == nil {
			return append(append([]string{p}, t[1:]...), launcher), nil
		}
	}
	return nil, errLaunchNoTerminal
}

// launchWTArgs are Windows Terminal's arguments (after wt.exe): a new window
// whose tab runs the launcher. The tab starts in the launch folder and runs
// the launcher by its bare name, so no user path goes through Windows
// Terminal's command line rebuilding or cmd.exe's quote rules; the launcher
// changes to the workspace itself. A ; separates Windows Terminal commands,
// so it is escaped in the title.
func launchWTArgs(title, launcher string) []string {
	dir, base := launchWinSplit(launcher)
	return []string{
		"-w", "new", "new-tab",
		"--title", strings.ReplaceAll(launchOneLine(title), ";", `\;`),
		"-d", dir,
		"cmd.exe", "/k", `.\` + base,
	}
}

// launchStartLine is the fallback when Windows Terminal is missing or fails:
// cmd.exe's own start opens a console window running the launcher. It is the
// whole command line of a hidden cmd.exe.
func launchStartLine(title, launcher string) string {
	t := strings.ReplaceAll(launchOneLine(title), `"`, `'`)
	dir, base := launchWinSplit(launcher)
	return fmt.Sprintf(`cmd.exe /d /c start "%s" /D "%s" cmd.exe /k .\%s`, t, dir, base)
}

// launchWindowsOpen tries Windows Terminal (when wt is set), then cmd.exe's
// start. startWT and startCmd run the commands; the tests inject them.
func launchWindowsOpen(wt, title, launcher string, startWT func(wt string, args []string) error, startCmd func(line string) error) (string, error) {
	if wt != "" {
		if err := startWT(wt, launchWTArgs(title, launcher)); err == nil {
			return "wt", nil
		}
	}
	if err := startCmd(launchStartLine(title, launcher)); err != nil {
		return "", err
	}
	return "start", nil
}
