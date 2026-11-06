package serve

import (
	"reflect"
	"strings"
	"testing"
)

// splitWinCmdline parses a Windows command line the way CommandLineToArgvW
// does, for the quoting cases the login entry can produce. It is the oracle
// for the round-trip tests below: a generated command must split back into the
// exact arguments Windows will hand the program.
func splitWinCmdline(s string) []string {
	var args []string
	var cur strings.Builder
	inQuotes, started := false, false
	flush := func() {
		if started {
			args = append(args, cur.String())
			cur.Reset()
			started = false
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\':
			j := i
			for j < len(s) && s[j] == '\\' {
				j++
			}
			n := j - i
			if j < len(s) && s[j] == '"' {
				cur.WriteString(strings.Repeat(`\`, n/2))
				if n%2 == 1 { // an escaped quote, not a delimiter
					cur.WriteByte('"')
					j++
				}
			} else {
				cur.WriteString(strings.Repeat(`\`, n))
			}
			started = true
			i = j - 1
		case c == '"':
			inQuotes = !inQuotes
			started = true
		case (c == ' ' || c == '\t') && !inQuotes:
			flush()
		default:
			cur.WriteByte(c)
			started = true
		}
	}
	flush()
	return args
}

func mustSplit(t *testing.T, cmdline string, want ...string) {
	t.Helper()
	got := splitWinCmdline(cmdline)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("command line %q splits to %q, want %q", cmdline, got, want)
	}
}

// A user profile with spaces and letters outside the ANSI code page, as under
// C:\Users\José Müller 王, must reach the program as one argument.
func TestLoginCommandUnicodeProfile(t *testing.T) {
	self := `C:\Users\José Müller 王\AppData\Local\Programs\EZLM\Engine\ezlm.exe`
	app := `C:\Users\José Müller 王\AppData\Local\Programs\EZLM\EZLM.exe`

	mustSplit(t, loginCommand(self, func(string) bool { return true }), app, "--autostart")
	mustSplit(t, loginCommand(self, func(string) bool { return false }), self, "start", "--login")
}

// OneDrive-redirected profiles and network (UNC) profiles put commas, hyphens
// and backslashes in the path. The generated command must still round-trip.
func TestLoginCommandRedirectedProfile(t *testing.T) {
	cases := []string{
		`C:\Users\Sam\OneDrive - Contoso, Ltd\Programs\EZLM\Engine\ezlm.exe`,
		`\\fileserver\home\Sam\Programs\EZLM\Engine\ezlm.exe`,
		`C:\Users\Smith & Jones\AppData\Local\Programs\EZLM\Engine\ezlm.exe`,
		`C:\Users\100%\AppData\Local\Programs\EZLM\Engine\ezlm.exe`,
		`C:\Users\a b\AppData\Local\Programs\EZLM\Engine\ezlm.exe`,
	}
	for _, self := range cases {
		got := splitWinCmdline(loginCommand(self, func(string) bool { return false }))
		want := []string{self, "start", "--login"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("profile %q splits to %q, want %q", self, got, want)
		}
	}
}

// The login switch is what makes a server the user stopped stay stopped. The
// CLI fallback used to generate a bare "start", which restarted it at login.
func TestLoginCommandPassesLoginSwitch(t *testing.T) {
	self := `C:\Users\José\EZLM\Engine\ezlm.exe`
	for _, appExists := range []bool{false, true} {
		args := splitWinCmdline(loginCommand(self, func(string) bool { return appExists }))
		if args[len(args)-1] != "--login" && args[len(args)-1] != "--autostart" {
			t.Fatalf("appExists=%v: command %q has no login switch", appExists, args)
		}
	}
	if args := splitWinCmdline(loginCommand(self, func(string) bool { return false })); args[len(args)-1] != "--login" {
		t.Fatalf("CLI fallback %q does not pass --login", args)
	}
}

// The app is only used when it really sits next to the engine; a moved or
// partially removed install falls back to the engine itself.
func TestLoginCommandAppNextToEngine(t *testing.T) {
	self := `C:\Users\Sam\OneDrive - Contoso\EZLM\Engine\ezlm.exe`
	var asked string
	cmd := loginCommand(self, func(p string) bool { asked = p; return false })
	if asked != `C:\Users\Sam\OneDrive - Contoso\EZLM\EZLM.exe` {
		t.Fatalf("checked %q for the app", asked)
	}
	mustSplit(t, cmd, self, "start", "--login")
}

// Task names are per user and can hold spaces, parentheses and unicode. They
// are passed to schtasks as one argv element, so they must not be quoted here.
func TestLegacyTaskNames(t *testing.T) {
	got := legacyTaskNames("José Müller (王)")
	want := []string{"EZLM Server", "EZLM Server (José Müller (王))"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("task names %q, want %q", got, want)
	}
	for _, name := range got {
		if strings.ContainsAny(name, `"`) {
			t.Fatalf("task name %q must not carry quotes; argv is passed directly", name)
		}
	}
}

// The quoting helper is the Windows argument rule; round-trip the hard cases.
func TestWinQuoteArgRoundTrip(t *testing.T) {
	for _, arg := range []string{
		``,
		`plain`,
		`C:\Users\Sam\EZLM\ezlm.exe`,
		`C:\Users\Sam Smith\EZLM\ezlm.exe`,
		`C:\Users\Sam\EZLM dir\`,
		`a"b`,
		`a b"c`,
		"tab\there",
		`\\server\share\dir with space\`,
	} {
		got := splitWinCmdline(winQuoteArg(arg))
		if len(got) != 1 || got[0] != arg {
			t.Errorf("winQuoteArg(%q) = %q, splits to %q", arg, winQuoteArg(arg), got)
		}
	}
}
