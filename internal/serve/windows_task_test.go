package serve

import (
	"strings"
	"testing"
)

// The Run key value is one command line that Explorer hands to Windows at
// login. A missing quote in a profile with a space or a non-ASCII name
// starts nothing, so the exact text matters.
func TestRunKeyCommand(t *testing.T) {
	cases := []struct {
		name      string
		self, app string
		appExists bool
		want      string
	}{
		{
			name:      "app with spaces and unicode",
			self:      `C:\Users\José Müller\AppData\Local\Programs\EZLM\Engine\ezlm.exe`,
			app:       `C:\Users\José Müller\AppData\Local\Programs\EZLM\EZLM.exe`,
			appExists: true,
			want:      `"C:\Users\José Müller\AppData\Local\Programs\EZLM\EZLM.exe" --autostart`,
		},
		{
			name:      "engine without spaces needs no quotes",
			self:      `C:\EZLM\Engine\ezlm.exe`,
			app:       `C:\EZLM\EZLM.exe`,
			appExists: true,
			want:      `C:\EZLM\EZLM.exe --autostart`,
		},
		{
			name:      "cli-only install starts the engine",
			self:      `C:\Program Files\EZLM\Engine\ezlm.exe`,
			app:       `C:\Program Files\EZLM\EZLM.exe`,
			appExists: false,
			want:      `"C:\Program Files\EZLM\Engine\ezlm.exe" start`,
		},
		{
			name:      "unicode user profile without spaces",
			self:      `C:\Users\王\EZLM\Engine\ezlm.exe`,
			app:       `C:\Users\王\EZLM\EZLM.exe`,
			appExists: false,
			want:      `C:\Users\王\EZLM\Engine\ezlm.exe start`,
		},
	}
	for _, tc := range cases {
		if got := runKeyCommand(tc.self, tc.app, tc.appExists); got != tc.want {
			t.Errorf("%s: runKeyCommand() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// escapeArg follows the CommandLineToArgvW rules that syscall.EscapeArg uses.
func TestEscapeArg(t *testing.T) {
	cases := map[string]string{
		"":                            `""`,
		`C:\EZLM\ezlm.exe`:            `C:\EZLM\ezlm.exe`,
		`C:\Program Files\EZLM\x.exe`: `"C:\Program Files\EZLM\x.exe"`,
		`C:\Users\José Müller\x.exe`:  `"C:\Users\José Müller\x.exe"`,
		`C:\dir with space\trailing\`: `"C:\dir with space\trailing\\"`,
		"plain":                       "plain",
		`C:\a"b\c.exe`:                `C:\a\"b\c.exe`,
	}
	for in, want := range cases {
		if got := escapeArg(in); got != want {
			t.Errorf("escapeArg(%q) = %q, want %q", in, got, want)
		}
	}
}

// The login entry must pass --autostart as its own argument, never glued to
// the executable path.
func TestRunKeyCommandSeparatesFlag(t *testing.T) {
	got := runKeyCommand(`C:\EZLM\Engine\ezlm.exe`, `C:\EZLM\EZLM.exe`, true)
	if !strings.HasSuffix(got, `" --autostart`) && !strings.HasSuffix(got, ` --autostart`) {
		t.Fatalf("--autostart is not a separate argument: %q", got)
	}
}
