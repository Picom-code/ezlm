package paths

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Settings are the user's choices that apply before anything is deployed.
type Settings struct {
	// ModelsDir is where new downloads go ("" = EZLM picks the roomiest drive).
	ModelsDir string `json:"models_dir,omitempty"`
	// AutoDir is the folder EZLM picked by itself. It sticks: choosing "the
	// roomiest drive" afresh on every scan made downloads jump drives as free
	// space changed (and re-download models that were on the other drive).
	AutoDir string `json:"auto_dir,omitempty"`
	// KnownDirs are every folder EZLM has ever kept models in, so models stay
	// findable (and removable) after the download location changes.
	KnownDirs []string `json:"known_dirs,omitempty"`
}

var settingsMu sync.Mutex

func SettingsPath() string { return filepath.Join(Home(), "settings.json") }

func LoadSettings() Settings {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	var s Settings
	if b, err := os.ReadFile(SettingsPath()); err == nil {
		json.Unmarshal([]byte(strings.TrimPrefix(string(b), "\xef\xbb\xbf")), &s) // tolerate a BOM
	}
	return s
}

func SaveSettings(s Settings) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	if err := os.MkdirAll(Home(), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	if err := os.WriteFile(SettingsPath(), append(b, '\n'), 0o600); err != nil {
		return err
	}
	// WriteFile keeps an existing file's mode; an older 0644 copy stays open.
	return os.Chmod(SettingsPath(), 0o600)
}

// RememberModelsDir adds dir to the known model folders.
func RememberModelsDir(dir string) {
	if dir == "" {
		return
	}
	s := LoadSettings()
	for _, d := range s.KnownDirs {
		if strings.EqualFold(filepath.Clean(d), filepath.Clean(dir)) {
			return
		}
	}
	s.KnownDirs = append(s.KnownDirs, filepath.Clean(dir))
	SaveSettings(s)
}

// DeployedModelsDir is the models folder of the current setup, if any (read
// straight from config.json to stay independent of the serve package).
func DeployedModelsDir() string {
	b, err := os.ReadFile(Config())
	if err != nil {
		return ""
	}
	var c struct {
		ModelsDir string `json:"models_dir"`
	}
	json.Unmarshal([]byte(strings.TrimPrefix(string(b), "\xef\xbb\xbf")), &c)
	return c.ModelsDir
}
