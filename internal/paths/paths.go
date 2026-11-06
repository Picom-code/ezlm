// Package paths centralizes where EZLM keeps things on disk.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

// Home is ~/.ezlm (override with EZLM_HOME for tests).
func Home() string {
	if h := os.Getenv("EZLM_HOME"); h != "" {
		return filepath.Clean(h)
	}
	u, err := os.UserHomeDir()
	if err != nil || u == "" {
		// Windows always has a per-user folder even when the profile
		// variables are missing (a service, or an environment stripped by a
		// launcher); a relative ".ezlm" would land in whatever folder the
		// app happened to start from.
		if runtime.GOOS == "windows" {
			for _, v := range []string{"LOCALAPPDATA", "APPDATA"} {
				if d := os.Getenv(v); d != "" {
					return filepath.Join(d, "EZLM")
				}
			}
		}
		u = "."
	}
	return filepath.Join(u, ".ezlm")
}

func Bin() string { return filepath.Join(Home(), "bin") }

// modelsDir overrides where weights go (set when another drive has more room).
var modelsDir string

// SetModels points model storage elsewhere ("" restores the default).
func SetModels(dir string) { modelsDir = dir }

func Models() string {
	if modelsDir != "" {
		return modelsDir
	}
	return filepath.Join(Home(), "models")
}
func Logs() string   { return filepath.Join(Home(), "logs") }
func Config() string { return filepath.Join(Home(), "config.json") }

// Ensure creates the EZLM directory tree. Home is owner-only: config.json
// there holds the API key, so other local users must not list or traverse it.
func Ensure() error {
	if err := os.MkdirAll(Home(), 0o700); err != nil {
		return err
	}
	// Installs from older versions made ~/.ezlm world-readable; repair them.
	if err := os.Chmod(Home(), 0o700); err != nil {
		return err
	}
	for _, d := range []string{Bin(), Models(), Logs()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}
