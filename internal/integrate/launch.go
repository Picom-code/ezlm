package integrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/picom-code/ezlm/internal/paths"
)

// LaunchDir holds the launchers EZLM writes and their exit files:
// ~/.ezlm/launch.
func LaunchDir() string { return filepath.Join(paths.Home(), "launch") }

// ExitFile is where id's launcher records how the app ended:
// "<exit code> <unix seconds>\n".
func ExitFile(id string) string { return filepath.Join(LaunchDir(), id+".exit") }

// ReadExit parses an exit file. ok is false while there is none (the app is
// still running, or the launcher doesn't write one). The Windows launcher
// can't tell the time in a locale-proof way, so it writes the code alone and
// at is the file's modification time.
func ReadExit(path string) (code int, at time.Time, ok bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, time.Time{}, false
	}
	f := strings.Fields(strings.TrimPrefix(string(b), "\ufeff"))
	if len(f) == 0 {
		return 0, time.Time{}, false
	}
	code, err = strconv.Atoi(f[0])
	if err != nil {
		return 0, time.Time{}, false
	}
	if len(f) > 1 {
		if s, err := strconv.ParseInt(f[1], 10, 64); err == nil && s > 0 {
			at = time.Unix(s, 0)
		}
	}
	if at.IsZero() {
		if st, err := os.Stat(path); err == nil {
			at = st.ModTime()
		}
	}
	return code, at, true
}

// DryRun: EZLM_LAUNCH_DRYRUN=1 writes the launcher but opens no window or
// browser (tests and the e2e script).
func DryRun() bool { return os.Getenv("EZLM_LAUNCH_DRYRUN") != "" }

// Launch writes spec's launcher (~/.ezlm/launch/<id>.command on Mac, .cmd on
// Windows, .sh elsewhere) and opens it in a new terminal window. The
// launcher sets spec.Env, runs the app in spec.Dir and writes the exit file
// when the app ends, so the caller can tell a crash from a running app. A
// spec with no Bin and a URL only opens the URL in the default browser (an
// app's web UI that is already running). A stale exit file is removed first,
// so the caller's watcher only sees this launch's exit.
//
// Keys never go into a launcher: the apps read them from their own config
// or env files. A spec whose Env or Args look like they carry one is refused.
func Launch(spec LaunchSpec) (Launched, error) {
	if !launchID.MatchString(spec.ID) {
		return Launched{}, fmt.Errorf("EZLM can't open an app with the id %q", spec.ID)
	}
	if err := launchCheckSecrets(spec); err != nil {
		return Launched{}, err
	}
	if spec.Dir == "" {
		spec.Dir = Workspace()
	}
	exit := ExitFile(spec.ID)
	os.Remove(exit)
	if spec.Bin == "" {
		if spec.URL == "" {
			return Launched{}, fmt.Errorf("EZLM doesn't know how to open %s yet", launchName(spec))
		}
		return Launched{DryRun: DryRun()}, OpenURL(spec.URL)
	}
	path, body, err := launchFile(runtime.GOOS, spec, exit)
	if err != nil {
		return Launched{}, err
	}
	// A window opened earlier may still be running this launcher. The shells
	// keep reading the file they opened, so a rename leaves theirs intact;
	// cmd.exe reopens it by name, so the Windows launcher keeps everything
	// after the app's start in one block it has already read.
	if err := WriteFileAtomic(path, body, 0o755); err != nil {
		if err := os.WriteFile(path, body, 0o755); err != nil {
			return Launched{}, err
		}
	}
	l := Launched{Launcher: path, ExitFile: exit, DryRun: DryRun()}
	if l.DryRun {
		return l, nil
	}
	if err := termOpen(spec, path); err != nil {
		return Launched{}, err
	}
	return l, nil
}

// OpenURL opens a page in the default browser (nothing under DryRun).
func OpenURL(url string) error {
	if DryRun() {
		return nil
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("EZLM only opens web pages, not %q", url)
	}
	switch runtime.GOOS {
	case "darwin":
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "open", url).CombinedOutput(); err != nil {
			return fmt.Errorf("macOS couldn't open the browser (%s). Open %s yourself", launchErrText(err, out), url)
		}
		return nil
	case "windows":
		return launchStart(exec.Command("rundll32", "url.dll,FileProtocolHandler", url))
	default:
		if err := launchStart(exec.Command("xdg-open", url)); err != nil {
			return fmt.Errorf("no browser opener was found. Open %s yourself", url)
		}
		return nil
	}
}

