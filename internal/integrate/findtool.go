package integrate

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// FindTool finds a program the way the user's shell would, since apps
// launched by double-click often get a minimal PATH. The search order:
//
//  1. the PATH of this process;
//  2. the PATH the user really has: on Mac and Linux the login shell's
//     (cached, see loginPath), on Windows the User and Machine PATH from the
//     registry, which installers change without touching running programs;
//  3. extraDirs (a tool's own folders, e.g. its EZLM-installed copy);
//  4. the usual per-user install folders, then on Windows the WindowsApps
//     folder that holds MSIX app aliases (Hermes Agent desktop).
//
// It returns "" when nothing is found.
//
// EZLM_TOOL_PATH (a PATH-style list) replaces the whole search, so tests
// never find the real programs on the machine; set but empty finds nothing.
func FindTool(name string, extraDirs ...string) string {
	if all := FindAllTools(name, extraDirs...); len(all) > 0 {
		return all[0]
	}
	return ""
}

// FindAllTools is every match in search order, without duplicates (for
// choices such as "the user's opencode or EZLM's").
func FindAllTools(name string, extraDirs ...string) []string {
	if only, ok := os.LookupEnv("EZLM_TOOL_PATH"); ok {
		return dedupe(matches(name, filepath.SplitList(only)))
	}
	var first []string
	if p, err := exec.LookPath(name); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			first = append(first, abs)
		}
	}
	dirs := filepath.SplitList(os.Getenv("PATH"))
	dirs = append(dirs, userPath()...)
	dirs = append(dirs, extraDirs...)
	dirs = append(dirs, toolDirs()...)
	return dedupe(append(first, matches(name, dirs)...))
}

// SearchDirs is the folder list FindAllTools searches after the process
// PATH (for logs and ezlm doctor apps).
func SearchDirs(extraDirs ...string) []string {
	if only, ok := os.LookupEnv("EZLM_TOOL_PATH"); ok {
		return filepath.SplitList(only)
	}
	dirs := append(filepath.SplitList(os.Getenv("PATH")), userPath()...)
	dirs = append(dirs, extraDirs...)
	return dedupeDirs(append(dirs, toolDirs()...))
}

// toolDirs are the usual per-user install folders.
func toolDirs() []string {
	home, _ := os.UserHomeDir()
	dirs := []string{
		".local/bin", ".opencode/bin", "bin", ".bun/bin", ".npm-global/bin", ".volta/bin",
		".local/share/mise/shims", ".hermes/hermes-agent/.hermes/bin",
	}
	if hh := os.Getenv("HERMES_HOME"); hh != "" {
		dirs = append(dirs, filepath.Join(hh, "bin"))
	}
	switch runtime.GOOS {
	case "darwin":
		dirs = append(dirs, "Library/pnpm", "/opt/homebrew/bin", "/usr/local/bin")
	case "windows":
		la, ad := os.Getenv("LOCALAPPDATA"), os.Getenv("APPDATA")
		if la != "" {
			dirs = append(dirs, filepath.Join(la, "Programs", "opencode"), filepath.Join(la, "hermes", "bin"), filepath.Join(la, "pnpm"))
		}
		if ad != "" {
			dirs = append(dirs, filepath.Join(ad, "npm"))
		}
		dirs = append(dirs, "scoop/shims")
		if la != "" {
			dirs = append(dirs, filepath.Join(la, "Microsoft", "WindowsApps"))
		}
	default:
		dirs = append(dirs, ".local/share/pnpm", "/usr/local/bin")
	}
	for i, d := range dirs {
		if !filepath.IsAbs(d) && !strings.HasPrefix(d, "/") {
			dirs[i] = filepath.Join(home, d)
		}
	}
	return dirs
}

func matches(name string, dirs []string) []string {
	var out []string
	for _, d := range dirs {
		if d == "" {
			continue
		}
		for _, ext := range exts() {
			p := filepath.Join(d, name+ext)
			if isProgram(p) {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// isProgram: a file (not a folder) exists at p. On Windows an MSIX app alias
// is a zero-byte reparse point that os.Stat can't follow, so Lstat decides.
func isProgram(p string) bool {
	if st, err := os.Stat(p); err == nil {
		return !st.IsDir()
	}
	if runtime.GOOS != "windows" {
		return false
	}
	st, err := os.Lstat(p)
	return err == nil && !st.IsDir()
}

// IsAppAlias reports whether bin is a Windows MSIX app execution alias (in
// %LOCALAPPDATA%\Microsoft\WindowsApps). Such a program can only be started
// through CreateProcess; its header can't be read.
func IsAppAlias(bin string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	return strings.EqualFold(filepath.Base(filepath.Dir(bin)), "WindowsApps")
}

func dedupe(list []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range list {
		k := filepath.Clean(p)
		if runtime.GOOS == "windows" {
			k = strings.ToLower(k)
		}
		if !seen[k] {
			seen[k] = true
			out = append(out, p)
		}
	}
	return out
}

func dedupeDirs(list []string) []string {
	var keep []string
	for _, d := range list {
		if d != "" {
			keep = append(keep, d)
		}
	}
	return dedupe(keep)
}

func exts() []string {
	if runtime.GOOS == "windows" {
		return []string{".exe", ".cmd", ".bat"}
	}
	return []string{""}
}
