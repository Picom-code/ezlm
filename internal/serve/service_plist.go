package serve

import (
	"os"
	"path/filepath"
	"strings"
)

// plistContent is the launchd agent for the server. It lives in a file without
// a GOOS suffix so its output can be checked on any machine (see
// launchd_plist_test.go); service_darwin.go writes and loads it.
//
// ProgramArguments is an array, so a path with spaces, quotes or unicode is
// one element and needs no shell quoting. Every string is still XML-escaped:
// launchd rejects the whole file on one bad character, and the server then
// never starts.
func plistContent(label, server string, args []string, logPath string) string {
	var b strings.Builder
	for _, a := range append([]string{server}, args...) {
		b.WriteString("\t\t<string>")
		b.WriteString(plistEscape(a))
		b.WriteString("</string>\n")
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + plistEscape(label) + `</string>
	<key>ProgramArguments</key>
	<array>
` + b.String() + `	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>ThrottleInterval</key>
	<integer>10</integer>
	<key>StandardOutPath</key>
	<string>` + plistEscape(logPath) + `</string>
	<key>StandardErrorPath</key>
	<string>` + plistEscape(logPath) + `</string>
</dict>
</plist>
`
}

// plistEscape makes a string safe for a plist XML text node. XML 1.0 has no
// escape for control characters other than tab, newline and carriage return,
// so they become spaces: one of them in a path would otherwise make launchd
// reject the agent.
func plistEscape(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		case '\t', '\n', '\r':
			b.WriteRune(r)
		default:
			if r < 0x20 || r == 0xfffe || r == 0xffff {
				b.WriteRune(' ')
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// writePlistFile writes the agent and prepares its log file. launchd creates
// StandardOutPath but not its folder, and a job whose log file can't be
// opened does not start at all.
func writePlistFile(plistPath, content, logPath string) error {
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return err
	}
	if err := ensureLog(logPath); err != nil {
		return err
	}
	return os.WriteFile(plistPath, []byte(content), 0o600)
}

// ensureLog creates the log file and its folder, and empties it.
func ensureLog(logPath string) error {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}