// launchStart starts a helper that hands off to another program and reaps
// it in the background.
func launchStart(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

// launchErrText is a short reason for a failed helper: its last output line,
// else the error.
func launchErrText(err error, out []byte) string {
	if line := ErrLine(string(out)); line != "no output" {
		return line
	}
	return err.Error()
}

var (
	launchID      = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	launchEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	launchEnvRef  = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	// A setting whose name has one of these words (between underscores)
	// holds a secret: OPENAI_API_KEY, GH_TOKEN, HERMES_AUTH_FILE.
	launchSecret = regexp.MustCompile(`(?i)(^|_)(APIKEY|KEY|KEYS|TOKEN|SECRET|PASSWORD|PASSWD|PASS|CREDENTIALS?|AUTH|BEARER)(_|$)`)
	// Key shapes, matched against whole words of a value or argument (split
	// at spaces, quotes, separators and path separators), so a folder such
	// as ezlm-integrate-test-123 or sk-learn-projects is not taken for one.
	// EZLM's keys are ezlm- and 40 base62 characters.
	launchEZLMKey   = regexp.MustCompile(`^ezlm-[A-Za-z0-9]{16,}$`)
	launchSKKey     = regexp.MustCompile(`^sk-[A-Za-z0-9_-]{20,}$`)
	launchBearer    = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{8,}`)
	launchWordSplit = regexp.MustCompile(`[\s"'=:;,/\\]+`)
)

// launchName is the app's name for the launcher's messages: the title
// without " (EZLM)".
func launchName(spec LaunchSpec) string {
	if n := strings.TrimSpace(strings.TrimSuffix(spec.Title, " (EZLM)")); n != "" {
		return n
	}
	return spec.ID
}

// launchCheckSecrets refuses a spec that would put a key into a launcher, an
// argument list or a process environment.
func launchCheckSecrets(spec LaunchSpec) error {
	for _, e := range spec.Env {
		name, value, ok := strings.Cut(e, "=")
		if !ok || !launchEnvName.MatchString(name) {
			return fmt.Errorf("EZLM can't start %s with the setting %q", launchName(spec), name)
		}
		if launchSecret.MatchString(name) || launchKeyLike(value) {
			return fmt.Errorf("EZLM won't put a key into the %s launcher (%s); the app reads it from its own settings", launchName(spec), name)
		}
	}
	for _, a := range spec.Args {
		if launchKeyLike(a) {
			return fmt.Errorf("EZLM won't put a key on the %s command line; the app reads it from its own settings", launchName(spec))
		}
	}
	return nil
}

func launchKeyLike(s string) bool {
	if launchBearer.MatchString(s) {
		return true
	}
	for _, w := range launchWordSplit.Split(s, -1) {
		if launchEZLMKey.MatchString(w) || (launchSKKey.MatchString(w) && strings.ContainsAny(w, "0123456789")) {
			return true
		}
	}
	return false
}

// launchEnvPart is one piece of an Env value: literal text, or a reference
// to a variable the terminal already has (Ref).
type launchEnvPart struct{ Text, Ref string }

// launchSplitEnv splits "NAME=value" into the name and the value's parts. A
// value may refer to the inherited value of a variable as ${NAME}, e.g.
// PATH=/x/node/bin:${PATH} (Windows: PATH=C:\x\node;${PATH}). The launchers
// turn that into the shell's own syntax, so the app gets the terminal's
// PATH with EZLM's folder in front; Smoke expands it against EZLM's own.
func launchSplitEnv(e string) (string, []launchEnvPart) {
	name, value, _ := strings.Cut(e, "=")
	var parts []launchEnvPart
	last := 0
	for _, m := range launchEnvRef.FindAllStringSubmatchIndex(value, -1) {
		if m[0] > last {
			parts = append(parts, launchEnvPart{Text: value[last:m[0]]})
		}
		parts = append(parts, launchEnvPart{Ref: value[m[2]:m[3]]})
		last = m[1]
	}
	if last < len(value) {
		parts = append(parts, launchEnvPart{Text: value[last:]})
	}
	return name, parts
}

// launchExpandEnv resolves Env's ${NAME} references against getenv, for
// programs EZLM starts itself (the TUI smoke).
func launchExpandEnv(env []string, getenv func(string) string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		name, parts := launchSplitEnv(e)
		var b strings.Builder
		for _, p := range parts {
			if p.Ref != "" {
				b.WriteString(getenv(p.Ref))
			} else {
				b.WriteString(p.Text)
			}
		}
		out = append(out, name+"="+b.String())
	}
	return out
}

// errLaunchNoTerminal: no terminal program was found (Linux).
var errLaunchNoTerminal = errors.New("no terminal program was found")
