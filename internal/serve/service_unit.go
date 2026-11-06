package serve

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// unitPathIn is the systemd user unit's path inside a config directory.
func unitPathIn(configDir string) string {
	return filepath.Join(configDir, "systemd", "user", unitName())
}

// unitName is the systemd user unit (see testSuffix).
func unitName() string { return "ezlm" + testSuffix() + ".service" }

// quote is one ExecStart argument in systemd's unit-file syntax: the value is
// double-quoted, backslashes and quotes are escaped, and a literal % is
// doubled because systemd expands % specifiers even inside quotes.
func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "%", "%%").Replace(s) + `"`
}

// unitEscape protects a literal value in a setting that takes one string
// (StandardOutput=, StandardError=) from % specifier expansion. A home
// directory can contain a %.
func unitEscape(s string) string { return strings.ReplaceAll(s, "%", "%%") }

// libPath lists every directory under the engine install holding shared
// libraries, so the CUDA runtime unpacked beside llama-server is found.
func libPath(server string) string {
	dir := filepath.Dir(server)
	root := dir
	// The engine lives in <home>/.ezlm/bin/<tag>/, but an older install put
	// llama-server straight in bin/. Stop at bin/ rather than walking up past
	// it (which, for a flat install, meant scanning the whole file system).
	for filepath.Base(root) != "bin" {
		parent := filepath.Dir(root)
		if parent == root || filepath.Base(parent) == "bin" {
			break
		}
		root = parent
	}
	if filepath.Base(root) != "bin" && filepath.Base(filepath.Dir(root)) != "bin" {
		root = dir // no engine install above this server: look beside it only
	}
	seen := map[string]bool{dir: true}
	dirs := []string{dir}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(d.Name(), ".so") && !seen[filepath.Dir(p)] {
			seen[filepath.Dir(p)] = true
			dirs = append(dirs, filepath.Dir(p))
		}
		return nil
	})
	return strings.Join(dirs, ":")
}

// unitFile is the systemd user unit: restart on crash, start at login and
// boot, and append the server's output to EZLM's log.
func unitFile(c *Config) string {
	var cmd []string
	for _, a := range append([]string{c.Server}, c.Args()...) {
		cmd = append(cmd, quote(a))
	}
	// Environment= takes whitespace-separated assignments, so the value is
	// quoted to keep a path with a space in it one assignment.
	env := quote("LD_LIBRARY_PATH=" + libPath(c.Server))
	logPath := unitEscape(LogPath())
	return fmt.Sprintf(`[Unit]
Description=EZLM local LLM server
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=%s
Environment=%s
Restart=on-failure
RestartSec=10
StandardOutput=append:%s
StandardError=append:%s

[Install]
WantedBy=default.target
`, strings.Join(cmd, " "), env, logPath, logPath)
}
